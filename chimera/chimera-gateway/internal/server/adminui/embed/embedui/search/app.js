/**
 * Workspace search application orchestrator.
 */
(function () {
  "use strict";

  var State = globalThis.ChimeraSearch.State;
  var Gateway = globalThis.ChimeraSearch.Gateway;
  var Results = globalThis.ChimeraSearch.Render.Results;
  var InputRender = globalThis.ChimeraChat && ChimeraChat.Render && ChimeraChat.Render.Input;

  var state = State.createState();
  var workspaces = [];
  var workspaceByKey = {};

  var resultsEl = document.getElementById("search-results");
  var workspaceSel = document.getElementById("search-workspace");
  var thresholdEl = document.getElementById("search-threshold");
  var thresholdSliderEl = document.getElementById("search-threshold-slider");
  var queryEl = document.getElementById("search-query");
  var submitBtn = document.getElementById("search-submit");

  var composer =
    InputRender && queryEl
      ? InputRender.mount({
          textarea: queryEl,
          onSubmit: runSearch,
          getHistory: function () {
            return [];
          }
        })
      : {
          focus: function () {
            if (queryEl) queryEl.focus();
          },
          clear: function () {},
          getValue: function () {
            return queryEl ? String(queryEl.value || "") : "";
          }
        };

  function paint() {
    if (!resultsEl || !Results) return;
    resultsEl.innerHTML = Results.renderResultsView(state);
  }

  function setSearching(active) {
    state.isSearching = !!active;
    if (submitBtn) submitBtn.disabled = active;
    if (queryEl) queryEl.disabled = active;
    if (workspaceSel) workspaceSel.disabled = active;
    if (thresholdEl) thresholdEl.disabled = active;
    if (thresholdSliderEl) thresholdSliderEl.disabled = active;
    paint();
  }

  function parseThresholdPercent(raw) {
    var s = String(raw == null ? "" : raw).trim().replace(/%/g, "");
    if (!s) return null;
    var n = parseInt(s, 10);
    if (isNaN(n)) return null;
    return Math.min(100, Math.max(0, n));
  }

  function formatThresholdPercent(percent) {
    return String(percent) + "%";
  }

  function fractionToPercent(fraction) {
    return Math.min(100, Math.max(0, Math.round(Number(fraction) * 100)));
  }

  function percentToFraction(percent) {
    return Math.min(1, Math.max(0, Number(percent) / 100));
  }

  function setThresholdUI(percent) {
    var p = Math.min(100, Math.max(0, Math.round(Number(percent))));
    state.scoreThreshold = percentToFraction(p);
    if (thresholdEl) thresholdEl.value = formatThresholdPercent(p);
    if (thresholdSliderEl) thresholdSliderEl.value = String(p);
  }

  function syncThresholdFromInput() {
    if (!thresholdEl) return state.scoreThreshold;
    var p = parseThresholdPercent(thresholdEl.value);
    if (p == null) {
      setThresholdUI(fractionToPercent(state.scoreThreshold));
      return state.scoreThreshold;
    }
    setThresholdUI(p);
    return state.scoreThreshold;
  }

  function nudgeThreshold(delta) {
    var current = parseThresholdPercent(thresholdEl && thresholdEl.value);
    if (current == null) current = fractionToPercent(state.scoreThreshold);
    setThresholdUI(current + delta);
  }

  function flashQueryHighlight() {
    if (!queryEl) return;
    queryEl.classList.add("search-query--highlight");
    window.setTimeout(function () {
      queryEl.classList.remove("search-query--highlight");
    }, 1800);
  }

  function selectedWorkspace() {
    var key = state.selectedWorkspaceKey || "";
    if (!key) return null;
    return workspaceByKey[key] || null;
  }

  function parseThreshold() {
    return syncThresholdFromInput();
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
    var placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.disabled = true;
    placeholder.textContent = workspaces.length ? "Select workspace…" : "No workspaces configured";
    placeholder.selected = !prev;
    workspaceSel.appendChild(placeholder);
    for (var j = 0; j < workspaces.length; j++) {
      var ws = workspaces[j];
      var opt = document.createElement("option");
      opt.value = Gateway.workspaceKey(ws);
      opt.textContent = Gateway.workspaceLabel(ws);
      workspaceSel.appendChild(opt);
    }
    if (prev && workspaceByKey[prev]) {
      workspaceSel.value = prev;
      placeholder.selected = false;
    } else {
      state.selectedWorkspaceKey = "";
      workspaceSel.value = "";
    }
  }

  function refreshWorkspaces() {
    Gateway.fetchWorkspaces()
      .then(populateWorkspaces)
      .catch(function (err) {
        console.warn("search workspaces refresh:", err);
      });
  }

  function runSearch() {
    if (state.isSearching) return;
    var ws = selectedWorkspace();
    if (!ws) {
      state.lastError = "Select a workspace before searching.";
      state.lastResponse = null;
      paint();
      if (workspaceSel) workspaceSel.focus();
      return;
    }
    var query = composer.getValue().trim();
    if (!query) {
      state.lastError = "";
      state.lastResponse = { hits: [] };
      state.lastQuery = "";
      paint();
      return;
    }

    state.lastError = "";
    state.lastQuery = query;
    flashQueryHighlight();
    setSearching(true);

    var threshold = parseThreshold();
    state.scoreThreshold = threshold;

    if (state.abortController) {
      try {
        state.abortController.abort();
      } catch (_e) {}
    }
    state.abortController = new AbortController();

    Gateway.search({
      query: query,
      project_id: ws.project_id,
      flavor_id: ws.flavor_id || "",
      score_threshold: threshold,
      signal: state.abortController.signal
    })
      .then(function (resp) {
        state.lastResponse = resp || { hits: [] };
        if (resp && resp.score_threshold != null) {
          setThresholdUI(fractionToPercent(resp.score_threshold));
        }
      })
      .catch(function (err) {
        if (err && err.name === "AbortError") return;
        state.lastResponse = null;
        state.lastError = err && err.message ? err.message : String(err);
      })
      .finally(function () {
        state.abortController = null;
        setSearching(false);
        composer.focus();
      });
  }

  if (workspaceSel) {
    workspaceSel.addEventListener("change", function () {
      state.selectedWorkspaceKey = workspaceSel.value || "";
      state.lastError = "";
      paint();
    });
  }
  if (thresholdEl) {
    thresholdEl.addEventListener("change", syncThresholdFromInput);
    thresholdEl.addEventListener("blur", syncThresholdFromInput);
    thresholdEl.addEventListener("keydown", function (ev) {
      if (ev.key === "ArrowUp") {
        ev.preventDefault();
        nudgeThreshold(1);
      } else if (ev.key === "ArrowDown") {
        ev.preventDefault();
        nudgeThreshold(-1);
      }
    });
  }
  if (thresholdSliderEl) {
    thresholdSliderEl.addEventListener("input", function () {
      setThresholdUI(Number(thresholdSliderEl.value));
    });
  }
  if (submitBtn) {
    submitBtn.addEventListener("click", runSearch);
  }

  window.addEventListener("focus", refreshWorkspaces);
  setInterval(refreshWorkspaces, 30000);

  refreshWorkspaces();
  syncThresholdFromInput();
  paint();
  composer.focus();
})();
