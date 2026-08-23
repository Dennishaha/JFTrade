# Brokers mutation route-group ledger

- Group: `brokers-write`
- Tier: A. These operations place/cancel orders or change the broker trading
  session state.
- Operations: 3: `DELETE /api/v1/brokers/{brokerId}/orders`,
  `POST /api/v1/brokers/{brokerId}/orders`, and
  `POST /api/v1/brokers/{brokerId}/unlock`.
- Dynamic baseline at worker start: `278 baseline / 26 shadow / 242
  cutover-test-only / 0 cutover-qualified / 10 remaining / 0 Rust production
  owner`.
- Handoff-time shared gate observed after the concurrent market-data group:
  `26 shadow / 248 cutover-test-only / 0 cutover-qualified / 4 remaining / 0
  Rust production owner`; the three broker mutation routes in this ledger are
  still among the remaining operations.
- Current ownership: these three routes remain `remaining` in the shared
  `route-ownership.json`; this worker deliberately did not edit that shared
  catalog. Go remains the sole production owner. The Rust code is a leaf and
  replay boundary only; integration must register it as explicit
  authenticated `test-cutover` before any status change.
- Fixture: `tests/fixtures/rust-migration/stage9/brokers-write.json`.
- Go reference: `scripts/rust-migration/stage9_brokers_write_reference_test.go`.
- Rust leaf/replay: `crates/jftrade-engine/src/product_brokers_write_port.rs` and
  `crates/jftrade-engine/tests/stage9_brokers_write.rs`.
- Dedicated differential: `node scripts/rust-migration/check-stage9-brokers-write.mjs`.

## Contract ledger

All fixture responses use the Go envelope and normalized timestamp with
`Content-Type: application/json; charset=utf-8`. The Rust timestamp is an
injected replay value; an eventual transport adapter must source it at the
transport boundary. The leaf has no broker, OpenD, SQLite, order-state,
notification, or risk-state capability.

| Method | Path | Request and success behavior | Error precedence and mapping |
| --- | --- | --- | --- |
| `DELETE` | `/api/v1/brokers/{brokerId}/orders` | Binds the broker URI and `tradingEnvironment`, `accountId`, and `market` query values; defaults `market` to `HK`. Binds `CancelOrdersRequest`, ignores unknown fields, accepts `null` and an empty object as zero orders, and accepts the first JSON value when JSON trails it. Success is `200` with `cancelled` equal to the submitted item count. | Malformed raw query or body/field shape is `400 BAD_REQUEST`; broker resolution is `404` for a missing broker or `503 NO_BROKER`; missing trading capability is `503 NO_TRADING`; broker/cancellation/context errors are `502 CANCEL_FAILED`. |
| `POST` | `/api/v1/brokers/{brokerId}/orders` | Binds `PlaceOrderRequest`, ignores unknown fields, accepts `null` and the first value of trailing JSON, and passes typed zero values for omitted/null fields. `tradingEnvironment` is trimmed, uppercased, and defaults to `SIMULATE`; `market` defaults to `HK`. Success is `200` with the submitted order projection and `placedAt`. | Malformed raw query or body/field shape is `400 BAD_REQUEST`; broker resolution is `404`/`503 NO_BROKER`; missing trading capability is `503 NO_TRADING`; pre-trade rejection is `409 PRE_TRADE_RISK_REJECTED`; broker/cancellation/context errors are `502 PLACE_ORDER_FAILED`. |
| `POST` | `/api/v1/brokers/{brokerId}/unlock` | Binds `UnlockTradeRequest`, ignores unknown fields, accepts `null` and the first value of trailing JSON. `unlock:false` is still forwarded to the broker. Query `market` defaults to `HK`; unlike place, the service preserves the trimmed environment spelling. Success is `200` with `unlocked:true` and `unlockedAt`. | Malformed raw query or body/field shape is `400 BAD_REQUEST`; broker resolution is `404`/`503 NO_BROKER`; missing unlock capability is `503 NOT_SUPPORTED`; broker/cancellation/context errors are `502 UNLOCK_FAILED`. |

The path/query/body order is intentional: URI binding, raw-query validation,
JSON binding, then service/port dispatch. A request rejected at the HTTP
binding layer does not cross the injected mutation port. A validly bound
request crosses the service adapter even when Go then rejects broker
resolution, capability, or risk; `portCall` in the fixture denotes that
adapter boundary, while `goCalls` denotes a real broker method invocation.

## Evidence and three-way quirk review

The Go reference uses the real Gin handlers and trading service with recording
broker doubles. It never opens OpenD, a production database, a production
trading session, or a real provider. The Rust replay consumes the same frozen
fixture, checks status/header/envelope, port-call count, decoded query/body
shape, actual broker-call observations, method/path isolation, repeated-call
behavior, and unavailable-port fencing.

Fixture coverage is `65 cases / 68 requests / 45 adapter port calls / 35 Go
broker method calls`:

- `POST orders`: 24 cases / 25 requests / 17 adapter calls / 13 broker calls.
- `DELETE orders`: 21 cases / 22 requests / 14 adapter calls / 11 broker calls.
- `POST unlock`: 20 cases / 21 requests / 14 adapter calls / 11 broker calls.

The status corpus covers success, binding `400`, broker `404`, unavailable
broker/capability `503`, pre-trade `409`, and broker/context `502` outcomes.

### Quirks

quirk: JSON `null` bodies are accepted by all three Gin handlers; place and
unlock reach the service with zero-value request fields, while cancel reaches
it with zero orders and succeeds with `cancelled: 0`.
范围: `brokers-write` / all three routes
证据: fixture cases `place-null-body-reaches-broker`,
`cancel-null-body-is-zero-operation`, `unlock-null-body-reaches_broker`; Go
reference; `brokers_write_fixture_replays_go_wire_for_all_three_routes`.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: preserve the compatibility exception in any authenticated adapter;
review separately before changing public behavior.

quirk: Gin's JSON binder accepts the first JSON value and ignores trailing
JSON, and it ignores unknown object fields.
范围: `brokers-write` / all three routes
证据: fixture `*-trailing-json-first-value-wins` and `*-unknown-fields-ignored`
cases; Go reference; Rust fixture replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go API owner / Rust adapter
后续: retain first-value and unknown-field coverage before owner cutover.

quirk: Place requests normalize missing/blank environment to `SIMULATE` and
missing/blank market to `HK`; cancel/unlock retain the service's read-query
environment semantics while still defaulting market to `HK`.
范围: `brokers-write` / query binding and service dispatch
证据: `place-success-query-normalization`, `place-success-query-defaults`,
`place-duplicate-query-first-values-win`, and the success query cases; Go
reference and Rust input-shape replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go trading service / integration branch
后续: keep query normalization and duplicate-key tests in the product
differential.

quirk: Duplicate query keys use the first value, even when that first value is
empty; a later non-empty value is not substituted.
范围: `brokers-write` / all three routes
证据: fixture `place-duplicate-query-first-values-win`; Go reference and Rust
replay after the explicit seen-key fix.
分类: rust-implementation
判定: deviated
处置: 修复 Rust 使其匹配 Go
风险: medium
owner: Rust worker
后续: retain the duplicate-key case when the adapter is wired.

quirk: Go's field-binding errors use different source tokens for string,
number, bool, array, and object inputs; non-integral `orderId` errors include
the numeric value, and nested errors use `CancelOrderItem.orders.orderId`.
范围: `brokers-write` / JSON body binding
证据: fixture cases `place-string-quantity-rejected`,
`place-number-symbol-rejected`, `cancel-number-order-id-rejected`,
`cancel-number-broker-order-id-rejected`, and the initial Rust replay failure;
Go reference and final Rust replay.
分类: rust-implementation
判定: deviated
处置: 修复 Rust 使其匹配 Go
风险: medium
owner: Rust worker
后续: preserve typed-token cases and add any new DTO fields to the same
error corpus.

quirk: The Go trace serializes `UnlockTradeRequest.PasswordMD5` with
`omitempty`, so an empty value is absent from `goCalls` even though the
decoded service request contains the zero string.
范围: `brokers-write` / `POST /api/v1/brokers/{brokerId}/unlock`
证据: fixture `unlock-false-is-still-submitted` and
`unlock-null-body-reaches_broker`; Go trace canonicalization and Rust replay.
分类: harness
判定: intended
处置: 保留 fixture/harness 语义，不改变 HTTP 行为
风险: low
owner: Go reference harness
后续: keep `goCalls` as a canonical observation, not as a second wire
contract.

quirk: Timeout, canceled, and deadline broker calls all map to generic HTTP
`502` operation failures with the underlying Go error text; no retry or
second mutation is issued.
范围: `brokers-write` / all three routes
证据: all `*-timeout`, `*-cancelled`, and `*-deadline` cases; Go reference and
Rust replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go trading owner / integration branch
后续: prove request cancellation propagation, join, and recovery before
qualification.

quirk: Repeated place, cancel, and unlock requests are forwarded independently
and are not idempotent; repeated calls can repeat trading/session side effects.
范围: `brokers-write` / all three routes
证据: `place-repeat-submits-twice`, `cancel-repeat-is-not-idempotent`, and
`unlock-repeat-is-not-idempotent`; Go observations and Rust replay.
分类: go-behavior
判定: unresolved
处置: 复刻 captured behavior; block qualification until duplicate-request,
transaction, and owner-fencing semantics are reviewed.
风险: release-blocker
owner: Go/integration branch
后续: resolve before any production owner switch; do not add local Rust
deduplication in this slice.

quirk: `portCall` and an actual broker method call are different boundaries:
validly bound requests can enter the service adapter and then stop at broker
resolution, capability, or risk checks without calling a broker method.
范围: `brokers-write` / missing-broker, no-broker, no-trading, risk, and
unsupported-unlock cases
证据: fixture `portCall` versus `goCalls`/`observation`; Go reference; Rust
`FixturePort` replay.
分类: harness
判定: intended
处置: 保留边界区分，禁止把 adapter replay 当成真实 broker side effect
风险: high
owner: integration branch
后续: product differential must keep the port unavailable/owner-failure
fence and must not double-dispatch.

## A-tier qualification blockers and owner fencing

- No default profile, public route catalog, route ownership entry, production
  owner, OpenD connection, SQLite writer, trade-state writer, notification,
  or real broker adapter was added by this worker.
- The only Rust mutation capability is the explicitly injected
  `BrokersWritePort`; with no port the leaf fails closed as `503
  BROKERS_WRITE_UNAVAILABLE` and does not retry or fall back to Go.
- Integration must wire the port only behind authenticated test-cutover and
  keep Go as the unique order/session/risk owner. A production switch must be
  one composition-root choice with rollback and no dual dispatch.
- Durable idempotency/replay policy, transaction/rollback boundaries,
  cancellation and timeout joins, restart/recovery, lock release, provider or
  OpenD qualification, security/signing, four-platform release, and hard-cut
  evidence are release blockers. This worker does not claim
  `cutover-qualified`.

## Verification and handoff

- Go fixture generation with `JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES=1` passed;
  the subsequent no-update drift replay passed.
- The focused Rust replay passed 4 tests against the final 65-case/68-request
  fixture. The dedicated differential
  `node scripts/rust-migration/check-stage9-brokers-write.mjs` also passed both
  the Go reference drift check and the workspace Rust replay.
- The current workspace already contains unrelated market-data integration
  edits in shared `crates/jftrade-engine/src/product*.rs`; this worker did not
  modify, stage, or revert them. They are outside this handoff.
- Shared `route-ownership.json`, product route assembly, product differential,
  module map, architecture docs, generated code, and default profile are
  intentionally untouched. The integration branch must add the three route
  entries, authenticated test-cutover wiring/route-isolation evidence, and
  its dynamic ledger update.
