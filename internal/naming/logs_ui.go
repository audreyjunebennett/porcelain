package naming

// Logs UI contract metadata for contracts.js codegen.
// String values must stay aligned with gateway_logs.go and contracts.go product bin names.

// LogsUIStringConst is one string constant exported to contracts.js.
type LogsUIStringConst struct {
	JSName string
	Value  string
}

// LogsUIProductNames are product keys shown in the operator logs shell.
var LogsUIProductNames = []LogsUIStringConst{
	{"ProductGateway", ProductGatewayBinName},
	{"ProductBroker", ProductBrokerName},
	{"ProductVectorstore", ProductVectorstoreName},
	{"ProductIndexer", ProductIndexerBinName},
	{"ProductSupervisor", ProductSupervisorName},
}

// LogsUILogSources mirror servicelogs ring-buffer source IDs (gateway_logs.go).
var LogsUILogSources = []LogsUIStringConst{
	{"LogSourceChimeraGateway", LogSourceChimeraGateway},
	{"LogSourceChimeraBroker", LogSourceChimeraBroker},
	{"LogSourceChimeraVectorstore", LogSourceChimeraVectorstore},
	{"LogSourceChimeraIndexer", LogSourceChimeraIndexer},
	{"LogSourceChimeraSupervisor", LogSourceChimeraSupervisor},
}

// LogsUITimelineKinds are JSON timeline_kind slugs on structured gateway logs.
var LogsUITimelineKinds = []LogsUIStringConst{
	{"TimelineKindWeb", TimelineKindWeb},
	{"TimelineKindBroker", TimelineKindBroker},
	{"TimelineKindVectorstore", TimelineKindVectorstore},
	{"TimelineKindIndexer", TimelineKindIndexer},
	{"TimelineKindGateway", TimelineKindGateway},
}

// LogsUITimelineBarKind is one segment on the request-timeline bar (key + display label).
type LogsUITimelineBarKind struct {
	Key   string
	Label string
}

// LogsUITimelineBarKinds order matches the summarized gateway card timeline bar.
// Keys use log source IDs for services and timeline_kind slugs for web traffic.
var LogsUITimelineBarKinds = []LogsUITimelineBarKind{
	{LogSourceChimeraGateway, LogSourceChimeraGateway},
	{LogSourceChimeraBroker, LogSourceChimeraBroker},
	{LogSourceChimeraVectorstore, LogSourceChimeraVectorstore},
	{LogSourceChimeraIndexer, LogSourceChimeraIndexer},
	{TimelineKindWeb, TimelineKindWeb},
}

// LogsUIServiceBadgeRule maps normalized service keys to a CSS class suffix (sum-svc-*).
type LogsUIServiceBadgeRule struct {
	Keys  []string
	Class string
}

// LogsUIServiceBadgeRules are checked in order; first match wins.
var LogsUIServiceBadgeRules = []LogsUIServiceBadgeRule{
	{Keys: []string{LogSourceChimeraBroker, TimelineKindBroker}, Class: "sum-svc-broker"},
	{Keys: []string{LogSourceChimeraVectorstore, TimelineKindVectorstore}, Class: "sum-svc-vectorstore"},
	{Keys: []string{LogSourceChimeraIndexer, TimelineKindIndexer}, Class: "sum-svc-indexer"},
	{Keys: []string{LogSourceChimeraGateway, TimelineKindGateway}, Class: "sum-svc-gateway"},
	{Keys: []string{TimelineKindWeb}, Class: "sum-svc-web"},
}

// LogsUIServiceBadgeDefault is used when no rule matches.
const LogsUIServiceBadgeDefault = "sum-svc-gateway"
