# Execution Read Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `execution-read`
- Tier: B; list/detail reads synchronously depend on order-update refresh and the execution SQLite ledger, while events are ledger-backed.
- Owner: Go remains the production owner of auth, `DatabaseExecution` guard, order-update worker, broker/OpenD lifecycle, execution SQLite and all writes. Rust accepts a complete `ExecutionReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring.
- Fixture: `tests/fixtures/rust-migration/stage9/execution-read.json`
- Differential: `TestStage9ExecutionReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_execution_read_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/execution/orders` | Preserves permissive `scope`, `brokerId`, `tradingEnvironment`, `accountId`, and `market` filters; output is `{orders: []}` with nullable order fields and Go ordering. | Store failure is `500 LIST_ORDERS_FAILED`; worker/provider refresh failures degrade to the store projection. |
| GET | `/api/v1/execution/orders/{internalOrderId}` | Trims the ID, forces active refresh, optionally refreshes non-terminal broker-referenced history, returns `{order,recentEvents,checkedAt}` and keeps only the last 10 recent events. | Missing order is `404 ORDER_NOT_FOUND`; list/event store failure is `500 GET_ORDER_FAILED`. |
| GET | `/api/v1/execution/orders/{internalOrderId}/events` | Trims the ID and returns the full chronological `{internalOrderId,events}` ledger projection. | Unknown IDs remain `200` with an empty array; store failure is `500 GET_ORDER_EVENTS_FAILED`. |

Known quirks frozen in the fixture:

- `checkedAt` and the common response timestamp are normalized to `fixture-time` for deterministic replay; no response field is otherwise changed.
- `scope` and filter values are intentionally permissive; Rust does not add validation beyond the Go route.
- Details use `404` for a missing order, while events use `200` for an unknown ID with `events: []`.

All three operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. Rust does not activate a provider, acquire the execution writer lease, or mutate the ledger; Go remains the sole execution SQLite and order-update owner.
