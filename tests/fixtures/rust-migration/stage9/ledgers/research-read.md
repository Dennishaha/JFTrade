# Research Read Group Ledger

- Group: `research-read`
- Tier: B: provider-backed company, market, calendar and ranking projections depend on the Go provider runtime, so Rust is test-cutover-only.
- Owner: Go remains production owner. Rust accepts a consumer-owned `ResearchReadSnapshotPort` only in `ProductConfig::test_cutover`; it never starts a provider, opens OpenD, or writes research state.
- Fixture: `tests/fixtures/rust-migration/stage9/research-read.json`
- Differential: `TestStage9ResearchReadFixtureMatchesCurrentGoOwner` plus parameterized Rust route tests.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/research/instruments/{instrumentId}` | Preserves provider `FeatureResult` entries, `asOf`, and provider selection metadata. | Provider failure is `503 RESEARCH_UNAVAILABLE` in the Rust snapshot adapter; Go capability/status behavior is frozen in the fixture. |
| GET | `/api/v1/research/financials/{instrumentId}` | Preserves financial research `FeatureResult` wire and query parameters. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/valuation/{instrumentId}` | Preserves valuation research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/analyst/{instrumentId}` | Preserves analyst research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/ownership/{instrumentId}` | Preserves ownership research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/corporate-actions/{instrumentId}` | Preserves corporate-action research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/short-interest/{instrumentId}` | Preserves short-interest research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/technical-indicators/{instrumentId}` | Preserves technical-indicator research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/screens` | Preserves screen query `FeatureResult` wire and query parameters. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/calendars` | Preserves calendar query `FeatureResult` wire and date filters. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/macro` | Preserves macro research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/rankings` | Preserves ranking `FeatureResult` wire and pagination parameters. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/institutions` | Preserves institution research `FeatureResult` wire. | Same provider snapshot failure mapping. |
| GET | `/api/v1/research/industries` | Preserves industry research `FeatureResult` wire. | Same provider snapshot failure mapping. |

Known quirk: provider selection timestamps (`asOf` and `resolvedAt`) are normalized to `fixture-time` for deterministic differential; no observable response field is otherwise changed.

All fourteen operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated wire/error/timeout/crash/restart rehearsal. The rehearsal uses missing-broker projections with Futu explicitly disabled on loopback ports `1/2`; Go remains the sole provider runtime and research-state owner.
