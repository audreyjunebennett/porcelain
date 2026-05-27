/**
 * Gallery-only: event log demos — formatLogDateTimeLocal, status + search filters,
 * click/multi-select rows, clipboard copy, search-no-results row in the table. Phase 2 lives in logs.js.
 */
(function () {
  "use strict";

  function pad2(n) {
    return n < 10 ? "0" + n : String(n);
  }

  /** Same semantics as ChimeraSettings.Main formatLogDateTimeLocal (UTC instant → viewer-local wall time). */
  function formatLogDateTimeLocal(ms) {
    if (ms == null || !isFinite(Number(ms))) return "—";
    var d = new Date(Number(ms));
    if (isNaN(d.getTime())) return "—";
    return (
      d.getFullYear() +
      "-" +
      pad2(d.getMonth() + 1) +
      "-" +
      pad2(d.getDate()) +
      " " +
      pad2(d.getHours()) +
      ":" +
      pad2(d.getMinutes()) +
      ":" +
      pad2(d.getSeconds())
    );
  }

  /** Rough "time until/since this instant" for tooltips and footer (gallery demo). */
  function formatRelativeAgo(ms) {
    if (ms == null || !isFinite(Number(ms))) return "—";
    var now = Date.now();
    var sec = Math.round((now - Number(ms)) / 1000);
    if (sec < 0) return "in the future";
    if (sec < 10) return "just now";
    if (sec < 60) return "about " + sec + " seconds ago";
    if (sec < 3600) {
      var m = Math.floor(sec / 60);
      return m === 1 ? "about 1 minute ago" : "about " + m + " minutes ago";
    }
    if (sec < 86400) {
      var h = Math.floor(sec / 3600);
      return h === 1 ? "about 1 hour ago" : "about " + h + " hours ago";
    }
    var d = Math.floor(sec / 86400);
    if (d < 60) return d === 1 ? "about 1 day ago" : "about " + d + " days ago";
    var mo = Math.floor(d / 30);
    if (mo < 24) return mo === 1 ? "about 1 month ago" : "about " + mo + " months ago";
    var y = Math.floor(d / 365);
    return y === 1 ? "about 1 year ago" : "about " + y + " years ago";
  }

  function escapeAttr(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/</g, "&lt;");
  }

  function applyLocalTimes(root) {
    try {
      var tbody = root.querySelector("[data-gallery-evlog-tbody]");
      if (!tbody) return;
      var els = tbody.querySelectorAll("time[datetime]");
      for (var i = 0; i < els.length; i++) {
        var iso = els[i].getAttribute("datetime");
        var parsed = iso ? Date.parse(iso) : NaN;
        els[i].textContent = formatLogDateTimeLocal(parsed);
        els[i].title = formatRelativeAgo(parsed);
      }
    } catch (e0) {}
  }

  function syncFooter(root) {
    var foot = root.querySelector("[data-gallery-evlog-oldest]");
    var tbody = root.querySelector("[data-gallery-evlog-tbody]");
    if (!foot || !tbody) return;

    var picked = tbody.querySelector(
      "tr[data-evlog-id].sum-evlog__row--selected:not(.sum-evlog__row--hidden)"
    );
    if (picked) {
      var msSel = rowTimespec(picked);
      var absSel = formatLogDateTimeLocal(msSel);
      var relSel = formatRelativeAgo(msSel);
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
        ? '<time datetime="' + escapeAttr(dtAttr) + '" title="' + escapeAttr(relSel) + '">'
        : '<time title="' + escapeAttr(relSel) + '">';
      foot.innerHTML =
        "Selected entry: " +
        timeOpen +
        absSel +
        "</time> <span class=\"sum-gallery-evlog__footer-rel\">(" +
        relSel +
        ")</span>";
      return;
    }

    var oldestMs = root._evlogOldestVisible;
    var relOld = formatRelativeAgo(oldestMs);
    foot.innerHTML =
      "Oldest <strong>visible</strong> entry: <time title=\"" +
      escapeAttr(relOld) +
      "\">" +
      formatLogDateTimeLocal(oldestMs) +
      "</time>";
  }

  function parseHttp(attr) {
    if (attr == null || String(attr).trim() === "") return null;
    var n = parseInt(String(attr).trim(), 10);
    return isNaN(n) ? null : n;
  }

  function levelKey(level) {
    var s = level == null ? "" : String(level).trim();
    return s === "" ? "NONE" : s.toUpperCase();
  }

  function isWarnish(levelCanon, http) {
    var lk = levelKey(levelCanon);
    if (lk === "WARN") return true;
    if (http === 429) return true;
    return false;
  }

  function isFailish(levelCanon, http) {
    var lk = levelKey(levelCanon);
    if (lk === "ERROR") return true;
    if (http == null) return false;
    if (http >= 200 && http <= 299) return false;
    return true;
  }

  function rowTimespec(tr) {
    var tEl = tr.querySelector("time[datetime]");
    if (!tEl || !tEl.getAttribute("datetime")) return NaN;
    var ms = Date.parse(tEl.getAttribute("datetime"));
    return isNaN(ms) ? NaN : ms;
  }

  function rowPassesStatus(http, levelCanon, mode) {
    if (mode === "all") return true;
    if (mode === "warnings") return isWarnish(levelCanon, http);
    if (mode === "errors") return isFailish(levelCanon, http);
    return true;
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
      var lk = levelKey(tr.dataset.evlogLevel);
      blob += " " + lk.toLowerCase();
    } catch (e2) {}
    return blob.toLowerCase().replace(/\s+/g, " ").trim();
  }

  function countMetrics(rows) {
    var warn = 0;
    var fail = 0;
    for (var i = 0; i < rows.length; i++) {
      var tr = rows[i];
      var raw = tr.dataset.evlogLevel;
      var http = parseHttp(tr.dataset.evlogHttp);
      if (isWarnish(raw, http)) warn++;
      if (isFailish(raw, http)) fail++;
    }
    return { warn: warn, fail: fail };
  }

  function allDataRows(tbody) {
    return tbody.querySelectorAll("tr[data-evlog-id]");
  }

  function rowIndex(rows, tr) {
    for (var i = 0; i < rows.length; i++) {
      if (rows[i] === tr) return i;
    }
    return -1;
  }

  function clearSelection(tbody) {
    var sel = tbody.querySelectorAll(".sum-evlog__row--selected");
    for (var i = 0; i < sel.length; i++) sel[i].classList.remove("sum-evlog__row--selected");
  }

  function setRangeSelected(rows, lo, hi, on) {
    for (var i = lo; i <= hi && i < rows.length; i++) {
      if (on) rows[i].classList.add("sum-evlog__row--selected");
      else rows[i].classList.remove("sum-evlog__row--selected");
    }
  }

  function ensureSearchEmptyRow(tbody) {
    var existing = tbody.querySelector("[data-gallery-evlog-search-empty-row]");
    if (existing) return existing;
    var tr = document.createElement("tr");
    tr.className = "sum-evlog__row sum-gallery-evlog__search-empty-row";
    tr.setAttribute("data-gallery-evlog-search-empty-row", "");
    tr.setAttribute("hidden", "");
    tr.setAttribute("role", "status");
    tr.setAttribute("aria-live", "polite");
    var td = document.createElement("td");
    td.className = "sum-gallery-evlog__search-empty-cell";
    td.colSpan = 3;
    td.appendChild(document.createTextNode("No matching entries for your search. "));
    var btn = document.createElement("button");
    btn.type = "button";
    btn.className = "sum-gallery-evlog__clear-search";
    btn.setAttribute("data-gallery-evlog-clear-search", "");
    btn.appendChild(document.createTextNode("Clear search"));
    td.appendChild(btn);
    tr.appendChild(td);
    tbody.insertBefore(tr, tbody.firstChild);
    return tr;
  }

  function rebuild(root) {
    var tbody = root.querySelector("[data-gallery-evlog-tbody]");
    if (!tbody) return;
    var select = root.querySelector("[data-evlog-filter-status]");
    var mode = select && select.value ? select.value : "all";

    var q = "";
    try {
      var inp = root.querySelector(".sum-evlog__search");
      q = inp && inp.value ? String(inp.value).trim().toLowerCase() : "";
    } catch (eIn) {}

    var rows = Array.prototype.slice.call(allDataRows(tbody), 0);
    var oldest = Infinity;
    var visibleCount = 0;

    for (var i = 0; i < rows.length; i++) {
      var tr = rows[i];
      var lk3 = levelKey(tr.dataset.evlogLevel);
      var httpVal = parseHttp(tr.dataset.evlogHttp);

      var passStat = rowPassesStatus(httpVal, lk3 === "NONE" ? "" : lk3, mode);
      var passSearch = q === "" || rowSearchBlob(tr).indexOf(q) !== -1;

      var show = passStat && passSearch;
      tr.classList.toggle("sum-evlog__row--hidden", !show);

      if (show) {
        visibleCount++;
        var ts = rowTimespec(tr);
        if (isFinite(ts) && ts < oldest) oldest = ts;
      }
    }

    var searchEmptyRow = tbody.querySelector("[data-gallery-evlog-search-empty-row]");
    if (searchEmptyRow) {
      searchEmptyRow.hidden = !(q !== "" && visibleCount === 0);
    }

    root._evlogOldestVisible = oldest === Infinity ? NaN : oldest;
    syncFooter(root);
  }

  function copySelected(root) {
    var tbody = root.querySelector("[data-gallery-evlog-tbody]");
    var toast = root.querySelector("[data-gallery-evlog-toast]");
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
      var cols = [timeStr];
      if (src) cols.push(srcStr);
      cols.push(msgStr, statStr);
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
          var suffix = allVisible
            ? " visible line(s)."
            : " line(s).";
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

  function bindRowActivate(root, tbody) {
    var rows = [];
    function refreshRows() {
      rows = Array.prototype.slice.call(allDataRows(tbody), 0);
    }
    refreshRows();

    tbody.addEventListener("click", function (ev) {
      var td = ev.target.closest("td");
      var tr = ev.target.closest("tr[data-evlog-id]");
      if (!td || !tr || tr.closest("thead")) return;

      refreshRows();
      var idx = rowIndex(rows, tr);
      if (idx < 0) return;

      if (ev.shiftKey && root._evlogSelAnchor != null && root._evlogSelAnchor >= 0) {
        clearSelection(tbody);
        var a = root._evlogSelAnchor;
        var lo = Math.min(a, idx);
        var hi = Math.max(a, idx);
        setRangeSelected(rows, lo, hi, true);
        syncFooter(root);
        return;
      }
      if (ev.ctrlKey || ev.metaKey) {
        tr.classList.toggle("sum-evlog__row--selected");
        root._evlogSelAnchor = idx;
        syncFooter(root);
        return;
      }
      clearSelection(tbody);
      tr.classList.add("sum-evlog__row--selected");
      root._evlogSelAnchor = idx;
      syncFooter(root);
    });
  }

  function wireRoot(root) {
    var tbody = root.querySelector("[data-gallery-evlog-tbody]");
    if (!tbody) return;

    applyLocalTimes(root);

    var rows = allDataRows(tbody);
    var m = countMetrics(rows);
    var wEl = root.querySelector("[data-sum-evlog-metric-warn]");
    var fEl = root.querySelector("[data-sum-evlog-metric-fail]");
    if (wEl) wEl.textContent = String(m.warn);
    if (fEl) fEl.textContent = String(m.fail);

    bindRowActivate(root, tbody);

    ensureSearchEmptyRow(tbody);

    var search = root.querySelector(".sum-evlog__search");
    var tSearch = null;
    if (search) {
      search.addEventListener("input", function () {
        window.clearTimeout(tSearch);
        tSearch = window.setTimeout(function () {
          rebuild(root);
        }, 120);
      });
    }

    var statusSel = root.querySelector("[data-evlog-filter-status]");
    if (statusSel) statusSel.addEventListener("change", function () { rebuild(root); });

    var copyBtn = root.querySelector(".sum-evlog__copy-btn");
    if (copyBtn) copyBtn.addEventListener("click", function () { copySelected(root); });

    root.addEventListener("click", function (ev) {
      var clearBtn = ev.target.closest("[data-gallery-evlog-clear-search]");
      if (!clearBtn || !root.contains(clearBtn)) return;
      ev.preventDefault();
      var inp = root.querySelector(".sum-evlog__search");
      if (!inp) return;
      inp.value = "";
      rebuild(root);
      inp.focus();
    });

    rebuild(root);
  }

  function init() {
    var roots = document.querySelectorAll("[data-gallery-evlog-root]");
    for (var r = 0; r < roots.length; r++) wireRoot(roots[r]);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();

  globalThis.GalleryEventLogDemo = { wireRoot: wireRoot, reinitAll: init };
})();
