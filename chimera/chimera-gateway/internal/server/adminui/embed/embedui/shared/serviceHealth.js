/**
 * Service health segment rendering (gateway overview + service cards).
 */
(function () {
  "use strict";

  function normalizeHealthTone(raw) {
    var s = String(raw || "").trim().toLowerCase();
    if (
      s === "up" ||
      s === "ok" ||
      s === "healthy" ||
      s === "ready" ||
      s === "running" ||
      s === "enabled" ||
      s === "supervised" ||
      s === "starting"
    ) {
      return "up";
    }
    if (s === "down" || s === "degraded" || s === "unavailable" || s === "error" || s === "failed" || s === "disabled") {
      return "down";
    }
    return "unknown";
  }

  function healthSegSpan(escapeHtml, title, tone, extraClass) {
    var yep = globalThis.ChimeraUI && globalThis.ChimeraUI.healthSegSpan;
    if (typeof yep === "function") return yep(title, tone, extraClass);
    var esc = escapeHtml || function (x) { return String(x); };
    var cls = "sum-bf-prov-health-seg sum-bf-prov-health-seg--" + (tone === "up" || tone === "down" ? tone : "unknown");
    if (extraClass) cls += " " + extraClass;
    return '<span class="' + esc(cls) + '" title="' + esc(title) + '"></span>';
  }

  function metricsWrapHtml(innerHtml) {
    return '<span class="sum-metrics">' + (innerHtml || "") + "</span>";
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.ServiceHealth = {
    normalizeHealthTone: normalizeHealthTone,
    healthSegSpan: healthSegSpan,
    metricsWrapHtml: metricsWrapHtml
  };
})();
