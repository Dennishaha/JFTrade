# Backtests Run Read Group Ledger

- Group: `backtests-run-read`
- Tier: C: persisted backtest-run projections are local reads, but the Go run store still owns SQLite, recovery, and the in-memory active view; Rust is test-cutover-only.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `BacktestReadSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the backtest database, starts PineTS, syncs market data, or mutates a run.
- Fixture: `tests/fixtures/rust-migration/stage9/backtests-read.json`
- Differential: `TestStage9BacktestsReadFixtureMatchesCurrentGoOwner` plus parameterized tests in `product_backtests_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/backtests` | Returns `{runs: [...]}` using Go's lightweight run projection with result details omitted; ordering and nullable/omitted fields are preserved. | Go's empty in-memory store remains the historical `runs: null` projection; snapshot failures map to `500 BACKTEST_RUN_STORE_FAILED` in the explicit adapter. |
| GET | `/api/v1/backtests/{runId}/status` | Returns only `{id, status}` for the requested run. | Blank/invalid IDs are `400 BAD_REQUEST`; unknown runs are `404 NOT_FOUND`; snapshot failures are `500 BACKTEST_RUN_STORE_FAILED`. |
| GET | `/api/v1/backtests/{runId}` | Returns the complete `RunState`, including request, timestamps, optional result/PnL curves, logs, warnings, and market-data provider metadata. | Blank/invalid IDs are `400 BAD_REQUEST`; unknown runs are `404 NOT_FOUND`; result-store failures are `500 BACKTEST_RUN_STORE_FAILED`. |

Known quirks: list uses the Go lightweight projection and therefore omits `result`; an empty Go store serializes `runs` as `null` rather than `[]`. Run timestamps are fixed in the fixture because the current owner persists wall-clock values. The sync worker route (`GET /api/v1/backtests/sync/{taskId}`) is deliberately excluded because it exposes mutable in-process task lifecycle state and will be cut separately.

All three operations are `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. Backtest POST/DELETE routes, the sync worker, PineTS, market-data providers, and SQLite writes remain outside this slice.
