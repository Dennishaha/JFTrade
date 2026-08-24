# Settings read

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

## Verification

- Rust authenticated/read-only replay:
  `cargo test -p jftrade-engine --lib 'product::tests::settings_read_tests::'`
- Go authenticated sidecar rehearsal:
  `go test ./internal/app/apiserver/servercoretest -run '^TestSettingsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`
- Unified Stage 9 differential:
  `pnpm run test:rust:stage9:product-differential`
- Current route coverage: 12 shadow / 133 cutover-test-only / 133
  cutover-qualified / 0 remaining / 0 Rust production owner.

The route entries are `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. No default Rust production owner or settings write
path was enabled.
