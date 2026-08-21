# Strategy Definitions Read Group Ledger

- Group: `strategy-definitions-read`
- Tier: C in the route inventory, with explicit test-cutover only because the projection depends on the Go strategy SQLite store and preview derivation.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `StrategyDefinitionSnapshotPort` only in `ProductConfig::test_cutover`; Rust never opens or mutates the strategy store.
- Fixture: `tests/fixtures/rust-migration/stage9/strategy-definitions.json`
- Differential: `TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner` plus the parameterized `strategy_definition_routes_match_group_fixture_in_cutover_only` test.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/strategy-definitions` | No request body; returns the ordered current-definition array, preserving timestamps, runtime, source format, script, interval, symbol, and version fields. | Store/snapshot failure maps to `500 STRATEGY_FAILED`; an unavailable test port is `404 NOT_FOUND` because the default catalog does not register the route. |
| GET | `/api/v1/strategy-definitions/{definitionId}` | Decodes one path segment and accepts `interval`, `symbol`, and `useExtendedHours` preview query values; returns the complete current version with derived warmup fields. | Invalid id/query is `400 BAD_REQUEST`; missing definition is `404 NOT_FOUND`; snapshot failure is `500 STRATEGY_FAILED`. |
| GET | `/api/v1/strategy-definitions/{definitionId}/versions` | Decodes the id and returns ordered immutable version summaries, including soft-deleted history where Go exposes it. | Invalid id is `400 BAD_REQUEST`; missing definition/history is `404 NOT_FOUND`; snapshot failure is `500 STRATEGY_FAILED`. |
| GET | `/api/v1/strategy-definitions/{definitionId}/versions/{version}` | Decodes both path segments and returns one immutable historical version projection. | Invalid segments are `400 BAD_REQUEST`; missing version is `404 NOT_FOUND`; snapshot failure is `500 STRATEGY_FAILED`. |

Known quirks: preview query values select Go's existing warmup projection without changing persisted definition fields; timestamps are normalized only in the fixture. This slice reproduces those values and does not repair legacy normalization behavior.

Route ownership for all four operations is `cutover-test-only`, `productionOwner=go`, `goRemovalStatus=retained`. The default shadow catalog does not register these routes.
