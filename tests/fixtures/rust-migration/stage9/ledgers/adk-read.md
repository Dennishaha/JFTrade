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
- Go reference: `scripts/rust-migration/stage9_adk_read_reference_test.go`.
- Rust differential: `product::tests::adk_read_tests` plus the Stage 9 product
  differential runner.

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

quirk: A successful Go ADK stream fixture with response headers, event order,
heartbeats, close behavior, and `X-ADK-Stream-ID` is not present in this corpus.
The Rust raw port can carry source headers and event IDs, but the existing
`ApiOutput::Sse` transport currently emits only standard SSE framing and has no
custom response-header channel, so `X-ADK-Stream-ID` is not proven compatible.
范围: `adk-read` / `GET /api/v1/adk/runs/{runId}/stream` and
`GET /api/v1/adk/streams/{streamId}`
证据: Go fixture has only missing/blank stream cases; Rust
`adk_read_streams_preserve_event_ids_and_payloads` checks event IDs/payloads,
while the source header is intentionally not emitted.
分类: unknown
判定: unresolved
处置: Preserve Go wire behavior when a successful stream corpus and transport
header decision are available; do not expand the public transport API in this
slice and do not mark stream capability qualified.
风险: high
owner: integration
后续: resolve before any ADK SSE cutover qualification and before Go deletion.

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
- The stream header/event finding remains unresolved because the Go baseline
  lacks a successful stream sample. It is release-blocking for SSE
  qualification, but does not block this test-only registration.

The 22 ordinary JSON operations are now `cutover-qualified`,
`productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated
wire/error/timeout/crash/restart rehearsal. It exercises empty and missing
resource projections without executing runs or mutating Assistant state.
`GET /api/v1/adk/runs/{runId}/stream` and
`GET /api/v1/adk/streams/{streamId}` remain `cutover-test-only` because the
successful SSE header/event corpus is still unresolved. No Provider, ADK
runtime, SQLite write, session mutation, approval/task mutation, notification,
or Rust production owner was introduced.
