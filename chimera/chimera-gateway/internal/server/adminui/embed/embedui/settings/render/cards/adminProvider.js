/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminProvider = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var formatInt = ctx.formatInt;
  var adminProviderModelCount = ctx.adminProviderModelCount;
  var adminProviderAvailabilityHtml = ctx.adminProviderAvailabilityHtml;
  var adminProviderIntro = ctx.adminProviderIntro;
  var adminProviderModelEditRows = ctx.adminProviderModelEditRows;
  var adminProviderSupportsFreeTierAssist = ctx.adminProviderSupportsFreeTierAssist;
  var providerRowsHtml = ctx.providerRowsHtml;
  var providerKeyAddBlockHtml = ctx.providerKeyAddBlockHtml;
  var adminProviderAvatarClass = ctx.adminProviderAvatarClass;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;
  var adminScopedEvlogPanelFromEvents = ctx.adminScopedEvlogPanelFromEvents;
  var adminProviderEventMatches = ctx.adminProviderEventMatches;

  function providerIsOllama(providerId) {
    if (providerId === "ollama") return true;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog &&
      typeof ChimeraSettings.Providers.Catalog.lookupProviderSpec === "function"
    ) {
      var spec = ChimeraSettings.Providers.Catalog.lookupProviderSpec(providerId);
      return !!(spec && spec.kind === "ollama");
    }
    return false;
  }

  /** True when the provider has saved or in-flight credentials (keys or Ollama base URL). */
  function providerHasCredentials(providerId, row) {
    row = row || {};
    if (providerIsOllama(providerId)) {
      var url =
        ctx.adminOllamaUrlDraft != null
          ? String(ctx.adminOllamaUrlDraft).trim()
          : String(row.ollama_base_url || "").trim();
      return url !== "";
    }
    if (row.key_configured === true) return true;
    var keys = Array.isArray(row.keys) ? row.keys : [];
    if (keys.length > 0) return true;
    for (var ki = 0; ki < keys.length; ki++) {
      var kr = keys[ki] || {};
      if (kr.key_configured === true) return true;
    }
    return false;
  }

  function providerModelsEditing(providerId) {
    return ctx.adminProviderModelsEditingId === providerId;
  }

  var ET = globalThis.ChimeraShared && globalThis.ChimeraShared.EditToolbar;

  function providerModelsIconBtn(action, providerId, title, iconName, opts) {
    opts = opts || {};
    if (ET && typeof ET.iconBtnHtml === "function") {
      return ET.iconBtnHtml(escapeHtml, {
        action: action,
        title: title,
        icon: iconName,
        extraClass: opts.extraClass,
        disabled: opts.disabled,
        dataAttrs: { provider: providerId }
      });
    }
    return "";
  }

  function buildProviderModelsToolbar(providerId, editing, saving) {
    var actions = "";
    if (editing) {
      if (adminProviderSupportsFreeTierAssist(providerId)) {
        actions += providerModelsIconBtn(
          "provider-models-apply-free-tier",
          providerId,
          "Apply free-tier defaults",
          "redeem"
        );
      }
      actions += providerModelsIconBtn("provider-models-save", providerId, "Keep model availability", "keep", {
        disabled: saving
      });
      actions += providerModelsIconBtn(
        "provider-models-refresh",
        providerId,
        "Revert to last saved availability",
        "refresh"
      );
      actions += providerModelsIconBtn("provider-models-cancel", providerId, "Cancel", "cancel");
    } else {
      actions += providerModelsIconBtn(
        "provider-models-configure",
        providerId,
        "Configure model availability",
        "settings"
      );
    }
    return actions;
  }

  function buildProviderPanel(title, headerActionsHtml, bodyHtml, panelKind, uiPart) {
    var partAttr = uiPart ? ' data-ui-part="' + escapeHtml(String(uiPart)) + '"' : "";
    return (
      '<section class="sg-op-provider-panel sg-op-provider-panel--' +
      escapeHtml(String(panelKind || "block")) +
      '"' +
      partAttr +
      ">" +
      '<header class="sg-op-provider-panel__head">' +
      '<h4 class="sg-op-provider-panel__title sum-section-label">' +
      escapeHtml(title) +
      "</h4>" +
      (headerActionsHtml ? '<div class="sg-op-provider-panel__actions">' + headerActionsHtml + "</div>" : "") +
      "</header>" +
      '<div class="sg-op-provider-panel__body">' +
      bodyHtml +
      "</div></section>"
    );
  }

  function buildProviderModelRowHtml(providerId, ur, editing) {
    var rowCls = "sg-op-provider-model-item";
    if (!ur.available) rowCls += " sg-op-provider-model-row--unavailable";
    var errCls = (Number(ur.errors) || 0) > 0 ? " sg-op-provider-model-stat--errors" : "";
    return (
      '<li class="' +
      escapeHtml(rowCls) +
      '" data-model-id="' +
      escapeHtml(ur.model_id) +
      '">' +
      '<div class="sg-op-provider-model-item__main">' +
      '<code class="sum-mono-id sg-op-provider-model-item__id">' +
      escapeHtml(ur.model_id) +
      "</code>" +
      '<div class="sg-op-provider-model-item__stats">' +
      '<span class="sg-op-provider-model-stat">' +
      escapeHtml(formatInt(ur.calls)) +
      ' <span class="sg-op-provider-model-stat__label">Requests</span></span>' +
      '<span class="sg-op-provider-model-stat' +
      errCls +
      '">' +
      escapeHtml(formatInt(ur.errors)) +
      ' <span class="sg-op-provider-model-stat__label">Errors</span></span>' +
      "</div></div>" +
      '<label class="sg-op-provider-model-toggle' +
      (editing ? "" : " sg-op-provider-model-toggle--readonly") +
      '">' +
      '<input type="checkbox"' +
      (editing ? ' data-admin-provider-model-toggle="1"' : "") +
      (editing ? "" : " disabled") +
      ' data-provider="' +
      escapeHtml(providerId) +
      '" data-model-id="' +
      escapeHtml(ur.model_id) +
      '"' +
      (ur.available ? " checked" : "") +
      ' aria-label="Available: ' +
      escapeHtml(ur.model_id) +
      '"/></label></li>'
    );
  }

  function buildProviderUsageListHtml(providerId, editing) {
    var rows = adminProviderModelEditRows(providerId);
    if (!rows.length) {
      return '<p class="muted sg-op-provider-model-list-empty">No usage yet in loaded metrics window.</p>';
    }
    var editingList = editing;
    var showUnavailable =
      editingList ||
      (ctx.adminProviderModelsShowUnavailable && !!ctx.adminProviderModelsShowUnavailable[providerId]);
    var availableRows = [];
    var unavailableRows = [];
    for (var ri = 0; ri < rows.length; ri++) {
      if (rows[ri].available) availableRows.push(rows[ri]);
      else unavailableRows.push(rows[ri]);
    }
    var displayRows = availableRows.slice();
    if (showUnavailable) {
      displayRows = displayRows.concat(unavailableRows);
    }
    if (!displayRows.length) {
      return '<p class="muted sg-op-provider-model-list-empty">No available models in catalog.</p>';
    }
    var list = '<ul class="sg-op-provider-model-list sg-op-provider-models-table">';
    for (var ui = 0; ui < displayRows.length; ui++) {
      list += buildProviderModelRowHtml(providerId, displayRows[ui], editing);
    }
    list += "</ul>";
    var hiddenUnavailable = unavailableRows.length;
    if (!editingList && !showUnavailable && hiddenUnavailable > 0) {
      var noun = hiddenUnavailable === 1 ? "model" : "models";
      list +=
        '<button type="button" class="sg-op-provider-models-toggle-unavailable" data-admin-action="provider-models-show-unavailable" data-provider="' +
        escapeHtml(providerId) +
        '">Show ' +
        escapeHtml(formatInt(hiddenUnavailable)) +
        " unavailable " +
        noun +
        "</button>";
    } else if (!editingList && showUnavailable && hiddenUnavailable > 0) {
      list +=
        '<button type="button" class="sg-op-provider-models-toggle-unavailable sg-op-provider-models-toggle-unavailable--less" data-admin-action="provider-models-hide-unavailable" data-provider="' +
        escapeHtml(providerId) +
        '">Hide unavailable models</button>';
    }
    return list;
  }

  function buildProviderUsagePanel(providerId, editing, saving) {
    if (!providerHasCredentials(providerId, ((ctx.adminStateCache || {}).providers || {})[providerId])) {
      return "";
    }
    return (
      '<section class="sg-op-provider-panel sg-op-provider-panel--usage">' +
      '<header class="sg-op-provider-panel__head">' +
      '<h4 class="sg-op-provider-panel__title sum-section-label">Model usage (24h)</h4>' +
      '<div class="sg-op-provider-panel__actions" data-ui-part="admin-provider.models-toolbar">' +
      buildProviderModelsToolbar(providerId, editing, saving) +
      "</div></header>" +
      '<div class="sg-op-provider-panel__body" data-ui-part="admin-provider.models-list">' +
      buildProviderUsageListHtml(providerId, editing) +
      "</div></section>"
    );
  }

  function buildAdminProviderCardHtml(providerId, title, avatar, subtitle) {
    var st = ctx.adminStateCache || {};
    var p = st.providers || {};
    var row = p[providerId] || {};
    var keys = row && Array.isArray(row.keys) ? row.keys : [];
    var keyCount = keys.length;
    var modelCount = adminProviderModelCount(providerId);
    var isOllama = providerIsOllama(providerId);
    var hasCredentials = providerHasCredentials(providerId, row);
    var editing = providerModelsEditing(providerId);
    var draftState = (ctx.adminProviderModelsDraft && ctx.adminProviderModelsDraft[providerId]) || {};
    var saving = !!draftState.saving;
    var metrics = "";
    if (isOllama) {
      metrics =
        '<span class="sum-metrics">' +
        sgOpHealthPillHtml(formatInt(modelCount), "metric", { icon: "network_intelligence", title: "Models" }) +
        (editing ? sgOpHealthPillHtml("editing", "warn") : "") +
        adminProviderAvailabilityHtml(providerId) +
        "</span>";
    } else {
      metrics =
        '<span class="sum-metrics">' +
        sgOpHealthPillHtml(formatInt(keyCount), "metric", { icon: "key", title: "Keys" }) +
        sgOpHealthPillHtml(formatInt(modelCount), "metric", { icon: "network_intelligence", title: "Models" }) +
        (editing ? sgOpHealthPillHtml("editing", "warn") : "") +
        adminProviderAvailabilityHtml(providerId) +
        "</span>";
    }
    var providerIntro = adminProviderIntro(providerId, subtitle);
    var usagePanel = buildProviderUsagePanel(providerId, editing, saving);

    var ollamaUrlVal =
      ctx.adminOllamaUrlDraft != null ? String(ctx.adminOllamaUrlDraft) : String(row.ollama_base_url || "");

    var ollamaCredsPanel = buildProviderPanel(
      "Server base URL",
      "",
      '<div class="sg-op-provider-edit-row sg-op-provider-edit-row--panel">' +
        '<div class="sg-op-provider-edit-main">' +
        '<input id="admin-ollama-url" class="sg-op-input" type="url" placeholder="http://127.0.0.1:11434" value="' +
        escapeHtml(ollamaUrlVal) +
        '"/></div>' +
        providerModelsIconBtn("ollama-save", providerId, "Save server URL", "keep") +
        "</div>",
      "endpoint",
      "admin-provider.keys"
    );

    var keysPanel = buildProviderPanel(
      "API keys",
      "",
      providerRowsHtml(providerId, row) + providerKeyAddBlockHtml(providerId),
      "keys",
      "admin-provider.keys"
    );

    var body = "";
    if (isOllama) {
      body = providerIntro + usagePanel + ollamaCredsPanel;
    } else {
      body = providerIntro + usagePanel + keysPanel;
    }
    var scopedPanel = "";
    if (hasCredentials) {
      var scoped = [];
      for (var ei = entryCache.length - 1; ei >= 0 && scoped.length < 18; ei--) {
        var ev = entryCache[ei];
        var fEv = getFlat(ev.parsed);
        if (adminProviderEventMatches(fEv, providerId)) scoped.push(ev);
      }
      scopedPanel = adminScopedEvlogPanelFromEvents("Scoped log - " + title, "provider-" + providerId, scoped, {
        uiPart: "admin-provider.scoped-evlog"
      });
    }
    var avatarClass = adminProviderAvatarClass(providerId);
    var cardCls = "sum-card sum-card--provider";
    if (editing) cardCls += " sum-card--provider-models-editing";
    return (
      '<details class="' +
      cardCls +
      '" id="admin-provider-' +
      escapeHtml(providerId) +
      '">' +
      '<summary data-ui-part="admin-provider.summary"><span class="sum-avatar ' +
      escapeHtml(avatarClass) +
      '">' +
      escapeHtml(avatar) +
      '</span><span class="sum-main"><span class="sum-title">' +
      escapeHtml(title) +
      "</span>" +
      '<span class="sum-sub sum-sub--clamp">' +
      escapeHtml(subtitle) +
      "</span></span>" +
      metrics +
      operatorCardChevronHtml() +
      '</summary><div class="sum-body sg-op-provider-card-body">' +
      body +
      scopedPanel +
      "</div></details>"
    );
  }

  var ADMIN_PROVIDERS_INTRO_HTML =
    '<div class="sum-workspaces-intro"><p class="sum-workspaces-intro-lead">Providers drive upstream inference through chimera-broker; each card shows configuration, usage, and scoped log activity.</p></div>';

  function providerCatalogApi() {
    return globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog
      ? ChimeraSettings.Providers.Catalog
      : null;
  }

  function buildAdminProviderPickerHtml() {
    var api = providerCatalogApi();
    var addable = api && typeof api.addableProviderEntries === "function" ? api.addableProviderEntries(ctx) : [];
    var listHtml = "";
    if (!addable.length) {
      listHtml = '<p class="sg-op-provider-picker__empty muted">All catalog providers are already on this page.</p>';
    } else {
      listHtml = '<ul class="sg-op-provider-picker__list" role="listbox" aria-label="Add provider">';
      for (var pi = 0; pi < addable.length; pi++) {
        var ent = addable[pi];
        listHtml +=
          '<li class="sg-op-provider-picker__item" role="presentation">' +
          '<button type="button" class="sg-op-provider-picker__option" role="option" data-admin-action="provider-picker-select" data-provider-id="' +
          escapeHtml(ent.id) +
          '">' +
          '<span class="sg-op-provider-picker__avatar" aria-hidden="true">' +
          escapeHtml(ent.avatar || ent.id.slice(0, 2).toUpperCase()) +
          "</span>" +
          '<span class="sg-op-provider-picker__meta">' +
          '<span class="sg-op-provider-picker__title">' +
          escapeHtml(ent.title || ent.id) +
          "</span>" +
          '<span class="sg-op-provider-picker__sub">' +
          escapeHtml(ent.subtitle || "") +
          "</span></span></button></li>";
      }
      listHtml += "</ul>";
    }
    return (
      '<div id="sg-op-provider-picker" class="sg-op-provider-picker" hidden>' +
      '<div class="sg-op-provider-picker__panel">' +
      listHtml +
      '<div class="sg-op-provider-picker__foot">' +
      '<button type="button" class="sg-op-btn sg-op-btn--ghost" data-admin-action="provider-picker-cancel">Cancel</button>' +
      "</div></div></div>"
    );
  }

  function buildAdminProvidersSectionBreakHtml() {
    if (typeof ctx.operatorSectionHeadHtml !== "function") {
      return (
        '<div class="sum-section-label sum-feed-section-title">Providers</div>' +
        ADMIN_PROVIDERS_INTRO_HTML
      );
    }
    var api = providerCatalogApi();
    var canAdd = api && typeof api.hasAddableProviders === "function" ? api.hasAddableProviders(ctx) : false;
    var actionHtml = "";
    if (typeof ctx.operatorSectionAddBtn === "function") {
      actionHtml = ctx.operatorSectionAddBtn(
        {
          "data-admin-action": "provider-picker-open",
          id: "sg-op-provider-picker-trigger",
          "aria-expanded": "false",
          "aria-controls": "sg-op-provider-picker",
          "aria-haspopup": "listbox"
        },
        "Add provider",
        canAdd
          ? { title: "Add a provider card" }
          : { disabled: true, title: "All catalog providers are already visible" }
      );
    }
    return (
      '<div class="sg-op-providers-section" id="sg-op-providers-section">' +
      '<div class="sg-op-providers-section__anchor">' +
      ctx.operatorSectionHeadHtml("Providers", "hub", { actionHtml: actionHtml }) +
      buildAdminProviderPickerHtml() +
      "</div>" +
      ADMIN_PROVIDERS_INTRO_HTML +
      "</div>"
    );
  }
  ctx.buildAdminProviderCardHtml = buildAdminProviderCardHtml;
  ctx.providerHasCredentials = providerHasCredentials;
  ctx.buildAdminProviderPickerHtml = buildAdminProviderPickerHtml;
  ctx.buildAdminProvidersSectionBreakHtml = buildAdminProvidersSectionBreakHtml;
};
