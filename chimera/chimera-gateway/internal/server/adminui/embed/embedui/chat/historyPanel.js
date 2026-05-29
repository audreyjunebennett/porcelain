/**
 * Conversation history sidebar panel.
 */
(function () {
  "use strict";

  var esc =
    globalThis.ChimeraUI && ChimeraUI.escapeHtml
      ? ChimeraUI.escapeHtml
      : function (s) {
          return String(s || "");
        };

  function relativeTime(iso) {
    if (!iso) return "";
    var t = Date.parse(iso);
    if (isNaN(t)) return "";
    var diff = Date.now() - t;
    var sec = Math.floor(diff / 1000);
    if (sec < 60) return "just now";
    var min = Math.floor(sec / 60);
    if (min < 60) return min + "m ago";
    var hr = Math.floor(min / 60);
    if (hr < 24) return hr + "h ago";
    var day = Math.floor(hr / 24);
    if (day < 30) return day + "d ago";
    return new Date(t).toLocaleDateString();
  }

  function mount(opts) {
    opts = opts || {};
    var root = opts.root;
    var onOpen = typeof opts.onOpen === "function" ? opts.onOpen : function () {};
    var Client = globalThis.ChimeraChat && ChimeraChat.HistoryClient;
    if (!root || !Client) return null;

    var embedded = !!opts.embedded;
    var filtersRoot = opts.filtersRoot || null;
    var filter = "all";
    var items = [];
    var activeId = "";
    var loading = false;

    var filtersHtml =
      '<div class="chat-history__filters" role="tablist" aria-label="History filter">' +
      '<button type="button" class="chat-history__filter chat-history__filter--active" data-filter="all" role="tab" aria-selected="true">All</button>' +
      '<button type="button" class="chat-history__filter" data-filter="flagged" role="tab" aria-selected="false">Bookmarks</button>' +
      "</div>";

    if (embedded && filtersRoot) {
      filtersRoot.innerHTML = filtersHtml;
      root.innerHTML = '<div class="chat-history__list" role="list" aria-label="Saved conversations"></div>';
    } else if (embedded) {
      root.innerHTML =
        '<div class="chat-history__head">' +
        filtersHtml +
        "</div>" +
        '<div class="chat-history__list" role="list" aria-label="Saved conversations"></div>';
    } else {
      root.innerHTML =
        '<div class="chat-history__head">' +
        '<span class="chat-history__title">History</span>' +
        filtersHtml +
        "</div>" +
        '<div class="chat-history__list" role="list" aria-label="Saved conversations"></div>';
    }

    var listEl = root.querySelector(".chat-history__list");
    var eventRoot =
      embedded && filtersRoot ? filtersRoot.closest(".shell-ribbon") || root : root;

    function syncActiveRows() {
      if (!listEl) return;
      var rows = listEl.querySelectorAll(".chat-history__row");
      for (var i = 0; i < rows.length; i++) {
        var rid = rows[i].getAttribute("data-id") || "";
        rows[i].classList.toggle("chat-history__row--active", !!activeId && rid === activeId);
      }
    }

    function render() {
      if (!listEl) return;
      var scrollTop = listEl.scrollTop;
      if (loading) {
        listEl.innerHTML = '<p class="chat-history__empty">Loading…</p>';
        listEl.scrollTop = scrollTop;
        return;
      }
      if (!items.length) {
        listEl.innerHTML =
          '<p class="chat-history__empty">' +
          (filter === "flagged" ? "No bookmarked conversations." : "No saved conversations yet.") +
          "</p>";
        listEl.scrollTop = scrollTop;
        return;
      }
      var html = "";
      for (var i = 0; i < items.length; i++) {
        var row = items[i] || {};
        var id = row.conversation_id || "";
        var title = row.title || row.preview_text || "Untitled";
        var flagged = !!row.flagged;
        var active = id && id === activeId;
        html +=
          '<article class="chat-history__row' +
          (active ? " chat-history__row--active" : "") +
          '" role="listitem" data-id="' +
          esc(id) +
          '">' +
          '<button type="button" class="chat-history__open" data-action="open" title="Open conversation">' +
          '<span class="chat-history__row-title">' +
          esc(title) +
          "</span>" +
          '<span class="chat-history__row-time">' +
          esc(relativeTime(row.updated_at)) +
          "</span>" +
          "</button>" +
          '<div class="chat-history__row-actions">' +
          '<button type="button" class="chat-history__icon-btn' +
          (flagged ? " chat-history__icon-btn--bookmarked" : "") +
          '" data-action="flag" title="' +
          (flagged ? "Remove bookmark" : "Bookmark") +
          '" aria-label="' +
          (flagged ? "Remove bookmark" : "Bookmark conversation") +
          '">' +
          '<span class="material-symbols-outlined" aria-hidden="true">bookmark_star</span></button>' +
          "</div></article>";
      }
      listEl.innerHTML = html;
      listEl.scrollTop = scrollTop;
    }

    function refresh() {
      loading = true;
      render();
      return Client.listConversations({
        limit: 100,
        flaggedOnly: filter === "flagged"
      })
        .then(function (data) {
          items = data && Array.isArray(data.conversations) ? data.conversations : [];
        })
        .catch(function (err) {
          items = [];
          console.warn("history list:", err);
        })
        .finally(function () {
          loading = false;
          render();
        });
    }

    function findRow(id) {
      for (var i = 0; i < items.length; i++) {
        if (items[i] && items[i].conversation_id === id) return items[i];
      }
      return null;
    }

    eventRoot.addEventListener("click", function (ev) {
      var t = ev.target;
      if (!t || !t.closest) return;
      var filterBtn = t.closest(".chat-history__filter");
      if (filterBtn) {
        filter = filterBtn.getAttribute("data-filter") || "all";
        var tabs = eventRoot.querySelectorAll(".chat-history__filter");
        for (var i = 0; i < tabs.length; i++) {
          var on = tabs[i] === filterBtn;
          tabs[i].classList.toggle("chat-history__filter--active", on);
          tabs[i].setAttribute("aria-selected", on ? "true" : "false");
        }
        refresh();
        return;
      }
      var row = t.closest(".chat-history__row");
      if (!row) return;
      var id = row.getAttribute("data-id") || "";
      if (!id) return;
      var actionEl = t.closest("[data-action]");
      var action = actionEl ? actionEl.getAttribute("data-action") : "";
      if (action === "open" || (!action && t.closest(".chat-history__open"))) {
        activeId = id;
        syncActiveRows();
        onOpen(id);
        return;
      }
      var item = findRow(id);
      if (action === "flag") {
        var next = !(item && item.flagged);
        Client.setFlagged(id, next)
          .then(function () {
            if (item) item.flagged = next;
            render();
          })
          .catch(function (err) {
            alert(err && err.message ? err.message : String(err));
          });
      }
    });

    refresh();

    return {
      refresh: refresh,
      setActiveId: function (id) {
        activeId = id || "";
        syncActiveRows();
      },
      clearActive: function () {
        activeId = "";
        syncActiveRows();
      }
    };
  }

  globalThis.ChimeraChat = globalThis.ChimeraChat || {};
  globalThis.ChimeraChat.HistoryPanel = { mount: mount };
})();
