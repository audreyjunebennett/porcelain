/**
 * Workspace draft + managed workspace path editor chrome.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountWorkspaceDraft = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var WP = globalThis.ChimeraShared && globalThis.ChimeraShared.WorkspacePaths;
  var ET = globalThis.ChimeraShared && globalThis.ChimeraShared.EditToolbar;
  var resolveLogsOperatorUserLabel =
    typeof ctx.resolveLogsOperatorUserLabel === "function"
      ? ctx.resolveLogsOperatorUserLabel
      : function () {
          return "—";
        };
  var workspaceDesktopFeaturesAvailable =
    typeof ctx.workspaceDesktopFeaturesAvailable === "function"
      ? ctx.workspaceDesktopFeaturesAvailable
      : function () {
          return false;
        };
  var wrapDesktopOnlyLockedControl =
    typeof ctx.wrapDesktopOnlyLockedControl === "function"
      ? ctx.wrapDesktopOnlyLockedControl
      : function (html) {
          return html;
        };
  var WORKSPACE_WEB_UNAVAILABLE_TITLE =
    "Configure watched paths in the Chimera desktop app (folder picker is not available in the browser-only UI).";

  function findWorkspaceDraft(id) {
    var drafts = ctx.workspaceDrafts;
    if (!Array.isArray(drafts)) return null;
    for (var i = 0; i < drafts.length; i++) {
      if (drafts[i].id === id) return drafts[i];
    }
    return null;
  }

  function removeWorkspaceDraft(id) {
    var next = [];
    var drafts = ctx.workspaceDrafts;
    if (!Array.isArray(drafts)) {
      ctx.workspaceDrafts = next;
      return;
    }
    for (var i = 0; i < drafts.length; i++) {
      if (drafts[i].id !== id) next.push(drafts[i]);
    }
    ctx.workspaceDrafts = next;
  }

  function notifyWorkspaceDraftMsg(msg, isErr) {
    var statusEl = ctx.statusEl;
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.className = msg
      ? isErr
        ? "workspace-draft-status workspace-draft-status--err"
        : "workspace-draft-status"
      : "";
    if (msg && !isErr) {
      try {
        window.clearTimeout(notifyWorkspaceDraftMsg._t);
      } catch (_e) {}
      notifyWorkspaceDraftMsg._t = window.setTimeout(function () {
        if (statusEl && statusEl.textContent === msg) {
          statusEl.textContent = "";
          statusEl.className = "";
        }
      }, 4800);
    }
  }

  function nativeFolderPickerFn() {
    try {
      var topw = window.top;
      if (topw && typeof topw.chimeraPickFolder === "function") return topw.chimeraPickFolder;
    } catch (_eTop) {}
    return typeof window.chimeraPickFolder === "function" ? window.chimeraPickFolder : null;
  }

  function dirBasenameForWorkspace(p) {
    if (typeof ctx.dirBasenameForWorkspace === "function") return ctx.dirBasenameForWorkspace(p);
    var s = String(p || "").replace(/\\/g, "/").replace(/\/+$/, "");
    var parts = s.split("/");
    return parts.length ? parts[parts.length - 1] : s;
  }

  function pathsEditorHtml(paths, opts) {
    if (WP && typeof WP.pathsEditorHtml === "function") {
      return WP.pathsEditorHtml(escapeHtml, Object.assign({ paths: paths }, opts || {}));
    }
    return "";
  }

  function iconBtn(action, title, icon, extraClass, dataAttrs, disabled) {
    if (ET && typeof ET.iconBtnHtml === "function") {
      return ET.iconBtnHtml(escapeHtml, {
        action: action,
        title: title,
        icon: icon,
        extraClass: extraClass,
        dataAttrs: dataAttrs,
        disabled: disabled
      });
    }
    return "";
  }

  function syncWorkspaceDraftHeader(cardEl, d) {
    if (!cardEl || !d) return;
    var u = cardEl.querySelector(".ws-draft-lbl-user");
    var p = cardEl.querySelector(".ws-draft-lbl-proj");
    var f = cardEl.querySelector(".ws-draft-lbl-flav");
    var ulab = resolveLogsOperatorUserLabel();
    if (u) u.textContent = ulab !== "—" ? ulab : "";
    if (p) p.textContent = String(d.projectId != null ? d.projectId : "").trim();
    if (f) f.textContent = String(d.flavorId != null ? d.flavorId : "").trim();
  }

  function buildWorkspaceDraftCardHtml(d) {
    var uid = "ws-draft-" + d.id;
    var ulab = resolveLogsOperatorUserLabel();
    var projShown = String(d.projectId != null ? d.projectId : "").trim();
    var flavShown = String(d.flavorId != null ? d.flavorId : "").trim();
    var titleBits =
      '<span class="ws-draft-head-inline">' +
      '<span class="ws-draft-lbl ws-draft-lbl-user">' +
      (ulab !== "—" ? escapeHtml(ulab) : "") +
      "</span>" +
      '<span class="ws-draft-sep muted">·</span>' +
      '<span class="ws-draft-lbl ws-draft-lbl-proj">' +
      escapeHtml(projShown) +
      "</span>" +
      '<span class="ws-draft-sep muted">·</span>' +
      '<span class="ws-draft-lbl ws-draft-lbl-flav">' +
      escapeHtml(flavShown) +
      "</span>" +
      "</span>";
    var paths = d.paths && d.paths.length ? d.paths : [];
    var prVal = escapeHtml(String(d.projectId != null ? d.projectId : ""));
    var fvVal = escapeHtml(String(d.flavorId != null ? d.flavorId : ""));
    var pathsRow = pathsEditorHtml(paths, {
      selectAttr: 'data-ws-draft-paths="' + String(d.id) + '"',
      removeDisabled: !paths.length
    });
    var hint =
      WP && typeof WP.folderPickerHintHtml === "function"
        ? WP.folderPickerHintHtml()
        : '<p class="muted ws-draft-hint">Folder picker requires desktop shell.</p>';
    return (
      '<article class="sum-card sum-card--workspace-draft" id="' +
      escapeHtml(uid) +
      '" data-workspace-draft="' +
      String(d.id) +
      '">' +
      '<header class="sum-card__workspace-draft-hdr">' +
      '<span class="sum-avatar sum-av-c" title="New workspace">+</span>' +
      '<span class="sum-main sum-main--workspace-draft">' +
      '<span class="sum-title">' +
      titleBits +
      "</span>" +
      "</span>" +
      '<span class="ws-draft-actions">' +
      '<button type="button" class="sg-op-btn sg-op-btn--ghost ws-draft-btn-cancel">Cancel</button>' +
      '<button type="button" class="sg-op-btn ws-draft-btn-save">Save</button>' +
      "</span>" +
      "</header>" +
      '<div class="sum-body" data-ui-part="workspace-draft.form">' +
      '<div class="ws-draft-fields">' +
      '<div class="ws-draft-field">' +
      '<label class="ws-draft-field-label" for="' +
      escapeHtml(uid) +
      '-pr">Project id</label>' +
      '<input id="' +
      escapeHtml(uid) +
      '-pr" class="ws-draft-input" type="text" autocomplete="off" data-ws-field="project" value="' +
      prVal +
      '" />' +
      "</div>" +
      '<div class="ws-draft-field">' +
      '<label class="ws-draft-field-label" for="' +
      escapeHtml(uid) +
      '-fv">Flavor id</label>' +
      '<input id="' +
      escapeHtml(uid) +
      '-fv" class="ws-draft-input" type="text" autocomplete="off" data-ws-field="flavor" value="' +
      fvVal +
      '" />' +
      "</div>" +
      "</div>" +
      '<div class="sum-section-label">Watched paths</div>' +
      pathsRow +
      hint +
      "</div>" +
      "</article>"
    );
  }

  function buildManagedWorkspacePathsEditHtml(wsNum, pathRows) {
    var rows = pathRows && pathRows.length ? pathRows : [];
    var addDisabled = ctx.workspaceManagedFolderPickerOpen ? " disabled" : "";
    return pathsEditorHtml(rows, {
      wrapClass: "ws-managed-paths-edit",
      wrapAttr: 'data-ws-managed-paths="' + String(wsNum) + '"',
      selectClass: "ws-draft-paths-select ws-managed-paths-select",
      addBtnClass: "sg-op-btn sg-op-btn--ghost ws-managed-btn-add",
      removeBtnClass: "sg-op-btn sg-op-btn--ghost ws-managed-btn-remove",
      addDisabled: !!ctx.workspaceManagedFolderPickerOpen,
      removeDisabled: !rows.length
    });
  }

  function buildManagedWorkspaceConfigureBtnHtml(wsNum, titleText) {
    var lab =
      titleText && String(titleText).trim()
        ? "Configure workspace " + String(titleText).trim()
        : "Configure workspace";
    var desktop = workspaceDesktopFeaturesAvailable();
    var tip = desktop ? "Configure" : WORKSPACE_WEB_UNAVAILABLE_TITLE;
    var btn = iconBtn(null, lab, "settings", "ws-managed-btn-configure", { "ws-managed-id": String(wsNum) }, !desktop);
    if (!btn) return "";
    return wrapDesktopOnlyLockedControl(btn, !desktop, true);
  }

  function buildManagedWorkspaceEditToolbarHtml(_wsNum) {
    var inner =
      iconBtn(null, "Delete workspace", "delete_forever", "ws-managed-btn-delete") +
      iconBtn(null, "Keep watched paths", "keep", "ws-managed-btn-save") +
      iconBtn(null, "Revert watched paths to last saved", "refresh", "ws-managed-btn-refresh") +
      iconBtn(null, "Cancel editing", "cancel", "ws-managed-btn-cancel");
    if (ET && typeof ET.toolbarWrapHtml === "function") {
      return ET.toolbarWrapHtml(inner, "ws-managed-edit-controls");
    }
    return '<div class="ws-managed-edit-controls">' + inner + "</div>";
  }

  function buildManagedWorkspaceToolbarHtml(wsNum, isEdit, titleText) {
    var inner;
    if (isEdit) inner = buildManagedWorkspaceEditToolbarHtml(wsNum);
    else inner = buildManagedWorkspaceConfigureBtnHtml(wsNum, titleText);
    if (ET && typeof ET.toolbarWrapHtml === "function") {
      return (
        '<div data-ui-part="indexer-operator-workspace.toolbar">' +
        ET.toolbarWrapHtml(inner, "ws-managed-edit-controls") +
        "</div>"
      );
    }
    return (
      '<div data-ui-part="indexer-operator-workspace.toolbar">' +
      '<div class="ws-managed-edit-controls">' +
      inner +
      "</div></div>"
    );
  }

  function buildWorkspacesCreateBtnHtml(label) {
    var lab = label != null && String(label).trim() ? String(label).trim() : "Create workspace";
    var desktop = workspaceDesktopFeaturesAvailable();
    var dis = desktop ? "" : " disabled aria-disabled=\"true\"";
    return wrapDesktopOnlyLockedControl(
      '<button type="button" class="sum-workspaces-create-btn" data-sum-workspaces-create="1"' +
        dis +
        ' title="' +
        escapeHtml(lab) +
        '">' +
        escapeHtml(lab) +
        "</button>",
      !desktop
    );
  }


  function saveWorkspaceDraftById(draftId) {
    var d = findWorkspaceDraft(draftId);
    if (!d) return;
    var pj = String(d.projectId != null ? d.projectId : "").trim();
    var fv = String(d.flavorId != null ? d.flavorId : "").trim();
    if (!pj) {
      notifyWorkspaceDraftMsg("Project id is required.", true);
      return;
    }
    if (!d.paths || !d.paths.length) {
      notifyWorkspaceDraftMsg("Add at least one watched path.", true);
      return;
    }
    fetch("/api/ui/indexer/workspaces", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        project_id: pj,
        flavor_id: fv,
        paths: d.paths.slice()
      })
    })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "save failed");
          return j;
        });
      })
      .then(function (j) {
        removeWorkspaceDraft(draftId);
        notifyWorkspaceDraftMsg("Workspace saved.", false);
        if (j && Array.isArray(j.roots)) {
          ctx.lastIndexerOperatorRoots = j.roots;
          try {
            ctx.lastIndexerOperatorRootsJson = JSON.stringify(j.roots);
          } catch (_eSaveRoots) {
            ctx.lastIndexerOperatorRootsJson = "";
          }
        }
        if (j && j.workspace && typeof j.workspace === "object") {
          if (typeof ctx.mergeWorkspaceIntoOperatorNested === "function") {
            ctx.mergeWorkspaceIntoOperatorNested(j.workspace);
          }
        } else if (j && Array.isArray(j.roots)) {
          if (typeof ctx.dedupeOperatorWorkspacesNested === "function" && typeof ctx.deriveNestedWorkspacesFromFlatRoots === "function") {
            ctx.lastIndexerOperatorWorkspacesNested = ctx.dedupeOperatorWorkspacesNested(
              ctx.deriveNestedWorkspacesFromFlatRoots(j.roots)
            );
          }
        }
        if (typeof ctx.hydrateIndexerServiceSummaryFromApi === "function") {
          ctx.hydrateIndexerServiceSummaryFromApi(true);
        }
        if (typeof ctx.scheduleStoryRebuild === "function") ctx.scheduleStoryRebuild();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }

  function appendWorkspaceDraftPath(d, absPath) {
    var p = String(absPath || "").trim();
    if (!p) return;
    if (!d.paths) d.paths = [];
    var qi;
    for (qi = 0; qi < d.paths.length; qi++) {
      if (d.paths[qi] === p) return;
    }
    d.paths.push(p);
    var pjBlank = !String(d.projectId != null ? d.projectId : "").trim();
    if (pjBlank) {
      var base = dirBasenameForWorkspace(p);
      d.projectId = base;
      d.flavorId = "";
    }
  }

  function pickFolderForWorkspaceDraft(startDir) {
    var fn = nativeFolderPickerFn();
    if (!fn) {
      notifyWorkspaceDraftMsg("Folder picker requires the Chimera desktop app (chimeraPickFolder).", true);
      return Promise.resolve("");
    }
    var sd = startDir != null && startDir !== undefined ? String(startDir) : "";
    return Promise.resolve(fn(sd)).then(function (path) {
      return (path && String(path).trim()) || "";
    });
  }

  var WORKSPACE_SECTION_CREATE_UNAVAILABLE_TITLE =
    "Not available through the web. Use the desktop app.";

  function summarizedWorkspacesSectionHead() {
    if (typeof ctx.operatorSectionHeadHtml !== "function") {
      return (
        '<div class="sum-feed-section-head">' +
        '<span class="sum-feed-section-title sum-section-label">Workspaces</span>' +
        buildWorkspacesCreateBtnHtml("Create") +
        "</div>"
      );
    }
    var webOnly = !workspaceDesktopFeaturesAvailable();
    return ctx.operatorSectionHeadHtml("Workspaces", "database", {
      actionHtml:
        typeof ctx.operatorSectionAddBtn === "function"
          ? ctx.operatorSectionAddBtn(
              { "data-sum-workspaces-create": "1" },
              "Create workspace",
              webOnly
                ? {
                    disabled: true,
                    title: WORKSPACE_SECTION_CREATE_UNAVAILABLE_TITLE,
                    desktopLocked: true
                  }
                : undefined
            )
          : buildWorkspacesCreateBtnHtml("Create workspace"),
    });
  }

  /** Intro copy for the summarized-feed Workspaces section (replaces per-card blurbs). */
  function buildWorkspacesSectionIntroHtml() {
    return (
      '<div class="sum-workspaces-intro">' +
      '<p class="sum-workspaces-intro-lead">' +
      "Find the right snippets and docs while you work, without you pasting whole files into the chat. Point it at the places you care about, and it stays quietly up to date." +
      "</p>" +
      "</div>"
    );
  }
  ctx.findWorkspaceDraft = findWorkspaceDraft;
  ctx.removeWorkspaceDraft = removeWorkspaceDraft;
  ctx.notifyWorkspaceDraftMsg = notifyWorkspaceDraftMsg;
  ctx.nativeFolderPickerFn = nativeFolderPickerFn;
  ctx.pickFolderForWorkspaceDraft = pickFolderForWorkspaceDraft;
  ctx.appendWorkspaceDraftPath = appendWorkspaceDraftPath;
  ctx.saveWorkspaceDraftById = saveWorkspaceDraftById;
  ctx.buildWorkspaceDraftCardHtml = buildWorkspaceDraftCardHtml;
  ctx.syncWorkspaceDraftHeader = syncWorkspaceDraftHeader;
  ctx.buildManagedWorkspacePathsEditHtml = buildManagedWorkspacePathsEditHtml;
  ctx.buildManagedWorkspaceToolbarHtml = buildManagedWorkspaceToolbarHtml;
  ctx.buildWorkspacesCreateBtnHtml = buildWorkspacesCreateBtnHtml;
  ctx.buildWorkspacesSectionIntroHtml = buildWorkspacesSectionIntroHtml;
  ctx.summarizedWorkspacesSectionHead = summarizedWorkspacesSectionHead;
};
