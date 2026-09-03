# Strategy-Pine Analyze Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `strategy-pine`
- Tier: B; the one analyze route depends on the PineTS worker/analysis projection and therefore remains lifecycle- and failure-sensitive.
- Operations: 1 `POST /api/v1/strategy-pine/analyze`.
- Current ownership: `cutover-qualified`; the route is registered only when the explicit product test-cutover profile supplies `StrategyPineAnalyzeSnapshotPort`. Go remains the production owner.
- Production owner: Go remains the only production owner of Pine parsing, analysis metadata, PineTS worker lifecycle, and the external shadow projection. Rust receives only a complete JSON projection through `StrategyPineAnalyzeSnapshotPort` in an explicit integration-owned test-cutover wiring.
- Fixture: `tests/fixtures/rust-migration/stage9/strategy-pine.json`
- Go reference: `scripts/rust-migration/stage9_strategy_pine_reference_test.go`
- Rust replay: `crates/jftrade-engine/tests/stage9_strategy_pine.rs` and `crates/jftrade-engine/src/strategy_pine.rs`
- Differential: `node scripts/rust-migration/check-stage9-strategy-pine.mjs`

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| POST | `/api/v1/strategy-pine/analyze` | JSON body has `script` (string), optional `sourceFormat` (trimmed/lower-cased, default `pine-v6`) and optional `includeAst` (boolean). A successful HTTP response is `200` and carries the complete Go analysis projection under `data`; omitted fields, empty arrays, nulls, diagnostics, AST/semantic payloads and `externalEngine` are preserved opaquely. `Content-Type` is `application/json; charset=utf-8`. | Malformed JSON or wrong field types: `400 BAD_REQUEST` / `invalid strategy pine analyze payload`; unsupported source format: `400 BAD_REQUEST` / `strategy-pine analyze supports pine-v6 only`; analysis diagnostics remain a `200` projection; PineTS unavailable/timeout/cancel/crash remains a `200` projection with `data.externalEngine.status=shadow_error`; an unavailable Rust snapshot port maps to `503 STRATEGY_PINE_ANALYZE_UNAVAILABLE`; adapter failures preserve status/code/message and `Retry-After` when provided. |

## Boundary and ownership

The leaf performs no Pine parsing, worker process management, Provider/OpenD access, persistence, strategy state mutation, or double write. The snapshot port is the consumer-owned boundary for the full Go result. The default Rust product profile must not register this route; only explicit test-cutover composition may expose it, and Go remains the production owner.

`null` request bodies and null scalar fields follow Go `encoding/json` zero-value behavior. An empty or abnormal but valid JSON projection from the snapshot port is forwarded without schema reinterpretation so Rust cannot silently diverge on future Pine metadata fields.

## Three-Way Review

The Go reference generated the fixture from the current handler/service/analyzer path. The Rust replay compares every fixture case's status, `Content-Type`, error envelope or complete data value. Worker unavailable, timeout, cancel, and crash are deterministic local scripts only; no real PineTS worker or external service is used by the Rust tests. Dedicated leaf tests additionally exercise port unavailable/failure, retry metadata, validation precedence, null zero-values, opaque projections, and non-route rejection.

### Reviewed quirks

quirk: The first deterministic worker fixture wrote a JSON-RPC response containing the two literal characters `\\n` instead of a newline, so the Go worker client waited for its fixed 15-second response deadline and reported a timeout.
范围: `strategy-pine` / worker unavailable, timeout, cancel, crash fixture harness
证据: initial Go reference run; `scripts/rust-migration/stage9_strategy_pine_reference_test.go` `writeStage9WorkerScript`; regenerated `strategy-pine.json`; Rust replay of all four worker projections
分类: fixture
判定: deviated
处置: 修复 fixture/harness
风险: low
owner: worker
后续: Keep the worker script response framing as real newline-delimited JSON and rerun the Go reference before changing the fixture.

quirk: The Go route does not pass `c.Request.Context()` into the injected analyzer; `ShadowPayloadForScript` creates an independent background context with a fixed 15-second timeout. A worker-level `AbortError` is therefore observable as a `200` response with `data.externalEngine.status=shadow_error`, while a client HTTP cancellation is not established as a route-level cancellation signal by this baseline.
范围: `strategy-pine` / `POST /api/v1/strategy-pine/analyze` cancellation and shadow error precedence
证据: `internal/api/strategy/routes.go` `handleAnalyzePine`; `internal/strategy/service.go` `AnalyzePine`; `pkg/strategy/pineengine/config.go` `ShadowPayloadForScript`; Go fixture `worker-cancel-projection`; Rust replay and focused precedence test; `TestStrategyPineRehearsalFencesOwnersAndRecoversAcrossRestart` client-cancellation boundary
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until cutover; integration must review before hard-cut
后续: Preserve the current `200` shadow-error projection in Rust test-cutover. The authenticated rehearsal now proves client cancellation at the private Rust boundary returns without replay or Go fallback; retain the Go analyzer context behavior as a hard-cut compatibility decision and do not change Go in this slice.

quirk: PineTS shadow worker failures do not override the main Go analyzer projection or HTTP status. The worker unavailable, timeout, cancel, and crash cases all retain `200`, including the analyzer's own diagnostics, and place the worker failure only in `externalEngine.diagnostics`.
范围: `strategy-pine` / worker lifecycle failure precedence
证据: all four worker cases in the Go-generated fixture, `ExternalEnginePayloadFromResult`, and Rust fixture replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Keep external shadow lifecycle separate from user-facing analysis status until an explicit owner/cutover decision changes the contract.

quirk: A JSON `null` request body and null scalar fields are accepted as Go struct zero values; the route then returns the normal `200` analysis projection instead of a request error. Unsupported `sourceFormat` is checked before the analyzer/snapshot port is invoked.
范围: `strategy-pine` / input validation and error precedence
证据: Go reference `null-body-is-zero-value-input`, Go service source-format normalization, Rust `decode_input`, and focused Rust precedence tests
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Retain null/zero-value and validation precedence in every future adapter; do not add stricter Rust-only schema validation.

quirk: Full workspace `check:rust` and `check:quick` cannot reach their final gates in this worker worktree because the desktop Tauri build references prepared assets that are absent: `internal/pineworkerassets/assets/bin/worker.mjs` and `var/tauri-runtime`.
范围: `strategy-pine` / workspace validation harness
证据: Earlier `pnpm run check:rust`/`pnpm run check:quick` failures on missing prepared assets; current worktree has the standard ignored assets and the subsequent complete `pnpm run check:quick` and `pnpm run check:rust` gates passed.
分类: harness
判定: deviated
处置: 修复 fixture/harness
风险: medium
owner: integration branch / desktop preparation harness
后续: Keep the asset preparation prerequisite explicit; no strategy-pine source change is required and the current local gate is closed.

quirk: One full `go test ./scripts/rust-migration -count=1` run inside `check:quick` reported strategy-pine fixture drift, although the fixture and strategy-pine sources were unchanged; the isolated reference test and a subsequent full package rerun both passed without regeneration.
范围: `strategy-pine` / Go reference package harness isolation
证据: the failed `check:quick` run at `TestStage9StrategyPineFixtureMatchesCurrentGoOwner`; five consecutive isolated reference runs; subsequent full `go test ./scripts/rust-migration -count=1` and complete Stage 9 product differential passed; no strategy-pine fixture was regenerated.
分类: harness
判定: deviated
处置: 保留现有 fixture，不在迁移切片内重生成或修改 Go 行为；按环境复现并隔离共享 worker/env 状态。
风险: medium
owner: integration branch / Go reference harness
后续: Keep the fixture immutable and retain environment/process isolation in CI; the current local qualification gate is closed after repeated isolated and full-package passes.

## Cutover-qualified status

The Go reference fixture, Rust leaf replay, authenticated loopback rehearsal, explicit product test-cutover adapter, and full Stage 9 product differential are green. The 12 fixture cases cover successful opaque projections, null/empty values, malformed and wrong-typed input, source-format precedence, and all four PineTS shadow worker failure projections. The authenticated rehearsal covers repeated success, Rust error, timeout, client cancellation, crash/fail-closed behavior, Go-only rollback, restart recovery, private bearer authentication, browser Cookie/Origin/Referer/CSRF forwarding, request IDs, and unchanged settings bytes. The route is absent from the default profile and is registered only with an injected `StrategyPineAnalyzeSnapshotPort`; Go remains the only production owner. No Pine parser, real PineTS worker, Provider/OpenD, SQLite, strategy state, notification, or user-visible side effect is owned by Rust.

This group is `cutover-qualified`, not a production migration. The Go-compatible analyzer context and worker-shadow error precedence remain recorded compatibility quirks for hard-cut review. Packaged worker release assets, four-platform release/signing, independent security review, SBOM, backup/restore, and final unique-owner/hard-cut gates remain open in the Stage 9 closeout manifest.

## Integration Review

- Product wiring adds a private `strategy_pine` module, a consumer-owned `StrategyPineAnalyzeSnapshotPort`, and an exact `POST /api/v1/strategy-pine/analyze` dispatch arm. The default product profile does not register the route; the explicit test-cutover profile reports 48 routes without the port and 49 with it.
- The shared differential runs `TestStage9StrategyPineFixtureMatchesCurrentGoOwner`, the authenticated Go rehearsal, the six-case Rust leaf replay, and the four product tests for fixture replay, snapshot failure/retry metadata, timeout/recovery/restart, and unregistered-route isolation. The product adapter maps the leaf projection into the existing JSON envelope and preserves `Retry-After` through `ApiFailure`.
- No Pine parser, PineTS worker, Provider/OpenD lifecycle, SQLite access, strategy state mutation, notification, or second production owner was added. `productionOwner=go` and `goRemovalStatus=retained` remain unchanged.
- The route ledger records this operation as `cutover-qualified` with `productionOwner=go` and `goRemovalStatus=retained`. Local qualification evidence is closed for contract, differential, error precedence, recovery rehearsal, authenticated fencing, default-profile isolation, and no-local-side-effect checks; production-owner, release/signing, security, SBOM, backup/restore, and hard-cut gates remain external to this group.
