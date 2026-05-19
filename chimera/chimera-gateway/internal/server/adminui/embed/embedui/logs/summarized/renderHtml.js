/**
 * Render summarized view model to feed HTML (Phase 4).
 */
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Summarized = globalThis.ChimeraLogs.Summarized || {};
globalThis.ChimeraLogs.Summarized.Render = globalThis.ChimeraLogs.Summarized.Render || {};

(function () {
  var SECTION_OVERVIEW = "overview";
  var SECTION_CONVERSATIONS = "conversations";
  var SECTION_WORKSPACES = "workspaces";
  var SECTION_SERVICES = "services";

  function cardsInSection(model, sectionId) {
    var out = [];
    if (!model || !model.cards) return out;
    for (var i = 0; i < model.cards.length; i++) {
      if (model.cards[i].section === sectionId) out.push(model.cards[i]);
    }
    return out;
  }

  function sortCards(cards, numericDesc) {
    return cards.slice().sort(function (a, b) {
      if (numericDesc && typeof a.sortKey === "number" && typeof b.sortKey === "number") {
        return b.sortKey - a.sortKey;
      }
      var ka = a.sortKey != null ? String(a.sortKey) : "";
      var kb = b.sortKey != null ? String(b.sortKey) : "";
      return ka.localeCompare(kb, undefined, { sensitivity: "base", numeric: true });
    });
  }

  function renderCardList(cards, renderers) {
    var html = "";
    for (var i = 0; i < cards.length; i++) {
      var card = cards[i];
      if (card.kind === "section-break") {
        html += card.summary && card.summary.html ? card.summary.html : "";
        continue;
      }
      if (renderers.renderCard) {
        var piece = renderers.renderCard(card);
        if (piece) html += piece;
      }
    }
    return html;
  }

  /**
   * @param {{ cards: object[], meta: object }} model
   * @param {object} renderers
   */
  function renderSummarizedHtml(model, renderers) {
    renderers = renderers || {};
    var meta = model && model.meta ? model.meta : {};
    var body = "";

    var overviewCards = cardsInSection(model, SECTION_OVERVIEW);
    if (overviewCards.length) {
      body += '<div class="sum-feed-section">' + renderCardList(overviewCards, renderers) + "</div>";
    }

    var convCards = sortCards(cardsInSection(model, SECTION_CONVERSATIONS), true);
    if (convCards.length) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Conversations</div>' +
        renderCardList(convCards, renderers) +
        "</div>";
    }

    var wsCards = sortCards(cardsInSection(model, SECTION_WORKSPACES), false);
    body +=
      '<div class="sum-feed-section sum-feed-section--workspaces">' +
      '<div class="sum-feed-section-head">' +
      '<span class="sum-feed-section-title sum-section-label">Workspaces</span>' +
      '<button type="button" class="sum-workspaces-create-btn" data-sum-workspaces-create="1">Create</button>' +
      "</div>";
    if (renderers.workspacesSectionIntro) body += renderers.workspacesSectionIntro();
    body += renderCardList(wsCards, renderers);
    body += "</div>";

    var svcCards = cardsInSection(model, SECTION_SERVICES);
    if (svcCards.length) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Services</div>' +
        renderCardList(svcCards, renderers) +
        "</div>";
    }

    if (!meta.hasThreads && renderers.emptyFeedMessage) {
      body += renderers.emptyFeedMessage();
    }
    return body;
  }

  function findCardById(model, cardId) {
    if (!model || !model.cards || !cardId) return null;
    for (var i = 0; i < model.cards.length; i++) {
      if (model.cards[i].id === cardId) return model.cards[i];
    }
    return null;
  }

  globalThis.ChimeraLogs.Summarized.Render.renderSummarizedHtml = renderSummarizedHtml;
  globalThis.ChimeraLogs.Summarized.Render.findCardById = findCardById;
  globalThis.ChimeraLogs.Summarized.Render.SECTION_OVERVIEW = SECTION_OVERVIEW;
  globalThis.ChimeraLogs.Summarized.Render.SECTION_CONVERSATIONS = SECTION_CONVERSATIONS;
  globalThis.ChimeraLogs.Summarized.Render.SECTION_WORKSPACES = SECTION_WORKSPACES;
  globalThis.ChimeraLogs.Summarized.Render.SECTION_SERVICES = SECTION_SERVICES;
})();
