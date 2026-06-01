/**
 * Icon toolbar buttons (provider models, VM sections, managed workspace).
 */
(function () {
  "use strict";

  function escAttr(escapeHtml, s) {
    return escapeHtml ? escapeHtml(String(s)) : String(s);
  }

  /**
   * opts: action, title, icon, extraClass, disabled, dataAttrs { key: value }
   */
  function iconBtnHtml(escapeHtml, opts) {
    opts = opts || {};
    var cls = "sg-op-yaml-ov-btn";
    if (opts.extraClass) cls += " " + String(opts.extraClass);
    var disabled = opts.disabled ? " disabled" : "";
    var attrs =
      ' type="button" class="' +
      escAttr(escapeHtml, cls) +
      '"';
    if (opts.action) {
      attrs += ' data-admin-action="' + escAttr(escapeHtml, opts.action) + '"';
    }
    var dataAttrs = opts.dataAttrs || {};
    var k;
    for (k in dataAttrs) {
      if (!Object.prototype.hasOwnProperty.call(dataAttrs, k)) continue;
      if (dataAttrs[k] == null) continue;
      attrs += " data-" + k + '="' + escAttr(escapeHtml, dataAttrs[k]) + '"';
    }
    var lab = opts.title != null ? String(opts.title) : "";
    attrs += ' title="' + escAttr(escapeHtml, lab) + '" aria-label="' + escAttr(escapeHtml, lab) + '"' + disabled;
    return (
      "<button" +
      attrs +
      '><span class="material-symbols-outlined" aria-hidden="true">' +
      escAttr(escapeHtml, opts.icon || "") +
      "</span></button>"
    );
  }

  function toolbarWrapHtml(innerHtml, wrapClass) {
    var cls = wrapClass != null ? String(wrapClass) : "ws-managed-edit-controls";
    return '<div class="' + cls + '">' + (innerHtml || "") + "</div>";
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.EditToolbar = {
    iconBtnHtml: iconBtnHtml,
    toolbarWrapHtml: toolbarWrapHtml
  };
})();
