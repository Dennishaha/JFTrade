# Strategy Definitions Read Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `strategy-definitions-read`
- Tier: C in the route inventory; qualification uses an authenticated Go-sidecar rehearsal while the projection remains explicit test-cutover-only because it depends on the Go strategy SQLite store and preview derivation.
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

quirk: Go `BindURI` rejects malformed path escapes with `400 BAD_REQUEST`, while Go's `URL.Query`/Gin query binder ignores a malformed query pair. 范围: `strategy-definitions-read` / GET `/api/v1/strategy-definitions/{definitionId}` and version variants. 证据: `internal/api/httpserver/bindings_test.go:56`, the malformed path/query cases in `stage9_strategy_definitions_reference_test.go` and `strategy-definitions.json`, and the Rust replay in `product_strategy_definitions_tests.rs`. 分类: go-behavior. 判定: confirmed and resolved in the strategy-owned adapter. 处置: reject malformed path escapes before snapshot lookup; drop malformed query pairs while preserving normal invalid boolean errors. 风险: medium. owner: strategy-definitions-read worker. 后续: retain the cases in the group differential and review any post-hard-cut Go behavior change separately.

quirk: The Go strategy fixture reference initially panicked when it used `httptest.NewRequestWithContext` for `/api/v1/strategy-definitions/%ZZ`; Go's HTTP test constructor rejects malformed request targets before the router can exercise `BindURI`. 范围: `strategy-definitions-read` / reference harness. 证据: failed `TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner/detail-malformed-id-escape`; `internal/api/httpserver/bindings_test.go:56-72` already uses a manually constructed request. 分类: harness. 判定: confirmed. 处置: use a manual `http.Request` with raw `RequestURI` for malformed path/query fixture cases; no production behavior changed. 风险: low. owner: worker. 后续: retain as harness evidence for future malformed-path cases.

quirk: The strategy rollback rehearsal initially used `defer jftradeCheckTestError(t, goAfterRestart.Close())`, which evaluated `Close` immediately and left the restarted Go owner backed by a closed database. It also captured the settings baseline before the first owner startup and compared a new response's dynamic envelope timestamp byte-for-byte. 三方复核: the failing rehearsal, the Go server's startup-time settings initialization and response timestamp generation, and the analogous alerts/plugins restart harnesses. 分类: harness. 判定: confirmed and resolved in the group-owned rehearsal. 处置: register `Close` through `t.Cleanup`, capture settings after the initial startup, and normalize only the top-level envelope timestamp for rollback comparison; route data, error payloads and headers remain exact. 风险: low. owner: strategy-definitions-read worker. 后续: keep restart rollback in the group rehearsal.

quirk: The four read-operation evidence entries previously named `pnpm run test:rust:stage9:strategy-definitions-differential`, but that command is not defined in `package.json` and the shared runner is the executable differential. 三方复核: Go read fixture/reference, Rust `strategy_definition_routes_match_group_fixture_in_cutover_only` replay, and `package.json`/`check-stage9-product-differential.mjs` were compared. 分类: harness. 判定: confirmed. 处置: corrected the ledger evidence to `pnpm run test:rust:stage9:product-differential`; no Go observable behavior or route ownership changed.

Route ownership for all four operations is `cutover-qualified`, `productionOwner=go`, `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. The default shadow catalog does not register these routes, and no strategy store or runtime write owner moved to Rust.

## Verification record

- Go observable fixture: `go test ./scripts/rust-migration -run '^TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner$' -count=1`.
- Rust replay/auth/fail-closed tests: `cargo test -p jftrade-engine 'product::tests::strategy_definition_tests::' --lib --locked`.
- Go authenticated sidecar rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestStrategyDefinitionsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Shared differential: `pnpm run test:rust:stage9:product-differential`; Go remains the sole production owner and the strategy SQLite/runtime lifecycle remains outside the cutover slice.
