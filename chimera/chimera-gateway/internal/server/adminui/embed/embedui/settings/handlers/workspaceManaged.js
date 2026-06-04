/**
 * Operator-managed workspace path edit/save (indexer API).
 * Exports: ChimeraSettings.Handlers.WorkspaceManaged.mount(ctx)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};
globalThis.ChimeraSettings.Handlers.WorkspaceManaged =
  globalThis.ChimeraSettings.Handlers.WorkspaceManaged || {};

globalThis.ChimeraSettings.Handlers.WorkspaceManaged.mount = function (ctx) {
  function notify(msg, isErr) {
    if (typeof ctx.notifyWorkspaceDraftMsg === "function") ctx.notifyWorkspaceDraftMsg(msg, isErr);
  }

  function normalizeManagedPathRowsForEdit(ws) {
    var out = [];
    if (!ws || !Array.isArray(ws.paths)) return out;
    var pi;
    for (pi = 0; pi < ws.paths.length; pi++) {
      var row = ws.paths[pi] || {};
      var pid = row.id != null ? Number(row.id) : NaN;
      var pth = row.path != null ? String(row.path).trim() : "";
      if (!pth) continue;
      if (!isNaN(pid) && pid > 0) out.push({ id: pid, path: pth });
    }
    return out;
  }

  function cloneManagedPathRows(arr) {
    var out = [];
    if (!Array.isArray(arr)) return out;
    var i;
    for (i = 0; i < arr.length; i++) {
      out.push({
        id: arr[i].id != null && !isNaN(Number(arr[i].id)) ? Math.trunc(Number(arr[i].id)) : null,
        path: String(arr[i].path != null ? arr[i].path : "")
      });
    }
    return out;
  }

  function beginWorkspaceManagedEdit(wsNum) {
    var ws =
      typeof ctx.findOperatorWorkspaceByNumericId === "function"
        ? ctx.findOperatorWorkspaceByNumericId(wsNum)
        : null;
    if (!ws) {
      notify("Workspace not found.", true);
      return;
    }
    var snap = normalizeManagedPathRowsForEdit(ws);
    ctx.workspaceManagedEditId = wsNum;
    ctx.workspaceManagedStaging = {
      wsNum: wsNum,
      initialSnapshot: cloneManagedPathRows(snap),
      paths: cloneManagedPathRows(snap)
    };
    ctx.summarizedForceFullRebuild = true;
    if (typeof ctx.refreshSummarizedPanel === "function") ctx.refreshSummarizedPanel();
  }

  function cancelWorkspaceManagedEdit() {
    ctx.workspaceManagedEditId = null;
    ctx.workspaceManagedStaging = null;
    ctx.workspaceManagedFolderPickerOpen = false;
    ctx.summarizedForceFullRebuild = true;
    if (typeof ctx.refreshSummarizedPanel === "function") ctx.refreshSummarizedPanel();
  }

  function refreshWorkspaceManagedPaths() {
    var st = ctx.workspaceManagedStaging;
    if (!st || !Array.isArray(st.initialSnapshot)) return;
    st.paths = cloneManagedPathRows(st.initialSnapshot);
    ctx.workspaceManagedFolderPickerOpen = false;
    ctx.summarizedForceFullRebuild = true;
    if (typeof ctx.refreshSummarizedPanel === "function") ctx.refreshSummarizedPanel();
  }

  function refreshOperatorIndexerWorkspaceStateFromConfig() {
    if (typeof ctx.hydrateIndexerServiceSummaryFromApi !== "function") {
      return Promise.resolve();
    }
    return ctx.hydrateIndexerServiceSummaryFromApi(true).then(function () {
      if (typeof ctx.scheduleStoryRebuild === "function") ctx.scheduleStoryRebuild();
    });
  }

  function saveManagedWorkspacePaths(wsNum) {
    var st = ctx.workspaceManagedStaging;
    if (!st || st.wsNum !== wsNum || !Array.isArray(st.paths)) {
      notify("Nothing to save.", true);
      return;
    }
    if (!st.paths.length) {
      notify("Add at least one watched path.", true);
      return;
    }
    var initial = st.initialSnapshot || [];
    var cur = st.paths;
    var curPersistedIds = {};
    var ci;
    for (ci = 0; ci < cur.length; ci++) {
      if (cur[ci].id != null && !isNaN(Number(cur[ci].id))) curPersistedIds[Math.trunc(Number(cur[ci].id))] = true;
    }
    var toDelete = [];
    var ii;
    for (ii = 0; ii < initial.length; ii++) {
      var iid = initial[ii].id != null ? Math.trunc(Number(initial[ii].id)) : NaN;
      if (!isNaN(iid) && iid > 0 && !curPersistedIds[iid]) toDelete.push(iid);
    }
    var toAdd = [];
    for (ci = 0; ci < cur.length; ci++) {
      var pth = String(cur[ci].path != null ? cur[ci].path : "").trim();
      if (!pth) continue;
      if (cur[ci].id == null || isNaN(Number(cur[ci].id))) toAdd.push(pth);
    }

    var chain = Promise.resolve();
    var di;
    for (di = 0; di < toDelete.length; di++) {
      (function (pathId) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspace-paths/" + pathId, {
            method: "DELETE",
            credentials: "same-origin"
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete path failed");
            });
          });
        });
      })(toDelete[di]);
    }
    var ai;
    for (ai = 0; ai < toAdd.length; ai++) {
      (function (absPath) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspaces/" + wsNum + "/paths", {
            method: "POST",
            credentials: "same-origin",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: absPath })
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "add path failed");
              return j;
            });
          });
        });
      })(toAdd[ai]);
    }
    chain
      .then(function () {
        ctx.workspaceManagedEditId = null;
        ctx.workspaceManagedStaging = null;
        notify("Workspace updated.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notify(err && err.message ? err.message : String(err), true);
      });
  }

  function reindexManagedWorkspace(wsNum) {
    var ws =
      typeof ctx.findOperatorWorkspaceByNumericId === "function"
        ? ctx.findOperatorWorkspaceByNumericId(wsNum)
        : null;
    var label = "";
    if (ws) {
      var pid = ws.project_id != null ? String(ws.project_id).trim() : "";
      var fid = ws.flavor_id != null ? String(ws.flavor_id).trim() : "";
      label = pid || fid ? pid + (pid && fid ? " / " : "") + fid : "workspace " + wsNum;
    } else {
      label = "workspace " + wsNum;
    }
    if (
      !window.confirm(
        "Re-upload all files in " +
          label +
          "? Local skip checkpoints will be cleared and the indexer will re-ingest on the next poll (about 30 seconds). This may take several minutes for large workspaces."
      )
    ) {
      return;
    }
    if (typeof ctx.markIndexerWorkspaceReindexPending === "function") {
      ctx.markIndexerWorkspaceReindexPending(wsNum, 120000);
    }
    ctx.summarizedForceFullRebuild = true;
    if (typeof ctx.scheduleDeferredSummarizedRefresh === "function") {
      ctx.scheduleDeferredSummarizedRefresh();
    }
    fetch("/api/ui/indexer/workspaces/" + wsNum + "/reindex", {
      method: "POST",
      credentials: "same-origin"
    })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "reindex failed");
          return j;
        });
      })
      .then(function (j) {
        var sec =
          j && j.indexer_poll_seconds != null && !isNaN(Number(j.indexer_poll_seconds))
            ? Math.round(Number(j.indexer_poll_seconds))
            : 30;
        notify(
          "Re-index requested for " +
            label +
            ". The indexer should pick this up within about " +
            sec +
            " seconds.",
          false
        );
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notify(err && err.message ? err.message : String(err), true);
      });
  }

  function deleteManagedWorkspace(wsNum) {
    if (
      !window.confirm(
        "Delete this workspace and all watched paths from configuration? The indexer will stop indexing these paths."
      )
    ) {
      return;
    }
    fetch("/api/ui/indexer/workspaces/" + wsNum, { method: "DELETE", credentials: "same-origin" })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete failed");
        });
      })
      .then(function () {
        ctx.workspaceManagedEditId = null;
        ctx.workspaceManagedStaging = null;
        notify("Workspace removed.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notify(err && err.message ? err.message : String(err), true);
      });
  }

  ctx.beginWorkspaceManagedEdit = beginWorkspaceManagedEdit;
  ctx.cancelWorkspaceManagedEdit = cancelWorkspaceManagedEdit;
  ctx.refreshWorkspaceManagedPaths = refreshWorkspaceManagedPaths;
  ctx.saveManagedWorkspacePaths = saveManagedWorkspacePaths;
  ctx.deleteManagedWorkspace = deleteManagedWorkspace;
  ctx.reindexManagedWorkspace = reindexManagedWorkspace;
};
