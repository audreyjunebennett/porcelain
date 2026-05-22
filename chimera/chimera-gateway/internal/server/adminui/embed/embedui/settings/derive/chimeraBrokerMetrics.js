/**
 * Pure metrics derivation for the chimera-broker service card.
 *
 * Exports:
 * - ChimeraSettings.Derive.brokerOperatorLine(flat, opts?)
 * - ChimeraSettings.Derive.brokerEntryHasRateLimit(ent, getFlat)
 * - ChimeraSettings.Derive.brokerSliceSinceLastBanner(arr, getFlat)
 * - ChimeraSettings.Derive.brokerCardModel(arr, getFlat)
 * - ChimeraSettings.Derive.brokerCardMetrics(arr, getFlat)
 */

function brokerLegacyToCanonical(msg) {
  var s = String(msg != null ? msg : "").trim();
  if (s.indexOf("chimera-broker.") === 0) return "broker." + s.slice("chimera-broker.".length);
  return s;
}

function brokerCanonicalMsg(flat) {
  if (!flat || typeof flat !== "object") return "";
  var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
  if (oc && typeof oc.resolveFlat === "function") {
    var slug = oc.resolveFlat(flat);
    if (slug) return slug;
  }
  return brokerLegacyToCanonical(flat.msg != null ? flat.msg : flat.message);
}

function brokerServiceMatch(flat) {
  var svc = String((flat && flat.service) || "").toLowerCase();
  return svc === "chimera-broker" || svc === "broker";
}

function brokerPathFromTarget(target) {
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

function brokerShortTailModel(model) {
  var m = model != null ? String(model).trim() : "";
  if (!m) return "";
  var parts = m.split("/");
  var tail = parts[parts.length - 1] || m;
  return tail.length > 48 ? tail.slice(0, 46) + "…" : tail;
}

function chimeraBrokerTrimDetail(flat, maxLen) {
  maxLen = maxLen > 0 ? maxLen : 220;
  var pd = flat.progress_detail != null ? String(flat.progress_detail) : "";
  if (!pd) return "";
  var t = pd.replace(/\s+/g, " ").trim();
  return t.length > maxLen ? t.slice(0, maxLen - 1) + "…" : t;
}

function brokerHttpInboundLine(flat, rateLimit, opts) {
  opts = opts || {};
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
  bits.push(rateLimit ? "Rate limited" : "Inbound");
  var pid = flat.provider_id != null ? String(flat.provider_id).trim() : "";
  if (pid) bits.push("provider " + pid);
  else if (flat.progress_detail != null && String(flat.progress_detail).trim()) {
    bits.push(String(flat.progress_detail).trim());
  }
  bits.push(meth + " " + path);
  if (!omitStatus) bits.push("→ " + st);
  if (!isNaN(ms) && ms >= 0) bits.push(Math.round(ms) + " ms");
  return bits.join(" · ");
}

/**
 * One-line operator headline for summarized logs (registry-driven; see logs/render/operatorMessage.js).
 * Returns "" when this row is not part of the chimera-broker / relay vocabulary.
 */
function chimeraBrokerOperatorLine(flat, opts) {
  if (!flat || typeof flat !== "object") return "";
  var render = globalThis.ChimeraSettings && ChimeraSettings.Render;
  if (render && typeof render.isBrokerOrRelayLine === "function" && !render.isBrokerOrRelayLine(flat)) {
    return "";
  }
  if (render && typeof render.operatorMessage === "function") {
    return render.operatorMessage(flat, opts) || "";
  }
  return "";
}

function chimeraBrokerSliceSinceLastBanner(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastIdx = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    var msg = brokerCanonicalMsg(getFlat(arr[i].parsed));
    if (msg === "broker.startup.banner" || msg === "chimera-broker.startup.banner") lastIdx = i;
  }
  if (lastIdx < 0) return arr.slice();
  return arr.slice(lastIdx);
}

function chimeraBrokerEntryHasRateLimit(ent, getFlat) {
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var f = getFlat(ent && ent.parsed);
  var msg = brokerCanonicalMsg(f);
  if (msg === "broker.rate_limit" || msg === "chimera-broker.rate_limit") return true;
  var comb = (String((ent && ent.text) || "") + " " + String(f.msg || "")).toLowerCase();
  return comb.indexOf("429") >= 0 || comb.indexOf("rate limit") >= 0 || comb.indexOf("rate_limit") >= 0;
}

function isRelayResponseMsg(msg) {
  return msg === "upstream chat response" || msg === "chat.chimera-broker.response";
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

/** Matches Go catalog.CatalogSnapshotFreshness (2 minutes). */
var BROKER_CATALOG_FRESH_MS = 120000;

function brokerProviderIdFromFlat(f) {
  if (!f || f.provider_id == null) return "";
  return String(f.provider_id).trim().toLowerCase();
}

function brokerParseStringSetField(f, key) {
  if (!f || typeof f !== "object") return null;
  var raw = f[key];
  if (Array.isArray(raw)) {
    var out = {};
    for (var i = 0; i < raw.length; i++) {
      var s = String(raw[i] != null ? raw[i] : "").trim().toLowerCase();
      if (s) out[s] = true;
    }
    return Object.keys(out).length ? out : null;
  }
  if (typeof raw === "string" && raw.trim()) {
    try {
      var parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return brokerParseStringSetField({ providers: parsed }, "providers");
      }
    } catch (e) { }
  }
  return null;
}

function brokerParseRFC3339Ms(value) {
  if (value == null || value === "") return NaN;
  var t = Date.parse(String(value).trim());
  return isNaN(t) ? NaN : t;
}

function brokerIsTruthyOk(value) {
  return value === true || value === "true" || value === 1 || value === "1";
}

function brokerIsCatalogLineFresh(atMs) {
  if (isNaN(atMs)) return true;
  return Date.now() - atMs <= BROKER_CATALOG_FRESH_MS;
}

/**
 * Latest chat.chimera-broker.available_models line (gateway /v1/models poll).
 * Returns { kind: "ok"|"fail"|"stale"|"none", set?, atMs?, err? }.
 */
function brokerLatestCatalogSignal(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  for (var i = arr.length - 1; i >= 0; i--) {
    var f = getFlat(arr[i].parsed);
    if (brokerCanonicalMsg(f) !== "chat.chimera-broker.available_models") continue;
    var atMs = brokerParseRFC3339Ms(f.fetched_at != null ? f.fetched_at : f.fetchedAt);
    var fresh = brokerIsCatalogLineFresh(atMs);
    if (brokerIsTruthyOk(f.ok)) {
      return {
        kind: fresh ? "ok" : "stale",
        set: brokerParseStringSetField(f, "providers"),
        atMs: atMs
      };
    }
    return {
      kind: fresh ? "fail" : "stale",
      atMs: atMs,
      err: String(f.err != null ? f.err : "")
    };
  }
  return { kind: "none" };
}

/** @deprecated use brokerLatestCatalogSignal */
function brokerLatestCatalogLiveSet(arr, getFlat) {
  var sig = brokerLatestCatalogSignal(arr, getFlat);
  if (sig.kind === "ok") {
    return { set: sig.set, fresh: true };
  }
  return { set: null, fresh: false };
}

function brokerProviderLoadedByMsg(msg) {
  return (
    msg === "broker.provider.loaded" ||
    msg === "broker.provider.key_loaded" ||
    msg === "broker.provider.health.ok" ||
    msg === "broker.provider.health.fail" ||
    msg === "broker.provider.model_discovery.fail" ||
    msg === "broker.provider.key_missing"
  );
}

function brokerProviderEventState(msg) {
  if (msg === "broker.provider.health.ok") return "up";
  if (msg === "broker.provider.health.fail" || msg === "broker.provider.model_discovery.fail") return "down";
  if (msg === "broker.provider.key_missing") return "key_missing";
  return "";
}

function brokerResolveProviderState(id, lastEvent, catalogSignal) {
  var pid = String(id != null ? id : "").trim().toLowerCase();
  if (!pid) return "unknown";
  if (lastEvent[pid] === "key_missing") return "key_missing";
  if (lastEvent[pid] === "down") return "down";
  if (lastEvent[pid] === "up") return "up";
  var cat = catalogSignal || { kind: "none" };
  if (cat.kind === "ok") {
    return cat.set && cat.set[pid] ? "up" : "down";
  }
  if (cat.kind === "fail") {
    return "down";
  }
  return "unknown";
}

/**
 * Walk broker + gateway lines to build loaded providers, subprocess events, and catalog liveness.
 * When slice is provided, broker subprocess events come from slice only; catalog uses full arr.
 */
function brokerCollectProviderHealthContext(arr, getFlat, slice) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var eventSource = slice != null ? slice : arr;
  var loaded = {};
  var lastEvent = {};
  var i;
  for (i = 0; i < eventSource.length; i++) {
    var f = getFlat(eventSource[i].parsed);
    var msg = brokerCanonicalMsg(f);
    var pid = brokerProviderIdFromFlat(f);
    if (!pid) continue;
    if (brokerProviderLoadedByMsg(msg)) loaded[pid] = true;
    var ev = brokerProviderEventState(msg);
    if (ev) lastEvent[pid] = ev;
  }
  return {
    loaded: loaded,
    lastEvent: lastEvent,
    catalogLive: brokerLatestCatalogSignal(arr, getFlat)
  };
}

function brokerCountProviderHealth(loaded, lastEvent, catalogLive) {
  var ids = Object.keys(loaded);
  var up = 0;
  var anyDown = false;
  for (var k = 0; k < ids.length; k++) {
    var st = brokerResolveProviderState(ids[k], lastEvent, catalogLive);
    if (st === "up") up++;
    else if (st === "down") anyDown = true;
  }
  return { total: ids.length, up: up, anyDown: anyDown };
}

/**
 * Relay / provider counters: prefer window after last chimera-broker.ready (includes full post-startup logs),
 * else after last chimera-broker.startup.banner (gateway restart clears buffer).
 */
function chimeraBrokerSliceForRelayMetrics(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var lastReady = -1;
  var i;
  for (i = 0; i < arr.length; i++) {
    var m = brokerCanonicalMsg(getFlat(arr[i].parsed));
    if (m === "broker.ready" || m === "chimera-broker.ready") lastReady = i;
  }
  if (lastReady >= 0) return arr.slice(lastReady);
  return chimeraBrokerSliceSinceLastBanner(arr, getFlat);
}

/** Aggregate KV fields — scan entire buffer (newest-first) so version lines before the last banner row still appear. */
function chimeraBrokerCardModel(arr, getFlat) {
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

  var i;
  for (i = arr.length - 1; i >= 0; i--) {
    var f = getFlat(arr[i].parsed);
    var msg = brokerCanonicalMsg(f);
    if (msg === "chat.chimera-broker.available_models") {
      var ng = Number(f.catalog_model_count != null ? f.catalog_model_count : f.catalogModelCount);
      if (!out.catalogModelCount && !isNaN(ng) && ng > 0) out.catalogModelCount = Math.round(ng);
      continue;
    }
    if (!brokerServiceMatch(f)) continue;

    if (!out.version && f.chimera_broker_version) out.version = String(f.chimera_broker_version).trim();
    if (!out.version && msg === "broker.version" && f.chimera_broker_version) out.version = String(f.chimera_broker_version).trim();

    if (!out.configuration && msg === "broker.config.loaded") out.configuration = "supervised";

    if (!out.port && f.listen_port != null && !isNaN(Number(f.listen_port))) {
      out.port = String(Math.round(Number(f.listen_port)));
    }
    if (!out.port && msg === "broker.listen.http" && f.progress_detail) {
      var mport = String(f.progress_detail).match(/:(\d{2,5})\b/);
      if (mport) out.port = mport[1];
    }
    if (!out.port && msg === "broker.ready" && f.listen_port != null && !isNaN(Number(f.listen_port))) {
      out.port = String(Math.round(Number(f.listen_port)));
    }

    if (!out.auth && msg === "broker.jwt.startup") {
      var pd = String(f.progress_detail || f.auth_mode || "").toLowerCase();
      if (pd.indexOf("jwt") >= 0) out.auth = "jwt";
      else if (pd.indexOf("api") >= 0) out.auth = "api-key";
      else if (pd.indexOf("disabled") >= 0) out.auth = "disabled";
    }
    if (!out.auth && msg === "broker.plugin.status") {
      var pname = String(f.plugin_name != null ? f.plugin_name : "").toLowerCase();
      var pst = String(f.plugin_status != null ? f.plugin_status : "").toLowerCase();
      var plug = pname + " " + pst;
      if (plug.indexOf("jwt") >= 0 || plug.indexOf("auth") >= 0) out.auth = "jwt";
    }

    if (!out.auth && msg === "broker.auth.token_refresh") {
      out.auth = "jwt";
    }

    if (!out.catalogModelCount && msg === "broker.catalog.sync") {
      var ncm = Number(f.catalog_model_count != null ? f.catalog_model_count : f.catalogModelCount);
      if (!isNaN(ncm) && ncm > 0) {
        out.catalogModelCount = Math.round(ncm);
      }
    }

    if (!out.mcp && msg === "broker.mcp.startup") out.mcp = "enabled";
    if (!out.mcp && msg === "broker.mcp.persistence.disabled") out.mcp = "disabled";

    if (!out.governance && msg === "broker.governance.startup") out.governance = "enabled";
  }

  var healthCtx = brokerCollectProviderHealthContext(arr, getFlat, null);
  var healthCounts = brokerCountProviderHealth(healthCtx.loaded, healthCtx.lastEvent, healthCtx.catalogLive);
  out.providersTotal = healthCounts.total;
  out.providersUp = healthCounts.up;
  out.providersAnyDown = healthCounts.anyDown;

  var sliceM = chimeraBrokerSliceForRelayMetrics(arr, getFlat);
  for (i = sliceM.length - 1; i >= 0; i--) {
    var f2 = getFlat(sliceM[i].parsed);
    var msg2 = brokerCanonicalMsg(f2);
    if (msg2 === "chat.chimera-broker.request" && f2.upstreamModel) {
      out.lastModel = String(f2.upstreamModel).trim();
      break;
    }
  }

  return out;
}

function chimeraBrokerCardMetrics(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var slice = chimeraBrokerSliceForRelayMetrics(arr, getFlat);

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

  var catalogModelCount = 0;

  for (var i = 0; i < slice.length; i++) {
    var ent = slice[i] || {};
    var p = ent.parsed || {};
    var f = getFlat(p);
    var sh = p.shape || "";
    var msg = brokerCanonicalMsg(f);

    if (chimeraBrokerEntryHasRateLimit(ent, getFlat)) rlN++;
    if (msg === "broker.rate_limit" || msg === "chimera-broker.rate_limit") rateLimitSlugN++;

    if (msg === "chat.routing.fallback") fallbackN++;

    if (msg === "chat.chimera-broker.request") {
      reqN++;
      var ot = outgoingTokensFromFlat(f);
      if (!isNaN(ot)) outgoingSum += ot;
      if (f.stream === true || f.stream === "true") streamOn++;
      else if (f.stream === false || f.stream === "false") streamOff++;
      var umr = f.upstreamModel != null && String(f.upstreamModel).trim() !== "" ? String(f.upstreamModel).trim() : "";
      if (umr) modelCounts[umr] = (modelCounts[umr] || 0) + 1;
    } else if (msg === "chat.chimera-broker.error" || msg.indexOf("chimera-broker.error") >= 0) {
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
      if (sh === "http.access" || isRelayResponseMsg(msg) || msg === "chat.chimera-broker.error") {
        if (sc >= 200 && sc < 300) sc2xx++;
        else if (sc >= 400) scErr++;
      }
    }
  }

  for (var ic = arr.length - 1; ic >= 0; ic--) {
    var fcat = getFlat(arr[ic].parsed);
    var mcat = brokerCanonicalMsg(fcat);
    if (mcat !== "broker.catalog.sync" && mcat !== "chat.chimera-broker.available_models") continue;
    if (mcat === "broker.catalog.sync" && !brokerServiceMatch(fcat)) continue;
    var ncat = Number(fcat.catalog_model_count != null ? fcat.catalog_model_count : fcat.catalogModelCount);
    if (!isNaN(ncat) && ncat > 0) {
      catalogModelCount = Math.round(ncat);
      break;
    }
  }

  var healthCtx = brokerCollectProviderHealthContext(arr, getFlat, slice);
  var healthCounts = brokerCountProviderHealth(healthCtx.loaded, healthCtx.lastEvent, healthCtx.catalogLive);
  var providersTotal = healthCounts.total;
  var providersUp = healthCounts.up;
  var providersAnyDown = healthCounts.anyDown;

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
 * Per-provider health snapshot for the chimera-broker provider-health strip.
 *
 * Walks `arr` oldest-to-newest so the latest health/key event for each provider id
 * wins. A provider counts as **loaded** if seen via `broker.provider.loaded` /
 * `broker.provider.key_loaded`, or implicitly via any health/key_missing event.
 *
 * Liveness (configured + models in live catalog) comes from the latest fresh
 * `chat.chimera-broker.available_models` line (Option B — one gateway poll, O(1) requests).
 * Subprocess failures (`broker.provider.health.fail`, `broker.provider.model_discovery.fail`)
 * override catalog when they occur later in the buffer (Option C).
 *
 * Returns a list (sorted by id) of `{ id, state }` where state is one of:
 *   - "down"        health.fail, model_discovery.fail, or absent from fresh catalog
 *   - "key_missing" latest event was key_missing
 *   - "up"          health.ok or present in fresh catalog
 *   - "unknown"     loaded but no fresh catalog or explicit health signal
 */
function chimeraBrokerProviderHealthList(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var ctx = brokerCollectProviderHealthContext(arr, getFlat, null);
  var ids = Object.keys(ctx.loaded).sort();
  var out = [];
  for (var k = 0; k < ids.length; k++) {
    var id = ids[k];
    out.push({
      id: id,
      state: brokerResolveProviderState(id, ctx.lastEvent, ctx.catalogLive)
    });
  }
  return out;
}

/** First operator-facing hint for collapsed broker card subtitle from log-derived health. */
function chimeraBrokerCollapsedHealthSubtitle(arr, getFlat) {
  var list = chimeraBrokerProviderHealthList(arr, getFlat);
  for (var i = 0; i < list.length; i++) {
    var ent = list[i] || {};
    var id = ent.id != null ? String(ent.id).trim() : "";
    if (ent.state === "down") {
      return "Provider unavailable" + (id ? " · " + id : "");
    }
    if (ent.state === "key_missing") {
      return "Missing key" + (id ? " for " + id : "");
    }
  }
  return "";
}

/**
 * Bucket every chat-relay row in the buffer by HTTP outcome for the BiFrost
 * relay-outcome strip. Uses `chimeraBrokerSliceForRelayMetrics` so counts reset on
 * BiFrost restart (matches the rest of the BiFrost card).
 *
 * Buckets (chat relay only — `chimera-broker.rate_limit` is excluded because it is
 * subprocess inbound HTTP, often `/v1/embeddings`, not a chat completion call;
 * the existing "Rate limits" mini-card still aggregates both):
 *   - ok          `chat.chimera-broker.response` with HTTP 2xx
 *   - redirect    `chat.chimera-broker.response` with HTTP 3xx (rare)
 *   - rateLimit   `chat.chimera-broker.response` HTTP 429
 *   - clientErr   `chat.chimera-broker.response` 4xx (excluding 429)
 *   - serverErr   `chat.chimera-broker.response` 5xx
 *   - errorNoResp `chat.chimera-broker.error` (relay fetch failed before any response)
 *   - inFlight    `chat.chimera-broker.request` with no matching response/error in buffer
 */
function chimeraBrokerRelayOutcomeBuckets(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var slice = chimeraBrokerSliceForRelayMetrics(arr, getFlat);

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
    if (msg === "chat.chimera-broker.request") {
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
    if (msg === "chat.chimera-broker.error") {
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

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
globalThis.ChimeraSettings.Derive.chimeraBrokerOperatorLine = chimeraBrokerOperatorLine;
globalThis.ChimeraSettings.Derive.chimeraBrokerEntryHasRateLimit = chimeraBrokerEntryHasRateLimit;
globalThis.ChimeraSettings.Derive.chimeraBrokerSliceSinceLastBanner = chimeraBrokerSliceSinceLastBanner;
globalThis.ChimeraSettings.Derive.chimeraBrokerSliceForRelayMetrics = chimeraBrokerSliceForRelayMetrics;
globalThis.ChimeraSettings.Derive.chimeraBrokerCardModel = chimeraBrokerCardModel;
globalThis.ChimeraSettings.Derive.chimeraBrokerCardMetrics = chimeraBrokerCardMetrics;
globalThis.ChimeraSettings.Derive.chimeraBrokerProviderHealthList = chimeraBrokerProviderHealthList;
globalThis.ChimeraSettings.Derive.chimeraBrokerCollapsedHealthSubtitle = chimeraBrokerCollapsedHealthSubtitle;
globalThis.ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets = chimeraBrokerRelayOutcomeBuckets;
