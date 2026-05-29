/**
 * Smart scroll: pin to bottom only when the user is already at the bottom.
 */
(function () {
  "use strict";

  var PIN_PX = 48;

  function nearBottom(el) {
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight <= PIN_PX;
  }

  function scrollToBottom(el, force) {
    if (!el) return;
    if (!force && !nearBottom(el)) return;
    el.scrollTop = el.scrollHeight;
  }

  function createTracker(viewport) {
    var pinned = true;
    if (viewport) {
      viewport.addEventListener("scroll", function () {
        pinned = nearBottom(viewport);
      }, { passive: true });
    }
    return {
      isPinned: function () {
        return pinned;
      },
      follow: function () {
        scrollToBottom(viewport, pinned);
      },
      forceBottom: function () {
        pinned = true;
        scrollToBottom(viewport, true);
      }
    };
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.Scroll = {
    nearBottom: nearBottom,
    scrollToBottom: scrollToBottom,
    createTracker: createTracker
  };
})();
