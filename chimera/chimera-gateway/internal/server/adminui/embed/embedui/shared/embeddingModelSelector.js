/**
 * Reusable embedding model combobox + re-embed warning (settings indexer card + setup wizard).
 */
(function () {
  "use strict";

  var REEMBED_WARN_SUFFIX =
    " invalidates all existing vectors across every workspace. " +
    "Search and chat retrieval will stop working correctly until each workspace is fully re\u2011indexed.";

  var POST_SAVE_BANNER_TEXT =
    "Embedding model changed \u2014 schedule re-index for affected workspaces.";

  var WORKSPACES_SECTION_ID = "sum-feed-workspaces";

  var SAVE_CONFIRM_PREFIX =
    "Changing the embedding model invalidates all existing vectors indexed with the previous model. " +
    "You must re-index every workspace after saving.\n\nSave embedding model ";

  function statusMeta(status) {
    var s = String(status || "ok").trim().toLowerCase();
    if (s === "ok") return { variant: "ok", label: "catalog ok" };
    if (s === "embed_model_not_in_catalog") return { variant: "warn", label: "not in catalog" };
    if (s === "embed_catalog_stale") return { variant: "unknown", label: "catalog stale" };
    return { variant: "unknown", label: s || "unknown" };
  }

  function selectedModel(savedModel, draftModel) {
    if (draftModel != null && String(draftModel).trim() !== "") return String(draftModel).trim();
    return String(savedModel || "").trim();
  }

  function draftDiffersFromSaved(savedModel, draftModel) {
    var saved = String(savedModel || "").trim();
    var draft = draftModel != null ? String(draftModel).trim() : saved;
    return draft !== "" && draft !== saved;
  }

  function selectOptionsHtml(escapeHtml, candidates, selected) {
    candidates = Array.isArray(candidates) ? candidates : [];
    selected = String(selected || "").trim();
    var parts = ['<option value="">Select a model…</option>'];
    var seen = {};
    for (var i = 0; i < candidates.length; i++) {
      var c = candidates[i] || {};
      var id = String(c.id || c.ID || "").trim();
      if (!id || seen[id]) continue;
      seen[id] = true;
      var label = id;
      if (c.embedding_likely || c.EmbeddingLikely) {
        label += " · embedding";
      }
      if (c.known_dim > 0 || c.KnownDim > 0) {
        label += " · dim " + String(c.known_dim || c.KnownDim);
      }
      parts.push(
        '<option value="' +
          escapeHtml(id) +
          '"' +
          (id === selected ? " selected" : "") +
          ">" +
          escapeHtml(label) +
          "</option>"
      );
    }
    if (selected && !seen[selected]) {
      parts.push(
        '<option value="' + escapeHtml(selected) + '" selected>' + escapeHtml(selected) + " · saved</option>"
      );
    }
    return parts.join("");
  }

  function savedModelPillHtml(pillFn, savedModel) {
    if (!pillFn) return "";
    return pillFn(String(savedModel || "").trim() || "—", "metric", {
      title: "Saved embedding model id",
      icon: "hub"
    });
  }

  function warningCalloutHtml(escapeHtml, visible, savedModel, pillFn) {
    var modelPill = savedModelPillHtml(pillFn, savedModel);
    var pillInline = modelPill || escapeHtml(String(savedModel || "").trim() || "—");
    return (
      '<div class="sg-op-rag-embedding-warn callout callout--warn' +
      (visible ? "" : " sg-op-rag-embedding-warn--hidden") +
      '" id="rag-embedding-warn" role="alert"' +
      (visible ? "" : ' hidden') +
      ">" +
      '<div class="sg-op-rag-embedding-warn__row">' +
      '<span class="material-symbols-outlined sg-op-rag-embedding-warn__icon" aria-hidden="true">warning</span>' +
      '<p class="sg-op-rag-embedding-warn__text">Changing the indexing model from ' +
      pillInline +
      REEMBED_WARN_SUFFIX +
      "</p></div></div>"
    );
  }

  function postSaveBannerHtml(escapeHtml, visible) {
    if (!visible) {
      return (
        '<div class="sg-op-rag-embedding-banner callout callout--info sg-op-rag-embedding-banner--hidden" id="rag-embedding-banner" hidden></div>'
      );
    }
    return (
      '<div class="sg-op-rag-embedding-banner callout callout--info" id="rag-embedding-banner" role="status">' +
      '<div class="sg-op-rag-embedding-banner__row">' +
      '<span class="material-symbols-outlined sg-op-rag-embedding-banner__icon" aria-hidden="true">info</span>' +
      '<p class="sg-op-rag-embedding-banner__text">' +
      escapeHtml(POST_SAVE_BANNER_TEXT) +
      ' <a href="#' +
      WORKSPACES_SECTION_ID +
      '" class="sg-op-rag-embedding-banner__link" data-rag-embedding-goto-workspaces="1">View workspaces</a>' +
      " to re-index each one.</p></div></div>"
    );
  }

  function panelHtml(escapeHtml, opts) {
    opts = opts || {};
    var savedModel = String(opts.savedModel || "").trim();
    var draftModel = opts.draftModel;
    var sel = selectedModel(savedModel, draftModel);
    var dim = opts.dim != null && !isNaN(Number(opts.dim)) ? Number(opts.dim) : 0;
    var st = statusMeta(opts.status);
    var warnVisible = draftDiffersFromSaved(savedModel, draftModel);
    var saving = !!opts.saving;
    var pillFn = typeof opts.sgOpHealthPillHtml === "function" ? opts.sgOpHealthPillHtml : null;
    var modelPill = savedModelPillHtml(pillFn, savedModel);
    var dimPill =
      dim > 0 && pillFn
        ? pillFn("dim " + String(dim), "metric", { title: "Vector dimension", icon: "straighten" })
        : "";
    var healthPill = pillFn ? pillFn(st.label, st.variant, { title: "Embedding catalog health" }) : "";
    var toolbar = "";
    if (typeof opts.toolbarHtml === "string") toolbar = opts.toolbarHtml;

    return (
      '<section class="sg-op-provider-panel sg-op-rag-embedding-panel" data-ui-part="indexer-rag.embedding" id="rag-embedding-panel">' +
      '<header class="sg-op-provider-panel__head">' +
      '<h4 class="sg-op-provider-panel__title sum-section-label">Indexing model</h4>' +
      '<div class="sg-op-provider-panel__actions">' +
      toolbar +
      "</div>" +
      "</header>" +
      '<div class="sg-op-provider-panel__body">' +
      '<p class="muted sg-op-rag-embedding-lead">Model used to convert files into indexable vectors for search and retrieval. All workspaces share this global setting.</p>' +
      '<label class="sg-op-rag-embedding-field" for="rag-embedding-model-select">' +
      '<select id="rag-embedding-model-select" class="sg-op-rag-embedding-select" data-rag-embedding-select="1"' +
      (saving ? " disabled" : "") +
      ">" +
      selectOptionsHtml(escapeHtml, opts.candidates, sel) +
      "</select></label>" +
      '<div class="sg-op-rag-embedding-pills">' +
      // modelPill +
      // dimPill +
      // healthPill +
      "</div>" +
      warningCalloutHtml(escapeHtml, warnVisible, savedModel, pillFn) +
      postSaveBannerHtml(escapeHtml, !!opts.postSaveBanner) +
      "</div></section>"
    );
  }

  function saveConfirmMessage(modelId) {
    return SAVE_CONFIRM_PREFIX + JSON.stringify(String(modelId || "").trim()) + "?";
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.EmbeddingModelSelector = {
    REEMBED_WARN_SUFFIX: REEMBED_WARN_SUFFIX,
    POST_SAVE_BANNER_TEXT: POST_SAVE_BANNER_TEXT,
    WORKSPACES_SECTION_ID: WORKSPACES_SECTION_ID,
    statusMeta: statusMeta,
    selectedModel: selectedModel,
    draftDiffersFromSaved: draftDiffersFromSaved,
    selectOptionsHtml: selectOptionsHtml,
    warningCalloutHtml: warningCalloutHtml,
    postSaveBannerHtml: postSaveBannerHtml,
    panelHtml: panelHtml,
    saveConfirmMessage: saveConfirmMessage
  };
})();
