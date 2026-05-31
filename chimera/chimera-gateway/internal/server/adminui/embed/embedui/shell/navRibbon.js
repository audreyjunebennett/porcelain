/**
 * Shell left navigation ribbon — collapsed icon rail + expandable chat history panel.
 */
(function () {
  "use strict";

  var STORAGE_KEY = "chimera-ribbon-expanded";
  var DEFAULT_ROUTE = "/ui/chat";
  var SETTINGS_ROUTE = "/ui/settings?embed=1";
  var NARROW_BREAKPOINT = "(max-width: 480px)";

  function readExpanded() {
    try {
      return localStorage.getItem(STORAGE_KEY) === "1";
    } catch (_e) {
      return false;
    }
  }

  function writeExpanded(expanded) {
    try {
      localStorage.setItem(STORAGE_KEY, expanded ? "1" : "0");
    } catch (_e) {}
  }

  function isEditableTarget(el) {
    if (!el || !el.tagName) return false;
    var tag = el.tagName.toLowerCase();
    if (tag === "textarea") return true;
    if (tag === "select") return true;
    if (tag === "input") {
      var type = String(el.type || "text").toLowerCase();
      return type !== "button" && type !== "submit" && type !== "checkbox" && type !== "radio";
    }
    return !!el.isContentEditable;
  }

  function mount(opts) {
    opts = opts || {};
    var ribbon = opts.ribbon;
    var historyRoot = opts.historyRoot;
    var frame = opts.frame;
    var origin = opts.origin || window.location.origin;
    var navigateFrame = typeof opts.navigateFrame === "function" ? opts.navigateFrame : function () {};
    var currentRouteFromFrame =
      typeof opts.currentRouteFromFrame === "function" ? opts.currentRouteFromFrame : function () {
        return "";
      };
    var isSettingsRoute =
      typeof opts.isSettingsRoute === "function"
        ? opts.isSettingsRoute
        : function (route) {
            return String(route || "").indexOf("/ui/settings") === 0;
          };
    var isChatRoute =
      typeof opts.isChatRoute === "function"
        ? opts.isChatRoute
        : function (route) {
            return String(route || "").split("?")[0] === "/ui/chat";
          };
    var getShellReturnRoute =
      typeof opts.getShellReturnRoute === "function" ? opts.getShellReturnRoute : function () {
        return DEFAULT_ROUTE;
      };
    var setShellReturnRoute =
      typeof opts.setShellReturnRoute === "function" ? opts.setShellReturnRoute : function () {};

    var getShellReturnConversationId =
      typeof opts.getShellReturnConversationId === "function"
        ? opts.getShellReturnConversationId
        : function () {
            return "";
          };
    var setShellReturnConversationId =
      typeof opts.setShellReturnConversationId === "function" ? opts.setShellReturnConversationId : function () {};

    if (!ribbon) return null;

    var toggleBtn = ribbon.querySelector("[data-ribbon-action='toggle']");
    var newChatBtn = ribbon.querySelector("[data-ribbon-action='new-chat']");
    var settingsBtn = ribbon.querySelector("[data-ribbon-action='settings']");
    var userExpandedPref = readExpanded();
    var expanded = userExpandedPref;
    var narrowForced = false;
    var narrowMq =
      typeof window.matchMedia === "function" ? window.matchMedia(NARROW_BREAKPOINT) : null;
    var historyPanel = null;
    var shellReturnRoute = getShellReturnRoute() || DEFAULT_ROUTE;
    var lastActiveConversationId = "";

    function postChatAction(action, extra) {
      var w = frame && frame.contentWindow;
      if (!w) return;
      var msg = { type: "chimera-chat-action", action: action };
      if (extra) {
        for (var k in extra) {
          if (Object.prototype.hasOwnProperty.call(extra, k)) msg[k] = extra[k];
        }
      }
      try {
        w.postMessage(msg, origin);
      } catch (_e) {}
    }

    function waitForChatApp() {
      return new Promise(function (resolve) {
        var attempts = 0;
        function check() {
          var w = frame && frame.contentWindow;
          try {
            if (w && w.ChimeraChat && w.ChimeraChat.App) {
              resolve();
              return;
            }
          } catch (_e) {}
          if (++attempts > 60) {
            resolve();
            return;
          }
          window.setTimeout(check, 50);
        }
        check();
      });
    }

    function ensureChatRoute() {
      var route = currentRouteFromFrame();
      if (isChatRoute(route)) return waitForChatApp();
      navigateFrame(DEFAULT_ROUTE);
      return new Promise(function (resolve) {
        if (!frame) {
          resolve();
          return;
        }
        var done = false;
        function finish() {
          if (done) return;
          done = true;
          frame.removeEventListener("load", finish);
          waitForChatApp().then(resolve);
        }
        frame.addEventListener("load", finish);
        window.setTimeout(finish, 2000);
      });
    }

    function isNarrowViewport() {
      return !!(narrowMq && narrowMq.matches);
    }

    function setExpanded(next, persist) {
      if (narrowForced && next) return;
      expanded = !!next;
      ribbon.classList.toggle("shell-ribbon--expanded", expanded);
      ribbon.setAttribute("aria-expanded", expanded ? "true" : "false");
      var tip = expanded ? "Hide chat history" : "Show chat history";
      if (toggleBtn && !narrowForced) {
        toggleBtn.title = tip;
        toggleBtn.setAttribute("aria-label", tip);
      }
      if (persist !== false && !narrowForced) {
        userExpandedPref = expanded;
        writeExpanded(expanded);
      }
    }

    function syncToggleForNarrow() {
      if (!toggleBtn) return;
      if (narrowForced) {
        var narrowTip = "Chat history requires a wider window";
        toggleBtn.disabled = true;
        toggleBtn.setAttribute("aria-disabled", "true");
        toggleBtn.title = narrowTip;
        toggleBtn.setAttribute("aria-label", narrowTip);
      } else {
        toggleBtn.disabled = false;
        toggleBtn.setAttribute("aria-disabled", "false");
        var tip = expanded ? "Hide chat history" : "Show chat history";
        toggleBtn.title = tip;
        toggleBtn.setAttribute("aria-label", tip);
      }
    }

    function applyViewportRibbonLayout() {
      narrowForced = isNarrowViewport();
      ribbon.classList.toggle("shell-ribbon--narrow", narrowForced);
      syncToggleForNarrow();
      if (narrowForced) {
        setExpanded(false, false);
        return;
      }
      setExpanded(userExpandedPref, false);
    }

    function toggleExpanded() {
      if (narrowForced) return;
      setExpanded(!expanded);
    }

    function newChat() {
      ensureChatRoute().then(function () {
        postChatAction("new");
      });
    }

    function restoreChatConversation(conversationId) {
      return waitForChatApp().then(function () {
        if (conversationId) {
          postChatAction("open", { conversationId: conversationId });
        }
      });
    }

    function waitForFrameLoad() {
      return new Promise(function (resolve) {
        if (!frame) {
          resolve();
          return;
        }
        var done = false;
        function finish() {
          if (done) return;
          done = true;
          frame.removeEventListener("load", finish);
          resolve();
        }
        frame.addEventListener("load", finish);
        window.setTimeout(finish, 2000);
      });
    }

    function openSettings() {
      var route = currentRouteFromFrame();
      if (isSettingsRoute(route)) {
        var returnRoute = getShellReturnRoute() || DEFAULT_ROUTE;
        var returnConvId = getShellReturnConversationId();
        navigateFrame(returnRoute);
        waitForFrameLoad().then(function () {
          return restoreChatConversation(returnConvId);
        });
        return;
      }
      var rememberedConvId = isChatRoute(route) ? lastActiveConversationId : "";
      setShellReturnRoute(route || DEFAULT_ROUTE);
      setShellReturnConversationId(rememberedConvId);
      syncHistoryActive("");
      navigateFrame(SETTINGS_ROUTE);
    }

    function openConversation(id, extra) {
      if (!id) return;
      ensureChatRoute().then(function () {
        postChatAction("open", extra || { conversationId: id });
      });
    }

    if (historyRoot && globalThis.ChimeraChat && ChimeraChat.HistoryPanel) {
      historyPanel = ChimeraChat.HistoryPanel.mount({
        root: historyRoot,
        filtersRoot: document.getElementById("shell-ribbon-filters"),
        embedded: true,
        onOpen: function (id) {
          openConversation(id);
        }
      });
    }

    function refreshHistory() {
      if (historyPanel && historyPanel.refresh) historyPanel.refresh();
    }

    function syncHistoryActive(conversationId) {
      if (!historyPanel) return;
      if (conversationId) historyPanel.setActiveId(conversationId);
      else historyPanel.clearActive();
    }

    if (toggleBtn) {
      toggleBtn.addEventListener("click", toggleExpanded);
    }
    if (newChatBtn) {
      newChatBtn.addEventListener("click", newChat);
    }
    if (settingsBtn) {
      settingsBtn.addEventListener("click", openSettings);
    }

    window.addEventListener("keydown", function (ev) {
      if (!ev || !ev.ctrlKey || ev.altKey || ev.metaKey) return;
      if (isEditableTarget(ev.target)) return;
      var key = String(ev.key || "").toLowerCase();
      if (key === "b") {
        ev.preventDefault();
        toggleExpanded();
        return;
      }
      if (key === "n") {
        ev.preventDefault();
        newChat();
      }
    });

    window.addEventListener("message", function (ev) {
      if (!ev || ev.origin !== origin) return;
      var data = ev.data;
      if (!data || data.type !== "chimera-chat-state") return;
      if (data.action === "refresh-history") refreshHistory();
      if (data.action === "set-active") {
        lastActiveConversationId = data.conversationId || "";
        syncHistoryActive(lastActiveConversationId);
      }
      if (data.action === "clear-active") {
        lastActiveConversationId = "";
        syncHistoryActive("");
      }
    });

    function updateSettingsButton(inSettings) {
      if (!settingsBtn) return;
      if (inSettings) {
        settingsBtn.title = "Close settings";
        settingsBtn.setAttribute("aria-label", "Close settings");
      } else {
        settingsBtn.title = "Settings";
        settingsBtn.setAttribute("aria-label", "Settings");
      }
    }

    function onFrameRouteChange() {
      updateSettingsButton(isSettingsRoute(currentRouteFromFrame()));
    }

    if (frame) {
      frame.addEventListener("load", onFrameRouteChange);
    }
    onFrameRouteChange();

    setExpanded(userExpandedPref, false);
    applyViewportRibbonLayout();
    if (narrowMq) {
      if (typeof narrowMq.addEventListener === "function") {
        narrowMq.addEventListener("change", applyViewportRibbonLayout);
      } else if (typeof narrowMq.addListener === "function") {
        narrowMq.addListener(applyViewportRibbonLayout);
      }
    }

    return {
      toggle: toggleExpanded,
      setExpanded: setExpanded,
      newChat: newChat,
      openSettings: openSettings,
      refreshHistory: refreshHistory,
      syncHistoryActive: syncHistoryActive,
      getHistoryPanel: function () {
        return historyPanel;
      },
      setShellReturnRoute: function (route) {
        shellReturnRoute = route || DEFAULT_ROUTE;
        setShellReturnRoute(shellReturnRoute);
      },
      getShellReturnRoute: function () {
        return shellReturnRoute;
      }
    };
  }

  globalThis.ChimeraShell = globalThis.ChimeraShell || {};
  globalThis.ChimeraShell.NavRibbon = { mount: mount };
})();
