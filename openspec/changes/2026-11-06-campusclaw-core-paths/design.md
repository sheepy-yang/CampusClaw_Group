# Design: CampusClaw core teaching paths

## Architecture

The MVP keeps the existing `net/http` application and explicit dependency direction:

```text
HTTP transport
  -> platform / teaching application services
    -> Memory repository ports and task queue
      -> model gateway / retrieval / MCP adapters
```

The in-process repository remains an explicitly documented MVP adapter so unit and HTTP tests are deterministic. All state transitions go through store methods rather than client-supplied identity. MySQL, Redis, Qdrant, and MinIO stay in Compose as production replacement points; the final report must not describe the memory adapter as production persistence.

## Authentication and Scope

Bearer tokens resolve to a server-owned `Principal`. Fixtures include two classes so cross-class scenarios are executable. Route parameters never establish identity or class scope. Teacher-only writes return 403 for students; resources outside the principal's class return 404.

## Materials and Retrieval

Text material ingestion validates title/content limits, assigns a monotonically versioned immutable source, and splits normalized text into bounded chunks. The MVP retriever combines token-overlap and deterministic hash-vector cosine scores. Every hit carries material ID/version, chunk ID/index, character range, hash, text, and component scores. Empty or cross-class retrieval produces no citation and never fabricates a source.

## Assistant and Skills

Each class has a teacher-managed assistant prompt and enabled skill set. A student creates a server-scoped conversation, then sends a message. The service retrieves only that class's chunks, builds a bounded prompt, optionally invokes an allowed discovered MCP tool, and emits SSE events in this order: metadata, optional citations/tool event, text deltas, completion. Model and tool outputs are untrusted data and are validated, truncated, and rendered as text.

## Assignment, Task, and Mistake Flow

Teachers publish assignments. A student submission is saved before an idempotent task is enqueued and returns HTTP 202 with one task ID. Task status is observable as `pending`, `running`, `succeeded`, or `failed`; duplicate idempotency keys return the same task. Deterministic objective grading and the evaluation module produce a result. Teacher override is authoritative. Incorrect results create one mistake record, and the mistake explanation skill gives hints without exposing the standard answer directly.

## Audit and Observability

Every protected write and skill call records actor, UTC time, action, object, result, skill, usage, duration, and request ID. Rejected authorization attempts are also recorded. Logs remain single-line JSON. Metrics aggregate invocations, failures, latency, and queue depth without high-cardinality user labels.

## Failure Semantics

- Invalid input: 400.
- Missing or invalid authentication: 401.
- Authenticated but forbidden role/disabled skill: 403.
- Missing, cross-class, or other-user resource: 404.
- Idempotency conflict with a different payload: 409.
- Invalid model/tool response: 502.
- Model, MCP, or required dependency unavailable: 503.

## Verification

OpenSpec strict validation precedes implementation. Unit and HTTP tests cover normal, invalid, unauthenticated, forbidden, not-found, duplicate, and dependency-failure paths. `make check` runs specifications, Go tests/vet, frontend build, and Compose validation; `make check-security` runs secret, dependency, and security regression checks.
