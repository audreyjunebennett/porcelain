/**
 * Qdrant vector collection naming (mirrors internal/vectorstore/vectorstore.go CollectionName)
 * and operator-facing strings for logs UI.
 *
 * Requires ChimeraLogs.sha1Hex from derive/sha1.js (emn178/js-sha1 MIT).
 */
function qdrantSlug(s) {
  s = String(s || "")
    .toLowerCase()
    .replace(/^\s+|\s+$/g, "");
  s = s.replace(/[^a-z0-9]+/g, "-");
  s = s.replace(/^-+|-+$/g, "");
  return s;
}

function qdrantCollectionName(tenantId, projectId, flavorId) {
  if (!ChimeraLogs || !ChimeraLogs.sha1Hex) return "";
  var parts = [qdrantSlug(tenantId), qdrantSlug(projectId), qdrantSlug(flavorId)];
  for (var i = 0; i < parts.length; i++) {
    if (!parts[i]) parts[i] = "_";
  }
  var prefix = parts.join("-");
  var key = String(tenantId || "") + "\0" + String(projectId || "") + "\0" + String(flavorId || "");
  var hex = ChimeraLogs.sha1Hex(key);
  var suffix = String(hex).slice(0, 8);
  var full = "chimera-" + prefix + "-" + suffix;
  if (full.length > 200) full = full.slice(0, 200);
  return full;
}

/** From collectIndexerRunMeta-style fields; empty project/tenant yields "". */
function qdrantCollectionNameFromIndexerMeta(meta) {
  meta = meta || {};
  var tenant = String(meta.tenantId != null ? meta.tenantId : "").trim();
  var proj = meta.projectId != null ? String(meta.projectId).trim() : "";
  if (proj === "—") proj = "";
  var flavor = meta.flavorId != null ? String(meta.flavorId).trim() : "";
  if (flavor === "—") flavor = "";
  if (!tenant || !proj) return "";
  return qdrantCollectionName(tenant, proj, flavor);
}

function vectorstoreLegacyToCanonical(msg) {
  var s = String(msg != null ? msg : "")
    .toLowerCase()
    .trim();
  if (s.indexOf("qdrant.") === 0) return "vectorstore." + s.slice("qdrant.".length);
  return s;
}

function vectorstoreCanonicalMsg(flat) {
  if (!flat || typeof flat !== "object") return "";
  var oc = globalThis.ChimeraLogs && ChimeraLogs.OperatorCopy;
  if (oc && typeof oc.resolveFlat === "function") {
    var slug = oc.resolveFlat(flat);
    if (slug) return slug;
  }
  return vectorstoreLegacyToCanonical(flat.msg != null ? flat.msg : flat.message);
}

function vectorstoreServiceMatch(flat) {
  var svc = String((flat && flat.service) || "").toLowerCase();
  return svc === "chimera-vectorstore" || svc === "qdrant" || svc === "vectorstore";
}

function qdrantSliceCurrentProcess(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastIdx = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    var msg = vectorstoreCanonicalMsg(getFlat(arr[i].parsed));
    if (msg === "vectorstore.version" || msg === "qdrant.version") lastIdx = i;
  }
  if (lastIdx < 0) return arr.slice();
  return arr.slice(lastIdx);
}

function qdrantCollectionDisplay(collRaw, resolveColl) {
  var r = collRaw != null ? String(collRaw).trim() : "";
  if (!r) return "";
  if (typeof resolveColl === "function") {
    var x = resolveColl(r);
    if (x != null && String(x).trim() !== "") return String(x).trim();
  }
  return r;
}

/** Operator line for vectorstore backend logs (registry-driven; qdrant.* aliases in operator_copy.js). */
function qdrantOperatorLine(flat, resolveColl, opts) {
  opts = opts || {};
  if (!flat || typeof flat !== "object") return "—";
  if (resolveColl) opts.resolveColl = resolveColl;
  if (
    globalThis.ChimeraLogs &&
    ChimeraLogs.Render &&
    typeof ChimeraLogs.Render.operatorMessage === "function"
  ) {
    var line = ChimeraLogs.Render.operatorMessage(flat, opts);
    if (line) return line;
  }
  return flat.msg != null ? String(flat.msg) : "—";
}

function qdrantIndexerCollectionStatusLabel(msg) {
  msg = String(msg || "").toLowerCase();
  switch (msg) {
    case "qdrant.collection.loading":
    case "qdrant.shard.recover_progress":
      return "Loading";
    case "qdrant.shard.recovered":
      return "Loaded";
    case "qdrant.http.collection_meta":
      return "Reading";
    case "qdrant.http.points_upsert_ok":
    case "qdrant.http.points_upsert_rejected":
      return "Upserting";
    case "qdrant.http.points_delete":
      return "Deleting";
    case "qdrant.http.vector_search":
      return "Searching";
    default:
      return "";
  }
}

/** Aggregate KV + counters for Qdrant service card (current Qdrant process only). */
function qdrantCardModel(arr, getFlat, resolveColl) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var slice = qdrantSliceCurrentProcess(arr, getFlat);

  var out = {
    version: "",
    configuration: "",
    mode: "",
    tls: "",
    tlsGrpc: "",
    tlsInternal: "",
    telemetry: "",
    recovery: "",
    restPort: null,
    grpcPort: null,
    collTotal: 0,
    collLoaded: 0,
    upsertOk: 0,
    upsertFail: 0,
    deleteOk: 0,
    deleteFail: 0,
    searchOk: 0,
    searchFail: 0,
    subtitle: "—"
  };

  var i;
  for (i = 0; i < slice.length; i++) {
    var f = getFlat(slice[i].parsed);
    if (!vectorstoreServiceMatch(f)) continue;
    var msg = vectorstoreCanonicalMsg(f);
    var httpSt = f.http_status != null ? Number(f.http_status) : NaN;
    var okHttp = !isNaN(httpSt) && httpSt === 200;

    if (f.qdrant_version) out.version = String(f.qdrant_version).trim();
    if (f.qdrant_config === "supervised") out.configuration = "supervised";
    if (f.qdrant_mode === "single-node") out.mode = "single-node";
    if (f.qdrant_tls_rest === "disabled") out.tls = "disabled";
    if (f.qdrant_tls_rest === "enabled") out.tls = "enabled";
    if (f.qdrant_tls_grpc === "disabled") out.tlsGrpc = "disabled";
    if (f.qdrant_tls_grpc === "enabled") out.tlsGrpc = "enabled";
    if (f.qdrant_internal_tls === "disabled") out.tlsInternal = "disabled";
    if (f.qdrant_internal_tls === "enabled") out.tlsInternal = "enabled";
    if (f.qdrant_telemetry === "disabled") out.telemetry = "disabled";
    if (f.qdrant_telemetry === "enabled") out.telemetry = "enabled";
    if (f.qdrant_recovery === "active") out.recovery = "active";
    if (f.rest_port != null && !isNaN(Number(f.rest_port))) out.restPort = Math.round(Number(f.rest_port));
    if (f.grpc_port != null && !isNaN(Number(f.grpc_port))) out.grpcPort = Math.round(Number(f.grpc_port));

    if (msg === "vectorstore.collection.loading" || msg === "qdrant.collection.loading") out.collTotal++;
    if (msg === "vectorstore.shard.recovered" || msg === "qdrant.shard.recovered") out.collLoaded++;

    if (msg === "vectorstore.http.points_upsert_ok" || msg === "qdrant.http.points_upsert_ok") {
      if (okHttp) out.upsertOk++;
      else out.upsertFail++;
    } else if (msg === "vectorstore.http.points_upsert_rejected" || msg === "qdrant.http.points_upsert_rejected") {
      out.upsertFail++;
    } else if (msg === "vectorstore.http.points_delete" || msg === "qdrant.http.points_delete") {
      if (okHttp) out.deleteOk++;
      else out.deleteFail++;
    } else if (msg === "vectorstore.http.vector_search" || msg === "qdrant.http.vector_search") {
      if (okHttp) out.searchOk++;
      else out.searchFail++;
    }
  }

  for (i = slice.length - 1; i >= 0; i--) {
    var f2 = getFlat(slice[i].parsed);
    if (vectorstoreServiceMatch(f2)) {
      out.subtitle = qdrantOperatorLine(f2, resolveColl);
      break;
    }
  }
  return out;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.vectorstoreSlug = qdrantSlug;
globalThis.ChimeraLogs.Derive.vectorstoreCollectionName = qdrantCollectionName;
globalThis.ChimeraLogs.Derive.vectorstoreCollectionNameFromIndexerMeta = qdrantCollectionNameFromIndexerMeta;
globalThis.ChimeraLogs.Derive.vectorstoreSliceCurrentProcess = qdrantSliceCurrentProcess;
globalThis.ChimeraLogs.Derive.vectorstoreCollectionDisplay = qdrantCollectionDisplay;
globalThis.ChimeraLogs.Derive.vectorstoreOperatorLine = qdrantOperatorLine;
globalThis.ChimeraLogs.Derive.vectorstoreIndexerCollectionStatusLabel = qdrantIndexerCollectionStatusLabel;
globalThis.ChimeraLogs.Derive.vectorstoreCardModel = qdrantCardModel;
