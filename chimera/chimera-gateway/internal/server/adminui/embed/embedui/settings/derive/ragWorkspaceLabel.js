/**
 * RAG workspace labels from requested tenant/project/flavor — not collection-id inference.
 *
 * Exports:
 * - ChimeraSettings.Derive.mountRagWorkspaceLabel(deps)
 * - ChimeraSettings.Derive.extractRagCoordsFromFlat(flat)
 * - ChimeraSettings.Derive.extractRagCoordsFromEvents(events, getFlat, opts)
 * - ChimeraSettings.Derive.resolveRagWorkspaceLabel(tenantId, projectId, flavorId)
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};

  var deps = null;
  var MISSING_SUFFIX = " - missing or undefined";

  function normalizeFlavor(v) {
    if (deps && typeof deps.normalizeFlavor === "function") return deps.normalizeFlavor(v);
    if (v == null || v === "—") return "";
    return String(v).trim();
  }

  function mountRagWorkspaceLabel(mountDeps) {
    deps = mountDeps || null;
  }

  function extractRagCoordsFromFlat(flat) {
    if (!flat || typeof flat !== "object") return null;
    var tenant = String(flat.tenant || flat.tenant_id || flat.principal_id || "").trim();
    var project = String(flat.project || flat.project_id || flat.ingest_project || "").trim();
    var flavor =
      flat.flavor != null
        ? String(flat.flavor).trim()
        : flat.flavor_id != null
          ? String(flat.flavor_id).trim()
          : "";
    if (!project) return null;
    return { tenantId: tenant, projectId: project, flavorId: flavor };
  }

  function extractRagCoordsFromEvents(events, getFlat, opts) {
    opts = opts || {};
    events = Array.isArray(events) ? events : [];
    getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
    var collFilter = opts.collectionFilter != null ? String(opts.collectionFilter).trim() : "";
    var slugs = ["conversation.rag.attached", "rag.query", "rag.embed", "conversation.rag.span"];
    var si;
    for (si = 0; si < slugs.length; si++) {
      var want = slugs[si];
      var i;
      for (i = events.length - 1; i >= 0; i--) {
        var f = getFlat(events[i].parsed);
        var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
        if (msg !== want) continue;
        var fc = String(f.collection || "").trim();
        if (collFilter && fc && fc !== collFilter) continue;
        var coords = extractRagCoordsFromFlat(f);
        if (coords && coords.projectId) return coords;
      }
    }
    return null;
  }

  function resolveRagWorkspaceLabel(tenantId, projectId, flavorId) {
    var project = String(projectId || "").trim();
    if (!project) return { label: "", known: false, proposed: "" };
    if (!deps) return { label: "", known: false, proposed: project };

    var flavNorm = normalizeFlavor(flavorId);
    var nested = typeof deps.getOperatorWorkspaces === "function" ? deps.getOperatorWorkspaces() : [];
    var wi;
    for (wi = 0; wi < nested.length; wi++) {
      var ws = nested[wi];
      if (String(ws.project_id || "").trim() !== project) continue;
      if (normalizeFlavor(ws.flavor_id) !== flavNorm) continue;
      var title = typeof deps.operatorWorkspaceTitle === "function" ? deps.operatorWorkspaceTitle(ws) : "";
      if (title) return { label: title, known: true, proposed: title };
    }

    var userLab = typeof deps.tenantUserLabel === "function" ? deps.tenantUserLabel(tenantId) : "—";
    var proposed =
      typeof deps.workspaceTitleFromParts === "function"
        ? deps.workspaceTitleFromParts(userLab, project, flavorId)
        : userLab + ":" + project;
    return { label: proposed + MISSING_SUFFIX, known: false, proposed: proposed };
  }

  ChimeraSettings.Derive.mountRagWorkspaceLabel = mountRagWorkspaceLabel;
  ChimeraSettings.Derive.extractRagCoordsFromFlat = extractRagCoordsFromFlat;
  ChimeraSettings.Derive.extractRagCoordsFromEvents = extractRagCoordsFromEvents;
  ChimeraSettings.Derive.resolveRagWorkspaceLabel = resolveRagWorkspaceLabel;
})();
