# Backtests Sync Read Group Ledger

- Group: `backtests-sync-read`
- Tier: B: the route projects mutable in-process sync-task lifecycle state; Rust receives a snapshot only in explicit test-cutover wiring.
- Owner: Go remains the production owner of the sync worker, task store, Provider/OpenD lifecycle, cancellation, and market-data writes. Rust never starts or cancels a task.
- Fixture: `tests/fixtures/rust-migration/stage9/backtests-sync-read.json`
- Differential: `TestStage9BacktestsSyncReadFixtureMatchesCurrentGoOwner` plus parameterized tests in `product_backtests_sync_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/backtests/sync/{taskId}` | Returns the concurrency-safe `SyncProgress` snapshot, preserving empty strings, zero counters, optional `error`, and RFC3339Nano timestamps. | Blank/invalid IDs are `400 BAD_REQUEST`; unknown or already-finished tasks are `404 NOT_FOUND`; an unavailable explicit snapshot adapter fails closed as `500 BACKTEST_SYNC_TASK_STORE_FAILED` in Rust test-cutover wiring. |

Known quirks: Go returns `404 NOT_FOUND` when the task store is absent because `GetSyncProgress` treats a nil store as no task. The fixture freezes wall-clock timestamps and the queued state’s empty `updatedAt`; this route is intentionally separate from backtest run projections because its values change during the worker lifecycle.

The operation is `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. POST/DELETE sync operations, Provider/OpenD acquisition, cancellation, SQLite writes, and the background worker remain outside this slice.
