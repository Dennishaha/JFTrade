# Appearance read

- Group: `appearance-read`
- Tier: C: the route is a side-effect-free settings-file projection.
- Operation: `GET /api/v1/settings/ui`.
- Production owner: Go remains the owner. Rust is limited to the authenticated
  loopback read-only shadow and opens the settings file read-only; no SQLite,
  Provider/OpenD, notification, or write state is owned by this slice.
- Fixture/reference: `tests/fixtures/rust-migration/stage9/settings-ui-read.json`
  and `scripts/rust-migration/stage9_settings_ui_read_reference_test.go`.
- Rust replay: `crates/jftrade-engine/src/product_appearance_read_tests.rs`.

## Initial quirk register

### Q1: a null appearance field is treated as absent by Go

quirk: Go `encoding/json` unmarshals a JSON `null` appearance pointer as nil,
so `GET /api/v1/settings/ui` returns the default colors. The current Rust
settings-file validation rejects the same document before the product shadow
starts.

范围: `settings.json` appearance field and `GET /api/v1/settings/ui`.

证据: the Go sidecar reference, pinned `null-appearance` fixture case, Rust
settings-file decoder, and the authenticated Rust replay.

分类: rust-behavior

判定: confirmed and resolved in the Rust compatibility layer.

处置: Rust `SettingsFileStore` now treats JSON `null` as an absent optional
field during validation and decoding, matching the Go owner without changing
the Go implementation or any public wire contract.

风险: low after fix

owner: Rust worker / 集成分支

后续: preserve the null case in every later settings-file differential and
re-review before any owner switch.

## Three-way conclusion

Final three-way review agrees that this was an existing Rust settings-file
compatibility difference, not a Go bug or fixture defect: the Go sidecar
reference and authenticated proxy harness return the default projection, the
pinned fixture freezes the same response, and the Rust replay now returns the
same response after treating `null` as absent. The Rust settings-file contract
test also proves the read-only document is not rewritten.

The route is `cutover-qualified` with `productionOwner=go` and
`goRemovalStatus=retained`. The authenticated loopback rehearsal selects this
route alongside the two previously qualified catalog routes; it never enables
a write path or changes the global production owner.

## Verification record

- Go owner fixture: `go test ./scripts/rust-migration -run '^TestStage9SettingsUIReadFixtureMatchesCurrentGoOwner$' -count=1`.
- Go authenticated wire/restart rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestAppearanceReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust settings-file null compatibility: `cargo test -p jftrade-store-settings-file --test settings_file_contracts null_appearance_is_treated_as_an_absent_optional_setting -- --exact`.
- Rust authenticated shadow replay and token fence: `cargo test -p jftrade-engine product::tests::appearance_read_tests::appearance_read_route_matches_go_fixture_for_all_seed_documents -- --exact` and `cargo test -p jftrade-engine product::tests::appearance_read_tests::appearance_read_route_requires_the_authenticated_shadow_token -- --exact` (the unified differential invokes both exact tests).
- Route coverage: 23 shadow / 238 cutover-test-only / 17 cutover-qualified / 0 remaining / 0 Rust production owner.
