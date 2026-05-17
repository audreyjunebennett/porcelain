/**
 * Raw logs textarea view rendering + clipboard.
 *
 * Exports:
 * - ChimeraLogs.RawLogs.rebuild(ctx, opts)
 * - ChimeraLogs.RawLogs.appendRawLine(ctx, ent, follow)
 * - ChimeraLogs.RawLogs.copyToClipboard(ctx)
 */

function formatRawLogLine(ent) {
  var t = ent && ent.text != null ? String(ent.text) : "";
  return t;
}

function rebuild(ctx, opts) {
  opts = opts || {};
  var ta = document.getElementById("raw-logs-textarea");
  if (!ta) return;
  var lines = [];
  var unfiltered = [];
  var matchers = typeof ctx.entryMatchesFilters === "function" ? ctx.entryMatchesFilters : null;
  for (var i = 0; i < ctx.entryCache.length; i++) {
    var ent = ctx.entryCache[i];
    if (!ent) continue;
    var line = formatRawLogLine(ent);
    unfiltered.push(line);
    var inc = true;
    if (matchers) {
      try {
        inc = matchers(ent.parsed);
      } catch (_e) {
        inc = true;
      }
    }
    if (inc) lines.push(line);
  }
  /* View switches + persisted filters can mismatch option lists; showing nothing is worse than loosening temporarily. */
  if (!lines.length && unfiltered.length) lines = unfiltered;
  ta.value = lines.join("\n");
  if (opts.scrollBottom !== false) {
    window.requestAnimationFrame(function () {
      ta.scrollTop = ta.scrollHeight;
    });
  }
}

function appendRawLine(ctx, ent, follow) {
  var ta = document.getElementById("raw-logs-textarea");
  if (!ta) return;
  var line = formatRawLogLine(ent);
  if (ta.value) ta.value += "\n";
  ta.value += line;
  if (follow) {
    window.requestAnimationFrame(function () {
      ta.scrollTop = ta.scrollHeight;
    });
  }
}

function copyToClipboard(ctx) {
  var ta = document.getElementById("raw-logs-textarea");
  var statusElCopy = document.getElementById("raw-logs-copy-status");
  if (!ta) return;
  var text = ta.value;
  function setStatus(msg) {
    if (statusElCopy) statusElCopy.textContent = msg || "";
  }
  function flash(ok) {
    setStatus(ok ? "Copied" : "Copy failed");
    window.setTimeout(function () {
      setStatus("");
    }, 2500);
  }
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(function () {
      flash(true);
    }).catch(function () {
      try {
        ta.focus();
        ta.select();
        document.execCommand("copy");
        flash(true);
      } catch (e) {
        flash(false);
      }
    });
    return;
  }
  try {
    ta.focus();
    ta.select();
    document.execCommand("copy");
    flash(true);
  } catch (e) {
    flash(false);
  }
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.RawLogs = { rebuild: rebuild, appendRawLine: appendRawLine, copyToClipboard: copyToClipboard };

