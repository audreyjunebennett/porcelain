/**
 * Mount admin/gateway card modules on ctx (settings feed, gallery, setup wizard).
 * Log-feed card families mount via mountSummarizedFeedCards after mountAll.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAll = function (ctx) {
  var C = globalThis.ChimeraSettings.Render.Cards;
  if (typeof C.mountSharedFormat === "function") C.mountSharedFormat(ctx);
  if (typeof C.mountAdminShared === "function") C.mountAdminShared(ctx);
  if (typeof C.mountConvCard === "function") C.mountConvCard(ctx);
  if (typeof C.mountServiceCard === "function") C.mountServiceCard(ctx);
  if (typeof C.mountGatewayUsage === "function") C.mountGatewayUsage(ctx);
  if (typeof C.mountGatewayOverview === "function") C.mountGatewayOverview(ctx);
  if (typeof C.mountAdminUsers === "function") C.mountAdminUsers(ctx);
  if (typeof C.mountAdminProvider === "function") C.mountAdminProvider(ctx);
  if (typeof C.mountAdminVirtualModels === "function") C.mountAdminVirtualModels(ctx);
  if (typeof C.mountWorkspaceDraft === "function") C.mountWorkspaceDraft(ctx);
};

/** Summarized log-feed cards: chrome → indexer → conv → service (single mount path). */
globalThis.ChimeraSettings.Render.Cards.mountSummarizedFeedCards = function (ctx) {
  var C = globalThis.ChimeraSettings.Render.Cards;
  if (typeof C.mountCardChrome === "function") C.mountCardChrome(ctx);
  if (typeof C.mountFeedLogIndexerRun === "function") C.mountFeedLogIndexerRun(ctx);
  if (typeof C.mountFeedLogIndexerWorkspace === "function") C.mountFeedLogIndexerWorkspace(ctx);
  if (typeof C.mountFeedLogConv === "function") C.mountFeedLogConv(ctx);
  if (typeof C.mountServiceFeed === "function") C.mountServiceFeed(ctx);
};
