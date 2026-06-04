/**
 * Broker and vectorstore operator formatters (Phase 3). Merges into ChimeraSettings.Render operator formatters.
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
  var base = globalThis.ChimeraSettings.Render._operatorFormatters || {};

  function trimDetail(flat, maxLen) {
    maxLen = maxLen > 0 ? maxLen : 220;
    var pd = flat.progress_detail != null ? String(flat.progress_detail) : "";
    if (!pd) return "";
    var t = pd.replace(/\s+/g, " ").trim();
    return t.length > maxLen ? t.slice(0, maxLen - 1) + "…" : t;
  }

  function brokerPathFromTarget(target) {
    var s = target != null ? String(target).trim() : "";
    if (!s) return "";
    if (typeof URL === "function") {
      try {
        var u = new URL(s, "http://localhost");
        return u.pathname || "/";
      } catch (e) {
        /* fall through */
      }
    }
    var i = s.indexOf("?");
    if (i >= 0) s = s.slice(0, i);
    var dbl = s.indexOf("//");
    if (dbl >= 0) {
      var rest = s.slice(dbl + 2);
      var p = rest.indexOf("/");
      if (p >= 0) return rest.slice(p) || "/";
    }
    return s;
  }

  function brokerShortTailModel(model) {
    var fmt = globalThis.ChimeraSettings && ChimeraSettings.Render && ChimeraSettings.Render._operatorFormatters;
    if (fmt && typeof fmt._brokerShortModel === "function") {
      return fmt._brokerShortModel(model);
    }
    var m = model != null ? String(model).trim() : "";
    if (!m) return "";
    var parts = m.split("/");
    var tail = parts[parts.length - 1] || m;
    return tail.length > 48 ? tail.slice(0, 46) + "…" : tail;
  }

  function operatorFmtHelpers() {
    var fmt = globalThis.ChimeraSettings && ChimeraSettings.Render && ChimeraSettings.Render._operatorFormatters;
    return fmt && typeof fmt === "object" ? fmt : null;
  }

  function usageTokensFromFlat(f) {
    var ut = Number(f.usageTotalTokens != null ? f.usageTotalTokens : f.usage_total_tokens);
    if (!isNaN(ut) && ut > 0) return ut;
    var up = Number(f.usagePromptTokens != null ? f.usagePromptTokens : f.usage_prompt_tokens);
    var uc = Number(f.usageCompletionTokens != null ? f.usageCompletionTokens : f.usage_completion_tokens);
    var sum = (isNaN(up) ? 0 : up) + (isNaN(uc) ? 0 : uc);
    return sum > 0 ? sum : NaN;
  }

  function vectorstoreCollectionDisplay(collRaw, resolveColl) {
    var r = collRaw != null ? String(collRaw).trim() : "";
    if (!r) return "";
    if (typeof resolveColl === "function") {
      var x = resolveColl(r);
      if (x != null && String(x).trim() !== "") return String(x).trim();
    }
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.vectorstoreCollectionDisplay === "function") {
      var lab = ChimeraSettings.Derive.vectorstoreCollectionDisplay(r, resolveColl);
      if (lab != null && String(lab).trim() !== "") return String(lab).trim();
    }
    return r;
  }

  function vectorstorePrefixFallback(flat, opts) {
    var msg = String(flat.msg != null ? flat.msg : "").toLowerCase();
    var prog = flat.progress_detail != null ? String(flat.progress_detail) : "";
    if (msg.indexOf("qdrant.") !== 0 && msg.indexOf("vectorstore.") !== 0) return "";
    if (prog && (msg === "qdrant.trace.other" || msg === "qdrant.unparsed" || msg === "vectorstore.trace.other" || msg === "vectorstore.unparsed")) {
      return prog.replace(/\s+/g, " ").slice(0, 280);
    }
    var slugRest = String(flat.msg || msg || "")
      .replace(/^qdrant\./, "")
      .replace(/^vectorstore\./, "")
      .replace(/\./g, " ")
      .trim();
    return slugRest ? "chimera-vectorstore backend · " + slugRest : "chimera-vectorstore backend event";
  }

  var svc = {
    http_broker_inbound: function (flat, entry, opts) {
      opts = opts || {};
      var flatM = String(flat.msg != null ? flat.msg : "").trim();
      var rateLimit = (entry && entry.slug === "broker.rate_limit") || flatM === "chimera-broker.rate_limit";
      if (
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.brokerHttpInboundLine === "function"
      ) {
        return ChimeraSettings.Derive.brokerHttpInboundLine(flat, rateLimit, opts);
      }
      var omitStatus = opts.forEventLog === true;
      var meth = flat.http_method != null ? String(flat.http_method).trim() : "?";
      var tgt = flat.http_target != null ? flat.http_target : flat.httpTarget;
      var path = brokerPathFromTarget(tgt);
      if (!path) path = "/";
      var st = Number(flat.http_status != null ? flat.http_status : flat.httpStatus);
      if (isNaN(st)) st = 0;
      var msRaw = flat.http_duration_ms != null ? flat.http_duration_ms : flat.httpDurationMS;
      var ms = Number(msRaw);
      var bits = [];
      bits.push(rateLimit ? "Rate limited" : "Broker HTTP");
      bits.push(meth + " " + path);
      if (!omitStatus) bits.push("→ " + st);
      if (!isNaN(ms) && ms >= 0) bits.push(Math.round(ms) + " ms");
      return bits.join(" · ");
    },
    broker_chat_relay: function (flat, entry, opts) {
      opts = opts || {};
      var omitHttpInMsg = opts.forEventLog === true;
      var msg = String(flat.msg != null ? flat.msg : flat.message != null ? flat.message : "").trim();
      var ml = msg.toLowerCase();
      if (msg === "chat.chimera-broker.available_models") {
        var nAvail = Number(flat.catalog_model_count != null ? flat.catalog_model_count : flat.catalogModelCount);
        if (!isNaN(nAvail) && nAvail > 0) return "Model list for routing · " + Math.round(nAvail) + " models";
        return entry.summary || "Model list for routing refreshed";
      }
      if (msg === "chat.chimera-broker.request") {
        if (opts.forEventLog === true) {
          var bitsSend = ["Sent prompt to provider"];
          var modSend = brokerShortTailModel(flat.upstreamModel);
          if (modSend) bitsSend.push(modSend);
          var otSend = Number(flat.outgoingTokens != null ? flat.outgoingTokens : flat.outgoing_tokens);
          var estSend = !isNaN(otSend) && otSend > 0 ? "~" + Math.round(otSend).toLocaleString() + " input tokens (estimated)" : "";
          if (estSend) bitsSend.push(estSend);
          return bitsSend.join(" · ") + ".";
        }
        var bitsRq = ["Relay request"];
        var modRq = brokerShortTailModel(flat.upstreamModel);
        if (modRq) bitsRq.push(modRq);
        var ot = Number(flat.outgoingTokens != null ? flat.outgoingTokens : flat.outgoing_tokens);
        if (!isNaN(ot) && ot > 0) bitsRq.push(ot + " input tok est");
        return bitsRq.join(" · ");
      }
      if (msg === "chat.chimera-broker.response" || ml === "upstream chat response") {
        var scR = Number(flat.statusCode != null ? flat.statusCode : flat.status_code);
        var isOk = !isNaN(scR) && scR >= 200 && scR <= 299;
        if (opts.forEventLog === true) {
          if (!isOk) {
            var rawErr =
              flat.err != null
                ? String(flat.err)
                : flat.upstreamErrorExcerpt != null
                  ? String(flat.upstreamErrorExcerpt)
                  : flat.responseBodyExcerpt != null
                    ? String(flat.responseBodyExcerpt)
                    : "";
            var helpers = operatorFmtHelpers();
            var tpm = helpers && typeof helpers._parseTpmRateLimitError === "function" ? helpers._parseTpmRateLimitError(rawErr) : null;
            if (tpm && helpers && typeof helpers._formatTpmRateLimitLine === "function") {
              return helpers._formatTpmRateLimitLine(flat.upstreamModel, tpm);
            }
            var errMsg = "";
            if (helpers && typeof helpers._extractOpenAIErrorMessage === "function") {
              errMsg = helpers._extractOpenAIErrorMessage(rawErr);
            }
            if (!errMsg && rawErr) {
              var msgMatch = rawErr.match(/"message"\s*:\s*"((?:[^"\\]|\\.)*)"/);
              if (msgMatch && msgMatch[1]) errMsg = msgMatch[1].replace(/\\"/g, '"').trim();
            }
            if (!errMsg) errMsg = rawErr.replace(/\s+/g, " ").trim().slice(0, 220);
            var scErr = Number(flat.statusCode != null ? flat.statusCode : flat.status_code);
            var is404 = scErr === 404 || errMsg.toLowerCase().indexOf("not found") >= 0;
            if (helpers && typeof helpers._sanitizeProviderErrorForOperator === "function") {
              errMsg = helpers._sanitizeProviderErrorForOperator(errMsg, { modelNotFound: is404 });
            }
            var modErr = brokerShortTailModel(flat.upstreamModel);
            if (modErr && errMsg) return "Provider rejected " + modErr + ": " + errMsg;
            return errMsg ? "Provider rejected request: " + errMsg : "Provider rejected request.";
          }
          var bitsOk = ["Provider responded"];
          var modOk = brokerShortTailModel(flat.upstreamModel);
          if (modOk) bitsOk.push(modOk);
          var utOk = usageTokensFromFlat(flat);
          if (!isNaN(utOk)) bitsOk.push(Math.round(utOk).toLocaleString() + " tokens used (prompt + completion)");
          var fr =
            flat.finish_reason != null
              ? String(flat.finish_reason).trim()
              : flat.finishReason != null
                ? String(flat.finishReason).trim()
                : "";
          if (fr) bitsOk.push("finish: " + fr);
          return bitsOk.join(" · ");
        }
        var bitsRes = ["Relay response"];
        if (!omitHttpInMsg && !isNaN(scR) && scR > 0) bitsRes.push("HTTP " + scR);
        var ut = usageTokensFromFlat(flat);
        if (!isNaN(ut)) bitsRes.push(ut + " usage tok");
        return bitsRes.join(" · ");
      }
      if (msg === "chat.chimera-broker.error") {
        var er = flat.err != null ? String(flat.err) : "";
        er = er.replace(/\s+/g, " ").trim();
        if (er.length > 200) er = er.slice(0, 199) + "…";
        return er ? "Relay failed · " + er : "Relay failed";
      }
      return entry.summary || "";
    },
    broker_routing_relay: function (flat, entry, opts) {
      opts = opts || {};
      var omitHttpInMsg = opts.forEventLog === true;
      var msg = String(flat.msg != null ? flat.msg : "").trim();
      var ml = msg.toLowerCase();
      if (msg === "chat.routing.fallback") {
        var bitsFb = ["Fallback retry"];
        var modFb = brokerShortTailModel(flat.upstreamModel);
        if (modFb) bitsFb.push(modFb);
        var stFb = Number(flat.status != null ? flat.status : flat.statusCode != null ? flat.statusCode : NaN);
        if (!omitHttpInMsg && !isNaN(stFb) && stFb > 0) bitsFb.push("HTTP " + stFb);
        var wr = flat.willRetry;
        if (wr === false) bitsFb.push("no retry");
        return bitsFb.join(" · ");
      }
      if (msg === "chat.routing.attempt" || ml === "virtual model fallback attempt") {
        var bitsVa = ["Routing attempt"];
        var modVa = brokerShortTailModel(flat.upstreamModel);
        if (modVa) bitsVa.push(modVa);
        var att = Number(flat.attempt);
        var chain = Number(flat.chainLen);
        if (!isNaN(att) && !isNaN(chain) && chain > 0) bitsVa.push("attempt " + att + "/" + chain);
        return bitsVa.join(" · ");
      }
      if (msg === "chat.routing.resolved" || ml === "virtual model routing resolved") {
        var bitsVr = ["Routing resolved"];
        var modVr = brokerShortTailModel(flat.upstreamModel);
        if (modVr) bitsVr.push(modVr);
        var attR = Number(flat.attempt);
        var chainR = Number(flat.chainLen);
        if (!isNaN(attR) && !isNaN(chainR) && chainR > 0) bitsVr.push("attempt " + attR + "/" + chainR);
        var scV = Number(flat.statusCode != null ? flat.statusCode : flat.status);
        if (!omitHttpInMsg && !isNaN(scV) && scV > 0) bitsVr.push("HTTP " + scV);
        return bitsVr.join(" · ");
      }
      return entry.summary || "";
    },
    broker_provider_limits: function (flat, entry) {
      var rsn = flat.reason != null ? String(flat.reason).replace(/\s+/g, " ").trim() : "";
      var bits = ["Blocked by provider limits"];
      var model = brokerShortTailModel(flat.upstreamModel);
      if (model) bits.push(model);
      if (rsn === "context_window") {
        var outTok = Number(flat.outgoingTokens != null ? flat.outgoingTokens : flat.outgoing_tokens);
        var maxTok = Number(flat.max_tokens != null ? flat.max_tokens : flat.maxTokens);
        var cap = Number(flat.context_cap != null ? flat.context_cap : flat.contextCap);
        var needParts = [];
        if (!isNaN(outTok) && outTok > 0) needParts.push(Math.round(outTok).toLocaleString() + " prompt");
        if (!isNaN(maxTok) && maxTok > 0) needParts.push(Math.round(maxTok).toLocaleString() + " max_tokens");
        var need = needParts.join(" + ");
        if (need && !isNaN(cap) && cap > 0) {
          bits.push("context window · " + need + " > cap " + Math.round(cap).toLocaleString());
        } else {
          bits.push("context window");
        }
        return bits.join(" · ");
      }
      if (rsn === "request_body_bytes") {
        var bodyBytes = Number(flat.body_bytes != null ? flat.body_bytes : flat.bodyBytes);
        var maxBody = Number(flat.max_body_bytes != null ? flat.max_body_bytes : flat.maxBodyBytes);
        if (!isNaN(bodyBytes) && !isNaN(maxBody) && maxBody > 0) {
          bits.push("body size · " + Math.round(bodyBytes).toLocaleString() + " bytes > cap " + Math.round(maxBody).toLocaleString());
        } else {
          bits.push("request body bytes");
        }
        return bits.join(" · ");
      }
      if (rsn === "tpm") bits.push("TPM quota");
      else if (rsn === "rpm") bits.push("RPM quota");
      else if (rsn === "tpd") bits.push("TPD quota");
      else if (rsn === "rpd") bits.push("RPD quota");
      else if (rsn) {
        if (rsn.length > 140) rsn = rsn.slice(0, 139) + "…";
        bits.push(rsn);
      }
      return bits.join(" · ");
    },
    broker_trim_detail: function (flat, entry, opts) {
      opts = opts || {};
      var det = trimDetail(flat, 260);
      var base = entry.summary || "";
      var slug = entry.slug || "";
      if (slug === "broker.store.config_ready") {
        var store = trimDetail(flat, 40).toLowerCase();
        if (store === "sqlite" || store === "memory") return "Config store ready · " + store;
        return "Config store ready";
      }
      if (!det) return base;
      if (opts.forEventLog === true && base === "chimera-broker") return det;
      if (base.indexOf("invalid") >= 0 || base.indexOf("warning") >= 0 || base.indexOf("error") >= 0 || base.indexOf("Upstream") >= 0) {
        return base + " · " + det;
      }
      if (base === "HTTP listening" || base === "chimera-broker") return det ? base + " · " + det : base;
      if (base.indexOf("Unrecognized") >= 0) return det ? base + " · " + det : base;
      if (base.indexOf("Shutting down") >= 0) return det ? base + " · " + det : base;
      if (base.indexOf("Rejected") >= 0) return det ? base + " · " + det : base;
      return base;
    },
    broker_bootstrap_complete: function (flat, entry) {
      var bms = Number(flat.bootstrap_ms != null ? flat.bootstrap_ms : flat.bootstrapMs);
      if (!isNaN(bms) && bms >= 0) return "Startup finished · bootstrap " + Math.round(bms) + " ms";
      return entry.summary || "Startup finished · bootstrap complete";
    },
    broker_ready: function (flat, entry) {
      var urlR = flat.listen_url != null ? String(flat.listen_url).trim() : "";
      if (urlR) return "Ready · UI at " + urlR;
      var lp = Number(flat.listen_port != null ? flat.listen_port : flat.listenPort);
      if (!isNaN(lp) && lp > 0) return "Ready · port " + lp;
      return entry.summary || "Ready";
    },
    broker_version: function (flat, entry) {
      var ver = flat.chimera_broker_version != null ? String(flat.chimera_broker_version).trim() : "";
      return ver ? "chimera-broker version " + ver + " (backend: BiFrost)" : entry.summary || "chimera-broker version (backend: BiFrost)";
    },
    broker_jwt_startup: function (flat, entry) {
      var authH = trimDetail(flat, 80).toLowerCase();
      if (authH.indexOf("jwt") >= 0) return "Auth · JWT";
      if (authH.indexOf("api") >= 0) return "Auth · API key";
      if (authH.indexOf("disabled") >= 0) return "Auth · disabled";
      return entry.summary || "Auth plugin started";
    },
    broker_catalog_sync: function (flat, entry) {
      var ncat = Number(flat.catalog_model_count != null ? flat.catalog_model_count : flat.catalogModelCount);
      var det = trimDetail(flat, 120).toLowerCase();
      if (!isNaN(ncat) && ncat > 0) return "Model catalog updated · " + Math.round(ncat) + " models";
      if (det.indexOf("pricing") >= 0) return "Model catalog / pricing · " + trimDetail(flat, 120);
      return entry.summary || "Model catalog sync";
    },
    broker_plugin_status: function (flat, entry) {
      var pn = flat.plugin_name != null ? String(flat.plugin_name).trim() : "";
      var ps = flat.plugin_status != null ? String(flat.plugin_status).trim() : "";
      if (pn && ps) return "Plugin " + pn + " · " + ps;
      if (pn) return "Plugin " + pn;
      return entry.summary || "Plugin status";
    },
    broker_provider_id: function (flat, entry, opts) {
      opts = opts || {};
      var pid = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      var base = entry.summary || "";
      var det = trimDetail(flat, 260);
      if (opts.forEventLog === true) {
        if (det) {
          var detL = det.toLowerCase();
          var pidL = pid.toLowerCase();
          if (
            pidL &&
            (detL.indexOf("provider " + pidL) >= 0 ||
              detL.indexOf("for provider " + pidL) >= 0 ||
              detL.indexOf("provider_id=" + pidL) >= 0)
          ) {
            return det;
          }
          return pid ? base + " · " + pid + " · " + det : base + " · " + det;
        }
        return pid ? base + " · " + pid : base;
      }
      return pid ? base + " · " + pid : base;
    },
    broker_log_retention: function (flat, entry) {
      var days = Number(flat.log_retention_days != null ? flat.log_retention_days : flat.logRetentionDays);
      if (!isNaN(days) && days > 0) return "Request log retention · " + Math.round(days) + " days";
      var d2 = trimDetail(flat, 120);
      if (d2) return "Log retention · " + d2;
      return entry.summary || "Log retention / cleanup";
    },
    vectorstore_version: function (flat, entry) {
      var ver = flat.qdrant_version != null ? String(flat.qdrant_version).trim() : "";
      return "Component: chimera-vectorstore · Backend: Qdrant " + (ver || "").trim();
    },
    vectorstore_collection_http: function (flat, entry, opts) {
      opts = opts || {};
      var omitHttpInMsg = opts.forEventLog === true;
      var resolveColl = opts.resolveColl;
      var coll = vectorstoreCollectionDisplay(flat.collection != null ? flat.collection : "", resolveColl);
      var st = flat.http_status != null ? Number(flat.http_status) : NaN;
      var stLab = !omitHttpInMsg && !isNaN(st) ? String(Math.round(st)) : "";
      var slug = entry.slug || "";
      if (slug === "vectorstore.collection.loading" || slug.indexOf("shard.recover") >= 0) {
        var line = "Loading collection " + coll;
        if (slug === "vectorstore.shard.recover_progress") {
          var prog = flat.progress_detail != null ? String(flat.progress_detail) : "";
          if (prog) line += " · " + prog.replace(/\s+/g, " ").slice(0, 280);
        }
        return line;
      }
      if (slug === "vectorstore.collection.creating") return "Creating collection " + coll;
      if (slug === "vectorstore.shard.recovered") return "Loaded collection " + coll;
      if (slug === "vectorstore.http.collection_meta") return "Reading collection " + coll + (stLab !== "" ? " · " + stLab : "");
      if (slug === "vectorstore.http.collection_create") return "Created collection " + coll + (stLab !== "" ? " · " + stLab : "");
      if (slug === "vectorstore.http.collection_create_rejected") {
        return "Create collection " + coll + (stLab !== "" ? " · " + stLab : "");
      }
      if (slug === "vectorstore.http.collection_index") return "Indexed collection " + coll + (stLab !== "" ? " · " + stLab : "");
      if (slug === "vectorstore.http.collection_index_rejected") {
        return "Index collection " + coll + (stLab !== "" ? " · " + stLab : "");
      }
      if (slug === "vectorstore.http.points_upsert_ok" || slug === "vectorstore.http.points_upsert_rejected") {
        return "Upsert into collection " + coll + (stLab !== "" ? " · " + stLab : "");
      }
      if (slug === "vectorstore.http.points_delete") return "Deleting from collection " + coll + (stLab !== "" ? " · " + stLab : "");
      if (slug === "vectorstore.http.vector_search") return "Searching collection " + coll + (stLab !== "" ? " · " + stLab : "");
      return (entry.summary || "") + " " + coll + (stLab !== "" ? " · " + stLab : "");
    },
    vectorstore_progress_detail: function (flat, entry, opts) {
      var prog = flat.progress_detail != null ? String(flat.progress_detail) : "";
      var base = entry.summary || "";
      if (!prog) return base;
      var tail = prog.replace(/\s+/g, " ").slice(0, 280);
      if (base.indexOf("·") >= 0) return base.split("·")[0].trim() + " · " + tail;
      return base + " · " + tail;
    },
    vectorstore_listen_ports: function (flat, entry) {
      var base = entry.summary || "";
      if (base.indexOf("REST listening") === 0) {
        var rp = flat.rest_port != null ? flat.rest_port : flat.RESTPort;
        return base + (rp != null ? " on port " + rp : "");
      }
      if (base.indexOf("gRPC listening") === 0) {
        var gp = flat.grpc_port != null ? flat.grpc_port : flat.GRPCPort;
        return base + (gp != null ? " on port " + gp : "");
      }
      if (base.indexOf("Internal gRPC listening") === 0) {
        var igp = flat.internal_grpc_port != null ? flat.internal_grpc_port : flat.InternalGRPCPort;
        return base + (igp != null ? " on port " + igp : "");
      }
      return base;
    },
    vectorstore_http_upsert_summary: function (flat) {
      var u = Number(flat.upserts_ok != null ? flat.upserts_ok : 0) || 0;
      var d = Number(flat.deletes_ok != null ? flat.deletes_ok : 0) || 0;
      var s = Number(flat.searches_ok != null ? flat.searches_ok : 0) || 0;
      var winSec =
        flat.window_ms != null && !isNaN(Number(flat.window_ms))
          ? Math.round(Number(flat.window_ms) / 1000)
          : null;
      var win =
        winSec === 1 ? "1 second" : winSec != null && !isNaN(winSec) ? winSec + " seconds" : "?";
      if (u > 0) {
        return (
          "Updated the indexes of " +
          u +
          " file" +
          (u === 1 ? "" : "s") +
          " in the last " +
          win
        );
      }
      if (d > 0) {
        return (
          "Removed index entries for " +
          d +
          " file" +
          (d === 1 ? "" : "s") +
          " in the last " +
          win
        );
      }
      if (s > 0) {
        return (
          "Ran " +
          s +
          " search" +
          (s === 1 ? "" : "es") +
          " against the index in the last " +
          win
        );
      }
      return "No vector index updates in the last " + win;
    }
  };

  Object.assign(base, svc);
  globalThis.ChimeraSettings.Render._operatorFormatters = base;
  globalThis.ChimeraSettings.Render.vectorstorePrefixFallback = vectorstorePrefixFallback;
  globalThis.ChimeraSettings.Render.isBrokerOrRelayLine = function (flat) {
    if (!flat || typeof flat !== "object") return false;
    var msg = String(flat.msg != null ? flat.msg : flat.message != null ? flat.message : "").trim();
    if (!msg) return false;
    var ml = msg.toLowerCase();
    var svcName = String(flat.service || "").toLowerCase();
    var isBrokerSvc = svcName === "chimera-broker";
    var isBrokerSlug = isBrokerSvc && (msg.indexOf("chimera-broker.") === 0 || msg.indexOf("broker.") === 0);
    var isChatBroker = msg.indexOf("chat.chimera-broker.") === 0;
    return (
      isBrokerSlug ||
      isChatBroker ||
      ml === "upstream chat response" ||
      msg === "chat.chimera-broker.response" ||
      msg === "chat.routing.fallback" ||
      msg === "chat.routing.attempt" ||
      msg === "chat.routing.resolved" ||
      msg === "chat.provider_limits.blocked" ||
      ml === "virtual model fallback attempt" ||
      ml === "virtual model routing resolved" ||
      ml === "chat blocked by provider limits" ||
      ml === "skipping upstream model (provider limits)"
    );
  };
  globalThis.ChimeraSettings.Render.isVectorstoreLine = function (flat) {
    if (!flat || typeof flat !== "object") return false;
    var svcName = String(flat.service || "").toLowerCase();
    if (svcName === "qdrant" || svcName === "chimera-vectorstore") return true;
    var msg = String(flat.msg != null ? flat.msg : "").toLowerCase();
    return msg.indexOf("qdrant.") === 0 || msg.indexOf("vectorstore.") === 0;
  };
})();
