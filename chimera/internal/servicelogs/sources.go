package servicelogs

// Canonical servicelogs ring-buffer source names (match JSON service on normalized lines).
const (
	SourceChimeraGateway     = "chimera-gateway"
	SourceChimeraBroker      = "chimera-broker"
	SourceChimeraVectorstore = "chimera-vectorstore"
	SourceChimeraEmbed       = "chimera-embed"
	SourceChimeraIndexer     = "chimera-indexer"
	SourceChimeraSupervisor  = "chimera-supervisor"
)
