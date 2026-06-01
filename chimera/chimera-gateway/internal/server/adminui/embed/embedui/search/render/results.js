/**
 * Search results rendering (reuses chat snippet cards).
 */
(function () {
  "use strict";

  var esc =
    globalThis.ChimeraUI && ChimeraUI.escapeHtml
      ? ChimeraUI.escapeHtml
      : function (s) {
          return String(s || "");
        };

  var CHEVRON_ICON =
    '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>';

  var HINT_COPY = {
    empty_collection: "No indexed content in this workspace yet — run the indexer or ingest files first.",
    no_search_index:
      "No search index yet for this workspace — files will upload on the next ingest.",
    embed_unavailable: "Embedding model is unavailable — check provider keys and catalog in Settings.",
    embed_model_not_in_catalog: "Configured embedding model is missing from the live catalog.",
    embed_provider_down: "Embedding provider is down — search cannot embed queries right now.",
    embed_provider_key_missing: "Embedding provider key is missing — add keys in Settings.",
    embed_catalog_stale: "Model catalog is stale — wait for refresh or check broker connectivity.",
    vectorstore_unreachable: "Vector store is unreachable — check Qdrant and gateway RAG config."
  };

  function formatRelevanceScore(raw) {
    if (raw == null || raw === "") return "";
    var n = Number(raw);
    if (isNaN(n)) return "";
    return String(Math.round(n * 100)) + "%";
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

  function snippetRenderer() {
    return globalThis.ChimeraChat &&
      ChimeraChat.Render &&
      ChimeraChat.Render.Snippet &&
      typeof ChimeraChat.Render.Snippet.render === "function"
      ? ChimeraChat.Render.Snippet.render
      : null;
  }

  function highlightFn() {
    return globalThis.ChimeraSearch &&
      ChimeraSearch.Render &&
      ChimeraSearch.Render.Highlight &&
      typeof ChimeraSearch.Render.Highlight.highlightPlain === "function"
      ? ChimeraSearch.Render.Highlight.highlightPlain
      : null;
  }

  function renderHitBody(source, text, query) {
    var snippetFn = snippetRenderer();
    var hl = highlightFn();
    if (snippetFn) {
      var lang =
        ChimeraChat.Render.Snippet && typeof ChimeraChat.Render.Snippet.inferLanguage === "function"
          ? ChimeraChat.Render.Snippet.inferLanguage(source, "")
          : "";
      if (lang === "markdown" || !lang) {
        if (hl) {
          return (
            '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' +
            hl(text, query) +
            "</code></pre>"
          );
        }
      }
      return snippetFn(source, text, lang);
    }
    if (hl) {
      return (
        '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' +
        hl(text, query) +
        "</code></pre>"
      );
    }
    return (
      '<pre class="chat-embed-item__snippet chat-embed-item__snippet--plain"><code>' +
      esc(text) +
      "</code></pre>"
    );
  }

  function renderHitItem(hit, query) {
    hit = hit || {};
    var src = hit.source != null ? String(hit.source) : "unknown";
    var text = hit.text_excerpt != null ? String(hit.text_excerpt) : hit.text != null ? String(hit.text) : "";
    var score = hit.score != null && !isNaN(Number(hit.score)) ? Number(hit.score) : "";
    return (
      '<li class="chat-embed-item">' +
      "<details open>" +
      '<summary class="chat-embed-item__summary">' +
      '<span class="chat-embed-item__lead">' +
      CHEVRON_ICON +
      "</span>" +
      '<span class="chat-embed-item__source">' +
      esc(src) +
      "</span>" +
      renderScoreMeta(score) +
      "</summary>" +
      renderHitBody(src, text, query) +
      "</details></li>"
    );
  }

  function hintMessage(code) {
    code = String(code || "").trim();
    if (!code) return "";
    return HINT_COPY[code] || "Indexer hint: " + code.replace(/_/g, " ");
  }

  function renderSummary(resp, query) {
    resp = resp || {};
    var hits = Array.isArray(resp.hits) ? resp.hits : [];
    var countLabel = hits.length === 1 ? "1 result" : hits.length + " results";
    if (hits.length === 0 && String(query || "").trim()) {
      countLabel = "No results";
    }
    var meta = [];
    if (resp.score_threshold != null && !isNaN(Number(resp.score_threshold))) {
      meta.push("threshold " + formatRelevanceScore(resp.score_threshold));
    }
    if (resp.top_k != null) {
      meta.push("top " + resp.top_k);
    }
    return (
      '<div class="search-summary">' +
      '<span class="search-summary__count">' +
      esc(countLabel) +
      "</span>" +
      (meta.length ? '<span class="search-summary__meta">' + esc(meta.join(" · ")) + "</span>" : "") +
      "</div>"
    );
  }

  function renderIdle() {
    return '<p class="search-idle">Select a workspace and enter a query to search indexed content.</p>';
  }

  function renderLoading() {
    return '<p class="search-idle">Searching…</p>';
  }

  function renderError(msg) {
    return '<div class="search-error" role="alert">' + esc(msg || "Search failed") + "</div>";
  }

  function renderHint(code) {
    var msg = hintMessage(code);
    if (!msg) return "";
    return '<div class="search-hint" role="status">' + esc(msg) + "</div>";
  }

  function renderResultsView(state) {
    state = state || {};
    if (state.isSearching) return renderLoading();
    if (state.lastError) return renderError(state.lastError);
    if (!state.lastResponse && !String(state.lastQuery || "").trim()) return renderIdle();

    var resp = state.lastResponse || { hits: [] };
    var query = state.lastQuery || "";
    var html = renderSummary(resp, query);
    if (resp.indexer_hint) {
      html += renderHint(resp.indexer_hint);
    }
    var hits = Array.isArray(resp.hits) ? resp.hits : [];
    if (!hits.length && String(query).trim()) {
      html += '<p class="search-empty">No matches for this query in the selected workspace.</p>';
      return html;
    }
    if (!hits.length) return html + renderIdle();

    html += '<ul class="search-hit-list">';
    for (var i = 0; i < hits.length; i++) {
      html += renderHitItem(hits[i], query);
    }
    html += "</ul>";
    return html;
  }

  globalThis.ChimeraSearch = globalThis.ChimeraSearch || {};
  globalThis.ChimeraSearch.Render = globalThis.ChimeraSearch.Render || {};
  globalThis.ChimeraSearch.Render.Results = {
    renderResultsView: renderResultsView,
    hintMessage: hintMessage,
    renderHitItem: renderHitItem
  };
})();
