# Market-Data Prediction Read Group Ledger

- Group: `market-data-prediction-read`
- Tier: B; every read depends on prediction-market broker eligibility, Provider/OpenD capability routing, or live contract data semantics.
- Operations: 12 GET routes.
- Production owner: Go remains the only production owner of account eligibility, capability selection, Provider/OpenD lifecycle, caching, prediction subscriptions, and all writes. Rust receives only a complete JSON projection through `MarketDataPredictionReadSnapshotPort` in an explicit integration-owned test-cutover wiring.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-prediction-read.json`
- Go reference: `scripts/rust-migration/stage9_market_data_prediction_read_reference_test.go`
- Rust replay: `crates/jftrade-engine/src/product_market_data_prediction_read_tests.rs` and `crates/jftrade-engine/tests/stage9_market_data_prediction_read.rs`
- Differential: `node scripts/rust-migration/check-stage9-market-data-prediction-read.mjs`

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/prediction/categories` | Prediction discovery query; Go-normalized `FeatureResult` is captured as a complete JSON value. | Eligibility `403 PREDICTION_MARKET_INELIGIBLE`; missing capability `409 BROKER_CAPABILITY_UNAVAILABLE`; provider failures preserve Go status/code/message. |
| GET | `/api/v1/market-data/prediction/combos/eligible-events` | `FeaturePredictionComboEligible`, fixed prediction segment and event-contract class. | Same Go error mapping; no combo quote or subscription side effect. |
| GET | `/api/v1/market-data/prediction/competitions` | Prediction discovery query with existing query parameters passed through. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/candles` | `FeaturePredictionHistory` with `operation=candles`; raw path/query is handed to the snapshot port. | Same Go error mapping; no live subscription is created. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/candles/history` | `FeaturePredictionHistory` with `operation=historical` and existing range parameters. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/milestones` | Prediction discovery milestone projection for one contract. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/order-book` | `FeaturePredictionDepth` with `operation=order_book`; depth values remain provider-owned. | Same Go error mapping; no subscription mutation. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/snapshot` | `FeaturePredictionSnapshot` with `operation=snapshot`; complete provider envelope is preserved. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/contracts/{code}/ticks` | `FeaturePredictionHistory` with `operation=ticks`; complete provider envelope is preserved. | Same Go error mapping; no subscription mutation. |
| GET | `/api/v1/market-data/prediction/events` | Prediction discovery event list with existing filters. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/events/{eventId}/contracts` | Prediction discovery contracts query with `eventId` carried by the path-derived instrument context. | Same Go error mapping. |
| GET | `/api/v1/market-data/prediction/series` | Prediction discovery series query with existing filters. | Same Go error mapping. |

## Three-Way Review

The Go owner, generated fixture, and Rust replay all use the raw request path/query boundary. Rust does not recreate Go's `PredictionMarketReader`, account eligibility, page-size normalization, market/product defaults, cache policy, or Provider/OpenD lifecycle. The port returns `serde_json::Value` so provider field order, omitted fields, nulls, empty arrays, numeric values, and future fields remain opaque and unchanged.

### Reviewed quirks

quirk: Prediction discovery and contract reads require an eligible `FUTUINC` account with US authority before the broker reader is invoked.
范围: `market-data-prediction-read` / all 12 GET routes
证据: Go reference cases `categories-success` and `categories-ineligible-account`; `internal/productfeatures/service_helpers.go`; Rust fixture replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Keep eligibility in the Go owner until the prediction capability itself is cut over; Rust must return the captured error and never infer eligibility.

quirk: Go route binding upper-cases path-derived contract/event identifiers, supplies fixed prediction market/product defaults, injects operation names, and normalizes omitted page size to `100` before calling the broker.
范围: `market-data-prediction-read` / route query projection
证据: `internal/api/productfeatures/routes.go`, fixture `providerCall` fields, and Rust raw path/query replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Do not parse or normalize these values in the snapshot adapter; regenerate the fixture if Go route behavior changes.

quirk: `asOf` and provider `resolvedAt` are runtime timestamps and are normalized to `fixture-time` only by the reference fixture for deterministic replay.
范围: all successful prediction-read responses
证据: Go reference normalization and `market-data-prediction-read.json`.
分类: fixture
判定: intended
处置: 修复 fixture/harness
风险: low
owner: worker
后续: Keep normalization limited to those timestamp keys and retain all other response fields exactly.

quirk: Go's in-memory `FeatureQuery.Params` preserves integer query values as `int64`, while decoding the generated JSON fixture represents those same values as `float64`; a direct Go fixture comparison therefore reported false drift for `depth=10`.
范围: `market-data-prediction-read` / `providerCall.params`
证据: initial `TestStage9MarketDataPredictionReadFixtureMatchesCurrentGoOwner` failure for `order-book-success`; the reference test now JSON-round-trips provider-call params before comparison, and the generated fixture plus Rust replay agree.
分类: fixture
判定: deviated
处置: 修复 fixture/harness; do not change the public response or Rust numeric semantics
风险: low
owner: worker
后续: Keep the canonicalization confined to fixture evidence fields and rerun the focused Go/Rust differential when query capture changes.

quirk: Prediction-read handler tests run below Go's authenticated HTTP middleware, so operation-level fixture coverage has no `401` branch; `400` is also not emitted by these twelve current GET bindings. Rust group tests cover `404` for routes outside the read group, while authentication remains the shared transport owner's gate.
范围: `market-data-prediction-read` / focused fixture and transport boundary
证据: `internal/api/productfeatures/routes.go`, Go fixture cases, `crates/jftrade-engine/tests/stage9_market_data_prediction_read.rs`, and existing `jftrade-api` transport tests.
分类: harness
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: integration for transport; Go for production route behavior
后续: Do not add synthetic `400`/`401` operation cases or change shared auth handling in this worker; include the shared transport/auth evidence in integration cutover review.

quirk: The worker handoff intentionally excluded shared composition files, so the group source was initially not included in the Rust product composition and its 12 ownership records remained `remaining` until integration.
范围: `market-data-prediction-read` / integration boundary
证据: initial worker write set, current `product.rs`/`product_route_assembly.rs` assembly points, and the serial integration patch.
分类: harness
判定: deviated
处置: apply the requested integration patch before test-cutover registration; keep Go as owner
风险: medium
owner: integration branch
后续: Keep composition changes serial on the integration branch and rerun route coverage plus the group differential after future assembly changes.

## Ownership

The integration branch registers all 12 routes only through the explicit test-cutover snapshot port. They are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated wire/error/timeout/crash/restart rehearsal. That rehearsal uses missing-broker projections with Futu explicitly disabled on loopback ports `1/2`; no Rust production owner, default route registration, subscription mutation, Provider/OpenD connection, or write path is introduced.
