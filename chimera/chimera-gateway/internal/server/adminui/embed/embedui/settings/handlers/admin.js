/**
 * Admin workflow cards and workspace draft UI (tokens, providers).
 * Exports: ChimeraSettings.Handlers.Admin.wire(ctx)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};
globalThis.ChimeraSettings.Handlers.Admin = globalThis.ChimeraSettings.Handlers.Admin || {};

globalThis.ChimeraSettings.Handlers.Admin.wire = function (ctx) {
  var refreshSummarizedPanel = ctx.refreshSummarizedPanel;
  var scheduleStoryRebuild = ctx.scheduleStoryRebuild;
  var findWorkspaceDraft = ctx.findWorkspaceDraft;
  var appendWorkspaceDraftPath = ctx.appendWorkspaceDraftPath;
  var syncWorkspaceDraftHeader = ctx.syncWorkspaceDraftHeader;
  var pickFolderForWorkspaceDraft = ctx.pickFolderForWorkspaceDraft;
  var fetchAdminState = ctx.fetchAdminState;
  var fetchAdminTokens = ctx.fetchAdminTokens;
  var adminPostJSON = ctx.adminPostJSON;
  var adminSetMessage = ctx.adminSetMessage;
  var saveWorkspaceDraftById = ctx.saveWorkspaceDraftById;
  var removeWorkspaceDraft = ctx.removeWorkspaceDraft;
  var beginWorkspaceManagedEdit = ctx.beginWorkspaceManagedEdit;
  var cancelWorkspaceManagedEdit = ctx.cancelWorkspaceManagedEdit;
  var refreshWorkspaceManagedPaths = ctx.refreshWorkspaceManagedPaths;
  var saveManagedWorkspacePaths = ctx.saveManagedWorkspacePaths;
  var deleteManagedWorkspace = ctx.deleteManagedWorkspace;

  if (!globalThis.__ChimeraSettingsWorkspaceDraftUiWired) {
    globalThis.__ChimeraSettingsWorkspaceDraftUiWired = true;

    function syncManagedWorkspacePathActionBtns(cardEl, selEl) {
      if (!cardEl) return;
      if (!selEl) selEl = cardEl.querySelector(".ws-managed-paths-select");
      var rmBt = cardEl.querySelector(".ws-managed-btn-remove");
      var addBt = cardEl.querySelector(".ws-managed-btn-add");
      if (rmBt && selEl) {
        rmBt.disabled =
          selEl.selectedIndex < 0 || !selEl.options || !selEl.options.length;
      }
      if (addBt) addBt.disabled = !!ctx.workspaceManagedFolderPickerOpen;
    }

    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        if (t.closest("[data-sum-workspaces-create]")) {
          var createBtn = t.closest("[data-sum-workspaces-create]");
          if (
            !createBtn ||
            createBtn.disabled ||
            createBtn.getAttribute("aria-disabled") === "true" ||
            (ctx.workspaceDesktopFeaturesAvailable &&
              !ctx.workspaceDesktopFeaturesAvailable())
          ) {
            return;
          }
          ev.preventDefault();
          ev.stopPropagation();
          ctx.workspaceDrafts.push({
            id: ctx.nextWorkspaceDraftId++,
            projectId: "",
            flavorId: "",
            paths: []
          });
          scheduleStoryRebuild();
          return;
        }
        var managedCard = t.closest("[data-workspace-managed-id]");
        if (managedCard) {
          var wsNumM = Number(managedCard.getAttribute("data-workspace-managed-id"));
          if (!wsNumM) return;
          if (t.closest(".ws-managed-btn-configure")) {
            var cfgBtn = t.closest(".ws-managed-btn-configure");
            if (
              !cfgBtn ||
              cfgBtn.disabled ||
              cfgBtn.getAttribute("aria-disabled") === "true" ||
              (ctx.workspaceDesktopFeaturesAvailable &&
                !ctx.workspaceDesktopFeaturesAvailable())
            ) {
              return;
            }
            ev.preventDefault();
            ev.stopPropagation();
            beginWorkspaceManagedEdit(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-cancel")) {
            ev.preventDefault();
            ev.stopPropagation();
            cancelWorkspaceManagedEdit();
            return;
          }
          if (t.closest(".ws-managed-btn-refresh")) {
            ev.preventDefault();
            ev.stopPropagation();
            refreshWorkspaceManagedPaths();
            return;
          }
          if (t.closest(".ws-managed-btn-save")) {
            ev.preventDefault();
            ev.stopPropagation();
            saveManagedWorkspacePaths(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-delete")) {
            ev.preventDefault();
            ev.stopPropagation();
            deleteManagedWorkspace(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-add")) {
            ev.preventDefault();
            ev.stopPropagation();
            if (
              ctx.workspaceManagedEditId !== wsNumM ||
              !ctx.workspaceManagedStaging ||
              ctx.workspaceManagedStaging.wsNum !== wsNumM
            ) {
              return;
            }
            if (ctx.workspaceManagedFolderPickerOpen) return;
            var addBtM = t.closest(".ws-managed-btn-add");
            ctx.workspaceManagedFolderPickerOpen = true;
            if (addBtM) addBtM.disabled = true;
            var stA = ctx.workspaceManagedStaging.paths;
            var startDirA = stA && stA.length ? stA[stA.length - 1].path : "";
            pickFolderForWorkspaceDraft(startDirA)
              .then(function (picked) {
                if (!picked) return;
                ctx.workspaceManagedStaging.paths.push({ id: null, path: String(picked).trim() });
                scheduleStoryRebuild();
              })
              .finally(function () {
                ctx.workspaceManagedFolderPickerOpen = false;
                var cardNow = document.querySelector(
                  '[data-workspace-managed-id="' + String(wsNumM) + '"]'
                );
                syncManagedWorkspacePathActionBtns(cardNow);
              });
            return;
          }
          if (t.closest(".ws-managed-btn-remove")) {
            ev.preventDefault();
            ev.stopPropagation();
            if (
              ctx.workspaceManagedEditId !== wsNumM ||
              !ctx.workspaceManagedStaging ||
              ctx.workspaceManagedStaging.wsNum !== wsNumM
            ) {
              return;
            }
            var selMR = managedCard.querySelector(".ws-managed-paths-select");
            if (!selMR || selMR.selectedIndex < 0 || !ctx.workspaceManagedStaging.paths.length) return;
            ctx.workspaceManagedStaging.paths.splice(selMR.selectedIndex, 1);
            scheduleStoryRebuild();
            return;
          }
        }
        var card = t.closest("[data-workspace-draft]");
        if (!card) return;
        var draftId = Number(card.getAttribute("data-workspace-draft"));
        if (!draftId) return;
        if (t.closest(".ws-draft-btn-cancel")) {
          ev.preventDefault();
          removeWorkspaceDraft(draftId);
          scheduleStoryRebuild();
          return;
        }
        if (t.closest(".ws-draft-btn-save")) {
          ev.preventDefault();
          saveWorkspaceDraftById(draftId);
          return;
        }
        if (t.closest(".ws-draft-btn-add")) {
          ev.preventDefault();
          var dAdd = findWorkspaceDraft(draftId);
          if (!dAdd) return;
          var startDir = "";
          if (dAdd.paths && dAdd.paths.length) startDir = dAdd.paths[dAdd.paths.length - 1];
          pickFolderForWorkspaceDraft(startDir).then(function (picked) {
            if (!picked) return;
            appendWorkspaceDraftPath(dAdd, picked);
            scheduleStoryRebuild();
          });
          return;
        }
        if (t.closest(".ws-draft-btn-remove")) {
          ev.preventDefault();
          var dRm = findWorkspaceDraft(draftId);
          if (!dRm || !dRm.paths || !dRm.paths.length) return;
          var selRm = card.querySelector(".ws-draft-paths-select");
          if (!selRm || selRm.selectedIndex < 0) return;
          dRm.paths.splice(selRm.selectedIndex, 1);
          scheduleStoryRebuild();
          return;
        }
      },
      false
    );
    document.body.addEventListener(
      "input",
      function (ev) {
        var el = ev.target;
        if (!el || !el.getAttribute) return;
        var field = el.getAttribute("data-ws-field");
        if (!field) return;
        var cardIn = el.closest("[data-workspace-draft]");
        if (!cardIn) return;
        var did = Number(cardIn.getAttribute("data-workspace-draft"));
        var dIn = findWorkspaceDraft(did);
        if (!dIn) return;
        var vv = el.value != null ? String(el.value) : "";
        if (field === "project") dIn.projectId = vv;
        else if (field === "flavor") dIn.flavorId = vv;
        syncWorkspaceDraftHeader(cardIn, dIn);
      },
      false
    );
    document.body.addEventListener(
      "change",
      function (ev) {
        var el = ev.target;
        if (!el || !el.classList) return;
        var cardManagedCh = el.closest("[data-workspace-managed-id]");
        if (cardManagedCh && el.classList.contains("ws-managed-paths-select")) {
          syncManagedWorkspacePathActionBtns(cardManagedCh, el);
          return;
        }
        if (!el.classList.contains("ws-draft-paths-select")) return;
        var cardCh = el.closest("[data-workspace-draft]");
        if (!cardCh) return;
        var rmBt = cardCh.querySelector(".ws-draft-btn-remove");
        if (rmBt)
          rmBt.disabled =
            el.selectedIndex < 0 || !el.options || !el.options.length;
      },
      false
    );
    document.body.addEventListener(
      "click",
      function (ev) {
        var sel = ev.target && ev.target.closest && ev.target.closest(".ws-managed-paths-select");
        if (!sel) return;
        var cardManagedClick = sel.closest("[data-workspace-managed-id]");
        if (!cardManagedClick) return;
        window.setTimeout(function () {
          syncManagedWorkspacePathActionBtns(cardManagedClick, sel);
        }, 0);
      },
      false
    );
  }

  if (!globalThis.__ChimeraSettingsAdminWorkflowWired) {
    globalThis.__ChimeraSettingsAdminWorkflowWired = true;

    var OF = globalThis.ChimeraShared && globalThis.ChimeraShared.OperatorFeedback;
    var YE = globalThis.ChimeraShared && globalThis.ChimeraShared.YamlEditor;
    var DI = globalThis.ChimeraShared && globalThis.ChimeraShared.DraftInput;
    var PC = globalThis.ChimeraShared && globalThis.ChimeraShared.ProviderCredentials;

    function setAdminSaveBtnPending(btn, pending) {
      if (OF && typeof OF.setSaveBtnPending === "function") {
        OF.setSaveBtnPending(btn, pending);
        return;
      }
      if (!btn) return;
      btn.disabled = !!pending;
      if (pending) btn.setAttribute("aria-disabled", "true");
      else btn.removeAttribute("aria-disabled");
    }

    function syncYamlOverlayVScrollFromTarget(t) {
      if (YE && typeof YE.syncOverlayVScrollFromTarget === "function") {
        YE.syncOverlayVScrollFromTarget(t);
      }
    }

    function reloadAdmin() {
      Promise.all([fetchAdminState(), fetchAdminTokens()]).then(function () {
        if (typeof ctx.patchAdminCardsFromPoll === "function" && ctx.patchAdminCardsFromPoll()) return;
        refreshSummarizedPanel();
      });
    }

    function credentialActionOpts(triggerBtn) {
      return {
        ctx: ctx,
        triggerBtn: triggerBtn,
        setMessage: adminSetMessage,
        setPending: setAdminSaveBtnPending,
        postJSON: adminPostJSON,
        onSuccess: reloadAdmin
      };
    }

    document.body.addEventListener("input", function (ev) {
      var t = ev.target;
      if (!t || !t.id) return;
      if (DI && typeof DI.applyAdminCredentialInput === "function" && DI.applyAdminCredentialInput(ctx, t)) {
        return;
      }
    });
    document.body.addEventListener("input", function (ev) {
      var t = ev.target;
      if (!t || typeof t.getAttribute !== "function") return;
      var fld = t.getAttribute("data-admin-user-field");
      if (!fld) return;
      var did = Number(t.getAttribute("data-draft-id"));
      if (!did) return;
      for (var i = 0; i < ctx.adminUserDrafts.length; i++) {
        if (ctx.adminUserDrafts[i] && ctx.adminUserDrafts[i].id === did) {
          ctx.adminUserDrafts[i][fld] = t.value != null ? String(t.value) : "";
          break;
        }
      }
    });

    document.body.addEventListener("click", function (ev) {
      var t = ev.target;
      if (!t || typeof t.closest !== "function") return;
      var actionEl = t.closest("[data-admin-action]");
      if (!actionEl || typeof actionEl.getAttribute !== "function") return;
      t = actionEl;
      var act = t.getAttribute("data-admin-action");
      if (!act) return;
      ev.preventDefault();
      ev.stopPropagation();

      function patchAdminUsersCardOrRefresh() {
        if (typeof ctx.patchAdminUsersCard === "function" && ctx.patchAdminUsersCard()) return;
        refreshSummarizedPanel();
      }

      function closeProviderPicker() {
        if (typeof ctx.closeAdminProviderPicker === "function") ctx.closeAdminProviderPicker();
      }

      if (act === "provider-picker-open") {
        if (typeof ctx.setAdminProviderPickerOpen === "function") {
          var pickerEl = document.getElementById("sg-op-provider-picker");
          var willOpen = !!(pickerEl && pickerEl.hidden);
          ctx.setAdminProviderPickerOpen(willOpen);
        }
        return;
      }

      if (act === "provider-picker-cancel") {
        closeProviderPicker();
        return;
      }

      if (act === "provider-picker-select") {
        var pickId = String(t.getAttribute("data-provider-id") || "")
          .trim()
          .toLowerCase();
        if (!pickId) return;
        var cat =
          globalThis.ChimeraSettings &&
          ChimeraSettings.Providers &&
          ChimeraSettings.Providers.Catalog;
        if (cat && typeof cat.addVisibleProviderId === "function" && cat.addVisibleProviderId(ctx, pickId)) {
          closeProviderPicker();
          scheduleStoryRebuild();
        }
        return;
      }

      if (act === "user-add") {
        ctx.adminUserDrafts.unshift({
          id: ctx.nextAdminUserDraftId++,
          name: "",
          email: "",
          saving: false,
          msg: ""
        });
        patchAdminUsersCardOrRefresh();
        return;
      }

      if (act === "user-draft-cancel") {
        var dCancel = Number(t.getAttribute("data-draft-id"));
        if (!dCancel) return;
        var kept = [];
        for (var dc = 0; dc < ctx.adminUserDrafts.length; dc++) {
          if (!ctx.adminUserDrafts[dc] || ctx.adminUserDrafts[dc].id !== dCancel) kept.push(ctx.adminUserDrafts[dc]);
        }
        ctx.adminUserDrafts = kept;
        patchAdminUsersCardOrRefresh();
        return;
      }

      if (act === "user-draft-save") {
        var dSave = Number(t.getAttribute("data-draft-id"));
        if (!dSave) return;
        var draft = null;
        for (var ds = 0; ds < ctx.adminUserDrafts.length; ds++) {
          if (ctx.adminUserDrafts[ds] && ctx.adminUserDrafts[ds].id === dSave) {
            draft = ctx.adminUserDrafts[ds];
            break;
          }
        }
        if (!draft) return;
        draft.saving = true;
        draft.msg = "";
        patchAdminUsersCardOrRefresh();
        var label = String(draft.name || draft.email || "token").trim();
        adminPostJSON("/api/ui/tokens", { label: label })
          .then(function (j) {
            adminSetMessage("", "User token created. Copy it now; it will not be shown again.");
            var keep = [];
            for (var di = 0; di < ctx.adminUserDrafts.length; di++) {
              if (!ctx.adminUserDrafts[di] || ctx.adminUserDrafts[di].id !== dSave) keep.push(ctx.adminUserDrafts[di]);
            }
            ctx.adminUserDrafts = keep;
            var tenant = j && j.tenant_id != null ? String(j.tenant_id).trim() : "";
            if (tenant) {
              ctx.adminCreatedTokenByTenant[tenant] = String((j && j.token) || "");
            }
            reloadAdmin();
          })
          .catch(function (e) {
            draft.saving = false;
            draft.msg = e && e.message ? e.message : String(e);
            patchAdminUsersCardOrRefresh();
            adminSetMessage("err", draft.msg);
          });
        return;
      }

      if (act === "user-token-copy") {
        var valCopy = String(t.getAttribute("data-token") || "");
        if (valCopy) {
          if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(valCopy).catch(function () { });
          } else {
            var taCopy = document.createElement("textarea");
            taCopy.value = valCopy;
            taCopy.style.position = "fixed";
            taCopy.style.opacity = "0";
            document.body.appendChild(taCopy);
            taCopy.focus();
            taCopy.select();
            try { document.execCommand("copy"); } catch (_eCopy) {}
            try { document.body.removeChild(taCopy); } catch (_eCopyRm) {}
          }
        }
        return;
      }

      if (act === "token-create") {
        var tokLabel = (document.getElementById("admin-token-label") || {}).value || "";
        adminPostJSON("/api/ui/tokens", { label: String(tokLabel).trim() })
          .then(function (j) {
            var tenant2 = j && j.tenant_id != null ? String(j.tenant_id).trim() : "";
            if (tenant2) ctx.adminCreatedTokenByTenant[tenant2] = String((j && j.token) || "");
            var tl = document.getElementById("admin-token-label");
            if (tl) tl.value = "";
            adminSetMessage("", "Token created.");
            reloadAdmin();
          })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        return;
      }

      if (act === "token-delete") {
        var idx = parseInt(String(t.getAttribute("data-index") || ""), 10);
        if (isNaN(idx)) return;
        adminPostJSON("/api/ui/tokens/delete", { index: idx })
          .then(function () { adminSetMessage("", "Token removed."); reloadAdmin(); })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        return;
      }

      if (act === "provider-key-add") {
        if (PC && typeof PC.runProviderKeyAdd === "function") {
          var provAdd = String(t.getAttribute("data-provider") || "").trim().toLowerCase();
          var keyOpts = credentialActionOpts(t);
          keyOpts.providerId = provAdd;
          PC.runProviderKeyAdd(keyOpts);
          return;
        }
        return;
      }

      if (act === "provider-key-delete") {
        var provDel = String(t.getAttribute("data-provider") || "");
        var nmDel = String(t.getAttribute("data-name") || "");
        if (!provDel || !nmDel) return;
        adminPostJSON("/api/ui/provider/" + provDel + "/keys/delete", { name: nmDel })
          .then(function () { adminSetMessage("", "Provider key removed."); reloadAdmin(); })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        return;
      }

      if (act === "ollama-save") {
        if (PC && typeof PC.runOllamaUrlSave === "function") {
          PC.runOllamaUrlSave(credentialActionOpts(t));
          return;
        }
        return;
      }

    });

    document.body.addEventListener("focusin", function (ev) {
      var t = ev.target;
      syncYamlOverlayVScrollFromTarget(t);
    });

    document.body.addEventListener("input", function (ev) {
      var t = ev.target;
      syncYamlOverlayVScrollFromTarget(t);
    });

    document.body.addEventListener("scroll", function (ev) {
      var t = ev.target;
      syncYamlOverlayVScrollFromTarget(t);
    }, true);

    window.addEventListener("resize", function () {
      var wraps = document.querySelectorAll(".sg-op-yaml-wrap");
      for (var i = 0; i < wraps.length; i++) {
        syncYamlOverlayVScrollFromTarget(wraps[i]);
      }
    });
  }
};
