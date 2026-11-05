# End-to-end verification

Run against the same deployed commit and keep the raw request/response or screenshot evidence.

## 1. Identity and isolation

1. Log in as `teacher`, `student`, and `student_other`; record the server-returned user, role, and class.
2. Verify a missing token returns 401.
3. Verify a student calling a teacher write returns 403.
4. Verify `student_other` requesting a `class-demo` resource returns 404.

## 2. Material, RAG, streaming, and MCP

1. Teacher ingests a unique text material.
2. Student searches it and records material version, chunk ID, character range, and hash.
3. Student creates a conversation; the page must select it immediately.
4. Ask a supported question and record SSE metadata, citations, deltas, and completion.
5. Ask an unsupported question and verify the response states that no source was found and contains no citation.
6. Invoke one enabled MCP tool; record discovery, call result, failure behavior, and audit event.

## 3. Assignment and asynchronous processing

1. Teacher creates and publishes an assignment.
2. Student view must omit answers and explanations.
3. Student submits once and receives 202 plus task ID.
4. Repeat the identical request with the same idempotency key; verify the same task/result.
5. Reuse the key with different content; verify 409.
6. Poll `pending → running → succeeded` (a very fast local run may omit an intermediate observation), then verify the score and mistake hint.
7. Stop the worker before submission, restart it, and verify the accepted task is not lost before claiming durable-queue evidence.
8. Teacher overrides the score; verify the previous and final values remain in history.

## 4. Skills, security, audit, and delivery

1. Exercise all twelve skill entry points and their declared normal/failure paths.
2. Run prompt-injection, cross-class, ownership, invalid-schema, model-failure, MCP-timeout, and duplicate-delivery tests.
3. Query audit as teacher and verify actor, UTC time, action, object, result, skill, usage, duration, and request ID; student audit access must return 403.
4. Run `make check`, `make check-security`, the container smoke path, and the load script.
5. Store only authentic outputs under `reports/` and link the matching pull request, review, public deployment, and three user feedback records.
