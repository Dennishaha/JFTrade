# Backtests Run Read Group Ledger

- Group: `backtests-run-read`
- Tier: C: qualification uses an authenticated Go-sidecar rehearsal for persisted backtest-run projections, while the Go run store still owns SQLite, recovery, and the in-memory active view; Rust remains test-cutover-only.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `BacktestReadSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the backtest database, starts PineTS, syncs market data, or mutates a run.
- Fixture: `tests/fixtures/rust-migration/stage9/backtests-read.json`
- Differential: `TestStage9BacktestsReadFixtureMatchesCurrentGoOwner` plus parameterized tests in `product_backtests_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/backtests` | Returns `{runs: [...]}` using Go's lightweight run projection with result details omitted; ordering and nullable/omitted fields are preserved. | Go's empty in-memory store remains the historical `runs: null` projection; snapshot failures map to `500 BACKTEST_RUN_STORE_FAILED` in the explicit adapter. |
| GET | `/api/v1/backtests/{runId}/status` | Returns only `{id, status}` for the requested run. | A decoded blank ID follows Go's lookup and is `404 NOT_FOUND`; malformed escapes are `400 BAD_REQUEST`; unknown runs are `404 NOT_FOUND`; snapshot failures are `500 BACKTEST_RUN_STORE_FAILED`. |
| GET | `/api/v1/backtests/{runId}` | Returns the complete `RunState`, including request, timestamps, optional result/PnL curves, logs, warnings, and market-data provider metadata. | A decoded blank ID follows Go's lookup and is `404 NOT_FOUND`; malformed escapes are `400 BAD_REQUEST`; unknown runs are `404 NOT_FOUND`; result-store failures are `500 BACKTEST_RUN_STORE_FAILED`. |

Known quirks: list uses the Go lightweight projection and therefore omits `result`; an empty Go store serializes `runs` as `null` rather than `[]`. Run timestamps are fixed in the fixture because the current owner persists wall-clock values. The sync worker route (`GET /api/v1/backtests/sync/{taskId}`) is deliberately excluded because it exposes mutable in-process task lifecycle state and will be cut separately.

All three operations are `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. Backtest POST/DELETE routes, the sync worker, PineTS, market-data providers, and SQLite writes remain outside this slice.

## Three-way review and quirks

### Q1: Go treats a blank status run ID as an unknown run

quirk: Go returns `404 NOT_FOUND / backtest run not found` for
`GET /api/v1/backtests/%20/status` and `/api/v1/backtests/%20`; a path adapter
that treats a trimmed blank ID as a validation error would change that
observable result.

范围: `backtests-run-read` / `GET /api/v1/backtests/{runId}/status`.

证据: `TestStage9BacktestsReadBlankRunIDsPreserveGoNotFoundBehavior` raw-path
reference, the Rust replay probe, and
`product_api_backtests.rs::backtest_run_id`.

分类: go-behavior

判定: confirmed and resolved in the backtests-owned Rust adapter.

处置: malformed percent escapes are rejected as 400, while a decoded blank ID
is passed to the snapshot port and maps to the existing-run lookup's 404. No
Go cleanup is part of this migration slice.

风险: low

owner: backtests-run-read worker

后续: retain both blank-ID cases in the group differential and review any
post-hard-cut Go behavior change separately.

### Q2: the first Rust replay reused the non-empty list fixture

quirk: after the Go reference added the empty-store case, the Rust test
fixture port still returned the `list` case for every request, so the
`list-empty` replay compared the non-empty projection with Go's `runs: null`.

范围: `backtests-run-read` / `GET /api/v1/backtests` fixture replay.

证据: Rust test failure in
`product_backtests_tests.rs::backtests_read_routes_match_group_fixture_in_cutover_only`,
the Go-generated `backtests-read.json` `list-empty` case, and the existing
`FixtureBacktestReadPort::list` selector.

分类: harness

判定: deviated, then resolved in the group-owned Rust test harness.

处置: select the requested list case per replay; no production code or Go
behavior changes.

风险: low

owner: backtests-run-read worker

后续: keep the empty-list case in the group differential.

### Q3: restart wire comparison must ignore only the dynamic envelope timestamp

quirk: Go generates a fresh top-level response `timestamp` for every HTTP
response, so a restart-time rollback response cannot be compared byte-for-byte
with the pre-failure snapshot even when the backtest projection and headers
are unchanged.

范围: `backtests-run-read` authenticated restart rehearsal.

证据: the Go sidecar response envelope, the restart path in
`rehearsal_backtests_read_routes_test.go`, and the exact fixture/Rust replay
for list, status, result, blank IDs and store errors.

分类: harness

判定: confirmed and resolved in the group-owned rehearsal.

处置: normalize only the top-level envelope timestamp for rollback comparison;
the response status, data/error payload, and selected wire headers remain exact.

风险: low

owner: backtests-run-read worker

后续: retain the authenticated restart check before any owner switch.

### Q4: worker-local Rust tests did not expose an include-module name collision

quirk: the backtests and strategy worker commits each introduced a private
`has_invalid_percent_escape` helper. The files are group-owned separately but
are `include!`-assembled into one Rust module, so the first integrated product
differential failed compilation with a duplicate definition even though both
worker-local test suites were green.

范围: integrated Stage 9 Rust product build; no HTTP route behavior changed.

证据: the failed `pnpm run test:rust:stage9:product-differential` cargo step,
the two helper definitions in `product_api_backtests.rs` and
`product_wire.rs`, and the successful pre-integration worker-local replays.

分类: rust-implementation

判定: confirmed and resolved on the integration branch by giving the
backtests helper a group-specific name.

处置: keep private compatibility helpers uniquely named across the assembled
module; no Go behavior, public wire, owner or persistence boundary changed.

风险: low

owner: integration branch

后续: retain the full integrated differential as the cross-group compile gate.

## Verification record

- Go observable fixture: `go test ./scripts/rust-migration -run 'TestStage9BacktestsReadFixtureMatchesCurrentGoOwner|TestStage9BacktestsReadBlankRunIDsPreserveGoNotFoundBehavior$' -count=1`.
- Go authenticated sidecar rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestBacktestsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust replay/auth/fail-closed tests: `cargo test -p jftrade-engine 'product::tests::backtests_read_tests::' --lib --locked`.
- Shared differential and route-ownership evidence are recorded on the integration branch; Go stays the production owner and the backtest database/PineTS/provider lifecycle remains outside this slice.
