/**
 * Operator settings UI constants — generated from internal/naming (gateway_logs.go, contracts.go, logs_ui.go).
 * DO NOT EDIT; run: make operator-contracts-generate
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Contracts = {
    ProductGateway: "chimera-gateway",
    ProductBroker: "chimera-broker",
    ProductVectorstore: "chimera-vectorstore",
    ProductIndexer: "chimera-indexer",
    ProductSupervisor: "chimera-supervisor",

    LogSourceChimeraGateway: "chimera-gateway",
    LogSourceChimeraBroker: "chimera-broker",
    LogSourceChimeraVectorstore: "chimera-vectorstore",
    LogSourceChimeraIndexer: "chimera-indexer",
    LogSourceChimeraSupervisor: "chimera-supervisor",

    TimelineKindWeb: "web",
    TimelineKindBroker: "broker",
    TimelineKindVectorstore: "vectorstore",
    TimelineKindIndexer: "indexer",
    TimelineKindGateway: "gateway",

    /** Request-timeline bar keys (product display names). */
    TimelineBarKinds: [
      { key: "chimera-gateway", label: "chimera-gateway" },
      { key: "chimera-broker", label: "chimera-broker" },
      { key: "chimera-vectorstore", label: "chimera-vectorstore" },
      { key: "chimera-indexer", label: "chimera-indexer" },
      { key: "web", label: "web" },
    ],

    serviceBadgeClass: function (productKey) {
      var k = String(productKey || "").toLowerCase();
      if (k === "chimera-broker" || k === "broker") return "sum-svc-broker";
      if (k === "chimera-vectorstore" || k === "vectorstore") return "sum-svc-vectorstore";
      if (k === "chimera-indexer" || k === "indexer") return "sum-svc-indexer";
      if (k === "chimera-gateway" || k === "gateway") return "sum-svc-gateway";
      if (k === "web") return "sum-svc-web";
      return "sum-svc-gateway";
    }
  };
})();
