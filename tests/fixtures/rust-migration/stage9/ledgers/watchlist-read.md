# Watchlist Read Group Ledger

- Group: `watchlist-read`
- Tier: C for local projections, with explicit test-cutover only because the Go watchlist service owns SQLite, source refresh, pagination, and cache/lifecycle behavior.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `WatchlistReadSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the watchlist SQLite database or activates a broker source reader.
- Fixture: `tests/fixtures/rust-migration/stage9/watchlist-read.json`
- Differential: `TestStage9WatchlistReadFixtureMatchesCurrentGoOwner` exercises the real Gin handlers, and the parameterized `watchlist_read_routes_match_group_fixture_in_cutover_only` test replays the resulting path-and-query corpus through the explicit Rust snapshot port.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/watchlist/groups` | Returns `{groups}` with group IDs, names, protection/default flags, revisions, item counts, and RFC3339 timestamps. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`; route is absent without the explicit port. |
| GET | `/api/v1/watchlist/items` | Returns the paginated item page with instrument metadata, group IDs and group references. | Go validation/pagination remains inside the snapshot producer; unavailable snapshot is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/sources` | Returns `{sources}` with broker, display name, status, optional error, and update time. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/sources/{sourceId}/groups` | Returns `{groups}` for the selected remote source, preserving ambiguity and observed-at fields. | Unknown source remains Go's not-found projection; snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/bindings` | Returns `{bindings}`, optionally filtered by the Go-owned query projection. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |
| GET | `/api/v1/watchlist/import-runs` | Returns the paginated import-run page. | Snapshot failure is `503 WATCHLIST_UNAVAILABLE`; cursor and limit semantics remain Go-owned. |
| GET | `/api/v1/watchlist/instruments/{market}/{symbol}/memberships` | Returns the normalized instrument membership revision and ordered group references from the Go-owned watchlist store. | Invalid market aliases preserve `400 WATCHLIST_INVALID`; snapshot failure is `503 WATCHLIST_UNAVAILABLE`. |

The primary fixture contains 11 cases covering the original six operations, seeded and empty projections, source and binding filters, invalid pagination, and unknown source `404`. The separate membership fixture adds five existing, unknown, alias, and invalid-instrument cases. Timestamps are fixed through the Go service clock so generated output is stable. Port-unavailable and absent-port behavior are covered for every operation in the Rust tests.

Membership PUT and all group/import/quote mutations remain separate operations. Route ownership for these seven operations is now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. The membership read keeps its separate seeded SQLite fixture and Rust snapshot-port replay; the default shadow catalog does not register any of these routes.

## Three-way review and quirks

### Q1: the Go sources GET refreshes persisted source state

quirk: Go `GET /api/v1/watchlist/sources` invokes every registered source reader and upserts the observed status into `watchlist_sources`, so this nominal GET is not byte-level SQLite read-only.

范围: `watchlist-read` / `GET /api/v1/watchlist/sources`.

证据: `internal/watchlist/service.go::ListSources`, the Go handler fixture, and the authenticated rollback rehearsal.

分类: go-behavior

判定: intended.

处置: preserve the current producer-owned refresh behavior behind the consumer-owned snapshot port; do not let Rust and Go refresh or write the source cache concurrently.

风险: medium

owner: Go watchlist service

后续: before the production owner switch, provide one fenced Rust source-refresh owner or retain the Go producer until hard cut; never dual-write `watchlist_sources`.

The Go handler fixture, Rust replay, and authenticated Go-sidecar rehearsal agree. The rehearsal covers exact status/body/headers, query forwarding, `400` and `404`, every operation's Rust error and timeout, process crash, proof that failed Rust requests never replay Go, and restart-time Go-only rollback. It uses the disabled local Futu boundary and a missing-source case; no live OpenD or external provider is contacted. Rust still never opens the watchlist SQLite database or activates a source reader.

## Verification record

- Go handler fixture: `go test ./scripts/rust-migration -run '^TestStage9WatchlistReadFixtureMatchesCurrentGoOwner$' -count=1`.
- Go authenticated sidecar wire/restart rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestWatchlistReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust fixture/error/port-isolation replay: `cargo test -p jftrade-engine 'product::tests::watchlist_read_tests::' --lib --locked`.
- Unified differential: `pnpm run test:rust:stage9:product-differential`.
- Route coverage: `23 shadow / 232 cutover-test-only / 23 cutover-qualified / 0 remaining / 0 Rust production owner`.
