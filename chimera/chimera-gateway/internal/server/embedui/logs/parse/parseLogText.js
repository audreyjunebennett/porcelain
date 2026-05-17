/**
 * Parsing pipeline: turns raw log line text into a normalized parsed object.
 * This module mirrors the legacy logic previously embedded in `embedui/logs.js`.
 *
 * Exports:
 * - ChimeraLogs.parseLogText(source, text, entryTS)
 */

function parseKeyValueLine(line) {
  var out = {};
  var i = 0;
  var len = line.length;
  while (i < len) {
    while (i < len && line[i] === " ") i++;
    if (i >= len) break;
    var eq = line.indexOf("=", i);
    if (eq < 0) break;
    var key = line.slice(i, eq);
    if (!key) break;
    i = eq + 1;
    var val = "";
    if (i < len && line[i] === "\"") {
      i++;
      while (i < len) {
        if (line[i] === "\\") {
          i++;
          if (i < len) val += line[i++];
          continue;
        }
        if (line[i] === "\"") {
          i++;
          break;
        }
        val += line[i++];
      }
    } else {
      while (i < len && line[i] !== " ") val += line[i++];
    }
    out[key] = val;
  }
  return out;
}

function tryParseJSONObject(text) {
  var t = text.trim();
  if (!t || t[0] !== "{") return null;
  try {
    return JSON.parse(t);
  } catch (x) {
    return null;
  }
}

function flattenObject(obj, prefix, acc) {
  acc = acc || {};
  if (obj === null || obj === undefined) return acc;
  if (typeof obj !== "object" || Array.isArray(obj)) {
    acc[prefix || "_"] = obj;
    return acc;
  }
  var keys = Object.keys(obj);
  if (keys.length === 0) {
    acc[prefix || "empty"] = {};
    return acc;
  }
  for (var k = 0; k < keys.length; k++) {
    var key = keys[k];
    var path = prefix ? prefix + "." + key : key;
    var v = obj[key];
    if (v !== null && typeof v === "object" && !Array.isArray(v)) {
      flattenObject(v, path, acc);
    } else {
      acc[path] = v;
    }
  }
  return acc;
}

function formatExtraValue(v) {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  try {
    return JSON.stringify(v);
  } catch (x) {
    return String(v);
  }
}

var reservedForColumns = { time: true, ts: true, timestamp: true, "@timestamp": true, level: true };

function canonicalLevel(raw) {
  if (raw === null || raw === undefined) return null;
  var s0 = String(raw).trim();
  if (s0 === "") return null;
  var n = parseInt(s0, 10);
  /* Go slog JSON uses small integers (-8 TRACE, -4 DEBUG … 8 ERROR). Skip larger ints (e.g. Zap). */
  if (!isNaN(n) && String(n) === s0 && n >= -8 && n <= 12) {
    if (n <= -5) return "TRACE";
    if (n <= -1) return "DEBUG";
    if (n < 4) return "INFO";
    if (n < 8) return "WARN";
    return "ERROR";
  }
  var s = s0.toUpperCase();
  if (s === "WARNING") return "WARN";
  if (s === "ERR") return "ERROR";
  if (s === "DEBUG" || s === "INFO" || s === "WARN" || s === "ERROR" || s === "TRACE" || s === "FATAL") {
    if (s === "FATAL") return "ERROR";
    return s;
  }
  return s0;
}

function displayLevelLabel(can) {
  if (!can) return "—";
  return can;
}

function sortExtraKeys(keys) {
  var pri = ["msg", "message", "status", "err", "error", "code", "path", "method"];
  keys.sort(function (a, b) {
    var la = a.toLowerCase();
    var lb = b.toLowerCase();
    var ia = -1, ib = -1;
    for (var p = 0; p < pri.length; p++) {
      if (ia < 0 && la === pri[p]) ia = p;
      if (ib < 0 && lb === pri[p]) ib = p;
    }
    if (ia >= 0 && ib >= 0) return ia - ib;
    if (ia >= 0) return -1;
    if (ib >= 0) return 1;
    return a.localeCompare(b);
  });
}

/**
 * @param {string} source
 * @param {string} text
 * @param {any} entryTS
 * @returns {any}
 */
function parseLogText(source, text, entryTS) {
  var flat = null;
  var fromJSON = tryParseJSONObject(text);
  if (fromJSON && typeof fromJSON === "object" && !Array.isArray(fromJSON)) {
    flat = flattenObject(fromJSON, "", {});
  } else {
    var kv = parseKeyValueLine(text);
    if (Object.keys(kv).length && (kv.time !== undefined || kv.level !== undefined)) {
      flat = kv;
    }
  }

  var instant = null;
  if (entryTS !== null && entryTS !== undefined && entryTS !== "") {
    var d0 = entryTS instanceof Date ? entryTS : new Date(entryTS);
    if (!isNaN(d0.getTime())) instant = d0;
  }
  var levelCanon = null;
  var extras = [];

  if (flat) {
    var tRaw = "";
    if (flat.time !== undefined && flat.time !== null && flat.time !== "") tRaw = flat.time;
    else if (flat.ts !== undefined && flat.ts !== null && flat.ts !== "") tRaw = flat.ts;
    else if (flat.timestamp !== undefined && flat.timestamp !== null && flat.timestamp !== "") tRaw = flat.timestamp;
    else if (flat["@timestamp"] !== undefined && flat["@timestamp"] !== null && flat["@timestamp"] !== "")
      tRaw = flat["@timestamp"];
    if (tRaw !== "" && tRaw !== undefined && tRaw !== null) {
      var tryDt = new Date(String(tRaw));
      if (!isNaN(tryDt.getTime())) instant = tryDt;
    }
    if (flat.level !== undefined && flat.level !== null && flat.level !== "") {
      levelCanon = canonicalLevel(flat.level);
    }
    var keys = Object.keys(flat);
    sortExtraKeys(keys);
    for (var i = 0; i < keys.length; i++) {
      var k = keys[i];
      var lk = k.toLowerCase();
      if (reservedForColumns[lk]) continue;
      if (lk === "time" || lk === "timestamp" || lk === "@timestamp") continue;
      extras.push({ k: k, v: formatExtraValue(flat[k]) });
    }
  } else {
    extras.push({ k: "text", v: text });
  }

  var buildDateTimeCells = globalThis.ChimeraLogs && globalThis.ChimeraLogs.buildDateTimeCells
    ? globalThis.ChimeraLogs.buildDateTimeCells
    : null;
  var inferShape = globalThis.ChimeraLogs && globalThis.ChimeraLogs.inferShape
    ? globalThis.ChimeraLogs.inferShape
    : null;

  // These are still in logs.js in this phase; require them to exist.
  var dtCells = buildDateTimeCells ? buildDateTimeCells(instant, entryTS) : { utc: "", local: "" };

  var rawFlat = null;
  if (flat) {
    rawFlat = {};
    for (var rk in flat) {
      if (Object.prototype.hasOwnProperty.call(flat, rk)) rawFlat[rk] = flat[rk];
    }
  }
  var shape = inferShape ? inferShape(rawFlat, source, text || "") : "generic";
  return {
    app: source || "—",
    dtUtcHtml: dtCells.utc,
    dtLocalHtml: dtCells.local,
    levelCanon: levelCanon,
    levelLabel: displayLevelLabel(levelCanon),
    extras: extras,
    rawFlat: rawFlat,
    shape: shape
  };
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.parseLogText = parseLogText;

