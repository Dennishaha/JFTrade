# Plugins Read Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `plugins-read`
- Tier: C in the route inventory. Qualification uses an authenticated Go-sidecar rehearsal; the Rust snapshot port remains explicit test-cutover-only because the catalog and persisted operation status are owned by the Go plugin lifecycle and catalog store.
- Owner: Go remains the production owner. Rust accepts consumer-owned `PluginSnapshotPort` and `PluginUninstallGuidanceSnapshotPort` instances only in `ProductConfig::test_cutover`; it never opens the plugin catalog store, scans plugin files, executes uninstall commands, loads plugin code, or starts a runtime/provider. The default shadow profile does not register these routes.
- Fixtures: `tests/fixtures/rust-migration/stage9/plugins-read.json` and `tests/fixtures/rust-migration/stage9/plugin-uninstall-guidance.json`.
- Differential: the two Go owner reference tests plus parameterized Rust catalog/operation/guidance replay.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/plugins` | No request body; returns the complete Go catalog projection with target directory, normalized descriptors, installation state, uninstall guidance, and compatibility metadata. Empty catalogs preserve `plugins: []`; JSON responses use `Content-Type: application/json; charset=utf-8`. | Catalog snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered when the explicit snapshot port is absent. |
| GET | `/api/v1/plugins/operations/{operationId}` | Decodes one operation ID path segment and returns the persisted plugin operation projection, preserving nullable completion/error fields. JSON success and error responses use `Content-Type: application/json; charset=utf-8`. | Blank or invalidly escaped IDs are `400 BAD_REQUEST`; unknown operation is `404 NOT_FOUND`; snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered without the explicit port. |
| GET | `/api/v1/plugins/{pluginId}/uninstall-guidance` | Returns the catalog-owned install path, current existence projection, and escaped POSIX/PowerShell removal commands without probing the filesystem or executing either command. | Blank, encoded-blank, or malformed percent escapes are `400 BAD_REQUEST`; unknown plugin and unmatched blank path are `404 NOT_FOUND`; snapshot failure is `503 PLUGIN_UNINSTALL_GUIDANCE_UNAVAILABLE`. |

Known quirks: the fixture normalizes host build metadata (`jftradeVersion`, Go version, OS, and architecture) to stable fixture values because those fields describe the executing Go host rather than catalog state. The Go wire shape, nullable fields, path handling, and error envelope remain unchanged; no behavior is corrected in this slice.

Route ownership for all three operations is `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/restart rehearsal. The default shadow catalog still does not register these snapshot-port routes. Plugin install/uninstall mutations remain separately Go-owned.

## Three-way review and quirks

### Q1: invalid percent escapes are rejected by the Go route

quirk: Go `BindURI` rejects `/api/v1/plugins/operations/%ZZ` with `400 BAD_REQUEST` and `operationId is required`; a normal `httptest.NewRequest` cannot construct that invalid URL, so the reference harness initially failed before reaching Gin.

范围: `plugins-read` / `GET /api/v1/plugins/operations/{operationId}`.

证据: the raw-`RequestURI` Go reference case, `plugins-read.json` `operation-invalid-escape`, and the Go sidecar wire/restart rehearsal.

分类: harness

判定: confirmed and resolved.

处置: the reference harness now constructs a request with an explicit raw path so the Go owner remains the baseline; no production Go behavior changed.

风险: low

owner: plugins-read worker

后续: retain the invalid-escape case in future differential runs.

### Q2: Rust percent decoding previously accepted invalid escapes

quirk: Rust's generic percent decoder treated `%ZZ` as ordinary text and would continue to operation lookup, while Go rejected the malformed escape before lookup.

范围: `plugins-read` / `GET /api/v1/plugins/operations/{operationId}`.

证据: Go reference and fixture `operation-invalid-escape`, Rust product route replay, and `product_api_plugins.rs` path validation.

分类: rust-implementation

判定: deviated, then resolved in the Rust route adapter.

处置: Rust now rejects malformed percent escapes with the Go-compatible `400 BAD_REQUEST` envelope; Go remains unchanged.

风险: low

owner: Rust worker

后续: preserve the malformed escape check before any operation-port call.

### Q3: uninstall-guidance invalid percent escapes previously reached lookup in Rust

quirk: Go `BindURI` rejects `/api/v1/plugins/%ZZ/uninstall-guidance` as `400 BAD_REQUEST`, while Rust's generic percent decoder previously preserved `%ZZ` and returned lookup-driven `404 NOT_FOUND`.

范围: `plugins-read` / `GET /api/v1/plugins/{pluginId}/uninstall-guidance`.

证据: Go handler reference case `invalid-escape`, the generated uninstall-guidance fixture, and Rust product replay.

分类: rust-implementation

判定: deviated, then resolved.

处置: both plugin path decoders now share strict percent-escape validation before snapshot-port lookup; Go behavior is unchanged.

风险: low

owner: Rust integration branch

后续: preserve malformed-escape precedence in the group differential.

The Go references, pinned fixtures, Rust replay, and authenticated Go-sidecar wire/error/timeout/crash/restart rehearsal now agree. The sidecar only forwards the three GET operations with the private Bearer, internal proxy protocol, and verified desktop access surface; failed Rust requests do not replay Go, and rollback is a restart-time Go-only composition decision. Both Rust snapshot ports remain read-only and consumer-owned; no plugin filesystem, runtime, provider, or production catalog store is opened by this slice.

The route group is now `cutover-qualified` after the integration branch applied the shared route-ownership and differential evidence update. Go remains the production owner; the explicit snapshot port and default-profile route isolation are unchanged.

## Verification record

- Go owner fixture: `go test ./scripts/rust-migration -run '^TestStage9PluginsReadFixtureMatchesCurrentGoOwner$' -count=1`.
- Go uninstall-guidance fixture: `go test ./scripts/rust-migration -run '^TestStage9PluginUninstallGuidanceFixtureMatchesCurrentGoOwner$' -count=1`.
- Go authenticated sidecar wire/restart rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestPluginsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust replay, headers, empty catalog, invalid escape, port failure, and route isolation: `cargo test -p jftrade-engine --lib product::tests::plugin_tests -- --nocapture` and `cargo test -p jftrade-engine plugin_uninstall_guidance --lib --locked`.
- Rust production compilation: `cargo check -p jftrade-engine --lib --locked`.
- Rust formatting: `rustfmt --edition 2024 --check crates/jftrade-engine/src/product_api_plugins.rs crates/jftrade-engine/src/product_plugins_tests.rs`.
- Route coverage remains integration-owned and currently derives `23 shadow / 232 cutover-test-only / 23 cutover-qualified / 0 remaining / 0 Rust production owner`.
