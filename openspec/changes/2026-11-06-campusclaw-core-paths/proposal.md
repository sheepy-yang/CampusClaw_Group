# Change: Complete CampusClaw core teaching paths

## Why

The repository can log in and list seeded data, but the course-required CampusClaw paths are incomplete: materials are not indexed or searchable, the assistant has no RAG/SSE conversation path, assignments cannot be submitted through an asynchronous task contract, mistakes have no student explanation path, and audit records do not expose the seven required fields. The final project must demonstrate these paths with class isolation and observable failures instead of relying on UI placeholders.

## What Changes

- Complete server-owned authentication and two-class fixtures for authorization tests.
- Ingest text materials into immutable, traceable chunks and provide class-scoped keyword/vector hybrid retrieval.
- Add teacher assistant configuration, enabled skills, RAG-grounded streaming chat, citations, bounded MCP use, and call records.
- Add assignment publication, student submission returning `202 + taskId`, task polling, teacher score override, and student mistake explanations.
- Record allowed and rejected operations with actor, time, action, object, result, skill, usage, duration, and request ID.
- Add OpenAPI, HTTP tests, security regressions, reproducible commands, and evidence-oriented documentation.

## Out of Scope

- Production-scale OCR, distributed tracing, model fine-tuning, and high availability.
- Fabricated peer review, public deployment, benchmark numbers, user feedback, screenshots, or Git/GitHub evidence.
- Replacing the selected teaching domain with a generic chat demo.
