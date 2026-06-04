/**
 * Workspace search UI state.
 */
(function () {
  "use strict";

  function createState() {
    return {
      selectedWorkspaceKey: "",
      scoreThreshold: 0.72,
      lastQuery: "",
      isSearching: false,
      lastResponse: null,
      lastError: ""
    };
  }

  globalThis.ChimeraSearch = globalThis.ChimeraSearch || {};
  globalThis.ChimeraSearch.State = {
    createState: createState
  };
})();
