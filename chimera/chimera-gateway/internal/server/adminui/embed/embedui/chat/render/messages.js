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

  var COPY_ICON =
    '<span class="material-symbols-outlined" aria-hidden="true">content_copy</span>';

  var CHEVRON_ICON =
    '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>';

  function formatRelevanceScore(raw) {
    if (raw == null || raw === "") return "";
    var n = Number(raw);
    if (isNaN(n)) return "";
    return String(Math.round(n * 100)) + "%";
  }

  function md() {
    return globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Markdown &&
      typeof ChimeraChat.Render.Markdown.render === "function"
      ? ChimeraChat.Render.Markdown
      : null;
  }

  function renderCopyButton(copyText) {
    return (
      '<button type="button" class="chat-msg__copy-btn" data-copy-text="' +
      esc(copyText) +
      '" title="Copy message" aria-label="Copy message">' +
      COPY_ICON +
      "</button>"
    );
  }

  function renderUserCopyFooter(copyText) {
    return (
      '<div class="chat-msg__bar-footer chat-msg__copy-footer">' +
      renderCopyButton(copyText) +
      "</div>"
    );
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
    var label = formatRelevanceScore(score);
    if (!label) return "";
    return (
      '<span class="chat-embed-item__meta" title="Retrieval confidence score">' +
      '<span class="chat-embed-item__score">' +
      esc(label) +
      "</span>" +
      '<span class="material-symbols-outlined material-symbols-outlined--sm chat-embed-item__score-icon" aria-hidden="true">readiness_score</span>' +
      "</span>"
    );
  }

  function snippetLineRangeLabel(h) {
    var Sn = globalThis.ChimeraChat && ChimeraChat.Render && ChimeraChat.Render.Snippet;
    if (Sn && typeof Sn.formatLineRangeLabel === "function") {
      return Sn.formatLineRangeLabel(h.start_line, h.end_line);
    }
    return "";
  }

  function ragHitIsStale(h) {
    var ctx = globalThis.ChimeraChat && ChimeraChat._ragStaleCtx;
    if (!ctx || ctx.mode === "off") return false;
    var src = h.source != null ? String(h.source) : "";
    var idx = h.content_sha256 != null ? String(h.content_sha256).trim() : "";
    if (!src || !idx || !ctx.map) return false;
    var ent = ctx.map[src];
    return !!(ent && String(ent.indexed_sha256 || "").trim() === idx);
  }

  function renderStaleBadge(h) {
    if (!ragHitIsStale(h)) return "";
    return (
      '<span class="chat-embed-item__stale" title="File changed on disk since this snippet was indexed">Stale</span>'
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
      var score = h.score != null && !isNaN(Number(h.score)) ? Number(h.score) : "";
      var lineMeta = {
        start_line: h.start_line,
        end_line: h.end_line,
        starts_mid_line: h.starts_mid_line
      };
      var lineLbl = snippetLineRangeLabel(h);
      var srcLabel = lineLbl ? esc(src) + " \u00b7 " + esc(lineLbl) : esc(src);
      var body = snippetFn
        ? snippetFn(src, text, langHint, lineMeta)
        : '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' + esc(text) + "</code></pre>";
      items +=
        '<li class="chat-embed-item">' +
        "<details>" +
        '<summary class="chat-embed-item__summary">' +
        '<span class="chat-embed-item__lead">' +
        CHEVRON_ICON +
        "</span>" +
        '<span class="chat-embed-item__source">' +
        srcLabel +
        "</span>" +
        renderStaleBadge(h) +
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
      '<div class="chat-msg__bar-footer chat-msg__snippets-footer">' +
      '<button type="button" class="chat-msg__snippets-toggle" aria-expanded="false" aria-controls="' +
      panelId +
      '">' +
      '<span class="chat-msg__snippets-toggle__lead">' +
      CHEVRON_ICON +
      "</span>" +
      '<span class="chat-msg__snippets-toggle__label">Workspace Snippets (' +
      msg.ragHits.length +
      ")</span></button>" +
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

    var headActions = "";
    if (msg.role !== "user") {
      headActions = '<div class="chat-msg__actions">' + renderCopyButton(copyText) + "</div>";
    }

    var userCopyFooter = msg.role === "user" ? renderUserCopyFooter(copyText) : "";

    return (
      '<article class="' +
      cls +
      '" data-msg-id="' +
      esc(msg.id) +
      '">' +
      '<div class="chat-msg__head">' +
      renderRoleHead(msg) +
      headActions +
      "</div>" +
      '<div class="' +
      bodyClass(msg) +
      '">' +
      renderBodyContent(msg) +
      "</div>" +
      (msg.role === "assistant" ? renderMessageFooter(msg) : "") +
      userCopyFooter +
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
