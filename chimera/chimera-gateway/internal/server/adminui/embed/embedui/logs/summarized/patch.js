/**
 * Summarized feed patch engine (Phase 5): model diff and DOM apply without panel innerHTML.
 */
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Summarized = globalThis.ChimeraLogs.Summarized || {};
globalThis.ChimeraLogs.Summarized.Patch = globalThis.ChimeraLogs.Summarized.Patch || {};

(function () {
  function feedStructureKey(model) {
    if (!model) return "";
    var parts = [];
    var cards = model.cards || [];
    for (var i = 0; i < cards.length; i++) {
      var c = cards[i];
      if (c.kind === "section-break") {
        parts.push("sb:" + c.id + ":" + c.hash);
      } else {
        parts.push("c:" + c.id);
      }
    }
    var meta = model.meta || {};
    parts.push("hasThreads:" + (meta.hasThreads ? "1" : "0"));
    return parts.join("|");
  }

  function indexCardsById(model) {
    var map = Object.create(null);
    if (!model || !model.cards) return map;
    for (var i = 0; i < model.cards.length; i++) {
      var c = model.cards[i];
      if (c && c.id && c.kind !== "section-break") map[c.id] = c;
    }
    return map;
  }

  /**
   * @param {{ cards: object[], meta: object }} prev
   * @param {{ cards: object[], meta: object }} next
   * @param {{ skipCardIds?: object, onlyCardIds?: object }} [options]
   * @returns {object[]}
   */
  function diffSummarizedModels(prev, next, options) {
    options = options || {};
    var skipCardIds = options.skipCardIds || Object.create(null);
    var onlyCardIds = options.onlyCardIds || null;

    if (!next || !next.cards) return [{ op: "replaceFeed", reason: "missing-next" }];
    if (!prev || !prev.cards) return [{ op: "replaceFeed", reason: "missing-prev" }];
    if (feedStructureKey(prev) !== feedStructureKey(next)) {
      return [{ op: "replaceFeed", reason: "structure" }];
    }

    var prevById = indexCardsById(prev);
    var ops = [];
    var cards = next.cards;
    for (var i = 0; i < cards.length; i++) {
      var card = cards[i];
      if (!card || card.kind === "section-break" || !card.id) continue;
      if (onlyCardIds && !onlyCardIds[card.id]) continue;
      if (skipCardIds[card.id]) continue;
      var prevCard = prevById[card.id];
      if (!prevCard) return [{ op: "replaceFeed", reason: "card-added" }];
      if (prevCard.hash === card.hash) continue;
      ops.push({
        op: "replaceCard",
        id: card.id,
        card: card,
        prevHash: prevCard.hash,
        nextHash: card.hash
      });
    }
    return ops;
  }

  function shouldUseFullRebuildFromOps(ops) {
    if (!ops || !ops.length) return false;
    for (var i = 0; i < ops.length; i++) {
      if (ops[i].op === "replaceFeed") return true;
    }
    return false;
  }

  function countReplaceCardOps(ops) {
    var n = 0;
    if (!ops) return n;
    for (var i = 0; i < ops.length; i++) {
      if (ops[i].op === "replaceCard") n++;
    }
    return n;
  }

  /**
   * @param {HTMLElement} container
   * @param {object[]} ops
   * @param {{ renderCard?: function }} renderers
   * @param {{ replaceCard?: function, preserveScrollSelectors?: string, cardVersionAttr?: boolean }} [options]
   */
  function applySummarizedPatches(container, ops, renderers, options) {
    options = options || {};
    renderers = renderers || {};
    if (!container || !ops || !ops.length) {
      return { ok: true, needsFullRebuild: false, applied: 0, failed: 0 };
    }
    if (shouldUseFullRebuildFromOps(ops)) {
      return { ok: false, needsFullRebuild: true, applied: 0, failed: 0 };
    }

    var replaceCardFn = options.replaceCard;
    var scrollSel =
      options.preserveScrollSelectors ||
      ".sum-metrics-table-wrap, .sum-full-log--evlog .sum-evlog-table-wrap";
    var useVersionAttr = options.cardVersionAttr !== false;
    var applied = 0;
    var failed = 0;

    for (var i = 0; i < ops.length; i++) {
      var op = ops[i];
      if (op.op === "replaceFeed") {
        return { ok: false, needsFullRebuild: true, applied: applied, failed: failed };
      }
      if (op.op !== "replaceCard") continue;
      if (!replaceCardFn || typeof renderers.renderCard !== "function") {
        failed++;
        continue;
      }
      var html = renderers.renderCard(op.card);
      if (!html) {
        failed++;
        continue;
      }
      var ok = replaceCardFn(op.id, html, {
        preserveOpen: true,
        preserveScrollSelectors: scrollSel,
        cardHash: op.nextHash,
        cardVersionAttr: useVersionAttr
      });
      if (ok) applied++;
      else failed++;
    }

    return {
      ok: failed === 0,
      needsFullRebuild: failed > 0,
      applied: applied,
      failed: failed
    };
  }

  globalThis.ChimeraLogs.Summarized.Patch.diffSummarizedModels = diffSummarizedModels;
  globalThis.ChimeraLogs.Summarized.Patch.shouldUseFullRebuildFromOps = shouldUseFullRebuildFromOps;
  globalThis.ChimeraLogs.Summarized.Patch.countReplaceCardOps = countReplaceCardOps;
  globalThis.ChimeraLogs.Summarized.Patch.applySummarizedPatches = applySummarizedPatches;
})();
