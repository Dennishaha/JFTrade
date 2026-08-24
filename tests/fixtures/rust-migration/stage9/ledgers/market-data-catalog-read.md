# Market-Data Catalog Read Group Ledger

- Group: `market-data-catalog-read`
- Tier: B; both projections depend on the active market-data Provider lifecycle and broker-neutral catalog/search behavior.
- Owner: Go remains the production owner of Provider/OpenD lifecycle, descriptor and market catalog queries, instrument resolver/search cache, and all market-data writes. Rust accepts a complete `MarketDataCatalogReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring and never activates a Provider or OpenD.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-catalog-read.json`
- Differential: `TestStage9MarketDataCatalogReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_market_data_catalog_read_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/markets` | Returns `{defaultMarket,markets}` after the Go service obtains the provider market profiles and descriptor default market. Map fields and empty arrays are preserved. | Provider market query or descriptor failure remains `500 MARKET_DATA_FAILED`; an unavailable Rust snapshot port fails closed as `503 MARKET_DATA_CATALOG_UNAVAILABLE`. |
| GET | `/api/v1/market-data/instruments` | Requires trimmed `query`; optional `market` is normalized by the Go resolver and `limit` defaults to 20 with an inclusive range of 1..100. Returns the complete `InstrumentResolution` projection, including `entries` and `failures`. | Missing/invalid query or limit preserves `400 MARKET_INSTRUMENT_INVALID`; provider search failure remains `502 MARKET_INSTRUMENT_SEARCH_FAILED`; unavailable Rust snapshot is `503 MARKET_DATA_CATALOG_UNAVAILABLE`. |

The fixture uses query values named `fixture` only to select isolated fake-provider failures in the Go reference harness; they are not new public parameters. No known Go observable quirk was found after comparing the Go baseline, fixture and Rust replay; no unresolved quirk is carried by this group.

Both operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. Its Futu integration is explicitly disabled and pinned to loopback ports `1/2`; Rust does not open market-data storage, start a helper, connect OpenD, create subscriptions, or register derivative catalog, snapshot, streaming, or mutation routes.
