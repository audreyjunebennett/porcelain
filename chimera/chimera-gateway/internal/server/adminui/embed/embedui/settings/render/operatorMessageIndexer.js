/**
 * Indexer operator formatters (Phase 4). Merges into ChimeraSettings.Render operator formatters.
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
  var base = globalThis.ChimeraSettings.Render._operatorFormatters || {};

  function numOrDash(v) {
    var n = Number(v);
    return isNaN(n) ? null : n;
  }

  function formatIntCount(v) {
    var n = numOrDash(v);
    return n != null ? String(Math.round(n)) : "?";
  }

  function formatBytesShort(n) {
    var x = Number(n);
    if (isNaN(x)) return "?";
    if (x < 1024) return x + " B";
    if (x < 1024 * 1024) return (x / 1024).toFixed(1) + " KB";
    return (x / (1024 * 1024)).toFixed(1) + " MB";
  }

  function indexerStateLabel(code) {
    var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
    var labels = oc && oc.indexerStateLabels;
    var key = String(code || "").trim();
    if (labels && key && labels[key]) return labels[key];
    return key ? key : "";
  }

  function indexerScopeLabel(flat) {
    var proj =
      flat.ingest_project != null && String(flat.ingest_project).trim() !== ""
        ? String(flat.ingest_project).trim()
        : flat.project_id != null && String(flat.project_id).trim() !== ""
          ? String(flat.project_id).trim()
          : "";
    return proj;
  }

  function skipSummaryTotals(flat) {
    return (
      (Number(flat.skip_unchanged_local_sync) || 0) +
      (Number(flat.skip_unchanged_corpus_client_hash) || 0) +
      (Number(flat.skip_unchanged_corpus_sync) || 0) +
      (Number(flat.skip_empty_or_whitespace) || 0)
    );
  }

  function windowSecondsLabel(flat) {
    if (flat.window_ms == null || isNaN(Number(flat.window_ms))) return "?";
    var sec = Math.round(Number(flat.window_ms) / 1000);
    return sec === 1 ? "1 second" : sec + " seconds";
  }

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

  var ix = {
    indexer_rag_source: function (flat) {
      var nRag = numOrDash(flat.source_hits);
      return (
        "RAG retrieved · " +
        (nRag != null ? nRag + " hit(s) · " : "") +
        String(flat.rel != null ? flat.rel : "?")
      );
    },
    indexer_supervised_hot_reload: function (flat, entry) {
      var slug = entry && entry.slug ? entry.slug : "";
      if (slug === "indexer.supervised.hot_reload_yaml_error") {
        return "Couldn't reload indexer settings — config file has an error; keeping the current session";
      }
      if (slug === "indexer.supervised.hot_reload_resolve_error") {
        var err = flat.err != null ? String(flat.err).replace(/\s+/g, " ").trim() : "";
        return err
          ? "Couldn't reload indexer settings — " + err.slice(0, 120)
          : "Couldn't reload indexer settings — configuration could not be applied";
      }
      return "Indexer settings changed — restarting the watch session";
    },
    indexer_supervised_wait_roots: function (flat, entry) {
      var base = entry.summary || "Waiting for at least one watch root";
      if (flat.config_path != null && String(flat.config_path).trim() !== "")
        return base + " in " + String(flat.config_path).trim();
      return base;
    },
    indexer_supervised_workspaces_reload_stalled: function (flat, entry) {
      var base = entry.summary || "Workspace reload stalled";
      if (flat.timeout_sec != null) return base + " · waited " + flat.timeout_sec + "s";
      return base;
    },
    indexer_supervised_workspaces_changed: function (flat, entry) {
      var base = entry.summary || "Workspace paths changed";
      var n = flat.roots != null ? flat.roots : null;
      if (n == null && flat.watch_root_paths) {
        n = Array.isArray(flat.watch_root_paths)
          ? flat.watch_root_paths.length
          : String(flat.watch_root_paths).split(",").filter(Boolean).length;
      }
      var bits = [base];
      if (n != null) bits.push(n + " watch root(s)");
      if (flat.new_paths_hash != null && String(flat.new_paths_hash).trim() !== "")
        bits.push("hash " + String(flat.new_paths_hash).trim());
      var paths = flat.watch_root_paths;
      if (paths) {
        var list = Array.isArray(paths) ? paths : String(paths).split(",");
        var sample = list
          .map(function (p) {
            return String(p || "").trim();
          })
          .filter(Boolean)
          .slice(-1)[0];
        if (sample) {
          var tail = sample.replace(/^.*[/\\]/, "");
          if (tail) bits.push("includes " + tail);
        }
      }
      return bits.join(" · ");
    },
    indexer_supervised_workspaces_reload: function (flat, entry) {
      var base = entry.summary || "Reloading watch session";
      var n = flat.roots != null ? flat.roots : null;
      if (n == null && flat.watch_root_paths) {
        n = Array.isArray(flat.watch_root_paths)
          ? flat.watch_root_paths.length
          : String(flat.watch_root_paths).split(",").filter(Boolean).length;
      }
      return n != null ? base + " · " + n + " watch root(s)" : base + " · paths updated";
    },
    indexer_supervised_workspaces_session_start: function (flat, entry) {
      var base = entry.summary || "Watch session restarted";
      var n = flat.roots != null ? flat.roots : "?";
      return base + " · " + n + " watch root(s)";
    },
    indexer_supervised_workspaces_apply_failed: function (flat, entry) {
      var base = entry.summary || "Couldn't apply workspace paths";
      if (flat.err != null && String(flat.err).trim() !== "")
        return base + " · " + String(flat.err).trim().slice(0, 120);
      return base;
    },
    gateway_operator_workspace_path_added: function (flat, entry) {
      var base = entry.summary || "Watch path added";
      if (flat.path != null && String(flat.path).trim() !== "") {
        var p = String(flat.path).trim();
        var tail = p.replace(/^.*[/\\]/, "") || p;
        return base + " · " + tail;
      }
      return base;
    },
    gateway_operator_workspace_path_deleted: function (flat, entry) {
      var base = entry.summary || "Watch path removed";
      if (flat.path != null && String(flat.path).trim() !== "") {
        var p = String(flat.path).trim();
        var tail = p.replace(/^.*[/\\]/, "") || p;
        return base + " · " + tail;
      }
      return base;
    },
    indexer_supervised_watch_shutdown_timeout: function (flat, entry) {
      var base = entry.summary || "Filesystem watchers did not stop after reload";
      if (flat.grace_sec != null) return base + " · grace " + flat.grace_sec + "s";
      return base;
    },
    indexer_supervised_session_fatal_exit: function (flat, entry) {
      var base = entry.summary || "Indexer watch session exited unexpectedly";
      if (flat.err != null && String(flat.err).trim() !== "")
        return base + " · " + String(flat.err).trim().slice(0, 120);
      return base;
    },
    indexer_supervised_process_exit: function (flat, entry) {
      var base = entry.summary || "Supervised indexer stopped";
      if (flat.err != null && String(flat.err).trim() !== "")
        return base + " · " + String(flat.err).trim().slice(0, 120);
      return base;
    },
    indexer_state: function (flat) {
      var st = indexerStateLabel(flat.state);
      var bits = [];
      if (st) bits.push(st);
      var qd = numOrDash(flat.queue_depth);
      if (qd != null) bits.push("queue depth " + qd);
      var infl = numOrDash(flat.ingest_inflight);
      if (infl != null && infl > 0) bits.push("uploads in flight " + infl);
      return bits.length ? bits.join(" · ") : "Indexer status update";
    },
    indexer_storage_stats: function (flat, entry) {
      var avail = flat.available === true || flat.available === "true";
      if (!avail) {
        if (flat.detail) {
          return (
            "Could not verify the stored search index — " +
            String(flat.detail).slice(0, 120)
          );
        }
        return "Could not verify the stored search index right now";
      }
      var pts = numOrDash(flat.qdrant_points);
      if (pts === 0) {
        return "Search index is empty — no embedded file content is stored for this workspace yet";
      }
      if (pts != null) {
        return (
          "Stored search index looks healthy — " +
          pts +
          " embedded text chunk(s) from your indexed files are saved and ready for retrieval"
        );
      }
      return "Verified the stored search index is reachable";
    },
    indexer_gateway_config: function (flat) {
      return (
        "Gateway indexer settings loaded (chunk " +
        (flat.chunk_size != null ? flat.chunk_size : "?") +
        ", model " +
        (flat.embedding_model ? String(flat.embedding_model).split("/").pop() : "?") +
        ")"
      );
    },
    indexer_run_start: function (flat, entry) {
      var nroots = numOrDash(flat.roots);
      var paths = flat.watch_root_paths;
      var labels = [];
      if (paths) {
        var list = Array.isArray(paths)
          ? paths
          : String(paths)
              .split(",")
              .map(function (p) {
                return String(p || "").trim();
              })
              .filter(Boolean);
        var pi;
        for (pi = 0; pi < list.length; pi++) {
          var tail = String(list[pi]).replace(/^.*[/\\]/, "") || String(list[pi]);
          if (tail && labels.indexOf(tail) < 0) labels.push(tail);
        }
      }
      var pathPhrase = "";
      if (labels.length === 1) pathPhrase = labels[0];
      else if (labels.length === 2) pathPhrase = labels[0] + " and " + labels[1];
      else if (labels.length > 2) {
        pathPhrase = labels.slice(0, 3).join(", ") + ", and " + (labels.length - 3) + " more";
      }
      var dirWord = nroots === 1 ? "directory" : "directories";
      if (nroots != null && pathPhrase) {
        return "Watching " + nroots + " " + dirWord + ": " + pathPhrase;
      }
      if (nroots != null) {
        return "Watching " + nroots + " " + dirWord + " for file changes";
      }
      return entry.summary || "Indexer started";
    },
    indexer_discovery: function (flat, entry) {
      var slug = entry && entry.slug ? entry.slug : "";
      if (slug === "indexer.discovery.summary.scope") {
        var n = flat.candidates_discovered != null ? flat.candidates_discovered : "?";
        return (
          "Discovery scan found " +
          n +
          " file(s) in this workspace's directories"
        );
      }
      if (slug === "indexer.scan.complete") {
        var nWs = flat.n_scopes != null ? flat.n_scopes : "?";
        var budget = flat.per_scope_fanout_budget != null ? flat.per_scope_fanout_budget : "?";
        var cap = flat.queue_cap != null ? flat.queue_cap : "?";
        var wsWord = Number(nWs) === 1 ? "workspace" : "workspaces";
        return (
          "Directory walk finished across " +
          nWs +
          " " +
          wsWord +
          " — each workspace gets up to " +
          budget +
          " pending file slot(s) in the ingest queue (" +
          cap +
          " total queue capacity)"
        );
      }
      return (
        "Discovery: " +
        (flat.candidates_discovered != null ? flat.candidates_discovered : "?") +
        " candidates, " +
        (flat.files_excluded_by_ignore_rules != null ? flat.files_excluded_by_ignore_rules : flat.skipped_ignored || "?") +
        " paths excluded by ignore rules"
      );
    },
    indexer_scope_status: function (flat) {
      if (flat.ingest_gate_closed === true) {
        var gateReason =
          flat.ingest_gate_reason_code != null
            ? String(flat.ingest_gate_reason_code).replace(/_/g, " ")
            : flat.embed_reason_code != null
              ? String(flat.embed_reason_code).replace(/_/g, " ")
              : "ingest paused";
        return "Indexing is paused because " + gateReason;
      }
      if (flat.in_recovery === true) {
        return "Waiting to resume — embedding or storage is still recovering";
      }
      if (flat.current_rel) {
        return "Now embedding " + String(flat.current_rel);
      }
      var wst = numOrDash(flat.workspace_files_total);
      var qIn = numOrDash(flat.queue_ingest_pending);
      var qFan = numOrDash(flat.queue_fanout_files_pending);
      var reason = flat.change_reason != null ? String(flat.change_reason).trim() : "";
      var wStr = wst != null ? String(wst) : "?";
      var inStr = qIn != null ? String(qIn) : "?";
      var fanStr = qFan != null ? String(qFan) : "?";
      if (qIn === 0 && qFan === 0) {
        if (reason === "heartbeat") {
          return (
            "Everything looks up to date — tracking " +
            wStr +
            " files and nothing needs to be indexed right now"
          );
        }
        return (
          "Indexing is idle with nothing in the queue — " + wStr + " files tracked in this workspace"
        );
      }
      if (qFan > 0 && qIn === 0) {
        return (
          "Still working through discovered files — " +
          fanStr +
          " of " +
          wStr +
          " tracked files are waiting to be examined"
        );
      }
      if (qIn > 0 && qFan === 0) {
        return (
          "Embedding in progress — " +
          inStr +
          " of " +
          wStr +
          " tracked files are waiting to be embedded"
        );
      }
      return (
        "Indexing in progress — " +
        wStr +
        " files tracked, " +
        fanStr +
        " waiting to be examined, and " +
        inStr +
        " waiting to be embedded"
      );
    },
    indexer_scope_active_file: function (flat) {
      return (
        "Indexing · project " +
        (flat.project_id != null ? String(flat.project_id) : flat.ingest_project != null ? String(flat.ingest_project) : "?") +
        " · relative path " +
        (flat.rel || "?")
      );
    },
    indexer_reconcile_summary: function (flat, entry) {
      var base = entry.summary || "Corpus inventory loaded";
      return (
        base +
        " · " +
        (flat.remote_source_paths != null ? flat.remote_source_paths : "?") +
        " remote source path(s)"
      );
    },
    indexer_queue_snapshot: function (flat) {
      var dep = numOrDash(flat.queue_depth);
      var comp = numOrDash(flat.ingest_completed);
      var dStr = dep != null ? String(dep) : "?";
      var cStr = comp != null ? String(comp) : "?";
      if (dep === 0 && (comp === 0 || comp == null)) {
        return "Ingest workers idle · embed queue empty · 0 files embedded this run";
      }
      return (
        "Ingest queue · " +
        dStr +
        " file(s) waiting for workers · " +
        cStr +
        " embedded this run"
      );
    },
    indexer_run_progress: function (flat, entry) {
      var phase = flat.phase != null ? String(flat.phase).trim() : "";
      var nRoots = numOrDash(flat.roots);
      if (phase === "scan_scheduled") {
        if (nRoots != null) {
          var dirWord = Number(nRoots) === 1 ? "directory" : "directories";
          return "Starting a scan of " + nRoots + " " + dirWord + " for files";
        }
        return "Starting a scan of directories for files";
      }
      if (phase === "initial_scan") {
        var total = numOrDash(flat.candidates_total);
        if (total != null && total > 0) {
          return (
            "Initial scan finished — found " +
            total +
            " file(s) and queued them for examination"
          );
        }
        if (flat.candidates_enqueued != null && Number(flat.candidates_enqueued) === 0) {
          return "Initial scan finished — no indexable files found in workspace directories";
        }
      }
      if (phase) {
        return "Indexer scan update — " + phase.replace(/_/g, " ");
      }
      return entry.summary || "Indexer scan in progress";
    },
    indexer_job_upload: function (flat) {
      var tr = flat.transport ? String(flat.transport) : "whole";
      var sz = flat.bytes != null ? formatBytesShort(flat.bytes) : "?";
      return "Uploading · " + (flat.rel || "file") + " · " + sz + " · " + tr;
    },
    indexer_job_ingested: function (flat) {
      return "Ingested · " + (flat.rel || "file") + " · " + (flat.chunks != null ? flat.chunks + " chunk(s)" : "done");
    },
    indexer_job_ingested_summary: function (flat) {
      var n = Number(flat.ingest_succeeded) || 0;
      var chunks = Number(flat.chunks_total) || 0;
      var win =
        flat.window_ms != null && !isNaN(Number(flat.window_ms))
          ? Math.round(Number(flat.window_ms) / 1000) + "s"
          : "?";
      var line = "Indexed " + n + " file(s) · " + chunks + " chunk(s)";
      if (flat.last_rel) line += " · last " + String(flat.last_rel);
      return line + " · last " + win;
    },
    indexer_job_skipped: function (flat) {
      return (
        "Skipped · " +
        (flat.rel || "file") +
        (flat.skip_reason ? " · " + String(flat.skip_reason).replace(/_/g, " ") : "")
      );
    },
    indexer_job_skipped_summary: function (flat) {
      var skip = skipSummaryTotals(flat);
      var ing = Number(flat.ingest_succeeded) || 0;
      var fail = Number(flat.ingest_failed) || 0;
      var evaluated = numOrDash(flat.files_evaluated);
      var win = windowSecondsLabel(flat);
      var evalN = evaluated != null ? evaluated : skip;
      if (evaluated != null && skip === evaluated && ing === 0 && fail === 0) {
        return (
          "Checked " +
          evalN +
          " files in the last " +
          win +
          " — everything was already indexed, so nothing new was embedded"
        );
      }
      if (evaluated != null && skip === 0 && ing > 0 && fail === 0) {
        return (
          "Checked " +
          evalN +
          " files in the last " +
          win +
          " — embedded " +
          ing +
          " new file(s)"
        );
      }
      var line =
        "Checked " +
        evalN +
        " files in the last " +
        win +
        " — " +
        skip +
        " unchanged";
      if (ing > 0) line += ", " + ing + " newly embedded";
      else line += ", nothing newly embedded";
      if (fail > 0) line += ", and " + fail + " failed";
      return line;
    },
    indexer_retry_recovery: function (flat, entry) {
      var slug = entry && entry.slug ? entry.slug : "";
      if (slug === "indexer.retry.scheduled") {
        return (
          "Retry scheduled · " +
          (flat.rel || "file") +
          " · attempt " +
          (flat.attempt != null ? flat.attempt : "?") +
          " · backoff " +
          (flat.delay_ms != null ? flat.delay_ms + " ms" : "?")
        );
      }
      if (slug === "indexer.recovery.poll") {
        var storage =
          flat.storage_ok === true ? "OK" : flat.storage_ok === false ? "not OK" : "?";
        var embed =
          flat.embed_ok === true ? "OK" : flat.embed_ok === false ? "not OK" : "?";
        var line =
          "Recovery poll #" +
          (flat.poll_n != null ? flat.poll_n : "?") +
          " · storage " +
          storage +
          " · embed " +
          embed;
        if (flat.embed_ok === false && flat.embed_reason_code) {
          line += " · " + String(flat.embed_reason_code).replace(/_/g, " ");
        }
        return line;
      }
      if (slug === "indexer.worker.paused") {
        return "Worker paused for recovery · " + (flat.rel || "pending job");
      }
      return entry.summary || "";
    },
    indexer_ingest_gate: function (flat, entry) {
      var slug = entry && entry.slug ? entry.slug : "";
      if (slug === "indexer.ingest.gate.closed") {
        var reason = flat.reason_code ? String(flat.reason_code).replace(/_/g, " ") : "ingest blocked";
        var q = flat.queue_depth != null ? flat.queue_depth : "?";
        return "Ingest paused — " + reason + " · queue " + q;
      }
      if (slug === "indexer.ingest.gate.open") {
        var qOpen = flat.queue_depth != null ? flat.queue_depth : "?";
        var model = flat.embed_model ? String(flat.embed_model) : "";
        return "Ingest resumed · queue " + qOpen + (model ? " · " + model : "");
      }
      return entry.summary || "";
    },
    indexer_run_done: function (flat) {
      return (
        "Run finished · mode " +
        (flat.mode || "?") +
        " · ingested " +
        (flat.ingest_completed != null ? flat.ingest_completed : "?") +
        " · failures " +
        (flat.ingest_failed_dropped != null ? flat.ingest_failed_dropped : "?")
      );
    },
    indexer_fanout: function (flat, entry) {
      if (entry && entry.slug === "indexer.fanout.enqueue_failed") {
        var nc = formatIntCount(flat.candidates);
        return "Couldn't queue discovery batch (queue full?) · " + nc + " paths";
      }
      return entry.summary || "Could not re-queue remaining discovery work";
    },
    indexer_work_failed: function (flat) {
      return "Background job failed · " + workKindShortLabel(flat) + " · dropped";
    },
    indexer_sync_state_failed: function (flat, entry) {
      var base = entry.summary || "Couldn't save sync checkpoint after ingest";
      return base + " · " + (flat.rel != null ? String(flat.rel) : "?");
    },
    indexer_job_failed: function (flat) {
      var tail = shortIngestFailureDetail(flat);
      return "Ingest failed (dropped) · " + (flat.rel || "?") + (tail ? " · " + tail : "");
    }
  };

  Object.assign(base, ix);
  globalThis.ChimeraSettings.Render._operatorFormatters = base;
  globalThis.ChimeraSettings.Render.shortIngestFailureDetail = shortIngestFailureDetail;
  globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
  globalThis.ChimeraSettings.Derive.shortIngestFailureDetail = shortIngestFailureDetail;
})();
