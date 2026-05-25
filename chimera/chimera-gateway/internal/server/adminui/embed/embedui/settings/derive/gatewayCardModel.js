/**
 * Gateway service card: subtitle priority, KV row, counters, and full-log row filter.
 *
 * Exports:
 * - ChimeraSettings.Derive.gatewayCardModel(arr, getFlat)
 * - ChimeraSettings.Derive.gatewayPanelHideRow(ent, getFlat)
 *
 * Gateway ingest/RAG/chat counters use metrics_counter tags from operator_copy.js (Phase 6).
 */

function gatewayNormMsg(flat) {
  if (!flat) return "";
  var m = flat.msg != null ? flat.msg : flat.message;
  return String(m || "")
    .trim()
    .toLowerCase();
}

function gatewayCanonicalSlug(flat) {
  var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
  if (oc && typeof oc.resolveFlat === "function") {
    var slug = oc.resolveFlat(flat);
    if (slug) return slug;
  }
  return gatewayNormMsg(flat);
}

function gatewayRegistryMetricsCounter(flat) {
  var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
  if (oc && typeof oc.metricsCounterForFlat === "function") {
    return oc.metricsCounterForFlat(flat) || "";
  }
  return "";
}

function gatewayBumpHttpCounters(counters, flat, parsedShape) {
  var msg = gatewayNormMsg(flat);
  var sh = parsedShape || "";
  var httpShape =
    sh === "http.access" ||
    msg === "gateway.http.access" ||
    msg === "http response" ||
    (flat.method && flat.path != null && flat.statusCode !== undefined && flat.statusCode !== null);
  if (!httpShape) return;
  var sc = Number(flat.statusCode);
  if (isNaN(sc)) return;
  if (sc >= 200 && sc < 300) counters.http2xx++;
  else counters.httpNot2xx++;
  if (sc === 429) counters.http429++;
}

function gatewayBumpRegistryCounters(counters, flat) {
  var mc = gatewayRegistryMetricsCounter(flat);
  if (mc && Object.prototype.hasOwnProperty.call(counters, mc)) counters[mc]++;
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
    case "/healthz":
    case "/readyz":
    case "/status":
    case "/api/ui/logs":
    case "/api/ui/logs/stream":
    case "/ui/settings":
    case "/api/ui/metrics":
    case "/api/ui/tokens":
    case "/api/ui/state":
    case "/api/ui/chimera-broker/providers":
    case "/api/ui/providers/catalog":
    case "/api/ui/indexer/config":
    case "/api/ui/indexer/workspaces":
    case "/v1/indexer/storage/stats":
      return true;
    default:
      return false;
  }
}

function gatewayCardModel(arr, getFlat) {
  arr = Array.isArray(arr) ? arr : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var S = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy && ChimeraSettings.OperatorCopy.Slug;

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
  var slug;

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.GatewayConfigReloaded) || slug === "gateway.config.reloaded") lastConfigReloadOk = i;
    if (
      slug === (S && S.GatewayConfigReloadFailed) ||
      slug === (S && S.GatewayConfigMissing) ||
      slug === "gateway.config.reload_failed" ||
      slug === "gateway.config.missing"
    )
      lastConfigReloadErr = i;
  }

  var listeningAddr = "";
  var brokerUrl = "";
  var brokerData = "";
  var qSup = null;
  var idxSup = null;
  var startupConfigPath = "";

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.GatewayStartupListening) || slug === "gateway.startup.listening") {
      if (f.addr != null && String(f.addr).trim() !== "") listeningAddr = String(f.addr);
      if (f.broker != null && String(f.broker).trim() !== "") brokerUrl = String(f.broker);
      if (f.chimera_broker_data != null && String(f.chimera_broker_data).trim() !== "") brokerData = String(f.chimera_broker_data);
      if (f.vectorstore_supervised !== undefined && f.vectorstore_supervised !== null) qSup = !!f.vectorstore_supervised;
      if (f.indexer_supervised !== undefined && f.indexer_supervised !== null) idxSup = !!f.indexer_supervised;
      if (f.config != null && String(f.config).trim() !== "") startupConfigPath = String(f.config);
    }
    if (slug === (S && S.GatewayStartupConfigResolved) || slug === "gateway.startup.config_resolved") {
      if (f.filePath != null && String(f.filePath).trim() !== "") startupConfigPath = String(f.filePath);
    }
  }

  var configReloadPath = "";
  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.GatewayConfigReloaded) || slug === "gateway.config.reloaded")
      if (f.path != null && String(f.path).trim() !== "") configReloadPath = String(f.path);
  }

  var configPathFull = configReloadPath || startupConfigPath;
  out.kv.config = configPathFull ? gatewayBasenamePath(configPathFull) : "—";
  out.kv.configReloadStale = lastConfigReloadErr > lastConfigReloadOk && lastConfigReloadErr >= 0;
  if (out.kv.configReloadStale && out.kv.config !== "—") out.kv.config += " ⚠";

  var lastAuthReload = -1;
  var lastCredFail = -1;
  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.GatewayAuthReloaded) || slug === "gateway.auth.reloaded") {
      lastAuthReload = i;
      if (f.count != null && !isNaN(Number(f.count))) out.kv.apiKeys = String(Math.round(Number(f.count)));
      else if (f.count != null) out.kv.apiKeys = String(f.count);
    }
    if (
      slug === (S && S.GatewayAuthFileMissing) ||
      slug === (S && S.GatewayAuthReadFailed) ||
      slug === (S && S.GatewayAuthParseFailed) ||
      slug === "gateway.auth.file_missing" ||
      slug === "gateway.auth.read_failed" ||
      slug === "gateway.auth.parse_failed"
    ) {
      if (i >= lastCredFail) lastCredFail = i;
    }
  }
  if (out.kv.apiKeys === "—") out.kv.apiKeys = "—";
  out.kv.apiKeysTint = lastCredFail > lastAuthReload && lastCredFail >= 0 ? "error" : "none";

  out.kv.listening = listeningAddr || "—";
  out.kv.broker = brokerUrl || "—";

  for (i = arr.length - 1; i >= 0; i--) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.RoutingPolicyReloaded) || slug === "routing.policy.reloaded") {
      if (f.rules != null) {
        out.kv.routingRules = String(f.rules);
        break;
      }
    }
  }

  var supParts = [];
  if (qSup === true) supParts.push("chimera-vectorstore");
  else if (qSup === false) supParts.push("no chimera-vectorstore");
  if (brokerData) supParts.push("chimera-broker");
  if (idxSup === true) supParts.push("chimera-indexer");
  else if (idxSup === false) supParts.push("no chimera-indexer");
  out.kv.supervised = supParts.length ? supParts.join(" · ") : "—";

  for (i = 0; i < arr.length; i++) {
    f = getFlat(arr[i].parsed);
    var psh = arr[i].parsed.shape || "";
    gatewayBumpHttpCounters(out.counters, f, psh);
    gatewayBumpRegistryCounters(out.counters, f);
    // Legacy fallbacks when registry lacks metrics_counter.
    slug = gatewayCanonicalSlug(f);
    if (!gatewayRegistryMetricsCounter(f)) {
      if (slug === "ingest.complete") out.counters.ingestOk++;
      if (slug === "ingest.failed" || slug === "ingest.chunked.error" || slug === "scope.chunked.error")
        out.counters.ingestFail++;
      if (slug === "chat.request") out.counters.chatReq++;
      if (slug === "chat.chimera-broker.response") out.counters.chatResp++;
      if (slug === "chat.chimera-broker.error") out.counters.chatErr++;
      if (slug === "rag.query") out.counters.ragQuery++;
      if (slug === "rag.hit") out.counters.ragHit++;
    }
  }

  for (i = arr.length - 1; i >= 0; i--) {
    f = getFlat(arr[i].parsed);
    slug = gatewayCanonicalSlug(f);
    if (slug === (S && S.RagRetrieveError) || slug === "rag.retrieve.error") {
      if (f.err != null && String(f.err).trim() !== "") {
        var er = String(f.err).replace(/\s+/g, " ");
        out.counters.ragRetrieveErr = er.length > 140 ? er.slice(0, 138) + "…" : er;
        break;
      }
    }
  }

  function tierPick() {
    var j;
    var m;
    var fJ;
    var sl;
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      sl = gatewayCanonicalSlug(fJ);
      if (sl === (S && S.GatewayConfigReloadFailed) || sl === "gateway.config.reload_failed")
        return { subtitle: "Gateway config reload failed", cardStatus: "error" };
      if (sl === (S && S.GatewayConfigMissing) || sl === "gateway.config.missing")
        return { subtitle: "Gateway config missing", cardStatus: "error" };
      if (sl === (S && S.GatewayAuthParseFailed) || sl === "gateway.auth.parse_failed")
        return { subtitle: "Client credentials parse failed", cardStatus: "error" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      sl = gatewayCanonicalSlug(fJ);
      if (sl === (S && S.UpstreamHealthProbeFailed) || sl === "upstream.health.probe_failed")
        return { subtitle: "Upstream health probe failed", cardStatus: "warn" };
      if (sl === (S && S.UpstreamModelsFetchFailed) || sl === "upstream.models.fetch_failed")
        return { subtitle: "Upstream models fetch failed", cardStatus: "warn" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      sl = gatewayCanonicalSlug(fJ);
      if (sl === (S && S.GatewayConfigReloaded) || sl === "gateway.config.reloaded")
        return { subtitle: "Gateway config reloaded", cardStatus: "active" };
      if (sl === (S && S.GatewayAuthReloaded) || sl === "gateway.auth.reloaded")
        return { subtitle: "Client credentials reloaded", cardStatus: "active" };
    }
    for (j = arr.length - 1; j >= 0; j--) {
      fJ = getFlat(arr[j].parsed);
      sl = gatewayCanonicalSlug(fJ);
      if (sl === (S && S.GatewayStartupListening) || sl === "gateway.startup.listening")
        return { subtitle: "Gateway listening", cardStatus: "active" };
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

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
globalThis.ChimeraSettings.Derive.gatewayCardModel = gatewayCardModel;
globalThis.ChimeraSettings.Derive.gatewayPanelHideRow = gatewayPanelHideRow;
