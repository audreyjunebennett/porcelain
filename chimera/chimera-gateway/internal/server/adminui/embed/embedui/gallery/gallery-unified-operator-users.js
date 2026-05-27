/**
 * Unified operator gallery — Users & gateway tokens: workspace-style Add → draft card
 * (sum-card--workspace-draft) with Save/Cancel, auto token + copy; saved cards show KV + sum-evlog.
 */
(function () {
  "use strict";

  var COPY_SVG =
    '<svg class="sum-evlog__copy-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>';

  var userSeq = 100;

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/"/g, "&quot;");
  }

  function randHex(n) {
    var a = [];
    for (var i = 0; i < n; i++) {
      a.push(((Math.random() * 16) | 0).toString(16));
    }
    return a.join("");
  }

  function genUserId() {
    return "usr_" + randHex(8);
  }

  function genGatewayToken() {
    return "gw_live_" + randHex(16);
  }

  function toastNear(btn, msg, ok) {
    var t = document.createElement("span");
    t.setAttribute("role", "status");
    t.style.cssText =
      "margin-left:0.35rem;font-size:0.72rem;color:" +
      (ok ? "var(--embed-semantic-success-fg)" : "var(--embed-semantic-error-fg)") +
      ";";
    t.textContent = msg;
    btn.parentNode.appendChild(t);
    window.setTimeout(function () {
      try {
        t.remove();
      } catch (e0) {}
    }, 2200);
  }

  function wireTokenCopy(btn, codeEl) {
    btn.addEventListener("click", function () {
      var v = codeEl && codeEl.textContent ? String(codeEl.textContent).trim() : "";
      if (!v) return;
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(v).then(
          function () {
            toastNear(btn, "Copied", true);
          },
          function () {
            toastNear(btn, "Clipboard blocked", false);
          }
        );
      } else {
        toastNear(btn, "No clipboard API", false);
      }
    });
  }

  function evlogTableRows(uid, name, prefix) {
    var t0 = "2026-05-11T09:15:01.100Z";
    var t1 = "2026-05-11T09:16:22.040Z";
    var t2 = "2026-05-11T09:18:44.881Z";
    return (
      '<tr class="sum-evlog__row" data-evlog-id="' +
      esc(prefix + "-1") +
      '" data-evlog-level="INFO" data-evlog-http="200">' +
      '<td class="sum-evlog__cell--time"><time datetime="' +
      t0 +
      '"></time></td>' +
      '<td class="sum-evlog__cell--msg">Auth OK · principal <code class="sum-mono-id">' +
      esc(uid) +
      "</code> · label <strong>" +
      esc(name) +
      "</strong></td>" +
      '<td class="sum-evlog__cell--source"><span class="sum-svc-badge sum-svc-gateway">gateway</span></td>' +
      '<td class="sum-evlog__cell--status"><div class="sum-evlog-status"><span class="pill-2xx">200</span></div></td></tr>' +
      '<tr class="sum-evlog__row" data-evlog-id="' +
      esc(prefix + "-2") +
      '" data-evlog-level="INFO">' +
      '<td class="sum-evlog__cell--time"><time datetime="' +
      t1 +
      '"></time></td>' +
      '<td class="sum-evlog__cell--msg">Workspace sync scheduled for tenant slice owned by <code class="sum-mono-id">' +
      esc(uid) +
      "</code>.</td>" +
      '<td class="sum-evlog__cell--source"><span class="sum-svc-badge sum-svc-indexer">indexer</span></td>' +
      '<td class="sum-evlog__cell--status"><div class="sum-evlog-status"><span class="sum-evlog-status__empty" aria-hidden="true"></span></div></td></tr>' +
      '<tr class="sum-evlog__row" data-evlog-id="' +
      esc(prefix + "-3") +
      '" data-evlog-level="WARN">' +
      '<td class="sum-evlog__cell--time"><time datetime="' +
      t2 +
      '"></time></td>' +
      '<td class="sum-evlog__cell--msg">Rate advisory for <code class="sum-mono-id">' +
      esc(uid) +
      "</code> — soft quota on embeddings.</td>" +
      '<td class="sum-evlog__cell--source"><span class="sum-svc-badge sum-svc-gateway">gateway</span></td>' +
      '<td class="sum-evlog__cell--status"><div class="sum-evlog-status"><span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN">WARN</span></div></td></tr>'
    );
  }

  function buildEvlogHtml(uid, name, prefix) {
    return (
      '<div class="sum-evlog sum-evlog--in-card" style="max-height:12rem;margin:0.35rem 0 0;padding:0.5rem 0.55rem 0.35rem" data-gallery-evlog-root data-sum-evlog-root data-sum-evlog-cols="4" data-sum-evlog-source>' +
      '<div class="sum-section-label">User activity</div>' +
      '<div class="sum-evlog__toolbar">' +
      '<input class="sum-evlog__search" type="search" placeholder="Search message or time…" aria-label="Search log entries" autocomplete="off" />' +
      '<label class="sum-evlog__lvl-label" style="margin-left:auto">' +
      '<span class="sum-evlog__level-filters-label" style="margin-right:0.35rem">Status</span>' +
      '<select class="sum-evlog__filter-select" data-evlog-filter-status aria-label="Filter by severity">' +
      '<option value="all">All</option><option value="warnings">⚠ Warnings</option><option value="errors">✖ Errors</option>' +
      "</select></label>" +
      '<button type="button" class="sum-evlog__copy-btn" title="Copy as TSV" aria-label="Copy as TSV">' +
      COPY_SVG +
      '<span class="sr-only">Copy</span></button></div>' +
      '<div class="sum-metrics-table-wrap sum-evlog__table-scroll">' +
      '<table class="sum-metrics-table sum-evlog__table">' +
      "<colgroup><col class=\"sum-evlog__col-time\" /><col class=\"sum-evlog__col-msg\" /><col class=\"sum-evlog__col-source\" /><col class=\"sum-evlog__col-status\" /></colgroup>" +
      "<thead><tr><th class=\"sum-evlog__cell--time\" scope=\"col\">Time</th><th class=\"sum-evlog__th-msg\" scope=\"col\">Message</th><th class=\"sum-evlog__th-source\" scope=\"col\">Source</th><th class=\"sum-evlog__th-status\" scope=\"col\">" +
      '<div class="sum-evlog__th-status-head" role="group" aria-label="Status counts">' +
      '<span class="sum-evlog__th-status-label">Status</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog-metric-num" data-sum-evlog-metric-warn>—</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog__metric-icon" aria-hidden="true">⚠</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog-metric-num" data-sum-evlog-metric-fail>—</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog__metric-icon" aria-hidden="true">✖</span>' +
      "</div></th></tr></thead>" +
      '<tbody data-gallery-evlog-tbody>' +
      evlogTableRows(uid, name, prefix) +
      "</tbody></table></div>" +
      '<div class="sum-evlog__footer-row">' +
      '<div class="sum-evlog__footer-left"><p class="sum-evlog__footer" data-gallery-evlog-oldest></p></div>' +
      '<p class="sum-evlog__toast sum-gallery-evlog__toast-align" data-gallery-evlog-toast role="status" aria-live="polite"></p>' +
      "</div></div>"
    );
  }

  function buildSavedCard(opts) {
    var uid = opts.userId;
    var name = opts.displayName || "User";
    var email = (opts.email && String(opts.email).trim()) || "";
    var conv = opts.conversations != null ? opts.conversations : Math.floor(Math.random() * 80) + 2;
    var ws = opts.workspaces != null ? opts.workspaces : Math.floor(Math.random() * 6);
    var tokCount = opts.gatewayTokens != null ? opts.gatewayTokens : 1;
    var prefix = "u" + userSeq++;
    var subParts = [];
    if (email && email !== String(name).trim()) subParts.push(esc(email));
    subParts.push(esc(uid));
    subParts.push(String(tokCount) + " gateway token" + (tokCount === 1 ? "" : "s"));
    var sub = subParts.join(" · ");
    var av = name.slice(0, 2);
    if (av.length < 2 && email) av = (email.split("@")[0] || email).slice(0, 2);
    if (av.length < 2) av = uid.replace(/^usr_/, "").slice(0, 2) || "??";
    return (
      '<article class="sum-card sg-op-user-saved" data-sg-op-user-id="' +
      esc(uid) +
      '">' +
      '<header class="sum-card__workspace-draft-hdr">' +
      '<span class="sum-avatar sum-av-b" title="Saved user">' +
      esc(av.toUpperCase()) +
      "</span>" +
      '<span class="sum-main sum-main--workspace-draft"><span class="sum-title">' +
      esc(name) +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      sub +
      "</span></span>" +
      '<span class="ws-draft-actions">' +
      '<button type="button" class="sg-op-btn sg-op-btn--small sg-op-btn--danger" disabled title="Gallery demo">Revoke</button>' +
      "</span></header>" +
      '<div class="sum-body">' +
      '<dl class="indexer-run-kv indexer-run-kv--gateway-summary">' +
      "<dt>User id</dt><dd><code class=\"sum-mono-id\">" +
      esc(uid) +
      "</code></dd>" +
      "<dt>Conversations</dt><dd>" +
      String(conv) +
      "</dd>" +
      "<dt>Workspaces</dt><dd>" +
      String(ws) +
      "</dd></dl>" +
      '<div class="sum-section-label">Scoped log — user</div>' +
      buildEvlogHtml(uid, name, prefix) +
      "</div></article>"
    );
  }

  function mountSavedFromDraft(article) {
    var nameInp = article.querySelector("[data-sg-op-user-name]");
    var emailInp = article.querySelector("[data-sg-op-user-email]");
    var nameRaw = (nameInp && nameInp.value.trim()) || "";
    var email = (emailInp && emailInp.value.trim()) || "";
    var name = nameRaw || email || "New user";
    var uid = genUserId();
    var wrap = document.getElementById("sg-op-user-saved");
    if (!wrap) return;
    var html = buildSavedCard({
      userId: uid,
      displayName: name,
      email: email,
      conversations: null,
      workspaces: null
    });
    var div = document.createElement("div");
    div.innerHTML = html.trim();
    var card = div.firstChild;
    wrap.insertBefore(card, wrap.firstChild);
    article.remove();
    var ev = card.querySelector("[data-gallery-evlog-root]");
    if (ev && globalThis.GalleryEventLogDemo && typeof globalThis.GalleryEventLogDemo.wireRoot === "function") {
      globalThis.GalleryEventLogDemo.wireRoot(ev);
    }
  }

  function createDraft() {
    var host = document.getElementById("sg-op-user-drafts");
    if (!host) return;
    var tok = genGatewayToken();
    var art = document.createElement("article");
    art.className = "sum-card sum-card--workspace-draft";
    art.setAttribute("data-sg-op-user-draft", "1");
    art.innerHTML =
      '<header class="sum-card__workspace-draft-hdr">' +
      '<span class="sum-avatar sum-av-a" title="New user">+</span>' +
      '<span class="sum-main sum-main--workspace-draft"><span class="sum-title">New user</span></span>' +
      '<span class="ws-draft-actions">' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-cancel">Cancel</button>' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-save">Save</button>' +
      "</span></header>" +
      '<div class="sum-body">' +
      '<div class="ws-draft-fields">' +
      '<div class="ws-draft-field"><label class="ws-draft-field-label">Display name</label>' +
      '<input class="ws-draft-input" type="text" data-sg-op-user-name placeholder="e.g. Nightly indexer bot" /></div>' +
      '<div class="ws-draft-field"><label class="ws-draft-field-label">Identifier / email</label>' +
      '<input class="ws-draft-input" type="text" data-sg-op-user-email placeholder="ops@company.example" /></div></div>' +
      '<div class="sum-section-label">Gateway token (auto-generated)</div>' +
      '<div class="sg-op-token-row">' +
      '<code class="sum-mono-id" data-sg-op-user-token>' +
      esc(tok) +
      "</code>" +
      '<button type="button" class="sg-op-token-copy-btn" data-sg-op-user-copy title="Copy token" aria-label="Copy token">' +
      COPY_SVG +
      "</button>" +
      '<span class="muted" style="font-size:0.72rem">Copy now — production would hide this after save.</span></div>' +
      '<p class="muted ws-draft-hint" style="margin-bottom:0">Gallery demo — wire to <code class="sum-mono-id">POST /api/ui/tokens</code> in a later phase.</p></div>';

    var cancel = art.querySelector(".ws-draft-btn-cancel");
    var save = art.querySelector(".ws-draft-btn-save");
    var copyBtn = art.querySelector("[data-sg-op-user-copy]");
    var codeEl = art.querySelector("[data-sg-op-user-token]");
    if (copyBtn && codeEl) wireTokenCopy(copyBtn, codeEl);
    if (cancel) {
      cancel.addEventListener("click", function () {
        art.remove();
      });
    }
    if (save) {
      save.addEventListener("click", function () {
        mountSavedFromDraft(art);
      });
    }
    host.appendChild(art);
    try {
      var n = art.querySelector("[data-sg-op-user-name]");
      if (n) n.focus();
    } catch (e1) {}
  }

  function init() {
    var add = document.getElementById("sg-op-user-add");
    if (add) add.addEventListener("click", createDraft);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
