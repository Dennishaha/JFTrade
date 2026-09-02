# Settings read

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `settings-read`
- Tier: C: side-effect-free settings projections backed by the existing Go
  settings file. The data-management database overview is intentionally kept
  in its Tier B slice because it depends on SQLite inspection and storage
  metadata.
- Owner: Go remains the production owner. Rust is exercised only through the
  authenticated loopback rehearsal and explicit test-cutover product profile;
  it does not acquire a settings writer lease, start Provider/OpenD, or write
  the settings file.
- Operations: the eleven ordinary settings GET routes for ADK, MCP, broker,
  onboarding, execution, security, notification, Pine worker, provider and
  exchange-calendar projections.

The Go `runReadRouteRehearsal` harness compares the complete status, body and
selected response headers for every operation, then verifies injected error,
timeout and crash fail-closed behavior and restart-time Go rollback. The Rust
product replay starts the real settings route assembly, checks Go-compatible
default projections for security, execution, provider, calendars and brokers,
requires the authenticated shadow token, and proves the settings file bytes
are unchanged after all reads.

The supporting settings-file boundary also reads the persisted
`interfaces.liveWebSocketConnectionLimit` for Rust transport composition
without adding a public route or write owner. The shared Go/Rust corpus covers
missing and invalid defaults, a positive override, malformed input, and the
exact `liveWebSocketConnectionLimit` persistence key.

## Verification

- Rust authenticated/read-only replay:
  `cargo test -p jftrade-engine --lib 'product::tests::settings_read_tests::'`
- Go authenticated sidecar rehearsal:
  `go test ./internal/app/apiserver/servercoretest -run '^TestSettingsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`
- Shared interface-settings corpus:
  `cargo test -p jftrade-store-settings-file --test interface_settings_contracts` and
  `go test ./internal/store/settingsfile -run '^TestLiveWebSocketInterfaceSettingsMatchRustMigrationCorpus$' -count=1`
- Unified Stage 9 differential:
  `pnpm run test:rust:stage9:product-differential`
- Current route coverage: 1 shadow / 118 cutover-test-only / 159
  cutover-qualified / 0 remaining / 0 Rust production owner.

The route entries are `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. No default Rust production owner or settings write
path was enabled.
