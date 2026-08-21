# Execution Read Recon

This is an exclusive reconnaissance note for the `execution-read` route group.
The eventual group ledger can fold these facts into `execution-read.md`; this
note intentionally does not change route ownership, product assembly, or the
shared Stage 9 differential runner.

## Scope and tier

- Tier: B. The list and detail projections can synchronously discover accounts,
  refresh broker orders, update the execution ledger, and use the OpenD/provider
  lifecycle. The events projection is ledger-backed, but it belongs to this
  broker-coupled group.
- Operations:
  - `GET /api/v1/execution/orders`
  - `GET /api/v1/execution/orders/{internalOrderId}`
  - `GET /api/v1/execution/orders/{internalOrderId}/events`
- Current ownership: all three are `remaining`, `productionOwner=go`, and
  `goRemovalStatus=retained` in `route-ownership.json` (lines 897-933).
- Production owner must remain Go. Rust can only expose these routes in an
  explicit `ProductConfig::test_cutover` profile through a complete,
  consumer-owned snapshot port. The port must not open SQLite, discover
  accounts, activate a broker, subscribe to OpenD, or mutate the execution
  ledger.

## Route binding, authentication, and database guard

Go registers the three GET handlers in `internal/api/trading/execution.go`
(`RegisterExecutionRoutes`, lines 27-42). They are mounted under
`databaseguard.Groups.Execution`, which checks `DatabaseExecution` before the
handler. If that database is unavailable, the guard returns HTTP 503 with code
`DATABASE_INCOMPATIBLE` and a message containing the database id; the handler is
never entered (`internal/app/apiserver/databaseguard/groups.go`).

The router applies CORS, desktop-token handling, web access, and the global
`middleware.Auth` before route dispatch. GET requests require a valid web
session or trusted desktop host, but do not require CSRF. There is no
execution-specific role/permission check. Origin validation still applies when
an Origin/Referer is supplied. Preserve this boundary at the composition root;
the snapshot port should not implement a second auth policy.

Successful and handler-error responses use the common envelope:

```json
{"ok":true,"data":{},"timestamp":"<RFC3339Nano>"}
{"ok":false,"error":{"code":"...","message":"..."},"timestamp":"<RFC3339Nano>"}
```

The timestamp is generated at response time and must be normalized to the
fixture clock by the differential harness, as with the other Stage 9 groups.

## Go observable behavior

### List

`handleExecutionOrders` reads `scope`, `brokerId`, `tradingEnvironment`,
`accountId`, and `market` without rejecting unknown or malformed values. Only a
case-insensitive, whitespace-trimmed `scope=ACTIVE` sets `activeOnly=true`.
The service trims broker/account values, uppercases environment and market,
and fills an omitted environment from the settings default (normally
`SIMULATE`; `servercore.defaultTradingEnvironment` is the source).

`ListExecutionOrders` first calls `SyncOrderUpdates(ctx, false, activeOnly)` and
then reads the durable order store. With an active scope, the worker can use a
fresh active-order cache; otherwise it queries current orders and then history.
The default history lookback is three days, overridden by execution settings.
Account discovery, subscription setup, current-order fetch, and history fetch
are all best effort: worker errors update worker state and skip that query, then
the store projection is still returned. A nil/inactive worker performs no
refresh and the store result is returned.

Store filtering is case-insensitive for broker, trading environment, and
market, but account ID comparison is exact after trimming the request value.
Order output is sorted descending by `updatedAt`, then `createdAt`, then
`internalOrderId` (`internal/store/trading/ledger.go`, lines 18-38). Empty
results are encoded as `{"orders":[]}`.

The route maps a store error to HTTP 500 code `LIST_ORDERS_FAILED`; normal
worker/provider failures alone do not become an HTTP error because the worker
degrades and the subsequent store read succeeds.

### Single-order details

The path id is required and whitespace is trimmed. A missing/blank path binding
returns HTTP 400 `BAD_REQUEST` with message `internalOrderId is invalid`.
The service rejects an empty trimmed id with `internalOrderId is required`,
although the normal Gin binding path reaches the former handler-level error.

Before reading the order, details calls `SyncOrderUpdates(ctx, true, true)`,
bypassing the active cache so a just-filled or just-cancelled order is fresh.
If the order is non-terminal and has a broker id or extended broker id, it then
calls `SyncExecutionOrderHistory` for the order's broker/environment/account/
market and rereads the store. Missing or incomplete query fields cause that
history refresh to be skipped. The final response is `{order, recentEvents,
checkedAt}`. `recentEvents` preserves event order and is truncated to the last
10 entries, while the full event route is unbounded.

Missing order maps to HTTP 404 code `ORDER_NOT_FOUND` and message
`execution order not found`. Any list or event-store failure maps to HTTP 500
code `GET_ORDER_FAILED` with the underlying error message. `checkedAt` is
RFC3339Nano UTC and must be fixture-normalized.

### Events

The path id is trimmed by the same binding helper. A valid but unknown id is
not a 404: the store returns `{internalOrderId:<id>,events:[]}` and the handler
returns 200. The store error maps to HTTP 500 code `GET_ORDER_EVENTS_FAILED`.
Events are cloned from the durable ledger and retain insertion chronology. On
SQLite reload, events are loaded ordered by `internal_order_id ASC,
created_at ASC, id ASC`; per-order retrieval therefore remains chronological.
Event status fields are canonicalized on load depending on whether the event
type is `BROKER_*` or command-side. `payloadJson` is always a string; nil or
unmarshalable event payloads are persisted as `{}`.

## Snapshot-port contract recommendation

The consumer-owned Go adapter should return all three projections together or
through one coherent read boundary, with a fixed fixture clock only in tests:

```text
list(query) -> ExecutionOrders
details(internalOrderId) -> ExecutionOrderDetails
events(internalOrderId) -> ExecutionOrderEvents
```

The adapter must capture the already-computed Go result after any refresh. It
must not reproduce `OrderUpdatesWorker` or invoke a second store. Preserve
nullable fields on `ExecutionOrder` and `ExecutionOrderEvent`; do not convert
null pointers into omitted fields. `normalizedRequest` is omitted only when
empty, and order legs/optional leg fields follow their existing `omitempty`
tags. The outer `orders` and `events` slices must remain present as empty arrays.

## Existing evidence and differential cases

- `internal/app/apiserver/servercoretest/exec_routes_test.go` exercises a
  complete place/list/events/cancel flow. The initial events are ordered
  `COMMAND_SUBMISSION_PREPARED`, `COMMAND_PLACE_ACCEPTED`; after cancellation
  `COMMAND_CANCEL_ACCEPTED` is appended.
- `internal/app/apiserver/servercoretest/execution_routes_test.go` covers list
  filtering by environment, broker, account, and market, default environment
  from settings, and current/history broker synchronization. Its live broker
  cases use the Futu testkit only; do not use a real OpenD in migration tests.
- `internal/api/trading/execution_validation_contracts_test.go` covers list and
  event store failures, ACTIVE vs default sync, missing path ids, and detail
  missing/order-store failures.
- `internal/trading/execution_test.go` covers detail refresh, missing order,
  bounded recent events, and the service facade ports.
- `internal/store/trading/ledger_test.go` freezes descending order sorting and
  case-insensitive filters; `persistence_query_plan_test.go` freezes the
  indexed event load and per-order chronology.

Recommended fixture rows/cases for the group-level corpus:

1. A successful list with two orders that exercises updated/created/id
   tie-break sorting, nullable fields, all four filters, and `scope=ACTIVE`.
2. Empty list with omitted scope and an omitted environment using the fixture
   default `SIMULATE`.
3. Successful details with a non-terminal broker-referenced order and more
   than 10 events; freeze only `checkedAt` and assert the last 10 events.
4. Successful details for a terminal order, proving no history refresh is
   required, plus missing-order 404.
5. Successful events for an unknown id (empty array), malformed/missing id 400,
   and event-store failure 500.
6. Execution database unavailable at the group guard (503
   `DATABASE_INCOMPATIBLE`) and unavailable snapshot port in Rust's explicit
   test-cutover profile (the eventual Rust error code should be chosen by the
   integration ledger and compared against the Go adapter failure policy).

## Quirk status

No Go-vs-Rust differential was run because no Rust execution-read adapter exists
yet. Potential differences to resolve as `unresolved` during implementation:

- `scope` and filter query values are permissive (no query 400); changing this
  would be a wire regression.
- list/details perform hidden broker refreshes and may return stale-but-successful
  ledger data after provider failure; an eager Rust port failure would diverge.
- an unknown id on the events route is 200 with an empty array, unlike details'
  404; do not normalize the two behaviors.
- details truncates only `recentEvents` to 10; the events route is unbounded.
- all clock-dependent `timestamp`/`checkedAt` fields require deterministic
  fixture normalization.
