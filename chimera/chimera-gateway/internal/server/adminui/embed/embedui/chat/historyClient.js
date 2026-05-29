/**
 * Conversation history API client.
 */
(function () {
  "use strict";

  function parseJSON(r) {
    return r.json().catch(function () {
      return {};
    });
  }

  function apiError(r, data) {
    var msg = data && data.error ? String(data.error) : "HTTP " + r.status;
    var err = new Error(msg);
    err.status = r.status;
    return err;
  }

  function listConversations(opts) {
    opts = opts || {};
    var q = [];
    if (opts.limit != null) q.push("limit=" + encodeURIComponent(String(opts.limit)));
    if (opts.offset != null) q.push("offset=" + encodeURIComponent(String(opts.offset)));
    if (opts.flaggedOnly) q.push("flagged=1");
    var path = "/api/ui/conversations" + (q.length ? "?" + q.join("&") : "");
    return fetch(path, { credentials: "same-origin" }).then(function (r) {
      return parseJSON(r).then(function (data) {
        if (!r.ok) throw apiError(r, data);
        return data;
      });
    });
  }

  function loadTranscript(conversationId) {
    return fetch("/api/ui/conversations/" + encodeURIComponent(conversationId), {
      credentials: "same-origin"
    }).then(function (r) {
      return parseJSON(r).then(function (data) {
        if (!r.ok) throw apiError(r, data);
        return data;
      });
    });
  }

  function patchTitle(conversationId, title) {
    return fetch("/api/ui/conversations/" + encodeURIComponent(conversationId), {
      method: "PATCH",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: title })
    }).then(function (r) {
      return parseJSON(r).then(function (data) {
        if (!r.ok) throw apiError(r, data);
        return data;
      });
    });
  }

  function setFlagged(conversationId, flagged) {
    return fetch("/api/ui/conversations/" + encodeURIComponent(conversationId) + "/flag", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ flagged: !!flagged })
    }).then(function (r) {
      return parseJSON(r).then(function (data) {
        if (!r.ok) throw apiError(r, data);
        return data;
      });
    });
  }

  function deleteConversation(conversationId) {
    return fetch("/api/ui/conversations/" + encodeURIComponent(conversationId), {
      method: "DELETE",
      credentials: "same-origin"
    }).then(function (r) {
      if (r.status === 204) return { ok: true };
      return parseJSON(r).then(function (data) {
        if (!r.ok) throw apiError(r, data);
        return data;
      });
    });
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.HistoryClient = {
    listConversations: listConversations,
    loadTranscript: loadTranscript,
    patchTitle: patchTitle,
    setFlagged: setFlagged,
    deleteConversation: deleteConversation
  };
})();
