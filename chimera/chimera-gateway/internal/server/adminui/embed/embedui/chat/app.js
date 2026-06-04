/**
 * Chat application orchestrator.
 */
(function () {
  "use strict";

  var State = globalThis.ChimeraChat.State;
  var Gateway = globalThis.ChimeraChat.Gateway;
  var Stream = globalThis.ChimeraChat.Stream;
  var Scroll = globalThis.ChimeraChat.Scroll;
  var MsgRender = globalThis.ChimeraChat.Render.Messages;
  var InputRender = globalThis.ChimeraChat.Render.Input;
  var HistoryClient = globalThis.ChimeraChat.HistoryClient;

  var state = State.createState();
  state.staleBySource = {};
  state.coherenceMode = "warn";
  var models = [];
  var workspaces = [];
  var workspaceByKey = {};

  var viewport = document.getElementById("chat-viewport");
  var modelSel = document.getElementById("chat-model");
  var workspaceSel = document.getElementById("chat-workspace");
  var inputEl = document.getElementById("chat-input");
  var sendBtn = document.getElementById("chat-send");

  var scrollTracker = Scroll.createTracker(viewport);
  var titleRoot = document.getElementById("chat-title-bar");
  var titleBar =
    globalThis.ChimeraChat.Render &&
    ChimeraChat.Render.TitleBar &&
    titleRoot
      ? ChimeraChat.Render.TitleBar.mount({
          root: titleRoot,
          onSave: saveConversationTitle
        })
      : null;
  var composer = InputRender.mount({
    textarea: inputEl,
    onSubmit: submitMessage,
    getHistory: function () {
      return state.inputHistory;
    }
  });

  function setStreaming(active) {
    state.isStreaming = !!active;
    if (sendBtn) sendBtn.disabled = active;
    if (inputEl) inputEl.disabled = active;
  }

  function findMessage(id) {
    for (var i = 0; i < state.messages.length; i++) {
      if (state.messages[i].id === id) return state.messages[i];
    }
    return null;
  }

  function syncStaleCtx() {
    globalThis.ChimeraChat = globalThis.ChimeraChat || {};
    ChimeraChat._ragStaleCtx = {
      map: state.staleBySource || {},
      mode: state.coherenceMode || "warn"
    };
  }

  function paint() {
    if (!viewport) return;
    syncStaleCtx();
    viewport.innerHTML = MsgRender.renderAll(state.messages);
    scrollTracker.follow();
  }

  function refreshCorpusStale() {
    return Gateway.fetchCorpusStale(selectedWorkspace())
      .then(function (data) {
        state.coherenceMode =
          data && data.coherence_mode != null ? String(data.coherence_mode) : "warn";
        var map = {};
        var entries = data && Array.isArray(data.entries) ? data.entries : [];
        var ei;
        for (ei = 0; ei < entries.length; ei++) {
          var e = entries[ei] || {};
          if (e.source != null) map[String(e.source)] = e;
        }
        state.staleBySource = map;
        paint();
      })
      .catch(function () {});
  }

  function paintAssistantDelta(msg) {
    MsgRender.updateMessageBody(viewport, msg);
    scrollTracker.follow();
  }

  function pushInputHistory(text) {
    var t = String(text || "").trim();
    if (!t) return;
    if (state.inputHistory.length && state.inputHistory[state.inputHistory.length - 1] === t) return;
    state.inputHistory.push(t);
    if (state.inputHistory.length > 100) state.inputHistory.shift();
  }

  function selectedWorkspace() {
    var key = state.selectedWorkspaceKey || "";
    if (!key) return null;
    return workspaceByKey[key] || null;
  }

  function populateModels(data) {
    models = [];
    if (data && Array.isArray(data.data)) {
      for (var i = 0; i < data.data.length; i++) {
        var m = data.data[i];
        if (m && m.id) models.push(String(m.id));
      }
    }
    if (!modelSel) return;
    var prev = state.selectedModel;
    modelSel.innerHTML = "";
    for (var j = 0; j < models.length; j++) {
      var opt = document.createElement("option");
      opt.value = models[j];
      opt.textContent = models[j];
      modelSel.appendChild(opt);
    }
    if (prev && models.indexOf(prev) >= 0) {
      modelSel.value = prev;
      state.selectedModel = prev;
    } else if (models.length) {
      state.selectedModel = models[0];
      modelSel.value = models[0];
    }
  }

  function populateWorkspaces(data) {
    workspaces = [];
    workspaceByKey = {};
    var list = data && Array.isArray(data.workspaces) ? data.workspaces : [];
    for (var i = 0; i < list.length; i++) {
      workspaces.push(list[i]);
      workspaceByKey[Gateway.workspaceKey(list[i])] = list[i];
    }
    if (!workspaceSel) return;
    var prev = state.selectedWorkspaceKey;
    workspaceSel.innerHTML = "";
    var defOpt = document.createElement("option");
    defOpt.value = "";
    defOpt.textContent = "Default";
    workspaceSel.appendChild(defOpt);
    for (var j = 0; j < workspaces.length; j++) {
      var ws = workspaces[j];
      var opt = document.createElement("option");
      opt.value = Gateway.workspaceKey(ws);
      opt.textContent = Gateway.workspaceLabel(ws);
      workspaceSel.appendChild(opt);
    }
    if (prev && workspaceByKey[prev]) {
      workspaceSel.value = prev;
    } else {
      state.selectedWorkspaceKey = "";
      workspaceSel.value = "";
    }
  }

  function refreshCatalogs() {
    Gateway.fetchModels()
      .then(populateModels)
      .catch(function (err) {
        console.warn("chat models refresh:", err);
      });
    Gateway.fetchWorkspaces()
      .then(function (data) {
        populateWorkspaces(data);
        return refreshCorpusStale();
      })
      .catch(function (err) {
        console.warn("chat workspaces refresh:", err);
      });
  }

  function copyText(text) {
    var t = String(text || "");
    if (!t) return false;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(t).then(
        function () {
          return true;
        },
        function () {
          return legacyCopy(t);
        }
      );
    }
    return legacyCopy(t);
  }

  function legacyCopy(text) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try {
      ok = document.execCommand("copy");
    } catch (_e) {}
    document.body.removeChild(ta);
    return Promise.resolve(!!ok);
  }

  function removeTrailingError() {
    if (!state.messages.length) return;
    var last = state.messages[state.messages.length - 1];
    if (last && last.role === "error") state.messages.pop();
  }

  function ensureConversationId() {
    if (String(state.conversationId || "").trim()) return state.conversationId;
    state.conversationId = State.newConversationId();
    return state.conversationId;
  }

  function notifyShell(data) {
    if (window.parent === window) return;
    try {
      window.parent.postMessage(data, window.location.origin);
    } catch (_e) {}
  }

  function refreshHistory() {
    notifyShell({ type: "chimera-chat-state", action: "refresh-history" });
  }

  function syncShellActive() {
    var hasTranscript = state.messages.length > 0;
    notifyShell({
      type: "chimera-chat-state",
      action: hasTranscript && state.conversationId ? "set-active" : "clear-active",
      conversationId: hasTranscript ? state.conversationId || "" : ""
    });
  }

  function syncTitleFromHistory() {
    if (!titleBar || !state.conversationId) return;
    var listFn = HistoryClient && HistoryClient.listConversations;
    if (!listFn) return;
    listFn({ limit: 100 })
      .then(function (data) {
        var rows = data && Array.isArray(data.conversations) ? data.conversations : [];
        for (var i = 0; i < rows.length; i++) {
          var row = rows[i];
          if (row && row.conversation_id === state.conversationId) {
            var t = row.title || row.preview_text || "";
            if (t) {
              state.conversationTitle = t;
              titleBar.setTitle(t);
            }
            return;
          }
        }
      })
      .catch(function () {});
  }

  function saveConversationTitle(title) {
    title = String(title || "").trim();
    if (!title) return;
    state.conversationTitle = title;
    if (titleBar) titleBar.setTitle(title);
    if (!HistoryClient || !state.conversationId) return;
    HistoryClient.patchTitle(state.conversationId, title)
      .then(function () {
        refreshHistory();
      })
      .catch(function (err) {
        console.warn("title save:", err);
      });
  }

  function maybeSetAutoTitle(text) {
    if (state.conversationTitle || (titleBar && titleBar.getTitle())) return;
    var auto = State.autoTitleFromMessage(text);
    if (!auto) return;
    state.conversationTitle = auto;
    if (titleBar) titleBar.setTitle(auto);
  }

  function sendUserText(text, opts) {
    opts = opts || {};
    text = String(text || "").trim();
    if (!text || state.isStreaming) return;
    if (!state.selectedModel) {
      alert("No model selected.");
      return;
    }

    removeTrailingError();

    var isRetry = !!opts.retry;
    if (!isRetry) {
      var userMsg = State.createMessage("user", text);
      state.messages.push(userMsg);
      maybeSetAutoTitle(text);
    }

    var assistant = State.createMessage("assistant", "", {
      status: "streaming",
      selectedModel: state.selectedModel
    });
    state.messages.push(assistant);
    paint();
    scrollTracker.forceBottom();

    if (!opts.skipHistory) pushInputHistory(text);
    composer.clear();
    syncShellActive();

    if (state.abortController) {
      try {
        state.abortController.abort();
      } catch (_e) {}
    }
    state.abortController = new AbortController();
    setStreaming(true);

    var apiMessages = State.messagesForAPI(state.messages.slice(0, -1));
    var conversationId = ensureConversationId();

    Gateway.chatCompletion({
      model: state.selectedModel,
      messages: apiMessages,
      stream: true,
      conversationId: conversationId,
      workspace: selectedWorkspace(),
      signal: state.abortController.signal
    })
      .then(function (res) {
        var meta = Gateway.readTurnMetadata(res);
        if (meta.conversationId) state.conversationId = meta.conversationId;
        assistant.upstreamModel = meta.upstreamModel || state.selectedModel;
        assistant.ragHits = meta.ragHits;
        return Stream.handleResponse(res, function (delta) {
          assistant.content += delta;
          paintAssistantDelta(assistant);
        }).then(function () {
          assistant.status = "done";
          paint();
        });
      })
      .catch(function (err) {
        if (err && err.name === "AbortError") return;
        var last = state.messages[state.messages.length - 1];
        if (last && last.role === "assistant" && last.status === "streaming") {
          state.messages.pop();
        }
        state.messages.push(
          State.createMessage("error", "", {
            error: err && err.message ? err.message : String(err),
            retryUserText: text
          })
        );
        paint();
      })
      .finally(function () {
        setStreaming(false);
        state.abortController = null;
        composer.focus();
        refreshHistory();
        syncTitleFromHistory();
      });
  }

  function submitMessage() {
    sendUserText(composer.getValue());
  }

  function turnsToMessages(turns) {
    var out = [];
    var list = Array.isArray(turns) ? turns : [];
    for (var i = 0; i < list.length; i++) {
      var t = list[i] || {};
      var role = t.role || "";
      if (role === "user") {
        out.push(State.createMessage("user", t.content || ""));
      } else if (role === "assistant") {
        out.push(
          State.createMessage("assistant", t.content || "", {
            selectedModel: t.selected_model || "",
            upstreamModel: t.resolved_model || t.selected_model || "",
            ragHits: t.ragHits || null
          })
        );
      } else if (role === "error") {
        out.push(
          State.createMessage("error", "", {
            error: t.content || t.error_detail || "Error",
            retryUserText: t.retryUserText || t.retry_user_text || ""
          })
        );
      }
    }
    return out;
  }

  function openConversation(conversationId, opts) {
    opts = opts || {};
    if (!HistoryClient || !conversationId || state.isStreaming) return Promise.resolve();
    return HistoryClient.loadTranscript(conversationId)
      .then(function (data) {
        state.conversationId = data.conversation_id || conversationId;
        state.conversationTitle = data.title || data.preview_text || "";
        state.messages = turnsToMessages(data.turns);
        if (titleBar) titleBar.setTitle(state.conversationTitle);
        syncShellActive();
        paint();
        scrollTracker.forceBottom();
        if (opts.startEdit && titleBar) titleBar.startEdit();
      })
      .catch(function (err) {
        if (err && err.status === 404) {
          newChat();
          return;
        }
        alert(err && err.message ? err.message : String(err));
      });
  }

  function newChat() {
    if (state.isStreaming && state.abortController) {
      try {
        state.abortController.abort();
      } catch (_e) {}
    }
    state.messages = [];
    state.conversationId = State.newConversationId();
    state.conversationTitle = "";
    setStreaming(false);
    composer.clear();
    syncShellActive();
    if (titleBar) titleBar.setTitle("");
    paint();
    composer.focus();
  }

  function getTranscriptText() {
    return State.transcriptText(state.messages);
  }

  function copyAllConversation() {
    return copyText(getTranscriptText());
  }

  if (modelSel) {
    modelSel.addEventListener("change", function () {
      state.selectedModel = modelSel.value || "";
    });
  }
  if (workspaceSel) {
    workspaceSel.addEventListener("change", function () {
      state.selectedWorkspaceKey = workspaceSel.value || "";
      refreshCorpusStale();
    });
  }
  if (sendBtn) sendBtn.addEventListener("click", submitMessage);

  window.addEventListener("message", function (ev) {
    if (!ev || ev.origin !== window.location.origin) return;
    var data = ev.data;
    if (!data || data.type !== "chimera-chat-action") return;
    if (data.action === "new") newChat();
    if (data.action === "copy-all") copyAllConversation();
    if (data.action === "open" && data.conversationId) {
      openConversation(data.conversationId, { startEdit: !!data.startEdit }).then(finishEmbedBootstrap);
    }
    if (data.action === "deleted" && data.conversationId && state.conversationId === data.conversationId) {
      newChat();
    }
  });

  var embedBootstrapped = false;
  function finishEmbedBootstrap() {
    if (embedBootstrapped || window.parent === window) return;
    embedBootstrapped = true;
    if (!state.conversationId) ensureConversationId();
    syncShellActive();
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.App = {
    newChat: newChat,
    getTranscriptText: getTranscriptText,
    copyAllConversation: copyAllConversation
  };

  if (viewport) {
    viewport.addEventListener("click", function (ev) {
      var t = ev.target;
      if (!t || !t.closest) return;
      var copyBtn = t.closest(".chat-msg__copy-btn");
      if (copyBtn) {
        copyText(copyBtn.getAttribute("data-copy-text") || "");
        return;
      }
      var retry = t.closest(".chat-msg__retry-btn");
      if (retry) {
        sendUserText(retry.getAttribute("data-retry-text") || "", { skipHistory: true, retry: true });
        return;
      }
      var snippetsToggle = t.closest(".chat-msg__snippets-toggle");
      if (snippetsToggle) {
        var panelId = snippetsToggle.getAttribute("aria-controls") || "";
        var panel = panelId ? document.getElementById(panelId) : null;
        if (!panel) return;
        var open = snippetsToggle.getAttribute("aria-expanded") === "true";
        open = !open;
        snippetsToggle.setAttribute("aria-expanded", open ? "true" : "false");
        panel.hidden = !open;
      }
    });
  }

  window.addEventListener("focus", refreshCatalogs);
  setInterval(refreshCatalogs, 30000);

  refreshCatalogs();
  paint();
  composer.focus();
  if (window.parent === window) {
    ensureConversationId();
    syncShellActive();
  } else {
    window.setTimeout(finishEmbedBootstrap, 500);
  }
})();
