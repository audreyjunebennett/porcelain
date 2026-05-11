Phase 2 correlation — example structured log lines (text format)

These samples document the gateway correlation triple for operator logs and UI tier-1/tier-2 joins:
  request_id, conversation_id, principal_id

They are not live captures; they mirror slog text handler output.

See: internal/conversationmerge/merge_correlation_test.go,
     internal/chat/correlation_contract_test.go,
     internal/rag/service_test.go (TestService_Retrieve_logContainsPrincipalId),
     internal/server/ingest_test.go (TestIngest_JSON_logsConversationIDWhenHeaderPresent),
     internal/server/lifecycle_phase3_doc_test.go (Phase 3 lifecycle msg slugs).

Example lifecycle lines: testdata/correlation/lifecycle-phase3.example.log
Example tier-4b Qdrant join lines: testdata/correlation/phase5-qdrant-tier4b.example.log
Example multi-turn turn_index lines: testdata/correlation/phase6-multi-turn.example.log
Example Phase 7 tool execution / router lines: testdata/correlation/phase7-tools.example.log
Example Phase 8 request/response witness lines: testdata/correlation/phase8-witness.example.log

