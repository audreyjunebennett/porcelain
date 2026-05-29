/**
 * Admin YAML editor chrome (.sg-op-yaml-wrap) with CodeMirror 6.
 */

function esc() {
  return globalThis.ChimeraUI && globalThis.ChimeraUI.escapeHtml
    ? globalThis.ChimeraUI.escapeHtml
    : function (s) {
        return String(s);
      };
}

function escA() {
  return globalThis.ChimeraUI && globalThis.ChimeraUI.escapeAttr
    ? globalThis.ChimeraUI.escapeAttr
    : esc();
}

var instances = Object.create(null);

function cmApi() {
  return globalThis.ChimeraCodeMirror || null;
}

function textareaSelector() {
  return "textarea.sg-op-yaml-textarea, textarea.sg-op-code";
}

function syncOverlayVScroll(wrap) {
  if (!wrap) return;
  var scroller = wrap.querySelector(".cm-scroller");
  if (scroller) {
    wrap.classList.toggle("sg-op-yaml-wrap--vscroll", scroller.scrollHeight > scroller.clientHeight + 1);
    return;
  }
  var ta = wrap.querySelector(textareaSelector());
  if (ta && !ta.classList.contains("sg-op-yaml-textarea--sync")) {
    wrap.classList.toggle("sg-op-yaml-wrap--vscroll", ta.scrollHeight > ta.clientHeight + 1);
  }
}

function syncOverlayVScrollFromTarget(t) {
  if (!t) return;
  if (t.classList && t.classList.contains("sg-op-yaml-wrap")) {
    syncOverlayVScroll(t);
    return;
  }
  if (String(t.tagName || "").toLowerCase() === "textarea") {
    var wrapTa = t.closest && t.closest(".sg-op-yaml-wrap");
    if (wrapTa) syncOverlayVScroll(wrapTa);
    return;
  }
  if (t.classList && t.classList.contains("cm-scroller")) {
    var wrapCm = t.closest && t.closest(".sg-op-yaml-wrap");
    if (wrapCm) syncOverlayVScroll(wrapCm);
  }
}

function destroyEditor(id) {
  var key = String(id || "");
  if (!key) return;
  var inst = instances[key];
  if (!inst) return;
  var api = cmApi();
  if (inst.view && api && typeof api.getValue === "function") {
    inst.textarea.value = api.getValue(inst.view);
  }
  if (inst.view && typeof inst.view.destroy === "function") {
    inst.view.destroy();
  }
  if (inst.host && inst.host.parentNode) {
    inst.host.parentNode.removeChild(inst.host);
  }
  if (inst.textarea) {
    inst.textarea.classList.remove("sg-op-yaml-textarea--sync");
    inst.textarea.removeAttribute("aria-hidden");
    inst.textarea.tabIndex = 0;
  }
  delete instances[key];
}

function destroyIn(root) {
  var scope = root || document;
  var wraps = scope.querySelectorAll ? scope.querySelectorAll(".sg-op-yaml-wrap") : [];
  for (var i = 0; i < wraps.length; i++) {
    var ta = wraps[i].querySelector(textareaSelector());
    if (ta && ta.id) destroyEditor(ta.id);
  }
}

function mountWrap(wrap) {
  if (!wrap || wrap.getAttribute("data-yaml-editor-skip") === "true") return;
  var ta = wrap.querySelector(textareaSelector());
  if (!ta || !ta.id || instances[ta.id]) return;
  var api = cmApi();
  if (!api || typeof api.createYamlEditor !== "function") return;

  var host = document.createElement("div");
  host.className = "sg-op-yaml-cm";
  host.setAttribute("role", "presentation");
  wrap.insertBefore(host, ta);
  ta.classList.add("sg-op-yaml-textarea--sync");
  ta.setAttribute("aria-hidden", "true");
  ta.tabIndex = -1;

  var view = api.createYamlEditor(host, {
    value: ta.value != null ? String(ta.value) : "",
    readOnly: !!ta.readOnly,
    onChange: function (val) {
      ta.value = val;
      ta.dispatchEvent(new Event("input", { bubbles: true }));
      syncOverlayVScroll(wrap);
    },
    onFocus: function () {
      wrap.classList.add("sg-op-yaml-wrap--active");
    },
    onBlur: function () {
      wrap.classList.remove("sg-op-yaml-wrap--active");
    },
    onScroll: function () {
      syncOverlayVScroll(wrap);
    },
  });

  instances[ta.id] = { view: view, textarea: ta, wrap: wrap, host: host };
  syncOverlayVScroll(wrap);
  window.requestAnimationFrame(function () {
    if (instances[ta.id] && api.requestMeasure) api.requestMeasure(view);
    syncOverlayVScroll(wrap);
  });
}

function mountAll(root) {
  var scope = root || document;
  var wraps = scope.querySelectorAll ? scope.querySelectorAll(".sg-op-yaml-wrap") : [];
  for (var i = 0; i < wraps.length; i++) mountWrap(wraps[i]);
}

function getValue(id) {
  var key = String(id || "");
  var inst = instances[key];
  var api = cmApi();
  if (inst && inst.view && api && typeof api.getValue === "function") {
    return api.getValue(inst.view);
  }
  var ta = document.getElementById(key);
  return ta && ta.value != null ? String(ta.value) : "";
}

function setValue(id, value) {
  var key = String(id || "");
  var next = value != null ? String(value) : "";
  var inst = instances[key];
  var api = cmApi();
  if (inst && inst.view && api && typeof api.setValue === "function") {
    api.setValue(inst.view, next);
    inst.textarea.value = next;
    syncOverlayVScroll(inst.wrap);
    return;
  }
  var ta = document.getElementById(key);
  if (ta) ta.value = next;
}

function remountAll(root) {
  destroyIn(root);
  mountAll(root);
}

/**
 * @param {{id: string, value: string, dirty?: boolean, full?: boolean, rows?: number, overlayHtml?: string}} model
 * @returns {string}
 */
function render(model) {
  model = model || {};
  var id = model.id != null ? String(model.id) : "yaml-editor";
  var value = model.value != null ? String(model.value) : "";
  var wrapCls = "sg-op-yaml-wrap";
  if (model.full) wrapCls += " sg-op-yaml-wrap--full";
  if (model.dirty) wrapCls += " sg-op-yaml-wrap--dirty";
  var rows = model.rows != null ? model.rows : 10;
  var overlay = model.overlayHtml != null ? String(model.overlayHtml) : "";
  return (
    '<div id="' +
    escA()(id) +
    '-wrap" class="' +
    escA()(wrapCls) +
    '">' +
    '<textarea id="' +
    escA()(id) +
    '" class="sg-op-yaml-textarea" rows="' +
    escA()(String(rows)) +
    '" spellcheck="false">' +
    esc(value) +
    "</textarea>" +
    '<div class="sg-op-yaml-ov">' +
    overlay +
    "</div></div>"
  );
}

globalThis.ChimeraUI = globalThis.ChimeraUI || {};
globalThis.ChimeraUI.YamlEditorPanel = {
  render: render,
  mountAll: mountAll,
  destroyIn: destroyIn,
  remountAll: remountAll,
  destroyEditor: destroyEditor,
  getValue: getValue,
  setValue: setValue,
  syncOverlayVScroll: syncOverlayVScroll,
  syncOverlayVScrollFromTarget: syncOverlayVScrollFromTarget,
};
