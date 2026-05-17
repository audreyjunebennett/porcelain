What is the canonical operator vocabulary: do UI/docs/logs always say chimera-vectorstore / chimera-broker, and only show upstream names in “technical details"?

Yes — the canonical names are chimera-vectorstore and chimera-broker, everywhere.
UI, docs, logs, supervisor, CLI → always Chimera names
Upstream names (Qdrant/Bifrost) appear only in:
  - architecture docs
  - debug views
  - error details

What level of compatibility is required for current CLI flags and env vars (-qdrant-*, -bifrost-*, QDRANT__*, APP_*)?

The wrapper binary would only need to support the flags and env vars that are used by the supervisor. 


Do wrappers need to be standalone binaries (chimera-vectorstore, chimera-broker) or just internal Go adapters first?

Make standalone wrapper binaries immediately.

chimera-vectorstore

chimera-broker

These are the processes the supervisor manages.
Inside the codebase, you can still have Go interfaces, but the operator-facing contract is the wrapper binary.

What is the expected health contract per wrapper (/healthz, /readyz, status payload schema, version fields)?

/healthz   → process alive
/readyz    → upstream ready
/metrics   → wrapper + upstream metrics

status: ok | degraded | error
component: chimera-vectorstore
upstream:
  name: qdrant
  version: 1.9.0
  status: ok

Should wrapper logs preserve upstream raw lines in a field (upstream_raw) while emitting normalized top-level msg slugs?

Yes, preserve upstream logs — but only on demand.

Default mode:

    - Emit only Chimera-normalized logs
    - Store upstream logs in a small in-memory ring buffer
    - Expose raw logs via /debug/upstream/logs

Debug mode:
    - Forward upstream logs
    - Add upstream metadata
    - Parse severity if needed


Where do you want config translation ownership: wrapper-only, supervisor-only, or shared library?

Put all translation inside the wrappers.

Supervisor → speaks Chimera config

Wrapper → translates to upstream config

UI → only shows Chimera config

Do you want wrapper-managed lifecycle features: pid files, lock files, restart policy, backoff, graceful shutdown timeout?

Yes — wrappers should own lifecycle semantics.

Start/stop upstream process

Graceful shutdown timeout

Restart/backoff policy

Optional pid/lock files

Deterministic “ready" log line

Is a future backend swap (Milvus/Weaviate/Redis Vector) in-scope now, or is this primarily naming + operational decoupling?

Design these wrappers assuming backend swaps will happen in the near future.

For docs: what is the rule for mentioning stack internals?

Upstreams are implementation details.

- Operator docs → Chimera names only
- UI → Chimera names only
- Architecture docs → "chimera-vectorstore is currently powered by Qdrant"
- Debug docs → show upstream logs/metrics


What migration strategy do you want: hard cut now vs dual naming period in logs/tests/docs?

hard cut now

