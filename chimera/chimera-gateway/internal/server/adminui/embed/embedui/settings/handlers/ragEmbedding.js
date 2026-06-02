/**
 * Embedding model combobox save/cancel on the chimera-indexer service card.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};
globalThis.ChimeraSettings.Handlers.RagEmbedding = globalThis.ChimeraSettings.Handlers.RagEmbedding || {};

globalThis.ChimeraSettings.Handlers.RagEmbedding.wire = function (ctx) {
  var adminSetMessage = ctx.adminSetMessage;
  var refreshSummarizedPanel = ctx.refreshSummarizedPanel;
  var AA = globalThis.ChimeraShared && globalThis.ChimeraShared.AdminAction;
  var EMS = globalThis.ChimeraShared && globalThis.ChimeraShared.EmbeddingModelSelector;

  if (globalThis.__ChimeraSettingsRagEmbeddingWired) return;
  globalThis.__ChimeraSettingsRagEmbeddingWired = true;

  function runJson(opts) {
    if (AA && typeof AA.runJson === "function") return AA.runJson(opts);
    return opts.request().then(opts.onSuccess).catch(function (e) {
      if (typeof opts.setMessage === "function") {
        opts.setMessage("err", e && e.message ? e.message : String(e));
      }
    });
  }

  function currentSelectValue() {
    var sel = document.getElementById("rag-embedding-model-select");
    return sel ? String(sel.value || "").trim() : "";
  }

  function clearDraftAndSync() {
    ctx.ragEmbeddingDraftModel = null;
    if (typeof ctx.syncRagEmbeddingDom === "function") ctx.syncRagEmbeddingDom();
  }

  document.body.addEventListener("change", function (ev) {
    var t = ev.target;
    if (!t || !t.getAttribute || t.getAttribute("data-rag-embedding-select") !== "1") return;
    var model = String(t.value || "").trim();
    var saved =
      ctx.ragEmbeddingCache && ctx.ragEmbeddingCache.model
        ? String(ctx.ragEmbeddingCache.model).trim()
        : "";
    ctx.ragEmbeddingDraftModel = model === saved ? null : model;
    ctx.ragEmbeddingPostSaveBanner = false;
    if (typeof ctx.syncRagEmbeddingDom === "function") ctx.syncRagEmbeddingDom();
  });

  document.body.addEventListener("click", function (ev) {
    var t = ev.target;
    if (!t || typeof t.closest !== "function") return;

    if (t.closest('[data-rag-embedding-goto-workspaces]')) {
      ev.preventDefault();
      ev.stopPropagation();
      var sectionId =
        EMS && EMS.WORKSPACES_SECTION_ID ? String(EMS.WORKSPACES_SECTION_ID) : "sum-feed-workspaces";
      var section = document.getElementById(sectionId);
      if (!section) section = document.querySelector(".sum-feed-section--workspaces");
      if (section && typeof section.scrollIntoView === "function") {
        section.scrollIntoView({ behavior: "smooth", block: "start" });
      }
      return;
    }

    if (t.closest('[data-admin-action="rag-embedding-cancel"]')) {
      ev.preventDefault();
      ev.stopPropagation();
      ctx.ragEmbeddingPostSaveBanner = false;
      clearDraftAndSync();
      return;
    }

    if (t.closest('[data-admin-action="rag-embedding-save"]')) {
      ev.preventDefault();
      ev.stopPropagation();
      var model = currentSelectValue();
      if (!model) {
        if (typeof adminSetMessage === "function") adminSetMessage("err", "Select an embedding model");
        return;
      }
      var confirmMsg =
        EMS && typeof EMS.saveConfirmMessage === "function"
          ? EMS.saveConfirmMessage(model)
          : "Save embedding model " + model + "?";
      if (!window.confirm(confirmMsg)) return;

      var btn = t.closest('[data-admin-action="rag-embedding-save"]');
      ctx.ragEmbeddingSaving = true;
      if (typeof ctx.syncRagEmbeddingDom === "function") ctx.syncRagEmbeddingDom();

      runJson({
        request: function () {
          if (typeof ctx.saveRagEmbedding !== "function") {
            return Promise.reject(new Error("save unavailable"));
          }
          return ctx.saveRagEmbedding(model);
        },
        setMessage: adminSetMessage,
        triggerBtn: btn,
        setPending: function (el, on) {
          if (el) el.disabled = !!on;
        },
        successKind: "",
        successMsg: "",
        onSuccess: function () {
          ctx.ragEmbeddingSaving = false;
          ctx.ragEmbeddingDraftModel = null;
          ctx.ragEmbeddingPostSaveBanner = true;
          if (typeof ctx.syncRagEmbeddingDom === "function") ctx.syncRagEmbeddingDom();
          if (typeof ctx.hydrateIndexerServiceSummaryFromApi === "function") {
            ctx.hydrateIndexerServiceSummaryFromApi(true);
          }
          if (typeof refreshSummarizedPanel === "function") refreshSummarizedPanel();
        },
        onError: function () {
          ctx.ragEmbeddingSaving = false;
          if (typeof ctx.syncRagEmbeddingDom === "function") ctx.syncRagEmbeddingDom();
        }
      });
    }
  });
};
