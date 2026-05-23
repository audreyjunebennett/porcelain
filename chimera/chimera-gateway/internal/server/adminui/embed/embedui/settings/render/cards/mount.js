/**
 * Mount all summarized-feed card render modules on ctx (call from summarizedFeed.js).
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
  if (typeof C.mountGatewayOverview === "function") C.mountGatewayOverview(ctx);
  if (typeof C.mountGatewayUsage === "function") C.mountGatewayUsage(ctx);
  if (typeof C.mountAdminUsers === "function") C.mountAdminUsers(ctx);
  if (typeof C.mountAdminProvider === "function") C.mountAdminProvider(ctx);
  if (typeof C.mountAdminRouting === "function") C.mountAdminRouting(ctx);
  if (typeof C.mountAdminFallback === "function") C.mountAdminFallback(ctx);
  if (typeof C.mountAdminRouterModels === "function") C.mountAdminRouterModels(ctx);
  if (typeof C.mountAdminWorkflows === "function") C.mountAdminWorkflows(ctx);
  if (typeof C.mountWorkspaceDraft === "function") C.mountWorkspaceDraft(ctx);
};
