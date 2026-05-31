/**
 * Gallery event-log markup — mirrors production sum-evlog two-column + inline meta.
 * Loaded before gallery-event-log-demo.js and gallery-unified-operator-users.js.
 */
(function () {
  "use strict";

  function pad2(n) {
    return n < 10 ? "0" + n : String(n);
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/"/g, "&quot;");
  }

  /** Stacked time + short date (no year) — same shape as ChimeraSettings.Main.formatLogDateTimeLocalHtml. */
  function formatLogDateTimeLocalHtml(ts) {
    if (ts === null || ts === undefined || ts === "") {
      return '<div class="dt-stack"><span class="dt-line muted">—</span></div>';
    }
    var d = ts instanceof Date ? ts : new Date(ts);
    if (isNaN(d.getTime())) {
      var raw = String(ts).replace("T", " ").slice(0, 23);
      return (
        '<div class="dt-stack dt-fallback"><span class="dt-line dt-time">' +
        escapeHtml(raw) +
        "</span></div>"
      );
    }
    var timeStr = new Intl.DateTimeFormat("en-US", {
      hour: "numeric",
      minute: "2-digit",
      second: "2-digit",
      hour12: false
    }).format(d);
    var dateStr = new Intl.DateTimeFormat("en-US", {
      month: "short",
      day: "numeric"
    }).format(d);
    return (
      '<div class="dt-stack">' +
      '<span class="dt-line dt-time">' +
      escapeHtml(timeStr) +
      '</span><span class="dt-line dt-date">' +
      escapeHtml(dateStr) +
      "</span></div>"
    );
  }

  function formatLogDateTimeLocalCompact(ts) {
    if (ts === null || ts === undefined || ts === "") return "—";
    var d = ts instanceof Date ? ts : new Date(ts);
    if (isNaN(d.getTime())) return String(ts).replace("T", " ").slice(0, 23);
    var dateStr = new Intl.DateTimeFormat("en-US", {
      month: "short",
      day: "numeric"
    }).format(d);
    return (
      dateStr +
      " " +
      pad2(d.getHours()) +
      ":" +
      pad2(d.getMinutes()) +
      ":" +
      pad2(d.getSeconds())
    );
  }

  var FOOTER_METRICS_HTML =
    '<div class="sum-evlog__footer-metrics" role="group" aria-label="Status counts">' +
    '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog-metric-num" data-sum-evlog-metric-warn>—</span>' +
    '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog__metric-icon" aria-hidden="true">⚠</span>' +
    '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog-metric-num" data-sum-evlog-metric-fail>—</span>' +
    '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog__metric-icon" aria-hidden="true">✖</span>' +
    "</div>";

  var TABLE_HEAD_2COL =
    '<colgroup><col class="sum-evlog__col-time" /><col class="sum-evlog__col-msg" /></colgroup>' +
    '<thead><tr><th class="sum-evlog__cell--time" scope="col">Time</th>' +
    '<th class="sum-evlog__th-msg" scope="col">Message</th></tr></thead>';

  function sourceMetaFromBadgeHtml(badgeHtml) {
    if (!badgeHtml) return "";
    if (badgeHtml.indexOf("sum-evlog-meta-source") >= 0) return badgeHtml;
    if (badgeHtml.indexOf('class="sum-svc-badge') >= 0) {
      return badgeHtml.replace('class="sum-svc-badge', 'class="sum-evlog-meta-source sum-svc-badge');
    }
    return '<span class="sum-evlog-meta-source">' + badgeHtml + "</span>";
  }

  function sourceMetaFromClass(cls, label) {
    return (
      '<span class="sum-evlog-meta-source sum-svc-badge ' +
      escapeHtml(cls) +
      '">' +
      escapeHtml(label) +
      "</span>"
    );
  }

  function statusMetaInner(innerHtml) {
    return (
      '<span class="sum-evlog-meta-status"><span class="sum-evlog-status">' +
      innerHtml +
      "</span></span>"
    );
  }

  function msgCellHtml(sourceMetaHtml, statusInnerHtml, textHtml) {
    return (
      '<td class="sum-evlog__cell--msg"><div class="sum-evlog__msg-wrap"><div class="sum-evlog__msg-meta">' +
      (sourceMetaHtml || "") +
      statusMetaInner(statusInnerHtml) +
      '</div><div class="sum-evlog__msg-text">' +
      textHtml +
      "</div></div></td>"
    );
  }

  function rowHtml(opts) {
    opts = opts || {};
    var iso = opts.datetime || "";
    var rel = opts.relTitle != null ? String(opts.relTitle) : "";
    var dtInner = formatLogDateTimeLocalHtml(iso ? Date.parse(iso) : NaN);
    var sourceMeta =
      opts.sourceBadgeClass && opts.sourceLabel
        ? sourceMetaFromClass(opts.sourceBadgeClass, opts.sourceLabel)
        : "";
    var statusInner = opts.statusInner != null ? String(opts.statusInner) : '<span class="sum-evlog-status__empty" aria-hidden="true"></span>';
    return (
      '<tr class="sum-evlog__row" data-evlog-id="' +
      escapeHtml(opts.id || "") +
      '" data-evlog-level="' +
      escapeHtml(opts.level || "INFO") +
      '"' +
      (opts.http != null ? ' data-evlog-http="' + escapeHtml(String(opts.http)) + '"' : "") +
      ">" +
      '<td class="sum-evlog__cell--time"><time datetime="' +
      escapeHtml(iso) +
      '" title="' +
      escapeHtml(rel) +
      '">' +
      dtInner +
      "</time></td>" +
      msgCellHtml(sourceMeta, statusInner, opts.textHtml || "") +
      "</tr>"
    );
  }

  function panelFooterHtml() {
    return (
      '<div class="sum-evlog__footer-row">' +
      '<div class="sum-evlog__footer-left"><p class="sum-evlog__footer" data-gallery-evlog-oldest></p></div>' +
      '<div class="sum-evlog__footer-right">' +
      '<p class="sum-evlog__toast sum-gallery-evlog__toast-align" data-gallery-evlog-toast role="status" aria-live="polite"></p>' +
      FOOTER_METRICS_HTML +
      "</div></div>"
    );
  }

  globalThis.GalleryEvlogMarkup = {
    escapeHtml: escapeHtml,
    formatLogDateTimeLocalHtml: formatLogDateTimeLocalHtml,
    formatLogDateTimeLocalCompact: formatLogDateTimeLocalCompact,
    footerMetricsHtml: FOOTER_METRICS_HTML,
    tableHead2Col: TABLE_HEAD_2COL,
    sourceMetaFromClass: sourceMetaFromClass,
    msgCellHtml: msgCellHtml,
    rowHtml: rowHtml,
    panelFooterHtml: panelFooterHtml
  };
})();
