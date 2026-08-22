# Alerts Write Group Ledger

- Group: `alerts-write`
- Tier: A, mutation operations
- Operations: `POST /api/v1/alerts/price`; `POST /api/v1/alerts/option-events`
- Current production owner: Go product feature API/service and broker `CustomizationService`; Rust has no production owner.
- Current route ownership: unchanged by this worker. The integration branch must register both operations as `cutover-test-only` only after applying the shared product wiring patch.
- Fixture: `tests/fixtures/rust-migration/stage9/alerts-write.json`
- Go reference: `scripts/rust-migration/stage9_alerts_write_reference_test.go`
- Rust leaf/test: `crates/jftrade-engine/src/product_alerts_write_port.rs`; `crates/jftrade-engine/tests/product_alerts_write_tests.rs`
- Differential: `node scripts/rust-migration/check-stage9-alerts-write.mjs`

| Method | Path | Request, response, and side-effect contract | Error branches covered |
| --- | --- | --- | --- |
| POST | `/api/v1/alerts/price` | JSON object, JSON `null`, and empty object are accepted by the Go binder. The action is `set`, feature ID is `alerts.price.set`, query `brokerId` and `accountId` use the first value, and the fake broker records one `ApplyCustomization` call plus the normalized payload state. Success is `200` with the Go provider attribution envelope. | Empty, malformed, and array bodies are `400 BAD_REQUEST` before capability resolution; missing or unavailable broker capability and missing `CustomizationService` are `409 BROKER_CAPABILITY_UNAVAILABLE`; provider 4xx is `PROVIDER_REQUEST_FAILED`; provider/internal failure is `502 BROKER_FEATURE_FAILED`; snapshot rate limiting is `429 MARKET_SNAPSHOT_RATE_LIMITED` with rounded-up `Retry-After`. |
| POST | `/api/v1/alerts/option-events` | Same wire and mutation rules with feature ID `alerts.option_event.set`; repeated `brokerId` preserves the first value; payload and call evidence are fixture-owned. | Same body precedence, capability, provider, internal-failure, and rate-limit mappings as the price route. |

## Three-way review and quirks

The Go reference uses the real Gin route and product feature service with an injected in-memory broker. The Rust replay uses only the consumer-owned `AlertWritePort` fixture adapter; it does not connect OpenD/Futu, a provider, SQLite, or production state. The fixture compares status, `Content-Type`, optional `Retry-After`, complete JSON envelope, normalized query/action payload, and `ApplyCustomization` call count.

quirk: A JSON `null` body is accepted by `ShouldBindJSON` as a nil customization payload; a nil customization result is normalized by the Go service and the `entries` field remains omitted.
范围: `alerts-write` / `POST /api/v1/alerts/price`
证据: Go reference case `price-null-body-nil-result`; fixture response and call trace; Rust fixture replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Preserve null and omitted-field behavior through qualification; do not add Rust-only body validation.

quirk: An empty JSON object is accepted and sent as an empty payload; an empty customization result also omits `entries` from the success data projection.
范围: `alerts-write` / `POST /api/v1/alerts/option-events`
证据: Go reference case `option-events-empty-object`; fixture payload state, call trace, and response; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Keep empty-object/null/omitted distinctions explicit in any future production adapter.

quirk: Repeated `brokerId` query keys use the first value, while the write action receives the resolved broker ID and preserves `accountId` only when non-empty.
范围: `alerts-write` / `POST /api/v1/alerts/option-events`
证据: Go reference case `option-events-success-repeated-broker-query`; fixture action and provider attribution; Rust query replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Keep first-value query semantics in the shared integration adapter; do not normalize duplicate query keys differently.

quirk: The shared Go error mapper labels a customization rate-limit failure `MARKET_SNAPSHOT_RATE_LIMITED` and rounds a 2.5 second delay up to `Retry-After: 3`.
范围: `alerts-write` / `POST /api/v1/alerts/price`
证据: Go reference case `generic-rate-limit-retry-after`; `internal/api/productfeatures/routes.go`; fixture headers and error envelope; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go/integration branch
后续: Preserve the exact code and header until a separate public-contract review approves a correction.

quirk: The first local Go reference attempt passed `*broker.CustomizationAction` to a value helper and failed to compile before fixture generation.
范围: `alerts-write` / Go reference fixture harness
证据: initial `go test` failure in `scripts/rust-migration/stage9_alerts_write_reference_test.go`; corrected helper call; regenerated fixture; Rust replay
分类: harness
判定: deviated
处置: 修复 fixture/harness
风险: low
owner: worker
后续: Keep the corrected reference test and rerun the group checker when the Go owner or fixture changes.

## Test-cutover status

The leaf and fixture slice is ready for an explicit test-only adapter, but it is not `cutover-qualified`. This worker did not change `route-ownership.json`, `product.rs`, `product_api*.rs`, `product_route_assembly.rs`, `product_wire.rs`, package scripts, architecture documents, or any production owner. The integration branch must supply the smallest shared test-cutover wiring and route evidence.

Outstanding Tier A evidence includes repeated-request/idempotency policy, cancellation and timeout fencing, transaction or rollback boundaries, restart recovery, notification/task isolation, four-platform release and signing gates, security review, backup/restore, and final unique-owner/hard-cut approval. No real provider, OpenD, broker lifecycle, or production state mutation is permitted in this leaf.

## Shared integration patch request

The integration branch must apply a minimal shared patch:

1. Add an explicit test-only `AlertWritePort` field and builder path to the product composition types.
2. Register the two exact POST paths only when that injected port is present; keep default profiles unregistered and Go as the production owner.
3. Dispatch both paths through the leaf and map its response into the existing product wire envelope without changing public OpenAPI or shared ownership metadata outside the integration branch.
4. Add both operations to `route-ownership.json` as `cutover-test-only`, with `productionOwner=go`, `goRemovalStatus=retained`, and this checker as evidence.
