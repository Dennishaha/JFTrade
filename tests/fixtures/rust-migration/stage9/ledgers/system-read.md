# System Read Group Ledger

- Group: `system-read`
- Tier: B: OpenD health and broker order-update worker projections depend on broker/provider lifecycle and runtime worker state, so Rust is test-cutover-only.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `SystemReadSnapshotPort` only in `ProductConfig::test_cutover`; it never connects OpenD, activates a broker, starts an order-update worker, or writes trading state.
- Fixture: `tests/fixtures/rust-migration/stage9/system-read.json`
- Differential: `TestStage9SystemReadFixtureMatchesCurrentGoOwner` plus parameterized tests in `product_system_read_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/system/futu-opend` | Preserves the complete Go OpenD health projection: status, runtime connectivity/configuration, diagnosis, socket diagnostics, local installation, latest-version and recommendations fields. Dynamic host platform is normalized only in the fixture. | Snapshot failure is `503 SYSTEM_READ_UNAVAILABLE`; the route is absent unless the explicit snapshot port is injected. Go's disabled-integration `offline` projection is frozen without correction. |
| GET | `/api/v1/system/worker/broker-order-updates` | Preserves the Go order-update worker snapshot, including subscriptions, invalidations, broker summaries and runtime fields; the default no-worker projection is `{}`. | Snapshot failure is `503 SYSTEM_READ_UNAVAILABLE`; the route is absent without the explicit snapshot port. |

Both operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. The default read-only shadow catalog does not register either route. OpenD reset (`POST /api/v1/system/futu-opend/manual-retry`) and all real-trade mutations remain outside this read-only slice; Go remains their sole production owner.

## Exchange-calendar reads

`GET /api/v1/system/exchange-calendars/sources` and `GET /api/v1/system/exchange-calendars/status` are also `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`. Their Go projections are frozen by `calendar-sources.json` and `calendar-status.json`, Rust uses the real calendar manager only in explicit test-cutover wiring, and the authenticated rehearsal covers exact wire/error/timeout/crash/restart behavior with auto-refresh disabled. Refresh/probe mutations and snapshot persistence remain exclusively Go-owned.
