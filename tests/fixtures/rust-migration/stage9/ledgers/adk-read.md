# ADK Read Group Ledger

- Group: `adk-read`
- Tier: B. ADK reads expose persisted Assistant state, session/run/task
  lifecycle projections, provider/catalog state, and SSE stream reads.
- Operations: 24 GET routes under `/api/v1/adk`.
- Production owner: Go. Rust accepts a consumer-owned `AdkReadSnapshotPort`
  only in explicit `cutover-test-only.v1` wiring. Rust does not open the ADK
  SQLite store, start an ADK/Provider runtime, execute a run, acquire a
  session service, or write Assistant state.
- Fixture: `tests/fixtures/rust-migration/stage9/adk-read.json` (45 cases).
- Successful SSE fixture: `tests/fixtures/rust-migration/stage9/adk-read-sse.json`
  (four stream/reconnect cases with normalized headers and event bodies).
- Go reference: `scripts/rust-migration/stage9_adk_read_reference_test.go`.
- Rust differential: `product::tests::adk_read_tests` plus
  `node scripts/rust-migration/check-stage9-adk-read.mjs`.

## Contract Matrix

| Method | Path | Request/response projection | Error and stream behavior |
| --- | --- | --- | --- |
| GET | `/api/v1/adk` | Complete ADK snapshot projection. | Go error envelope is preserved. |
| GET | `/api/v1/adk/agents` | Agent list, with `limit`/`offset` query paging. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/approvals` | Approval list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/audit` | Audit list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/memory` | Memory list/projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/metrics` | Metrics projection, including normalized `since`. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/optimization-tasks` | Optimization task list with query paging. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/optimization-tasks/{taskId}` | One optimization task projection. | Blank ID is `400`; missing task is `404`. |
| GET | `/api/v1/adk/providers` | Provider descriptor/config projection. | Go error envelope is preserved. |
| GET | `/api/v1/adk/runs` | Run list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/runs/{runId}` | One run projection. | Blank ID is `400`; missing run is `404`. |
| GET | `/api/v1/adk/runs/{runId}/stream` | Run stream endpoint. | Blank ID is `400`; missing stream is `404`; successful SSE requires a stream snapshot. |
| GET | `/api/v1/adk/sessions` | Session list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/sessions/{sessionId}` | One session projection. | Blank ID is `400`; missing session is `404`. |
| GET | `/api/v1/adk/sessions/{sessionId}/context` | Session context projection. | Blank ID is `400`; missing session is `404` with Go context code. |
| GET | `/api/v1/adk/skills` | Skill catalog projection. | Go error envelope is preserved. |
| GET | `/api/v1/adk/streams/{streamId}` | One stream projection. | Blank ID is `400`; missing stream is `404`. |
| GET | `/api/v1/adk/tasks` | Task list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/tasks/{taskId}` | One task projection. | Blank ID is `400`; missing task is `404` with Go task code. |
| GET | `/api/v1/adk/tools` | Tool catalog projection. | Go error envelope is preserved. |
| GET | `/api/v1/adk/workflow-trigger-logs` | Workflow trigger log projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/workflows` | Workflow list projection. | Malformed query is `400 BAD_REQUEST`. |
| GET | `/api/v1/adk/workflows/{workflowId}` | One workflow projection. | Blank ID is `400`; missing workflow is `404`. |
| GET | `/api/v1/adk/workflows/{workflowId}/triggers` | Workflow trigger projection. | Blank ID is `400`; missing workflow is `404`. |

The fixture freezes success data and exact error code/message values. It
normalizes only host/runtime values (`installPath`, ADK timestamps, and metrics
`since`) that are not stable wire state; it does not correct Go behavior.

## Quirks

quirk: Go rejects identifiers that decode to blank text or contain a decoded
slash, and rejects malformed percent escapes in the query. Rust's initial
replay used the URL decoder's permissive treatment of `%zz` and could reach the
snapshot port as a `503` instead of returning Go's `400 BAD_REQUEST`.
范围: `adk-read` / dynamic path identifiers and paged GET queries
证据: Go reference cases `*-query-error`, `*-blank`, the frozen fixture, and
`product_adk_read_tests.rs` path/query replay.
分类: rust-implementation
判定: deviated, fixed after three-way review
处置: Rust now rejects invalid percent triplets before decoding and preserves
Go's blank/encoded-slash validation; the Go observable behavior is unchanged.
风险: medium
owner: integration
后续: retain focused regression while the routes remain cutover-test-only.

quirk: The first ADK Rust fixture adapter filtered all expected `404` cases as
if a dynamic route miss were an unknown endpoint. Valid route matches such as
`/runs/missing` then reached an unavailable port and produced `503`, losing the
Go resource-specific `404` code/message.
范围: `adk-read` / resource, session-context, task, workflow and stream 404s
证据: Go reference and fixture contain the exact 404 envelope; the initial Rust
focused replay returned `503` until the fixture port retained 404 entries.
分类: harness
判定: deviated, fixed after three-way review
处置: The test port now retains every fixture response; route validation owns
only malformed input and the consumer snapshot owns resource-level 404s.
风险: medium
owner: integration
后续: retain the complete fixture replay.

quirk: The first shared compile missed the ADK test module import and the
shared `ProductRoutePorts` initializer missed the new `adk_read` field.
范围: `adk-read` / Rust test harness and route assembly test
证据: `cargo test -p jftrade-engine 'product::tests::adk_read_tests::' --lib`
reported unresolved ADK types and then `E0063` before the minimal wiring fixes.
分类: harness
判定: deviated, fixed after three-way review
处置: Add the existing `use super::*` convention and initialize the field
without changing default route registration.
风险: low
owner: integration
后续: keep the focused compile/test gate in the product differential runner.

quirk: The successful Go ADK stream corpus is produced through a real local
Assistant test runtime and contains dynamic stream/run/session/event IDs.
The fixture normalizes those identifiers and timestamps, while preserving
headers, retry directive, event IDs/order, event JSON and `after` filtering.
范围: `adk-read` / `GET /api/v1/adk/runs/{runId}/stream` and
`GET /api/v1/adk/streams/{streamId}`
证据: Go `TestStage9ADKReadSSEFixtureMatchesCurrentGoOwner`, fixture
`adk-read-sse.json`, Rust
`adk_read_success_sse_fixture_matches_go_wire_in_cutover_only`, and the raw
product response replay.
分类: unknown
判定: resolved for local wire compatibility
处置: Keep the port consumer-owned and use `ApiOutput::Raw` only inside the
existing product boundary so source headers are emitted without changing the
public HTTP/OpenAPI contract. Do not connect the production Assistant runtime.
风险: medium
owner: integration
后续: Retain the authenticated Go sidecar GET-stream rehearsal, including
timeout/cancellation and restart evidence, through hard-cut review; Go remains
the production owner.

## Three-Way Review

- The Go reference, frozen fixture, and Rust replay agree on all 17 successful
  list/snapshot cases, ten malformed-query `400` cases, and the exact dynamic
  identifier/resource error envelopes.
- The Go reference, fixture, and Rust unit replay agree that decoded blank IDs
  and encoded slash IDs are rejected before the snapshot port; the separate
  invalid-percent regression now matches Go's `400` behavior.
- The Go reference, fixture, and Rust focused tests agree that a missing
  snapshot port is not enough to register ADK routes in the default profile;
  registration requires explicit test-cutover capability plus the port.
- The new Go success corpus, Rust leaf replay and raw product replay agree on
  SSE headers, retry framing, event IDs/order/body, run/stream reconnect paths
  and `after` filtering. The dedicated authenticated GET-sidecar rehearsal
  now covers transport and recovery; production owner and release gates remain
  open.

The 24 ADK GET operations are now `cutover-qualified`,
`productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated
wire/error/timeout/crash/restart rehearsal. It exercises empty and missing
resource projections without executing runs or mutating Assistant state.
`GET /api/v1/adk/runs/{runId}/stream` and
`GET /api/v1/adk/streams/{streamId}` now have a dedicated authenticated
GET-sidecar rehearsal covering successful SSE, error, timeout, caller
cancellation, Rust crash, Go rollback/restart, and settings immutability.
They are qualified only as compatibility/rehearsal routes: Go remains the
production owner, and no Provider, ADK runtime, SQLite write, session
mutation, approval/task mutation, notification, or Rust production owner was
introduced.

## 2026-08-26 verification

- `go test ./scripts/rust-migration -run '^TestStage9ADKRead(SSE)?FixtureMatchesCurrentGoOwner$' -count=1 -timeout=300s`
- `cargo test -p jftrade-engine --lib 'product::tests::adk_read_tests::' -- --nocapture`
- `node scripts/rust-migration/check-stage9-adk-read.mjs`
- `go test ./internal/app/apiserver/rustrehearsal -run '^TestRehearsalProxyRecognizesADKStreamReplayAsSSE$' -count=1`
- `go test ./internal/app/apiserver/servercoretest -run '^TestADKReadStreamRehearsalPreservesAuthenticatedSSEAndRecoversAcrossRestart$' -count=1 -timeout=300s`
- `cargo fmt --all -- --check`

These checks prove local Go JSON/SSE fixture parity, authenticated GET-sidecar
transport/recovery behavior, and explicit Rust test-cutover transport replay.
The two stream routes are now `cutover-qualified` for compatibility evidence
only; Go remains the production owner and no production owner switch occurred.
