# Plugins Read Group Ledger

- Group: `plugins-read`
- Tier: C in the route inventory, with explicit test-cutover only because the catalog and persisted operation status are owned by the Go plugin lifecycle and catalog store.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `PluginSnapshotPort` only in `ProductConfig::test_cutover`; it never opens the plugin catalog store, scans plugin files, loads plugin code, or starts a runtime/provider.
- Fixture: `tests/fixtures/rust-migration/stage9/plugins-read.json`
- Differential: `TestStage9PluginsReadFixtureMatchesCurrentGoOwner` plus the parameterized `plugins_read_routes_match_group_fixture_in_cutover_only` test.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/plugins` | No request body; returns the complete Go catalog projection with target directory, normalized descriptors, installation state, uninstall guidance, and compatibility metadata. | Catalog snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered when the explicit snapshot port is absent. |
| GET | `/api/v1/plugins/operations/{operationId}` | Decodes one operation ID path segment and returns the persisted plugin operation projection, preserving nullable completion/error fields. | Blank encoded ID is `400 BAD_REQUEST`; unknown operation is `404 NOT_FOUND`; snapshot failure is `503 PLUGINS_UNAVAILABLE`; the route is not registered without the explicit port. |

Known quirks: the fixture normalizes host build metadata (`jftradeVersion`, Go version, OS, and architecture) to stable fixture values because those fields describe the executing Go host rather than catalog state. The Go wire shape, nullable fields, path handling, and error envelope remain unchanged; no behavior is corrected in this slice.

Route ownership for both operations is `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. The default shadow catalog does not register these routes. Plugin install/uninstall mutations and the existing uninstall-guidance route remain separately owned and are not expanded by this group.
