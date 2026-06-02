/**
 * Indexer / RAG service card: global embedding model combobox (Phase 2).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountRagEmbedding = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;
  var EMS = globalThis.ChimeraShared && globalThis.ChimeraShared.EmbeddingModelSelector;
  var ET = globalThis.ChimeraShared && globalThis.ChimeraShared.EditToolbar;

  function embeddingToolbarHtml(saving) {
    var draftDiff =
      EMS && typeof EMS.draftDiffersFromSaved === "function"
        ? EMS.draftDiffersFromSaved(
            ctx.ragEmbeddingCache && ctx.ragEmbeddingCache.model,
            ctx.ragEmbeddingDraftModel
          )
        : false;
    if (!ET || typeof ET.iconBtnHtml !== "function") return "";
    var parts = "";
    if (draftDiff) {
      parts += ET.iconBtnHtml(escapeHtml, {
        action: "rag-embedding-save",
        title: "Save embedding model",
        icon: "keep",
        disabled: saving
      });
      parts += ET.iconBtnHtml(escapeHtml, {
        action: "rag-embedding-cancel",
        title: "Revert selection",
        icon: "refresh",
        disabled: saving
      });
    }
    return parts;
  }

  function ragEmbeddingPanelHtml() {
    if (!EMS || typeof EMS.panelHtml !== "function") return "";
    var cache = ctx.ragEmbeddingCache || {};
    return EMS.panelHtml(escapeHtml, {
      savedModel: cache.model,
      draftModel: ctx.ragEmbeddingDraftModel,
      dim: cache.dim,
      status: cache.status,
      candidates: cache.candidates,
      saving: !!ctx.ragEmbeddingSaving,
      postSaveBanner: ctx.ragEmbeddingPostSaveBanner,
      sgOpHealthPillHtml: sgOpHealthPillHtml,
      toolbarHtml: embeddingToolbarHtml(!!ctx.ragEmbeddingSaving)
    });
  }

  function syncRagEmbeddingDom() {
    var panel = document.getElementById("rag-embedding-panel");
    if (!panel) return;
    var html = ragEmbeddingPanelHtml();
    if (!html) return;
    var wrap = document.createElement("div");
    wrap.innerHTML = html;
    var next = wrap.firstElementChild;
    if (next && panel.parentNode) panel.parentNode.replaceChild(next, panel);
  }

  var ragEmbeddingFetchTimer = null;
  function scheduleRagEmbeddingFetch(force) {
    if (ctx.ragEmbeddingFetchInFlight) {
      ctx.ragEmbeddingFetchWanted = true;
      return;
    }
    if (!force && ctx.ragEmbeddingHydratedOnce) return;
    if (ragEmbeddingFetchTimer) return;
    ragEmbeddingFetchTimer = window.setTimeout(function () {
      ragEmbeddingFetchTimer = null;
      hydrateRagEmbeddingFromApi(!!force);
    }, force ? 0 : 200);
  }

  function hydrateRagEmbeddingFromApi(force) {
    if (typeof ctx.fetchRagEmbedding !== "function") return Promise.resolve();
    if (ctx.ragEmbeddingFetchInFlight) {
      ctx.ragEmbeddingFetchWanted = true;
      return Promise.resolve();
    }
    ctx.ragEmbeddingFetchInFlight = true;
    return ctx
      .fetchRagEmbedding()
      .then(function () {
        ctx.ragEmbeddingHydratedOnce = true;
        ctx.ragEmbeddingDraftModel = null;
        syncRagEmbeddingDom();
      })
      .catch(function () {
        if (force) syncRagEmbeddingDom();
      })
      .finally(function () {
        ctx.ragEmbeddingFetchInFlight = false;
        if (ctx.ragEmbeddingFetchWanted) {
          ctx.ragEmbeddingFetchWanted = false;
          return hydrateRagEmbeddingFromApi(true);
        }
      });
  }

  ctx.ragEmbeddingPanelHtml = ragEmbeddingPanelHtml;
  ctx.syncRagEmbeddingDom = syncRagEmbeddingDom;
  ctx.scheduleRagEmbeddingFetch = scheduleRagEmbeddingFetch;
  ctx.hydrateRagEmbeddingFromApi = hydrateRagEmbeddingFromApi;
};
