/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminProvider = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var formatInt = ctx.formatInt;
  var adminProviderModelCount = ctx.adminProviderModelCount;
  var adminProviderAvailabilityHtml = ctx.adminProviderAvailabilityHtml;
  var adminProviderIntro = ctx.adminProviderIntro;
  var adminProviderUsageRows = ctx.adminProviderUsageRows;
  var providerRowsHtml = ctx.providerRowsHtml;
  var adminProviderAvatarClass = ctx.adminProviderAvatarClass;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;
  var adminScopedEventsForPrincipal = ctx.adminScopedEventsForPrincipal;
  var adminScopedEvlogPanelFromEvents = ctx.adminScopedEvlogPanelFromEvents;

  function providerIsOllama(providerId) {
    if (providerId === "ollama") return true;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog &&
      typeof ChimeraSettings.Providers.Catalog.lookupProviderSpec === "function"
    ) {
      var spec = ChimeraSettings.Providers.Catalog.lookupProviderSpec(providerId);
      return !!(spec && spec.kind === "ollama");
    }
    return false;
  }

  /** True when the provider has saved or in-flight credentials (keys or Ollama base URL). */
  function providerHasCredentials(providerId, row) {
    row = row || {};
    if (providerIsOllama(providerId)) {
      var url =
        ctx.adminOllamaUrlDraft != null
          ? String(ctx.adminOllamaUrlDraft).trim()
          : String(row.ollama_base_url || "").trim();
      return url !== "";
    }
    if (row.key_configured === true) return true;
    var keys = Array.isArray(row.keys) ? row.keys : [];
    if (keys.length > 0) return true;
    for (var ki = 0; ki < keys.length; ki++) {
      var kr = keys[ki] || {};
      if (kr.key_configured === true) return true;
    }
    return false;
  }

  function buildAdminProviderCardHtml(providerId, title, avatar, subtitle) {
    var st = ctx.adminStateCache || {};
    var p = st.providers || {};
    var row = p[providerId] || {};
    var keys = row && Array.isArray(row.keys) ? row.keys : [];
    var keyCount = keys.length;
    var modelCount = adminProviderModelCount(providerId);
    var isOllama = providerIsOllama(providerId);
    var hasCredentials = providerHasCredentials(providerId, row);
    var metrics = "";
    if (isOllama) {
      metrics =
        '<span class="sum-metrics">' +
        sgOpHealthPillHtml("models " + formatInt(modelCount), "metric") +
        adminProviderAvailabilityHtml(providerId) +
        "</span>";
    } else {
      metrics =
        '<span class="sum-metrics">' +
        sgOpHealthPillHtml("keys " + formatInt(keyCount), "metric") +
        sgOpHealthPillHtml("models " + formatInt(modelCount), "metric") +
        adminProviderAvailabilityHtml(providerId) +
        "</span>";
    }
    var providerIntro = adminProviderIntro(providerId, subtitle);
    var usageBlock = "";
    if (hasCredentials) {
      var usageRows = adminProviderUsageRows(providerId);
      var usageHtml = "";
      if (!usageRows.length) {
        usageHtml = '<p class="muted">No usage yet in loaded metrics window.</p>';
      } else {
        usageHtml =
          '<div class="sum-metrics-table-wrap"><table class="sum-metrics-table"><thead><tr><th>Model</th><th class="num">Requests</th><th class="num">Errors</th></tr></thead><tbody>';
        for (var ui = 0; ui < usageRows.length; ui++) {
          var ur = usageRows[ui];
          usageHtml +=
            '<tr><td><code class="sum-mono-id">' +
            escapeHtml(ur.model_id) +
            '</code></td><td class="num">' +
            escapeHtml(formatInt(ur.calls)) +
            '</td><td class="num">' +
            escapeHtml(formatInt(ur.errors)) +
            "</td></tr>";
        }
        usageHtml += "</tbody></table></div>";
      }
      usageBlock = '<div class="sum-section-label">Model usage (24h)</div>' + usageHtml;
    }
    var keyDrafts = ctx.adminProviderKeyDraft || {};
    var keyDraftVal = keyDrafts[providerId] != null ? String(keyDrafts[providerId]) : "";
    var keyPlaceholder = "API key";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog &&
      typeof ChimeraSettings.Providers.Catalog.lookupProviderSpec === "function"
    ) {
      var catSpec = ChimeraSettings.Providers.Catalog.lookupProviderSpec(providerId);
      if (catSpec && catSpec.key_placeholder) keyPlaceholder = String(catSpec.key_placeholder);
    }
    var ollamaUrlVal =
      ctx.adminOllamaUrlDraft != null ? String(ctx.adminOllamaUrlDraft) : String(row.ollama_base_url || "");

    var body = "";
    if (isOllama) {
      body =
        providerIntro +
        usageBlock +
        '<div class="sg-op-provider-edit-row"><div class="sg-op-provider-edit-main"><label class="sg-op-label">Server base URL</label>' +
        '<input id="admin-ollama-url" class="sg-op-input" type="url" placeholder="http://127.0.0.1:11434" value="' + escapeHtml(ollamaUrlVal) + '"/></div>' +
        '<button class="sum-workspaces-create-btn sg-op-save-btn" type="button" data-admin-action="ollama-save">Save</button></div>';
    } else {
      body =
        providerIntro +
        usageBlock +
        '<div class="sum-section-label">API KEYS</div>' +
        '<ul class="sg-op-key-list">' + providerRowsHtml(providerId, row) + "</ul>" +
        '<div class="sg-op-provider-edit-row"><div class="sg-op-provider-edit-main">' +
        '<input id="admin-' + escapeHtml(providerId) + '-key" class="sg-op-input" type="password" placeholder="' + escapeHtml(keyPlaceholder) + '" value="' + escapeHtml(keyDraftVal) + '"/></div>' +
        '<button class="sum-workspaces-create-btn sg-op-save-btn" type="button" data-admin-action="provider-key-add" data-provider="' + escapeHtml(providerId) + '">Save</button></div>';
    }
    var scopedPanel = "";
    if (hasCredentials) {
      var scoped = [];
      for (var ei = entryCache.length - 1; ei >= 0 && scoped.length < 18; ei--) {
        var ev = entryCache[ei];
        var fEv = getFlat(ev.parsed);
        var msgEv = String(fEv.msg || fEv.message || "").toLowerCase();
        var providerHit =
          String(fEv.provider_id || fEv.provider || fEv.upstream_provider || "").toLowerCase() ===
            String(providerId).toLowerCase() ||
          String(fEv.upstreamModel || fEv.model || "")
            .toLowerCase()
            .indexOf(String(providerId).toLowerCase() + "/") === 0 ||
          msgEv.indexOf(String(providerId).toLowerCase()) >= 0;
        if (providerHit) scoped.push(ev);
      }
      scopedPanel = adminScopedEvlogPanelFromEvents("Scoped log — " + title, "provider-" + providerId, scoped);
    }
    var avatarClass = adminProviderAvatarClass(providerId);
    return (
      '<details class="sum-card" id="admin-provider-' + escapeHtml(providerId) + '">' +
      '<summary><span class="sum-avatar ' + escapeHtml(avatarClass) + '">' + escapeHtml(avatar) + '</span><span class="sum-main"><span class="sum-title">' + escapeHtml(title) + "</span>" +
      '<span class="sum-sub sum-sub--clamp">' + escapeHtml(subtitle) + "</span></span>" +
      metrics +
      operatorCardChevronHtml() +
      "</summary><div class=\"sum-body\">" + body + scopedPanel + "</div></details>"
    );
  }

  ctx.buildAdminProviderCardHtml = buildAdminProviderCardHtml;
  ctx.providerHasCredentials = providerHasCredentials;
};
