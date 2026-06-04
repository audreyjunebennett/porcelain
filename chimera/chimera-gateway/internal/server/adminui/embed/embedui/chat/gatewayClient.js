/**
 * Gateway API client for chat UI.
 */
(function () {
  "use strict";

  var tokenCache = null;

  function parseJSON(r) {
    return r.json().catch(function () {
      return {};
    });
  }

  function authHeaders(token) {
    return {
      Authorization: "Bearer " + token,
      "Content-Type": "application/json"
    };
  }

  function fetchToken() {
    if (tokenCache) return Promise.resolve(tokenCache);
    return fetch("/api/ui/tokens", { credentials: "same-origin" })
      .then(function (r) {
        if (!r.ok) throw new Error("Could not load gateway token (HTTP " + r.status + ")");
        return parseJSON(r);
      })
      .then(function (data) {
        var list = data && Array.isArray(data.tokens) ? data.tokens : [];
        for (var i = 0; i < list.length; i++) {
          var t = list[i] && list[i].token ? String(list[i].token).trim() : "";
          if (t) {
            tokenCache = t;
            return t;
          }
        }
        throw new Error("No gateway token available — sign in again.");
      });
  }

  function fetchModels() {
    return fetchToken().then(function (token) {
      return fetch("/v1/models", { headers: { Authorization: "Bearer " + token } }).then(function (r) {
        if (!r.ok) throw new Error("Could not load models (HTTP " + r.status + ")");
        return parseJSON(r);
      });
    });
  }

  function fetchWorkspaces() {
    return fetch("/api/ui/indexer/workspaces", { credentials: "same-origin" }).then(function (r) {
      if (!r.ok) throw new Error("Could not load workspaces (HTTP " + r.status + ")");
      return parseJSON(r);
    });
  }

  function fetchCorpusStale(ws) {
    var q = "";
    if (ws) {
      if (ws.project_id) q += "project_id=" + encodeURIComponent(String(ws.project_id)) + "&";
      if (ws.flavor_id) q += "flavor_id=" + encodeURIComponent(String(ws.flavor_id)) + "&";
    }
    return fetch("/api/ui/indexer/corpus/stale?" + q.replace(/&$/, ""), { credentials: "same-origin" }).then(
      function (r) {
        if (!r.ok) return { coherence_mode: "off", entries: [] };
        return parseJSON(r);
      }
    );
  }

  function workspaceLabel(ws) {
    if (!ws) return "Default";
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

  function base64ToUtf8(b64) {
    var bin = atob(b64);
    var bytes = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    if (globalThis.TextDecoder) {
      return new TextDecoder("utf-8").decode(bytes);
    }
    return decodeURIComponent(
      bin
        .split("")
        .map(function (c) {
          return "%" + ("00" + c.charCodeAt(0).toString(16)).slice(-2);
        })
        .join("")
    );
  }

  function parseRAGHitsHeader(raw) {
    if (!raw) return null;
    var s = String(raw).trim();
    if (!s) return null;
    var jsonText = s;
    // Gateway sends base64-encoded JSON to keep headers ASCII-safe for UTF-8 snippets.
    if (s.charAt(0) !== "[") {
      try {
        jsonText = base64ToUtf8(s);
      } catch (_e) {
        return null;
      }
    }
    try {
      var parsed = JSON.parse(jsonText);
      return Array.isArray(parsed) ? parsed : null;
    } catch (_e2) {
      return null;
    }
  }

  function readTurnMetadata(res) {
    var upstream = res.headers.get("X-Chimera-Upstream-Model") || "";
    var ragRaw = res.headers.get("X-Chimera-RAG-Hits") || "";
    var conv = res.headers.get("X-Chimera-Conversation-Id") || "";
    return {
      upstreamModel: upstream.trim(),
      ragHits: parseRAGHitsHeader(ragRaw),
      conversationId: conv.trim()
    };
  }

  function chatCompletion(opts) {
    opts = opts || {};
    return fetchToken().then(function (token) {
      var headers = authHeaders(token);
      if (opts.conversationId) {
        headers["X-Chimera-Conversation-Id"] = opts.conversationId;
      }
      if (opts.workspace) {
        if (opts.workspace.project_id) {
          headers["X-Chimera-Project"] = String(opts.workspace.project_id);
        }
        if (opts.workspace.flavor_id) {
          headers["X-Chimera-Flavor-Id"] = String(opts.workspace.flavor_id);
        }
        if (opts.workspace.id != null) {
          headers["X-Chimera-Workspace-Id"] = String(opts.workspace.id);
        }
      }
      return fetch("/v1/chat/completions", {
        method: "POST",
        headers: headers,
        body: JSON.stringify({
          model: opts.model,
          messages: opts.messages || [],
          stream: opts.stream !== false
        }),
        signal: opts.signal
      });
    });
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Gateway = {
    fetchToken: fetchToken,
    fetchModels: fetchModels,
    fetchWorkspaces: fetchWorkspaces,
    fetchCorpusStale: fetchCorpusStale,
    chatCompletion: chatCompletion,
    readTurnMetadata: readTurnMetadata,
    workspaceLabel: workspaceLabel,
    workspaceKey: workspaceKey,
    clearTokenCache: function () {
      tokenCache = null;
    }
  };
})();
