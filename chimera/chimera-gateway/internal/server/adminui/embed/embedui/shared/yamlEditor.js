/**
 * YAML overlay dirty state and vertical-scroll class (delegates to ChimeraUI when present).
 */
(function () {
  "use strict";

  function syncOverlayVScrollFromTarget(target) {
    var yep = globalThis.ChimeraUI && globalThis.ChimeraUI.YamlEditorPanel;
    if (yep && typeof yep.syncOverlayVScrollFromTarget === "function") {
      yep.syncOverlayVScrollFromTarget(target);
      return;
    }
    if (!target || String(target.tagName || "").toLowerCase() !== "textarea") return;
    var wrap = target.closest && target.closest(".sg-op-yaml-wrap");
    if (!wrap) return;
    wrap.classList.toggle("sg-op-yaml-wrap--vscroll", target.scrollHeight > target.clientHeight + 1);
  }

  function setWrapDirty(wrapEl, dirty) {
    if (!wrapEl) return;
    wrapEl.classList.toggle("sg-op-yaml-wrap--dirty", !!dirty);
  }

  function textareaWrapHtml(escapeHtml, opts) {
    opts = opts || {};
    var esc = escapeHtml || function (s) { return String(s); };
    var wrapCls = "sg-op-yaml-wrap sg-op-yaml-wrap--full";
    if (opts.full === false) wrapCls = "sg-op-yaml-wrap";
    if (opts.touched) wrapCls += " sg-op-yaml-wrap--dirty";
    if (opts.wrapClass) wrapCls += " " + String(opts.wrapClass);
    var id = opts.id != null ? String(opts.id) : "";
    var rows = opts.rows != null ? opts.rows : 8;
    var yaml = opts.yaml != null ? String(opts.yaml) : "";
    var ro = opts.readonly ? " readonly" : "";
    var spell = opts.spellcheck === false ? ' spellcheck="false"' : "";
    return (
      '<div class="' +
      esc(wrapCls) +
      '"><textarea id="' +
      esc(id) +
      '" class="sg-op-yaml-textarea" rows="' +
      esc(String(rows)) +
      '"' +
      spell +
      ro +
      ">" +
      esc(yaml) +
      "</textarea></div>"
    );
  }

  /**
   * Mark VM/admin YAML textarea dirty from input; opts: { wrapSelector, onDirty(ui), ui }
   */
  function applyTextareaInputDirty(target, opts) {
    opts = opts || {};
    if (!target || !target.id) return false;
    var wrap =
      (opts.wrapSelector && target.closest && target.closest(opts.wrapSelector)) ||
      (target.closest && target.closest(".sg-op-yaml-wrap"));
    if (wrap) setWrapDirty(wrap, true);
    if (typeof opts.onDirty === "function" && opts.ui) opts.onDirty(opts.ui, target);
    syncOverlayVScrollFromTarget(target);
    return true;
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.YamlEditor = {
    syncOverlayVScrollFromTarget: syncOverlayVScrollFromTarget,
    setWrapDirty: setWrapDirty,
    textareaWrapHtml: textareaWrapHtml,
    applyTextareaInputDirty: applyTextareaInputDirty
  };
})();
