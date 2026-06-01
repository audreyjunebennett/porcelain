/**
 * Shared JSON save/action helper (settings handlers + wizard).
 */
(function () {
  "use strict";

  function errorMessage(err) {
    return err && err.message ? String(err.message) : String(err);
  }

  /**
   * opts: { request, setMessage, triggerBtn, setPending, successMsg, successKind, onSuccess, onError }
   * request: function () returning a Promise
   */
  function runJson(opts) {
    opts = opts || {};
    var req = opts.request;
    if (typeof req !== "function") return Promise.resolve();
    if (opts.triggerBtn && typeof opts.setPending === "function") {
      opts.setPending(opts.triggerBtn, true);
    }
    return req()
      .then(function (result) {
        if (typeof opts.setMessage === "function") {
          opts.setMessage(opts.successKind != null ? opts.successKind : "", opts.successMsg != null ? opts.successMsg : "");
        }
        if (typeof opts.onSuccess === "function") return opts.onSuccess(result);
        return result;
      })
      .catch(function (err) {
        if (typeof opts.setMessage === "function") opts.setMessage("err", errorMessage(err));
        if (typeof opts.onError === "function") opts.onError(err);
        throw err;
      })
      .finally(function () {
        if (opts.triggerBtn && typeof opts.setPending === "function") {
          opts.setPending(opts.triggerBtn, false);
        }
      });
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.AdminAction = {
    runJson: runJson,
    errorMessage: errorMessage
  };
})();
