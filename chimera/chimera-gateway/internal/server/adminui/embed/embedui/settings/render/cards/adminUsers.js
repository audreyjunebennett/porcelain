/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountAdminUsers = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var strHash = ctx.strHash;
  var formatInt = ctx.formatInt;
  var adminScopedEventsForPrincipal = ctx.adminScopedEventsForPrincipal;
  var adminScopedEvlogPanelFromEvents = ctx.adminScopedEvlogPanelFromEvents;
  var operatorSectionHeadHtml = ctx.operatorSectionHeadHtml;
  var operatorSectionAddBtn = ctx.operatorSectionAddBtn;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;

  function adminBuildUserCardHtml(principalId, tokensForUser, stats) {
    var label = ctx.tokenLabelByTenant[principalId] || (tokensForUser[0] && tokensForUser[0].label) || principalId;
    var initials =
      typeof ctx.avatarInitials === "function" ? ctx.avatarInitials(label) : String(label || "??").slice(0, 2);
    var convN = 0;
    var wsN = 0;
    if (stats) {
      for (var ck in stats.conv) if (Object.prototype.hasOwnProperty.call(stats.conv, ck)) convN++;
      for (var wk in stats.ws) if (Object.prototype.hasOwnProperty.call(stats.ws, wk)) wsN++;
    }
    var tokenRaw = "";
    if (tokensForUser[0] && tokensForUser[0].token != null && String(tokensForUser[0].token).trim() !== "") {
      tokenRaw = String(tokensForUser[0].token).trim();
    } else if (ctx.adminCreatedTokenByTenant[principalId]) {
      tokenRaw = String(ctx.adminCreatedTokenByTenant[principalId] || "").trim();
    }
    var createdTokenHint = tokenRaw ? ("****************************" + tokenRaw.slice(-4)) : "****************************";
    var createdTokenCopyBtn = tokenRaw
      ? '<button type="button" class="sg-op-token-copy-btn" data-admin-action="user-token-copy" data-token="' + escapeHtml(tokenRaw) + '" title="Copy API key" aria-label="Copy API key">' +
        '<svg class="sum-evlog__copy-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg></button>'
      : "";
    var scoped = adminScopedEventsForPrincipal(principalId, 18);
    var metricsHtml = "";
    if (typeof sgOpHealthPillHtml === "function") {
      metricsHtml =
        '<span class="sum-metrics">' +
        sgOpHealthPillHtml(formatInt(convN), "metric", { icon: "forum", title: "Conversations" }) +
        sgOpHealthPillHtml(formatInt(wsN), "metric", { icon: "database", title: "Workspaces" }) +
        "</span>";
    }
    return (
      '<article class="sum-card sg-op-user-saved sum-card--collapsible" id="admin-user-' +
      strHash("admin-user-" + principalId) +
      '" data-sg-op-user-id="' +
      escapeHtml(principalId) +
      '">' +
      '<header class="sum-card__hdr">' +
      '<span class="sum-avatar sum-av-b" title="User">' +
      escapeHtml(initials) +
      "</span>" +
      '<span class="sum-main"><span class="sum-title">' +
      escapeHtml(label) +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      escapeHtml(principalId) +
      "</span></span>" +
      metricsHtml +
      '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>' +
      '<span class="sum-chev" aria-hidden="true"></span></header>' +
      '<div class="sum-body">' +
      '<div class="sum-section-label">Gateway API key</div><div class="sg-op-token-row"><code class="sum-mono-id">' +
      escapeHtml(createdTokenHint) +
      "</code>" +
      createdTokenCopyBtn +
      "</div>" +
      adminScopedEvlogPanelFromEvents("Scoped log — user", "user-" + principalId, scoped, { showSourceColumn: true }) +
      "</div></article>"
    );
  }

  function buildAdminUserDraftCardHtml(draft) {
    var nm = draft && draft.name ? String(draft.name) : "";
    var em = draft && draft.email ? String(draft.email) : "";
    var msg = draft && draft.msg ? String(draft.msg) : "";
    return (
      '<article class="sum-card sum-card--workspace-draft" data-admin-user-draft="' + escapeHtml(draft.id) + '">' +
      '<header class="sum-card__workspace-draft-hdr">' +
      '<span class="sum-avatar sum-av-a">+</span>' +
      '<span class="sum-main sum-main--workspace-draft"><span class="sum-title">New user</span>' +
      '<span class="sum-sub sum-sub--clamp muted">Create a gateway token and save this principal.</span></span>' +
      '<span class="ws-draft-actions">' +
      '<button type="button" class="sg-op-btn sg-op-btn--ghost ws-draft-btn-cancel" data-admin-action="user-draft-cancel" data-draft-id="' +
      escapeHtml(draft.id) +
      '">Cancel</button>' +
      '<button type="button" class="sg-op-btn ws-draft-btn-save" data-admin-action="user-draft-save" data-draft-id="' +
      escapeHtml(draft.id) +
      '"' +
      (draft.saving ? " disabled" : "") +
      ">Save</button></span>" +
      "</header>" +
      '<div class="sum-body"><div class="ws-draft-fields">' +
      '<div class="ws-draft-field"><label class="ws-draft-field-label">Display name</label>' +
      '<input class="ws-draft-input" data-admin-user-field="name" data-draft-id="' + escapeHtml(draft.id) + '" type="text" value="' + escapeHtml(nm) + '" placeholder="e.g. Operations" /></div>' +
      '<div class="ws-draft-field"><label class="ws-draft-field-label">Identifier / email</label>' +
      '<input class="ws-draft-input" data-admin-user-field="email" data-draft-id="' + escapeHtml(draft.id) + '" type="text" value="' + escapeHtml(em) + '" placeholder="ops@example.com" /></div>' +
      "</div>" +
      (msg ? '<p class="muted ws-draft-hint">' + escapeHtml(msg) + "</p>" : "") +
      "</div></article>"
    );
  }

  function buildAdminUsersCardHtml() {
    var toks = ctx.tokenListCache || [];
    var byPrincipal = {};
    for (var i = 0; i < toks.length; i++) {
      var row = toks[i] || {};
      var pid = String(row.tenant_id || "").trim();
      if (!pid) continue;
      if (!byPrincipal[pid]) byPrincipal[pid] = [];
      byPrincipal[pid].push(row);
    }
    var userStats = ctx.adminUserStatsByPrincipal();
    var draftHtml = "";
    for (var d = 0; d < ctx.adminUserDrafts.length; d++) draftHtml += buildAdminUserDraftCardHtml(ctx.adminUserDrafts[d]);
    var usersHtml = "";
    var pids = Object.keys(byPrincipal);
    pids.sort();
    for (var p = 0; p < pids.length; p++) {
      var pid2 = pids[p];
      usersHtml += adminBuildUserCardHtml(pid2, byPrincipal[pid2], userStats[pid2] || null);
    }
    if (!usersHtml) usersHtml = '<p class="muted">No users yet. Add one to create a gateway token.</p>';
    return (
      '<div class="sum-feed-section" id="admin-users">' +
      operatorSectionHeadHtml("Users", "face_3", {
        actionHtml:
          typeof operatorSectionAddBtn === "function"
            ? operatorSectionAddBtn({ "data-admin-action": "user-add" }, "Add user")
            : "",
      }) +
      '<div class="sum-workspaces-intro"><p class="sum-workspaces-intro-lead">Onboard principals as first-class users with gateway tokens, conversation/workspace counts, and a scoped activity stream.</p></div>' +
      '<div class="sg-op-user-drafts-stack">' + draftHtml + "</div>" +
      '<div class="sg-op-user-cards-stack">' + usersHtml + "</div></div>"
    );
  }

  ctx.buildAdminUserDraftCardHtml = buildAdminUserDraftCardHtml;
  ctx.buildAdminUsersCardHtml = buildAdminUsersCardHtml;
  ctx.adminBuildUserCardHtml = adminBuildUserCardHtml;
};
