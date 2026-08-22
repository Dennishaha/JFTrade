# Strategy-Pine Analyze Group Ledger

- Group: `strategy-pine`
- Tier: B; the one analyze route depends on the PineTS worker/analysis projection and therefore remains lifecycle- and failure-sensitive.
- Operations: 1 `POST /api/v1/strategy-pine/analyze`.
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
证据: `internal/api/strategy/routes.go` `handleAnalyzePine`; `internal/strategy/service.go` `AnalyzePine`; `pkg/strategy/pineengine/config.go` `ShadowPayloadForScript`; Go fixture `worker-cancel-projection`; Rust replay and focused precedence test
分类: go-behavior
判定: unresolved
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover; integration must review before hard-cut
后续: Preserve the current `200` shadow-error projection in Rust test-cutover. Before cutover-qualified, add an explicit HTTP-cancel rehearsal and decide whether the Go context behavior is an approved compatibility quirk or a post-hard-cut fix; do not change Go in this worker.

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
证据: `pnpm run check:rust` failed in `apps/desktop/src-tauri` on the missing Pine worker asset; concurrent `pnpm run check:quick` failed on the missing `var/tauri-runtime`; leaf-level `cargo check`/clippy and strategy-pine differential passed.
分类: harness
判定: unresolved
处置: 修复 fixture/harness
风险: medium
owner: integration branch / desktop preparation harness
后续: Prepare the standard desktop development/release assets on the integration branch and rerun `pnpm run check:rust` and `pnpm run check:quick`; no strategy-pine source change is required.

## Integration handoff

This worker intentionally does not edit `route-ownership.json`, any `product*.rs` composition or route assembly file, the shared differential runner, migration architecture docs, or manifests. The group remains Go-owned and not Rust production-owned until the integration branch applies the wiring patch.

The exact shared patch is:

1. Add `mod strategy_pine` or an `include!` leaf entry in `crates/jftrade-engine/src/product.rs`, import the exclusive `StrategyPineAnalyzeSnapshotPort`, add an optional field to `ProductOptionalPorts` and `ProductApi`, and add a test-only builder on `ProductConfig` that accepts an `Arc<dyn StrategyPineAnalyzeSnapshotPort>`.
2. Add a `ProductApi` dispatch arm for exactly `POST /api/v1/strategy-pine/analyze` that calls `dispatch_strategy_pine_analyze`, maps the leaf response into the existing transport envelope, and registers the route only when the explicit test-cutover port is present. Do not register it in the default profile and do not create a production worker or analyzer owner.
3. Add the one route to `product_route_assembly.rs`/capability metadata and the existing product test-cutover differential registration, preserving Go as `productionOwner` and keeping `goRemovalStatus=retained`.
4. On the integration branch, update `tests/fixtures/rust-migration/stage9/route-ownership.json` and shared Stage 9 coverage/evidence only after the group differential, `check:quick`, and `check:rust` pass.

Suggested next wave: after this B-tier group is integrated, choose another independent B-tier worker-dependent group with no shared file overlap; keep this group out of production cutover until the cancellation rehearsal and worker recovery evidence are complete.
