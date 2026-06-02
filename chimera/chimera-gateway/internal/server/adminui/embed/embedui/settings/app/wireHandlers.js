/**
 * DOM event wiring for summarized cards, workspaces, admin workflows, and chrome links.
 *
 * Exports: ChimeraSettings.App.mountWireHandlers(ctx)
 */

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.App = globalThis.ChimeraSettings.App || {};
globalThis.ChimeraSettings.App.mountWireHandlers = function (ctx) {
  var H = globalThis.ChimeraSettings && globalThis.ChimeraSettings.Handlers;
  if (!H) return;
  if (typeof H.Evlog.wire === "function") H.Evlog.wire(ctx);
  if (typeof H.Chrome.wire === "function") H.Chrome.wire(ctx);
  if (typeof H.Admin.wire === "function") H.Admin.wire(ctx);
  if (typeof H.VirtualModels.wire === "function") H.VirtualModels.wire(ctx);
  if (typeof H.ProviderModels.wire === "function") H.ProviderModels.wire(ctx);
  if (typeof H.RagEmbedding.wire === "function") H.RagEmbedding.wire(ctx);
};
