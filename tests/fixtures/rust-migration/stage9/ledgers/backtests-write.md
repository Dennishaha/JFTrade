# Backtests Write Group Ledger

- Group: `backtests-write`
- Tier: A mutation / asynchronous run and market-data task state change; this worker remains rehearsal-only.
- Operations: 4
  - `POST /api/v1/backtests`
  - `POST /api/v1/backtests/sync`
  - `DELETE /api/v1/backtests/sync/{taskId}`
  - `DELETE /api/v1/backtests/{runId}`
- Current status: `remaining`; this worker does not register a Rust product route or change `route-ownership.json`.
- Go owner: `internal/api/backtest/routes.go`, `internal/backtest`, `internal/store/backtest`, PineTS/market-data lifecycle, and all run/task persistence remain the only production owner.
- Rust boundary: `crates/jftrade-engine/src/product_backtests_write_port.rs` is a consumer-owned test-only mutation port. It has no SQLite, run store, PineTS, market-data worker, provider, notification, or default-profile registration.
- Fixture: `tests/fixtures/rust-migration/stage9/backtests-write.json`.
- Go reference: `scripts/rust-migration/stage9_backtests_write_reference_test.go`.
- Differential: `scripts/rust-migration/check-stage9-backtests-write.mjs`.
- Rust behavior test: `crates/jftrade-engine/tests/stage9_backtests_write.rs`.

## Contract ledger

| Method | Path | Go observable behavior | Error branches and precedence |
| --- | --- | --- | --- |
| POST | `/api/v1/backtests` | `ShouldBindJSON` decodes one `StartRequest`; a valid request resolves/compiles the strategy, persists a queued run, starts the asynchronous owner, and returns `{id,status:"queued",message:"backtest queued"}`. | Malformed/empty/non-object JSON is `400 BAD_REQUEST` / `invalid backtest request`; request validation is `400 BAD_REQUEST`; missing strategy is `404 NOT_FOUND`; strategy/store failures are `500 BACKTEST_START_FAILED` / `start backtest failed`. |
| POST | `/api/v1/backtests/sync` | `ShouldBindJSON` decodes one `SyncRequest`; defaults and normalizes intervals/session scope, opens the injected sync adapter, creates a task, and returns the task descriptor before the worker completes. | Malformed/empty/non-object JSON is `400 BAD_REQUEST` / `invalid sync request`; request validation is `400 BAD_REQUEST`; adapter, task-store, cancellation/deadline and other setup failures are `500 SYNC_FAILED` with the Go error string. |
| DELETE | `/api/v1/backtests/sync/{taskId}` | URI binding validates the escape and trims the task ID, then calls the task store cancellation hook; a successful hook returns `{taskId,status:"cancelled"}`. | Invalid escape is `400 BAD_REQUEST` / `taskId is invalid`; a missing/already-finished task is `404 NOT_FOUND` / `sync task not found or already completed`. The handler has no separate blank-ID guard. |
| DELETE | `/api/v1/backtests/{runId}` | URI binding validates the escape and trims the run ID; the handler checks status before calling delete. Terminal `completed`, `failed`, and `cancelled` runs return `{deleted:true,id}`. | Invalid/blank ID is `400 BAD_REQUEST`; missing or delete-race is `404 NOT_FOUND`; non-terminal status is `400 BAD_REQUEST`; store failure is `500 BACKTEST_RUN_STORE_FAILED` / `delete backtest run failed`. |

The fixture contains 38 cases and 40 requests. It records response status, content type, canonicalized envelope timestamp, structural port-call boundary, delegation input, and fake Go owner effects. The fake owner never opens a database, starts PineTS, connects a provider, or sends a market-data request. Generated run/task IDs are normalized only after validating their dynamic format; request fields and error messages are retained.

## A-tier owner and fencing evidence

- The four routes register only when the explicit `BacktestsWritePort` is supplied in Rust product assembly; they are `cutover-test-only` in the ownership ledger. Go remains the only production write owner, and current route coverage is `26 shadow / 184 cutover-test-only / 0 qualified / 68 remaining / 0 Rust production owner`.
- The Rust leaf fails closed with `503 BACKTESTS_WRITE_UNAVAILABLE` when no explicit mutation port is supplied; structural body/URI errors still win before the missing-port response.
- Start and sync mutations are delegated once per structurally valid request. Repeated POSTs are recorded as independent calls; no idempotency key is invented.
- Delete performs no store work for blank/invalid IDs, performs the status guard before delete, and preserves the Go delete-failure-then-retry behavior in the restart fixture. No notification or second persistent owner is introduced.
- Durable SQLite transaction rollback, worker recovery against a real run store, external provider lifecycle, cancellation fencing across processes, and production owner switching are not exercised by this worker and remain qualification gates.

## Quirks and three-way review

quirk: `DELETE /api/v1/backtests/sync/%20` returns `404 NOT_FOUND` rather than `400 BAD_REQUEST`; the Go handler trims the bound value but does not reject blank task IDs before calling `CancelSync`, while the Rust leaf initially applied the stricter blank-ID guard.
范围: `backtests-write` / DELETE `/api/v1/backtests/sync/{taskId}`
证据: Go reference case `cancel-blank-id` and `internal/api/backtest/routes.go` `handleSyncCancel`; the Go task-store effect trace (`CancelSync("")`); final Rust fixture replay and `backtests_write_cancel_blank_id_preserves_go_route_branch` route-isolation test.
分类: go-behavior
判定: intended
处置: 已修正 Rust 叶子以复刻 Go observable branch；待硬切后评估是否收紧公开契约。
风险: medium
owner: Go / integration branch
后续: preserve the empty-task cancellation call through any explicit test-cutover adapter; do not tighten the public contract during migration.

quirk: The initial Rust path parser treated malformed `%zz` as an unavailable port and later exposed the wrong `taskId` error text because the percent decoder alone does not validate escape pairs.
范围: `backtests-write` / DELETE `/api/v1/backtests/sync/{taskId}` and DELETE `/api/v1/backtests/{runId}`
证据: Go fixture `cancel-invalid-escape`/`delete-invalid-escape`, Go `BindURI` invalid-escape behavior, initial Rust replay failure, and final Rust route-isolation/error-precedence tests.
分类: rust-implementation
判定: deviated
处置: 已修复 Rust 叶子：先验证 `%HH` 结构，再 decode，并保留 Go 的精确 error message；不改变 Go。
风险: low
owner: worker / integration branch
后续: retain malformed-escape cases in every explicit test-cutover replay.

quirk: `ShouldBindJSON` accepts the first JSON value and ignores trailing JSON, so a valid object followed by another JSON object still queues/schedules the operation.
范围: `backtests-write` / both POST routes
证据: Go reference `start-trailing-json-is-ignored`, fixture delegation trace, and Rust `backtests_write_leaf_preserves_trailing_json_and_error_precedence`.
分类: go-behavior
判定: intended
处置: 已复刻；待硬切后评估是否收紧公开契约。
风险: medium
owner: Go / integration branch
后续: retain exact one-value decoder behavior until a separate public-contract change is approved.

quirk: Repeated start, sync-cancellation, and run-delete requests are not uniformly idempotent: repeated starts create two queued run mutations, repeated sync cancellation returns success then 404, and repeated run deletion returns success then 404. A repeated sync-start request is not included in this fixture and remains an A-tier qualification gap.
范围: all four routes where applicable
证据: Go fixture repeated-write cases, fake store/task effect traces, and parameterized Rust delegation replay.
分类: go-behavior
判定: intended
处置: 已复刻；不增加本地 fencing 或 retry，待硬切前补外部幂等证据。
风险: high
owner: Go / integration branch
后续: release-blocker until external idempotency, duplicate dispatch, and retry acceptance are separately reviewed for A-tier qualification.

quirk: Adapter acquisition cancellation/deadline errors on sync are exposed as HTTP 500 `SYNC_FAILED` with the wrapped raw context error rather than a transport-specific 499/504.
范围: `backtests-write` / POST `/api/v1/backtests/sync`
证据: Go fixture `sync-adapter-canceled` and `sync-adapter-deadline`, `internal/backtest/sync.go`, and Rust error mapping replay.
分类: go-behavior
判定: intended
处置: 已复刻；本迁移切片不规范化 cancellation/timeout status。
风险: high
owner: Go / integration branch
后续: release-blocker until cancellation, timeout, partial task creation, and recovery are covered by the durable owner differential.

quirk: Run/task IDs and envelope timestamps are dynamic wall-clock values; the fixture canonicalizes only the IDs and timestamp fields while retaining all request and business fields.
范围: all POST success cases / fixture harness
证据: Go reference timestamp/ID validation, fixture values, and Rust fixed-clock replay.
分类: fixture
判定: intended
处置: 保留窄 canonicalization；cutover 仍需单独验证格式和时钟来源。
风险: low
owner: integration branch
后续: a production rehearsal must validate RFC3339Nano, monotonicity where applicable, and unique ID generation independently.

quirk: JSON `null` on the sync-start route is accepted as a zero `SyncRequest`; Go then applies the default `HK.00700` instrument and default date range, creates a real sync task, and returns HTTP 200. The generated dates are wall-clock values.
范围: `backtests-write` / POST `/api/v1/backtests/sync`
证据: Go reference `sync-null-body`, fake task/adapter effects, final fixture replay, and Rust parameterized port input.
分类: go-behavior
判定: intended
处置: 已复刻；fixture 仅 canonicalize the dynamic date range, not the defaulting rule。
风险: medium
owner: Go / integration branch
后续: preserve null/default behavior until an explicit public contract change is approved.

## Verification record

- Go reference fixture generation and drift test: passed after final `%20` and dynamic-date canonicalization.
- Rust leaf replay: passed (5 tests), including exact route inventory, unavailable-port fencing, malformed percent escapes, trailing JSON, blank-task route isolation, and all 38 fixture cases.
- Go reference fixture drift test: passed (`GIN_MODE=release go test scripts/rust-migration/stage9_backtests_write_reference_test.go -run '^TestStage9BacktestsWriteFixtureMatchesCurrentGoOwner$' -count=1`).
- Rust leaf/parameterized replay: passed, 5 tests (`CARGO_TARGET_DIR=/tmp/jftrade-stage9-backtests-write-target cargo test -p jftrade-engine --test stage9_backtests_write -- --nocapture`).
- Rust targeted Clippy: passed (`CARGO_TARGET_DIR=/tmp/jftrade-stage9-backtests-write-target cargo clippy -p jftrade-engine --test stage9_backtests_write -- -D warnings`).
- Rust formatting and differential script syntax: passed (`rustfmt --edition 2024 --check ...`; `node --check scripts/rust-migration/check-stage9-backtests-write.mjs`).
- Dedicated differential: passed (`CARGO_TARGET_DIR=/tmp/jftrade-stage9-backtests-write-target node scripts/rust-migration/check-stage9-backtests-write.mjs`).
- Affected Go owner regression: passed (`go test ./internal/api/backtest ./internal/backtest ./internal/store/backtest -count=1`).
- Route coverage after integration: passed at `26 shadow / 184 cutover-test-only / 0 qualified / 68 remaining / 0 Rust production owner`.
- Shared `product*.rs`, `route-ownership.json`, unified product differential, `package.json`, architecture docs, default profile, real SQLite/run store, PineTS, market-data worker and provider lifecycle are intentionally untouched.

## Handoff state

- Group: `backtests-write`
- Tier: A
- Operation count: 4 (38 fixture cases / 40 requests)
- Status: test-only leaf/replay complete; Go owner and route ownership remain unchanged.
- Qualification blockers: durable transaction/restart evidence against the real run/task owner, cancellation/timeout and worker recovery, duplicate/idempotency policy, cross-process fencing, notifications/task isolation, four-platform release/signing/security/SBOM, backup/restore, and hard-cut owner evidence.
