# Runtime dependencies read

- Group: `runtime-dependencies-read`.
- Tier: B because the projection reads normalized settings and process
  environment, resolves an executable candidate and launches `node --version`
  under a two-second timeout.
- Operation: `GET /api/v1/system/runtime-dependencies`.
- Owner: Go remains the production owner. Rust performs one read-only process
  probe and neither starts a Pine worker nor changes settings or runtime state.
- Frozen reference:
  `tests/fixtures/rust-migration/stage9/product-slice-corpus.json` covers valid,
  outdated and unrecognized Node version output. Go runtime tests additionally
  cover configured/missing/PATH/macOS-common candidates and command errors.

Qualification fixed a composition mismatch: the Rust product route previously
ignored `pineWorker.nodeBinaryPath`, and a retained Pine runtime could report a
non-Go `runtime:managed` source. The route now reads the normalized setting on
each request and applies the Go precedence `settings >
JFTRADE_PINEWORKER_RUNTIME > JFTRADE_NODE_BINARY > PATH/common`. Its process
future kills the child on timeout.

The Rust authenticated product test proves the settings candidate, complete
path/source projection, Node minimum, authentication boundary and unchanged
settings bytes. The Go sidecar rehearsal preserves exact status/body/headers
for an installed configured Node and verifies error, proxy timeout, crash and
restart-time Go rollback.

## Verification

- Shared Go/Rust version corpus: `pnpm run test:rust:stage9:product-differential`.
- Rust runtime unit batch: `cargo test -p jftrade-engine --lib runtime_dependencies::tests`.
- Rust authenticated route: `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests::runtime_dependencies_use_the_normalized_settings_node_candidate -- --exact`.
- Go authenticated rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestRuntimeDependenciesReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Current route coverage: 1 shadow / 118 cutover-test-only / 159
  cutover-qualified / 0 remaining / 0 Rust production owner.

The route is `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. Rust production ownership, Pine worker lifecycle,
settings writes and Go removal do not change.
