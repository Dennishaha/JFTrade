# Portfolio Read Group Ledger

- Group: `portfolio-read`
- Tier: B: broker-backed portfolio projections depend on the Go broker runtime and provider lifecycle, so Rust is test-cutover-only.
- Owner: Go remains production owner. Rust accepts a consumer-owned `PortfolioSnapshotPort` only in `ProductConfig::test_cutover`; it never discovers accounts, connects OpenD, queries a broker, or writes portfolio state.
- Fixture: `tests/fixtures/rust-migration/stage9/portfolio-read.json`
- Differential: `TestStage9PortfolioReadFixtureMatchesCurrentGoOwner` plus parameterized Rust route tests.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/portfolio/{brokerId}/cash-balances` | Preserves Go `balances`, `checkedAt`, `connectivity`, and nullable `lastError` fields. | Snapshot failure is `503 PORTFOLIO_UNAVAILABLE`; route is absent without the explicit port. |
| GET | `/api/v1/portfolio/{brokerId}/positions` | Preserves Go `positions`, `checkedAt`, `connectivity`, and nullable `lastError` fields. | Snapshot failure is `503 PORTFOLIO_UNAVAILABLE`; route is absent without the explicit port. |

Known quirk: the Go no-provider fallback reports a degraded connection and empty arrays; the fixture freezes only the clock-dependent `checkedAt` value to `fixture-time`. This is reproduced without correction.

Both operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. Go remains the sole portfolio and broker-runtime owner; Rust only replays the explicit snapshot port.
