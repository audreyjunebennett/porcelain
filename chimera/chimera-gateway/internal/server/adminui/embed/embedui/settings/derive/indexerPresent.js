/**
 * Indexer log presentation helpers (pure). Used by logs.js and tested via goja.
 *
 * ChimeraSettings.Derive.indexerDeclaredStateLabel(code)
 * ChimeraSettings.Derive.indexerSlugHistogramBucket(msgLower)
 * ChimeraSettings.Derive.indexerGroupKeyFromFlat(flat)
 * ChimeraSettings.Derive.indexerFlatMsg(flat) — registry resolveFlat (slog duplicate msg keys)
 * ChimeraSettings.Derive.indexerProseSummary(flat) — registry operatorMessage; null → caller fallback
 */

function indexerDeclaredStateLabel(code) {
  var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
  var labels = oc && oc.indexerStateLabels;
  var key = String(code || "").trim();
  if (labels && key && labels[key]) return labels[key];
  return key ? key : "";
}

function indexerFlatMsg(fl) {
  if (!fl || typeof fl !== "object") fl = {};
  var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
  if (oc && typeof oc.resolveFlat === "function") {
    var slug = oc.resolveFlat(fl);
    if (slug) return slug;
  }
  var msg = fl.msg != null ? fl.msg : fl.message;
  return String(msg != null ? msg : "").toLowerCase().trim();
}

function indexerSlugHistogramBucket(msgLower) {
  var m = String(msgLower || "").trim();
  if (!m) return "other";
  if (m.indexOf("indexer.run.") === 0) return "lifecycle";
  if (m.indexOf("indexer.supervised.") === 0) return "lifecycle";
  if (m.indexOf("indexer.discovery") === 0 || m.indexOf("indexer.reconcile") === 0) return "discovery";
  if (m.indexOf("indexer.job.") === 0) return "jobs";
  if (m.indexOf("indexer.queue") === 0) return "queue";
  if (m.indexOf("indexer.scope.") === 0) return "statestats";
  if (m === "indexer.state" || m.indexOf("indexer.storage.stats") === 0) return "statestats";
  if (m.indexOf("gateway.indexer.config") === 0) return "config";
  if (
    m.indexOf("indexer.recovery") === 0 ||
    m.indexOf("indexer.ingest.gate") === 0 ||
    m.indexOf("indexer.retry") === 0 ||
    m.indexOf("indexer.worker.paused") === 0
  ) {
    return "recovery";
  }
  if (m.indexOf("indexer.") === 0) return "indexer_misc";
  return "other";
}

function indexerGroupKeyFromFlat(fl) {
  if (!fl || typeof fl !== "object") return "";
  var itk =
    fl.indexer_target_key != null && String(fl.indexer_target_key).trim() !== ""
      ? String(fl.indexer_target_key).trim()
      : "";
  if (itk) return itk;
  var ik =
    fl.indexer_key != null && String(fl.indexer_key).trim() !== ""
      ? String(fl.indexer_key).trim()
      : "";
  if (ik) return ik;
  var uid = String(fl.tenant_id || fl.principal_id || fl.tenant || "").trim();
  var proj = String(
    fl.project_id ||
      fl.ingest_project ||
      fl.defaults_project_id ||
      fl.scope_project_id ||
      fl.scope_workspace_id ||
      ""
  ).trim();
  var fav = String(fl.flavor_id || fl.defaults_flavor_id || "").trim();
  if (uid !== "" && proj !== "" && fav !== "") {
    return "ig\x1e" + uid + "\x1e" + proj + "\x1e" + fav;
  }
  if (uid !== "" && proj !== "" && fav === "") {
    return "ig\x1e" + uid + "\x1e" + proj + "\x1e";
  }
  var rid =
    fl.index_run_id != null && String(fl.index_run_id).trim() !== ""
      ? String(fl.index_run_id).trim()
      : "";
  return rid || "";
}

/** Operator-facing one-line description for indexer structured logs. */
function indexerProseSummary(flat) {
  if (!flat || typeof flat !== "object") return null;
  var m = indexerFlatMsg(flat);
  var svc = String(flat.service || "").toLowerCase();
  var indexerish =
    svc === "indexer" ||
    svc === "chimera-indexer" ||
    m.indexOf("indexer.") === 0 ||
    m.indexOf("gateway.indexer") === 0 ||
    (svc === "gateway" && m === "rag.retrieve.source");
  if (!indexerish) return null;
  var render = globalThis.ChimeraSettings && ChimeraSettings.Render;
  if (render && typeof render.operatorMessage === "function") {
    var line = render.operatorMessage(flat, {});
    if (line && String(line).trim() !== "") return String(line).trim();
  }
  return null;
}

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
globalThis.ChimeraSettings.Derive.indexerDeclaredStateLabel = indexerDeclaredStateLabel;
globalThis.ChimeraSettings.Derive.indexerSlugHistogramBucket = indexerSlugHistogramBucket;
globalThis.ChimeraSettings.Derive.indexerGroupKeyFromFlat = indexerGroupKeyFromFlat;
globalThis.ChimeraSettings.Derive.indexerFlatMsgForPresent = indexerFlatMsg;
globalThis.ChimeraSettings.Derive.indexerProseSummary = indexerProseSummary;
