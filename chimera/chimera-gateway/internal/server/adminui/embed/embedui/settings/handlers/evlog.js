/**
 * Summarized event-log panels: search, filter, row selection, copy.
 * Exports: ChimeraSettings.Handlers.Evlog.wire(ctx)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Handlers = globalThis.ChimeraSettings.Handlers || {};
globalThis.ChimeraSettings.Handlers.Evlog = globalThis.ChimeraSettings.Handlers.Evlog || {};

globalThis.ChimeraSettings.Handlers.Evlog.wire = function (ctx) {
  if (globalThis.__ChimeraSettingsSumEvlogWired) return;
  globalThis.__ChimeraSettingsSumEvlogWired = true;
  var scheduleStoryRebuild = ctx.scheduleStoryRebuild;
  var sumEvlogIsWarnish = ctx.sumEvlogIsWarnish;
  var sumEvlogIsFailish = ctx.sumEvlogIsFailish;
  var formatLogDateTimeLocal = ctx.formatLogDateTimeLocal;
  var formatLogRelativeAgo = ctx.formatLogRelativeAgo;
  var escapeHtml = ctx.escapeHtml;
    var searchTimers = typeof WeakMap !== "undefined" ? new WeakMap() : null;
    /** Pixels from tbody bottom to treat as "stuck to tail" when summarized panel is rebuilt. */
    var SUM_EVLOG_TB_TAIL_SLACK = 6;
    var SUM_EVLOG_TBODY_MIN_PX = 64;
    var SUM_EVLOG_TBODY_MAX_PX = 720;

    function sumEvlogTbodyEl(root) {
      return root ? root.querySelector("[data-sum-evlog-tbody]") : null;
    }
    function sumEvlogDefaultTbodyHeightPx(root) {
      var tb = sumEvlogTbodyEl(root);
      if (!tb) return 176;
      var cs = window.getComputedStyle(tb);
      var mh = cs && cs.maxHeight ? cs.maxHeight : "";
      if (mh && mh !== "none") {
        var n = parseFloat(mh);
        if (isFinite(n) && n > 0) return n;
      }
      return root && root.classList && root.classList.contains("sum-evlog--in-card") ? 176 : 256;
    }
    function sumEvlogReadTbodyHeightPx(root) {
      if (!root) return null;
      if (root._sumEvlogTbodyHeightPx != null && isFinite(root._sumEvlogTbodyHeightPx)) {
        return root._sumEvlogTbodyHeightPx;
      }
      var tb = sumEvlogTbodyEl(root);
      if (!tb || !tb.style || !tb.style.maxHeight) return null;
      var inline = parseFloat(tb.style.maxHeight);
      return isFinite(inline) && inline > 0 ? inline : null;
    }
    function sumEvlogClampTbodyHeightPx(px) {
      var n = Number(px);
      if (!isFinite(n)) n = SUM_EVLOG_TBODY_MIN_PX;
      var cap = SUM_EVLOG_TBODY_MAX_PX;
      try {
        if (window.innerHeight) cap = Math.min(cap, Math.round(window.innerHeight * 0.72));
      } catch (eCap) {}
      return Math.max(SUM_EVLOG_TBODY_MIN_PX, Math.min(cap, Math.round(n)));
    }
    function sumEvlogApplyTbodyHeightPx(root, px, opts) {
      opts = opts || {};
      if (!root) return;
      var tb = sumEvlogTbodyEl(root);
      if (!tb) return;
      var next = sumEvlogClampTbodyHeightPx(px != null ? px : sumEvlogDefaultTbodyHeightPx(root));
      tb.style.maxHeight = next + "px";
      root._sumEvlogTbodyHeightPx = next;
      if (opts.persist !== false) root.setAttribute("data-sum-evlog-tbody-height", String(next));
    }

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
        var tbHeightPx = sumEvlogReadTbodyHeightPx(root);
        out[tb.id] = {
          q: q,
          mode: mode,
          selectedIds: selectedIds,
          anchorId: anchorId,
          scrollTop: st,
          nearBottom: nearBottom,
          tbodyHeightPx: tbHeightPx
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

        if (pack.tbodyHeightPx != null && isFinite(Number(pack.tbodyHeightPx))) {
          sumEvlogApplyTbodyHeightPx(root, Number(pack.tbodyHeightPx));
        } else {
          var savedAttr = root.getAttribute("data-sum-evlog-tbody-height");
          if (savedAttr) {
            var savedPx = parseFloat(savedAttr);
            if (isFinite(savedPx) && savedPx > 0) sumEvlogApplyTbodyHeightPx(root, savedPx);
          }
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
    function sumEvlogPanelColCount(root) {
      if (!root) return 3;
      var attr = root.getAttribute("data-sum-evlog-cols");
      if (attr) {
        var n = parseInt(String(attr), 10);
        if (!isNaN(n) && n >= 3) return n;
      }
      return root.hasAttribute("data-sum-evlog-source") ? 4 : 3;
    }
    function rowSearchBlob(tr) {
      var blob = "";
      try {
        var t = tr.querySelector("time");
        if (t) blob += " " + t.textContent.trim();
        var iso = tr.querySelector("time[datetime]");
        if (iso && iso.getAttribute("datetime")) blob += " " + iso.getAttribute("datetime");
        var src = tr.querySelector(".sum-evlog__cell--source");
        if (src) blob += " " + src.textContent.trim();
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
      var root = tbody.closest("[data-sum-evlog-root]");
      var colspan = sumEvlogPanelColCount(root);
      var tr = document.createElement("tr");
      tr.className = "sum-evlog__row sum-evlog__search-empty-row";
      tr.setAttribute("data-sum-evlog-search-empty", "");
      tr.setAttribute("hidden", "");
      tr.setAttribute("role", "status");
      var td = document.createElement("td");
      td.className = "sum-evlog__search-empty-cell";
      td.colSpan = colspan;
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
      var root = tbody.closest("[data-sum-evlog-root]");
      var colspan = sumEvlogPanelColCount(root);
      var tr = document.createElement("tr");
      tr.className = "sum-evlog__row sum-evlog__search-empty-row";
      tr.setAttribute("data-sum-evlog-data-empty", "");
      tr.setAttribute("hidden", "");
      tr.setAttribute("role", "status");
      var td = document.createElement("td");
      td.className = "sum-evlog__search-empty-cell";
      td.colSpan = colspan;
      td.appendChild(document.createTextNode("No events to display"));
      tr.appendChild(td);
      tbody.appendChild(tr);
      return tr;
    }
    function sumEvlogMetricCount(btn) {
      if (!btn) return 0;
      var numEl = btn.querySelector(".sum-evlog-metric-num");
      if (!numEl) return 0;
      var n = parseInt(String(numEl.textContent || "").trim(), 10);
      return isNaN(n) ? 0 : n;
    }
    function sumEvlogSyncMetricFilterState(root) {
      var select = root.querySelector("[data-evlog-filter-status]");
      var mode = select && select.value ? select.value : "all";
      var wBtn = root.querySelector("[data-sum-evlog-metric-warn]");
      var fBtn = root.querySelector("[data-sum-evlog-metric-fail]");
      if (wBtn) {
        var wCount = sumEvlogMetricCount(wBtn);
        var wDisabled = wCount <= 0;
        wBtn.disabled = wDisabled;
        wBtn.title = wDisabled ? "No warnings" : "Show warnings (click again for all)";
        var wOn = !wDisabled && mode === "warnings";
        wBtn.classList.toggle("sum-evlog-metric-group--active", wOn);
        wBtn.setAttribute("aria-pressed", wOn ? "true" : "false");
      }
      if (fBtn) {
        var fCount = sumEvlogMetricCount(fBtn);
        var fDisabled = fCount <= 0;
        fBtn.disabled = fDisabled;
        fBtn.title = fDisabled ? "No errors" : "Show errors (click again for all)";
        var fOn = !fDisabled && mode === "errors";
        fBtn.classList.toggle("sum-evlog-metric-group--active", fOn);
        fBtn.setAttribute("aria-pressed", fOn ? "true" : "false");
      }
    }
    function sumEvlogToggleStatusFilter(root, wantMode) {
      var select = root.querySelector("[data-evlog-filter-status]");
      if (!select) return;
      var btn = root.querySelector(
        wantMode === "warnings" ? "[data-sum-evlog-metric-warn]" : "[data-sum-evlog-metric-fail]"
      );
      if (sumEvlogMetricCount(btn) <= 0) return;
      var cur = select.value ? select.value : "all";
      select.value = cur === wantMode ? "all" : wantMode;
      sumEvlogRebuildRoot(root);
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
      sumEvlogSyncMetricFilterState(root);
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
        var src = tr.querySelector(".sum-evlog__cell--source");
        var srcStr = src ? src.textContent.trim().replace(/\s+/g, " ") : "";
        var msg = tr.querySelector(".sum-evlog__cell--msg");
        var msgStr = msg ? msg.textContent.trim().replace(/\s+/g, " ") : "";
        var stat = tr.querySelector(".sum-evlog__cell--status");
        var statStr = stat ? stat.textContent.trim().replace(/\s+/g, " ") : "";
        var cols = [timeStr, msgStr];
        if (src) cols.push(srcStr);
        cols.push(statStr);
        lines.push(cols.join("\t"));
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
      var savedAttr = root.getAttribute("data-sum-evlog-tbody-height");
      if (savedAttr) {
        var savedPx = parseFloat(savedAttr);
        if (isFinite(savedPx) && savedPx > 0) sumEvlogApplyTbodyHeightPx(root, savedPx);
      }
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
        if (t.closest("[data-sum-evlog-metric-warn]")) {
          ev.preventDefault();
          sumEvlogToggleStatusFilter(root, "warnings");
          return;
        }
        if (t.closest("[data-sum-evlog-metric-fail]")) {
          ev.preventDefault();
          sumEvlogToggleStatusFilter(root, "errors");
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
        var handle = t.closest("[data-sum-evlog-resize-handle]");
        if (!handle) return;
        var root = handle.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        ev.preventDefault();
        ev.stopPropagation();
        var tb = sumEvlogTbodyEl(root);
        if (!tb) return;
        var startY = ev.clientY;
        var startH = sumEvlogReadTbodyHeightPx(root);
        if (startH == null) startH = sumEvlogDefaultTbodyHeightPx(root);
        root.classList.add("sum-evlog--resizing");
        try {
          handle.setPointerCapture(ev.pointerId);
        } catch (eCap0) {}
        function onMove(eMove) {
          var dy = eMove.clientY - startY;
          sumEvlogApplyTbodyHeightPx(root, startH + dy);
        }
        function onUp(eUp) {
          root.classList.remove("sum-evlog--resizing");
          try {
            handle.releasePointerCapture(eUp.pointerId);
          } catch (eCap1) {}
          handle.removeEventListener("pointermove", onMove);
          handle.removeEventListener("pointerup", onUp);
          handle.removeEventListener("pointercancel", onUp);
        }
        handle.addEventListener("pointermove", onMove);
        handle.addEventListener("pointerup", onUp);
        handle.addEventListener("pointercancel", onUp);
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
        if (!t.closest("#panel-summarized")) return;
        var summary = t.closest("summary");
        if (summary) {
          var detCard = summary.closest("details.sum-card");
          if (detCard && summary.parentElement === detCard) {
            ctx.sumEvlogPointerSuppressedUntil = Date.now() + 480;
            return;
          }
        }
        var hdr = t.closest(".sum-card__hdr");
        if (hdr) {
          var artCard = hdr.closest("article.sum-card--collapsible");
          if (artCard && hdr.parentElement === artCard) {
            ctx.sumEvlogPointerSuppressedUntil = Date.now() + 480;
          }
        }
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
};
