/**
 * Gateway service card: subtitle priority, KV row, counters, and full-log row filter.
 *
 * Exports:
 * - ClaudiaLogs.Derive.gatewayCardModel(arr, getFlat)
 * - ClaudiaLogs.Derive.gatewayPanelHideRow(ent, getFlat)
 */

function gatewayNormMsg(flat) {
  if (!flat) return "";
  var m = flat.msg != null ? flat.msg : flat.message;
  return String(m || "")
    .trim()
    .toLowerCase();
}

function gatewayBasenamePath(p) {
  p = String(p || "").trim();
  if (!p) return "";
  var parts = p.replace(/\\/g, "/").split("/");
  var tail = parts[parts.length - 1];
  return tail || p;
}

/** True when this http.access row is probe noise for the gateway panel full log (2xx only). */
function gatewayPanelHideRow(ent, getFlat) {
  if (!ent || !ent.parsed) return false;
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var f = getFlat(ent.parsed);
  var p = ent.parsed;
  var sh = p.shape || "";
  var msg = gatewayNormMsg(f);
  var isHttp =
    sh === "http.access" ||
    msg === "gateway.http.access" ||
    msg === "http response" ||
    (f.method && f.path != null && f.statusCode !== undefined && f.statusCode !== null);
  if (!isHttp) return false;
  var sc = Number(f.statusCode);
  if (isNaN(sc) || sc < 200 || sc >= 300) return false;
  var pathStr = String(f.path || "").split("?")[0];
  switch (pathStr) {
    case "/health":
    case "/status":
    case "/api/ui/logs":
    case "/api/ui/logs/stream":
    case "/ui/logs":
    case "/api/ui/metrics":
      return true;
    default:
      return false;
  }
}

function gatewayCardModel(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var out = {
    subtitle: "—",
    cardStatus: "active",
    kv: {
      listening: "—",
      upstream: "—",
      config: "—",
      configReloadStale: false,
      apiKeys: "—",
      apiKeysTint: "none",
      routingRules: "—",
      supervised: "—"
    },
    counters: {
      http2xx: 0,
      httpNot2xx: 0,
      http429: 0,
      chatReq: 0,
      chatResp: 0,
      chatErr: 0,
      ragQuery: 0,
      ragHit: 0,
      ragRetrieveErr: "",
      ingestOk: 0,
      ingestFail: 0
    }
  };

  var lastConfigReloadOk = -1;
  var lastConfigReloadErr = -1;
  var i;
  var f;
  var msg;

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    msg = gatewayNormMsg(f);
    if (msg === "gateway.config.reloaded") lastConfigReloadOk = i;
    if (msg === "gateway.config.reload_failed" || msg === "gateway.config.missing") lastConfigReloadErr = i;
  }

  var listeningAddr = "";
  var upstreamUrl = "";
  var bifrostData = "";
  var qSup = null;
  var idxSup = null;
  var startupConfigPath = "";

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    msg = gatewayNormMsg(f);
    if (msg === "gateway.startup.listening") {
      if (f.addr != null && String(f.addr).trim() !== "") listeningAddr = String(f.addr);
      if (f.upstream != null && String(f.upstream).trim() !== "") upstreamUrl = String(f.upstream);
      if (f.bifrost_data != null && String(f.bifrost_data).trim() !== "") bifrostData = String(f.bifrost_data);
      if (f.qdrant_supervised !== undefined && f.qdrant_supervised !== null) qSup = !!f.qdrant_supervised;
      if (f.indexer_supervised !== undefined && f.indexer_supervised !== null) idxSup = !!f.indexer_supervised;
      if (f.config != null && String(f.config).trim() !== "") startupConfigPath = String(f.config);
    }
    if (msg === "gateway.startup.config_resolved") {
      if (f.filePath != null && String(f.filePath).trim() !== "") startupConfigPath = String(f.filePath);
    }
  }

  var configReloadPath = "";
  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    msg = gatewayNormMsg(f);
    if (msg === "gateway.config.reloaded" && f.path != null && String(f.path).trim() !== "")
      configReloadPath = String(f.path);
  }

  var configPathFull = configReloadPath || startupConfigPath;
  out.kv.config = configPathFull ? gatewayBasenamePath(configPathFull) : "—";
  out.kv.configReloadStale = lastConfigReloadErr > lastConfigReloadOk && lastConfigReloadErr >= 0;
  if (out.kv.configReloadStale && out.kv.config !== "—") out.kv.config += " ⚠";

  var lastAuthReload = -1;
  var lastCredFail = -1;
  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    msg = gatewayNormMsg(f);
    if (msg === "gateway.auth.reloaded") {
      lastAuthReload = i;
      if (f.count != null && !isNaN(Number(f.count))) out.kv.apiKeys = String(Math.round(Number(f.count)));
      else if (f.count != null) out.kv.apiKeys = String(f.count);
    }
    if (msg === "gateway.auth.file_missing" || msg === "gateway.auth.read_failed" || msg === "gateway.auth.parse_failed") {
      if (i >= lastCredFail) lastCredFail = i;
    }
  }
  if (out.kv.apiKeys === "—") out.kv.apiKeys = "—";
  out.kv.apiKeysTint = lastCredFail > lastAuthReload && lastCredFail >= 0 ? "error" : "none";

  out.kv.listening = listeningAddr || "—";
  out.kv.upstream = upstreamUrl || "—";

  for (i = arr.length - 1; i >= 0; i--) {
    f = getFlat(arr[i].parsed);
    if (gatewayNormMsg(f) === "routing.policy.reloaded" && f.rules != null) {
      out.kv.routingRules = String(f.rules);
      break;
    }
  }

  var supParts = [];
  if (qSup === true) supParts.push("qdrant");
  else if (qSup === false) supParts.push("no qdrant");
  if (bifrostData) supParts.push("bifrost");
  if (idxSup === true) supParts.push("indexer");
  else if (idxSup === false) supParts.push("no indexer");
  out.kv.supervised = supParts.length ? supParts.join(" · ") : "—";

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    msg = gatewayNormMsg(f);
    var psh = arr[i].parsed.shape || "";

    var httpShape =
      psh === "http.access" ||
      msg === "gateway.http.access" ||
      msg === "http response" ||
      (f.method && f.path != null && f.statusCode !== undefined && f.statusCode !== null);
    if (httpShape) {
      var sc = Number(f.statusCode);
      if (!isNaN(sc)) {
        if (sc >= 200 && sc < 300) out.counters.http2xx++;
        else out.counters.httpNot2xx++;
        if (sc === 429) out.counters.http429++;
      }
    }

    if (msg === "chat.request") out.counters.chatReq++;
    if (msg === "chat.bifrost.response" || msg === "upstream chat response") out.counters.chatResp++;
    if (msg === "chat.bifrost.error") out.counters.chatErr++;

    if (msg === "rag.query") out.counters.ragQuery++;
    if (msg === "rag.hit") out.counters.ragHit++;

    if (msg === "ingest.complete") out.counters.ingestOk++;
    if (msg === "ingest.failed" || msg === "ingest.chunked.error") out.counters.ingestFail++;
  }

  for (i = arr.length - 1; i >= 0; i--) {
    f = getFlat(arr[i].parsed);
    if (gatewayNormMsg(f) === "rag.retrieve.error" && f.err != null && String(f.err).trim() !== "") {
      var er = String(f.err).replace(/\s+/g, " ");
      out.counters.ragRetrieveErr = er.length > 140 ? er.slice(0, 138) + "…" : er;
      break;
    }
  }

  function tierPick() {
    var j;
    var m;
    var fJ;
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      m = gatewayNormMsg(fJ);
      if (m === "gateway.config.reload_failed") return { subtitle: "Gateway config reload failed", cardStatus: "error" };
      if (m === "gateway.config.missing") return { subtitle: "Gateway config missing", cardStatus: "error" };
      if (m === "gateway.auth.parse_failed") return { subtitle: "Client credentials parse failed", cardStatus: "error" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      m = gatewayNormMsg(fJ);
      if (m === "upstream.health.probe_failed") return { subtitle: "Upstream health probe failed", cardStatus: "warn" };
      if (m === "upstream.models.fetch_failed") return { subtitle: "Upstream models fetch failed", cardStatus: "warn" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      m = gatewayNormMsg(fJ);
      if (m === "gateway.config.reloaded") return { subtitle: "Gateway config reloaded", cardStatus: "active" };
      if (m === "gateway.auth.reloaded") return { subtitle: "Client credentials reloaded", cardStatus: "active" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      m = gatewayNormMsg(fJ);
      if (m === "gateway.startup.listening") return { subtitle: "Gateway listening", cardStatus: "active" };
    }
    return null;
  }

  var tier = tierPick();
  if (tier) {
    out.subtitle = tier.subtitle;
    out.cardStatus = tier.cardStatus;
  }

  return out;
}

globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
globalThis.ClaudiaLogs.Derive = globalThis.ClaudiaLogs.Derive || {};
globalThis.ClaudiaLogs.Derive.gatewayCardModel = gatewayCardModel;
globalThis.ClaudiaLogs.Derive.gatewayPanelHideRow = gatewayPanelHideRow;
