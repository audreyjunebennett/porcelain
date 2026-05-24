/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminRouterModels = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var formatInt = ctx.formatInt;
  var fallbackChainToYAML = ctx.fallbackChainToYAML;
  var parseFallbackChainInput = ctx.parseFallbackChainInput;
  var adminModelUsageById = ctx.adminModelUsageById;
  var adminExtractProviderModel = ctx.adminExtractProviderModel;
  var adminProviderTierSpan = ctx.adminProviderTierSpan;
  var adminScopedEvlogPanelFromEvents = ctx.adminScopedEvlogPanelFromEvents;
  var adminScopedEventsForRouting = ctx.adminScopedEventsForRouting;
  var operatorConfigureBtnInline = ctx.operatorConfigureBtnInline;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;

  function buildAdminRouterModelCardHtml() {
    var gw = (ctx.adminStateCache && ctx.adminStateCache.gateway) || {};
    var routerModels = Array.isArray(gw.router_models) ? gw.router_models : [];
    var freeTierOnly = !!gw.filter_free_tier_models;
    var thresholdSaved = String(gw.tool_router_confidence_threshold != null ? gw.tool_router_confidence_threshold : 0.5);
    var threshold = ctx.routerThresholdTouched && ctx.routerThresholdDraft != null ? String(ctx.routerThresholdDraft) : thresholdSaved;
    var routerEnabled = ctx.routerEnabledTouched && ctx.routerEnabledDraft != null ? !!ctx.routerEnabledDraft : !!gw.tool_router_enabled;
    var routerModelsYAML = ctx.routerModelsTouched
      ? String(ctx.routerModelsDraft != null ? ctx.routerModelsDraft : ((document.getElementById("admin-router-models-yaml") && document.getElementById("admin-router-models-yaml").value) || fallbackChainToYAML(routerModels)))
      : fallbackChainToYAML(routerModels);
    var routerChain = routerModels;
    if (ctx.routerModelsTouched) {
      try {
        routerChain = parseFallbackChainInput(routerModelsYAML);
      } catch (_eRouterParse) {
        routerChain = routerModels;
      }
    }
    var usesByModel = adminModelUsageById();
    var routerTableRows = "";
    for (var i = 0; i < routerChain.length; i++) {
      var rid = String(routerChain[i] || "");
      var rpm = adminExtractProviderModel(rid);
      routerTableRows +=
        "<tr>" +
        '<td class="num">' + escapeHtml(String(i + 1)) + "</td>" +
        "<td>" + adminProviderTierSpan(rpm.provider) + "</td>" +
        '<td><code class="sum-mono-id">' + escapeHtml(rid) + "</code></td>" +
        '<td class="num">' + escapeHtml(formatInt(usesByModel[rid] || 0)) + "</td>" +
        "</tr>";
    }
    if (!routerTableRows) routerTableRows = '<tr><td colspan="4" class="muted">No router models configured.</td></tr>';
    return (
      '<article class="sum-card sum-card--collapsible" id="admin-router-model">' +
      '<header class="sum-card__hdr">' +
      '<span class="sum-avatar sum-av-svc-chimera-gateway">Tr</span>' +
      '<span class="sum-main"><span class="sum-title">Router model</span>' +
      '<span class="sum-sub sum-sub--clamp">Tool-router controls and ordered router model list.</span></span>' +
      '<button class="sum-router-toggle" type="button" id="admin-router-enabled" data-admin-action="router-enabled-toggle" aria-label="Toggle tool router" aria-pressed="' + (routerEnabled ? "true" : "false") + '">' +
      '<span class="sum-router-toggle__track"><span class="sum-router-toggle__thumb"></span></span>' +
      "</button>" +
      operatorCardChevronHtml() +
      "</header>" +
      '<div class="sum-body">' +
      '<div class="sg-op-head-row">' +
      '<div class="sg-op-card-note sg-op-card-note--tight">Manage tool-router model order, enabled state, and confidence threshold from one panel.</div>' +
      "</div>" +
      '<div class="sg-op-head-row">' +
      '<div class="sum-section-label">Router Models</div>' +
      '<div class="sg-op-head-actions">' +
      (ctx.adminRouterEditing
        ? ('<button class="sg-op-btn sg-op-btn--ghost sg-op-btn--toggle' + (freeTierOnly ? " is-active" : "") + '" type="button" data-admin-action="routing-free-tier-toggle" aria-pressed="' + (freeTierOnly ? "true" : "false") + '">Free Tier Only</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="routing-generate">Generate from live catalog</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="router-cancel">Cancel</button>')
        : operatorConfigureBtnInline("router-configure", "Configure router models", "Configure")) +
      "</div></div>" +
      '<div id="admin-router-table-view"' + (ctx.adminRouterEditing ? " hidden" : "") + ">" +
      '<div class="sum-metrics-table-wrap sg-op-router-table-scroll"><table class="sum-metrics-table sg-op-router-table"><thead><tr><th class="num">Order</th><th>Provider</th><th>Model</th><th class="num">Uses (24h)</th></tr></thead><tbody>' + routerTableRows + "</tbody></table></div>" +
      "</div>" +
      '<div id="admin-router-yaml-view"' + (ctx.adminRouterEditing ? "" : " hidden") + ">" +
      '<div id="admin-router-models-wrap" class="sg-op-yaml-wrap sg-op-yaml-wrap--full' + (ctx.routerModelsTouched ? " sg-op-yaml-wrap--dirty" : "") + '">' +
      '<textarea id="admin-router-models-yaml" class="sg-op-yaml-textarea" rows="8" spellcheck="false">' + escapeHtml(routerModelsYAML) + "</textarea>" +
      '<div class="sg-op-yaml-ov"><button type="button" class="sg-op-yaml-ov-btn" data-admin-action="router-models-refresh" title="Revert router models" aria-label="Revert router models"><span class="material-symbols-outlined" aria-hidden="true">refresh</span></button>' +
      '<button type="button" class="sg-op-yaml-ov-btn sg-op-yaml-ov-btn--save sg-op-yaml-ov-save" data-admin-action="router-save">Save</button></div></div>' +
      "</div>" +
      '<div class="sg-op-head-row">' +
      '<label class="sg-op-label sg-op-label--inline" for="admin-router-threshold">Confidence threshold</label>' +
      '<div class="sg-op-head-actions">' +
      '<input id="admin-router-threshold" class="sg-op-input" type="number" min="0" max="1" step="0.05" value="' + escapeHtml(threshold) + '" style="max-width:9rem"/>' +
      '<button type="button" class="sg-op-yaml-ov-btn sg-op-yaml-ov-btn--save" data-admin-action="router-save">Save</button>' +
      "</div></div>" +
      adminScopedEvlogPanelFromEvents("Scoped log — tool-router", "routing-router", adminScopedEventsForRouting("router")) +
      "</div></article>"
    );
  }

  ctx.buildAdminRouterModelCardHtml = buildAdminRouterModelCardHtml;
};
