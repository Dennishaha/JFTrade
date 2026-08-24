# `market-data-derivatives-read` ledger

Tier B; group-level ledger for the two read-only derivative catalog routes.

| method | path | request projection | response/error projection |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/warrants` | `brokerId`, `accountId`, `tradingEnvironment`, `market`, `pageSize`, `cursor`, `operation` and remaining query fields are passed through the Go `FeatureQuery`; market is upper-cased and page size is normalized by the service. | Go `broker.FeatureResult` envelope from `DerivativeCatalogReader`; capability resolution failures remain `409 BROKER_CAPABILITY_UNAVAILABLE`, invalid query remains `400 BAD_REQUEST`, provider failures remain the Go status/code/message. |
| GET | `/api/v1/market-data/futures` | Same query mapping; product class is fixed to `future` and market segment to `derivatives`. | Same `FeatureResult` projection and error envelope; empty entry slices remain arrays. |

## Ownership and boundary

Go `internal/api/productfeatures` and `internal/productfeatures.Service` remain the production owner of broker resolution, Provider/OpenD lifecycle, capability normalization, caching and all market-data writes. Rust receives a complete JSON snapshot only through `MarketDataDerivativeReadSnapshotPort` in explicit test-cutover wiring; the default authenticated read-only shadow does not register these routes. Both operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated wire/error/timeout/crash/restart rehearsal with Futu explicitly disabled on loopback ports `1/2`.

## Evidence

- Go route binding: `internal/api/productfeatures/routes.go` (`/warrants`, `/futures` use `handleQuery` and `DerivativeCatalogReader`).
- Go query/error mapping: `routeQuery`, `Service.Query`, `writeQueryError`; provider and broker failures are preserved bug-for-bug.
- Fixture and differential: `stage9_market_data_derivatives_read_reference_test.go`, `market-data-derivatives-read.json`, and `pnpm run test:rust:stage9:product-differential`.
