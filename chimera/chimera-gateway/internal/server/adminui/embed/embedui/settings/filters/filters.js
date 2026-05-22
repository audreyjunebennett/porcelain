/**
 * Filters (Application / Level).
 *
 * Exports:
 * - ChimeraSettings.Filters.init(ctx)
 * - ChimeraSettings.Filters.apply()
 * - ChimeraSettings.Filters.ensureAppOption(app)
 * - ChimeraSettings.Filters.ensureLevelOption(lvl)
 * - ChimeraSettings.Filters.matchesRow(ctx, tr)   // StructuredLogs tbody rows
 *
 * ctx requirements:
 * - fltAppEl, fltLevelEl, tbodyEl
 * - viewModeGetter(): string
 * - rebuildRawLogsTextarea(opts)
 * - levelOptionSet (object)
 */

function ensureAppOption(ctx, app) {
  if (!app) return;
  var fltApp = ctx.fltAppEl;
  for (var i = 0; i < fltApp.options.length; i++) {
    if (fltApp.options[i].value === app) return;
  }
  var opt = document.createElement("option");
  opt.value = app;
  opt.textContent = app;
  fltApp.appendChild(opt);
}

function ensureLevelOption(ctx, lvl) {
  if (!lvl) return;
  if (ctx.levelOptionSet && ctx.levelOptionSet[lvl]) return;
  if (ctx.levelOptionSet) ctx.levelOptionSet[lvl] = true;
  var fltLevel = ctx.fltLevelEl;
  var opt = document.createElement("option");
  opt.value = lvl;
  opt.textContent = lvl;
  fltLevel.appendChild(opt);
}

function rowMatchesFilter(ctx, tr) {
  var fa = ctx.fltAppEl.value;
  var fl = ctx.fltLevelEl.value;
  if (fa && tr.dataset.app !== fa) return false;
  if (!fl) return true;
  if (fl === "(none)") return !tr.dataset.level || tr.dataset.level === "";
  return tr.dataset.level === fl;
}

function entryMatchesFilters(ctx, parsed) {
  if (!parsed || typeof parsed !== "object") return true;
  var fa = ctx.fltAppEl.value;
  var fl = ctx.fltLevelEl.value;
  var app = parsed.app || "";
  var lvl = parsed.levelCanon || "";
  if (fa && app !== fa) return false;
  if (!fl) return true;
  if (fl === "(none)") return !lvl || lvl === "";
  return lvl === fl;
}

function applyFilters(ctx) {
  var viewMode = ctx.viewModeGetter();
  if (viewMode === "raw_logs") {
    var ta = document.getElementById("raw-logs-textarea");
    var follow = true;
    if (ctx.nearBottomTextarea && typeof ctx.nearBottomTextarea === "function") {
      try {
        follow = ctx.nearBottomTextarea(ta);
      } catch (_e) {
        follow = true;
      }
    }
    ctx.rebuildRawLogsTextarea({ scrollBottom: follow });
    return;
  }
  // tbody.rows = log rows only; querySelectorAll("tr") also matched inner details <tr>s.
  var rows = ctx.tbodyEl.rows;
  for (var i = 0; i < rows.length; i++) {
    rows[i].style.display = rowMatchesFilter(ctx, rows[i]) ? "" : "none";
  }
}

function init(ctx) {
  ctx.fltAppEl.addEventListener("change", function () {
    applyFilters(ctx);
  });
  ctx.fltLevelEl.addEventListener("change", function () {
    applyFilters(ctx);
  });
}

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Filters = {
  init: init,
  apply: function (ctx) { return applyFilters(ctx); },
  ensureAppOption: function (ctx, app) { return ensureAppOption(ctx, app); },
  ensureLevelOption: function (ctx, lvl) { return ensureLevelOption(ctx, lvl); },
  entryMatches: function (ctx, parsed) { return entryMatchesFilters(ctx, parsed); },
  /** StructuredLogs table rows (dataset.app / dataset.level). */
  matchesRow: function (ctx, tr) { return rowMatchesFilter(ctx, tr); }
};
