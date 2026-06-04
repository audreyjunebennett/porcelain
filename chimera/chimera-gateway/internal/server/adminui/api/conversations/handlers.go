package conversations

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/conversationtitle"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func operatorStore(h *handler.Handler) *operatorstore.Store {
	if h == nil || h.RT == nil {
		return nil
	}
	return h.RT.OperatorStore()
}

func requirePrincipal(h *handler.Handler, w http.ResponseWriter, r *http.Request) (string, bool) {
	pid := strings.TrimSpace(h.SessionPrincipal(r))
	if pid == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "session principal unavailable"})
		return "", false
	}
	return pid, true
}

func requireStore(h *handler.Handler, w http.ResponseWriter) (*operatorstore.Store, bool) {
	store := operatorStore(h)
	if store == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "operator store unavailable"})
		return nil, false
	}
	return store, true
}

func displayTitle(sum operatorstore.ConversationSummary) string {
	if sum.Title.Valid && strings.TrimSpace(sum.Title.String) != "" {
		return strings.TrimSpace(sum.Title.String)
	}
	return sum.PreviewText
}

func summaryWire(sum operatorstore.ConversationSummary) operatorapi.ConversationSummary {
	out := operatorapi.ConversationSummary{
		ConversationID:     sum.ConversationID,
		Title:              displayTitle(sum),
		PreviewText:        sum.PreviewText,
		Flagged:            sum.Flagged,
		WorkspaceProjectID: sum.WorkspaceProjectID,
		WorkspaceFlavorID:  sum.WorkspaceFlavorID,
		CreatedAt:          sum.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          sum.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if sum.WorkspaceRowID.Valid {
		v := sum.WorkspaceRowID.Int64
		out.WorkspaceRowID = &v
	}
	return out
}

func handleListGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	principalID, ok := requirePrincipal(h, w, r)
	if !ok {
		return
	}
	store, ok := requireStore(h, w)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	flaggedOnly := strings.TrimSpace(r.URL.Query().Get("flagged")) == "1"
	list, err := store.ListConversations(r.Context(), principalID, operatorstore.ListConversationsFilter{
		Limit: limit, Offset: offset, FlaggedOnly: flaggedOnly,
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	out := make([]operatorapi.ConversationSummary, 0, len(list))
	for _, row := range list {
		out = append(out, summaryWire(row))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.ConversationListResponse{Conversations: out})
}

func handleDetailGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	principalID, ok := requirePrincipal(h, w, r)
	if !ok {
		return
	}
	store, ok := requireStore(h, w)
	if !ok {
		return
	}
	cid := strings.TrimSpace(r.PathValue("conversation_id"))
	tr, err := store.GetConversationTranscript(r.Context(), principalID, cid)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if tr == nil {
		writeNotFound(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(transcriptWire(*tr))
}

func handlePatchTitle(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	principalID, ok := requirePrincipal(h, w, r)
	if !ok {
		return
	}
	store, ok := requireStore(h, w)
	if !ok {
		return
	}
	var body operatorapi.ConversationTitlePatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeBadRequest(w, "invalid JSON body")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeBadRequest(w, "title required")
		return
	}
	if runeLen(title) > conversationtitle.EditMaxRunes {
		writeBadRequest(w, "title too long")
		return
	}
	cid := strings.TrimSpace(r.PathValue("conversation_id"))
	if err := store.UpdateConversationTitle(r.Context(), principalID, cid, title); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeNotFound(w)
			return
		}
		writeStoreErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.OKResponse{OK: true})
}

func handleFlagPOST(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	principalID, ok := requirePrincipal(h, w, r)
	if !ok {
		return
	}
	store, ok := requireStore(h, w)
	if !ok {
		return
	}
	var body operatorapi.ConversationFlagPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeBadRequest(w, "invalid JSON body")
		return
	}
	cid := strings.TrimSpace(r.PathValue("conversation_id"))
	if err := store.SetConversationFlagged(r.Context(), principalID, cid, body.Flagged); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeNotFound(w)
			return
		}
		writeStoreErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(operatorapi.OKResponse{OK: true})
}

func handleDeleteDELETE(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
	principalID, ok := requirePrincipal(h, w, r)
	if !ok {
		return
	}
	store, ok := requireStore(h, w)
	if !ok {
		return
	}
	cid := strings.TrimSpace(r.PathValue("conversation_id"))
	if err := store.DeleteConversation(r.Context(), principalID, cid); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeNotFound(w)
			return
		}
		writeStoreErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func transcriptWire(tr operatorstore.ConversationTranscript) operatorapi.ConversationDetailResponse {
	sum := tr.Summary
	out := operatorapi.ConversationDetailResponse{
		ConversationID:     sum.ConversationID,
		Title:              displayTitle(sum),
		PreviewText:        sum.PreviewText,
		Flagged:            sum.Flagged,
		WorkspaceProjectID: sum.WorkspaceProjectID,
		WorkspaceFlavorID:  sum.WorkspaceFlavorID,
		CreatedAt:          sum.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          sum.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Turns:              make([]operatorapi.ConversationTurn, 0, len(tr.Turns)),
	}
	if sum.WorkspaceRowID.Valid {
		v := sum.WorkspaceRowID.Int64
		out.WorkspaceRowID = &v
	}
	for _, t := range tr.Turns {
		out.Turns = append(out.Turns, turnWire(t))
	}
	return out
}

func turnWire(t operatorstore.ConversationTurn) operatorapi.ConversationTurn {
	out := operatorapi.ConversationTurn{
		TurnID:        t.TurnID,
		TurnIndex:     t.TurnIndex,
		Role:          t.Role,
		Content:       t.Content,
		SelectedModel: t.SelectedModel,
		ResolvedModel: t.ResolvedModel,
		ErrorDetail:   t.ErrorDetail,
		RetryUserText: t.RetryUserText,
		CreatedAt:     t.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if t.PromptTokens.Valid {
		v := int(t.PromptTokens.Int64)
		out.PromptTokens = &v
	}
	if t.CompletionTokens.Valid {
		v := int(t.CompletionTokens.Int64)
		out.CompletionTokens = &v
	}
	if t.TotalTokens.Valid {
		v := int(t.TotalTokens.Int64)
		out.TotalTokens = &v
	}
	if len(t.Retrievals) > 0 {
		out.RagHits = make([]operatorapi.ConversationRAGHit, 0, len(t.Retrievals))
		for _, r := range t.Retrievals {
			out.RagHits = append(out.RagHits, operatorapi.ConversationRAGHit{
				Source:        r.FilePath,
				Text:          r.SnippetText,
				Score:         r.Score,
				Language:      r.Language,
				VectorPointID: r.VectorPointID,
				ContentSHA256: r.ContentSHA256,
				StartLine:     r.StartLine,
				EndLine:       r.EndLine,
				StartsMidLine: r.StartsMidLine,
			})
		}
	}
	return out
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func writeBadRequest(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: msg})
}

func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "not found"})
}

func writeStoreErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(operatorapi.ErrorBody{Error: "store error", Detail: err.Error()})
}
