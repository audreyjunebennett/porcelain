/**
 * Provider model availability configure/save/cancel (Phase 4).
 * Exports: ChimeraSettings.Handlers.ProviderModels.wire(ctx)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};
globalThis.ChimeraSettings.Handlers.ProviderModels = globalThis.ChimeraSettings.Handlers.ProviderModels || {};

globalThis.ChimeraSettings.Handlers.ProviderModels.wire = function (ctx) {
  var refreshAdminCardAfterEditToggle = ctx.refreshAdminCardAfterEditToggle;
  var refreshSummarizedPanel = ctx.refreshSummarizedPanel;
  var fetchAdminState = ctx.fetchAdminState;
  var fetchAdminTokens = ctx.fetchAdminTokens;
  var fetchChimeraBrokerProviderSnapshot = ctx.fetchChimeraBrokerProviderSnapshot;
  var fetchProviderModels = ctx.fetchProviderModels;
  var adminPostJSON = ctx.adminPostJSON;
  var adminPutJSON = ctx.adminPutJSON;
  var adminSetMessage = ctx.adminSetMessage;
  var patchAdminProviderCard = ctx.patchAdminProviderCard;
  var adminProviderModelsMapFromResponse = ctx.adminProviderModelsMapFromResponse;

  if (globalThis.__ChimeraSettingsProviderModelsWired) return;
  globalThis.__ChimeraSettingsProviderModelsWired = true;

  function ensureProviderDraft(providerId) {
    if (!ctx.adminProviderModelsDraft) ctx.adminProviderModelsDraft = {};
    if (!ctx.adminProviderModelsDraft[providerId]) {
      ctx.adminProviderModelsDraft[providerId] = { models: {} };
    }
    return ctx.adminProviderModelsDraft[providerId];
  }

  function refreshProviderModelsCard(providerId) {
    if (typeof refreshAdminCardAfterEditToggle === "function") {
      refreshAdminCardAfterEditToggle(function () {
        return typeof patchAdminProviderCard === "function" && patchAdminProviderCard(providerId);
      });
      return;
    }
    refreshSummarizedPanel();
  }

  function reloadAdminAfterProviderModelsSave() {
    var jobs = [fetchAdminState(), fetchAdminTokens()];
    if (typeof fetchChimeraBrokerProviderSnapshot === "function") {
      jobs.push(fetchChimeraBrokerProviderSnapshot());
    }
    return Promise.all(jobs).then(function () {
      if (typeof ctx.patchAdminCardsFromPoll === "function" && ctx.patchAdminCardsFromPoll()) return;
      refreshSummarizedPanel();
    });
  }

  function syncProviderModelCheckbox(input, checked) {
    if (!input || input.disabled) return;
    var providerId = String(input.getAttribute("data-provider") || "").trim().toLowerCase();
    var modelId = String(input.getAttribute("data-model-id") || "").trim();
    if (!providerId || !modelId || ctx.adminProviderModelsEditingId !== providerId) return;
    input.checked = !!checked;
    var draft = ensureProviderDraft(providerId);
    if (!draft.models) draft.models = {};
    draft.models[modelId] = !!checked;
    var row = input.closest && input.closest(".sg-op-provider-model-item, tr");
    if (row && row.classList) {
      row.classList.toggle("sg-op-provider-model-row--unavailable", !checked);
    }
  }

  function providerModelCheckboxAnchorFromEl(el, providerId) {
    if (!el || typeof el.closest !== "function" || !providerId) return null;
    var input = null;
    if (el.matches && el.matches('input[data-admin-provider-model-toggle="1"]')) {
      input = el;
    } else {
      var label = el.closest(".sg-op-provider-model-toggle:not(.sg-op-provider-model-toggle--readonly)");
      if (label) input = label.querySelector('input[data-admin-provider-model-toggle="1"]');
    }
    if (!input || input.disabled) return null;
    var pid = String(input.getAttribute("data-provider") || "").trim().toLowerCase();
    if (!pid || pid !== providerId || ctx.adminProviderModelsEditingId !== providerId) return null;
    return input;
  }

  function providerModelCheckboxFromRowEl(el, providerId) {
    if (!el || typeof el.closest !== "function" || !providerId) return null;
    var list = el.closest(".sg-op-provider-models-table");
    if (!list) return null;
    var row = el.closest(".sg-op-provider-model-item, tr");
    if (!row || !list.contains(row)) return null;
    var input = row.querySelector('input[data-admin-provider-model-toggle="1"]');
    if (!input || input.disabled) return null;
    var pid = String(input.getAttribute("data-provider") || "").trim().toLowerCase();
    if (!pid || pid !== providerId || ctx.adminProviderModelsEditingId !== providerId) return null;
    return input;
  }

  var providerModelDragPaint = null;
  var providerModelDragThresholdPx = 4;
  var providerModelGesturePending = false;

  function endProviderModelDragPaint() {
    if (!providerModelDragPaint) return;
    providerModelDragPaint = null;
    try {
      document.body.classList.remove("sg-op-provider-models-drag-paint");
    } catch (x) {}
  }

  function providerModelDragMovedEnough(ev) {
    if (!providerModelDragPaint || !ev) return false;
    var dx = ev.clientX - providerModelDragPaint.startX;
    var dy = ev.clientY - providerModelDragPaint.startY;
    var t = providerModelDragThresholdPx;
    return dx * dx + dy * dy >= t * t;
  }

  function beginProviderModelDragPaint() {
    if (!providerModelDragPaint || providerModelDragPaint.dragging) return;
    providerModelDragPaint.dragging = true;
    try {
      document.body.classList.add("sg-op-provider-models-drag-paint");
    } catch (x) {}
    applyProviderModelDragPaint(providerModelDragPaint.anchor);
  }

  function applyProviderModelDragPaint(input) {
    if (!providerModelDragPaint || !input) return;
    var modelId = String(input.getAttribute("data-model-id") || "").trim();
    if (!modelId || providerModelDragPaint.visited[modelId]) return;
    providerModelDragPaint.visited[modelId] = true;
    syncProviderModelCheckbox(input, providerModelDragPaint.targetState);
  }

  document.body.addEventListener(
    "change",
    function (ev) {
      var t = ev.target;
      if (!t || !t.getAttribute || !t.getAttribute("data-admin-provider-model-toggle")) return;
      syncProviderModelCheckbox(t, t.checked);
    },
    false
  );

  document.body.addEventListener(
    "mousedown",
    function (ev) {
      if (ev.button !== 0) return;
      var editingId = ctx.adminProviderModelsEditingId;
      if (!editingId) return;
      var input = providerModelCheckboxAnchorFromEl(ev.target, editingId);
      if (!input) return;
      var providerId = String(input.getAttribute("data-provider") || "").trim().toLowerCase();
      var modelId = String(input.getAttribute("data-model-id") || "").trim();
      if (!providerId || !modelId) return;
      ev.preventDefault();
      providerModelGesturePending = true;
      providerModelDragPaint = {
        providerId: providerId,
        anchor: input,
        targetState: !input.checked,
        startX: ev.clientX,
        startY: ev.clientY,
        dragging: false,
        visited: Object.create(null)
      };
    },
    false
  );

  document.addEventListener(
    "mousemove",
    function (ev) {
      if (!providerModelDragPaint) return;
      if (!providerModelDragPaint.dragging && providerModelDragMovedEnough(ev)) {
        beginProviderModelDragPaint();
      }
      if (!providerModelDragPaint || !providerModelDragPaint.dragging) return;
      var input = providerModelCheckboxFromRowEl(
        document.elementFromPoint(ev.clientX, ev.clientY),
        providerModelDragPaint.providerId
      );
      if (input) applyProviderModelDragPaint(input);
    },
    true
  );

  document.addEventListener(
    "mouseup",
    function (ev) {
      if (!providerModelDragPaint) return;
      if (!providerModelDragPaint.dragging) {
        syncProviderModelCheckbox(providerModelDragPaint.anchor, providerModelDragPaint.targetState);
      }
      endProviderModelDragPaint();
      setTimeout(function () {
        providerModelGesturePending = false;
      }, 0);
    },
    true
  );

  document.body.addEventListener(
    "click",
    function (ev) {
      if (!providerModelGesturePending) return;
      var editingId = ctx.adminProviderModelsEditingId;
      if (!editingId) return;
      var input = providerModelCheckboxAnchorFromEl(ev.target, editingId);
      if (!input) return;
      ev.preventDefault();
      ev.stopPropagation();
      providerModelGesturePending = false;
    },
    true
  );

  document.body.addEventListener(
    "click",
    function (ev) {
      var t = ev.target;
      if (!t || typeof t.closest !== "function") return;
      var actionEl = t.closest("[data-admin-action]");
      if (!actionEl || typeof actionEl.getAttribute !== "function") return;
      var act = actionEl.getAttribute("data-admin-action");
      if (
        act !== "provider-models-configure" &&
        act !== "provider-models-cancel" &&
        act !== "provider-models-save" &&
        act !== "provider-models-refresh" &&
        act !== "provider-models-apply-free-tier" &&
        act !== "provider-models-show-unavailable" &&
        act !== "provider-models-hide-unavailable"
      ) {
        return;
      }
      ev.preventDefault();
      ev.stopPropagation();

      var providerId = String(actionEl.getAttribute("data-provider") || "")
        .trim()
        .toLowerCase();
      if (!providerId) return;

      if (act === "provider-models-show-unavailable") {
        if (!ctx.adminProviderModelsShowUnavailable) ctx.adminProviderModelsShowUnavailable = {};
        ctx.adminProviderModelsShowUnavailable[providerId] = true;
        refreshProviderModelsCard(providerId);
        return;
      }

      if (act === "provider-models-hide-unavailable") {
        if (ctx.adminProviderModelsShowUnavailable) delete ctx.adminProviderModelsShowUnavailable[providerId];
        refreshProviderModelsCard(providerId);
        return;
      }

      if (act === "provider-models-configure") {
        if (ctx.adminProviderModelsEditingId && ctx.adminProviderModelsEditingId !== providerId) {
          adminSetMessage("err", "Finish or cancel the current provider model edit first.");
          return;
        }
        if (typeof fetchProviderModels !== "function") {
          adminSetMessage("err", "Provider models API unavailable.");
          return;
        }
        fetchProviderModels(providerId)
          .then(function (doc) {
            if (!ctx.adminProviderModelsCache) ctx.adminProviderModelsCache = {};
            ctx.adminProviderModelsCache[providerId] = doc;
            var draft = ensureProviderDraft(providerId);
            draft.models =
              typeof adminProviderModelsMapFromResponse === "function"
                ? adminProviderModelsMapFromResponse(doc)
                : {};
            draft.saving = false;
            ctx.adminProviderModelsEditingId = providerId;
            refreshProviderModelsCard(providerId);
          })
          .catch(function (e) {
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
        return;
      }

      if (act === "provider-models-cancel") {
        if (ctx.adminProviderModelsEditingId !== providerId) return;
        ctx.adminProviderModelsEditingId = null;
        if (ctx.adminProviderModelsDraft) delete ctx.adminProviderModelsDraft[providerId];
        refreshProviderModelsCard(providerId);
        return;
      }

      if (act === "provider-models-refresh") {
        if (ctx.adminProviderModelsEditingId !== providerId) return;
        var draftRefresh = ensureProviderDraft(providerId);
        var cached = ctx.adminProviderModelsCache && ctx.adminProviderModelsCache[providerId];
        if (cached) {
          draftRefresh.models =
            typeof adminProviderModelsMapFromResponse === "function"
              ? adminProviderModelsMapFromResponse(cached)
              : {};
          draftRefresh.saving = false;
          refreshProviderModelsCard(providerId);
          return;
        }
        if (typeof fetchProviderModels !== "function") {
          adminSetMessage("err", "Provider models API unavailable.");
          return;
        }
        fetchProviderModels(providerId)
          .then(function (doc) {
            if (!ctx.adminProviderModelsCache) ctx.adminProviderModelsCache = {};
            ctx.adminProviderModelsCache[providerId] = doc;
            draftRefresh.models =
              typeof adminProviderModelsMapFromResponse === "function"
                ? adminProviderModelsMapFromResponse(doc)
                : {};
            draftRefresh.saving = false;
            refreshProviderModelsCard(providerId);
          })
          .catch(function (e) {
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
        return;
      }

      if (act === "provider-models-apply-free-tier") {
        if (ctx.adminProviderModelsEditingId !== providerId) return;
        adminPostJSON("/api/ui/providers/" + encodeURIComponent(providerId) + "/models/apply-free-tier", {})
          .then(function (doc) {
            var draft = ensureProviderDraft(providerId);
            draft.models =
              typeof adminProviderModelsMapFromResponse === "function"
                ? adminProviderModelsMapFromResponse(doc)
                : {};
            if (doc && doc.note) adminSetMessage("", String(doc.note));
            else adminSetMessage("", "Free-tier draft applied. Save to persist.");
            refreshProviderModelsCard(providerId);
          })
          .catch(function (e) {
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
        return;
      }

      if (act === "provider-models-save") {
        if (ctx.adminProviderModelsEditingId !== providerId) return;
        var draftSave = ensureProviderDraft(providerId);
        var modelsMap = draftSave.models || {};
        draftSave.saving = true;
        refreshProviderModelsCard(providerId);
        adminPutJSON("/api/ui/providers/" + encodeURIComponent(providerId) + "/models", { models: modelsMap })
          .then(function (doc) {
            if (!ctx.adminProviderModelsCache) ctx.adminProviderModelsCache = {};
            ctx.adminProviderModelsCache[providerId] = doc;
            ctx.adminProviderModelsEditingId = null;
            if (ctx.adminProviderModelsDraft) delete ctx.adminProviderModelsDraft[providerId];
            adminSetMessage("", "Model availability saved.");
            return reloadAdminAfterProviderModelsSave();
          })
          .catch(function (e) {
            draftSave.saving = false;
            refreshProviderModelsCard(providerId);
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
      }
    },
    false
  );
};
