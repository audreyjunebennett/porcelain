/**
 * Service card descriptor registry (chimera-broker, gateway, vectorstore, indexer, default).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.createRegistry = function () {
  var byId = Object.create(null);
  var defaultImpl = null;
  return {
    register: function (impl) {
      if (!impl || !impl.id) return;
      byId[impl.id] = impl;
    },
    setDefault: function (impl) {
      defaultImpl = impl;
    },
    get: function (name) {
      if (byId[name]) return byId[name];
      return defaultImpl;
    }
  };
};
