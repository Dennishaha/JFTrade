# Watchlists Read Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `watchlists-read`
- Tier: B: remote watchlist listing depends on the Go broker registry, provider/OpenD capability discovery, and broker lifecycle, so Rust is test-cutover-only.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `RemoteWatchlistSnapshotPort` only in `ProductConfig::test_cutover`; it never discovers brokers, connects OpenD, activates a provider, or writes watchlist state.
- Fixture: `tests/fixtures/rust-migration/stage9/watchlists-read.json`
- Differential: `TestStage9WatchlistsReadFixtureMatchesCurrentGoOwner` plus the parameterized Rust tests in `product_watchlists_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/watchlists/remote` | Preserves the Go broker-feature query envelope for remote watchlist listing, including the timestamp and error/data projection. Query text is passed unchanged to the consumer-owned snapshot port. | Without a configured snapshot port or when the snapshot producer fails, Rust fails closed with `503 WATCHLIST_UNAVAILABLE`; the Go no-broker fixture preserves its existing `409 BROKER_CAPABILITY_UNAVAILABLE` response. |

The fixture normalizes only the dynamic response timestamp to `fixture-time`; no observable Go behavior is corrected. The route is now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. The default shadow catalog does not register it. Remote watchlist mutation (`POST /api/v1/watchlists/remote`) remains `remaining` and is outside this read-only slice.
