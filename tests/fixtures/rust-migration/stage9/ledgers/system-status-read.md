# System status read audit

- Group: `system-status-read`.
- Tier: B because this route aggregates build identity, persistence,
  observability, runtime resources, calendar, broker, strategy and real-trade
  control-plane state.
- Operation: `GET /api/v1/system/status`.
- Owner: Go remains the production owner and the route remains `shadow`.

The 2026-08-24 audit removed a non-contract `migrationOwner` field and aligned
the Rust stable projection with Go for build default and platform naming,
settings persistence, request-observability shape, the static Futu descriptor,
idle strategy summary, calendar snapshot wiring and the public message. The
authenticated Rust product test also proves settings bytes remain unchanged.
The Go sidecar rehearsal covers status/body/headers, error, proxy timeout,
crash and restart-time Go rollback.

Qualification remains blocked on owner-backed dynamic projections:

- Rust runtime resources currently list only resources actually composed by
  the Rust product runtime; Go lists the complete Go-owned settings, SQLite,
  Assistant, calendar, plugin and real-trade inventory.
- Live, market-data and request observability do not yet share the complete Go
  counters, bounded event history and lifecycle generations.
- Broker and strategy descriptors have static parity, but their live runtime
  state must move through typed Rust-owned ports before production cutover.
- Platform build metadata must be injected into Rust release builds by the
  release pipeline before cross-platform qualification.

Verification: `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests::system_status_matches_go_stable_fields_without_claiming_migration_ownership -- --exact`; `go test ./internal/app/apiserver/servercoretest -run '^TestSystemStatusReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `pnpm run check:rust`.

Current route coverage remains 1 shadow / 133 cutover-test-only / 144
cutover-qualified / 0 remaining / 0 Rust production owner. The ledger retains
`productionOwner=go` and `goRemovalStatus=retained`.
