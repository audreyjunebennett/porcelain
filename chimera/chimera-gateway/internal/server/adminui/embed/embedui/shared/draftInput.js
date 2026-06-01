/**
 * Draft field tracking for provider credentials on ctx.
 */
(function () {
  "use strict";

  var CS = globalThis.ChimeraShared && globalThis.ChimeraShared.ProviderCredentials;

  function providerIdFromKeyInputId(inputId) {
    var id = String(inputId || "");
    if (id.indexOf("admin-") !== 0 || id.slice(-4) !== "-key") return "";
    return id.slice("admin-".length, -"-key".length);
  }

  function applyAdminCredentialInput(ctx, target) {
    if (!ctx || !target || !target.id) return false;
    if (target.id === (CS && CS.ollamaUrlInputId ? CS.ollamaUrlInputId() : "admin-ollama-url")) {
      ctx.adminOllamaUrlDraft = target.value != null ? String(target.value) : "";
      return true;
    }
    var provKey = providerIdFromKeyInputId(target.id);
    if (provKey) {
      if (!ctx.adminProviderKeyDraft) ctx.adminProviderKeyDraft = {};
      ctx.adminProviderKeyDraft[provKey] = target.value != null ? String(target.value) : "";
      return true;
    }
    return false;
  }

  globalThis.ChimeraShared = globalThis.ChimeraShared || {};
  globalThis.ChimeraShared.DraftInput = {
    applyAdminCredentialInput: applyAdminCredentialInput,
    providerIdFromKeyInputId: providerIdFromKeyInputId
  };
})();
