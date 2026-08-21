# Market-Data Provider Read Group Ledger

- Group: `market-data-provider-read`
- Tier: B; the projection depends on the live market-data Provider descriptor, health, runtime, and subscription lifecycle, so it is not a simple static GET.
- Owner: Go remains the production owner of Provider/OpenD lifecycle, health probes, runtime state, subscription demand/cache, and all market-data writes. Rust accepts a complete `MarketDataProviderReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring and never activates a Provider or OpenD.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-provider-read.json`
- Differential: `TestStage9MarketDataProviderReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_market_data_provider_read_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/provider` | Returns the Go `ProviderStatusResponse` projection: `checkedAt`, descriptor/capabilities/constraints, health, runtime counters/timestamps, and subscription quota/entries. The outer API envelope and nullable fields are preserved byte-for-byte. | Provider status failure remains `502 MARKET_DATA_PROVIDER_FAILED` with the provider error message; a missing Rust snapshot port fails closed as `503 MARKET_DATA_PROVIDER_UNAVAILABLE`. |

The fixture covers ready health (`200`), degraded health with `lastError` and idle stream (`200`), and provider failure (`502`). `checkedAt` is normalized to `fixture-time` only by the reference generator for deterministic replay; runtime zero timestamps and nullable quota fields remain unchanged. The `fixture` query values in the corpus select fake provider behavior in the Go reference harness and are not interpreted as a new public request parameter.

The operation is `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. Rust does not connect to OpenD, start a helper, acquire subscription leases, mutate provider settings, or register any market-data mutation route.
