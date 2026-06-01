/**
 * Summarized feed polling: metrics, UI state, chimera-broker provider snapshot.
 * Exports: ChimeraSettings.Summarized.mountPolling(bridge, patch)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

globalThis.ChimeraSettings.Summarized.mountPolling = function (bridge, patch) {
  var ctx = bridge.ctx;
  var getViewMode = bridge.getViewMode;
  var statusEl = bridge.statusEl;
  var embedded = bridge.embedded;
  var getFlat = bridge.getFlat;
  var entryCache = bridge.entryCache;
  var adminVisibleProviderIds = bridge.adminVisibleProviderIds;

  var metricsPollTimer = null;
  var METRICS_POLL_MS = 12000;
  var uiStatePollTimer = null;
  var UI_STATE_POLL_MS = 60000;
  var chimeraBrokerProviderPollTimer = null;
  var CHIMERA_BROKER_PROVIDER_POLL_MS = 30000;
  var CHIMERA_BROKER_PROVIDER_STALE_MS = bridge.CHIMERA_BROKER_PROVIDER_STALE_MS || 90000;

  function stopSummarizedPolling() {
    if (metricsPollTimer) {
      try {
        clearInterval(metricsPollTimer);
      } catch (_eM) {}
      metricsPollTimer = null;
    }
    if (uiStatePollTimer) {
      try {
        clearInterval(uiStatePollTimer);
      } catch (_eU) {}
      uiStatePollTimer = null;
    }
    if (chimeraBrokerProviderPollTimer) {
      try {
        clearInterval(chimeraBrokerProviderPollTimer);
      } catch (_eB) {}
      chimeraBrokerProviderPollTimer = null;
    }
  }

  function markUiUnauthorized(msg) {
    if (ctx.uiUnauthorized) return;
    ctx.uiUnauthorized = true;
    stopSummarizedPolling();
    if (typeof ctx.stopLogsTransport === "function") ctx.stopLogsTransport();
    var text = msg || (embedded ? "Unauthorized — sign in from the shell" : "Unauthorized — sign in");
    if (statusEl) {
      statusEl.textContent = text;
      statusEl.className = "status-line err";
    }
    if (!embedded) {
      try {
        var next = window.location.pathname + window.location.search;
        window.location.replace("/ui/login?next=" + encodeURIComponent(next));
      } catch (_eLogin) {}
    }
  }

  function fetchGatewayMetrics() {
    if (ctx.uiUnauthorized) return;
    fetch("/api/ui/metrics?limit=150", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        ctx.metricsCache = data;
        if (getViewMode() === "summarized") patch.patchGatewayUsageMetricsCard();
      })
      .catch(function (e) {
        ctx.metricsCache = {
          metrics_store_open: false,
          message: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patch.patchGatewayUsageMetricsCard();
      });
  }

  function syncMetricsPolling() {
    if (metricsPollTimer) {
      try {
        clearInterval(metricsPollTimer);
      } catch (x) {}
      metricsPollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    fetchGatewayMetrics();
    metricsPollTimer = setInterval(fetchGatewayMetrics, METRICS_POLL_MS);
  }

  function fetchUiState() {
    if (ctx.uiUnauthorized) return Promise.resolve(null);
    return fetch("/api/ui/state", { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (j) {
        if (!j) return null;
        ctx.adminStateCache = j;
        if (j.gateway) ctx.gatewayOverviewCache = j.gateway;
        return j;
      });
  }

  function fetchGatewayOverview() {
    if (ctx.uiUnauthorized) return;
    fetchUiState()
      .then(function (data) {
        if (!data || !data.gateway) return;
        if (getViewMode() === "summarized") patch.patchGatewayOverviewCard();
      })
      .catch(function (e) {
        ctx.gatewayOverviewCache = {
          _error: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patch.patchGatewayOverviewCard();
      });
  }

  function resyncVisibleProvidersFromCatalog() {
    if (adminVisibleProviderIds().length > 0) return Promise.resolve();
    var api = bridge.providerCatalogApi();
    if (!api) return Promise.resolve();
    return api
      .fetchProviderCatalog(ctx, { force: true })
      .then(function (data) {
        if (!data || getViewMode() !== "summarized") return;
        if (adminVisibleProviderIds().length > 0) bridge.scheduleStoryRebuild();
      })
      .catch(function () {
        return null;
      });
  }

  function runUiStatePoll(opts) {
    if (ctx.uiUnauthorized) return Promise.resolve();
    var showErr = opts && opts.showErr;
    return Promise.all([fetchUiState(), bridge.fetchAdminTokens()])
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        return resyncVisibleProvidersFromCatalog();
      })
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        return bridge.prefetchProviderModelsAvailability();
      })
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        patch.patchGatewayOverviewCard();
        patch.patchAdminCardsFromPoll();
      })
      .catch(function (e) {
        ctx.gatewayOverviewCache = {
          _error: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patch.patchGatewayOverviewCard();
        if (showErr && !ctx.uiUnauthorized && typeof bridge.adminSetMessage === "function") {
          bridge.adminSetMessage("err", e && e.message ? e.message : String(e));
        }
      });
  }

  function syncUiStatePolling() {
    if (uiStatePollTimer) {
      try {
        clearInterval(uiStatePollTimer);
      } catch (x) {}
      uiStatePollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    runUiStatePoll({ showErr: true });
    uiStatePollTimer = setInterval(function () {
      runUiStatePoll({ showErr: false });
    }, UI_STATE_POLL_MS);
  }

  function fetchChimeraBrokerProviderSnapshot() {
    if (ctx.uiUnauthorized) return;
    fetch("/api/ui/chimera-broker/providers", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        ctx.chimeraBrokerProviderSnapshot = { fetchedClientMs: Date.now(), data: data };
        if (getViewMode() === "summarized") patchChimeraBrokerProviderUiFromSnapshot();
      })
      .catch(function () {
        // Keep any prior snapshot — staleness check in the renderer handles fallback.
      });
  }

  function syncChimeraBrokerProviderPolling() {
    if (chimeraBrokerProviderPollTimer) {
      try {
        clearInterval(chimeraBrokerProviderPollTimer);
      } catch (x) {}
      chimeraBrokerProviderPollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    fetchChimeraBrokerProviderSnapshot();
    chimeraBrokerProviderPollTimer = setInterval(
      fetchChimeraBrokerProviderSnapshot,
      CHIMERA_BROKER_PROVIDER_POLL_MS
    );
  }

  function collectChimeraBrokerBufferForStrip() {
    var out = [];
    var entryRoutes = bridge.entryRoutesToChimeraBrokerBucket;
    for (var i = 0; i < entryCache.length; i++) {
      var e = entryCache[i];
      if (!e || !e.parsed) continue;
      var f = getFlat(e.parsed);
      var svc = f.service ? String(f.service) : "";
      var isChimeraBroker =
        svc === "chimera-broker" ||
        e.source === "chimera-broker" ||
        (typeof entryRoutes === "function" && entryRoutes(e));
      if (isChimeraBroker) out.push(e);
    }
    return out;
  }

  function patchChimeraBrokerProviderHealthStrip() {
    if (getViewMode() !== "summarized") return;
    var arr = collectChimeraBrokerBufferForStrip();
    var oldEl = document.getElementById("chimera-broker-provider-health-strip");
    if (oldEl) {
      var wrap = document.createElement("div");
      wrap.innerHTML = bridge.chimeraBrokerProviderHealthStripHtml(arr).trim();
      var newEl = wrap.firstElementChild;
      if (newEl && newEl.id === "chimera-broker-provider-health-strip") {
        oldEl.parentNode.replaceChild(newEl, oldEl);
      }
    }
    var compactOld = document.getElementById("chimera-broker-provider-health-compact");
    if (compactOld) {
      var w2 = document.createElement("div");
      w2.innerHTML = bridge.chimeraBrokerProviderHealthStripHtml(arr, { compact: true }).trim();
      var n2 = w2.firstElementChild;
      if (n2 && n2.id === "chimera-broker-provider-health-compact") {
        compactOld.parentNode.replaceChild(n2, compactOld);
      }
    }
  }

  function patchChimeraBrokerAvailableModelsCount() {
    if (getViewMode() !== "summarized") return;
    var el = document.getElementById("chimera-broker-available-models-count");
    if (!el) return;
    if (typeof bridge.chimeraBrokerAvailableModelCountLabel !== "function") return;
    el.textContent = bridge.chimeraBrokerAvailableModelCountLabel(collectChimeraBrokerBufferForStrip());
  }

  function patchChimeraBrokerProviderUiFromSnapshot() {
    if (getViewMode() !== "summarized") return;
    patchChimeraBrokerProviderHealthStrip();
    patchChimeraBrokerAvailableModelsCount();
    var needRebuild = false;
    var visibleForBroker = adminVisibleProviderIds();
    for (var pi = 0; pi < visibleForBroker.length; pi++) {
      if (!patch.patchAdminProviderCard(visibleForBroker[pi])) needRebuild = true;
    }
    if (needRebuild) bridge.refreshSummarizedPanel();
  }

  function chimeraBrokerProviderSnapshotDataForUi() {
    if (!ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data) return null;
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > CHIMERA_BROKER_PROVIDER_STALE_MS) return null;
    return ctx.chimeraBrokerProviderSnapshot.data;
  }

  ctx.CHIMERA_BROKER_PROVIDER_STALE_MS = CHIMERA_BROKER_PROVIDER_STALE_MS;

  return {
    stopSummarizedPolling: stopSummarizedPolling,
    markUiUnauthorized: markUiUnauthorized,
    fetchGatewayMetrics: fetchGatewayMetrics,
    syncMetricsPolling: syncMetricsPolling,
    fetchUiState: fetchUiState,
    fetchGatewayOverview: fetchGatewayOverview,
    runUiStatePoll: runUiStatePoll,
    syncUiStatePolling: syncUiStatePolling,
    fetchChimeraBrokerProviderSnapshot: fetchChimeraBrokerProviderSnapshot,
    syncChimeraBrokerProviderPolling: syncChimeraBrokerProviderPolling,
    patchChimeraBrokerProviderHealthStrip: patchChimeraBrokerProviderHealthStrip,
    patchChimeraBrokerProviderUiFromSnapshot: patchChimeraBrokerProviderUiFromSnapshot,
    chimeraBrokerProviderSnapshotDataForUi: chimeraBrokerProviderSnapshotDataForUi,
    patchChimeraBrokerAvailableModelsCount: patchChimeraBrokerAvailableModelsCount,
    collectChimeraBrokerBufferForStrip: collectChimeraBrokerBufferForStrip
  };
};
