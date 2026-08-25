# ADK Mutation/Control Group Ledger

- Group: `adk-mutations`
- Stage 9 rehearsal: C → B → A. C is the Rust route-shape and port leaf;
  B is the frozen Go fixture/reference and Rust replay; A qualification is
  intentionally outstanding because this worker does not wire a production
  owner or execute Assistant side effects.
- Tier: A mutation/control capability, rehearsed only through C/B evidence.
- Operations: 37 remaining `/api/v1/adk` write/control routes.
- Production owner: Go remains the sole owner of Assistant runtime, provider
  lifecycle, SQLite, sessions, tasks, approvals, workflows, skills,
  notifications, and all production writes. Rust has no production owner.
- Route ownership after integration: all 37 operations are `cutover-test-only`
  only when the explicit `AdkMutationPort` is supplied; Go remains the
  production owner. Current dynamic coverage is `1 shadow / 120
  cutover-test-only / 157 cutover-qualified / 0 remaining / 0 Rust production
  owner`.
- Fixture: `tests/fixtures/rust-migration/stage9/adk-mutations.json`
  (`stage9.adk-mutations.v1`, 40 cases: 37 valid route cases and 3 shape/
  identifier error cases).
- Go reference:
  `scripts/rust-migration/stage9_adk_mutations_reference_test.go`.
- Rust leaf: `crates/jftrade-engine/src/product_adk_mutation_port.rs`.
- Rust replay:
  `crates/jftrade-engine/tests/stage9_adk_mutations.rs`.
- Differential:
  `node scripts/rust-migration/check-stage9-adk-mutations.mjs`.
- Authenticated owner-fencing rehearsal:
  `go test ./internal/app/apiserver/servercoretest -run
  '^TestADKMutationRehearsalPreservesAuthenticatedOwnerFencingAcrossRecovery$'
  -count=1 -timeout=300s`.
- Rust product recovery evidence:
  `cargo test -p jftrade-engine --lib adk_mutation_product -- --nocapture`.

## Route inventory

| Method | Routes |
| --- | --- |
| DELETE | `/api/v1/adk/agents/{agentId}`; `/api/v1/adk/memory/{memoryId}`; `/api/v1/adk/providers/{providerId}`; `/api/v1/adk/sessions/{sessionId}`; `/api/v1/adk/skills/{skillId}`; `/api/v1/adk/tasks/{taskId}`; `/api/v1/adk/workflows/{workflowId}`; `/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}` |
| PATCH | `/api/v1/adk/runs/{runId}/objective`; `/api/v1/adk/sessions/{sessionId}/composer-state` |
| POST | `/api/v1/adk/agents`; `/api/v1/adk/approvals/{approvalId}/approve`; `/api/v1/adk/approvals/{approvalId}/deny`; `/api/v1/adk/memory`; `/api/v1/adk/optimization-tasks/{taskId}/cancel`; `/api/v1/adk/providers`; `/api/v1/adk/providers/{providerId}/default`; `/api/v1/adk/providers/{providerId}/test`; `/api/v1/adk/runs/{runId}/cancel`; `/api/v1/adk/runs/{runId}/input-response`; `/api/v1/adk/runs/{runId}/pause`; `/api/v1/adk/runs/{runId}/resume`; `/api/v1/adk/sessions`; `/api/v1/adk/sessions/{sessionId}/context/compact`; `/api/v1/adk/skills`; `/api/v1/adk/tasks`; `/api/v1/adk/workflow-triggers/{triggerId}/run`; `/api/v1/adk/workflow-webhooks/{triggerId}`; `/api/v1/adk/workflows`; `/api/v1/adk/workflows/{workflowId}/run`; `/api/v1/adk/workflows/{workflowId}/triggers` |
| PUT | `/api/v1/adk/agents/{agentId}`; `/api/v1/adk/providers/{providerId}`; `/api/v1/adk/sessions/{sessionId}`; `/api/v1/adk/tasks/{taskId}`; `/api/v1/adk/workflows/{workflowId}`; `/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}` |

The leaf accepts an explicitly injected consumer-owned `AdkMutationPort`.
It parses identifiers, first JSON values, workflow inputs, control routes,
and webhook secret headers, then returns the existing product envelope. It
does not open SQLite, construct an Assistant runtime/session service, call a
provider, install a skill, publish a notification, or execute a workflow.

## Wire and boundary evidence

- Go reference cases use a temporary ADK store and in-memory session service;
  no production database, provider, external runtime, skill source, or
  notification channel is used.
- The fixture freezes status, `Content-Type`, complete success/error envelope,
  timestamp, and port-call boundary. The Rust replay checks every case and
  requires all 37 valid cases to call the injected port exactly once.
- Go's `ShouldBindJSON` first-value/trailing-value behavior is retained for
  object payloads. Control operations ignore body bytes where the Go handler
  does not bind them. `null` is projected as the empty object at the leaf
  boundary where the Go binder yields a nil payload.
- Empty or malformed shape errors are returned before a missing port is
  considered. A valid route without a supplied port fails closed with
  `503 ADK_MUTATIONS_UNAVAILABLE`.
- No default profile registration, authenticated production switch,
  Go/Wails removal, or production owner change is part of this rehearsal.

## Authenticated owner-fencing rehearsal

- The new servercoretest drives all 40 frozen fixture cases through the
  explicit Go loopback rehearsal proxy. The Rust boundary verifies the private
  Bearer credential, internal proxy protocol, web access surface, browser
  Cookie, Origin, Referer, and CSRF headers; the public Bearer credential is
  never forwarded.
- The rehearsal compares every fixture status, content type, and complete
  envelope, including malformed/empty/blank-identifier precedence, repeated
  mutation forwarding, trailing JSON transport, timeout, caller cancellation,
  Rust crash/unavailable fail-closed behavior, Go-only malformed-input
  rollback, restart recovery, and byte-identical settings.
- The Rust product test verifies malformed input precedes an unavailable port,
  unavailable-port failure does not fall back to Go, an explicitly injected
  fixture port recovers after restart, and settings bytes remain unchanged.
- The rehearsal uses only httptest and injected ports. It does not start the
  Assistant runtime, Provider/OpenD, notification or task delivery, or a
  production SQLite writer.

## Quirks and three-way review

### Generated session IDs require fixture normalization

quirk: `POST /api/v1/adk/sessions` returns a wall-clock/random generated ID
with the `session-<uuid>` shape. Session context compaction also returns a
random `contextRevisionId`, and workflow/trigger deletion returns a runtime
`deletedAt`. These values are not stable across Go fixture runs.

范围: `adk-mutations` / session create, context compaction, workflow/trigger delete

证据: Go handler/reference responses contain fresh generated IDs/timestamps;
the frozen fixture stores `fixture-session-created`,
`fixture-context-revision`, and the fixed timestamp; the Rust replay consumes
that normalized data and still compares the complete success envelopes.

分类: fixture/harness

判定: intended, normalized after three-way review

处置: Normalize only the known generated values after validating the
`session-` prefix where applicable, and normalize known runtime timestamp
fields. Preserve all other IDs and wire fields; do not change Go.

风险: low

owner: worker fixture/harness

后续: Keep the normalization while Go owns these operations; a future stable
ID/timestamp corpus may replace it.

三方复核: Go reference generation → frozen JSON fixture → Rust leaf replay.

### Empty-suffix route matcher collision

quirk: The initial Rust dynamic matcher returned `404 NOT_FOUND` for valid
PATCH/PUT routes sharing a resource prefix with a DELETE route whose dynamic
template had an empty suffix. The DELETE entry matched first and returned on
method mismatch before the later route could be considered.

范围: `adk-mutations` / session composer update and same-prefix PUT updates

证据: Go fixture cases such as `session-composer-update` and `agent-update`
are `200` with one port call; the initial Rust leaf returned `404`; the Rust
fixture replay caught the mismatch before integration wiring.

分类: rust-implementation

判定: deviated, fixed after three-way review

处置: Match the method before returning a dynamic route and reject literal
extra path segments as route misses. Retain the replay and route inventory
regression.

风险: low after fix

owner: Rust leaf worker

后续: Keep the route-order regression in the focused group test.

三方复核: Go reference/fixture → Rust leaf dispatch → Rust replay harness.

### Skill uninstall missing-file projection

quirk: `DELETE /api/v1/adk/skills/missing-skill` returns `500
ADK_SKILL_UNINSTALL_FAILED` with message `file does not exist`, rather than a
resource-not-found response.

范围: `adk-mutations` / skill uninstall

证据: Go reference and fixture freeze the exact status/code/message; the Rust
fixture port returns that error and the leaf replay agrees.

分类: go-behavior

判定: observed, unresolved

处置: Preserve the Go projection byte-for-byte. Do not repair the Go error
classification in this migration slice.

风险: medium

owner: Go Assistant skill service until hard-cut review

后续: Re-review only after a separately approved public-contract change.

### Provider test missing-provider projection

quirk: `POST /api/v1/adk/providers/missing-provider/test` returns `502
ADK_PROVIDER_TEST_FAILED` with message `provider not found`.

范围: `adk-mutations` / provider test

证据: Go reference and fixture freeze the exact error; Rust leaf replay uses
the same port error and envelope.

分类: go-behavior

判定: observed, unresolved

处置: Preserve the existing Go response and keep provider access outside the
Rust leaf. No real provider is started by this worker.

风险: medium

owner: Go provider service until cutover

### Workflow control error projections

quirk: Missing workflow-trigger run returns `404
ADK_WORKFLOW_TRIGGER_RUN_FAILED`; a disabled workflow run returns `409
ADK_WORKFLOW_RUN_FAILED`; a disabled webhook returns `400
ADK_WORKFLOW_WEBHOOK_FAILED`.

范围: `adk-mutations` / workflow-trigger, workflow-run, workflow-webhook

证据: Go fixture/reference, the Rust injected error responses, and the full
leaf replay agree on status, code, message, timestamp, and no local runtime
execution.

分类: go-behavior

判定: observed, unresolved

处置: Preserve all current projections and leave workflow lifecycle and
webhook authentication to the Go owner/integration composition root.

风险: medium

owner: Go Assistant workflow runtime until cutover

## Qualification status and gates

The group remains `cutover-test-only` rehearsal evidence, not
cutover-qualified. The C leaf, B differential, and authenticated integration
rehearsal are complete for the 37 routes. Qualification is still blocked on
durable Assistant state/transaction ownership, unique-owner and no-double-write
proof for sessions/tasks/approvals/workflows/skills, real side-effect and
Provider/runtime cancellation and restart evidence, four-platform release and
signing, security/SBOM review, backup/restore/crash recovery, and the final
hard-cut gates. Go remains the only production owner; this worker does not
permit Go/Wails deletion.

Integration evidence: the explicit product test-cutover wiring, shared route
ledger, unified product differential, and route-isolation test all pass with
the 37 operations registered only when the mutation port is injected.

## Verification

- `go test scripts/rust-migration/stage9_adk_mutations_reference_test.go -run '^TestStage9ADKMutationsFixtureMatchesCurrentGoOwner$' -count=1`
- `cargo test -p jftrade-engine --test stage9_adk_mutations -- --nocapture`
- `node scripts/rust-migration/check-stage9-adk-mutations.mjs`
- `node --check scripts/rust-migration/check-stage9-adk-mutations.mjs`
- `rustfmt --edition 2024 --check crates/jftrade-engine/src/product_adk_mutation_port.rs crates/jftrade-engine/tests/stage9_adk_mutations.rs`
- `cargo clippy -p jftrade-engine --test stage9_adk_mutations -- -D warnings`
- `go test ./internal/app/apiserver/servercoretest -run '^TestADKMutationRehearsalPreservesAuthenticatedOwnerFencingAcrossRecovery$' -count=1 -timeout=300s`
- `cargo test -p jftrade-engine --lib adk_mutation_product -- --nocapture`
- `pnpm run test:rust:stage9:product-differential`
- `pnpm run check:quick`
- `pnpm run check:rust`
- `git diff --check`

The shared product differential, shared route ownership ledger, default
profile, generated-contract checks, release/signing/security/recovery gates,
and Go/Wails hard-cut are intentionally outside this worker.
