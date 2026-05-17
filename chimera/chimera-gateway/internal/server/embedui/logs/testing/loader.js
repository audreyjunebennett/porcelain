/**
 * Minimal loader to evaluate component modules in-order when no bundler is present.
 *
 * In-browser, we can include this first, then include individual modules.
 * In tests (goja), we call eval() on each file; this exists mostly as documentation.
 */
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};

