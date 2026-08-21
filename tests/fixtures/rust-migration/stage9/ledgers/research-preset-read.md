# Research Preset Read Group Ledger

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

Both operations are `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`.
