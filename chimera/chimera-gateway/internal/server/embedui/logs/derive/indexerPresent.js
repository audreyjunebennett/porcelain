/**
 * Indexer log presentation helpers (pure). Used by logs.js and tested via goja.
 *
 * ChimeraLogs.Derive.indexerDeclaredStateLabel(code)
 * ChimeraLogs.Derive.indexerSlugHistogramBucket(msgLower)
 * ChimeraLogs.Derive.indexerGroupKeyFromFlat(flat)
 * ChimeraLogs.Derive.indexerFlatMsg(flat) — normalizes msg/message
 * ChimeraLogs.Derive.indexerProseSummary(flat) — one-line operator prose; null → caller fallback
 */

function indexerDeclaredStateLabel(code) {
  switch (String(code || "").trim()) {
    case "watch_idle":
      return "Waiting for file changes";
    case "backlog":
      return "Queued files to process";
    case "uploading":
      return "Embedding";
    case "recovery":
      return "Recovering (waiting for gateway / storage)";
    case "initial_scanning":
      return "Scanning workspace";
    case "idle":
      return "Idle";
    default:
      return code ? String(code) : "";
  }
}

function indexerFlatMsg(fl) {
  if (!fl || typeof fl !== "object") fl = {};
  var msg = fl.msg != null ? fl.msg : fl.message;
  var raw = String(msg != null ? msg : "").toLowerCase().trim();
  // slog JSON often contains duplicate `"msg"` keys (human title + slug). JSON.parse tends to
  // keep one implementation-defined winner — infer slug from sibling attrs when ambiguous.
  if (raw === "indexer.state") return "indexer.state";
  if (raw === "indexer state") {
    if (
      fl.state != null &&
      String(fl.state).trim() !== "" &&
      (fl.queue_depth != null || fl.ingest_inflight != null || typeof fl.watch_mode === "boolean")
    )
      return "indexer.state";
  }
  if (raw === "indexer.storage.stats" || raw.indexOf("indexer.storage.stats") === 0) return "indexer.storage.stats";
  if (raw === "indexer storage stats sync" || raw === "indexer storage stats") {
    if (fl.qdrant_points != null || fl.collection != null || fl.available != null || fl.detail != null) {
      return "indexer.storage.stats";
    }
  }
  if (raw === "indexer.queue.snapshot" || raw.indexOf("indexer.queue.snapshot") === 0) return "indexer.queue.snapshot";
  if (raw === "indexer queue snapshot" && fl.queue_depth != null && fl.phase != null) return "indexer.queue.snapshot";
  if (raw === "indexer.discovery.summary.scope" || raw.indexOf("indexer.discovery.summary.scope") === 0)
    return "indexer.discovery.summary.scope";
  if (raw === "discovery summary (scope)" && fl.ingest_project != null) return "indexer.discovery.summary.scope";
  if (raw === "indexer.discovery.summary" || raw.indexOf("indexer.discovery.summary") === 0) return "indexer.discovery.summary";
  if (raw === "discovery summary" && fl.candidates_enqueued != null) return "indexer.discovery.summary";
  if (raw === "indexer.scope.status" || raw.indexOf("indexer.scope.status") === 0) return "indexer.scope.status";
  if (raw === "indexer scope status" && fl.workspace_files_total != null) return "indexer.scope.status";
  if (raw === "indexer.scope.active_file" || raw.indexOf("indexer.scope.active_file") === 0) return "indexer.scope.active_file";
  if (raw === "indexer scope active file" && fl.rel != null && fl.indexer_target_key != null)
    return "indexer.scope.active_file";
  if (raw === "indexer.scan.complete" || raw.indexOf("indexer.scan.complete") === 0) return "indexer.scan.complete";
  if (raw === "scan fan-out budget" && fl.n_scopes != null) return "indexer.scan.complete";
  if (raw === "gateway.indexer.config" || raw.indexOf("gateway.indexer.config") === 0) return "gateway.indexer.config";
  if (raw === "gateway indexer config" && (fl.gateway_version != null || fl.embedding_model != null)) return "gateway.indexer.config";
  if (raw === "indexer.supervised.wait_roots" || raw.indexOf("indexer.supervised.wait_roots") === 0)
    return "indexer.supervised.wait_roots";
  if (raw === "indexer.run.start" || raw.indexOf("indexer.run.start") === 0) return "indexer.run.start";
  if (
    raw === "indexer run start" &&
    (fl.watch_root_paths != null || fl.root_ids != null || fl.root_scopes != null || fl.roots != null)
  )
    return "indexer.run.start";
  if (raw === "indexer.job.ingested") return "indexer.job.ingested";
  if (raw === "ingested" && fl.chunks != null) return "indexer.job.ingested";
  if (raw === "indexer.job.upload") return "indexer.job.upload";
  if (raw === "job upload" && fl.rel != null) return "indexer.job.upload";
  if (raw === "indexer.job.failed") return "indexer.job.failed";
  if (raw === "ingest failed (dropped)") return "indexer.job.failed";
  if (raw === "indexer.job.skipped") return "indexer.job.skipped";
  if (raw === "job skipped" && (fl.rel != null || fl.skip_reason != null)) return "indexer.job.skipped";
  if (raw === "indexer.fanout.enqueue_failed" || raw.indexOf("indexer.fanout.enqueue_failed") === 0)
    return "indexer.fanout.enqueue_failed";
  if (raw === "failed to enqueue fan-out list job" && fl.candidates != null) return "indexer.fanout.enqueue_failed";
  if (raw === "indexer.fanout.remainder_blocked" || raw.indexOf("indexer.fanout.remainder_blocked") === 0)
    return "indexer.fanout.remainder_blocked";
  if (raw.indexOf("queue full while retaining fan-out remainder") === 0) return "indexer.fanout.remainder_blocked";
  if (raw === "indexer.work.failed" || raw.indexOf("indexer.work.failed") === 0) return "indexer.work.failed";
  if (raw === "work item failed (dropped)" && fl.kind != null) return "indexer.work.failed";
  if (raw === "indexer.sync_state.write_failed" || raw.indexOf("indexer.sync_state.write_failed") === 0)
    return "indexer.sync_state.write_failed";
  if (raw === "sync state write failed" && fl.rel != null) return "indexer.sync_state.write_failed";
  if (raw === "rag.retrieve.source" || raw.indexOf("rag.retrieve.source") === 0) return "rag.retrieve.source";
  if (raw === "rag retrieved hits from source" && fl.rel != null && fl.source_hits != null)
    return "rag.retrieve.source";
  return raw;
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
  // Match backend IndexerKey inputs: tenant + effective project + flavor (no workspace).
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
  // flavor_id is allowed to be empty (default flavor); keep it in the key so
  // gateway retrieval lines can bucket with indexer scope cards.
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

function numOrDash(v) {
  var n = Number(v);
  return isNaN(n) ? null : n;
}

function formatIntCount(v) {
  var n = numOrDash(v);
  return n != null ? String(Math.round(n)) : "?";
}

/** Short label for WorkKind in logs (non-ingest failures). */
function workKindShortLabel(flat) {
  var k = flat.kind;
  if (k === 1 || k === "1") return "scan";
  if (k === 2 || k === "2") return "fanout list";
  if (k === 0 || k === "0") return "ingest";
  var s = k != null ? String(k).trim() : "";
  if (!s) return "?";
  if (s === "WorkScan" || s.toLowerCase() === "workscan") return "scan";
  if (s === "WorkFanoutList") return "fanout list";
  if (s === "WorkIngest") return "ingest";
  return s;
}

/** Short explanation for ingest failure `err` strings (HTTP paths + JSON are noisy in summaries). */
function shortIngestFailureDetail(flat) {
  var e = flat.err != null ? String(flat.err) : "";
  if (!e) return "";
  var el = e.toLowerCase().replace(/\s+/g, " ");
  if (el.indexOf("unknown or expired session") >= 0)
    return "chunked upload session missing or expired on gateway (restart, long stall, or different host)";
  if (
    el.indexOf("/v1/ingest/session/") >= 0 &&
    el.indexOf("/complete") >= 0 &&
    (el.indexOf("404") >= 0 || el.indexOf("status 404") >= 0)
  )
    return "ingest /complete returned 404 — session gone before upload finished";
  return e.replace(/\s+/g, " ").slice(0, 140);
}

/** Operator-facing one-line description for indexer structured logs. */
function indexerProseSummary(flat) {
  if (!flat || typeof flat !== "object") return null;
  var m = indexerFlatMsg(flat);
  var svc = String(flat.service || "").toLowerCase();
  var indexerish =
    svc === "indexer" ||
    m.indexOf("indexer.") === 0 ||
    m.indexOf("gateway.indexer") === 0 ||
    (svc === "gateway" && m === "rag.retrieve.source");
  if (!indexerish) return null;

  switch (m) {
    case "rag.retrieve.source": {
      var nRag = numOrDash(flat.source_hits);
      return (
        "RAG retrieved · " +
        (nRag != null ? nRag + " hit(s) · " : "") +
        String(flat.rel != null ? flat.rel : "?")
      );
    }
    case "indexer.supervised.wait_roots":
      return (
        "Waiting for at least one watch root" +
        (flat.config_path != null && String(flat.config_path).trim() !== ""
          ? " in " + String(flat.config_path).trim()
          : "")
      );
    case "indexer.state": {
      var st = indexerDeclaredStateLabel(flat.state);
      var bits = [];
      if (st) bits.push(st);
      var qd = numOrDash(flat.queue_depth);
      if (qd != null) bits.push("queue depth " + qd);
      var infl = numOrDash(flat.ingest_inflight);
      if (infl != null && infl > 0) bits.push("uploads in flight " + infl);
      return bits.length ? bits.join(" · ") : "Indexer status update";
    }
    case "indexer.storage.stats": {
      var pts = numOrDash(flat.qdrant_points);
      var avail = flat.available === true || flat.available === "true";
      if (pts != null) return "Indexed vectors: " + pts;
      if (!avail && flat.detail) return "Storage stats unavailable: " + String(flat.detail).slice(0, 120);
      return "Storage stats sync";
    }
    case "gateway.indexer.config":
      return (
        "Gateway indexer settings loaded (chunk " +
        (flat.chunk_size != null ? flat.chunk_size : "?") +
        ", model " +
        (flat.embedding_model ? String(flat.embedding_model).split("/").pop() : "?") +
        ")"
      );
    case "indexer.run.start": {
      var nroots = flat.roots != null ? flat.roots : "?";
      return "Indexer started · " + nroots + " watch root(s) · roots " + (flat.root_ids || "—");
    }
    case "indexer.discovery.summary":
      return (
        "Discovery: " +
        (flat.candidates_discovered != null ? flat.candidates_discovered : "?") +
        " candidates, " +
        (flat.files_excluded_by_ignore_rules != null ? flat.files_excluded_by_ignore_rules : flat.skipped_ignored || "?") +
        " paths excluded by ignore rules"
      );
    case "indexer.discovery.summary.scope":
      return (
        "Discovery · " +
        (flat.ingest_project != null ? String(flat.ingest_project) : "?") +
        " / " +
        (flat.flavor_id != null ? String(flat.flavor_id) : "?") +
        " · " +
        (flat.candidates_discovered != null ? flat.candidates_discovered : "?") +
        " files · " +
        (flat.path_sample_count != null ? flat.path_sample_count : "?") +
        " path(s) logged" +
        (flat.paths_truncated ? " (truncated)" : "")
      );
    case "indexer.scan.complete":
      return (
        "Scan done · " +
        (flat.n_scopes != null ? flat.n_scopes : "?") +
        " scope(s) · budget " +
        (flat.per_scope_fanout_budget != null ? flat.per_scope_fanout_budget : "?") +
        " pending bulk per scope · cap " +
        (flat.queue_cap != null ? flat.queue_cap : "?")
      );
    case "indexer.scope.status": {
      var wst = formatIntCount(flat.workspace_files_total);
      var qIn = formatIntCount(flat.queue_ingest_pending);
      var qFan = formatIntCount(flat.queue_fanout_files_pending);
      return (
        "~" +
        wst +
        " files in workspace · " +
        qIn +
        " waiting to embed · " +
        qFan +
        " waiting in discovery queue"
      );
    }
    case "indexer.scope.active_file":
      return (
        "Indexing · project " +
        (flat.project_id != null ? String(flat.project_id) : flat.ingest_project != null ? String(flat.ingest_project) : "?") +
        " · relative path " +
        (flat.rel || "?")
      );
    case "indexer.reconcile.summary":
      return (
        "Corpus inventory loaded · " +
        (flat.remote_source_paths != null ? flat.remote_source_paths : "?") +
        " remote source path(s)"
      );
    case "indexer.queue.snapshot": {
      var dep = numOrDash(flat.queue_depth);
      var comp = numOrDash(flat.ingest_completed);
      var dStr = dep != null ? String(dep) : "?";
      var cStr = comp != null ? String(comp) : "?";
      var idleLead = dep === 0 ? "idle · " : "";
      return "Queue · " + idleLead + dStr + " waiting · " + cStr + " completed this run";
    }
    case "indexer.run.progress":
      return (
        "Progress · " +
        (flat.phase ? String(flat.phase) : "phase") +
        (flat.candidates_enqueued != null ? " · " + flat.candidates_enqueued + " enqueued" : "")
      );
    case "indexer.job.upload": {
      var tr = flat.transport ? String(flat.transport) : "whole";
      var sz = flat.bytes != null ? formatBytesShort(flat.bytes) : "?";
      return "Uploading · " + (flat.rel || "file") + " · " + sz + " · " + tr;
    }
    case "indexer.job.ingested":
    case "ingested":
      return "Ingested · " + (flat.rel || "file") + " · " + (flat.chunks != null ? flat.chunks + " chunk(s)" : "done");
    case "indexer.job.skipped":
      return (
        "Skipped · " +
        (flat.rel || "file") +
        (flat.skip_reason ? " · " + String(flat.skip_reason).replace(/_/g, " ") : "")
      );
    case "indexer.retry.scheduled":
      return (
        "Retry scheduled · " +
        (flat.rel || "file") +
        " · attempt " +
        (flat.attempt != null ? flat.attempt : "?") +
        " · backoff " +
        (flat.delay_ms != null ? flat.delay_ms + " ms" : "?")
      );
    case "indexer.recovery.poll":
      return (
        "Recovery poll #" +
        (flat.poll_n != null ? flat.poll_n : "?") +
        " · storage " +
        (flat.storage_ok === true ? "OK" : flat.storage_ok === false ? "not OK" : "?")
      );
    case "indexer.recovery.resumed":
      return "Storage recovered — resuming workers";
    case "indexer.worker.paused":
      return "Worker paused for recovery · " + (flat.rel || "pending job");
    case "indexer.run.done":
    case "indexer run done":
    case "indexer run stopped":
      return (
        "Run finished · mode " +
        (flat.mode || "?") +
        " · ingested " +
        (flat.ingest_completed != null ? flat.ingest_completed : "?") +
        " · failures " +
        (flat.ingest_failed_dropped != null ? flat.ingest_failed_dropped : "?")
      );
    case "indexer.fanout.enqueue_failed": {
      var nc = formatIntCount(flat.candidates);
      return "Couldn't queue discovery batch (queue full?) · " + nc + " paths";
    }
    case "indexer.fanout.remainder_blocked":
      return "Could not re-queue remaining discovery work";
    case "indexer.work.failed":
      return "Background job failed · " + workKindShortLabel(flat) + " · dropped";
    case "indexer.sync_state.write_failed":
      return "Couldn't save sync checkpoint after ingest · " + (flat.rel != null ? String(flat.rel) : "?");
    default:
      if (m.indexOf("indexer.job.failed") === 0 || m.indexOf("ingest failed (dropped)") === 0) {
        var tail = shortIngestFailureDetail(flat);
        return (
          "Ingest failed (dropped) · " +
          (flat.rel || "?") +
          (tail ? " · " + tail : "")
        );
      }
      return null;
  }
}

function formatBytesShort(n) {
  var x = Number(n);
  if (isNaN(x)) return "?";
  if (x < 1024) return x + " B";
  if (x < 1024 * 1024) return (x / 1024).toFixed(1) + " KB";
  return (x / (1024 * 1024)).toFixed(1) + " MB";
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.indexerDeclaredStateLabel = indexerDeclaredStateLabel;
globalThis.ChimeraLogs.Derive.indexerSlugHistogramBucket = indexerSlugHistogramBucket;
globalThis.ChimeraLogs.Derive.indexerGroupKeyFromFlat = indexerGroupKeyFromFlat;
globalThis.ChimeraLogs.Derive.indexerFlatMsgForPresent = indexerFlatMsg;
globalThis.ChimeraLogs.Derive.indexerProseSummary = indexerProseSummary;
