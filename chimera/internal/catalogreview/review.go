// Package catalogreview builds proposal-only provider/model refresh reports.
package catalogreview

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/freecatalog"
	"github.com/lynn/porcelain/chimera/internal/modelcatalog"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// CatalogModel is the evidence used from one BiFrost catalog row.
type CatalogModel struct {
	ID             string   `yaml:"id"`
	ContextLength  int64    `yaml:"context_length,omitempty"`
	Capabilities   []string `yaml:"capabilities,omitempty"`
	CapabilityFrom string   `yaml:"capability_source,omitempty"`
}

// LoadCatalog reads an OpenAI-style YAML catalog snapshot without dropping rows that omit context.
func LoadCatalog(path string) ([]CatalogModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var doc struct {
		Data []CatalogModel `yaml:"data"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	seen := make(map[string]struct{}, len(doc.Data))
	out := make([]CatalogModel, 0, len(doc.Data))
	for _, model := range doc.Data {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// PublishedThroughput keeps public-plan ceilings separate from context and live remaining quota.
type PublishedThroughput struct {
	RPM string `yaml:"rpm,omitempty"`
	RPD string `yaml:"rpd,omitempty"`
	TPM string `yaml:"tpm,omitempty"`
	TPD string `yaml:"tpd,omitempty"`
	ASH string `yaml:"audio_seconds_per_hour,omitempty"`
	ASD string `yaml:"audio_seconds_per_day,omitempty"`
}

// ModelReview records all independent evidence axes and the routing decision for one model.
type ModelReview struct {
	ModelID             string                `yaml:"model_id"`
	Availability        string                `yaml:"availability"`
	Capability          modelcatalog.Decision `yaml:"capability"`
	PublicFreeEvidence  bool                  `yaml:"public_free_evidence"`
	OperatorAllowlisted bool                  `yaml:"operator_allowlisted"`
	ContextWindow       int64                 `yaml:"context_window,omitempty"`
	PublishedThroughput *PublishedThroughput  `yaml:"published_throughput,omitempty"`
	Lifecycle           string                `yaml:"lifecycle"`
	Reasons             []string              `yaml:"reasons"`
}

type QuotaChange struct {
	ModelID   string `yaml:"model_id"`
	Dimension string `yaml:"dimension"`
	From      *int64 `yaml:"from,omitempty"`
	To        int64  `yaml:"to"`
}

type ChainImpact struct {
	VirtualModelID string   `yaml:"virtual_model_id"`
	Unavailable    []string `yaml:"unavailable,omitempty"`
	Specialist     []string `yaml:"specialist,omitempty"`
	SkippedUnknown []string `yaml:"skipped_unknown,omitempty"`
}

type Summary struct {
	LiveModels             int           `yaml:"live_models"`
	PublicFreeModels       int           `yaml:"public_free_models"`
	GeneralCandidates      int           `yaml:"general_candidates"`
	ModelsAddedToEvidence  []string      `yaml:"models_added_to_public_evidence,omitempty"`
	RetiredOrRemoved       []string      `yaml:"retired_or_removed_from_current_evidence,omitempty"`
	SpecialistsExcluded    []string      `yaml:"specialist_models_excluded,omitempty"`
	UnknownModelsSkipped   []string      `yaml:"unknown_models_skipped,omitempty"`
	ContextChanges         []string      `yaml:"context_changes,omitempty"`
	QuotaChanges           []QuotaChange `yaml:"quota_changes,omitempty"`
	VirtualChainsAffected  []ChainImpact `yaml:"virtual_model_chains_affected,omitempty"`
	RecommendedForApproval []string      `yaml:"recommended_changes_requiring_approval,omitempty"`
}

type Report struct {
	FormatVersion int           `yaml:"format_version"`
	GeneratedAt   string        `yaml:"generated_at"`
	Evidence      Evidence      `yaml:"evidence"`
	Summary       Summary       `yaml:"summary"`
	Models        []ModelReview `yaml:"models"`
	Limitations   []string      `yaml:"limitations"`
}

type Evidence struct {
	CatalogSource       string `yaml:"catalog_source"`
	GroqSource          string `yaml:"groq_source"`
	GeminiSource        string `yaml:"gemini_source"`
	OperatorSQLiteState string `yaml:"operator_sqlite_state"`
}

type OperatorChain struct {
	VirtualModelID string
	Models         []string
}

// BuildReport compares independent availability, capability, free-access, context, and quota evidence.
func BuildReport(now time.Time, catalogSource, groqSource, geminiSource, operatorState string, live []CatalogModel, public []freecatalog.Entry, active *providerfreetier.Spec, contextChanges []string, quotaChanges []QuotaChange, chains []OperatorChain) Report {
	liveByID := make(map[string]CatalogModel, len(live))
	var liveIDs []string
	for _, model := range live {
		liveByID[model.ID] = model
		liveIDs = append(liveIDs, model.ID)
	}
	activeModels := cleanIDs(nil)
	if active != nil {
		activeModels = cleanIDs(active.Models)
	}
	activeSet := stringSet(activeModels)

	livePublic := freecatalog.AlignEntriesToCatalog(freecatalog.FilterEntriesByCatalog(public, liveIDs), liveIDs)
	activePublic := freecatalog.AlignEntriesToCatalog(freecatalog.FilterEntriesByCatalog(public, activeModels), activeModels)
	publicByReviewedID := make(map[string]freecatalog.Entry)
	for _, entry := range public {
		publicByReviewedID[entry.BiFrostID] = entry
	}
	for _, entry := range livePublic {
		publicByReviewedID[entry.BiFrostID] = entry
	}
	for _, entry := range activePublic {
		publicByReviewedID[entry.BiFrostID] = entry
	}

	union := make(map[string]struct{})
	for id := range liveByID {
		union[id] = struct{}{}
	}
	for id := range publicByReviewedID {
		union[id] = struct{}{}
	}
	for id := range activeSet {
		union[id] = struct{}{}
	}
	for _, chain := range chains {
		for _, id := range chain.Models {
			if strings.TrimSpace(id) != "" {
				union[strings.TrimSpace(id)] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(union))
	for id := range union {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	report := Report{
		FormatVersion: 1,
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		Evidence:      Evidence{CatalogSource: catalogSource, GroqSource: groqSource, GeminiSource: geminiSource, OperatorSQLiteState: operatorState},
		Limitations: []string{
			"Public free-plan pages are evidence, not proof of account entitlement; live BiFrost availability is evaluated separately.",
			"Absence from the live account catalog means unavailable for this review, not necessarily globally deprecated.",
			"BiFrost can fall back to static model metadata after provider discovery errors; verify provider health before treating catalog rows as account entitlement.",
			"Actual remaining quota from response headers is not persisted yet; configured ceilings and local usage admission remain separate.",
		},
	}
	report.Summary.LiveModels = len(liveByID)
	report.Summary.PublicFreeModels = len(public)
	report.Summary.ContextChanges = append([]string(nil), contextChanges...)
	report.Summary.QuotaChanges = append([]QuotaChange(nil), quotaChanges...)

	for _, id := range ids {
		model, available := liveByID[id]
		entry, publicFree := publicByReviewedID[id]
		_, exactOperatorEntry := activeSet[id]
		operatorAllowlisted := active != nil && active.Match(id)
		decision := modelcatalog.Classify(id, model.Capabilities)
		review := ModelReview{
			ModelID: id, Capability: decision, PublicFreeEvidence: publicFree,
			OperatorAllowlisted: operatorAllowlisted, ContextWindow: model.ContextLength,
		}
		if available {
			review.Availability = "available_to_account"
		} else {
			review.Availability = "not_in_live_account_catalog"
		}
		if publicFree && entry.Groq != nil {
			review.PublishedThroughput = &PublishedThroughput{RPM: entry.Groq.RPM, RPD: entry.Groq.RPD, TPM: entry.Groq.TPM, TPD: entry.Groq.TPD, ASH: entry.Groq.ASH, ASD: entry.Groq.ASD}
		}
		switch {
		case available && decision.Status == modelcatalog.StatusIncluded:
			review.Lifecycle = "current_candidate"
			review.Reasons = append(review.Reasons, "present in live account catalog", decision.Reason)
			report.Summary.GeneralCandidates++
		case decision.Status == modelcatalog.StatusExcluded:
			review.Lifecycle = "specialist_excluded"
			review.Reasons = append(review.Reasons, decision.Reason)
			report.Summary.SpecialistsExcluded = append(report.Summary.SpecialistsExcluded, id)
		case decision.Status == modelcatalog.StatusSkipped:
			review.Lifecycle = "capability_unknown_skipped"
			review.Reasons = append(review.Reasons, decision.Reason)
			report.Summary.UnknownModelsSkipped = append(report.Summary.UnknownModelsSkipped, id)
		case publicFree && !available:
			review.Lifecycle = "public_evidence_but_unavailable_to_account"
			review.Reasons = append(review.Reasons, "current public free-tier evidence exists", "absent from live account catalog")
		default:
			review.Lifecycle = "removed_from_current_evidence"
			review.Reasons = append(review.Reasons, "operator entry is absent from current public evidence and live account catalog")
		}
		if exactOperatorEntry && !publicFree && !strings.HasPrefix(id, "ollama/") {
			report.Summary.RetiredOrRemoved = append(report.Summary.RetiredOrRemoved, id)
		}
		report.Models = append(report.Models, review)
	}

	publicRawSet := make(map[string]struct{}, len(public))
	for _, entry := range public {
		publicRawSet[entry.BiFrostID] = struct{}{}
	}
	for id := range publicRawSet {
		if _, ok := activeSet[id]; !ok {
			report.Summary.ModelsAddedToEvidence = append(report.Summary.ModelsAddedToEvidence, id)
		}
	}
	sort.Strings(report.Summary.ModelsAddedToEvidence)
	report.Summary.VirtualChainsAffected = ChainImpacts(chains, liveByID)
	if len(report.Summary.RetiredOrRemoved) > 0 {
		report.Summary.RecommendedForApproval = append(report.Summary.RecommendedForApproval, "remove retired/removed exact ids from the active free-tier allowlist after operator review")
	}
	if len(quotaChanges) > 0 {
		report.Summary.RecommendedForApproval = append(report.Summary.RecommendedForApproval, "review public Groq throughput changes in provider-model-limits.generated.yaml")
	}
	if len(contextChanges) > 0 {
		report.Summary.RecommendedForApproval = append(report.Summary.RecommendedForApproval, "review live catalog context additions in provider-model-limits.generated.yaml")
	}
	publicGroq, liveGroq := false, false
	for _, entry := range public {
		publicGroq = publicGroq || entry.Provider == "groq" || strings.HasPrefix(entry.BiFrostID, "groq/")
	}
	for id := range liveByID {
		liveGroq = liveGroq || strings.HasPrefix(id, "groq/")
	}
	if publicGroq && !liveGroq {
		report.Summary.RecommendedForApproval = append(report.Summary.RecommendedForApproval, "verify the persisted Groq credential and provider health before treating absent Groq models as retired")
	}
	return report
}

func cleanIDs(ids []string) []string {
	set := stringSet(ids)
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func stringSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = struct{}{}
		}
	}
	return set
}

// ChainImpacts reports saved-chain entries that current routing would reject or skip.
func ChainImpacts(chains []OperatorChain, live map[string]CatalogModel) []ChainImpact {
	var out []ChainImpact
	for _, chain := range chains {
		impact := ChainImpact{VirtualModelID: chain.VirtualModelID}
		for _, id := range chain.Models {
			model, ok := live[id]
			if !ok {
				impact.Unavailable = append(impact.Unavailable, id)
				continue
			}
			decision := modelcatalog.Classify(id, model.Capabilities)
			switch decision.Status {
			case modelcatalog.StatusExcluded:
				impact.Specialist = append(impact.Specialist, id)
			case modelcatalog.StatusSkipped:
				impact.SkippedUnknown = append(impact.SkippedUnknown, id)
			}
		}
		if len(impact.Unavailable)+len(impact.Specialist)+len(impact.SkippedUnknown) > 0 {
			out = append(out, impact)
		}
	}
	return out
}

// ParsePublishedCount converts provider cells such as 14.4K and 2M to integer ceilings.
func ParsePublishedCount(raw string) (int64, bool) {
	s := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(raw, ",", "")))
	if s == "" || s == "-" || s == "UNLIMITED" {
		return 0, false
	}
	multiplier := float64(1)
	if strings.HasSuffix(s, "K") {
		multiplier, s = 1_000, strings.TrimSuffix(s, "K")
	}
	if strings.HasSuffix(s, "M") {
		multiplier, s = 1_000_000, strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return int64(math.Round(n * multiplier)), true
}

// ApplyPublishedGroqLimits updates only existing operator model layers in a proposal config.
// Public values never establish account availability and active files are not written here.
func ApplyPublishedGroqLimits(cfg *providerlimits.Config, entries []freecatalog.Entry) []QuotaChange {
	if cfg == nil {
		return nil
	}
	provider, ok := cfg.Providers["groq"]
	if !ok || len(provider.Models) == 0 {
		return nil
	}
	activeIDs := make([]string, 0, len(provider.Models))
	for id := range provider.Models {
		activeIDs = append(activeIDs, id)
	}
	aligned := freecatalog.AlignEntriesToCatalog(freecatalog.FilterEntriesByCatalog(entries, activeIDs), activeIDs)
	var changes []QuotaChange
	for _, entry := range aligned {
		if entry.Groq == nil {
			continue
		}
		layer, exists := provider.Models[entry.BiFrostID]
		if !exists {
			continue
		}
		apply := func(dimension, raw string, field **int64) {
			value, parsed := ParsePublishedCount(raw)
			if !parsed {
				return
			}
			if *field != nil && **field == value {
				return
			}
			var from *int64
			if *field != nil {
				copied := **field
				from = &copied
			}
			copied := value
			*field = &copied
			changes = append(changes, QuotaChange{ModelID: entry.BiFrostID, Dimension: dimension, From: from, To: value})
		}
		apply("rpm", entry.Groq.RPM, &layer.RPM)
		apply("rpd", entry.Groq.RPD, &layer.RPD)
		apply("tpm", entry.Groq.TPM, &layer.TPM)
		apply("tpd", entry.Groq.TPD, &layer.TPD)
		provider.Models[entry.BiFrostID] = layer
	}
	cfg.Providers["groq"] = provider
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].ModelID != changes[j].ModelID {
			return changes[i].ModelID < changes[j].ModelID
		}
		return changes[i].Dimension < changes[j].Dimension
	})
	return changes
}

// LoadOperatorChains opens SQLite read-only and tolerates a runtime database that predates the VM schema.
func LoadOperatorChains(path string) ([]OperatorChain, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "not_configured", nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, "database_missing", nil
		}
		return nil, "stat_error", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "path_error", err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro")
	if err != nil {
		return nil, "open_error", err
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('virtual_models','virtual_model_fallback')`).Scan(&count); err != nil {
		return nil, "query_error", err
	}
	if count != 2 {
		return nil, "schema_not_migrated", nil
	}
	rows, err := db.Query(`SELECT v.model_id, f.chain_json FROM virtual_models v JOIN virtual_model_fallback f ON f.virtual_model_id=v.id WHERE v.enabled=1 ORDER BY v.id`)
	if err != nil {
		return nil, "query_error", err
	}
	defer rows.Close()
	var out []OperatorChain
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, "scan_error", err
		}
		var models []string
		if err := json.Unmarshal([]byte(raw), &models); err != nil {
			return nil, "invalid_chain_json", err
		}
		out = append(out, OperatorChain{VirtualModelID: id, Models: models})
	}
	if err := rows.Err(); err != nil {
		return nil, "query_error", err
	}
	return out, "ready", nil
}

type CodingCandidate struct {
	ModelID          string `yaml:"model_id"`
	Availability     string `yaml:"availability"`
	EffectiveContext int64  `yaml:"effective_context_tokens,omitempty"`
	Reason           string `yaml:"reason"`
}

type CodingProposal struct {
	FormatVersion          int               `yaml:"format_version"`
	GeneratedAt            string            `yaml:"generated_at"`
	Status                 string            `yaml:"status"`
	VirtualModelID         string            `yaml:"virtual_model_id"`
	PrivacyPolicy          string            `yaml:"privacy_policy"`
	EffectiveFallbackChain []string          `yaml:"effective_fallback_chain"`
	Candidates             []CodingCandidate `yaml:"candidates"`
	PendingConsultants     []CodingCandidate `yaml:"pending_cloud_consultants,omitempty"`
	SelectionOrder         []string          `yaml:"selection_order"`
	UnknownContextPolicy   string            `yaml:"unknown_context_policy"`
}

var codingModelPreference = []string{
	"ollama/qwen3-vl:8b",
	"groq/openai/gpt-oss-120b",
	"gemini/gemini-2.5-pro",
}

// BuildCodingProposal emits a small chain only from current, capable models with known context.
func BuildCodingProposal(now time.Time, live []CatalogModel, limits *providerlimits.Config, public []freecatalog.Entry) CodingProposal {
	proposal := CodingProposal{
		FormatVersion: 1, GeneratedAt: now.UTC().Format(time.RFC3339), Status: "proposal_only",
		VirtualModelID: "Coding-0.1.0", PrivacyPolicy: "local_first",
		SelectionOrder: []string{
			"general text/code capability",
			"request fits effective context and configured quota",
			"available to this account",
			"local/cloud privacy policy",
			"curated fallback preference: " + strings.Join(codingModelPreference, " -> "),
		},
		UnknownContextPolicy: "reject_before_send_or_route_elsewhere",
	}
	var eligible []string
	byID := make(map[string]CatalogModel, len(live))
	for _, model := range live {
		byID[model.ID] = model
		if modelcatalog.Classify(model.ID, model.Capabilities).Status == modelcatalog.StatusIncluded {
			eligible = append(eligible, model.ID)
		}
	}
	ordered := modelcatalog.OrderByPreference(eligible, codingModelPreference, true)
	curated := make(map[string]struct{}, len(codingModelPreference))
	for _, id := range codingModelPreference {
		curated[id] = struct{}{}
	}
	for _, id := range ordered {
		effective := int64(0)
		if limits != nil {
			effective, _ = limits.Resolve(id).EffectiveContextCap()
		}
		if effective == 0 && byID[id].ContextLength > 0 {
			effective = byID[id].ContextLength
		}
		candidate := CodingCandidate{ModelID: id, Availability: "available_to_account", EffectiveContext: effective, Reason: modelcatalog.Classify(id, byID[id].Capabilities).Reason}
		if effective == 0 {
			candidate.Reason += "; excluded from chain because context is unknown"
			proposal.Candidates = append(proposal.Candidates, candidate)
			continue
		}
		proposal.Candidates = append(proposal.Candidates, candidate)
		if _, selected := curated[id]; selected {
			proposal.EffectiveFallbackChain = append(proposal.EffectiveFallbackChain, id)
		}
	}
	publicSet := make(map[string]struct{}, len(public))
	for _, entry := range public {
		publicSet[entry.BiFrostID] = struct{}{}
	}
	for _, id := range []string{"groq/openai/gpt-oss-120b", "gemini/gemini-2.5-pro"} {
		if _, documented := publicSet[id]; !documented {
			continue
		}
		if _, available := byID[id]; available {
			continue
		}
		proposal.PendingConsultants = append(proposal.PendingConsultants, CodingCandidate{ModelID: id, Availability: "not_in_live_account_catalog", Reason: "current official free-tier evidence; requires provider/account availability and operator approval"})
	}
	return proposal
}

// MarshalYAML writes a generated artifact with a standard review warning.
func MarshalYAML(value any) ([]byte, error) {
	body, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append([]byte("# Generated proposal only. Review the semantic report and diff; do not copy over active operator settings blindly.\n"), body...), nil
}
