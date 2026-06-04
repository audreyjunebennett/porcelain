/**
 * GET/PUT /api/ui/rag/embedding for the indexer service card embedding selector.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Api = globalThis.ChimeraSettings.Api || {};

globalThis.ChimeraSettings.Api.mountRagEmbeddingApi = function (ctx) {
  function fetchRagEmbedding() {
    if (ctx.uiUnauthorized) return Promise.reject(new Error("Unauthorized"));
    return fetch("/api/ui/rag/embedding", { credentials: "same-origin" }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (r.status === 401) throw new Error("Unauthorized");
        if (!r.ok) throw new Error((j && j.error) || ("HTTP " + r.status));
        ctx.ragEmbeddingCache = j;
        return j;
      });
    });
  }

  function saveRagEmbedding(model) {
    if (ctx.uiUnauthorized) return Promise.reject(new Error("Unauthorized"));
    model = String(model || "").trim();
    if (!model) return Promise.reject(new Error("model required"));
    return fetch("/api/ui/rag/embedding", {
      method: "PUT",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ model: model })
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (r.status === 401) throw new Error("Unauthorized");
        if (!r.ok) throw new Error((j && j.error) || ("HTTP " + r.status));
        if (ctx.ragEmbeddingCache) {
          ctx.ragEmbeddingCache.model = j.model != null ? j.model : model;
          ctx.ragEmbeddingCache.dim = j.dim != null ? j.dim : ctx.ragEmbeddingCache.dim;
          ctx.ragEmbeddingCache.status = "ok";
          ctx.ragEmbeddingCache.model_in_catalog = true;
        }
        return j;
      });
    });
  }

  ctx.fetchRagEmbedding = fetchRagEmbedding;
  ctx.saveRagEmbedding = saveRagEmbedding;
};
