/**
 * Service card avatar helpers; full buildServiceCard is in feedLogService.js.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountServiceCard = function (ctx) {
  var strHash = ctx.strHash;
  var getFlat = ctx.getFlat;

  function serviceDisplayLabel(key) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Contracts &&
      typeof ChimeraSettings.Contracts.serviceDisplayLabel === "function"
    ) {
      return ChimeraSettings.Contracts.serviceDisplayLabel(key);
    }
    var k = String(key || "").trim().toLowerCase();
    if (!k) return "";
    if (k.indexOf("chimera-") === 0) return k.slice("chimera-".length);
    return k;
  }

  function serviceBadge(key, cls) {
    return { cls: cls, key: key, lab: serviceDisplayLabel(key) };
  }

  function inferServiceBadge(ev) {
    var src = (ev.source || (ev.parsed && ev.parsed.app) || "").toLowerCase();
    var f = typeof getFlat === "function" ? getFlat(ev.parsed) : (ev.parsed && ev.parsed.rawFlat) || {};
    var sh = (ev.parsed && ev.parsed.shape) || "";
    if (src === "chimera-vectorstore" || sh === "service.chimera-vectorstore" || f.service === "chimera-vectorstore") {
      return serviceBadge("chimera-vectorstore", "sum-svc-vectorstore");
    }
    if (src === "chimera-indexer" || sh.indexOf("chimera-indexer") === 0 || f.service === "chimera-indexer") {
      return serviceBadge("chimera-indexer", "sum-svc-indexer");
    }
    if (src === "chimera-broker" || sh.indexOf("chimera-broker") >= 0 || sh.indexOf("chat.chimera-broker") === 0) {
      return serviceBadge("chimera-broker", "sum-svc-broker");
    }
    if (sh === "http.access" || (f.method && f.path)) return { cls: "sum-svc-web", key: "web", lab: "web" };
    if (sh === "chat.routing") return { cls: "sum-svc-gateway", key: "routing", lab: "routing" };
    if (
      src === "chimera-gateway" ||
      src === "gateway" ||
      f.service === "chimera-gateway" ||
      f.service === "gateway"
    ) {
      return serviceBadge("chimera-gateway", "sum-svc-gateway");
    }
    return serviceBadge("chimera-gateway", "sum-svc-gateway");
  }

  function avatarInitials(label) {
    var s = String(label || "?").trim();
    if (!s) return "??";
    var parts = s.split(/[^a-zA-Z0-9]+/).filter(Boolean);
    if (parts.length >= 2) {
      return (String(parts[0][0] || "") + String(parts[1][0] || ""))
        .toUpperCase()
        .slice(0, 2);
    }
    var t = s.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
    return t.slice(0, 2) || "??";
  }

  function avatarHueClass(seed) {
    var h = typeof strHash === "function" ? strHash(String(seed || "")) : "0";
    var n = parseInt(String(h).replace(/[^\d]/g, "0"), 10) || 0;
    var classes = ["sum-av-a", "sum-av-b", "sum-av-c", "sum-av-d", "sum-av-e", "sum-av-f"];
    return classes[n % classes.length];
  }

  function serviceAvatarClass(name) {
    switch (name) {
      case "chimera-gateway":
        return "sum-av-svc-chimera-gateway";
      case "chimera-broker":
        return "sum-av-svc-chimera-broker";
      case "chimera-vectorstore":
        return "sum-av-svc-chimera-vectorstore";
      case "chimera-indexer":
        return "sum-av-svc-chimera-indexer";
      default:
        return avatarHueClass(name);
    }
  }

  function serviceAvatarInitials(name) {
    switch (name) {
      case "chimera-broker":
        return "CB";
      case "chimera-gateway":
        return "CW";
      case "chimera-vectorstore":
        return "CV";
      case "chimera-indexer":
        return "CI";
      default:
        return avatarInitials(name);
    }
  }

  var SH = globalThis.ChimeraShared && globalThis.ChimeraShared.ServiceHealth;

  function serviceHealthSegHtml(title, tone, extraClass) {
    if (SH && typeof SH.healthSegSpan === "function") {
      return SH.healthSegSpan(ctx.escapeHtml, title, tone, extraClass);
    }
    return "";
  }

  function serviceMetricsWrapHtml(innerHtml) {
    if (SH && typeof SH.metricsWrapHtml === "function") return SH.metricsWrapHtml(innerHtml);
    return '<span class="sum-metrics">' + (innerHtml || "") + "</span>";
  }

  ctx.serviceDisplayLabel = serviceDisplayLabel;
  ctx.serviceBadge = serviceBadge;
  ctx.inferServiceBadge = inferServiceBadge;
  ctx.avatarInitials = avatarInitials;
  ctx.avatarHueClass = avatarHueClass;
  ctx.serviceAvatarClass = serviceAvatarClass;
  ctx.serviceAvatarInitials = serviceAvatarInitials;
  ctx.serviceHealthSegHtml = serviceHealthSegHtml;
  ctx.serviceMetricsWrapHtml = serviceMetricsWrapHtml;
};

