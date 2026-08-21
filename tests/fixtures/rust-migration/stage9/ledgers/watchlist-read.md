# Watchlist Read Group Ledger

- Group: `watchlist-read`
- Tier: C for local projections, with explicit test-cutover only because the Go watchlist service owns SQLite, source refresh, pagination, and cache/lifecycle behavior.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `WatchlistReadSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the watchlist SQLite database or activates a broker source reader.
- Fixture: `tests/fixtures/rust-migration/stage9/watchlist-read.json`
- Differential: `TestStage9WatchlistReadFixtureMatchesCurrentGoOwner` plus the parameterized `watchlist_read_routes_match_group_fixture_in_cutover_only` test.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/watchlist/groups` | Returns `{groups}` with group IDs, names, protection/default flags, revisions, item counts, and RFC3339 timestamps. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`; route is absent without the explicit port. |
| GET | `/api/v1/watchlist/items` | Returns the paginated item page with instrument metadata, group IDs and group references. | Go validation/pagination remains inside the snapshot producer; unavailable snapshot is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/sources` | Returns `{sources}` with broker, display name, status, optional error, and update time. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/sources/{sourceId}/groups` | Returns `{groups}` for the selected remote source, preserving ambiguity and observed-at fields. | Unknown source remains Go's not-found projection; snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/bindings` | Returns `{bindings}`, optionally filtered by the Go-owned query projection. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/import-runs` | Returns the paginated import-run page. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`; cursor and limit semantics remain Go-owned. |

Known quirks: fixture timestamps are fixed through the Go service clock so generated output is stable; no observable Go behavior is corrected. Membership PUT and all group/import/quote mutations remain `remaining`, and the already-fenced memberships GET remains a separate operation.

Route ownership for these six operations is `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. The default shadow catalog does not register them.
