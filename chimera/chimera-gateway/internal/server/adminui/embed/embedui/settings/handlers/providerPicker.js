/**
 * Admin provider catalog bootstrap + add-provider picker DOM.
 * Exports: ChimeraSettings.Handlers.ProviderPicker.mount(ctx, bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};

globalThis.ChimeraSettings.Handlers.ProviderPicker = globalThis.ChimeraSettings.Handlers.ProviderPicker || {};

globalThis.ChimeraSettings.Handlers.ProviderPicker.mount = function (ctx, bridge) {
  bridge = bridge || {};

  function providerCatalogApi() {
    return globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog
      ? ChimeraSettings.Providers.Catalog
      : null;
  }

  function ensureAdminProviderCatalog() {
    var api = providerCatalogApi();
    if (!api) return Promise.resolve(null);
    return api.fetchProviderCatalog(ctx).then(function (data) {
      var getViewMode = bridge.getViewMode || ctx.getViewMode;
      if (data && typeof getViewMode === "function" && getViewMode() === "summarized") {
        if (typeof bridge.scheduleStoryRebuild === "function") bridge.scheduleStoryRebuild();
      }
      return data;
    }).catch(function () {
      return null;
    });
  }

  function closeAdminProviderPicker() {
    var picker = document.getElementById("sg-op-provider-picker");
    var trigger = document.getElementById("sg-op-provider-picker-trigger");
    if (picker) picker.hidden = true;
    if (trigger) trigger.setAttribute("aria-expanded", "false");
  }

  function setAdminProviderPickerOpen(open) {
    var picker = document.getElementById("sg-op-provider-picker");
    var trigger = document.getElementById("sg-op-provider-picker-trigger");
    if (!picker || !trigger || trigger.disabled || trigger.getAttribute("aria-disabled") === "true") return;
    picker.hidden = !open;
    trigger.setAttribute("aria-expanded", open ? "true" : "false");
  }

  ctx.ensureAdminProviderCatalog = ensureAdminProviderCatalog;
  ctx.closeAdminProviderPicker = closeAdminProviderPicker;
  ctx.setAdminProviderPickerOpen = setAdminProviderPickerOpen;

  if (bridge.bootstrapOnMount !== false) {
    ensureAdminProviderCatalog();
  }

  return {
    ensureAdminProviderCatalog: ensureAdminProviderCatalog,
    closeAdminProviderPicker: closeAdminProviderPicker,
    setAdminProviderPickerOpen: setAdminProviderPickerOpen
  };
};
