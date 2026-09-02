# Research Preset Read Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `research-preset-read`
- Tier: C read-only projections backed by the Go research preset service and SQLite store; Rust is test-cutover-only.
- Owner: Go remains the production owner of preset SQLite, `NormalizeDefinitionV2`, revision checks, and all preset writes. Rust accepts a complete `ResearchPresetReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring and never opens the research database or registers mutation routes.
- Fixture: `tests/fixtures/rust-migration/stage9/research-preset-read.json`
- Differential: `TestStage9ResearchPresetReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_research_preset_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/research/screens/presets` | Returns `{presets}` with the complete Go `ScreenPreset` projection, including normalized definition, schema version, revision, and RFC3339 timestamps. | Go store failure remains the owner error; an unavailable Rust snapshot port fails closed as `503 RESEARCH_PRESET_UNAVAILABLE`. |
| GET | `/api/v1/research/screens/presets/{presetId}` | Returns one complete `ScreenPreset` projection for the trimmed preset ID. | Missing preset preserves `404 RESEARCH_PRESET_NOT_FOUND`; snapshot failure is `503 RESEARCH_PRESET_UNAVAILABLE`. |

The complete projection is transported through the consumer-owned port so Rust does not approximate `NormalizeDefinitionV2` or duplicate SQLite query semantics. POST/PATCH/DELETE remain unregistered and Go-owned.

Both operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar rehearsal below. The default shadow catalog still does not register these snapshot-port routes; Rust remains test-cutover-only at the composition boundary.

## Three-way review and quirks

quirk: The first restart rehearsal captured the settings baseline before the initial Go sidecar startup. Go's startup initialization then added the default `backtestMarketDataProvider` field, so the otherwise read-only research-preset rehearsal appeared to mutate settings when the restarted Go owner was compared with the pre-start baseline.

范围: `research-preset-read` authenticated restart rollback rehearsal; no research preset response or route behavior changed.
证据: failed `TestResearchPresetReadRehearsalPreservesWireAndRequiresRestartForGoRollback`; Go `NewSidecarHandlerWithOptions` startup settings initialization; Rust sidecar replay and the before/after settings comparison in `rehearsal_research_preset_read_routes_test.go`.
三方复核: Go startup baseline behavior, the Rust authenticated wire/error/timeout/crash replay, and the rehearsal's baseline capture order were compared.
分类: harness
判定: confirmed and resolved
处置: capture the settings baseline after the initial Go owner has started and before proxy requests; preserve the restart comparison and do not alter Go initialization or Rust behavior.
风险: low
owner: 集成分支
后续: retain post-start baseline capture for future sidecar rollback rehearsals.

## Qualification status

The Go fixture/reference, Rust group replay, authenticated sidecar wire comparison, explicit Rust error/timeout/crash fail-closed checks, restart-time Go rollback, and settings read-only fencing all pass for both GET operations. The Rust snapshot port remains consumer-owned and opt-in; Go keeps the research preset SQLite/service owner and all POST/PATCH/DELETE routes.

Verification: `go test ./scripts/rust-migration -run '^TestStage9ResearchPresetReadFixtureMatchesCurrentGoOwner$' -count=1`; `go test ./internal/app/apiserver/servercoretest -run '^TestResearchPresetReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `cargo test -p jftrade-engine 'product::tests::research_preset_read_tests::' --lib --locked`; `pnpm run test:rust:stage9:product-differential`. Current route coverage is `23 shadow / 232 cutover-test-only / 23 cutover-qualified / 0 remaining / 0 Rust production owner`.
