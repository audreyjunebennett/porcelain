/**
 * Shared JSON admin HTTP helpers for settings handlers.
 * Exports: ChimeraSettings.Api.mountAdminClient(ctx)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Api = globalThis.ChimeraSettings.Api || {};

globalThis.ChimeraSettings.Api.mountAdminClient = function (ctx) {
  function adminJsonRequest(url, method, body) {
    return fetch(url, {
      method: method || "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body || {})
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (r.status === 401) throw new Error("Unauthorized");
        if (!r.ok) throw new Error((j && (j.error || (j.error && j.error.message))) || ("HTTP " + r.status));
        return j;
      });
    });
  }

  function adminPostJSON(url, body) {
    return adminJsonRequest(url, "POST", body);
  }

  function adminPutJSON(url, body) {
    return adminJsonRequest(url, "PUT", body);
  }

  ctx.adminPostJSON = adminPostJSON;
  ctx.adminPutJSON = adminPutJSON;

  return {
    adminPostJSON: adminPostJSON,
    adminPutJSON: adminPutJSON,
    adminJsonRequest: adminJsonRequest
  };
};
