/**
 * Registry-driven operator headlines (gateway, broker, vectorstore).
 * Requires ChimeraSettings.OperatorCopy (operator_copy.js) and operatorMessageServices.js.
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
  var formatters = (globalThis.ChimeraSettings.Render._operatorFormatters = globalThis.ChimeraSettings.Render._operatorFormatters || {});

  function flatMsg(flat) {
    if (!flat || typeof flat !== "object") return "";
    var raw = flat.msg != null ? flat.msg : flat.message != null ? flat.message : "";
    return String(raw).trim();
  }

  function ragCollectionSuffix(flat) {
    var collRaw = flat.collection != null ? String(flat.collection).trim() : "";
    if (!collRaw) return "";
    if (typeof ragCollectionLabelForUi === "function") {
      var collLab = ragCollectionLabelForUi(collRaw);
      if (collLab) return " Reading collection " + collLab + ".";
    }
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.vectorstoreCollectionDisplay === "function") {
      var lab2 = ChimeraSettings.Derive.vectorstoreCollectionDisplay(collRaw);
      if (lab2 != null && String(lab2).trim() !== "") return " Reading collection " + String(lab2).trim() + ".";
    }
    return " Reading collection " + collRaw + ".";
  }

  function brokerShortModel(model) {
    var m = model != null ? String(model).trim() : "";
    if (!m) return "";
    var parts = m.split("/");
    var tail = parts[parts.length - 1] || m;
    return tail.length > 48 ? tail.slice(0, 46) + "…" : tail;
  }

  function formatConvDurationMs(ms) {
    var n = Number(ms);
    if (isNaN(n) || n < 0) return "";
    if (n >= 1000) {
      var sec = n / 1000;
      return (sec >= 10 ? Math.round(sec) : Math.round(sec * 10) / 10) + " s";
    }
    return Math.round(n) + " ms";
  }

  function formatEstInputTokens(n) {
    var t = Number(n);
    if (isNaN(t) || t <= 0) return "";
    return "~" + Math.round(t).toLocaleString() + " input tokens (estimated)";
  }

  function formatUsageTokens(n) {
    var t = Number(n);
    if (isNaN(t) || t <= 0) return "";
    return Math.round(t).toLocaleString() + " tokens used (prompt + completion)";
  }

  function extractOpenAIErrorMessage(raw) {
    var er = String(raw || "").replace(/\s+/g, " ").trim();
    if (!er) return "";
    var msgMatch = er.match(/"message"\s*:\s*"((?:[^"\\]|\\.)*)"/);
    if (msgMatch && msgMatch[1]) {
      var inner = msgMatch[1].replace(/\\"/g, '"').replace(/\\n/g, " ").trim();
      if (inner.length > 220) inner = inner.slice(0, 219) + "…";
      return inner;
    }
    if (er.charAt(0) === "{") {
      try {
        var root = JSON.parse(er);
        if (root && root.error && root.error.message) {
          var m = String(root.error.message).trim();
          return m.length > 220 ? m.slice(0, 219) + "…" : m;
        }
      } catch (eJson) {
        /* fall through */
      }
    }
    if (er.length > 220) er = er.slice(0, 219) + "…";
    return er;
  }

  function convEvlogMetaFromOpts(opts) {
    opts = opts || {};
    return opts.convEvlogMeta && typeof opts.convEvlogMeta === "object" ? opts.convEvlogMeta : null;
  }

  function virtualModelIdFromMetaOrFlat(flat, meta) {
    if (flat.virtualModelId != null && String(flat.virtualModelId).trim() !== "") {
      return String(flat.virtualModelId).trim();
    }
    if (flat.virtual_model_id != null && String(flat.virtual_model_id).trim() !== "") {
      return String(flat.virtual_model_id).trim();
    }
    if (meta && meta.routingSummary && meta.routingSummary.virtualModelId) {
      return meta.routingSummary.virtualModelId;
    }
    var cache = globalThis.gatewayOverviewCache;
    if (cache && cache.virtual_model_id != null && String(cache.virtual_model_id).trim() !== "") {
      return String(cache.virtual_model_id).trim();
    }
    return "";
  }

  function isRoutingPassthrough(flat, meta) {
    if (flat.routingPassthrough === true || flat.routing_passthrough === true) return true;
    var rs = meta && meta.routingSummary ? meta.routingSummary : null;
    var client = flat.clientModel != null ? String(flat.clientModel).trim() : rs && rs.clientModel ? rs.clientModel : "";
    var upstream = flat.upstreamModel != null ? String(flat.upstreamModel).trim() : rs && rs.upstream ? rs.upstream : "";
    var chain = flat.chainLen != null ? Number(flat.chainLen) : rs ? Number(rs.chainLen) : NaN;
    var virtualId = virtualModelIdFromMetaOrFlat(flat, meta);
    if (client && virtualId && client !== virtualId && client === upstream) return true;
    if (!isNaN(chain) && chain <= 1 && client && upstream && client === upstream) return true;
    return false;
  }

  function summarizeRagRetrieveErr(rawErr) {
    var er = String(rawErr || "").replace(/\s+/g, " ").trim();
    if (!er) return "";
    var low = er.toLowerCase();
    if (low.indexOf("context length") >= 0 || low.indexOf("exceeds the context") >= 0)
      return "Embedding input too long for the model context window.";
    var msgMatch = er.match(/"message"\s*:\s*"((?:[^"\\]|\\.)*)"/);
    if (msgMatch && msgMatch[1]) {
      var inner = msgMatch[1].replace(/\\"/g, '"').replace(/\\n/g, " ").trim();
      if (inner.length > 220) inner = inner.slice(0, 219) + "…";
      return inner;
    }
    er = er.replace(/^embed query:\s*embed:\s*/i, "").trim();
    var stMatch = er.match(/\bstatus\s+(\d{3})\b/i);
    if (stMatch) {
      var code = stMatch[1];
      var idx = er.toLowerCase().indexOf("status " + code);
      var tail = idx >= 0 ? er.slice(idx + ("status " + code).length).replace(/^:\s*/, "").trim() : "";
      if (tail.charAt(0) === "{") {
        var nested = summarizeRagRetrieveErr(tail);
        if (nested) return "Embedding API rejected the query (HTTP " + code + "): " + nested;
      }
      if (tail.length > 120) tail = tail.slice(0, 119) + "…";
      return tail ? "Embedding API HTTP " + code + ": " + tail : "Embedding API returned HTTP " + code + ".";
    }
    if (er.length > 140) er = er.slice(0, 139) + "…";
    return er;
  }

  function urlHostTail(urlStr, maxLen) {
    maxLen = maxLen > 0 ? maxLen : 96;
    var url = urlStr != null ? String(urlStr).trim() : "";
    if (!url) return "";
    if (typeof URL === "function") {
      try {
        var u = new URL(url);
        return (u.host + (u.pathname === "/" ? "" : u.pathname)).slice(0, maxLen);
      } catch (eUrl) {
        /* fall through */
      }
    }
    var stripped = url.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "");
    return stripped.length > maxLen ? stripped.slice(0, maxLen - 1) + "…" : stripped;
  }

  formatters.rag_collection = function (flat, entry) {
      var slug = entry.slug || "";
      if (slug === "conversation.rag.span") {
        var collLab = "";
        var collRaw = flat.collection != null ? String(flat.collection).trim() : "";
        if (collRaw && typeof ragCollectionLabelForUi === "function") {
          collLab = ragCollectionLabelForUi(collRaw);
        }
        if (!collLab && collRaw) {
          var dash = collRaw.indexOf("-_-");
          collLab = dash >= 0 ? collRaw.slice(0, dash) : collRaw;
        }
        return collLab
          ? "Searched collection " + collLab + " · context attached for this turn."
          : "Searched collection · context attached for this turn.";
      }
      var base = entry.summary || "";
      return base + ragCollectionSuffix(flat);
  };
  formatters.rag_retrieve_error = function (flat, entry) {
    var baseErr = entry.summary || "RAG retrieval failed; continuing without injected chunks.";
    var rawEr = flat.err != null ? String(flat.err) : "";
    var sum = summarizeRagRetrieveErr(rawEr);
    return sum ? baseErr + " Cause: " + sum : baseErr;
  };
  formatters.conversation_turn_started = function (flat, entry, opts) {
      var meta = convEvlogMetaFromOpts(opts);
      var turnIdx =
        flat.turn_index != null && !isNaN(Number(flat.turn_index))
          ? Math.round(Number(flat.turn_index))
          : meta && meta.turnIndex != null
            ? meta.turnIndex
            : null;
      var client = flat.clientModel != null ? String(flat.clientModel).trim() : "";
      var msgCount = flat.message_count != null ? Number(flat.message_count) : flat.messageCount != null ? Number(flat.messageCount) : NaN;
      var bits = [];
      if (turnIdx != null) bits.push("Turn " + turnIdx + " started");
      else bits.push("Turn started");
      var showNew = meta ? meta.isNewConversation : turnIdx === 1;
      if (showNew) bits.push("new conversation");
      if (client) bits.push("client asked for " + client);
      if (!isNaN(msgCount) && msgCount > 0) {
        bits.push(msgCount + " message" + (msgCount === 1 ? "" : "s") + " in prompt");
      }
      return bits.join(" · ") + ".";
  };
  formatters.conversation_errored = function (flat, entry, opts) {
      opts = opts || {};
      if (opts.forEventLog !== true) {
        var scErr = flat.statusCode != null ? Number(flat.statusCode) : NaN;
        var msLegacy = flat.total_ms != null ? Number(flat.total_ms) : flat.totalMs != null ? Number(flat.totalMs) : NaN;
        var bitsLegacy = ["This conversation turn ended with an error (no successful completion delivered)."];
        if (!isNaN(scErr)) bitsLegacy.push("HTTP " + Math.round(scErr));
        if (!isNaN(msLegacy) && msLegacy >= 0) bitsLegacy.push(Math.round(msLegacy) + " ms");
        return bitsLegacy.join(" · ");
      }
      var ms = flat.total_ms != null ? Number(flat.total_ms) : flat.totalMs != null ? Number(flat.totalMs) : NaN;
      var dur = formatConvDurationMs(ms);
      return dur ? "Turn failed · " + dur + "." : "Turn failed.";
  };
  formatters.conversation_delivered = function (flat, entry, opts) {
      opts = opts || {};
      var ms = flat.total_ms != null ? Number(flat.total_ms) : flat.totalMs != null ? Number(flat.totalMs) : NaN;
      var dur = formatConvDurationMs(ms);
      if (opts.forEventLog === true) {
        return dur ? "Turn completed · " + dur + " · response delivered to client." : "Turn completed · response delivered to client.";
      }
      var bitsD = ["Completion delivered to the client (this turn finished successfully)."];
      if (!isNaN(ms) && ms >= 0) bitsD.push(Math.round(ms) + " ms");
      return bitsD.join(" · ");
  };
  formatters.conversation_routing = function (flat, entry, opts) {
      var meta = convEvlogMetaFromOpts(opts);
      var rs = meta && meta.routingSummary ? meta.routingSummary : null;
      var upstream = brokerShortModel(flat.upstreamModel || (rs && rs.upstream));
      var client =
        flat.clientModel != null ? String(flat.clientModel).trim() : rs && rs.clientModel ? rs.clientModel : "";
      var att = flat.attempt != null ? Number(flat.attempt) : rs ? Number(rs.attempt) : NaN;
      var chain = flat.chainLen != null ? Number(flat.chainLen) : rs ? Number(rs.chainLen) : NaN;
      var est = flat.outgoingTokens != null ? Number(flat.outgoingTokens) : flat.outgoing_tokens != null ? Number(flat.outgoing_tokens) : rs && !isNaN(Number(rs.outgoingTokens)) ? Number(rs.outgoingTokens) : NaN;

      if (isRoutingPassthrough(flat, meta)) {
        var passBits = ["Client model used as-is (not a configured virtual model)"];
        if (client || upstream) passBits.push("sent `" + (client || upstream) + "` to provider");
        var estPass = formatEstInputTokens(est);
        if (estPass) passBits.push(estPass);
        return passBits.join(" · ") + ".";
      }

      var virtualId = virtualModelIdFromMetaOrFlat(flat, meta) || client;
      var partsR = ["Routed virtual model " + virtualId + " → " + (upstream || "?")];
      if (!isNaN(att) && !isNaN(chain) && chain > 0) {
        partsR.push("attempt " + Math.round(att) + " of " + Math.round(chain));
      }
      if (rs && rs.skipped && rs.skipped.length) {
        var skipNames = [];
        var si;
        for (si = 0; si < rs.skipped.length; si++) skipNames.push(rs.skipped[si].model);
        var skipReason = rs.skipped[0].reason === "tpm" ? "provider TPM quota" : rs.skipped[0].reason || "quota";
        partsR.push("skipped " + rs.skipped.length + " (" + skipReason + "): " + skipNames.join(", "));
      }
      return partsR.join(" · ") + ".";
  };
  formatters.conversation_broker_started = function (flat, entry) {
      return "";
  };
  formatters.ingest_complete = function (flat, entry) {
      var bitsIc = [entry.summary || "Ingest finished — document indexed."];
      var ch = flat.chunks != null ? Number(flat.chunks) : NaN;
      if (!isNaN(ch) && ch >= 0) bitsIc.push(Math.round(ch) + " chunk" + (ch === 1 ? "" : "s"));
      var srcIc = flat.source != null ? String(flat.source).trim() : "";
      if (srcIc) bitsIc.push("source: " + (srcIc.length > 80 ? srcIc.slice(0, 79) + "…" : srcIc));
      var tenIc = flat.tenant != null ? String(flat.tenant).trim() : "";
      if (tenIc) bitsIc.push("tenant " + tenIc);
      return bitsIc.join(" · ");
  };
  formatters.gateway_auth_reloaded = function (flat, entry) {
      var baseAuth = entry.summary || "Client credentials reloaded from disk.";
      var nAuth = flat.count != null ? Number(flat.count) : NaN;
      if (!isNaN(nAuth) && nAuth >= 0) return baseAuth + " Active keys: " + Math.round(nAuth) + ".";
      return baseAuth;
  };
  formatters.gateway_health_upstream = function (flat, entry) {
      var okH = flat.ok === true || flat.ok === "true" || flat.ok === 1;
      var baseH = okH ? "Upstream health OK" : "Upstream health failed";
      if (entry.summary && !okH && entry.summary.indexOf("failed") < 0) baseH = entry.summary;
      var bitsH = [];
      var stH = flat.status != null ? Number(flat.status) : NaN;
      if (!isNaN(stH)) bitsH.push("probe HTTP " + Math.round(stH));
      var detH = flat.detail != null ? String(flat.detail).replace(/\s+/g, " ").trim() : "";
      if (detH.length > 100) detH = detH.slice(0, 99) + "…";
      if (!okH && detH) bitsH.push(detH);
      var tgtH = flat.target != null ? String(flat.target).trim() : "";
      if (tgtH) {
        var hostH = urlHostTail(tgtH, 72);
        if (!hostH) hostH = tgtH.length > 72 ? tgtH.slice(0, 71) + "…" : tgtH;
        if (hostH) bitsH.push(hostH);
      }
      return bitsH.length ? baseH + " · " + bitsH.join(" · ") : baseH;
  };
  formatters.gateway_startup_listening = function (flat, entry) {
      var bitsL = [entry.summary || "Gateway listening for HTTP requests."];
      var addrL = flat.addr != null ? String(flat.addr).trim() : "";
      if (addrL) bitsL.push("bind " + addrL);
      var brL = flat.broker != null ? String(flat.broker).trim() : "";
      if (brL) {
        var brShort = urlHostTail(brL, 56) || brL;
        if (brShort.length > 56) brShort = brShort.slice(0, 55) + "…";
        bitsL.push("chimera-broker " + brShort);
      }
      return bitsL.join(" · ");
  };
  formatters.gateway_supervisor_bin_cfg = function (flat, entry) {
      var bitsIxS = [entry.summary || "Supervised indexer process starting."];
      if (flat.bin != null && String(flat.bin).trim() !== "") {
        var bn = String(flat.bin).replace(/\\/g, "/");
        var leaf = bn.split("/").pop();
        bitsIxS.push(leaf || bn);
      }
      var cfgIxS = flat.config != null ? String(flat.config).trim() : "";
      if (cfgIxS) bitsIxS.push("config " + (cfgIxS.length > 48 ? cfgIxS.slice(0, 47) + "…" : cfgIxS));
      return bitsIxS.join(" · ");
  };
  formatters.gateway_supervisor_url_tail = function (flat, entry) {
      var base = entry.summary || "";
      var url = flat.url != null ? String(flat.url).trim() : "";
      if (!url) return base;
      var tail = urlHostTail(url, 96);
      return tail ? base + " · " + tail : base;
  };
  formatters.gateway_supervisor_broker_start = function (flat, entry) {
      var bitsBs = [entry.summary || "chimera-broker subprocess starting."];
      if (flat.bin != null && String(flat.bin).trim() !== "") {
        var bbs = String(flat.bin).replace(/\\/g, "/").split("/").pop();
        if (bbs) bitsBs.push(bbs);
      }
      var appD = flat.app_dir != null ? String(flat.app_dir).trim() : flat.dir != null ? String(flat.dir).trim() : "";
      if (appD) bitsBs.push("data " + (appD.length > 40 ? appD.slice(0, 39) + "…" : appD));
      if (flat.host != null && String(flat.host).trim() !== "") bitsBs.push("host " + String(flat.host).trim());
      if (flat.port != null && String(flat.port).trim() !== "") bitsBs.push("port " + String(flat.port).trim());
      return bitsBs.join(" · ");
  };
  formatters.gateway_supervisor_vectorstore_start = function (flat, entry) {
      var bitsQs = [entry.summary || "chimera-vectorstore subprocess starting."];
      if (flat.bin != null && String(flat.bin).trim() !== "") {
        var bqs = String(flat.bin).replace(/\\/g, "/").split("/").pop();
        if (bqs) bitsQs.push(bqs);
      }
      var stor = flat.storage != null ? String(flat.storage).trim() : "";
      if (stor) bitsQs.push("storage " + (stor.length > 40 ? stor.slice(0, 39) + "…" : stor));
      if (flat.http_port != null) bitsQs.push("http " + String(flat.http_port));
      if (flat.grpc_port != null) bitsQs.push("grpc " + String(flat.grpc_port));
      if (flat.host != null && String(flat.host).trim() !== "") bitsQs.push("host " + String(flat.host).trim());
      return bitsQs.join(" · ");
  };
  formatters.gateway_startup_disk_log = function (flat, entry) {
      var phaseD = flat.phase != null ? String(flat.phase).trim() : "";
      var pathD = flat.path != null ? String(flat.path).trim() : "";
      var dirD = flat.dir != null ? String(flat.dir).trim() : "";
      var errD = flat.err != null ? String(flat.err).replace(/\s+/g, " ").trim() : "";
      if (errD.length > 120) errD = errD.slice(0, 119) + "…";
      if (phaseD === "mkdir" || phaseD === "open") {
        var locD = pathD || dirD || "";
        if (locD.length > 72) locD = locD.slice(0, 71) + "…";
        return "Disk log setup failed (" + phaseD + ")" + (locD ? " · " + locD : "") + (errD ? " · " + errD : "");
      }
      if (pathD) return "Disk logging enabled · " + (pathD.length > 100 ? pathD.slice(0, 99) + "…" : pathD);
      return entry.summary || "Disk logging enabled.";
  };
  formatters.gateway_startup_config_resolved = function (flat, entry) {
      var bitsCfg = [entry.summary || "Gateway configuration paths resolved."];
      var fpCfg = flat.filePath != null ? String(flat.filePath).trim() : "";
      if (fpCfg) bitsCfg.push("gateway " + (fpCfg.length > 56 ? fpCfg.slice(0, 55) + "…" : fpCfg));
      var akCfg =
        flat.api_keys_path != null
          ? String(flat.api_keys_path).trim()
          : flat.tokens_path != null
            ? String(flat.tokens_path).trim()
            : "";
      if (akCfg) bitsCfg.push("keys " + (akCfg.length > 48 ? akCfg.slice(0, 47) + "…" : akCfg));
      var rpCfg = flat.routingPolicyPath != null ? String(flat.routingPolicyPath).trim() : "";
      if (rpCfg) bitsCfg.push("routing " + (rpCfg.length > 48 ? rpCfg.slice(0, 47) + "…" : rpCfg));
      return bitsCfg.join(" · ");
  };
  formatters.upstream_models_ok = function (flat, entry) {
      var n = flat.count != null ? Number(flat.count) : NaN;
      var base = entry.summary || "Upstream model catalog responded successfully.";
      if (!isNaN(n) && n >= 0) return base + " Models listed: " + Math.round(n) + ".";
      return base;
  };
  formatters.http_access = function (flat, entry, opts) {
      opts = opts || {};
      var omitStatus = opts.forEventLog === true;
      var line =
        (flat.method || "?") +
        " " +
        (flat.path || "") +
        (omitStatus ? "" : flat.statusCode != null ? " → " + flat.statusCode : "") +
        (flat.responseTimeMs != null ? " · " + flat.responseTimeMs + " ms" : "");
      return line;
  };

  function renderRegistryEntry(flat, canonical, opts) {
    var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
    if (!oc || !oc.bySlug) return "";
    var raw = oc.bySlug[canonical];
    if (!raw) return "";
    var entry = { slug: canonical, summary: raw.summary, formatter: raw.formatter || "" };
    var fmt = entry.formatter ? String(entry.formatter).trim() : "";
    if (fmt && formatters[fmt]) return formatters[fmt](flat, entry, opts) || "";
    return entry.summary != null ? String(entry.summary) : "";
  }

  function resolveCanonicalSlug(flat) {
    var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
    if (oc && typeof oc.resolveFlat === "function" && flat && typeof flat === "object") {
      var fromFlat = oc.resolveFlat(flat);
      if (fromFlat) return fromFlat;
    }
    var msg = flatMsg(flat);
    if (!msg) return "";
    if (oc && typeof oc.resolveCanonical === "function") {
      var c = oc.resolveCanonical(msg);
      if (c) return c;
    }
    return "";
  }

  function operatorMessage(flat, opts) {
    opts = opts || {};
    if (!flat || typeof flat !== "object") return "";
    var render = globalThis.ChimeraSettings && ChimeraSettings.Render;
    var canonical = resolveCanonicalSlug(flat);
    if (canonical) {
      var line = renderRegistryEntry(flat, canonical, opts);
      if (line) return line;
    }
    if (render && typeof render.isBrokerOrRelayLine === "function" && render.isBrokerOrRelayLine(flat)) {
      var msg = flatMsg(flat);
      if (msg.indexOf("chimera-broker.") === 0 || msg.indexOf("broker.") === 0) {
        return "chimera-broker · " + msg;
      }
    }
    if (render && typeof render.isVectorstoreLine === "function" && render.isVectorstoreLine(flat)) {
      if (typeof render.vectorstorePrefixFallback === "function") {
        return render.vectorstorePrefixFallback(flat, opts) || "";
      }
    }
    return "";
  }

  ChimeraSettings.Render.resolveCanonicalSlug = resolveCanonicalSlug;
  ChimeraSettings.Render.operatorMessage = operatorMessage;
  ChimeraSettings.Render.operatorFriendlyGatewayMsg = operatorMessage;
  formatters._extractOpenAIErrorMessage = extractOpenAIErrorMessage;
})();
