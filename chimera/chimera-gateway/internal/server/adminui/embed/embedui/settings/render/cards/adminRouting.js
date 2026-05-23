/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminRouting = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var formatInt = ctx.formatInt;
  var countRoutingRulesFromYAML = ctx.countRoutingRulesFromYAML;
  var parseRoutingRulesFromYAML = ctx.parseRoutingRulesFromYAML;
  var adminModelUsageById = ctx.adminModelUsageById;
  var adminScopedEvlogPanelFromEvents = ctx.adminScopedEvlogPanelFromEvents;
  var adminScopedEventsForRouting = ctx.adminScopedEventsForRouting;
  var operatorConfigureBtnInline = ctx.operatorConfigureBtnInline;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;

  function buildAdminRoutingRulesCardHtml() {
    var gw = (ctx.adminStateCache && ctx.adminStateCache.gateway) || {};
    var policy = gw.routing_policy_yaml || "";
    var policyLive = ctx.routingPolicyDraft != null ? String(ctx.routingPolicyDraft) : String(policy);
    var policyDirty = String(policyLive) !== String(policy);
    var rulesCount = countRoutingRulesFromYAML(policyLive);
    var freeTierOnly = !!gw.filter_free_tier_models;
    var usesByModel = adminModelUsageById();
    var routingRulesRows = parseRoutingRulesFromYAML(policy);
    var tableRows = "";
    for (var ri = 0; ri < routingRulesRows.length; ri++) {
      var rr = routingRulesRows[ri] || {};
      var matchVal = "";
      if (rr.whenInline) {
        matchVal = rr.whenInline === "{}" ? "(catch-all)" : rr.whenInline;
      } else if (rr.whenParts && rr.whenParts.length) {
        matchVal = rr.whenParts.join("; ");
      } else {
        matchVal = "(catch-all)";
      }
      var modelCell = "—";
      if (rr.models && rr.models.length) {
        var parts = [];
        for (var mi = 0; mi < rr.models.length; mi++) {
          parts.push('<code class="sum-mono-id">' + escapeHtml(rr.models[mi]) + "</code>");
        }
        modelCell = parts.join(", ");
      }
      var hits = 0;
      for (var hm = 0; hm < (rr.models || []).length; hm++) {
        hits += Number(usesByModel[rr.models[hm]] || 0);
      }
      tableRows +=
        "<tr>" +
        '<td><code class="sum-mono-id">' + escapeHtml(rr.name || "unnamed") + "</code></td>" +
        '<td><code class="sum-mono-id">' + escapeHtml(matchVal) + "</code></td>" +
        "<td>" + modelCell + "</td>" +
        '<td class="num">' + escapeHtml(formatInt(hits)) + "</td>" +
        "</tr>";
    }
    if (!tableRows) tableRows = '<tr><td colspan="4" class="muted">No routing rules configured.</td></tr>';
    return (
      '<details class="sum-card" id="admin-routing-rules">' +
      '<summary><span class="sum-avatar sum-av-svc-chimera-gateway">Rt</span><span class="sum-main"><span class="sum-title">Routing rules</span>' +
      '<span class="sum-sub sum-sub--clamp">Virtual model policy with editable YAML and live catalog generation.</span></span>' +
      '<span class="sum-metrics">' + sgOpHealthPillHtml(formatInt(rulesCount) + " active rules", "metric") + "</span>" +
      operatorCardChevronHtml() +
      "</summary>" +
      '<div class="sum-body">' +
      '<div class="sg-op-card-note">Review active routing rules and 24h hits; use Configure to edit policy YAML.</div>' +
      '<div class="sg-op-head-row">' +
      '<div class="sum-section-label">Routing Policy</div>' +
      '<div class="sg-op-head-actions">' +
      (ctx.adminRoutingEditing
        ? ('<button class="sg-op-btn sg-op-btn--ghost sg-op-btn--toggle' + (freeTierOnly ? " is-active" : "") + '" type="button" data-admin-action="routing-free-tier-toggle" aria-pressed="' + (freeTierOnly ? "true" : "false") + '">Free Tier Only</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="routing-generate">Generate from live catalog</button>' +
          '<button class="sg-op-btn sg-op-btn--ghost" type="button" data-admin-action="routing-cancel">Cancel</button>')
        : operatorConfigureBtnInline("routing-configure", "Configure routing policy", "Configure")) +
      "</div>" +
      "</div>" +
      '<div id="admin-routing-table-view"' + (ctx.adminRoutingEditing ? " hidden" : "") + ">" +
      '<div class="sum-metrics-table-wrap sg-op-routing-table-scroll"><table class="sum-metrics-table"><thead><tr><th>Name</th><th>Match</th><th>Models</th><th class="num">Hits (24h)</th></tr></thead><tbody>' + tableRows + "</tbody></table></div>" +
      "</div>" +
      '<div id="admin-routing-yaml-view"' + (ctx.adminRoutingEditing ? "" : " hidden") + ">" +
      '<div id="admin-routing-policy-wrap" class="sg-op-yaml-wrap sg-op-yaml-wrap--full' + (policyDirty ? " sg-op-yaml-wrap--dirty" : "") + '">' +
      '<textarea id="admin-routing-yaml" class="sg-op-yaml-textarea" rows="10" spellcheck="false">' + escapeHtml(policyLive) + "</textarea>" +
      '<div class="sg-op-yaml-ov">' +
      '<button type="button" class="sg-op-yaml-ov-btn" data-admin-action="routing-policy-refresh" title="Revert to last saved routing YAML" aria-label="Revert routing policy"><span class="material-symbols-outlined" aria-hidden="true">refresh</span></button>' +
      '<button type="button" class="sg-op-yaml-ov-btn sg-op-yaml-ov-btn--save sg-op-yaml-ov-save" data-admin-action="routing-policy-save">Save</button>' +
      "</div></div></div>" +
      '<h4 class="sum-section-label" style="margin-top:1rem">Dry-run router</h4>' +
      '<p class="sg-op-card-note sg-op-card-note--tight">Evaluate policy + fallback against a sample user message (same API as the retired admin panel).</p>' +
      '<label class="sg-op-label" for="admin-routing-eval-msg">Sample user message</label>' +
      '<input id="admin-routing-eval-msg" class="sg-op-input" type="text" placeholder="Short or long text…" style="width:100%;max-width:100%;box-sizing:border-box"/>' +
      '<label class="sg-op-label sg-op-label--inline" style="margin-top:0.5rem"><input type="checkbox" id="admin-routing-eval-smoke"/> Optional smoke <code>POST /v1/chat/completions</code> (1 token)</label>' +
      '<div class="sg-op-head-row" style="margin-top:0.35rem"><div class="sg-op-head-actions">' +
      '<button type="button" class="sg-op-btn sg-op-btn--ghost" data-admin-action="routing-evaluate">Dry-run</button>' +
      "</div></div>" +
      '<pre id="admin-routing-eval-out" class="sg-op-eval-out muted">—</pre>' +
      adminScopedEvlogPanelFromEvents("Scoped log — routing decisions", "routing-rules", adminScopedEventsForRouting("rules")) +
      "</div></details>"
    );
  }

  ctx.buildAdminRoutingRulesCardHtml = buildAdminRoutingRulesCardHtml;
};
