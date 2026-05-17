/**
 * Per-conversation BiFrost / upstream relay rollup (pure).
 *
 * Exports:
 * - ChimeraLogs.Derive.conversationBifrostTimelineFlat(flat)
 * - ChimeraLogs.Derive.conversationBifrostRelayCount(events, getFlat)
 *
 * Excludes startup-only lines (`chat.chimera-broker.available_models`) from per-conversation counts.
 */

function conversationBifrostTimelineFlat(flat) {
  if (!flat || typeof flat !== "object") return false;
  var raw = flat.msg != null ? flat.msg : flat.message != null ? flat.message : "";
  var msg = String(raw).trim();
  if (!msg) return false;
  var ml = msg.toLowerCase();
  if (msg === "chat.chimera-broker.available_models") return false;

  if (msg === "chat.chimera-broker.request") return true;
  if (msg === "upstream chat response" || msg === "chat.chimera-broker.response") return true;
  if (msg === "chat.chimera-broker.error") return true;
  if (msg.indexOf("chimera-broker.error") >= 0) return true;

  if (msg === "chat.routing.fallback") return true;
  if (msg === "chat.routing.attempt") return true;
  if (msg === "chat.routing.resolved") return true;
  if (msg === "chat.provider_limits.blocked") return true;

  if (ml.indexOf("virtual model fallback attempt") >= 0) return true;
  if (ml.indexOf("virtual model routing resolved") >= 0) return true;
  if (ml.indexOf("chat blocked by provider limits") >= 0) return true;
  if (ml.indexOf("skipping upstream model (provider limits)") >= 0) return true;

  return false;
}

function conversationBifrostRelayCount(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var n = 0;
  for (var i = 0; i < events.length; i++) {
    var ev = events[i];
    var p = ev && ev.parsed;
    var f = getFlat(p);
    if (conversationBifrostTimelineFlat(f)) n++;
  }
  return n;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.conversationBifrostTimelineFlat = conversationBifrostTimelineFlat;
globalThis.ChimeraLogs.Derive.conversationBifrostRelayCount = conversationBifrostRelayCount;
