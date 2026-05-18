Chimera release archive
=======================

Binaries (CGO_ENABLED=0; no desktop WebView):
  chimera-gateway, chimera-broker, chimera-supervisor, chimera-vectorstore, chimera-indexer, qdrant

BiFrost (bifrost-http) is NOT included. Install separately or use make release-package for a full local desktop folder.

Quick start
-----------
1. Copy env.example to .env and set provider API keys.
2. Edit config/gateway.yaml and config/chimera-broker.config.json as needed.
3. Copy config/api-keys.example.yaml to config/api-keys.yaml when you are ready for client tokens.
4. Run ./chimera-gateway -version then start the stack, e.g.:
     ./chimera-supervisor
   (install bifrost-http on PATH or pass -broker-bin / -bifrost-bin flags; see PACKAGING.md)

See PACKAGING.md and README.md for full operator documentation.
