/**
 * Composer input: auto-resize, keyboard shortcuts.
 */
(function () {
  "use strict";

  var MAX_HEIGHT_PX = 160;

  function autoResize(ta) {
    if (!ta) return;
    ta.style.height = "auto";
    var next = Math.min(ta.scrollHeight, MAX_HEIGHT_PX);
    ta.style.height = next + "px";
    ta.style.overflowY = ta.scrollHeight > MAX_HEIGHT_PX ? "auto" : "hidden";
  }

  function mount(opts) {
    opts = opts || {};
    var ta = opts.textarea;
    var onSubmit = typeof opts.onSubmit === "function" ? opts.onSubmit : function () {};
    var getHistory = typeof opts.getHistory === "function" ? opts.getHistory : function () { return []; };

    var histIdx = -1;
    var draftBeforeHist = "";

    if (!ta) return { focus: function () {}, clear: function () {}, getValue: function () { return ""; } };

    ta.addEventListener("input", function () {
      autoResize(ta);
      histIdx = -1;
    });

    ta.addEventListener("keydown", function (ev) {
      if (ev.key === "Enter" && !ev.shiftKey) {
        ev.preventDefault();
        onSubmit();
        return;
      }
      if (ev.key === "Escape") {
        ta.blur();
        return;
      }
      if (ev.key === "ArrowUp" && !ev.shiftKey && !ev.altKey && !ev.ctrlKey && !ev.metaKey) {
        if (ta.value !== "") return;
        var hist = getHistory();
        if (!hist.length) return;
        ev.preventDefault();
        if (histIdx < 0) {
          draftBeforeHist = ta.value;
          histIdx = hist.length - 1;
        } else if (histIdx > 0) {
          histIdx--;
        }
        ta.value = hist[histIdx] || "";
        autoResize(ta);
      }
      if (ev.key === "ArrowDown" && !ev.shiftKey && !ev.altKey && !ev.ctrlKey && !ev.metaKey && histIdx >= 0) {
        ev.preventDefault();
        if (histIdx < getHistory().length - 1) {
          histIdx++;
          ta.value = getHistory()[histIdx] || "";
        } else {
          histIdx = -1;
          ta.value = draftBeforeHist;
        }
        autoResize(ta);
      }
    });

    return {
      focus: function () {
        ta.focus();
      },
      clear: function () {
        ta.value = "";
        histIdx = -1;
        draftBeforeHist = "";
        autoResize(ta);
      },
      getValue: function () {
        return String(ta.value || "");
      },
      setValue: function (v) {
        ta.value = v == null ? "" : String(v);
        autoResize(ta);
      },
      resize: function () {
        autoResize(ta);
      }
    };
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Render = globalThis.ChimeraChat.Render || {};
  globalThis.ChimeraChat.Render.Input = {
    mount: mount,
    autoResize: autoResize
  };
})();
