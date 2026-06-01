/**
 * Conversation card helpers; full buildConvCard is in feedLogConv.js.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountConvCard = function (ctx) {
  function formatMergedConversationSubtitle(mergedCount) {
    if (!mergedCount || mergedCount <= 1) return "";
    return (
      ' <span class="muted" style="font-size:0.85em" title="Multiple conversation ids in one card (unusual).">(' +
      mergedCount +
      " ids)</span>"
    );
  }

  ctx.formatMergedConversationSubtitle = formatMergedConversationSubtitle;
};
