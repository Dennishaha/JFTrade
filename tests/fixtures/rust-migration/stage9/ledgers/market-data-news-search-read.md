# Market-Data News Search Read Group Ledger

- Group: `market-data-news-search-read`
- Tier: B. The route is a provider-backed product-feature aggregation read and
  inherits embedded-provider descriptor selection, capability, fallback,
  warming, busy, and provider-generation behavior.
- Operations: 1 (`GET /api/v1/market-data/news`).
- Production owner: Go. Rust is limited to a consumer-owned raw snapshot port
  in explicit `cutover-test-only` wiring; it must not activate a Provider,
  OpenD, sidecar, cache, subscription, SQLite, or mutation path.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-news-search-read.json`
- Go reference: `scripts/rust-migration/stage9_market_data_news_search_read_reference_test.go`
- Current route status: `cutover-test-only` in `route-ownership.json`, with
  `productionOwner=go` and `goRemovalStatus=retained`. The default shadow
  catalog does not register this route; registration requires the explicit
  test-cutover capability and snapshot port.

## Contract Matrix

| Case | Go observable contract | Fixture evidence |
| --- | --- | --- |
| Embedded success | `brokerId`/`providerId` matching a non-Futu active descriptor and a usable `instrumentId` intercepts before broker routing | `embedded-ready-page-size-precedes-limit`, `embedded-default-limit-and-instrument-market` |
| Query precedence | `market` overrides the instrument prefix; `pageSize > 0` wins over `limit`; otherwise `limit`, then default `10`; provider limit clamps to `50` | `embedded-ready-page-size-precedes-limit`, `embedded-explicit-market-overrides-prefix-and-clamps`, `embedded-limit-used-without-page-size` |
| Normalization | broker/account/trading environment/market/instrument are trimmed or uppercased by the Go service; `operation` query overrides route default in the parameter bag | `embedded-ready-page-size-precedes-limit`, `operation-query-overrides-route-default` |
| Empty/null | allocated and nil provider entry slices both project to JSON `entries: []`; nullable entry fields are omitted by the product projection | `embedded-empty-entries`, `embedded-null-entries-project-to-empty-array` |
| Capability | sentinel capability error maps to `409 BROKER_CAPABILITY_UNAVAILABLE` with the exact facade message | `embedded-capability-unsupported` |
| Provider failures | generic failure and provider-generation change map to `502 BROKER_FEATURE_FAILED` with the original message | `embedded-provider-fallback-failure`, `embedded-provider-changed` |
| Lifecycle retry | warming maps to `503 MARKET_DATA_PROVIDER_WARMING` + `Retry-After: 1`; busy maps to `503 MARKET_DATA_PROVIDER_BUSY` + `Retry-After: 2` | `embedded-provider-warming`, `embedded-provider-busy` |
| Fallback | explicit `futu` bypasses the embedded provider and resolves the registered broker; descriptor failure also falls through to broker resolution | `explicit-futu-falls-back-to-broker`, `descriptor-error-falls-back-to-broker` |
| Query boundary | missing `instrumentId` and malformed `pageSize` are not rejected by this route; they fall through or use the Go default path | `missing-instrument-falls-to-broker`, `malformed-page-size-is-accepted-as-default` |

## Quirk Review Log

### Q1: Capability sentinel versus generic error changes the HTTP status

quirk: The product-feature route maps an error to `409` only when the
`ErrCapabilityUnsupported` sentinel survives the embedded facade; the same
human-readable text without the sentinel maps to generic `502`.
范围: `market-data-news-search-read` / `GET /api/v1/market-data/news`
证据: `internal/productfeatures/provider_facade.go`,
`internal/api/productfeatures/routes.go`, and the initial reference harness
replay before the sentinel was wrapped.
分类: go-behavior
判定: intended after three-way review
处置: preserve Go's `errors.Is`-based mapping in the fixture and raw snapshot
replay; do not infer status from message text. No Go behavior was changed.
风险: high
owner: Go/Rust worker
后续: retain the sentinel regression while the route remains cutover-test-only.

### Q2: Nil provider entries are projected as an empty JSON array

quirk: A nil `NewsResponse.Entries` slice and an allocated empty slice both
become `entries: []` because `projectProviderNews` allocates a zero-length
projection before transport serialization.
范围: `market-data-news-search-read` / `GET /api/v1/market-data/news`
证据: `internal/productfeatures/provider_projection.go` and the two fixture
cases `embedded-empty-entries` and `embedded-null-entries-project-to-empty-array`.
分类: go-behavior
判定: intended after three-way review
处置: preserve the captured JSON value exactly; no Rust DTO is allowed to
reintroduce a null/empty distinction absent from the Go wire.
风险: medium
owner: Go/Rust worker
后续: retain the empty/null projection cases while the route remains
cutover-test-only.

### Q3: Missing instrument and malformed page size are accepted at this route

quirk: `/market-data/news` does not validate that `instrumentId` is present;
the request can fall through to broker routing. `pageSize=abc` is parsed as
zero by the handler and the embedded facade then uses the `limit` parameter or
default `10`.
范围: `market-data-news-search-read` / query validation
证据: `internal/api/productfeatures/routes.go`,
`internal/productfeatures/provider_facade.go`, and fixture cases
`missing-instrument-falls-to-broker` and `malformed-page-size-is-accepted-as-default`.
分类: go-behavior
判定: intended after three-way review
处置: reproduce the observable behavior in the fixture; do not add Rust-side
validation or repair this possible Go bug in the migration slice.
风险: high
owner: Go/Rust worker
后续: reproduce until any post-hard-cut repair is explicitly approved.

### Q4: Fixture time normalization and provider-call evidence are not public wire

quirk: `asOf` and `provider.resolvedAt` are wall-clock values while provider
call evidence is an internal fixture field. The reference normalizes only the
dynamic timestamps and JSON-number types, leaving the public data shape intact.
范围: group fixture/reference harness
证据: `stage9_market_data_news_search_read_reference_test.go` and fixture
`data`/`providerCall` fields.
分类: fixture
判定: intended after three-way review
处置: Rust tests must compare normalized public data and use `providerCall`
only as evidence; it must never be emitted by the Rust HTTP response.
风险: medium
owner: Go/Rust worker
后续: keep normalization internal to the fixture/reference harness and never
emit provider-call evidence from Rust.

## Three-Way Conclusions

### Q5: Included port files share the product module namespace

quirk: The first Rust replay compile failed because the included port file
redeclared the product root imports `serde_json::Value` and
`thiserror::Error`.
范围: `market-data-news-search-read` / Rust include boundary
证据: `cargo test -p jftrade-engine product::tests::market_data_news_search_read_tests::`
reported E0252 before the port import cleanup.
分类: rust-implementation
判定: deviated
处置: Reuse the existing product-root imports in the included port file; keep
the focused compile regression and do not add a second namespace/import layer.
风险: low
owner: integration
后续: Retain the focused test while the route remains cutover-test-only.

三方复核结论: The Go reference and frozen fixture are unchanged, and the Rust
port now compiles in the same include namespace after removing duplicate
imports. The focused group replay and complete Stage 9 differential are green.

### Three-Way Review Results

- Q1: Go's sentinel-aware mapping, the 16-case fixture, and the Rust raw
  snapshot error mapper agree on `409 BROKER_CAPABILITY_UNAVAILABLE` versus
  `502 BROKER_FEATURE_FAILED`. 判定: intended; preserve the distinction.
- Q2: Go's projection, the empty/null fixture cases, and the Rust value replay
  all produce `entries: []`. 判定: intended; no Rust null coercion is needed.
- Q3: Go accepts the missing `instrumentId` and malformed `pageSize` cases as
  captured, the fixture freezes the fall-through/default behavior, and Rust
  replays the captured response without adding validation. 判定: intended;
  reproduce until any post-hard-cut repair is explicitly approved.
- Q4: Go reference normalization, fixture timestamp/provider-call evidence,
  and Rust response comparison agree that only public data is emitted and
  dynamic timestamps are fixture-normalized. 判定: intended.
- Q5: The Go reference and fixture are unchanged; the Rust focused replay now
  compiles and passes after removing duplicate include-scope imports. 判定:
  deviated; retain the integration regression.

The complete group replay is green for all 16 cases plus unavailable and
unregistered-route isolation. The group remains `cutover-test-only`; no
cutover-qualified evidence or production owner change is implied.

## Integration Completion

The integration branch applied the following minimal additions, following the
existing `MarketDataNewsActionsRead` pattern:

1. Included `product_market_data_news_search_read_port.rs` from `product.rs`;
   add `market_data_news_search_read_snapshot_port: Option<Arc<dyn
   MarketDataNewsSearchReadSnapshotPort>>` to `ProductConfig`, initialize it to
   `None`, copy it into the running product state, and include the API module.
2. Added the optional port to `ProductOptionalPorts` in
   `product_api_types.rs`, pass it through `ProductApi::new` in `product_api.rs`,
   and store it on `ProductApi`.
3. Included `product_market_data_news_search_read_api.rs` from `product.rs`.
   Its dispatcher must map `Unavailable` to `503
   MARKET_DATA_NEWS_SEARCH_READ_UNAVAILABLE`; map `Failed` status/code/message
   and optional retry metadata without parsing or rewriting the query.
4. Added a `MarketDataNewsSearchRead` variant to `ProductCapability`, included it
   in `ProductCapabilities::test_cutover` and the read-only branch of
   `requires_writable_settings`, derive the port boolean in
   `product_route_ports`, and extend `product_routes` with
   `product_market_data_news_search_read_routes`.
5. Included `product_market_data_news_search_read_routes.rs` from
   `product_route_assembly.rs`; register exactly `GET
   /api/v1/market-data/news` only when both the capability and test port are
   present.
6. Included the new API matcher before the existing news-actions matcher in
   `product_wire.rs`; the matcher must be exact path equality so it does not
   capture `/api/v1/market-data/news/{market}/{symbol}`.
7. Included `product_market_data_news_search_read_tests.rs` through the shared
   product test module, updated route counts and registered the operation in
   `route-ownership.json`.

## Completion Evidence

- Go reference generation: passed with `go test ./scripts/rust-migration
  -run 'Stage9(ADKRead|MarketDataNewsSearchRead)' -count=1`.
- Rust group replay: passed with the focused 3-case group tests and the full
  Stage 9 product differential.
- `check:quick`, `check:rust`, and route coverage: passed after the shared
  integration wiring; the closeout gate correctly remains open.
- Production owner, default registration, Provider/OpenD, cache,
  subscription, SQLite, mutation, and public contract: unchanged.
