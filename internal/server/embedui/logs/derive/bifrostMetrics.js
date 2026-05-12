/**
 * Pure metrics derivation for the bifrost service card.
 *
 * Exports:
 * - ClaudiaLogs.Derive.bifrostOperatorLine(flat, opts?)
 * - ClaudiaLogs.Derive.bifrostEntryHasRateLimit(ent, getFlat)
 * - ClaudiaLogs.Derive.bifrostSliceSinceLastBanner(arr, getFlat)
 * - ClaudiaLogs.Derive.bifrostCardModel(arr, getFlat)
 * - ClaudiaLogs.Derive.bifrostCardMetrics(arr, getFlat)
 */

function bifrostPathFromTarget(target) {
  var s = target != null ? String(target).trim() : "";
  if (!s) return "";
  try {
    var u = new URL(s, "http://localhost");
    return u.pathname || "/";
  } catch (e) {
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
}

function bifrostShortTailModel(model) {
  var m = model != null ? String(model).trim() : "";
  if (!m) return "";
  var parts = m.split("/");
  var tail = parts[parts.length - 1] || m;
  return tail.length > 48 ? tail.slice(0, 46) + "…" : tail;
}

function bifrostTrimDetail(flat, maxLen) {
  maxLen = maxLen > 0 ? maxLen : 220;
  var pd = flat.progress_detail != null ? String(flat.progress_detail) : "";
  if (!pd) return "";
  var t = pd.replace(/\s+/g, " ").trim();
  return t.length > maxLen ? t.slice(0, maxLen - 1) + "…" : t;
}

function bifrostHttpInboundLine(flat, rateLimit, opts) {
  opts = opts || {};
  var omitStatus = opts.forEventLog === true;
  var meth = flat.http_method != null ? String(flat.http_method).trim() : "?";
  var tgt = flat.http_target != null ? flat.http_target : flat.httpTarget;
  var path = bifrostPathFromTarget(tgt);
  if (!path) path = "/";
  var st = Number(flat.http_status != null ? flat.http_status : flat.httpStatus);
  if (isNaN(st)) st = 0;
  var msRaw = flat.http_duration_ms != null ? flat.http_duration_ms : flat.httpDurationMS;
  var ms = Number(msRaw);
  var bits = [];
  bits.push(rateLimit ? "Rate limited" : "Inbound");
  bits.push(meth + " " + path);
  if (!omitStatus) bits.push("→ " + st);
  if (!isNaN(ms) && ms >= 0) bits.push(Math.round(ms) + " ms");
  return bits.join(" · ");
}

/**
 * One-line operator headline for summarized logs and detail headlines.
 * Returns "" when this row is not part of the BiFrost / relay vocabulary.
 */
function bifrostOperatorLine(flat, opts) {
  opts = opts || {};
  var omitHttpInMsg = opts.forEventLog === true;
  if (!flat || typeof flat !== "object") return "";
  var msg = String(flat.msg != null ? flat.msg : flat.message != null ? flat.message : "").trim();
  if (!msg) return "";
  var ml = msg.toLowerCase();
  var svc = String(flat.service || "").toLowerCase();
  var isBifrostSvc = svc === "bifrost";
  var isBifrostSlug = isBifrostSvc && msg.indexOf("bifrost.") === 0;
  var isChatBifrost = msg.indexOf("chat.bifrost.") === 0;
  var isRelayMsg =
    isChatBifrost ||
    ml === "upstream chat response" ||
    msg === "chat.bifrost.response" ||
    msg === "chat.routing.fallback" ||
    msg === "chat.routing.attempt" ||
    msg === "chat.routing.resolved" ||
    msg === "chat.provider_limits.blocked" ||
    ml === "virtual model fallback attempt" ||
    ml === "virtual model routing resolved" ||
    ml === "chat blocked by provider limits" ||
    ml === "skipping upstream model (provider limits)";
  if (!isBifrostSlug && !isRelayMsg) return "";

  if (msg === "bifrost.http.access") return bifrostHttpInboundLine(flat, false, opts);
  if (msg === "bifrost.rate_limit") return bifrostHttpInboundLine(flat, true, opts);

  if (msg === "chat.bifrost.available_models") {
    var nAvail = Number(flat.catalog_model_count != null ? flat.catalog_model_count : flat.catalogModelCount);
    if (!isNaN(nAvail) && nAvail > 0) return "Model list for routing · " + Math.round(nAvail) + " models";
    return "Model list for routing refreshed";
  }

  if (msg === "chat.bifrost.request") {
    var bitsRq = ["Relay request"];
    var modRq = bifrostShortTailModel(flat.upstreamModel);
    if (modRq) bitsRq.push(modRq);
    if (flat.stream === true) bitsRq.push("streaming on");
    else if (flat.stream === false) bitsRq.push("streaming off");
    var ot = Number(flat.outgoingTokens != null ? flat.outgoingTokens : flat.outgoing_tokens);
    if (!isNaN(ot) && ot > 0) bitsRq.push(ot + " tok out");
    return bitsRq.join(" · ");
  }

  if (msg === "chat.bifrost.response" || ml === "upstream chat response") {
    var bitsRes = ["Relay response"];
    var scR = Number(flat.statusCode != null ? flat.statusCode : flat.status_code);
    if (!omitHttpInMsg && !isNaN(scR) && scR > 0) bitsRes.push("HTTP " + scR);
    var ut = usageTokensFromFlat(flat);
    if (!isNaN(ut)) bitsRes.push(ut + " usage tok");
    var rb = Number(flat.responseBytes != null ? flat.responseBytes : flat.response_bytes);
    if (!isNaN(rb) && rb >= 0) bitsRes.push(rb + " B");
    return bitsRes.join(" · ");
  }

  if (msg === "chat.bifrost.error") {
    var er = flat.err != null ? String(flat.err) : "";
    er = er.replace(/\s+/g, " ").trim();
    if (er.length > 200) er = er.slice(0, 199) + "…";
    return er ? "Relay failed · " + er : "Relay failed";
  }

  if (msg === "chat.routing.fallback") {
    var bitsFb = ["Fallback retry"];
    var modFb = bifrostShortTailModel(flat.upstreamModel);
    if (modFb) bitsFb.push(modFb);
    var stFb = Number(flat.status != null ? flat.status : flat.statusCode != null ? flat.statusCode : NaN);
    if (!omitHttpInMsg && !isNaN(stFb) && stFb > 0) bitsFb.push("HTTP " + stFb);
    var wr = flat.willRetry;
    if (wr === false) bitsFb.push("no retry");
    return bitsFb.join(" · ");
  }

  if (msg === "chat.routing.attempt" || ml === "virtual model fallback attempt") {
    var bitsVa = ["Routing attempt"];
    var modVa = bifrostShortTailModel(flat.upstreamModel);
    if (modVa) bitsVa.push(modVa);
    var att = Number(flat.attempt);
    var chain = Number(flat.chainLen);
    if (!isNaN(att) && !isNaN(chain) && chain > 0) bitsVa.push("attempt " + att + "/" + chain);
    return bitsVa.join(" · ");
  }

  if (msg === "chat.routing.resolved" || ml === "virtual model routing resolved") {
    var bitsVr = ["Routing resolved"];
    var modVr = bifrostShortTailModel(flat.upstreamModel);
    if (modVr) bitsVr.push(modVr);
    var attR = Number(flat.attempt);
    var chainR = Number(flat.chainLen);
    if (!isNaN(attR) && !isNaN(chainR) && chainR > 0) bitsVr.push("attempt " + attR + "/" + chainR);
    var scV = Number(flat.statusCode != null ? flat.statusCode : flat.status);
    if (!omitHttpInMsg && !isNaN(scV) && scV > 0) bitsVr.push("HTTP " + scV);
    return bitsVr.join(" · ");
  }

  if (
    msg === "chat.provider_limits.blocked" ||
    ml === "chat blocked by provider limits" ||
    ml === "skipping upstream model (provider limits)"
  ) {
    var rsn = flat.reason != null ? String(flat.reason).replace(/\s+/g, " ").trim() : "";
    if (rsn.length > 140) rsn = rsn.slice(0, 139) + "…";
    return rsn ? "Blocked by provider limits · " + rsn : "Blocked by provider limits";
  }

  if (!isBifrostSlug) return "";

  switch (msg) {
    case "bifrost.startup.banner":
      return "BiFrost starting";
    case "bifrost.version": {
      var ver = flat.bifrost_version != null ? String(flat.bifrost_version).trim() : "";
      return ver ? "BiFrost version " + ver : "BiFrost version";
    }
    case "bifrost.bootstrap.complete": {
      var bms = Number(flat.bootstrap_ms != null ? flat.bootstrap_ms : flat.bootstrapMs);
      if (!isNaN(bms) && bms >= 0) return "Startup finished · bootstrap " + Math.round(bms) + " ms";
      return "Startup finished · bootstrap complete";
    }
    case "bifrost.client.ready":
      return "Core client ready";
    case "bifrost.jobs.async_ready":
      return "Background jobs enabled";
    case "bifrost.governance.startup":
      return "Governance enabled";
    case "bifrost.mcp.startup":
      return "MCP catalog initializing";
    case "bifrost.mcp.persistence.disabled":
      return "MCP disabled · no config store";
    case "bifrost.jwt.startup": {
      var authH = bifrostTrimDetail(flat, 80).toLowerCase();
      if (authH.indexOf("jwt") >= 0) return "Auth · JWT";
      if (authH.indexOf("api") >= 0) return "Auth · API key";
      if (authH.indexOf("disabled") >= 0) return "Auth · disabled";
      return "Auth plugin started";
    }
    case "bifrost.auth.token_refresh":
      return "Auth · token refresh worker started";
    case "bifrost.config.loaded":
      return "Configuration loaded · supervised";
    case "bifrost.config.validation_failed":
      return bifrostTrimDetail(flat, 260)
        ? "Configuration invalid · " + bifrostTrimDetail(flat, 260)
        : "Configuration invalid";
    case "bifrost.config.schema_warn":
      return bifrostTrimDetail(flat, 260)
        ? "Configuration warning · " + bifrostTrimDetail(flat, 260)
        : "Configuration warning";
    case "bifrost.store.config_ready": {
      var store = bifrostTrimDetail(flat, 40).toLowerCase();
      if (store === "sqlite" || store === "memory") return "Config store ready · " + store;
      return "Config store ready";
    }
    case "bifrost.store.request_logs_ready":
      return "Usage / request log store ready";
    case "bifrost.catalog.sync": {
      var ncat = Number(flat.catalog_model_count != null ? flat.catalog_model_count : flat.catalogModelCount);
      var det = bifrostTrimDetail(flat, 120).toLowerCase();
      var lineCat = "";
      if (!isNaN(ncat) && ncat > 0) lineCat = "Model catalog updated · " + Math.round(ncat) + " models";
      else if (det.indexOf("pricing") >= 0) lineCat = "Model catalog / pricing · " + bifrostTrimDetail(flat, 120);
      else lineCat = "Model catalog sync";
      return lineCat;
    }
    case "bifrost.listen.http":
      return bifrostTrimDetail(flat, 200) ? "HTTP listening · " + bifrostTrimDetail(flat, 200) : "HTTP listening";
    case "bifrost.ready": {
      var urlR = flat.listen_url != null ? String(flat.listen_url).trim() : "";
      if (urlR) return "Ready · UI at " + urlR;
      var lp = Number(flat.listen_port != null ? flat.listen_port : flat.listenPort);
      if (!isNaN(lp) && lp > 0) return "Ready · port " + lp;
      return "Ready";
    }
    case "bifrost.plugin.status": {
      var pn = flat.plugin_name != null ? String(flat.plugin_name).trim() : "";
      var ps = flat.plugin_status != null ? String(flat.plugin_status).trim() : "";
      if (pn && ps) return "Plugin " + pn + " · " + ps;
      if (pn) return "Plugin " + pn;
      return "Plugin status";
    }
    case "bifrost.provider.loaded": {
      var pid = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      return pid ? "Provider registered · " + pid : "Provider registered";
    }
    case "bifrost.provider.health.ok": {
      var pidOk = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      return pidOk ? "Provider healthy · " + pidOk : "Provider healthy";
    }
    case "bifrost.provider.health.fail": {
      var pidBad = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      return pidBad ? "Provider health failed · " + pidBad : "Provider health failed";
    }
    case "bifrost.provider.key_loaded": {
      var pidK = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      return pidK ? "Provider API key loaded · " + pidK : "Provider API key loaded";
    }
    case "bifrost.provider.key_missing": {
      var pidM = flat.provider_id != null ? String(flat.provider_id).trim() : "";
      return pidM ? "Missing API key · " + pidM : "Missing API key";
    }
    case "bifrost.maintenance.log_retention": {
      var days = Number(flat.log_retention_days != null ? flat.log_retention_days : flat.logRetentionDays);
      if (!isNaN(days) && days > 0) return "Request log retention · " + Math.round(days) + " days";
      var d2 = bifrostTrimDetail(flat, 120);
      if (d2) return "Log retention · " + d2;
      return "Log retention / cleanup";
    }
    case "bifrost.transport.serve_error":
      return bifrostTrimDetail(flat, 260)
        ? "Connection error · " + bifrostTrimDetail(flat, 260)
        : "Connection error";
    case "bifrost.log.zerolog":
      return bifrostTrimDetail(flat, 280) ? bifrostTrimDetail(flat, 280) : "BiFrost";
    case "bifrost.unparsed":
      return bifrostTrimDetail(flat, 280)
        ? "Unrecognized BiFrost log · " + bifrostTrimDetail(flat, 280)
        : "Unrecognized BiFrost log line";
    case "bifrost.governance.rejected":
      return bifrostTrimDetail(flat, 200)
        ? "Rejected by governance · " + bifrostTrimDetail(flat, 200)
        : "Rejected by governance";
    case "bifrost.upstream.request":
      return bifrostTrimDetail(flat, 180) ? "Upstream request · " + bifrostTrimDetail(flat, 180) : "Upstream request started";
    case "bifrost.upstream.response":
      return bifrostTrimDetail(flat, 180) ? "Upstream response · " + bifrostTrimDetail(flat, 180) : "Upstream response";
    case "bifrost.upstream.error":
      return bifrostTrimDetail(flat, 200) ? "Upstream error · " + bifrostTrimDetail(flat, 200) : "Upstream error";
    case "bifrost.shutdown":
    case "bifrost.shutdown.signal":
      return bifrostTrimDetail(flat, 200) ? "Shutting down · " + bifrostTrimDetail(flat, 200) : "Shutting down";
    default:
      if (msg.indexOf("bifrost.") === 0) {
        return "BiFrost · " + msg;
      }
      return "";
  }
}

function bifrostSliceSinceLastBanner(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastIdx = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    var msg = String(getFlat(arr[i].parsed).msg || "");
    if (msg === "bifrost.startup.banner") lastIdx = i;
  }
  if (lastIdx < 0) return arr.slice();
  return arr.slice(lastIdx);
}

function bifrostEntryHasRateLimit(ent, getFlat) {
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var f = getFlat(ent && ent.parsed);
  var msg = String(f.msg != null ? f.msg : "").trim();
  if (msg === "bifrost.rate_limit") return true;
  var comb = (String((ent && ent.text) || "") + " " + String(f.msg || "")).toLowerCase();
  return comb.indexOf("429") >= 0 || comb.indexOf("rate limit") >= 0 || comb.indexOf("rate_limit") >= 0;
}

function isRelayResponseMsg(msg) {
  return msg === "upstream chat response" || msg === "chat.bifrost.response";
}

function outgoingTokensFromFlat(f) {
  var o = Number(f.outgoingTokens != null ? f.outgoingTokens : f.outgoing_tokens);
  return !isNaN(o) && o > 0 ? o : NaN;
}

function usageTokensFromFlat(f) {
  var ut = Number(f.usageTotalTokens != null ? f.usageTotalTokens : f.usage_total_tokens);
  if (!isNaN(ut) && ut > 0) return ut;
  var up = Number(f.usagePromptTokens != null ? f.usagePromptTokens : f.usage_prompt_tokens);
  var uc = Number(f.usageCompletionTokens != null ? f.usageCompletionTokens : f.usage_completion_tokens);
  var sum = (isNaN(up) ? 0 : up) + (isNaN(uc) ? 0 : uc);
  return sum > 0 ? sum : NaN;
}

function statusCodeFromFlat(f) {
  var sc = Number(f.statusCode != null ? f.statusCode : f.status_code);
  return !isNaN(sc) && sc > 0 ? sc : NaN;
}

/**
 * Relay / provider counters: prefer window after last bifrost.ready (includes full post-startup logs),
 * else after last bifrost.startup.banner (gateway restart clears buffer).
 */
function bifrostSliceForRelayMetrics(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastReady = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    if (String(getFlat(arr[i].parsed).msg || "") === "bifrost.ready") lastReady = i;
  }
  if (lastReady >= 0) return arr.slice(lastReady);
  return bifrostSliceSinceLastBanner(arr, getFlat);
}

/** Aggregate KV fields — scan entire buffer (newest-first) so version lines before the last banner row still appear. */
function bifrostCardModel(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var out = {
    version: "",
    configuration: "",
    port: "",
    auth: "",
    mcp: "",
    governance: "",
    providersUp: 0,
    providersTotal: 0,
    providersAnyDown: false,
    lastModel: "",
    catalogModelCount: 0
  };

  var loaded = {};
  var healthLast = {};

  var i;
  for (i = arr.length - 1; i >= 0; i--) {
    var f = getFlat(arr[i].parsed);
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "chat.bifrost.available_models") {
      var ng = Number(f.catalog_model_count != null ? f.catalog_model_count : f.catalogModelCount);
      if (!out.catalogModelCount && !isNaN(ng) && ng > 0) out.catalogModelCount = Math.round(ng);
      continue;
    }
    if (String(f.service || "").toLowerCase() !== "bifrost") continue;

    if (!out.version && f.bifrost_version) out.version = String(f.bifrost_version).trim();
    if (!out.version && msg === "bifrost.version" && f.bifrost_version) out.version = String(f.bifrost_version).trim();

    if (!out.configuration && msg === "bifrost.config.loaded") out.configuration = "supervised";

    if (!out.port && f.listen_port != null && !isNaN(Number(f.listen_port))) {
      out.port = String(Math.round(Number(f.listen_port)));
    }
    if (!out.port && msg === "bifrost.listen.http" && f.progress_detail) {
      var mport = String(f.progress_detail).match(/:(\d{2,5})\b/);
      if (mport) out.port = mport[1];
    }
    if (!out.port && msg === "bifrost.ready" && f.listen_port != null && !isNaN(Number(f.listen_port))) {
      out.port = String(Math.round(Number(f.listen_port)));
    }

    if (!out.auth && msg === "bifrost.jwt.startup") {
      var pd = String(f.progress_detail || f.auth_mode || "").toLowerCase();
      if (pd.indexOf("jwt") >= 0) out.auth = "jwt";
      else if (pd.indexOf("api") >= 0) out.auth = "api-key";
      else if (pd.indexOf("disabled") >= 0) out.auth = "disabled";
    }
    if (!out.auth && msg === "bifrost.plugin.status") {
      var pname = String(f.plugin_name != null ? f.plugin_name : "").toLowerCase();
      var pst = String(f.plugin_status != null ? f.plugin_status : "").toLowerCase();
      var plug = pname + " " + pst;
      if (plug.indexOf("jwt") >= 0 || plug.indexOf("auth") >= 0) out.auth = "jwt";
    }

    if (!out.auth && msg === "bifrost.auth.token_refresh") {
      out.auth = "jwt";
    }

    if (!out.catalogModelCount && msg === "bifrost.catalog.sync") {
      var ncm = Number(f.catalog_model_count != null ? f.catalog_model_count : f.catalogModelCount);
      if (!isNaN(ncm) && ncm > 0) {
        out.catalogModelCount = Math.round(ncm);
      }
    }

    if (!out.mcp && msg === "bifrost.mcp.startup") out.mcp = "enabled";
    if (!out.mcp && msg === "bifrost.mcp.persistence.disabled") out.mcp = "disabled";

    if (!out.governance && msg === "bifrost.governance.startup") out.governance = "enabled";

    if (msg === "bifrost.provider.loaded" && f.provider_id) {
      var pid = String(f.provider_id).trim();
      if (pid) loaded[pid] = true;
    }
    if (msg === "bifrost.provider.health.ok" && f.provider_id) {
      healthLast[String(f.provider_id).trim()] = true;
    }
    if (msg === "bifrost.provider.health.fail" && f.provider_id) {
      healthLast[String(f.provider_id).trim()] = false;
    }
  }

  var sliceM = bifrostSliceForRelayMetrics(arr, getFlat);
  for (i = sliceM.length - 1; i >= 0; i--) {
    var f2 = getFlat(sliceM[i].parsed);
    var msg2 = String(f2.msg != null ? f2.msg : "").trim();
    if (msg2 === "chat.bifrost.request" && f2.upstreamModel) {
      out.lastModel = String(f2.upstreamModel).trim();
      break;
    }
  }

  var ids = Object.keys(loaded);
  out.providersTotal = ids.length;
  var up = 0;
  var anyDown = false;
  for (var k = 0; k < ids.length; k++) {
    var id = ids[k];
    if (Object.prototype.hasOwnProperty.call(healthLast, id)) {
      if (healthLast[id] === false) anyDown = true;
      else up++;
    } else {
      up++;
    }
  }
  out.providersUp = up;
  out.providersAnyDown = anyDown;

  return out;
}

function bifrostCardMetrics(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var slice = bifrostSliceForRelayMetrics(arr, getFlat);

  var reqN = 0;
  var resN = 0;
  var errN = 0;
  var streamOn = 0;
  var streamOff = 0;
  var outgoingSum = 0;
  var usageSum = 0;
  var bytesSum = 0;
  var sc2xx = 0;
  var scErr = 0;
  var modelCounts = {};
  var rlN = 0;
  var relayOk = 0;
  var relayFail = 0;
  var rateLimitSlugN = 0;
  var relay429N = 0;
  var fallbackN = 0;

  var loaded = {};
  var healthLast = {};
  var catalogModelCount = 0;

  for (var i = 0; i < slice.length; i++) {
    var ent = slice[i] || {};
    var p = ent.parsed || {};
    var f = getFlat(p);
    var sh = p.shape || "";
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();

    if (bifrostEntryHasRateLimit(ent, getFlat)) rlN++;
    if (msg === "bifrost.rate_limit") rateLimitSlugN++;

    if (msg === "chat.routing.fallback") fallbackN++;

    if (msg === "chat.bifrost.request") {
      reqN++;
      var ot = outgoingTokensFromFlat(f);
      if (!isNaN(ot)) outgoingSum += ot;
      if (f.stream === true || f.stream === "true") streamOn++;
      else if (f.stream === false || f.stream === "false") streamOff++;
      var umr = f.upstreamModel != null && String(f.upstreamModel).trim() !== "" ? String(f.upstreamModel).trim() : "";
      if (umr) modelCounts[umr] = (modelCounts[umr] || 0) + 1;
    } else if (msg === "chat.bifrost.error" || msg.indexOf("bifrost.error") >= 0) {
      errN++;
      relayFail++;
    } else if (isRelayResponseMsg(msg)) {
      resN++;
      var utok = usageTokensFromFlat(f);
      if (!isNaN(utok)) usageSum += utok;
      var rb = Number(f.responseBytes != null ? f.responseBytes : f.response_bytes);
      if (!isNaN(rb) && rb > 0) bytesSum += rb;

      var scr = statusCodeFromFlat(f);
      if (!isNaN(scr)) {
        if (scr >= 200 && scr < 300) relayOk++;
        else if (scr >= 400) relayFail++;
        if (scr === 429) relay429N++;
      }
    }

    var sc = statusCodeFromFlat(f);
    if (!isNaN(sc) && sc > 0) {
      if (sh === "http.access" || isRelayResponseMsg(msg) || msg === "chat.bifrost.error") {
        if (sc >= 200 && sc < 300) sc2xx++;
        else if (sc >= 400) scErr++;
      }
    }

    if (String(f.service || "").toLowerCase() === "bifrost") {
      if (msg === "bifrost.provider.loaded" && f.provider_id) {
        var pid = String(f.provider_id).trim();
        if (pid) loaded[pid] = true;
      }
      if (msg === "bifrost.provider.health.ok" && f.provider_id) {
        healthLast[String(f.provider_id).trim()] = true;
      }
      if (msg === "bifrost.provider.health.fail" && f.provider_id) {
        healthLast[String(f.provider_id).trim()] = false;
      }
    }
  }

  for (var ic = slice.length - 1; ic >= 0; ic--) {
    var fcat = getFlat(slice[ic].parsed);
    var mcat = String(fcat.msg != null ? fcat.msg : "").trim();
    if (mcat !== "bifrost.catalog.sync" && mcat !== "chat.bifrost.available_models") continue;
    if (mcat === "bifrost.catalog.sync" && String(fcat.service || "").toLowerCase() !== "bifrost") continue;
    var ncat = Number(fcat.catalog_model_count != null ? fcat.catalog_model_count : fcat.catalogModelCount);
    if (!isNaN(ncat) && ncat > 0) {
      catalogModelCount = Math.round(ncat);
      break;
    }
  }

  var topModel = "";
  var topC = 0;
  for (var mk in modelCounts) {
    if (!Object.prototype.hasOwnProperty.call(modelCounts, mk)) continue;
    var c = modelCounts[mk];
    if (c > topC) {
      topC = c;
      topModel = mk;
    } else if (c === topC && topModel && mk.localeCompare(topModel) < 0) {
      topModel = mk;
    }
  }
  if (!topModel) topModel = "—";

  var ids = Object.keys(loaded);
  var providersTotal = ids.length;
  var providersUp = 0;
  var providersAnyDown = false;
  for (var ki = 0; ki < ids.length; ki++) {
    var idk = ids[ki];
    if (Object.prototype.hasOwnProperty.call(healthLast, idk)) {
      if (healthLast[idk] === false) providersAnyDown = true;
      else providersUp++;
    } else {
      providersUp++;
    }
  }

  var rateLimitBoxN = rateLimitSlugN + relay429N;

  return {
    reqN: reqN,
    resN: resN,
    errN: errN,
    streamOn: streamOn,
    streamOff: streamOff,
    outgoingSum: outgoingSum,
    usageSum: usageSum,
    bytesSum: bytesSum,
    sc2xx: sc2xx,
    scErr: scErr,
    topModel: topModel,
    rlN: rlN,
    relayOk: relayOk,
    relayFail: relayFail,
    rateLimitSlugN: rateLimitSlugN,
    relay429N: relay429N,
    rateLimitBoxN: rateLimitBoxN,
    fallbackN: fallbackN,
    providersTotal: providersTotal,
    providersUp: providersUp,
    providersAnyDown: providersAnyDown,
    catalogModelCount: catalogModelCount
  };
}

/**
 * Per-provider health snapshot for the BiFrost provider-health strip.
 *
 * Walks `arr` oldest-to-newest so the latest health/key event for each provider id
 * wins. A provider counts as **loaded** if seen via `bifrost.provider.loaded` /
 * `bifrost.provider.key_loaded`, or implicitly via any health/key_missing event.
 *
 * Returns a list (sorted by id) of `{ id, state }` where state is one of:
 *   - "down"        latest event was `bifrost.provider.health.fail`
 *   - "key_missing" latest event was `bifrost.provider.key_missing`
 *   - "up"          latest event was `bifrost.provider.health.ok` OR provider
 *                   was loaded but never probed (matches existing
 *                   `bifrostCardModel.providersUp` convention).
 *   - "unknown"     reserved (currently unreachable; kept for forward-compat).
 */
function bifrostProviderHealthList(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var loaded = {};
  var lastEvent = {};
  for (var i = 0; i < arr.length; i++) {
    var f = getFlat(arr[i].parsed);
    var msg = String(f.msg != null ? f.msg : "").trim();
    var pid = f.provider_id != null ? String(f.provider_id).trim() : "";
    if (!pid) continue;
    if (
      msg === "bifrost.provider.loaded" ||
      msg === "bifrost.provider.key_loaded" ||
      msg === "bifrost.provider.health.ok" ||
      msg === "bifrost.provider.health.fail" ||
      msg === "bifrost.provider.key_missing"
    ) {
      loaded[pid] = true;
    }
    if (msg === "bifrost.provider.health.ok") lastEvent[pid] = "up";
    else if (msg === "bifrost.provider.health.fail") lastEvent[pid] = "down";
    else if (msg === "bifrost.provider.key_missing") lastEvent[pid] = "key_missing";
  }
  var ids = Object.keys(loaded).sort();
  var out = [];
  for (var k = 0; k < ids.length; k++) {
    var id = ids[k];
    out.push({ id: id, state: Object.prototype.hasOwnProperty.call(lastEvent, id) ? lastEvent[id] : "up" });
  }
  return out;
}

/**
 * Bucket every chat-relay row in the buffer by HTTP outcome for the BiFrost
 * relay-outcome strip. Uses `bifrostSliceForRelayMetrics` so counts reset on
 * BiFrost restart (matches the rest of the BiFrost card).
 *
 * Buckets (chat relay only — `bifrost.rate_limit` is excluded because it is
 * subprocess inbound HTTP, often `/v1/embeddings`, not a chat completion call;
 * the existing "Rate limits" mini-card still aggregates both):
 *   - ok          `chat.bifrost.response` with HTTP 2xx
 *   - redirect    `chat.bifrost.response` with HTTP 3xx (rare)
 *   - rateLimit   `chat.bifrost.response` HTTP 429
 *   - clientErr   `chat.bifrost.response` 4xx (excluding 429)
 *   - serverErr   `chat.bifrost.response` 5xx
 *   - errorNoResp `chat.bifrost.error` (relay fetch failed before any response)
 *   - inFlight    `chat.bifrost.request` with no matching response/error in buffer
 */
function bifrostRelayOutcomeBuckets(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var slice = bifrostSliceForRelayMetrics(arr, getFlat);

  var ok = 0;
  var redirect = 0;
  var rateLimit = 0;
  var clientErr = 0;
  var serverErr = 0;
  var errorNoResp = 0;
  var requestN = 0;
  var responseN = 0;

  for (var i = 0; i < slice.length; i++) {
    var f = getFlat(slice[i].parsed);
    var msg = String(f.msg != null ? f.msg : "").trim();
    if (msg === "chat.bifrost.request") {
      requestN++;
      continue;
    }
    if (isRelayResponseMsg(msg)) {
      responseN++;
      var sc = statusCodeFromFlat(f);
      if (!isNaN(sc) && sc > 0) {
        if (sc === 429) rateLimit++;
        else if (sc >= 200 && sc < 300) ok++;
        else if (sc >= 300 && sc < 400) redirect++;
        else if (sc >= 400 && sc < 500) clientErr++;
        else if (sc >= 500) serverErr++;
        else ok++;
      } else {
        ok++;
      }
      continue;
    }
    if (msg === "chat.bifrost.error") {
      errorNoResp++;
      continue;
    }
  }

  var settled = ok + redirect + rateLimit + clientErr + serverErr + errorNoResp;
  var inFlight = requestN - settled;
  if (inFlight < 0) inFlight = 0;

  return {
    ok: ok,
    redirect: redirect,
    rateLimit: rateLimit,
    clientErr: clientErr,
    serverErr: serverErr,
    errorNoResp: errorNoResp,
    inFlight: inFlight,
    requestN: requestN,
    responseN: responseN,
    total: settled + inFlight
  };
}

globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
globalThis.ClaudiaLogs.Derive = globalThis.ClaudiaLogs.Derive || {};
globalThis.ClaudiaLogs.Derive.bifrostOperatorLine = bifrostOperatorLine;
globalThis.ClaudiaLogs.Derive.bifrostEntryHasRateLimit = bifrostEntryHasRateLimit;
globalThis.ClaudiaLogs.Derive.bifrostSliceSinceLastBanner = bifrostSliceSinceLastBanner;
globalThis.ClaudiaLogs.Derive.bifrostSliceForRelayMetrics = bifrostSliceForRelayMetrics;
globalThis.ClaudiaLogs.Derive.bifrostCardModel = bifrostCardModel;
globalThis.ClaudiaLogs.Derive.bifrostCardMetrics = bifrostCardMetrics;
globalThis.ClaudiaLogs.Derive.bifrostProviderHealthList = bifrostProviderHealthList;
globalThis.ClaudiaLogs.Derive.bifrostRelayOutcomeBuckets = bifrostRelayOutcomeBuckets;
