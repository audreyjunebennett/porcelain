/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminFallback = function (ctx) {
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
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;

  function buildAdminFallbackCardHtml() {
    var gw = (ctx.adminStateCache && ctx.adminStateCache.gateway) || {};
    var fallback = Array.isArray(gw.fallback_chain) ? gw.fallback_chain : [];
    var freeTierOnly = !!gw.filter_free_tier_models;
    var fallbackYAML = ctx.fallbackTouched ? ((document.getElementById("admin-fallback-yaml") && document.getElementById("admin-fallback-yaml").value) || fallbackChainToYAML(fallback)) : fallbackChainToYAML(fallback);
    var chain = fallback;
    if (ctx.fallbackTouched) {
      try {
        chain = parseFallbackChainInput(fallbackYAML);
      } catch (_eFbParse) {
        chain = fallback;
      }
    }
    var usesByModel = adminModelUsageById();
    var tableRows = "";
    for (var i = 0; i < chain.length; i++) {
      var mid = String(chain[i] || "");
      var pm = adminExtractProviderModel(mid);
      tableRows +=
        "<tr>" +
        '<td class="num">' + escapeHtml(String(i + 1)) + "</td>" +
        "<td>" + adminProviderTierSpan(pm.provider) + "</td>" +
        '<td><code class="sum-mono-id">' + escapeHtml(mid) + "</code></td>" +
        '<td class="num">' + escapeHtml(formatInt(usesByModel[mid] || 0)) + "</td>" +
        "</tr>";
    }
    if (!tableRows) tableRows = '<tr><td colspan="4" class="muted">No fallback routes configured.</td></tr>';
    return (
      '<details class="sum-card" id="admin-fallback-chain">' +
      '<summary><span class="sum-avatar sum-av-svc-chimera-gateway">Fb</span><span class="sum-main"><span class="sum-title">Fallback chain</span>' +
      '<span class="sum-sub sum-sub--clamp">Ordered failover list used when the first route cannot serve.</span></span>' +
      '<span class="sum-metrics">' + sgOpHealthPillHtml(formatInt(fallback.length) + " tiers", "metric") + "</span>" +
      operatorCardChevronHtml() +
      "</summary>" +
      '<div class="sum-body">' +
      '<div class="sg-op-card-note">Define the fallback sequence used when the selected route cannot serve a request.</div>' +
      '<div class="sg-op-head-row">' +
      '<div class="sum-section-label">Fallback Order</div>' +
      '<div class="sg-op-head-actions">' +
      (ctx.adminFallbackEditing
        ? ('<button class="sg-op-btn sg-op-btn--ghost sg-op-btn--toggle' + (freeTierOnly ? " is-active" : "") + '" type="button" data-admin-action="routing-free-tier-toggle" aria-pressed="' + (freeTierOnly ? "true" : "false") + '">Free Tier Only</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="routing-generate">Generate from live catalog</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="fallback-cancel">Cancel</button>')
        : operatorConfigureBtnInline("fallback-configure", "Configure fallback chain", "Configure")) +
      "</div></div>" +
      '<div id="admin-fallback-table-view"' + (ctx.adminFallbackEditing ? " hidden" : "") + ">" +
      '<div class="sum-metrics-table-wrap sg-op-fallback-table-scroll"><table class="sum-metrics-table sg-op-fallback-table"><thead><tr><th class="num">Order</th><th>Provider</th><th>Model</th><th class="num">Uses (24h)</th></tr></thead><tbody>' + tableRows + "</tbody></table></div>" +
      "</div>" +
      '<div id="admin-fallback-yaml-view"' + (ctx.adminFallbackEditing ? "" : " hidden") + ">" +
      '<div id="admin-fallback-yaml-wrap" class="sg-op-yaml-wrap sg-op-yaml-wrap--full' + (ctx.fallbackTouched ? " sg-op-yaml-wrap--dirty" : "") + '">' +
      '<textarea id="admin-fallback-yaml" class="sg-op-yaml-textarea" rows="8" spellcheck="false">' + escapeHtml(fallbackYAML) + "</textarea>" +
      '<div class="sg-op-yaml-ov"><button type="button" class="sg-op-yaml-ov-btn" data-admin-action="fallback-refresh" title="Revert fallback chain" aria-label="Revert fallback chain"><span class="material-symbols-outlined" aria-hidden="true">refresh</span></button>' +
      '<button type="button" class="sg-op-yaml-ov-btn sg-op-yaml-ov-btn--save sg-op-yaml-ov-save" data-admin-action="fallback-save">Save</button></div></div>' +
      "</div>" +
      adminScopedEvlogPanelFromEvents("Scoped log — fallback / failover", "routing-fallback", adminScopedEventsForRouting("fallback")) +
      "</div></details>"
    );
  }

  ctx.buildAdminFallbackCardHtml = buildAdminFallbackCardHtml;
};
