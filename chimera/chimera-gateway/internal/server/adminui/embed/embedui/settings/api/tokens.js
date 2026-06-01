/**
 * Token list + label cache for admin users and log streaming.
 * Exports: ChimeraSettings.Api.mountTokensApi(ctx, bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Api = globalThis.ChimeraSettings.Api || {};

globalThis.ChimeraSettings.Api.mountTokensApi = function (ctx, bridge) {
  bridge = bridge || {};

  function markUnauthorized() {
    if (typeof bridge.markUnauthorized === "function") bridge.markUnauthorized();
    else if (typeof ctx.markUiUnauthorized === "function") ctx.markUiUnauthorized();
  }

  function applyTokenRows(rows, opts) {
    opts = opts || {};
    if (!Array.isArray(rows)) return;
    if (opts.storeListCache !== false) {
      ctx.tokenListCache = rows;
    }
    if (opts.labels) {
      ctx.tokenLabelByTenant = ctx.tokenLabelByTenant || {};
    }
    if (!ctx.adminCreatedTokenByTenant) ctx.adminCreatedTokenByTenant = {};
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i] || {};
      var tid = row.tenant_id != null ? String(row.tenant_id).trim() : "";
      if (!tid) continue;
      var tok = row.token != null ? String(row.token).trim() : "";
      if (tok) ctx.adminCreatedTokenByTenant[tid] = tok;
      if (opts.labels) {
        var lb =
          row.label != null && String(row.label).trim() !== "" ? String(row.label).trim() : "";
        ctx.tokenLabelByTenant[tid] = lb || tid;
      }
    }
  }

  function fetchTokenListFromApi() {
    if (ctx.uiUnauthorized) return Promise.resolve(null);
    return fetch("/api/ui/tokens", { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      });
  }

  function fetchAdminTokens() {
    return fetchTokenListFromApi().then(function (j) {
      if (!j) return;
      applyTokenRows(Array.isArray(j.tokens) ? j.tokens : [], { labels: false });
    });
  }

  function fetchTokenLabels() {
    if (ctx.uiUnauthorized) return;
    fetchTokenListFromApi()
      .then(function (data) {
        if (!data) return;
        applyTokenRows(Array.isArray(data.tokens) ? data.tokens : [], { labels: true, storeListCache: false });
        var getViewMode = bridge.getViewMode || ctx.getViewMode;
        if (typeof getViewMode === "function" && getViewMode() === "summarized") {
          if (typeof bridge.scheduleStoryRebuild === "function") bridge.scheduleStoryRebuild();
        }
      })
      .catch(function () {});
  }

  ctx.fetchAdminTokens = fetchAdminTokens;
  ctx.fetchTokenLabels = fetchTokenLabels;

  return {
    fetchAdminTokens: fetchAdminTokens,
    fetchTokenLabels: fetchTokenLabels
  };
};
