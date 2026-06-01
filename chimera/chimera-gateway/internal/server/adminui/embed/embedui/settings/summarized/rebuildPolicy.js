/**
 * Summarized feed rebuild vs patch guards (focus, admin edit, skip-card patch).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

globalThis.ChimeraSettings.Summarized.mountRebuildPolicy = function (ctx) {
  var strHash = ctx.strHash;

  function operatorWorkspaceNumericId(ws) {
    if (!ws || ws.id == null) return 0;
    var k = String(ws.id != null ? ws.id : "").trim();
    if (/^\d+$/.test(k)) {
      var n = parseInt(k, 10);
      return isNaN(n) ? 0 : n;
    }
    return 0;
  }

  function summarizedAdminEditingActive() {
    if (ctx.adminUserDrafts && ctx.adminUserDrafts.length) return true;
    if (ctx.virtualModelDrafts && ctx.virtualModelDrafts.length) return true;
    if (ctx.workspaceManagedEditId != null) return true;
    if (ctx.adminProviderModelsEditingId) return true;
    return false;
  }

  /** Skipped cards (e.g. while editing) may still need a rebuild when their hash changes. */
  function summarizedSkippedCardsHashDelta(prevModel, nextModel) {
    var skip = summarizedPatchSkipCardIds();
    if (!prevModel || !nextModel || !prevModel.cards || !nextModel.cards) return false;
    var prevMap = Object.create(null);
    var nextMap = Object.create(null);
    var i;
    for (i = 0; i < prevModel.cards.length; i++) {
      var pc = prevModel.cards[i];
      if (pc && pc.id && pc.kind !== "section-break") prevMap[pc.id] = pc;
    }
    for (i = 0; i < nextModel.cards.length; i++) {
      var nc = nextModel.cards[i];
      if (nc && nc.id && nc.kind !== "section-break") nextMap[nc.id] = nc;
    }
    for (var id in skip) {
      if (!Object.prototype.hasOwnProperty.call(skip, id) || !skip[id]) continue;
      if (prevMap[id] && nextMap[id] && prevMap[id].hash !== nextMap[id].hash) return true;
    }
    return false;
  }

  function summarizedPanelInteractionBlocksRebuild() {
    if (Date.now() < ctx.sumEvlogPointerSuppressedUntil) return true;
    var a = document.activeElement;
    if (!a || !a.closest) return false;
    if (!a.closest("#panel-summarized")) return false;
    var tag = String(a.tagName || "").toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select") return true;
    if (a.classList && a.classList.contains("sum-evlog__search")) return true;
    if (a.matches && a.matches("[data-evlog-filter-status]")) return true;
    var aid = a.id != null ? String(a.id) : "";
    if (aid.indexOf("vm-") === 0 && (tag === "input" || tag === "textarea" || tag === "select")) return true;
    if (a.closest && a.closest(".sum-card--virtual-model")) return true;
    if (a.closest && a.closest(".sum-card--virtual-model-draft")) return true;
    if (a.getAttribute && a.getAttribute("data-vm-draft-field")) return true;
    return false;
  }

  function summarizedPatchSkipCardIds() {
    var skip = Object.create(null);
    if (ctx.workspaceManagedEditId != null) {
      var wsn = ctx.lastIndexerOperatorWorkspacesNested || [];
      var wi;
      for (wi = 0; wi < wsn.length; wi++) {
        var w = wsn[wi];
        if (!w || w.id == null) continue;
        if (operatorWorkspaceNumericId(w) === ctx.workspaceManagedEditId) {
          skip["ix-opws-" + strHash(String(w.id))] = true;
          break;
        }
      }
    }
    var gwVm = ctx.adminStateCache && ctx.adminStateCache.gateway;
    var vmList = gwVm && gwVm.virtual_models && Array.isArray(gwVm.virtual_models) ? gwVm.virtual_models : [];
    for (var vmi = 0; vmi < vmList.length; vmi++) {
      var vmRow = vmList[vmi];
      if (!vmRow || vmRow.id == null) continue;
      var vmKey = String(vmRow.id);
      var vmCardId = "virtual-model-" + vmKey;
      var vmEl = document.getElementById(vmCardId);
      if (vmEl && vmEl.open) skip[vmCardId] = true;
      var vmUi = ctx.virtualModelUi && ctx.virtualModelUi[vmKey];
      if (vmUi && (vmUi.identityEditing || vmUi.fallbackEditing || vmUi.routingEditing || vmUi.routerEditing)) {
        skip[vmCardId] = true;
      }
    }
    if (ctx.adminProviderModelsEditingId) {
      skip["admin-provider-" + String(ctx.adminProviderModelsEditingId)] = true;
    }
    return skip;
  }

  ctx.summarizedAdminEditingActive = summarizedAdminEditingActive;
  ctx.summarizedSkippedCardsHashDelta = summarizedSkippedCardsHashDelta;
  ctx.summarizedPanelInteractionBlocksRebuild = summarizedPanelInteractionBlocksRebuild;
  ctx.summarizedPatchSkipCardIds = summarizedPatchSkipCardIds;
};
