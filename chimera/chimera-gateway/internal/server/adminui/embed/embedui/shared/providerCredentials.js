/**
 * Provider API key add block + Ollama base URL save (reference shared implementations).
 */
(function () {
  "use strict";

  var MSG = {
    keyRequired: "Enter a key.",
    keyAdded: "Provider key added.",
    urlRequired: "Enter a URL.",
    urlSaved: "Ollama URL saved."
  };

  function providerKeyInputId(providerId) {
    return "admin-" + String(providerId || "").trim().toLowerCase() + "-key";
  }

  function ollamaUrlInputId() {
    return "admin-ollama-url";
  }

  function keyPlaceholderForProvider(providerId) {
    var placeholder = "API key";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog &&
      typeof ChimeraSettings.Providers.Catalog.lookupProviderSpec === "function"
    ) {
      var catSpec = ChimeraSettings.Providers.Catalog.lookupProviderSpec(providerId);
      if (catSpec && catSpec.key_placeholder) placeholder = String(catSpec.key_placeholder);
    }
    return placeholder;
  }

  function keyAddBlockHtml(escapeHtml, ctx, providerId) {
    var esc = escapeHtml || function (s) { return String(s); };
    var keyDrafts = (ctx && ctx.adminProviderKeyDraft) || {};
    var keyDraftVal = keyDrafts[providerId] != null ? String(keyDrafts[providerId]) : "";
    return (
      '<div class="sg-op-provider-key-add">' +
      '<div class="sg-op-provider-key-add__label sum-section-label">Add new key</div>' +
      '<div class="sg-op-provider-key-add__row">' +
      '<input id="' +
      esc(providerKeyInputId(providerId)) +
      '" class="sg-op-input sg-op-provider-key-add__input" type="password" placeholder="' +
      esc(keyPlaceholderForProvider(providerId)) +
      '" value="' +
      esc(keyDraftVal) +
      '"/>' +
      '<button type="button" class="sg-op-provider-key-add-btn" data-admin-action="provider-key-add" data-provider="' +
      esc(providerId) +
      '">Add</button></div></div>'
    );
  }

  function clearKeyDraft(ctx, providerId) {
    if (!ctx || !providerId) return;
    var inp = document.getElementById(providerKeyInputId(providerId));
    if (inp) inp.value = "";
    if (ctx.adminProviderKeyDraft && ctx.adminProviderKeyDraft[providerId] != null) {
      delete ctx.adminProviderKeyDraft[providerId];
    }
  }

  function runProviderKeyAdd(opts) {
    opts = opts || {};
    var prov = String(opts.providerId || "").trim().toLowerCase();
    if (!prov) return;
    var inputId = providerKeyInputId(prov);
    var val = inputId ? String(((document.getElementById(inputId) || {}).value || "")).trim() : "";
    var setMessage = opts.setMessage;
    if (!val) {
      if (setMessage) setMessage("err", MSG.keyRequired);
      return;
    }
    var setPending = opts.setPending;
    if (setPending) setPending(opts.triggerBtn, true);
    var postJSON = opts.postJSON;
    if (!postJSON) return;
    postJSON("/api/ui/provider/" + prov + "/keys", { value: val })
      .then(function () {
        if (opts.ctx) clearKeyDraft(opts.ctx, prov);
        if (setMessage) setMessage("", MSG.keyAdded);
        if (typeof opts.onSuccess === "function") opts.onSuccess();
      })
      .catch(function (e) {
        if (setPending) setPending(opts.triggerBtn, false);
        if (setMessage) setMessage("err", e && e.message ? e.message : String(e));
      });
  }

  function runOllamaUrlSave(opts) {
    opts = opts || {};
    var inputId = ollamaUrlInputId();
    var baseURL = String(((document.getElementById(inputId) || {}).value || "")).trim();
    var setMessage = opts.setMessage;
    if (!baseURL) {
      if (setMessage) setMessage("err", MSG.urlRequired);
      return;
    }
    var setPending = opts.setPending;
    if (setPending) setPending(opts.triggerBtn, true);
    var postJSON = opts.postJSON;
    if (!postJSON) return;
    postJSON("/api/ui/provider/ollama/base_url", { base_url: baseURL })
      .then(function () {
        if (opts.ctx) opts.ctx.adminOllamaUrlDraft = null;
        if (setMessage) setMessage("", MSG.urlSaved);
        if (typeof opts.onSuccess === "function") opts.onSuccess();
      })
      .catch(function (e) {
        if (setPending) setPending(opts.triggerBtn, false);
        if (setMessage) setMessage("err", e && e.message ? e.message : String(e));
      });
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.ProviderCredentials = {
    MSG: MSG,
    providerKeyInputId: providerKeyInputId,
    ollamaUrlInputId: ollamaUrlInputId,
    keyAddBlockHtml: keyAddBlockHtml,
    clearKeyDraft: clearKeyDraft,
    runProviderKeyAdd: runProviderKeyAdd,
    runOllamaUrlSave: runOllamaUrlSave
  };
})();
