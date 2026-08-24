# Market-Data Options Read Group Ledger

- Group: `market-data-options-read`
- Tier: B; these projections depend on active Provider/OpenD capability routing and derivative query semantics.
- Owner: Go remains the production owner of Provider/OpenD lifecycle, broker capability selection, option catalog/analytics/event queries, subscriptions and all market-data writes. Rust accepts a complete `MarketDataOptionsReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring and never activates a Provider or OpenD.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-options-read.json`
- Differential: `TestStage9MarketDataOptionsReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_market_data_options_read_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/market-data/options/chains/{instrumentId}` | Provider feature query for an option chain; query and provider envelope are preserved byte-for-byte. | Missing broker capability remains `409 BROKER_CAPABILITY_UNAVAILABLE`; unavailable Rust snapshot fails closed as `503 MARKET_DATA_OPTIONS_UNAVAILABLE`. |
| GET | `/api/v1/market-data/options/expirations/{instrumentId}` | Same option-chain capability with `operation=expirations`; response preserves provider metadata and entries. | Same capability-unavailable and snapshot-unavailable branches. |
| GET | `/api/v1/market-data/options/screens` | Provider option screen projection with the existing query parameters and envelope. | Provider capability failure remains the Go error envelope; snapshot port failure is `503 MARKET_DATA_OPTIONS_UNAVAILABLE`. |
| GET | `/api/v1/market-data/options/analysis/{instrumentId}` | Provider option analytics projection, including instrument context and provider metadata. | Provider capability failure remains the Go error envelope; snapshot port failure is `503 MARKET_DATA_OPTIONS_UNAVAILABLE`. |
| GET | `/api/v1/market-data/options/events` | Provider option event projection with existing market/broker selection. | Provider capability failure remains the Go error envelope; snapshot port failure is `503 MARKET_DATA_OPTIONS_UNAVAILABLE`. |

No Go observable quirk was found after comparing the Go baseline, fixture and Rust replay. All five operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated wire/error/timeout/crash/restart rehearsal with Futu explicitly disabled on loopback ports `1/2`. Rust does not open market-data storage, start a helper, connect OpenD, create subscriptions, or register option mutation routes.
