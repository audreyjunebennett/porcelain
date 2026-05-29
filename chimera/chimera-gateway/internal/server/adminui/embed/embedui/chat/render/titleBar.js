/**
 * Conversation title header (display + inline edit).
 */
(function () {
  "use strict";

  var esc =
    globalThis.ChimeraUI && ChimeraUI.escapeHtml
      ? ChimeraUI.escapeHtml
      : function (s) {
          return String(s || "");
        };

  function escAttr(s) {
    return esc(s).replace(/"/g, "&quot;");
  }

  function mount(opts) {
    opts = opts || {};
    var root = opts.root;
    var onSave = typeof opts.onSave === "function" ? opts.onSave : function () {};
    if (!root) return null;

    var savedTitle = "";
    var editing = false;
    var editSnapshot = "";

    function defaultLabel() {
      return "New chat";
    }

    function displayText() {
      var t = String(savedTitle || "").trim();
      return t || defaultLabel();
    }

    function renderDisplay() {
      root.innerHTML =
        '<div class="chat-title-bar">' +
        '<h1 class="chat-title" id="chat-title-heading">' +
        esc(displayText()) +
        "</h1>" +
        '<button type="button" class="chat-title__edit-btn" aria-label="Edit title" title="Edit title">' +
        '<span class="material-symbols-outlined" aria-hidden="true">edit</span>' +
        "</button></div>";
    }

    function finishEdit(commit) {
      if (!editing) return;
      var input = root.querySelector(".chat-title__input");
      editing = false;
      if (!commit) {
        savedTitle = editSnapshot;
        renderDisplay();
        return;
      }
      var next = input ? String(input.value || "").trim() : editSnapshot;
      if (!next) {
        savedTitle = editSnapshot;
        renderDisplay();
        return;
      }
      var prev = editSnapshot;
      savedTitle = next;
      renderDisplay();
      onSave(next, prev);
    }

    function onInputKeydown(ev) {
      if (ev.key === "Escape") {
        ev.preventDefault();
        finishEdit(false);
      } else if (ev.key === "Enter") {
        ev.preventDefault();
        finishEdit(true);
      }
    }

    function onInputBlur() {
      window.setTimeout(function () {
        if (editing) finishEdit(true);
      }, 0);
    }

    function renderEdit() {
      root.innerHTML =
        '<div class="chat-title-bar chat-title-bar--editing">' +
        '<input type="text" class="chat-title__input" id="chat-title-input" ' +
        'aria-label="Conversation title" maxlength="256" value="' +
        escAttr(editSnapshot) +
        '" /></div>';
      var input = root.querySelector(".chat-title__input");
      if (!input) return;
      input.addEventListener("keydown", onInputKeydown);
      input.addEventListener("blur", onInputBlur);
      input.focus();
      input.select();
    }

    function startEdit() {
      if (editing) return;
      editing = true;
      editSnapshot = String(savedTitle || "").trim();
      renderEdit();
    }

    root.addEventListener("click", function (ev) {
      var t = ev.target;
      if (!t || !t.closest) return;
      if (t.closest(".chat-title__edit-btn")) startEdit();
    });

    renderDisplay();

    return {
      setTitle: function (title) {
        savedTitle = String(title || "").trim();
        if (!editing) renderDisplay();
      },
      getTitle: function () {
        return String(savedTitle || "").trim();
      },
      startEdit: startEdit
    };
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Render = globalThis.ChimeraChat.Render || {};
  globalThis.ChimeraChat.Render.TitleBar = { mount: mount };
})();
