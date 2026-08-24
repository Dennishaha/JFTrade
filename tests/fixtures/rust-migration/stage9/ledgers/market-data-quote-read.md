# Market-Data Quote Read Group Ledger

- Group: `market-data-quote-read`
- Tier: B; the routes depend on Provider/OpenD freshness, broker capability selection, or live subscription state even though the HTTP methods are GET.
- Operations: 10 GET routes.
- Production owner: Go remains the only production owner of Provider/OpenD lifecycle, cache freshness, broker selection, subscription demand, and all market-data writes. Rust receives only complete JSON projections through `MarketDataQuoteReadSnapshotPort` in explicit integration-owned test-cutover wiring.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-quote-read.json`
- Go reference: `scripts/rust-migration/stage9_market_data_quote_read_reference_test.go`
- Rust replay: `crates/jftrade-engine/src/product_market_data_quote_read_tests.rs`
- Differential: `node scripts/rust-migration/check-stage9-market-data-quote-read.mjs`

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/broker-queue/{instrumentId}` | Broker feature projection with Go query selection and complete provider envelope preserved. | Capability unavailable, provider failure, warming and busy preserve Go status/code/message and `Retry-After`. |
| GET | `/api/v1/market-data/candles/{market}/{symbol}` | Historical candles projection; period, limit, range and repeated session query semantics remain Go-owned. | Invalid query, provider failure and provider readiness errors preserve Go behavior. |
| GET | `/api/v1/market-data/capital-flow/{instrumentId}` | Broker feature projection with explicit broker and market query semantics. | Capability/provider errors preserve the Go envelope. |
| GET | `/api/v1/market-data/depth/{market}/{symbol}` | Order-book projection; `num` parsing and decimal wire values remain provider-owned. | Invalid depth, provider busy and capability errors preserve Go status/message and retry metadata. |
| GET | `/api/v1/market-data/instruments/{instrumentId}/profile` | Broker instrument-profile feature projection. | Capability/provider errors preserve Go behavior. |
| GET | `/api/v1/market-data/intraday/{instrumentId}` | Broker intraday feature projection. | Capability/provider errors preserve Go behavior. |
| GET | `/api/v1/market-data/securities/{market}/{symbol}` | Provider security details projection; Go keeps market-qualified symbol normalization. | Provider failure preserves `MARKET_SECURITY_DETAILS_FAILED`. |
| GET | `/api/v1/market-data/snapshots/{market}/{symbol}` | Snapshot projection with `refresh` query semantics and complete quote envelope. | Invalid query, unsupported capability, provider warming and provider failure preserve Go status/message and retry metadata. |
| GET | `/api/v1/market-data/subscriptions` | Read-only consumer subscription state and quota projection; no demand or subscription mutation occurs in Rust. | The snapshot port can report unavailable/failure; Go remains the sole demand owner. |
| GET | `/api/v1/market-data/ticks/{instrumentId}` | Broker tick feature projection with page-size and provider selection normalized by Go. | Capability/provider errors preserve Go status/message and retry metadata. |

## Three-Way Review

The Go owner, generated fixture, and Rust replay use the raw request path/query boundary. Rust does not recreate Provider/OpenD lifecycle, freshness, market normalization, broker selection, decimal conversion, quota accounting, or subscription demand. The port returns `serde_json::Value` so omitted fields, nulls, empty arrays, numeric strings, provider metadata, and future fields remain opaque and unchanged.

### Reviewed quirks

quirk: `GET /api/v1/market-data/subscriptions` is a read projection of live demand/quota state, but the same domain also has subscription POST/DELETE/heartbeat/release routes.
范围: `market-data-quote-read` / `GET /api/v1/market-data/subscriptions`
证据: Go fixture `subscriptions-empty`, route ownership records for the subscription mutation routes, and the Go market-data service boundary.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until market-data owner cutover
后续: Keep the Rust port read-only; do not register or implement subscription mutation from this group.

quirk: Go normalizes market-qualified symbols, validates `refresh`, candle and depth query values, and maps Provider/OpenD errors to localized/status-specific envelopes before producing the JSON projection.
范围: `market-data-quote-read` / all 10 GET routes
证据: Go reference cases, `market-data-quote-read.json`, and the focused Go fixture test.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: Do not parse or normalize these values in the Rust snapshot adapter; regenerate the fixture if the Go owner changes.

quirk: Runtime quote timestamps (`resolvedAt`, `observedAt`, and `quoteAt`) are normalized to `fixture-time` only in the reference harness.
范围: successful snapshot and provider-backed quote responses
证据: Go reference normalization and `market-data-quote-read.json`.
分类: fixture
判定: intended
处置: 修复 fixture/harness
风险: low
owner: integration
后续: Keep timestamp normalization limited to fixture evidence and retain all other response fields exactly.

quirk: The focused Go handler fixture runs below shared authentication middleware, so it does not synthesize `401`; shared transport/auth tests remain the authority for that boundary.
范围: `market-data-quote-read` / focused fixture and transport boundary
证据: Go reference test, `crates/jftrade-api/tests/transport_contracts.rs`, and group product tests.
分类: harness
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: integration
后续: Do not add synthetic auth cases to the group fixture; include shared transport evidence in cutover review.

quirk: The worker handoff initially omitted this group ledger even though the fixture and reference existed.
范围: `market-data-quote-read` / handoff evidence
证据: integration branch review before this ledger was added.
分类: harness
判定: deviated
处置: 修复 fixture/harness
风险: medium
owner: integration branch
后续: Keep this ledger and require its route/evidence entries before accepting the group handoff.

quirk: The fixture contains repeated `path?query` keys with different provider outcomes (`broker-queue` and `ticks`); a single-value Rust fixture map let the later case overwrite the earlier success case.
范围: `market-data-quote-read` / Rust fixture replay
证据: initial Rust product test failure for `broker-queue-ready` (503 replayed instead of Go's 200), fixture cases `broker-queue-ready`/`broker-queue-provider-warming-retry`, and `ticks-ready`/`ticks-provider-failure`.
分类: harness
判定: deviated
处置: 修复 fixture/harness; the Rust fixture adapter now preserves ordered duplicate-key responses and the Go/Rust replay passes.
风险: medium
owner: integration branch
后续: Keep the ordered-response adapter and rerun it whenever duplicate request keys are added to the fixture.

## Ownership

The integration branch registers the ten routes only when the explicit test-cutover profile injects `MarketDataQuoteReadSnapshotPort`. They are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated wire/error/timeout/crash/restart rehearsal. That rehearsal explicitly disables Futu on loopback ports `1/2`; default shadow and production launchers do not register these routes. Go remains the production owner, and no Provider/OpenD connection, subscription mutation, SQLite write, notification, or user-visible event is introduced.
