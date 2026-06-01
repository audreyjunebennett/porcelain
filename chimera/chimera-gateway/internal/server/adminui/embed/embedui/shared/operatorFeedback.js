/**
 * Operator status line and save-button pending state (settings + wizard).
 */
(function () {
  "use strict";

  function setStatusMessage(statusEl, kind, msg) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.className = msg
      ? kind === "err"
        ? "status-line err"
        : "status-line"
      : "status-line";
  }

  function setSaveBtnPending(btn, pending) {
    if (!btn) return;
    btn.disabled = !!pending;
    if (pending) btn.setAttribute("aria-disabled", "true");
    else btn.removeAttribute("aria-disabled");
  }

  function bindStatusApi(statusEl) {
    return {
      setMessage: function (kind, msg) {
        setStatusMessage(statusEl, kind, msg);
      }
    };
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.OperatorFeedback = {
    setStatusMessage: setStatusMessage,
    setSaveBtnPending: setSaveBtnPending,
    bindStatusApi: bindStatusApi
  };
})();
