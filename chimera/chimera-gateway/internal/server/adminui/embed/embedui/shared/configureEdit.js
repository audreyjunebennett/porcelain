/**
 * Configure / edit-mode affordances shared across admin cards.
 */
(function () {
  "use strict";

  function configureBtnInline(escapeHtml, action, ariaLabel, title) {
    var esc = escapeHtml || function (s) { return String(s); };
    var lab = ariaLabel != null ? String(ariaLabel) : "Configure";
    var tit = title != null ? String(title) : "Configure";
    return (
      '<button type="button" class="sg-op-configure-btn sg-op-configure-btn--inline" data-admin-action="' +
      esc(String(action || "")) +
      '" aria-label="' +
      esc(lab) +
      '" title="' +
      esc(tit) +
      '"><span class="material-symbols-outlined" aria-hidden="true">settings</span></button>'
    );
  }

  /**
   * Restore ctx flags and optional drafts when the operator cancels an edit session.
   * opts: { editingKey, touchedKey, draftKey, draftValue, onAfter }
   */
  function restoreEditOnCancel(ctx, opts) {
    opts = opts || {};
    if (!ctx) return;
    if (opts.editingKey != null) ctx[opts.editingKey] = false;
    if (opts.touchedKey != null) ctx[opts.touchedKey] = false;
    if (opts.draftKey != null) {
      if (opts.draftValue === undefined || opts.draftValue === null) {
        delete ctx[opts.draftKey];
      } else {
        ctx[opts.draftKey] = opts.draftValue;
      }
    }
    if (typeof opts.onAfter === "function") opts.onAfter();
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.ConfigureEdit = {
    configureBtnInline: configureBtnInline,
    restoreEditOnCancel: restoreEditOnCancel
  };
})();
