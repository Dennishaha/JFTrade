# Plugins Read Group Ledger

- Group: `plugins-read`
- Tier: C in the route inventory, with explicit test-cutover only because the catalog and persisted operation status are owned by the Go plugin lifecycle and catalog store.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `PluginSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the plugin catalog store, scans plugin files, loads plugin code, or starts a runtime/provider.
- Fixture: `tests/fixtures/rust-migration/stage9/plugins-read.json`
- Differential: `TestStage9PluginsReadFixtureMatchesCurrentGoOwner` plus the parameterized `plugins_read_routes_match_group_fixture_in_cutover_only` test.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/plugins` | No request body; returns the complete Go catalog projection with target directory, normalized descriptors, installation state, uninstall guidance, and compatibility metadata. Empty catalogs preserve `plugins: []`; JSON responses use `Content-Type: application/json; charset=utf-8`. | Catalog snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered when the explicit snapshot port is absent. |
| GET | `/api/v1/plugins/operations/{operationId}` | Decodes one operation ID path segment and returns the persisted plugin operation projection, preserving nullable completion/error fields. JSON success and error responses use `Content-Type: application/json; charset=utf-8`. | Blank or invalidly escaped IDs are `400 BAD_REQUEST`; unknown operation is `404 NOT_FOUND`; snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered without the explicit port. |

Known quirks: the fixture normalizes host build metadata (`jftradeVersion`, Go version, OS, and architecture) to stable fixture values because those fields describe the executing Go host rather than catalog state. The Go wire shape, nullable fields, path handling, and error envelope remain unchanged; no behavior is corrected in this slice.

Route ownership for both operations is `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. The default shadow catalog does not register these routes. Plugin install/uninstall mutations and the existing uninstall-guidance route remain separately owned and are not expanded by this group.

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

The Go reference, pinned fixture, Rust replay, and authenticated Go-sidecar wire/error/timeout/crash/restart rehearsal now agree. The sidecar only forwards the two GET operations with the private Bearer, internal proxy protocol, and verified desktop access surface; failed Rust requests do not replay Go, and rollback is a restart-time Go-only composition decision. The Rust snapshot port remains read-only and consumer-owned; no plugin filesystem, runtime, provider, or production catalog store is opened by this slice.

The route group remains `cutover-test-only` until the integration branch applies the shared route-ownership and product-profile evidence update. This worker did not change those shared files.

## Verification record

- Go owner fixture: `go test ./scripts/rust-migration -run '^TestStage9PluginsReadFixtureMatchesCurrentGoOwner$' -count=1`.
- Go authenticated sidecar wire/restart rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestPluginsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust replay, headers, empty catalog, invalid escape, port failure, and route isolation: `cargo test -p jftrade-engine --lib product::tests::plugin_tests -- --nocapture` (3 passed).
- Rust production compilation: `cargo check -p jftrade-engine --lib --locked`.
- Rust formatting: `rustfmt --edition 2024 --check crates/jftrade-engine/src/product_api_plugins.rs crates/jftrade-engine/src/product_plugins_tests.rs`.
- Route coverage remains integration-owned and currently derives `23 shadow / 252 cutover-test-only / 3 cutover-qualified / 0 remaining / 0 Rust production owner`.
