# Execution Write Group Ledger

- Group: `execution-write`
- Tier: A mutation; the seven routes can submit, cancel, preview, or query
  broker/product-rule state and therefore require the full write rehearsal.
- Routes:
  - `POST /api/v1/execution/buying-power`
  - `POST /api/v1/execution/combos`
  - `POST /api/v1/execution/combos/previews`
  - `POST /api/v1/execution/combos/{internalOrderId}/cancel`
  - `POST /api/v1/execution/orders`
  - `POST /api/v1/execution/orders/{internalOrderId}/cancel`
  - `POST /api/v1/execution/previews`
- Production owner: Go remains the sole owner of broker/OpenD sessions,
  pre-trade risk, preview/RFQ persistence, execution SQLite, order-update
  workers, notifications, and all order side effects. Rust is an isolated,
  consumer-owned test-only leaf registered only when the explicit product
  test-cutover profile supplies `ExecutionWritePort`.
- Fixture: `tests/fixtures/rust-migration/stage9/execution-write.json`
  (`stage9.execution-write.v1`, 57 cases / 62 requests).
- Go reference:
  `scripts/rust-migration/stage9_execution_write_reference_test.go`. It uses
  the real Gin route handlers and `internal/trading.Service`; broker and
  gateway doubles record normalized Go calls without opening production
  SQLite, connecting OpenD, or submitting an order.
- Rust leaf:
  `crates/jftrade-engine/src/product_execution_write_port.rs`.
- Rust test-cutover durability adapter:
  `crates/jftrade-engine/src/product_execution_write_test_cutover.rs`, included
  only under `cfg(test)` and backed by an isolated SQLite database; it is not
  the production execution store or order-update worker.
- Rust replay:
  `crates/jftrade-engine/tests/stage9_execution_write.rs`.
- Differential:
  `scripts/rust-migration/check-stage9-execution-write.mjs`.

## Contract and rehearsal evidence

| Method | Path | Request/response contract and boundary | Error/recovery coverage |
| --- | --- | --- | --- |
| POST | `/api/v1/execution/buying-power` | Binds the Go `ProductRuleQuery` shape, preserves nullable numeric fields and delegates the product-rule result. | Empty/malformed/array JSON, null body, missing broker, broker timeout, and exact success data are covered. |
| POST | `/api/v1/execution/combos/previews` | Normalizes broker/account/market, combo kind/class, leg sides and IDs, client id, option strategy, RFQ fields and server-side preview output. | Option and event-parlay success, caller-controlled RFQ price rejection, mixed legs, null/empty/malformed/trailing JSON, repeated preview, rate limit and cancellation are covered. |
| POST | `/api/v1/execution/combos` | Requires the Go combo preview/client-id shape and delegates the normalized combo intent to the command gateway. | Missing preview, timeout, cancellation, trailing/null/malformed JSON, repeated submission and event-parlay submission are covered. |
| POST | `/api/v1/execution/combos/{internalOrderId}/cancel` | Trims the bound path id after Gin URI binding and returns the command receipt. | Success, whitespace and blank IDs, unknown/terminal failure, timeout, cancellation and repeated cancel are covered. |
| POST | `/api/v1/execution/orders` | Preserves `tradingEnvironment` precedence over `env`, defaulting/normalizing terms before the Go risk and order gateway. | Success, null/empty/malformed/trailing JSON, missing broker, broker timeout, REAL risk rejection, cancellation and repeated submission are covered. |
| POST | `/api/v1/execution/orders/{internalOrderId}/cancel` | Trims the path id and delegates cancellation without a JSON body. | Success, whitespace and blank IDs, unknown/terminal failure, timeout, cancellation, invalid percent escape and repeated cancel are covered. |
| POST | `/api/v1/execution/previews` | Uses the same single-order binding/normalization as placement but returns the non-submitting preview projection. | Success, null/empty/malformed/trailing JSON, derivative client-id validation and provider timeout/error precedence are covered. |

The Rust leaf parses the first JSON value exactly as Go's `ShouldBindJSON`
does, ignores trailing values, accepts `null` as a zero request, rejects a
non-object body, trims percent-decoded cancel IDs, and forwards cancellation
context metadata through the explicit test port. It does not normalize a
second copy of broker or ledger state.

## Quirks and three-way review

### Go JSON binding accepts null and ignores trailing values

quirk: Go accepts `null` for the bound request structs and decodes only the
first JSON value. A null buying-power body therefore reaches the fake default
broker as a zero `ProductRuleQuery`, while null combo/order bodies reach the
service and fail its domain validation. A valid object followed by another JSON
value is accepted and the second value is ignored.

范围: `execution-write` / all five JSON-body routes

证据: Go reference cases `buying-power-null-body`, `combo-preview-null-body`,
`order-place-null-body`, `order-preview-null-body`, and every
`*-trailing-json-first-value-wins`; fixture envelopes and the Rust replay.

分类: `go-behavior`

判定: `intended`

处置: 复刻，待硬切后若产品要收紧 JSON 输入再单独变更公开契约；本切片不修 Go。

风险: medium

owner: Go API contract / integration branch

后续: retain through the Go deletion gate and keep first-value behavior in any
future authenticated adapter.

三方复核: Go Gin fixture → frozen `execution-write.json` → Rust leaf replay and
dedicated differential all agree.

### Preview route error precedence differs from command routes

quirk: `POST /api/v1/execution/previews` maps every service error to HTTP 400
`BAD_REQUEST`, including a broker timeout (`broker futu: [TIMEOUT] ...`). The
buying-power and combo-preview handlers use `executionCommandError`, so broker
timeout is 504 `BROKER_TIMEOUT` and rate limiting is 429
`BROKER_RATE_LIMITED`.

范围: `execution-write` / `POST /api/v1/execution/previews`,
`POST /api/v1/execution/buying-power`,
`POST /api/v1/execution/combos/previews`

证据: Go handler branches in `internal/api/trading/execution.go`; fixture cases
`order-preview-option-provider-timeout`, `buying-power-timeout`, and
`combo-preview-rate-limited`; Rust error projection.

分类: `go-behavior`

判定: `intended`

处置: 复刻，待硬切后若统一错误映射再单独做契约评审；不在迁移切片内修复。

风险: medium

owner: Go API contract / integration branch

后续: include in the hard-cut error-precedence matrix.

三方复核: Go fixture status/code/message → frozen envelope → Rust replay status,
headers and error envelope are identical.

### Canceled broker calls use the generic gateway error envelope

quirk: When a canceled request reaches a gateway/provider double and returns
`context.Canceled`, Go emits HTTP 502 `BROKER_COMMAND_FAILED` with message
`context canceled`; a broker-native TIMEOUT instead emits 504 `BROKER_TIMEOUT`.
The combo preview, combo place/cancel, order place/cancel cases preserve this
distinction.

范围: `execution-write` / combo preview/place/cancel and order place/cancel

证据: fixture cases `combo-preview-cancelled`, `combo-place-cancelled`,
`combo-cancel-cancelled`, `order-place-cancelled`, and `order-cancel-cancelled`,
alongside the timeout cases.

分类: `go-behavior`

判定: `intended`

处置: 复刻，待硬切后若取消专用状态码获批再单独变更；当前保持 Go observable behavior。

风险: high

owner: Go execution API / integration branch

后续: must remain in the final hard-cut cancellation/timeout matrix; no
cutover-qualified claim until a real adapter proves cancellation fencing.

三方复核: canceled/deadline Go request → frozen status/code/message → Rust
context-bearing leaf replay and differential all agree.

### Whitespace path IDs pass Gin required binding before trim

quirk: `binding:"required"` validates the raw URI segment before the handler
trims it. `%20` therefore reaches the Go cancel service as an empty
`internalOrderId`; the fixture gateway returns the same successful receipt with
an empty ID. This is a likely Go observable bug, intentionally preserved.

范围: `execution-write` / combo and order cancel routes

证据: fixture cases `combo-cancel-blank-id-reaches-service` and
`order-cancel-blank-id-reaches-service`; Go trace records an empty ID and the
Rust leaf passes the trimmed empty string to its port.

分类: `go-behavior`

判定: `intended` for migration compatibility; product defect remains deferred

处置: 复刻，待硬切后由 a separately approved API/security change decide whether
to reject blank IDs; do not fix Go in this slice.

风险: high

owner: Go execution API / integration branch

后续: release-blocker for any contract-tightening decision; retain the evidence
until the final quirk disposition is approved.

三方复核: Go URI binding/handler → frozen empty-ID response and call trace → Rust
percent decode/trim replay and differential agree.

### Invalid percent escapes fail before the cancel service

quirk: An invalid `%zz` path escape is rejected by Gin URI binding with HTTP 400
`BAD_REQUEST` and `internalOrderId is invalid`; the cancel gateway is not
called. The fixture helper must preserve the original `RawPath` while using a
safe placeholder URL for `httptest` construction.

范围: `execution-write` / `POST /api/v1/execution/orders/{internalOrderId}/cancel`

证据: Go fixture case `order-cancel-invalid-percent-is-bound-as-literal`,
`stage9ExecutionWriteRequest`, and the Rust route parser/replay.

分类: `harness`

判定: `intended` harness representation; Go wire behavior is fixed by the
reference result

处置: 保留 RawPath workaround and Rust rejection; do not make the fixture send
an invalid URL to `httptest.NewRequest` directly.

风险: medium

owner: migration harness / integration branch

后续: reuse the RawPath pattern for future execution path corpus additions.

三方复核: Go handler with RawPath-preserving fixture → frozen 400 envelope and
zero gateway calls → Rust parser and differential agree.

### Duplicate command qualification remains outside this leaf

quirk: The fixture intentionally uses stateful service gateway doubles to show
that repeated place requests dispatch twice and repeated cancel requests reach
the Go gateway again before the double returns a generic terminal failure. It
does not instantiate the durable production `ExecutionGateway` plus SQLite
submission ledger, so durable client-order idempotency, crash recovery and
replay fencing remain unproven.

范围: `execution-write` / repeated order/combo place and cancel requests

证据: fixture cases `combo-place-repeated-write-submits-twice`,
`order-place-repeated-write-submits-twice`,
`combo-cancel-repeated-write-is-not-idempotent`, and
`order-cancel-repeated-write-is-not-idempotent`; Go service trace; Rust replay.

分类: `harness`

判定: `unresolved`

处置: 保留 current Go observable service behavior, but block qualification;
the integration branch must add a separate production-ledger adapter/recovery
rehearsal before any owner decision.

风险: release-blocker

owner: integration branch / Go execution ledger owner

后续: prove unique durable owner, idempotency key behavior, failed submission
unknown state, cancel-vs-fill race, restart recovery, and no-double-write before
the group can become cutover-qualified.

三方复核: Go handler/service fixture → frozen repeat trace → Rust leaf replay
agree on the exercised boundary; durable store evidence is explicitly absent.

## Ownership and integration handoff

- The shared integration registers all seven operations only behind an
  authenticated explicit `ExecutionWritePort`; default and read-only profiles
  remain unchanged. `route-ownership.json` records them as
  `cutover-test-only`, with Go still the production owner.
- The current dynamic gate is `278 baseline / 1 shadow / 118
  cutover-test-only / 159 cutover-qualified / 0 remaining / 0 Rust production
  owner`.
- No default profile, production owner, broker/OpenD connection, SQLite write,
  notification/task side effect, Wails binding, OpenAPI asset, or Go deletion
  changed in this wave.

## Verification handoff

- Go fixture reference: `go test scripts/rust-migration/stage9_execution_write_reference_test.go -run '^TestStage9ExecutionWriteFixtureMatchesCurrentGoOwner$' -count=1`.
- Rust replay: `cargo test -p jftrade-engine --test stage9_execution_write -- --nocapture`.
- Differential: `node scripts/rust-migration/check-stage9-execution-write.mjs`.
- Authenticated Go loopback rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestExecutionWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1` passed, covering private bearer/internal protocol, browser context forwarding, success/error/timeout/cancellation, crash fail-closed behavior, Go rollback, restart recovery, and unchanged settings bytes.
- Authenticated Rust product replay: `cargo test -p jftrade-engine --lib product::tests::execution_write_product_tests -- --nocapture` passed, covering explicit registration, browser auth/CSRF fencing, unavailable/error/recovery, all seven route projections, repeated order forwarding, restart, and unchanged settings bytes.
- Rust durable test-cutover replay: `cargo test -p jftrade-engine --test stage9_execution_write -- --nocapture` passed, covering isolated order/combo placement, preview and cancel persistence, allocator rollback on event failure, concurrent cancellation fencing, canceled-context rejection, close/reopen recovery, and all seven route projections.
- Rust durable product replay: `cargo test -p jftrade-engine --lib execution_sqlite_test_cutover_replays_transport_and_restart -- --nocapture` passed, covering authenticated transport, restart recovery, and unchanged production settings/state boundaries through the isolated adapter.
- Integration verification: dedicated differential, Rust leaf replay, product
  route-isolation test, isolated durability replay, route coverage, layout,
  focused Clippy, `node --check`, `git diff --check`, `check:quick`, and full
  `check:rust` pass after this
  wave's final composition.
- This is a rehearsal only; A-tier unique-owner switch, production ledger
  fencing, broker/OpenD live, durable recovery, four-platform signed release,
  security/SBOM, backup/restore, and hard-cut gates remain open.
