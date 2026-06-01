/**
 * Render production card HTML into /ui/settings/gallery fixture mount points.
 * Requires the same shared + settings card modules as the live summarized feed.
 */
(function () {
  function mountGalleryCardCtx() {
    var escapeHtml = ChimeraSettings.escapeHtml;
    var ctx = {
      escapeHtml: escapeHtml,
      strHash: ChimeraSettings.strHash,
      getFlat: function (p) {
        return (p && p.rawFlat) || {};
      },
      entryCache: [],
      entryInstant: function () {
        return null;
      },
      formatInt: function (n) {
        return String(n != null ? n : 0);
      },
      RECENT_CARD_STATUS_N: 8,
      sumEvlogPanelHtml: function (o) {
        return (
          '<div class="sum-evlog sum-evlog--stub" data-gallery-evlog-stub>' +
          escapeHtml(o && o.title ? o.title : "Scoped log") +
          "</div>"
        );
      },
      sumEvlogBuildTbodyFromServiceEntries: function () {
        return "";
      },
      sumEvlogCountWarnFailFromEntries: function () {
        return { warn: 0, fail: 0 };
      },
      scopedEvlogTitle: function (t) {
        return String(t || "Scoped log");
      },
      adminVisibleProviderIds: ["groq", "ollama"],
      adminProviderKeyDraft: { groq: "gsk-gallery-demo" },
      adminOllamaUrlDraft: "http://127.0.0.1:11434",
      adminProviderModelsEditingId: null,
      adminProviderModelsDraft: {},
      adminProviderModelsCache: {},
      adminStateCache: {
        providers: {
          groq: {
            keys: [{ name: "gallery", key_configured: true }],
            ok: true,
            key_configured: true,
            models_configured: true,
            models_available_count: 2,
            models_unavailable_count: 0
          },
          ollama: {
            keys: [],
            ok: true,
            ollama_base_url: "http://127.0.0.1:11434"
          }
        },
        gateway: {
          virtual_models: [
            {
              id: 42,
              model_id: "Chimera-0.2.0",
              name: "Chimera",
              version: "0.2.0",
              description: "Gallery fixture",
              enabled: true,
              visibility: "public",
              fallback_depth: 2,
              routing_policy_enabled: true,
              tool_router_enabled: false,
              router_models: []
            }
          ]
        }
      },
      chimeraBrokerProviderSnapshot: {
        fetchedClientMs: Date.now(),
        data: { providers: [{ id: "groq", model_ids: ["groq/free", "groq/paid"] }] }
      },
      metricsCache: {
        day_rollups: [{ provider: "groq", model_id: "groq/free", calls: 12, status: 200 }]
      },
      gatewayOverviewCache: {
        semver: "1.4.2+gallery",
        virtual_model_id: "virtual/claude-opus-proxy",
        service_overview: {
          refreshed_at: new Date().toISOString(),
          services: {
            "chimera-broker": { state: "up" },
            "chimera-vectorstore": { state: "up" },
            "chimera-indexer": { worker: "idle" }
          }
        }
      },
      tokenListCache: [{ tenant_id: "tenant-a", label: "Gallery", index: 0 }],
      tokenLabelByTenant: { "tenant-a": "Gallery" },
      virtualModelDrafts: [],
      virtualModelUi: { "42": { panelOpen: true, hydrated: true } },
      virtualModelDetails: {
        "42": {
          fallback_chain: ["groq/free"],
          fallback_unavailable: [],
          routing_policy: "ambiguous_default_model: groq/free\nrules: []\n",
          router_models: ["- groq/free"]
        }
      },
      workspaceDrafts: [
        {
          id: 1,
          projectId: "acme-docs",
          flavorId: "main",
          paths: [{ path: "C:\\\\data\\\\gallery-draft" }]
        }
      ],
      workspaceManagedEditId: null,
      workspaceManagedStaging: null,
      lastIndexerOperatorWorkspacesNested: [
        {
          id: "3",
          project_id: "acme-docs",
          flavor_id: "main",
          paths: [{ id: 10, path: "C:\\\\data\\\\managed" }]
        }
      ],
      operatorWsFullLogCtx: {},
      workspaceDesktopFeaturesAvailable: function () {
        return false;
      },
      wrapDesktopOnlyLockedControl: function (html) {
        return html;
      },
      resolveLogsOperatorUserLabel: function () {
        return "Gallery operator";
      }
    };
    ChimeraSettings.Render.mountSumEvlog(ctx);
    ChimeraSettings.Render.Cards.mountAll(ctx);
    var C = ChimeraSettings.Render.Cards;
    if (typeof C.mountFeedLogConv === "function") C.mountFeedLogConv(ctx);
    if (typeof C.mountFeedLogService === "function") C.mountFeedLogService(ctx);
    if (typeof C.mountFeedLogIndexerRun === "function") C.mountFeedLogIndexerRun(ctx);
    if (typeof C.mountFeedLogIndexerWorkspace === "function") C.mountFeedLogIndexerWorkspace(ctx);
    return ctx;
  }

  function setHtml(id, html) {
    var el = document.getElementById(id);
    if (!el) return;
    el.innerHTML = html || "";
  }

  function renderFixtures() {
    if (!globalThis.ChimeraSettings || !ChimeraSettings.Render || !ChimeraSettings.Render.Cards) return;
    var ctx = mountGalleryCardCtx();
    setHtml("gallery-fixture-overview", ctx.buildGatewayOverviewCardHtml());
    setHtml(
      "gallery-fixture-provider-groq",
      ctx.buildAdminProviderCardHtml("groq", "Groq", "Gq", "LPU inference — gallery fixture")
    );
    setHtml(
      "gallery-fixture-provider-ollama",
      ctx.buildAdminProviderCardHtml("ollama", "Ollama", "Ol", "Local chat + embeddings")
    );
    if (ctx.workspaceDrafts && ctx.workspaceDrafts.length && typeof ctx.buildWorkspaceDraftCardHtml === "function") {
      setHtml("gallery-fixture-workspace-draft", ctx.buildWorkspaceDraftCardHtml(ctx.workspaceDrafts[0]));
    }
    var vmList =
      ctx.adminStateCache &&
      ctx.adminStateCache.gateway &&
      Array.isArray(ctx.adminStateCache.gateway.virtual_models)
        ? ctx.adminStateCache.gateway.virtual_models
        : [];
    if (vmList.length && typeof ctx.buildVirtualModelCardHtml === "function") {
      setHtml("gallery-fixture-virtual-model", ctx.buildVirtualModelCardHtml(vmList[0]));
    }
    if (
      ctx.lastIndexerOperatorWorkspacesNested &&
      ctx.lastIndexerOperatorWorkspacesNested[0] &&
      typeof ctx.buildIndexerOperatorWorkspaceCard === "function"
    ) {
      setHtml(
        "gallery-fixture-opws",
        ctx.buildIndexerOperatorWorkspaceCard(ctx.lastIndexerOperatorWorkspacesNested[0], {})
      );
    }
    if (typeof ctx.buildIndexerStaleSnapshotCard === "function") {
      setHtml(
        "gallery-fixture-indexer-stale",
        ctx.buildIndexerStaleSnapshotCard("gallery-stale-bucket", {
          userLabel: "Gallery",
          projectId: "acme-docs",
          flavorId: "main",
          paths: ["C:\\\\data\\\\stale-watch"]
        })
      );
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", renderFixtures);
  } else {
    renderFixtures();
  }
})();
