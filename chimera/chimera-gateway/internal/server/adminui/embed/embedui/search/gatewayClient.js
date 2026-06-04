/**
 * Gateway client for workspace search UI.
 */
(function () {
  "use strict";

  function parseJSON(r) {
    return r.json().catch(function () {
      return {};
    });
  }

  function fetchWorkspaces() {
    return fetch("/api/ui/indexer/workspaces", { credentials: "same-origin" }).then(function (r) {
      if (!r.ok) throw new Error("Could not load workspaces (HTTP " + r.status + ")");
      return parseJSON(r);
    });
  }

  function workspaceLabel(ws) {
    if (!ws) return "Workspace";
    var proj = ws.project_id != null ? String(ws.project_id).trim() : "";
    var flav = ws.flavor_id != null ? String(ws.flavor_id).trim() : "";
    if (proj && flav) return proj + " / " + flav;
    if (proj) return proj;
    if (flav) return flav;
    if (ws.id != null) return "workspace " + ws.id;
    return "Workspace";
  }

  function workspaceKey(ws) {
    if (!ws) return "";
    var proj = ws.project_id != null ? String(ws.project_id).trim() : "";
    var flav = ws.flavor_id != null ? String(ws.flavor_id).trim() : "";
    return proj + "\x00" + flav;
  }

  function search(opts) {
    opts = opts || {};
    var body = {
      query: String(opts.query || ""),
      project_id: String(opts.project_id || ""),
      flavor_id: opts.flavor_id != null ? String(opts.flavor_id) : ""
    };
    if (opts.score_threshold != null && !isNaN(Number(opts.score_threshold))) {
      body.score_threshold = Number(opts.score_threshold);
    }
    if (opts.top_k != null && Number(opts.top_k) > 0) {
      body.top_k = Number(opts.top_k);
    }
    return fetch("/api/ui/rag/search", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: opts.signal
    }).then(function (r) {
      return parseJSON(r).then(function (data) {
        if (!r.ok) {
          var msg = data && data.error ? String(data.error) : "Search failed (HTTP " + r.status + ")";
          throw new Error(msg);
        }
        return data;
      });
    });
  }

  globalThis.ChimeraSearch = globalThis.ChimeraSearch || {};
  globalThis.ChimeraSearch.Gateway = {
    fetchWorkspaces: fetchWorkspaces,
    search: search,
    workspaceLabel: workspaceLabel,
    workspaceKey: workspaceKey
  };
})();
