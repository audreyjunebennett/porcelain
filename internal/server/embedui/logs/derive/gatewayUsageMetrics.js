/**
 * Pure derivations for the Gateway usage metrics card (data from /api/ui/metrics).
 *
 * Exports:
 * - ClaudiaLogs.Derive.gatewayUsageCardModel(data, aggregateRollupRows, bifrostShortModelLabel)
 */

function gatewayUsageCardModel(data, aggregateRollupRows, bifrostShortModelLabel) {
  var loading = !data;
  var storeOpen = !!(data && data.metrics_store_open);

  var minAgg = { models: 0, tokens: 0 };
  var dayAgg = { models: 0, tokens: 0 };

  if (data && data.minute_rollups && typeof aggregateRollupRows === "function")
    minAgg = aggregateRollupRows(data.minute_rollups);
  if (data && data.day_rollups && typeof aggregateRollupRows === "function")
    dayAgg = aggregateRollupRows(data.day_rollups);

  var lastModelId = "—";
  if (data && data.recent_events && data.recent_events.length && data.recent_events[0].model_id)
    lastModelId = String(data.recent_events[0].model_id);

  var lastModelLabel =
    typeof bifrostShortModelLabel === "function" ? bifrostShortModelLabel(lastModelId) : lastModelId;

  var lblMin = data && data.current_minute_utc ? String(data.current_minute_utc) : "";
  var lblDay = data && data.current_day_utc ? String(data.current_day_utc) : "";

  return {
    loading: loading,
    storeOpen: storeOpen,
    lastModelId: lastModelId,
    lastModelLabel: lastModelLabel,
    lblMin: lblMin,
    lblDay: lblDay,
    minAgg: minAgg,
    dayAgg: dayAgg,
    message: data && data.message ? String(data.message) : ""
  };
}

globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
globalThis.ClaudiaLogs.Derive = globalThis.ClaudiaLogs.Derive || {};
globalThis.ClaudiaLogs.Derive.gatewayUsageCardModel = gatewayUsageCardModel;

