/**
 * Virtual model cards for /ui/settings summarized feed.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminVirtualModels = function (ctx) {
  var escapeHtml = ctx.escapeHtml;

  function buildVirtualModelCardHtml(vm) {
    var chips =
      (vm.enabled ? '<span class="sum-chip sum-chip--ok">enabled</span>' : '<span class="sum-chip">disabled</span>') +
      '<span class="sum-chip">' + escapeHtml(String(vm.visibility || "public")) + "</span>" +
      '<span class="sum-chip">fallback×' + String(vm.fallback_depth != null ? vm.fallback_depth : 0) + "</span>";
    if (vm.routing_policy_enabled) {
      chips += '<span class="sum-chip">routing</span>';
    }
    if (vm.tool_router_enabled) {
      chips += '<span class="sum-chip">tool-router</span>';
    }
    return (
      '<article class="sum-card sum-card--virtual-model" id="vm-card-' + escapeHtml(String(vm.id)) + '" data-virtual-model-id="' +
      escapeHtml(String(vm.id)) +
      '">' +
      '<header class="sum-card__hdr">' +
      '<span class="sum-main">' +
      '<span class="sum-title"><code class="sum-mono-id">' +
      escapeHtml(String(vm.model_id || "")) +
      "</code></span>" +
      '<span class="sum-sub sum-sub--clamp">' +
      escapeHtml(String(vm.name || "") + " " + String(vm.version || "")) +
      (vm.description ? " — " + escapeHtml(String(vm.description)) : "") +
      "</span>" +
      "</span>" +
      '<span class="sum-chips">' +
      chips +
      "</span>" +
      "</header>" +
      "</article>"
    );
  }

  function buildVirtualModelsSectionIntroHtml(count) {
    return (
      '<p class="intro">Operator-managed virtual models (<strong>' +
      String(count) +
      "</strong>). Each model owns fallback, routing policy, and optional tool-router settings stored in operator SQLite.</p>"
    );
  }

  ctx.buildVirtualModelCardHtml = buildVirtualModelCardHtml;
  ctx.buildVirtualModelsSectionIntroHtml = buildVirtualModelsSectionIntroHtml;
};
