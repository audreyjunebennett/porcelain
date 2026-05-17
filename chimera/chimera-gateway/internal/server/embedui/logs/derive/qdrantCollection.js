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

function qdrantSliceCurrentProcess(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastIdx = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    var msg = String(getFlat(arr[i].parsed).msg || "").toLowerCase();
    if (msg === "qdrant.version") lastIdx = i;
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

function qdrantOperatorLine(flat, resolveColl, opts) {
  opts = opts || {};
  var omitHttpInMsg = opts.forEventLog === true;
  if (!flat || typeof flat !== "object") return "—";
  var msg = String(flat.msg != null ? flat.msg : "").toLowerCase();
  var coll = qdrantCollectionDisplay(flat.collection != null ? flat.collection : "", resolveColl);
  var st = flat.http_status != null ? Number(flat.http_status) : NaN;
  var stLab = !omitHttpInMsg && !isNaN(st) ? String(Math.round(st)) : "";
  var prog = flat.progress_detail != null ? String(flat.progress_detail) : "";
  var ver = flat.qdrant_version != null ? String(flat.qdrant_version).trim() : "";

  switch (msg) {
    case "qdrant.startup.banner":
      return "Starting up …";
    case "qdrant.version":
      return "Component: chimera-vectorstore · Backend: Qdrant " + (ver || "").trim();
    case "qdrant.web_ui_hint":
      return "Optional web UI (dashboard may be unavailable without static assets)";
    case "qdrant.config.optional_missing":
      return "Supervised configuration (no YAML config file)";
    case "qdrant.consensus.raft_load":
      return "Loading collections …";
    case "qdrant.collection.loading":
      return "Loading collection " + coll;
    case "qdrant.shard.recover_progress": {
      var line = "Loading collection " + coll;
      if (prog) line += " · " + prog.replace(/\s+/g, " ").slice(0, 280);
      return line;
    }
    case "qdrant.shard.recovered":
      return "Loaded collection " + coll;
    case "qdrant.cluster.single_node":
      return "Cluster mode: single-node";
    case "qdrant.listen.tls_disabled_rest":
      return "REST TLS disabled";
    case "qdrant.listen.tls_enabled_rest":
      return "REST TLS enabled";
    case "qdrant.listen.tls_enabled_grpc":
      return "gRPC TLS enabled (public API)";
    case "qdrant.listen.tls_disabled_grpc":
      return "gRPC TLS disabled (public API)";
    case "qdrant.cluster.internal_tls_disabled":
      return "Internal cluster gRPC TLS disabled";
    case "qdrant.cluster.internal_tls_enabled":
      return "Internal cluster gRPC TLS enabled";
    case "qdrant.telemetry.enabled":
      return "Telemetry reporting enabled";
    case "qdrant.telemetry.disabled":
      return "Telemetry reporting disabled";
    case "qdrant.hardware_reporting.enabled":
      return "Hardware metrics included in API responses";
    case "qdrant.inference.configured":
      return "Inference service configured";
    case "qdrant.inference.disabled":
      return "Inference service not configured";
    case "qdrant.grpc.endpoint_disabled":
      return "gRPC API disabled in configuration";
    case "qdrant.listen.internal_grpc": {
      var igp = flat.internal_grpc_port != null ? flat.internal_grpc_port : flat.InternalGRPCPort;
      return "Internal gRPC listening" + (igp != null ? " on port " + igp : "");
    }
    case "qdrant.storage.recovery_mode":
      return "Recovery mode · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "see log line");
    case "qdrant.cluster.bootstrap_uri_duplicate":
      return "Cluster bootstrap: duplicate bootstrap URI warning";
    case "qdrant.process.server_start_failed":
      return "Failed to start server · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "");
    case "qdrant.runtime.panic":
      return "Runtime panic · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "");
    case "qdrant.gpu.init_failed":
      return "GPU initialization failed · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 180) : "");
    case "qdrant.runtime.init_file_warning":
      return "Init file indicator warning · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "");
    case "qdrant.security.jwt_rbac_warning":
      return "JWT / API key configuration warning · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "");
    case "qdrant.process.shutdown_signal":
      return "Shutdown signal · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 160) : "");
    case "qdrant.debug.feature_flags":
      return "Feature flags (debug)";
    case "qdrant.debug.collection_loaded":
      return "Collection loaded (debug) · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 220) : "");
    case "qdrant.ui.static_missing":
      return "Web UI static assets missing or disabled";
    case "qdrant.actix.workers":
      return "HTTP worker pool configured";
    case "qdrant.actix.bind":
      return "HTTP server binding";
    case "qdrant.http.access_other":
      return "HTTP request (other route) · " + (prog ? prog.replace(/\s+/g, " ").slice(0, 200) : "");
    case "qdrant.trace.other":
      return prog ? prog.replace(/\s+/g, " ").slice(0, 280) : "Unclassified Qdrant trace";
    case "qdrant.unparsed":
      return prog ? prog.replace(/\s+/g, " ").slice(0, 280) : "Unparsed Qdrant output";
    case "qdrant.listen.http": {
      var rp = flat.rest_port != null ? flat.rest_port : flat.RESTPort;
      return "REST listening" + (rp != null ? " on port " + rp : "");
    }
    case "qdrant.listen.grpc": {
      var gp = flat.grpc_port != null ? flat.grpc_port : flat.GRPCPort;
      return "gRPC listening" + (gp != null ? " on port " + gp : "");
    }
    case "qdrant.http.collection_meta":
      return "Reading collection " + coll + (stLab !== "" ? " · " + stLab : "");
    case "qdrant.http.points_upsert_ok":
    case "qdrant.http.points_upsert_rejected":
      return "Upsert into collection " + coll + (stLab !== "" ? " · " + stLab : "");
    case "qdrant.http.points_delete":
      return "Deleting from collection " + coll + (stLab !== "" ? " · " + stLab : "");
    case "qdrant.http.vector_search":
      return "Searching collection " + coll + (stLab !== "" ? " · " + stLab : "");
    default:
      if (msg.indexOf("qdrant.") === 0) {
        if (prog && (msg === "qdrant.trace.other" || msg === "qdrant.unparsed")) {
          return prog.replace(/\s+/g, " ").slice(0, 280);
        }
        var slugRest = String(flat.msg || msg || "")
          .replace(/^qdrant\./, "")
          .replace(/\./g, " ")
          .trim();
        return slugRest ? "chimera-vectorstore backend · " + slugRest : "chimera-vectorstore backend event";
      }
      return flat.msg != null ? String(flat.msg) : "—";
  }
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
    if (String(f.service || "").toLowerCase() !== "qdrant") continue;
    var msg = String(f.msg || "").toLowerCase();
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

    if (msg === "qdrant.collection.loading") out.collTotal++;
    if (msg === "qdrant.shard.recovered") out.collLoaded++;

    if (msg === "qdrant.http.points_upsert_ok") {
      if (okHttp) out.upsertOk++;
      else out.upsertFail++;
    } else if (msg === "qdrant.http.points_upsert_rejected") {
      out.upsertFail++;
    } else if (msg === "qdrant.http.points_delete") {
      if (okHttp) out.deleteOk++;
      else out.deleteFail++;
    } else if (msg === "qdrant.http.vector_search") {
      if (okHttp) out.searchOk++;
      else out.searchFail++;
    }
  }

  for (i = slice.length - 1; i >= 0; i--) {
    var f2 = getFlat(slice[i].parsed);
    if (String(f2.service || "").toLowerCase() === "qdrant") {
      out.subtitle = qdrantOperatorLine(f2, resolveColl);
      break;
    }
  }
  return out;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.qdrantSlug = qdrantSlug;
globalThis.ChimeraLogs.Derive.qdrantCollectionName = qdrantCollectionName;
globalThis.ChimeraLogs.Derive.qdrantCollectionNameFromIndexerMeta = qdrantCollectionNameFromIndexerMeta;
globalThis.ChimeraLogs.Derive.qdrantSliceCurrentProcess = qdrantSliceCurrentProcess;
globalThis.ChimeraLogs.Derive.qdrantCollectionDisplay = qdrantCollectionDisplay;
globalThis.ChimeraLogs.Derive.qdrantOperatorLine = qdrantOperatorLine;
globalThis.ChimeraLogs.Derive.qdrantIndexerCollectionStatusLabel = qdrantIndexerCollectionStatusLabel;
globalThis.ChimeraLogs.Derive.qdrantCardModel = qdrantCardModel;
