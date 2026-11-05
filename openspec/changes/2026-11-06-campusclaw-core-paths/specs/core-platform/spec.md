# Spec: core-platform

## ADDED Requirements

### Requirement: Server-owned identity and class isolation

The system MUST derive user, role, and class from a validated bearer token and MUST enforce role and resource ownership on every protected operation.

#### Scenario: Access a protected route without a token

- **Given** a protected materials, assistant, assignment, task, or audit route
- **When** the request has no valid bearer token
- **Then** the response is 401 and no state changes

#### Scenario: Student writes teacher configuration

- **Given** an authenticated student
- **When** the student updates an assistant, skill, assignment, or score
- **Then** the response is 403 and the previous state is unchanged

#### Scenario: Access another class

- **Given** an authenticated principal from class B
- **When** the principal requests a class A resource
- **Then** the response is 404 or an empty scoped search result without disclosing existence

### Requirement: Traceable class-scoped material retrieval

The system MUST ingest validated text into versioned chunks and MUST return only current-class hits with immutable source metadata.

#### Scenario: Ingest and retrieve a material

- **Given** a class teacher submits a valid title and text body
- **When** the material is ingested and a class member searches for a matching phrase
- **Then** the response contains ranked hits with material ID/version, chunk ID/index, character range, content hash, exact text, and scores

#### Scenario: Search without a matching source

- **Given** no current-class chunk supports the query
- **When** a class member searches
- **Then** the result has zero hits and no fabricated citation

#### Scenario: Reject invalid material input

- **Given** an empty title, empty content, oversized content, or unsupported encoding
- **When** a teacher uploads the material
- **Then** the response is 400 and no material or chunk is stored

#### Scenario: Search another class

- **Given** class A contains a unique chunk and class B does not
- **When** a class B user searches for that unique text
- **Then** the response contains zero hits

### Requirement: Grounded streaming assistant

The system MUST support teacher-managed prompts and skills and MUST stream student answers grounded in current-class retrieval and bounded tools.

#### Scenario: Ask a grounded question

- **Given** an authenticated student, an enabled assistant skill, and matching current-class chunks
- **When** the student sends a message to a server-scoped conversation
- **Then** SSE emits metadata, citations, text deltas, and a completion event with traceable sources

#### Scenario: Ask without a knowledge hit

- **Given** retrieval returns no current-class hit
- **When** the student asks a factual course question
- **Then** the assistant states that no supporting course material was found and emits no fabricated citation

#### Scenario: Use a disabled skill

- **Given** the teacher disabled the requested skill
- **When** a student sends a message with that skill
- **Then** the response is 403 and no model or tool call occurs

#### Scenario: Reject an unavailable or excessive tool loop

- **Given** the selected MCP server is disconnected, unauthorized, or the call limit is reached
- **When** the assistant attempts a tool call
- **Then** the call is recorded as rejected or failed and the conversation degrades to existing grounded information

### Requirement: Asynchronous assignment and mistake path

The system MUST accept student submissions quickly, process them through an observable idempotent task, preserve teacher override, and create traceable mistake feedback.

#### Scenario: Submit an assignment

- **Given** a published assignment belonging to the student's class
- **When** the student submits a valid answer with an idempotency key
- **Then** the response is 202 with one task ID and the saved submission is queryable

#### Scenario: Poll task states

- **Given** an accepted submission task
- **When** the client polls the task endpoint
- **Then** it observes a valid state from `pending`, `running`, `succeeded`, or `failed` and the final result is linked to the submission

#### Scenario: Repeat the same submission

- **Given** a submission was accepted with an idempotency key
- **When** the same student repeats the same request with that key
- **Then** the response returns the existing task and does not create a second result

#### Scenario: Reuse an idempotency key with different content

- **Given** a key already identifies a saved payload
- **When** the same student uses it for different answer content
- **Then** the response is 409 and the original submission remains unchanged

#### Scenario: Teacher overrides an automatic score

- **Given** an automatic result exists and a class teacher submits a final score
- **When** the override is saved
- **Then** the teacher score is authoritative and both old and new values remain in history

#### Scenario: Explain a mistake without revealing the answer

- **Given** an incorrect result created a mistake for the current student
- **When** the student requests mistake guidance
- **Then** the response explains the error cause and gives a next-step hint without returning the standard answer verbatim

### Requirement: Seven-field audit and operational evidence

The system MUST make allowed and rejected protected actions queryable by authorized teachers.

#### Scenario: Query a skill audit event

- **Given** a protected skill call completed or was rejected
- **When** the class teacher queries audit records
- **Then** each event contains actor, timestamp, action, object, result, skill, usage, duration, and request ID

#### Scenario: Student queries audit records

- **Given** an authenticated student
- **When** the student requests the audit endpoint
- **Then** the response is 403 and the rejection itself is logged without sensitive values

#### Scenario: Secret and sensitive content handling

- **Given** application logs and API responses
- **When** automated security checks scan them
- **Then** tokens, cookies, API keys, passwords, and full sensitive model context are absent
