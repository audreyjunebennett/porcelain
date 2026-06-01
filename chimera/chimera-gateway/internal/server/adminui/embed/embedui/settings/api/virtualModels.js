/**
 * Virtual model detail fetch + adminStateCache summary sync.
 * Exports: ChimeraSettings.Api.mountVirtualModelsApi(ctx, bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Api = globalThis.ChimeraSettings.Api || {};

globalThis.ChimeraSettings.Api.mountVirtualModelsApi = function (ctx, bridge) {
  bridge = bridge || {};

  function markUnauthorized() {
    if (typeof bridge.markUnauthorized === "function") bridge.markUnauthorized();
    else if (typeof ctx.markUiUnauthorized === "function") ctx.markUiUnauthorized();
  }

  function syncVmSummaryFromDetail(detail) {
    if (!detail || detail.id == null) return;
    var gw = ctx.adminStateCache && ctx.adminStateCache.gateway;
    if (!gw || !gw.virtual_models) return;
    var key = String(detail.id);
    for (var i = 0; i < gw.virtual_models.length; i++) {
      if (gw.virtual_models[i] && String(gw.virtual_models[i].id) === key) {
        var row = gw.virtual_models[i];
        row.enabled = !!detail.enabled;
        row.name = detail.name;
        row.version = detail.version;
        row.description = detail.description;
        row.visibility = detail.visibility;
        row.routing_policy_enabled = !!detail.routing_policy_enabled;
        row.tool_router_enabled = !!detail.tool_router_enabled;
        row.router_models = detail.router_models;
        row.fallback_depth = detail.fallback_chain && detail.fallback_chain.length ? detail.fallback_chain.length : 0;
        break;
      }
    }
  }

  function fetchVirtualModelDetail(vmId, force) {
    if (ctx.uiUnauthorized) return Promise.resolve(null);
    var key = String(vmId);
    if (!ctx.virtualModelDetails) ctx.virtualModelDetails = {};
    if (!ctx.virtualModelUi) ctx.virtualModelUi = {};
    var ui = ctx.virtualModelUi[key];
    if (!ui) {
      ui = ctx.virtualModelUi[key] = { panelOpen: false, hydrated: false };
    }
    if (!force && ctx.virtualModelDetails[key]) {
      return Promise.resolve(ctx.virtualModelDetails[key]);
    }
    ui.detailLoading = true;
    return fetch("/api/ui/virtual-models/" + encodeURIComponent(key), { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (j) {
        ui.detailLoading = false;
        if (!j) return null;
        ctx.virtualModelDetails[key] = j;
        syncVmSummaryFromDetail(j);
        return j;
      })
      .catch(function (e) {
        ui.detailLoading = false;
        throw e;
      });
  }

  ctx.fetchVirtualModelDetail = fetchVirtualModelDetail;

  return {
    fetchVirtualModelDetail: fetchVirtualModelDetail,
    syncVmSummaryFromDetail: syncVmSummaryFromDetail
  };
};
