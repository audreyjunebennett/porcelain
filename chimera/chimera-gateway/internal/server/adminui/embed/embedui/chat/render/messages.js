/**
 * Message list rendering.
 */
(function () {
  "use strict";

  var esc =
    globalThis.ChimeraUI && ChimeraUI.escapeHtml
      ? ChimeraUI.escapeHtml
      : function (s) {
          return String(s || "");
        };

  var COPY_ICON_SVG =
    '<svg class="sum-evlog__copy-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';

  function md() {
    return globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Markdown &&
      typeof ChimeraChat.Render.Markdown.render === "function"
      ? ChimeraChat.Render.Markdown
      : null;
  }

  function renderAssistantMarkdown(content) {
    var renderer = md();
    if (!renderer) return esc(content || "");
    if (typeof renderer.renderSafe === "function") {
      return renderer.renderSafe(content || "");
    }
    if (typeof renderer.renderPartial === "function") {
      return renderer.renderPartial(content || "");
    }
    var html = renderer.render(content || "");
    if (typeof renderer.closeOpenHtmlTags === "function") {
      html = renderer.closeOpenHtmlTags(html);
    }
    return html;
  }

  function roleLabel(role) {
    if (role === "user") return "You";
    if (role === "error") return "Error";
    return "Assistant";
  }

  function displayModel(msg) {
    if (!msg) return "";
    var m = msg.upstreamModel || msg.selectedModel || "";
    return String(m).trim();
  }

  function renderRoleHead(msg) {
    var label = roleLabel(msg.role);
    if (msg.role !== "assistant") {
      return '<span class="chat-msg__role">' + esc(label) + "</span>";
    }
    var model = displayModel(msg);
    if (!model) {
      return '<span class="chat-msg__role">' + esc(label) + "</span>";
    }
    return (
      '<span class="chat-msg__role">' +
      esc(label) +
      ' <span class="chat-msg__model">(Model: <code>' +
      esc(model) +
      "</code>)</span></span>"
    );
  }

  function renderScoreMeta(score) {
    if (!score) return "";
    return (
      '<span class="chat-embed-item__meta" title="Retrieval confidence score">' +
      '<span class="chat-embed-item__score">' +
      esc(score) +
      "</span>" +
      '<span class="material-symbols-outlined material-symbols-outlined--sm chat-embed-item__score-icon" aria-hidden="true">readiness_score</span>' +
      "</span>"
    );
  }

  function renderRAGHitItems(hits) {
    if (!hits || !hits.length) return "";
    var snippetFn =
      globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Snippet &&
      typeof ChimeraChat.Render.Snippet.render === "function"
        ? ChimeraChat.Render.Snippet.render
        : null;
    var items = "";
    for (var i = 0; i < hits.length; i++) {
      var h = hits[i] || {};
      var src = h.source != null ? String(h.source) : "source";
      var text = h.text != null ? String(h.text) : "";
      var langHint = h.language != null ? String(h.language) : "";
      var score = h.score != null && !isNaN(Number(h.score)) ? Number(h.score).toFixed(3) : "";
      var body = snippetFn
        ? snippetFn(src, text, langHint)
        : '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' + esc(text) + "</code></pre>";
      items +=
        '<li class="chat-embed-item">' +
        "<details>" +
        '<summary class="chat-embed-item__summary">' +
        '<span class="chat-embed-item__source">' +
        esc(src) +
        "</span>" +
        renderScoreMeta(score) +
        "</summary>" +
        body +
        "</details></li>";
    }
    return items;
  }

  function renderMessageFooter(msg) {
    var hasRag = msg.ragHits && msg.ragHits.length;
    if (!hasRag) return "";

    var panelId = "chat-snippets-" + esc(msg.id);
    return (
      '<hr class="chat-msg__divider" aria-hidden="true"><div class="chat-msg__footer">' +
      '<button type="button" class="chat-msg__snippets-toggle" aria-expanded="false" aria-controls="' +
      panelId +
      '">Workspace Snippets (' +
      msg.ragHits.length +
      ")</button>" +
      '<div class="chat-msg__snippets-panel" id="' +
      panelId +
      '" hidden><ul class="chat-embed-list">' +
      renderRAGHitItems(msg.ragHits) +
      "</ul></div></div>"
    );
  }

  function renderBodyContent(msg) {
    if (msg.role === "error") {
      return esc(msg.error || msg.content);
    }
    if (msg.role === "assistant") {
      return renderAssistantMarkdown(msg.content || "");
    }
    return esc(msg.content || "");
  }

  function bodyClass(msg) {
    var cls = "chat-msg__body";
    if (msg.role === "assistant") cls += " chat-msg__body--markdown";
    if (msg.status === "streaming") cls += " chat-msg__body--streaming";
    return cls;
  }

  function renderCopyButton(copyText) {
    return (
      '<button type="button" class="sum-evlog__copy-btn chat-msg__copy" data-copy-text="' +
      esc(copyText) +
      '" title="Copy message" aria-label="Copy message">' +
      COPY_ICON_SVG +
      "</button>"
    );
  }

  function renderMessage(msg) {
    if (!msg) return "";
    var cls = "chat-msg chat-msg--assistant";
    if (msg.role === "user") cls = "chat-msg chat-msg--user";
    if (msg.role === "error") cls = "chat-msg chat-msg--error";

    var copyText = msg.role === "error" ? msg.error || msg.content : msg.content;
    var retryBtn = "";
    if (msg.role === "error" && msg.retryUserText) {
      retryBtn =
        '<div class="chat-msg__retry"><button type="button" class="btn btn--primary chat-msg__retry-btn" data-retry-text="' +
        esc(msg.retryUserText) +
        '">Retry</button></div>';
    }

    return (
      '<article class="' +
      cls +
      '" data-msg-id="' +
      esc(msg.id) +
      '">' +
      '<div class="chat-msg__head">' +
      renderRoleHead(msg) +
      '<div class="chat-msg__actions">' +
      renderCopyButton(copyText) +
      "</div></div>" +
      '<div class="' +
      bodyClass(msg) +
      '">' +
      renderBodyContent(msg) +
      "</div>" +
      (msg.role === "assistant" ? renderMessageFooter(msg) : "") +
      retryBtn +
      "</article>"
    );
  }

  function renderAll(messages) {
    if (!messages || !messages.length) {
      return '<p class="chat-empty">Send a message to start the conversation.</p>';
    }
    var html = "";
    for (var i = 0; i < messages.length; i++) {
      html += renderMessage(messages[i]);
    }
    return html;
  }

  function updateMessageBody(root, msg) {
    if (!root || !msg) return;
    var el = root.querySelector('[data-msg-id="' + msg.id + '"]');
    if (!el) return;
    var body = el.querySelector(".chat-msg__body");
    if (!body) return;

    if (msg.role === "assistant") {
      body.innerHTML = renderAssistantMarkdown(msg.content || "");
    } else {
      body.textContent = msg.content || "";
    }
    body.classList.toggle("chat-msg__body--streaming", msg.status === "streaming");
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Render = globalThis.ChimeraChat.Render || {};
  globalThis.ChimeraChat.Render.Messages = {
    renderAll: renderAll,
    renderMessage: renderMessage,
    updateMessageBody: updateMessageBody
  };
})();
