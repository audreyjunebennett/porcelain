/**
 * Watched-paths editor row (workspace draft + managed workspace edit).
 */
(function () {
  "use strict";

  function esc(escapeHtml, s) {
    return escapeHtml ? escapeHtml(String(s)) : String(s);
  }

  /**
   * opts.paths: string[] or { path: string }[]
   * opts.selectSize, opts.selectClass, opts.selectAttr, opts.wrapClass, opts.wrapAttr
   * opts.addDisabled, opts.removeDisabled, opts.addBtnClass, opts.removeBtnClass
   */
  function pathsEditorHtml(escapeHtml, opts) {
    opts = opts || {};
    var rows = opts.paths && opts.paths.length ? opts.paths : [];
    var selOpts = "";
    var pi;
    for (pi = 0; pi < rows.length; pi++) {
      var path =
        typeof rows[pi] === "string" ? rows[pi] : rows[pi] && rows[pi].path != null ? rows[pi].path : "";
      selOpts += '<option value="' + esc(escapeHtml, String(pi)) + '">' + esc(escapeHtml, String(path)) + "</option>";
    }
    var size = opts.selectSize != null ? Number(opts.selectSize) : 6;
    var selCls = opts.selectClass || "ws-draft-paths-select";
    var selAttr = opts.selectAttr ? " " + opts.selectAttr : "";
    var addDis = opts.addDisabled ? " disabled" : "";
    var rmDis = "";
    if (opts.removeDisabled === true || (opts.removeDisabled !== false && !rows.length)) {
      rmDis = " disabled";
    }
    var addCls = opts.addBtnClass || "sg-op-btn sg-op-btn--ghost ws-draft-btn-add";
    var rmCls = opts.removeBtnClass || "sg-op-btn sg-op-btn--ghost ws-draft-btn-remove";
    var inner =
      '<select class="' +
      esc(escapeHtml, selCls) +
      '" size="' +
      esc(escapeHtml, String(size)) +
      '" aria-label="Watched paths"' +
      selAttr +
      ">" +
      selOpts +
      "</select>" +
      '<div class="ws-draft-path-btns">' +
      '<button type="button" class="' +
      esc(escapeHtml, addCls) +
      '"' +
      addDis +
      ">Add</button>" +
      '<button type="button" class="' +
      esc(escapeHtml, rmCls) +
      '"' +
      rmDis +
      ">Remove</button>" +
      "</div>";
    if (opts.wrapClass) {
      var wrapAttr = opts.wrapAttr ? " " + opts.wrapAttr : "";
      return '<div class="' + esc(escapeHtml, opts.wrapClass) + '"' + wrapAttr + ">" + inner + "</div>";
    }
    return '<div class="ws-draft-paths-row">' + inner + "</div>";
  }

  function folderPickerHintHtml() {
    return (
      '<p class="muted ws-draft-hint">Folder picker requires the Chimera desktop shell (or an environment that exposes <code>chimeraPickFolder</code>).</p>'
    );
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.WorkspacePaths = {
    pathsEditorHtml: pathsEditorHtml,
    folderPickerHintHtml: folderPickerHintHtml
  };
})();
