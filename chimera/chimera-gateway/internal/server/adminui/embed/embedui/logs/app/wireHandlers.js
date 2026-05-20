/**
 * DOM event wiring for summarized cards, workspaces, admin workflows, and chrome links.
 *
 * Exports: ChimeraLogs.App.mountWireHandlers(ctx)
 */

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.App = globalThis.ChimeraLogs.App || {};
globalThis.ChimeraLogs.App.mountWireHandlers = function (ctx) {
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
  var parseFallbackChainInput = ctx.parseFallbackChainInput;
  var fallbackChainToYAML = ctx.fallbackChainToYAML;
  var sumEvlogIsWarnish = ctx.sumEvlogIsWarnish;
  var sumEvlogIsFailish = ctx.sumEvlogIsFailish;
  var formatLogDateTimeLocal = ctx.formatLogDateTimeLocal;
  var formatLogRelativeAgo = ctx.formatLogRelativeAgo;
  var escapeHtml = ctx.escapeHtml;
  var saveWorkspaceDraftById = ctx.saveWorkspaceDraftById;
  var removeWorkspaceDraft = ctx.removeWorkspaceDraft;
  var beginWorkspaceManagedEdit = ctx.beginWorkspaceManagedEdit;
  var cancelWorkspaceManagedEdit = ctx.cancelWorkspaceManagedEdit;
  var saveManagedWorkspacePaths = ctx.saveManagedWorkspacePaths;
  var deleteManagedWorkspace = ctx.deleteManagedWorkspace;
  (function wireSummarizedEvlogPanels() {
    if (globalThis.__chimeraLogsSumEvlogWired) return;
    globalThis.__chimeraLogsSumEvlogWired = true;
    var searchTimers = typeof WeakMap !== "undefined" ? new WeakMap() : null;
    /** Pixels from tbody bottom to treat as "stuck to tail" when summarized panel is rebuilt. */
    var SUM_EVLOG_TB_TAIL_SLACK = 6;

    globalThis.sumEvlogCapturePanelState = function (container) {
      var out = {};
      if (!container || !container.querySelectorAll) return out;
      var roots = container.querySelectorAll("[data-sum-evlog-root]");
      for (var i = 0; i < roots.length; i++) {
        var root = roots[i];
        var tb = root.querySelector("tbody[data-sum-evlog-tbody]");
        if (!tb || !tb.id) continue;
        var q = "";
        try {
          var inpCap = root.querySelector(".sum-evlog__search");
          q = inpCap && inpCap.value != null ? String(inpCap.value) : "";
        } catch (eInC) {}
        var mode = "all";
        try {
          var stCap = root.querySelector("[data-evlog-filter-status]");
          mode = stCap && stCap.value ? String(stCap.value) : "all";
        } catch (eStC) {}
        var selectedIds = [];
        try {
          var picked = tb.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
          for (var pi = 0; pi < picked.length; pi++) {
            var did = picked[pi].getAttribute("data-evlog-id");
            if (did) selectedIds.push(did);
          }
        } catch (ePk) {}
        var anchorId = null;
        try {
          if (root._sumEvlogAnchor != null && root._sumEvlogAnchor >= 0) {
            var rowsA = tb.querySelectorAll("tr[data-evlog-id]");
            var ar = rowsA[root._sumEvlogAnchor];
            if (ar) anchorId = ar.getAttribute("data-evlog-id");
          }
        } catch (eAnc) {}
        var st = tb.scrollTop;
        var sh = tb.scrollHeight;
        var ch = tb.clientHeight;
        var nearBottom = sh - st - ch <= SUM_EVLOG_TB_TAIL_SLACK;
        out[tb.id] = {
          q: q,
          mode: mode,
          selectedIds: selectedIds,
          anchorId: anchorId,
          scrollTop: st,
          nearBottom: nearBottom
        };
      }
      return out;
    };

    globalThis.sumEvlogApplyPanelState = function (container, saved, opts) {
      opts = opts || {};
      var scrollOnly = !!opts.scrollOnly;
      if (!container || !saved || typeof saved !== "object") return;
      var keys = Object.keys(saved);
      for (var ki = 0; ki < keys.length; ki++) {
        var tid = keys[ki];
        var pack = saved[tid];
        if (!pack || typeof pack !== "object") continue;
        var tb = document.getElementById(tid);
        /* Boolean attribute `data-sum-evlog-tbody` has no value; getAttribute returns "" which is falsy. */
        if (!tb || !tb.hasAttribute || !tb.hasAttribute("data-sum-evlog-tbody")) {
          continue;
        }
        if (!container.contains(tb)) {
          continue;
        }
        var root = tb.closest("[data-sum-evlog-root]");
        if (!root) continue;

        if (scrollOnly) {
          var maxSo = Math.max(0, tb.scrollHeight - tb.clientHeight);
          if (pack.nearBottom) {
            tb.scrollTop = maxSo;
          } else {
            var wantSo = Number(pack.scrollTop);
            if (isNaN(wantSo)) wantSo = 0;
            tb.scrollTop = Math.min(wantSo, maxSo);
          }
          continue;
        }

        var inpAp = root.querySelector(".sum-evlog__search");
        if (inpAp) inpAp.value = pack.q != null ? String(pack.q) : "";
        var stSelAp = root.querySelector("[data-evlog-filter-status]");
        if (stSelAp && pack.mode) stSelAp.value = String(pack.mode);
        sumEvlogRebuildRoot(root);

        var selIds = pack.selectedIds;
        if (selIds && selIds.length) {
          var allR = tb.querySelectorAll("tr[data-evlog-id]");
          for (var ui = 0; ui < allR.length; ui++) {
            allR[ui].classList.remove("sum-evlog__row--selected");
          }
          for (var sj = 0; sj < selIds.length; sj++) {
            var wantId = selIds[sj];
            for (var sk = 0; sk < allR.length; sk++) {
              if (allR[sk].getAttribute("data-evlog-id") === wantId) {
                allR[sk].classList.add("sum-evlog__row--selected");
                break;
              }
            }
          }
        } else {
          var prevSel = tb.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
          for (var u2 = 0; u2 < prevSel.length; u2++) prevSel[u2].classList.remove("sum-evlog__row--selected");
        }

        var aid = pack.anchorId;
        if (aid != null && String(aid) !== "") {
          var rows2 = tb.querySelectorAll("tr[data-evlog-id]");
          root._sumEvlogAnchor = null;
          for (var ax = 0; ax < rows2.length; ax++) {
            if (rows2[ax].getAttribute("data-evlog-id") === aid) {
              root._sumEvlogAnchor = ax;
              break;
            }
          }
        } else {
          root._sumEvlogAnchor = null;
        }

        sumEvlogSyncFooter(root);

        if (opts.scroll !== false) {
          var maxT2 = Math.max(0, tb.scrollHeight - tb.clientHeight);
          if (pack.nearBottom) {
            tb.scrollTop = maxT2;
          } else {
            var want2 = Number(pack.scrollTop);
            if (isNaN(want2)) want2 = 0;
            tb.scrollTop = Math.min(want2, maxT2);
          }
        }
      }
    };
    function parseHttpAttr(attr) {
      if (attr == null || String(attr).trim() === "") return null;
      var n = parseInt(String(attr).trim(), 10);
      return isNaN(n) ? null : n;
    }
    function rowTimespec(tr) {
      var tEl = tr.querySelector("time[datetime]");
      if (!tEl || !tEl.getAttribute("datetime")) return NaN;
      var ms = Date.parse(tEl.getAttribute("datetime"));
      return isNaN(ms) ? NaN : ms;
    }
    function rowSearchBlob(tr) {
      var blob = "";
      try {
        var t = tr.querySelector("time");
        if (t) blob += " " + t.textContent.trim();
        var iso = tr.querySelector("time[datetime]");
        if (iso && iso.getAttribute("datetime")) blob += " " + iso.getAttribute("datetime");
        var msg = tr.querySelector(".sum-evlog__cell--msg");
        if (msg) blob += " " + msg.textContent.trim();
        var stat = tr.querySelector(".sum-evlog__cell--status");
        if (stat) blob += " " + stat.textContent.trim();
        var lk = (tr.getAttribute("data-evlog-level") || "").trim().toLowerCase();
        blob += " " + lk;
      } catch (e0) {}
      return blob.toLowerCase().replace(/\s+/g, " ").trim();
    }
    function rowPassesStatus(tr, mode) {
      var http = parseHttpAttr(tr.getAttribute("data-evlog-http"));
      var lk = (tr.getAttribute("data-evlog-level") || "").trim();
      if (mode === "all") return true;
      if (mode === "warnings") return sumEvlogIsWarnish(lk, http);
      if (mode === "errors") return sumEvlogIsFailish(lk, http);
      return true;
    }
    function ensureSearchEmptyRow(tbody) {
      var existing = tbody.querySelector("[data-sum-evlog-search-empty]");
      if (existing) return existing;
      var tr = document.createElement("tr");
      tr.className = "sum-evlog__row sum-evlog__search-empty-row";
      tr.setAttribute("data-sum-evlog-search-empty", "");
      tr.setAttribute("hidden", "");
      tr.setAttribute("role", "status");
      var td = document.createElement("td");
      td.className = "sum-evlog__search-empty-cell";
      td.colSpan = 3;
      td.appendChild(document.createTextNode("No matching entries for your search. "));
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "sum-evlog__clear-inline-search";
      btn.setAttribute("data-sum-evlog-clear-search", "");
      btn.appendChild(document.createTextNode("Clear search"));
      td.appendChild(btn);
      tr.appendChild(td);
      tbody.insertBefore(tr, tbody.firstChild);
      return tr;
    }
    function ensureDataEmptyRow(tbody) {
      var existing = tbody.querySelector("[data-sum-evlog-data-empty]");
      if (existing) return existing;
      var tr = document.createElement("tr");
      tr.className = "sum-evlog__row sum-evlog__search-empty-row";
      tr.setAttribute("data-sum-evlog-data-empty", "");
      tr.setAttribute("hidden", "");
      tr.setAttribute("role", "status");
      var td = document.createElement("td");
      td.className = "sum-evlog__search-empty-cell";
      td.colSpan = 3;
      td.appendChild(document.createTextNode("No events to display"));
      tr.appendChild(td);
      tbody.appendChild(tr);
      return tr;
    }
    function sumEvlogSyncFooter(root) {
      var foot = root.querySelector("[data-sum-evlog-oldest]");
      var footLeft = root.querySelector(".sum-evlog__footer-left");
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!foot || !tbody) return;
      var picked = tbody.querySelector(
        "tr[data-evlog-id].sum-evlog__row--selected:not(.sum-evlog__row--hidden)"
      );
      var visibleRows = tbody.querySelectorAll("tr[data-evlog-id]:not(.sum-evlog__row--hidden)");
      if (picked) {
        var msSel = rowTimespec(picked);
        var absSel = formatLogDateTimeLocal(msSel);
        var relSel = formatLogRelativeAgo(msSel);
        var tEl = picked.querySelector("time[datetime]");
        var dtAttr = tEl && tEl.getAttribute("datetime") ? tEl.getAttribute("datetime") : "";
        if (!dtAttr && isFinite(msSel)) {
          try {
            dtAttr = new Date(msSel).toISOString();
          } catch (eIso) {
            dtAttr = "";
          }
        }
        var timeOpen = dtAttr
          ? '<time datetime="' + escapeHtml(dtAttr) + '" title="' + escapeHtml(relSel) + '">'
          : '<time title="' + escapeHtml(relSel) + '">';
        foot.innerHTML =
          "Selected entry: " +
          timeOpen +
          escapeHtml(absSel) +
          "</time> <span class=\"sum-evlog__footer-rel\">(" +
          escapeHtml(relSel) +
          ")</span>";
        if (footLeft) footLeft.hidden = false;
        return;
      }
      if (footLeft) footLeft.hidden = visibleRows.length === 0;
      if (visibleRows.length === 0) return;
      var oldestMs = root._sumEvlogOldestVisible;
      foot.innerHTML =
        "Oldest <strong>visible</strong> entry: <time title=\"" +
        escapeHtml(formatLogRelativeAgo(oldestMs)) +
        "\">" +
        escapeHtml(formatLogDateTimeLocal(isFinite(oldestMs) ? oldestMs : null)) +
        "</time>";
    }
    function sumEvlogRebuildRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!tbody) return;
      var select = root.querySelector("[data-evlog-filter-status]");
      var mode = select && select.value ? select.value : "all";
      var q = "";
      try {
        var inp = root.querySelector(".sum-evlog__search");
        q = inp && inp.value ? String(inp.value).trim().toLowerCase() : "";
      } catch (eIn) {}
      var rows = Array.prototype.slice.call(tbody.querySelectorAll("tr[data-evlog-id]"), 0);
      var oldest = Infinity;
      var visibleCount = 0;
      var i;
      for (i = 0; i < rows.length; i++) {
        var tr = rows[i];
        var show =
          rowPassesStatus(tr, mode) &&
          (q === "" || rowSearchBlob(tr).indexOf(q) !== -1);
        tr.classList.toggle("sum-evlog__row--hidden", !show);
        if (show) {
          visibleCount++;
          var ts = rowTimespec(tr);
          if (isFinite(ts) && ts < oldest) oldest = ts;
        }
      }
      var searchEmptyRow = tbody.querySelector("[data-sum-evlog-search-empty]");
      if (searchEmptyRow) {
        searchEmptyRow.hidden = !(q !== "" && visibleCount === 0);
      }
      var dataEmptyRow = ensureDataEmptyRow(tbody);
      if (dataEmptyRow) {
        dataEmptyRow.hidden = rows.length > 0;
      }
      root._sumEvlogOldestVisible = oldest === Infinity ? NaN : oldest;
      sumEvlogSyncFooter(root);
    }
    function sumEvlogCopyFromRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      var toast = root.querySelector("[data-sum-evlog-toast]");
      if (!tbody) return;
      var lines = [];
      var picked = tbody.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
      var allVisible = false;
      if (picked.length === 0) {
        picked = tbody.querySelectorAll("tr[data-evlog-id]:not(.sum-evlog__row--hidden)");
        allVisible = true;
      }
      for (var i = 0; i < picked.length; i++) {
        var tr = picked[i];
        var t = tr.querySelector("time");
        var timeStr = t ? t.textContent.trim() : "";
        var msg = tr.querySelector(".sum-evlog__cell--msg");
        var msgStr = msg ? msg.textContent.trim().replace(/\s+/g, " ") : "";
        var stat = tr.querySelector(".sum-evlog__cell--status");
        var statStr = stat ? stat.textContent.trim().replace(/\s+/g, " ") : "";
        lines.push(timeStr + "\t" + msgStr + "\t" + statStr);
      }
      var text = lines.join("\n");
      function showToast(ok, msg) {
        if (!toast) return;
        toast.classList.toggle("sum-evlog__toast--error", !ok);
        toast.textContent = msg;
      }
      if (!text) {
        showToast(false, allVisible ? "No visible rows." : "No rows selected.");
        return;
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(
          function () {
            var suffix = allVisible ? " visible line(s)." : " line(s).";
            showToast(true, "Copied " + lines.length + suffix);
          },
          function () {
            showToast(false, "Clipboard blocked.");
          }
        );
      } else {
        showToast(false, "Clipboard API unavailable in this browser.");
      }
    }
    function sumEvlogHydrateRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!tbody) return;
      ensureSearchEmptyRow(tbody);
      ensureDataEmptyRow(tbody);
      sumEvlogRebuildRoot(root);
    }
    globalThis.sumEvlogHydrateAllIn = function (container) {
      if (!container || !container.querySelectorAll) return;
      var roots = container.querySelectorAll("[data-sum-evlog-root]");
      for (var r = 0; r < roots.length; r++) {
        sumEvlogHydrateRoot(roots[r]);
      }
    };
    function debounceSearch(root) {
      if (searchTimers && searchTimers.get) {
        var prev = searchTimers.get(root);
        if (prev) window.clearTimeout(prev);
        searchTimers.set(
          root,
          window.setTimeout(function () {
            searchTimers.delete(root);
            sumEvlogRebuildRoot(root);
          }, 120)
        );
      } else {
        window.setTimeout(function () {
          sumEvlogRebuildRoot(root);
        }, 120);
      }
    }
    document.body.addEventListener(
      "input",
      function (ev) {
        var el = ev.target;
        if (!el || !el.classList || !el.classList.contains("sum-evlog__search")) return;
        var root = el.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        debounceSearch(root);
      },
      false
    );
    document.body.addEventListener(
      "change",
      function (ev) {
        var el = ev.target;
        if (!el || !el.closest) return;
        var root = el.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        if (el.matches("[data-evlog-filter-status]")) {
          sumEvlogRebuildRoot(root);
        }
      },
      false
    );
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var root = t.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        if (t.closest(".sum-evlog__copy-btn")) {
          ev.preventDefault();
          sumEvlogCopyFromRoot(root);
          return;
        }
        var clr = t.closest("[data-sum-evlog-clear-search]");
        if (clr) {
          ev.preventDefault();
          var inp = root.querySelector(".sum-evlog__search");
          if (inp) inp.value = "";
          sumEvlogRebuildRoot(root);
          if (inp) inp.focus();
          return;
        }
        var tr = t.closest("tr[data-evlog-id]");
        if (!tr || !root.contains(tr)) return;
        var tbody = tr.parentNode;
        if (!tbody || !tbody.hasAttribute || !tbody.hasAttribute("data-sum-evlog-tbody")) return;
        var rows = Array.prototype.slice.call(tbody.querySelectorAll("tr[data-evlog-id]"), 0);
        function rowIndex(rowsArr, trg) {
          for (var ri = 0; ri < rowsArr.length; ri++) {
            if (rowsArr[ri] === trg) return ri;
          }
          return -1;
        }
        function clearSel() {
          var sel = tbody.querySelectorAll(".sum-evlog__row--selected");
          for (var si = 0; si < sel.length; si++) sel[si].classList.remove("sum-evlog__row--selected");
        }
        function setRange(lo, hi, on) {
          var j;
          for (j = lo; j <= hi && j < rows.length; j++) {
            if (on) rows[j].classList.add("sum-evlog__row--selected");
            else rows[j].classList.remove("sum-evlog__row--selected");
          }
        }
        var idx = rowIndex(rows, tr);
        if (idx < 0) return;
        if (ev.shiftKey && root._sumEvlogAnchor != null && root._sumEvlogAnchor >= 0) {
          clearSel();
          var a = root._sumEvlogAnchor;
          setRange(Math.min(a, idx), Math.max(a, idx), true);
          sumEvlogSyncFooter(root);
          return;
        }
        if (ev.ctrlKey || ev.metaKey) {
          tr.classList.toggle("sum-evlog__row--selected");
          root._sumEvlogAnchor = idx;
          sumEvlogSyncFooter(root);
          return;
        }
        clearSel();
        tr.classList.add("sum-evlog__row--selected");
        root._sumEvlogAnchor = idx;
        sumEvlogSyncFooter(root);
      },
      false
    );
    document.body.addEventListener(
      "pointerdown",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var root = t.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        ctx.sumEvlogPointerSuppressedUntil = Date.now() + 480;
      },
      true
    );
    document.body.addEventListener(
      "pointerdown",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var summary = t.closest("summary");
        if (!summary) return;
        var card = summary.closest("details.sum-card");
        if (!card || summary.parentElement !== card || !card.closest("#panel-summarized")) return;
        ctx.sumEvlogPointerSuppressedUntil = Date.now() + 480;
      },
      true
    );
    document.body.addEventListener(
      "focusout",
      function (ev) {
        var t = ev.target;
        if (!t || !t.closest) return;
        if (!t.closest("#panel-summarized")) return;
        var isSearch = t.classList && t.classList.contains("sum-evlog__search");
        var isStat = t.matches && t.matches("[data-evlog-filter-status]");
        if (!isSearch && !isStat) return;
        if (ctx.sumEvlogUiDeferTimer) {
          clearTimeout(ctx.sumEvlogUiDeferTimer);
          ctx.sumEvlogUiDeferTimer = null;
        }
        if (!ctx.getViewMode || ctx.getViewMode() === "summarized") scheduleStoryRebuild();
      },
      true
    );
  })();

  (function wireWorkspaceDraftUi() {
    if (globalThis.__chimeraLogsWorkspaceDraftUiWired) return;
    globalThis.__chimeraLogsWorkspaceDraftUiWired = true;
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        if (t.closest("[data-sum-workspaces-create]")) {
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
            var stA = ctx.workspaceManagedStaging.paths;
            var startDirA = stA && stA.length ? stA[stA.length - 1].path : "";
            pickFolderForWorkspaceDraft(startDirA).then(function (picked) {
              if (!picked) return;
              ctx.workspaceManagedStaging.paths.push({ id: null, path: String(picked).trim() });
              scheduleStoryRebuild();
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
          var rmBtM = cardManagedCh.querySelector(".ws-managed-btn-remove");
          if (rmBtM)
            rmBtM.disabled =
              el.selectedIndex < 0 || !el.options || !el.options.length;
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
  })();

  (function wireLogsChromeLinks() {
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var ext = t.closest("a.sum-ext-link");
        if (ext) {
          var href = ext.getAttribute("href") || "";
          if (/^https?:\/\//i.test(href)) {
            ev.preventDefault();
            ev.stopPropagation();
            if (typeof globalThis.chimeraOpenExternalURL === "function") {
              try {
                var ret = globalThis.chimeraOpenExternalURL(href);
                if (ret && typeof ret.then === "function") ret.catch(function () { });
              } catch (x) { }
            } else {
              try {
                window.open(href, "_blank", "noopener,noreferrer");
              } catch (x2) { }
            }
          }
          return;
        }
        var proj = t.closest("a.sum-proj-path");
        if (proj) {
          ev.preventDefault();
          ev.stopPropagation();
          var rel = proj.getAttribute("data-rel") || "";
          if (!rel) rel = proj.textContent || "";
          rel = String(rel).replace(/\s+/g, " ").trim();
          if (!rel) return;
          if (typeof globalThis.chimeraRevealProjectPath === "function") {
            try {
              var ret2 = globalThis.chimeraRevealProjectPath(rel);
              if (ret2 && typeof ret2.then === "function") ret2.catch(function () { });
            } catch (x3) { }
          }
          return;
        }
      },
      true
    );
  })();

  (function wireAdminWorkflowCards() {
    function setAdminSaveBtnPending(btn, pending) {
      if (!btn) return;
      btn.disabled = !!pending;
      if (pending) btn.setAttribute("aria-disabled", "true");
      else btn.removeAttribute("aria-disabled");
    }

    function syncYamlOverlayVScrollFromTarget(t) {
      if (!t || String(t.tagName || "").toLowerCase() !== "textarea") return;
      var wrap = t.closest && t.closest(".sg-op-yaml-wrap");
      if (!wrap) return;
      wrap.classList.toggle("sg-op-yaml-wrap--vscroll", t.scrollHeight > t.clientHeight + 1);
    }

    function applyRoutingPolicyDraftToEditor() {
      var y = document.getElementById("admin-routing-yaml");
      if (!y) return;
      y.value = String(ctx.routingPolicyDraft != null ? ctx.routingPolicyDraft : "");
      var savedPolicy = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
      ctx.routingPolicyTouched = String(y.value) !== savedPolicy;
      var wrap = document.getElementById("admin-routing-policy-wrap");
      if (wrap) wrap.classList.toggle("sg-op-yaml-wrap--dirty", !!ctx.routingPolicyTouched);
      syncYamlOverlayVScrollFromTarget(y);
    }

    document.body.addEventListener("input", function (ev) {
      var t = ev.target;
      if (!t || !t.id) return;
      if (t.id === "admin-routing-yaml") {
        ctx.routingPolicyDraft = t.value != null ? String(t.value) : "";
        var savedPolicy = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
        ctx.routingPolicyTouched = String(ctx.routingPolicyDraft) !== savedPolicy;
        var routingWrap = document.getElementById("admin-routing-policy-wrap");
        if (routingWrap) routingWrap.classList.toggle("sg-op-yaml-wrap--dirty", !!ctx.routingPolicyTouched);
      }
      else if (t.id === "admin-fallback-yaml") {
        ctx.fallbackTouched = true;
        var fallbackWrap = document.getElementById("admin-fallback-yaml-wrap");
        if (fallbackWrap) fallbackWrap.classList.add("sg-op-yaml-wrap--dirty");
      }
      else if (t.id === "admin-router-models-yaml") {
        ctx.routerModelsTouched = true;
        ctx.routerModelsDraft = t.value != null ? String(t.value) : "";
        var routerWrap = document.getElementById("admin-router-models-wrap");
        if (routerWrap) routerWrap.classList.add("sg-op-yaml-wrap--dirty");
      }
      else if (t.id === "admin-router-threshold") {
        ctx.routerThresholdTouched = true;
        ctx.routerThresholdDraft = t.value != null ? String(t.value) : "";
      } else if (t.id === "admin-groq-key") {
        if (!ctx.adminProviderKeyDraft) ctx.adminProviderKeyDraft = { groq: null, gemini: null };
        ctx.adminProviderKeyDraft.groq = t.value != null ? String(t.value) : "";
      } else if (t.id === "admin-gemini-key") {
        if (!ctx.adminProviderKeyDraft) ctx.adminProviderKeyDraft = { groq: null, gemini: null };
        ctx.adminProviderKeyDraft.gemini = t.value != null ? String(t.value) : "";
      } else if (t.id === "admin-ollama-url") {
        ctx.adminOllamaUrlDraft = t.value != null ? String(t.value) : "";
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

      function reloadAdmin() {
        Promise.all([fetchAdminState(), fetchAdminTokens()]).then(function () {
          if (typeof ctx.patchAdminCardsFromPoll === "function" && ctx.patchAdminCardsFromPoll()) return;
          refreshSummarizedPanel();
        });
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

      if (act === "fallback-configure") {
        ctx.adminFallbackEditing = true;
        refreshSummarizedPanel();
        return;
      }

      if (act === "routing-configure") {
        ctx.adminRoutingEditing = true;
        if (ctx.routingPolicyDraft == null) ctx.routingPolicyDraft = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
        refreshSummarizedPanel();
        return;
      }

      if (act === "routing-cancel") {
        ctx.adminRoutingEditing = false;
        ctx.routingPolicyTouched = false;
        ctx.routingPolicyDraft = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
        refreshSummarizedPanel();
        return;
      }

      if (act === "router-configure") {
        ctx.adminRouterEditing = true;
        refreshSummarizedPanel();
        return;
      }

      if (act === "router-cancel") {
        ctx.adminRouterEditing = false;
        ctx.routerModelsTouched = false;
        ctx.routerModelsDraft = null;
        ctx.routerThresholdTouched = false;
        ctx.routerThresholdDraft = null;
        ctx.routerEnabledTouched = false;
        ctx.routerEnabledDraft = null;
        refreshSummarizedPanel();
        return;
      }

      if (act === "fallback-cancel") {
        ctx.adminFallbackEditing = false;
        ctx.fallbackTouched = false;
        refreshSummarizedPanel();
        return;
      }

      if (act === "routing-policy-refresh") {
        fetchAdminState()
          .catch(function () {})
          .then(function () {
            var saved = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
            ctx.routingPolicyDraft = saved;
            applyRoutingPolicyDraftToEditor();
          });
        return;
      }

      if (act === "fallback-refresh") {
        ctx.fallbackTouched = false;
        refreshSummarizedPanel();
        return;
      }

      if (act === "router-models-refresh") {
        ctx.routerModelsTouched = false;
        ctx.routerModelsDraft = null;
        refreshSummarizedPanel();
        return;
      }

      if (act === "router-enabled-toggle") {
        var toggleEl = t;
        if (!toggleEl || !toggleEl.getAttribute || !toggleEl.classList || !toggleEl.classList.contains("sum-router-toggle")) {
          toggleEl = t.closest && t.closest(".sum-router-toggle");
        }
        if (!toggleEl || !toggleEl.getAttribute) return;
        var nextPressed = String(toggleEl.getAttribute("aria-pressed") || "").toLowerCase() !== "true";
        var savedModels = Array.isArray((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).router_models))
          ? (((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).router_models)
          : [];
        var savedThr = parseFloat(String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).tool_router_confidence_threshold) || "0.5"));
        if (isNaN(savedThr) || savedThr < 0 || savedThr > 1) savedThr = 0.5;
        adminPostJSON("/api/ui/routing/router_tooling", {
          router_models: savedModels,
          tool_router_enabled: nextPressed,
          confidence_threshold: savedThr
        })
          .then(function () {
            ctx.routerEnabledTouched = false;
            ctx.routerEnabledDraft = null;
            adminSetMessage("", "Tool router " + (nextPressed ? "enabled." : "disabled."));
            reloadAdmin();
          })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
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
        var prov = String(t.getAttribute("data-provider") || "");
        var inputId = prov === "groq" ? "admin-groq-key" : prov === "gemini" ? "admin-gemini-key" : "";
        var val = inputId ? ((document.getElementById(inputId) || {}).value || "") : "";
        if (!val.trim()) {
          adminSetMessage("err", "Enter a key.");
          return;
        }
        setAdminSaveBtnPending(t, true);
        adminPostJSON("/api/ui/provider/" + prov + "/keys", { value: String(val).trim() })
          .then(function () {
            var inp = document.getElementById(inputId);
            if (inp) inp.value = "";
            if (ctx.adminProviderKeyDraft) {
              if (prov === "groq") ctx.adminProviderKeyDraft.groq = null;
              if (prov === "gemini") ctx.adminProviderKeyDraft.gemini = null;
            }
            adminSetMessage("", "Provider key added.");
            reloadAdmin();
          })
          .catch(function (e) {
            setAdminSaveBtnPending(t, false);
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
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
        var baseURL = ((document.getElementById("admin-ollama-url") || {}).value || "").trim();
        if (!baseURL) {
          adminSetMessage("err", "Enter a URL.");
          return;
        }
        setAdminSaveBtnPending(t, true);
        adminPostJSON("/api/ui/provider/ollama/base_url", { base_url: baseURL })
          .then(function () {
            ctx.adminOllamaUrlDraft = null;
            adminSetMessage("", "Ollama URL saved.");
            reloadAdmin();
          })
          .catch(function (e) {
            setAdminSaveBtnPending(t, false);
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
        return;
      }

      if (act === "routing-generate") {
        adminPostJSON("/api/ui/routing/preview", {})
          .then(function (j) {
            var savedPolicy = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
            ctx.routingPolicyDraft = String((j && j.routing_policy_yaml) || "");
            ctx.routingPolicyTouched = String(ctx.routingPolicyDraft) !== savedPolicy;
            adminSetMessage("", "Routing preview generated. Save to apply.");
            refreshSummarizedPanel();
          })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        return;
      }

      if (act === "routing-evaluate") {
        var policyYAML = ((document.getElementById("admin-routing-yaml") || {}).value || "");
        if (!String(policyYAML).trim()) {
          policyYAML = String((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).routing_policy_yaml) || "");
        }
        var fbRaw = ((document.getElementById("admin-fallback-yaml") || {}).value || "");
        if (!String(fbRaw).trim()) {
          var fc = (((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).fallback_chain) || [];
          fbRaw = fallbackChainToYAML(fc);
        }
        var fallbackList;
        try {
          fallbackList = parseFallbackChainInput(fbRaw);
        } catch (e) {
          adminSetMessage("err", "Fallback chain: " + (e && e.message ? e.message : String(e)));
          return;
        }
        if (!fallbackList.length) {
          adminSetMessage("err", "Fallback chain: add at least one model id.");
          return;
        }
        var evalMsg = String(((document.getElementById("admin-routing-eval-msg") || {}).value || ""));
        var evalSmoke = !!((document.getElementById("admin-routing-eval-smoke") || {}).checked);
        var outEl = document.getElementById("admin-routing-eval-out");
        adminPostJSON("/api/ui/routing/evaluate", {
          routing_policy_yaml: policyYAML,
          fallback_chain: fallbackList,
          messages: [{ role: "user", content: evalMsg }],
          smoke_completion: evalSmoke
        })
          .then(function (j) {
            if (outEl) outEl.textContent = JSON.stringify(j, null, 2);
            adminSetMessage("", "Dry-run complete.");
          })
          .catch(function (e) {
            if (outEl) outEl.textContent = "";
            adminSetMessage("err", e && e.message ? e.message : String(e));
          });
        return;
      }

      if (act === "routing-policy-save") {
        var policyYAML = ((document.getElementById("admin-routing-yaml") || {}).value || "");
        if (!String(policyYAML).trim()) {
          adminSetMessage("err", "Routing policy YAML is required.");
          return;
        }
        adminPostJSON("/api/ui/routing/policy", { routing_policy_yaml: policyYAML })
          .then(function () {
            ctx.routingPolicyTouched = false;
            ctx.routingPolicyDraft = null;
            ctx.adminRoutingEditing = false;
            adminSetMessage("", "Routing policy saved.");
            reloadAdmin();
          })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        return;
      }

      if (act === "fallback-save") {
        try {
          var fallbackList = parseFallbackChainInput(((document.getElementById("admin-fallback-yaml") || {}).value || ""));
          adminPostJSON("/api/ui/routing/fallback_chain", { fallback_chain: fallbackList })
            .then(function () {
              ctx.fallbackTouched = false;
              ctx.adminFallbackEditing = false;
              adminSetMessage("", "Fallback chain saved.");
              reloadAdmin();
            })
            .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        } catch (e) {
          adminSetMessage("err", e && e.message ? e.message : String(e));
        }
        return;
      }

      if (act === "router-save") {
        try {
          var modelsRaw = ((document.getElementById("admin-router-models-yaml") || {}).value || "");
          if (!String(modelsRaw).trim() && ctx.routerModelsTouched && ctx.routerModelsDraft != null) modelsRaw = String(ctx.routerModelsDraft);
          if (!String(modelsRaw).trim()) modelsRaw = fallbackChainToYAML((((ctx.adminStateCache && ctx.adminStateCache.gateway) || {}).router_models) || []);
          var models = parseFallbackChainInput(modelsRaw);
          var thr = parseFloat(String(((document.getElementById("admin-router-threshold") || {}).value || "0.5"), 10));
          if (isNaN(thr) || thr < 0 || thr > 1) throw new Error("Threshold must be a number between 0 and 1.");
          var routerEnabledBtn = document.getElementById("admin-router-enabled");
          var enabled = String((routerEnabledBtn && routerEnabledBtn.getAttribute && routerEnabledBtn.getAttribute("aria-pressed")) || "").toLowerCase() === "true";
          adminPostJSON("/api/ui/routing/router_tooling", {
            router_models: models,
            tool_router_enabled: enabled,
            confidence_threshold: thr
          })
            .then(function () {
              ctx.routerModelsTouched = false;
              ctx.routerModelsDraft = null;
              ctx.routerThresholdTouched = false;
              ctx.routerThresholdDraft = null;
              ctx.routerEnabledTouched = false;
              ctx.routerEnabledDraft = null;
              ctx.adminRouterEditing = false;
              adminSetMessage("", "Router settings saved.");
              reloadAdmin();
            })
            .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
        } catch (e) {
          adminSetMessage("err", e && e.message ? e.message : String(e));
        }
        return;
      }

      if (act === "routing-free-tier-toggle") {
        var curPressed = String(t.getAttribute("aria-pressed") || "").toLowerCase() === "true";
        var nextEnabled = !curPressed;
        adminPostJSON("/api/ui/routing/filter_free_tier_models", { enabled: nextEnabled })
          .then(function () {
            adminSetMessage("", "Free-tier filter updated.");
            reloadAdmin();
          })
          .catch(function (e) { adminSetMessage("err", e && e.message ? e.message : String(e)); });
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
      var textareas = document.querySelectorAll(".sg-op-yaml-wrap textarea");
      for (var i = 0; i < textareas.length; i++) {
        syncYamlOverlayVScrollFromTarget(textareas[i]);
      }
    });
  })();
};

