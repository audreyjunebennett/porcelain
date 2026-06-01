/**
 * Scoped in-card event log panel (settings cards + wizard).
 */
(function () {
  "use strict";

  var _deps = null;

  function setDeps(deps) {
    _deps = deps || null;
  }

  function panelFromEvents(title, scopeId, evs, opts) {
    if (!_deps) return "";
    opts = opts || {};
    var escapeHtml = _deps.escapeHtml;
    var getFlat = _deps.getFlat;
    var sumEvlogRowTrHtml = _deps.sumEvlogRowTrHtml;
    var sumEvlogPanelHtml = _deps.sumEvlogPanelHtml;
    var sumEvlogHttpCode = _deps.sumEvlogHttpCode;
    var sumEvlogIsWarnish = _deps.sumEvlogIsWarnish;
    var sumEvlogIsFailish = _deps.sumEvlogIsFailish;
    var inferServiceBadge = _deps.inferServiceBadge;
    var showSource = opts.showSourceColumn === true;
    var rowOpts = showSource ? { showSourceColumn: true } : {};
    var parts = [];
    var warnN = 0;
    var failN = 0;
    evs = evs || [];
    for (var i = 0; i < evs.length; i++) {
      var ev = evs[i];
      var flat = getFlat(ev.parsed);
      var http = sumEvlogHttpCode(ev.parsed, flat);
      var lvl = String(ev.parsed.levelCanon || ev.parsed.levelLabel || "").trim();
      if (sumEvlogIsWarnish(lvl, http)) warnN++;
      if (sumEvlogIsFailish(lvl, http)) failN++;
      parts.push(sumEvlogRowTrHtml(ev, scopeId, i, inferServiceBadge(ev), rowOpts));
    }
    return sumEvlogPanelHtml({
      title: title,
      scrollTbodyId: "sum-evlog-" + escapeHtml(scopeId),
      warnN: warnN,
      failN: failN,
      showSourceColumn: showSource,
      tbodyInnerHtml: parts.join(""),
      uiPart: opts.uiPart
    });
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.ScopedEvlog = {
    setDeps: setDeps,
    panelFromEvents: panelFromEvents
  };
})();
