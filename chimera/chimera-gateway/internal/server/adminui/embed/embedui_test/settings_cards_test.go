package embedui_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestLogsCards_gatewayOverview_rendersIdAndVersion(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`ctx.buildGatewayOverviewCardHtml()`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	for _, want := range []string{`id="gw-overview"`, "9.9.9-test", "virtual/test", "Overview"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestLogsCards_adminUsers_section(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`ctx.buildAdminUsersCardHtml()`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	for _, want := range []string{`id="admin-users"`, "tenant-a", "Alice", "Add user"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestLogsCards_serviceAvatarInitials(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`ctx.serviceAvatarInitials("chimera-gateway")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "CW" {
		t.Fatalf("got %q", v.String())
	}
}

func TestLogsCards_adminProvider_keyDraftInHtml(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	_, err := vm.RunString(`
		ctx.adminProviderKeyDraft.groq = "gsk-draft-secret";
		ctx.adminOllamaUrlDraft = "http://draft.local:11434";
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ctx.buildAdminProviderCardHtml("groq", "Groq", "G", "Fast inference")`)
	if err != nil {
		t.Fatal(err)
	}
	groqHTML := v.String()
	if !strings.Contains(groqHTML, `id="admin-groq-key"`) || !strings.Contains(groqHTML, `value="gsk-draft-secret"`) {
		t.Fatalf("groq draft not in html: %q", groqHTML)
	}

	v2, err := vm.RunString(`ctx.buildAdminProviderCardHtml("ollama", "Ollama", "O", "Local")`)
	if err != nil {
		t.Fatal(err)
	}
	ollamaHTML := v2.String()
	if !strings.Contains(ollamaHTML, `id="admin-ollama-url"`) || !strings.Contains(ollamaHTML, `value="http://draft.local:11434"`) {
		t.Fatalf("ollama draft not in html: %q", ollamaHTML)
	}
}

func TestLogsCards_adminProvider_emptyCredentialsOmitsUsageAndScopedLog(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	_, err := vm.RunString(`
		ctx.adminStateCache = {
			providers: {
				groq: { keys: [], ok: true, key_configured: false },
				gemini: { keys: [{ name: "k1", key_configured: true }], ok: true, key_configured: true }
			}
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	emptyGroq, err := vm.RunString(`ctx.buildAdminProviderCardHtml("groq", "Groq", "Gq", "subtitle")`)
	if err != nil {
		t.Fatal(err)
	}
	emptyHTML := emptyGroq.String()
	for _, mustNot := range []string{"Model usage (24h)", "Scoped log —"} {
		if strings.Contains(emptyHTML, mustNot) {
			t.Fatalf("empty groq card must not contain %q: %q", mustNot, emptyHTML)
		}
	}
	if !strings.Contains(emptyHTML, "API KEYS") || !strings.Contains(emptyHTML, `id="admin-groq-key"`) {
		t.Fatalf("empty groq card should still show key editor: %q", emptyHTML)
	}

	configured, err := vm.RunString(`ctx.buildAdminProviderCardHtml("gemini", "Gemini", "Gm", "subtitle")`)
	if err != nil {
		t.Fatal(err)
	}
	cfgHTML := configured.String()
	for _, want := range []string{"Model usage (24h)", "Scoped log —"} {
		if !strings.Contains(cfgHTML, want) {
			t.Fatalf("configured gemini card missing %q: %q", want, cfgHTML)
		}
	}
}

func TestLogsCards_adminProvider_ollamaDraftShowsUsageAndScopedLog(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	_, err := vm.RunString(`
		ctx.adminStateCache = { providers: { ollama: { keys: [], ok: true, ollama_base_url: "" } } };
		ctx.adminOllamaUrlDraft = "http://draft.local:11434";
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ctx.buildAdminProviderCardHtml("ollama", "Ollama", "Ol", "local")`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	for _, want := range []string{"Model usage (24h)", "Scoped log —", `id="admin-ollama-url"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestLogsCards_providerHasCredentials_afterKeyPresent(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`
		(function () {
			var empty = ctx.providerHasCredentials("groq", { keys: [], key_configured: false });
			var withKey = ctx.providerHasCredentials("groq", { keys: [{ name: "k1", key_configured: true }] });
			return { empty: empty, withKey: withKey };
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("empty").ToBoolean() {
		t.Fatal("expected no credentials when keys empty")
	}
	if !obj.Get("withKey").ToBoolean() {
		t.Fatal("expected credentials when key row present")
	}
}

func TestLogsCards_adminProvider_keyChipReflectsState(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	_, err := vm.RunString(`
		ctx.adminStateCache = {
			providers: {
				groq: { keys: [{ name: "k1" }, { name: "k2" }], ok: true },
				gemini: { keys: [], ok: false },
				ollama: { keys: [], ok: true, ollama_base_url: "http://127.0.0.1:11434" }
			}
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ctx.buildAdminProviderCardHtml("groq", "Groq", "Gq", "subtitle")`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	for _, want := range []string{`id="admin-provider-groq"`, "keys 2", `id="admin-groq-key"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestLogsCards_adminRoutingCards_stableIds(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	_, err := vm.RunString(`
		ctx.adminStateCache = {
			gateway: {
				routing_policy_yaml: "rules: []\n",
				fallback_chain: ["groq/llama"],
				router_models: ["groq/llama"],
				tool_router_enabled: true,
				tool_router_confidence_threshold: 0.5
			}
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	for _, spec := range []struct {
		fn   string
		want string
	}{
		{`ctx.buildAdminRoutingRulesCardHtml()`, `id="admin-routing-rules"`},
		{`ctx.buildAdminFallbackCardHtml()`, `id="admin-fallback-chain"`},
		{`ctx.buildAdminRouterModelCardHtml()`, `id="admin-router-model"`},
	} {
		v, err := vm.RunString(spec.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(v.String(), spec.want) {
			t.Fatalf("%s: missing %q", spec.fn, spec.want)
		}
	}
}

func TestLogsCards_virtualModelCard_detailsLayout(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`
		ctx.buildVirtualModelCardHtml({
			id: 42,
			model_id: "Chimera-0.2.0",
			name: "Chimera",
			version: "0.2.0",
			description: "Bootstrap",
			enabled: true,
			visibility: "public",
			fallback_depth: 18,
			routing_policy_enabled: true,
			tool_router_enabled: false,
			router_models: []
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	for _, want := range []string{
		`<details class="sum-card sum-card--virtual-model"`,
		`id="virtual-model-42"`,
		`data-virtual-model-id="42"`,
		`<summary>`,
		`sum-avatar sum-av-svc-chimera-gateway">Vm</span>`,
		`Chimera · 0.2.0`,
		`Bootstrap`,
		`>public</span>`,
		`Chimera-0.2.0`,
		`sg-op-health-pill`,
		`sum-body--virtual-model`,
		`sum-vm-client-usage`,
		`sum-vm-client-usage-hdr`,
		`sum-vm-card-toggles`,
		`vm-identity-configure`,
		`vm-identity-visibility-toggle`,
		`vm-identity-enabled-toggle`,
		`vm-routing-enabled-toggle`,
		`vm-router-enabled-toggle`,
		`sum-vm-section__hdr-toggles`,
		`sum-vm-section__hdr-desc`,
		`Required. Ordered upstream model ids`,
		`vm-42-identity-view`,
		`vm-42-visibility-toggle`,
		`sg-op-kv--vm-identity`,
		`sum-vm-section`,
		`Identity`,
		`Fallback chain`,
		`Routing policy`,
		`sum-vm-routing-panel`,
		`vm-42-routing-table`,
		`vm-42-routing-yaml`,
		`Tool router`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
	for _, absent := range []string{
		`18 fallback`,
		`sg-op-health-pill--routing`,
		`Dry-run router`,
		`vm-42-eval-msg`,
		`>required</span>`,
		`>optional</span>`,
		` tiers</span>`,
		` rules</span>`,
		`>Chat completions URL</`,
		`Client usage`,
		`chat completion url with your API key`,
		`vm-chat-url-copy`,
		`vm-chat-body-copy`,
		`sum-vm-client-usage-panel`,
		`/v1/chat/completions`,
		`vm-identity-delete`,
		`vm-identity-btn-delete`,
	} {
		if strings.Contains(html, absent) {
			t.Fatalf("unexpected %q in card header html", absent)
		}
	}
}

func TestLogsCards_formatMergedConversationSubtitle(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	v, err := vm.RunString(`ctx.formatMergedConversationSubtitle(3)`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "3 ids") {
		t.Fatalf("got %q", html)
	}
	v2, err := vm.RunString(`ctx.formatMergedConversationSubtitle(1)`)
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "" {
		t.Fatalf("expected empty for single id, got %q", v2.String())
	}
}
