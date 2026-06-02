/**
 * Service summarized cards (gateway, broker, vectorstore, indexer) — orchestrator mount.
 * Per-service implementations live in render/cards/serviceFeed/*.js.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.mountServiceFeed = function (ctx) {
  var SF = globalThis.ChimeraSettings.Render.Cards.ServiceFeed;
  var deps = {
    ctx: ctx,
    escapeHtml: ctx.escapeHtml,
    getFlat: ctx.getFlat,
    entryCache: ctx.entryCache,
    strHash: ctx.strHash,
    entryInstant: ctx.entryInstant,
    primaryLogMessage: ctx.primaryLogMessage,
    formatInt: ctx.formatInt,
    getViewMode: ctx.getViewMode,
    sumEvlogPanelHtml: ctx.sumEvlogPanelHtml,
    sumEvlogBuildTbodyFromServiceEntries: ctx.sumEvlogBuildTbodyFromServiceEntries,
    sumEvlogCountWarnFailFromEntries: ctx.sumEvlogCountWarnFailFromEntries,
    scopedEvlogTitle: ctx.scopedEvlogTitle,
    RECENT_CARD_STATUS_N: ctx.RECENT_CARD_STATUS_N,
    CHIMERA_BROKER_PROVIDER_STALE_MS: ctx.CHIMERA_BROKER_PROVIDER_STALE_MS || 90000,
    operatorCardChevronHtml: ctx.operatorCardChevronHtml
  };

  var registry = SF.createRegistry();
  var mountFns = [
    SF.mountBroker,
    SF.mountGateway,
    SF.mountVectorstore,
    SF.mountIndexer,
    SF.mountDefault
  ];
  for (var mi = 0; mi < mountFns.length; mi++) {
    if (typeof mountFns[mi] !== "function") continue;
    var pack = mountFns[mi](deps);
    if (pack && pack.impl) {
      if (pack.impl.id) registry.register(pack.impl);
      else registry.setDefault(pack.impl);
    }
    if (pack && pack.exports) {
      var keys = Object.keys(pack.exports);
      for (var ki = 0; ki < keys.length; ki++) {
        ctx[keys[ki]] = pack.exports[keys[ki]];
      }
    }
  }

  var shell = SF.mountShell(deps, registry);

  ctx.recentServiceCardHasError = shell.recentServiceCardHasError;
  ctx.serviceWindowMs = shell.serviceWindowMs;
  ctx.badgeForServicePanel = shell.badgeForServicePanel;
  ctx.renderExpandedService = shell.renderExpandedService;
  ctx.buildServiceCard = shell.buildServiceCard;
  ctx.summarizedServicesSectionHead = shell.summarizedServicesSectionHead;
  ctx.entryIsGatewayUpstreamRelay = shell.entryIsGatewayUpstreamRelay;
  ctx.entryRoutesToChimeraBrokerBucket = shell.entryRoutesToChimeraBrokerBucket;
};
