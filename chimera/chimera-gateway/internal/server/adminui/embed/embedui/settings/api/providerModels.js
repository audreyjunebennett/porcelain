/**
 * Provider model catalog fetch + poll prefetch for admin provider cards.
 * Exports: ChimeraSettings.Api.mountProviderModelsApi(ctx, bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Api = globalThis.ChimeraSettings.Api || {};

globalThis.ChimeraSettings.Api.mountProviderModelsApi = function (ctx, bridge) {
  bridge = bridge || {};
  var providerModelsPrefetchInFlight = Object.create(null);

  function adminVisibleProviderIds() {
    if (typeof bridge.adminVisibleProviderIds === "function") return bridge.adminVisibleProviderIds();
    return Array.isArray(ctx.adminVisibleProviderIds) ? ctx.adminVisibleProviderIds : [];
  }

  function fetchProviderModels(providerId) {
    if (ctx.uiUnauthorized) return Promise.reject(new Error("Unauthorized"));
    var pid = String(providerId || "").trim().toLowerCase();
    if (!pid) return Promise.reject(new Error("provider required"));
    return fetch("/api/ui/providers/" + encodeURIComponent(pid) + "/models", { credentials: "same-origin" })
      .then(function (r) {
        return r.json().catch(function () { return {}; }).then(function (j) {
          if (r.status === 401) throw new Error("Unauthorized");
          if (!r.ok) throw new Error((j && j.error) || ("HTTP " + r.status));
          return j;
        });
      });
  }

  function providerIdsNeedingModelsPrefetch() {
    var st = ctx.adminStateCache || {};
    var providers = st.providers || {};
    var hasCreds = typeof ctx.providerHasCredentials === "function" ? ctx.providerHasCredentials : null;
    var visible = adminVisibleProviderIds();
    var out = [];
    for (var i = 0; i < visible.length; i++) {
      var pid = String(visible[i] || "").trim().toLowerCase();
      if (!pid) continue;
      if (ctx.adminProviderModelsEditingId === pid) continue;
      if (ctx.adminProviderModelsCache && ctx.adminProviderModelsCache[pid]) continue;
      var prow = providers[pid] || {};
      if (!prow.models_configured) continue;
      if (hasCreds && !hasCreds(pid, prow)) continue;
      out.push(pid);
    }
    return out;
  }

  function prefetchProviderModelsAvailability() {
    if (ctx.uiUnauthorized || typeof fetchProviderModels !== "function") return Promise.resolve(false);
    var ids = providerIdsNeedingModelsPrefetch();
    if (!ids.length) return Promise.resolve(false);
    var jobs = [];
    for (var i = 0; i < ids.length; i++) {
      (function (pid) {
        if (providerModelsPrefetchInFlight[pid]) {
          jobs.push(providerModelsPrefetchInFlight[pid]);
          return;
        }
        providerModelsPrefetchInFlight[pid] = fetchProviderModels(pid)
          .then(function (doc) {
            if (!ctx.adminProviderModelsCache) ctx.adminProviderModelsCache = {};
            ctx.adminProviderModelsCache[pid] = doc;
          })
          .catch(function () {
            /* keep read-only fallback until a later poll retries */
          })
          .finally(function () {
            delete providerModelsPrefetchInFlight[pid];
          });
        jobs.push(providerModelsPrefetchInFlight[pid]);
      })(ids[i]);
    }
    return Promise.all(jobs).then(function () {
      var getViewMode = bridge.getViewMode || ctx.getViewMode;
      if (typeof getViewMode !== "function" || getViewMode() !== "summarized") return false;
      var patchAdminProviderCard =
        bridge.patchAdminProviderCard ||
        (typeof ctx.patchAdminProviderCard === "function" ? ctx.patchAdminProviderCard : null);
      var anyPatched = false;
      var needRebuild = false;
      for (var j = 0; j < ids.length; j++) {
        var id = ids[j];
        if (!ctx.adminProviderModelsCache || !ctx.adminProviderModelsCache[id]) continue;
        if (patchAdminProviderCard && patchAdminProviderCard(id)) anyPatched = true;
        else needRebuild = true;
      }
      if (needRebuild && typeof bridge.scheduleStoryRebuild === "function") {
        bridge.scheduleStoryRebuild();
      }
      return anyPatched;
    });
  }

  ctx.fetchProviderModels = fetchProviderModels;
  ctx.providerIdsNeedingModelsPrefetch = providerIdsNeedingModelsPrefetch;
  ctx.prefetchProviderModelsAvailability = prefetchProviderModelsAvailability;

  return {
    fetchProviderModels: fetchProviderModels,
    providerIdsNeedingModelsPrefetch: providerIdsNeedingModelsPrefetch,
    prefetchProviderModelsAvailability: prefetchProviderModelsAvailability
  };
};
