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

Rust transport now owns the same bounded request-observability projection as
Go: newest-first error and slow-request histories, 20-event default bound,
750ms default threshold, importance filtering and OpenD health/correlation
fields. A shared Go/Rust corpus covers populated histories and OpenD success
and failure; the authenticated status test covers the empty runtime shape.
The Tauri release launcher also rejects development or zero versions before
asset preparation, injects one validated version/commit/build-time tuple into
the Rust compile, and applies the same version as the final bundle config.

Rust now also projects `observability.live.connected`, `limit` and `atLimit`
from the same typed connection metrics used by its authenticated WebSocket
transport. The counter includes accepted upgrades while their session permit
is alive, rejects acquisition at the effective Rust transport limit, and
releases through RAII. `activeInstruments` remains empty because Rust does not
yet own the live subscription registry.

The effective limit now comes from the Go-compatible
`interfaces.liveWebSocketConnectionLimit` settings projection. A shared
Go/Rust corpus covers the exact `WebSocket` acronym spelling, missing/zero/
negative defaults, a positive override and malformed-type failure. Rust opens
the settings file read-only and preserves its bytes.

Qualification remains blocked on owner-backed dynamic projections:

- Rust runtime resources now enumerate every resource actually composed by
  the current Rust product: the settings file, nine read-only data-management
  SQLite inventories, the read-only real-trade control file, configured Pine
  workers and the configured market-data helper. The SQLite entries reuse the
  same descriptor/path resolution as the authenticated data-management route,
  including environment overrides and the ADK artifact path derived from the
  session database. They do not initialize or write any database. Go-only
  Assistant directories/secrets, calendar storage, plugin storage and logical
  strategy sub-resources remain absent until their ownership is composed in
  Rust, so the public route is not yet qualified.
- Live observability still lacks a Rust-owned active-subscription registry, and
  market-data observability does not yet expose complete owner-backed counters
  and lifecycle generations. The Rust OpenD recorder entry is ready, but the
  Rust provider must call it when OpenD ownership migrates.
- Broker and strategy descriptors have static parity, but their live runtime
  state must move through typed Rust-owned ports before production cutover.

Verification: `cargo test -p jftrade-api websocket::tests -- --nocapture`; `cargo test -p jftrade-store-settings-file --test interface_settings_contracts`; `go test ./internal/store/settingsfile -run '^TestLiveWebSocketInterfaceSettingsMatchRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-api observability::tests::request_observability_matches_stage9_go_corpus -- --exact`; `go test ./pkg/observability -run '^TestRequestObservabilityMatchesRustMigrationCorpus$' -count=1`; `node --test scripts/lib/tauri-runtime.test.mjs scripts/lib/desktop-release-metadata.test.mjs`; `cargo test -p jftrade-engine --lib product_runtime::tests::product_runtime_without_optional_workers_starts_and_stops_cleanly -- --exact`; `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests -- --nocapture`; `go test ./internal/app/apiserver/servercoretest -run '^TestSystemStatusReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `pnpm run check:rust`.

Current route coverage remains 1 shadow / 133 cutover-test-only / 144
cutover-qualified / 0 remaining / 0 Rust production owner. The ledger retains
`productionOwner=go` and `goRemovalStatus=retained`.
