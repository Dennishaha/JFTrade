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

Rust now also projects `observability.live.connected`, `limit`, `atLimit` and
`activeInstruments` from the same typed connection registry used by its
authenticated WebSocket transport. Each accepted upgrade owns a client-scoped
RAII permit; Go-compatible subscribe messages replace that client's normalized
active instruments, the status snapshot returns a sorted/deduplicated union,
and disconnect removes the client state. The registry is transport-local and
does not reconcile Provider/OpenD demand or write Go subscription state.

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
- Market-data observability does not yet expose complete owner-backed counters
  and lifecycle generations. Rust now fails closed to the Go-compatible
  `unavailable` projection without a typed `MarketDataRuntimeStatusPort`;
  Python helper readiness no longer masquerades as quote connectivity or
  leaks generic process errors into `quoteLastError`. A shared Go/Rust corpus
  fixes idle/connecting/connected/degraded/closed precedence, timestamps,
  counters and trimmed nullable errors. The Rust OpenD recorder entry is ready,
  and `jftrade-marketdata` now owns a concurrent collector recorder with exact
  normalized demand generations, old-generation rejection, independent
  5/10/20/30-second quote/stream retry counters, recovery resets and
  idempotent close precedence. It directly implements the product port and the
  authenticated handler test exercises its live state. The Rust provider and
  OpenD adapters must still drive the recorder from production composition.
  The product composition seam now accepts an optional
  `Arc<MarketDataRuntimeRecorder>` exported by that router and injects the same
  instance into `/system/status`; a product-runtime test proves
  updates after startup are visible without a duplicate recorder. The default
  desktop composition still leaves this port absent until the real provider/OpenD
  lifecycle is assembled, so the route remains a Go-owned shadow.
  `ProductRuntimeConfig` now also accepts the `ProviderRouter` itself, retains
  that router in `ProductRuntimeHandle`, and derives the status port from its
  recorder; a fixture-provider test proves router demand updates reach the
  product status projection through the shared instance. This uses no real
  OpenD socket and does not change the default desktop owner.
  `jftrade-integration-futu` now also provides a deadline-bound
  `OpenDTcpTransport` with complete frame-length/hash validation and a local
  mock-listener round-trip test. It remains an adapter primitive only: no
  login probe, poll/push worker, ProviderRouter wiring or default product
  composition uses a real OpenD socket yet.
  A read-only `OpenDTcpProbe` now adds the Go-compatible `InitConnect` and
  `GetGlobalState` protobuf handshake, retType/default handling, minimum
  version gate, login/market/program-state mapping and UTC timestamp
  projection. Its success, rejection, unsupported-version and socket paths
  are covered by local framed mock tests; it still does not drive the router
  or default product owner.
  `market_data_health_from_probe` now provides the explicit composition
  mapping into broker-neutral `HealthStatus`; a fixture test drives a router
  from ready to failed after a disconnected probe. This remains opt-in
  composition evidence only and does not activate a default provider or
  OpenD connection.
  `OpenDSubscriptionLifecycle` now combines normalized demand with physical
  subscription actions, generation-fenced callbacks, bounded 5/10/20/30-second
  retries, poll/quote/stream recorder recovery and idempotent close. Local
  tests cover stale callbacks and old-generation rejection. The explicit
  `OpenDSubscriptionExecutor` now performs the Go-compatible `InitConnect` and
  Qot_Sub subscribe/unsubscribe exchange over one deadline-bound TCP session,
  with local mock coverage for market/security/subtype mapping and retType
  rejection. Qot_Update push decoding, poll worker execution and default
  product composition remain open.
- Broker descriptors still have only static parity. Strategy runtime status no
  longer comes from a fabricated idle JSON object: a public typed
  `StrategyRuntimeStatusPort` now supplies the exact Go summary shape, and a
  shared Go/Rust corpus fixes missing-runtime defaults, available zero-value
  behavior, active instance fields, nullable symbol slices, optional fields
  and owner-provided order. `jftrade-strategy` now owns a concurrent active
  instance registry that normalizes identity, symbols and errors, derives
  sorted counts/status, and directly implements the product port; the
  authenticated handler test proves live registry updates reach both status
  projections with UTC timestamps. The Pine lifecycle must still populate and
  remove this registry from the production composition before cutover.
  The product composition seam now accepts an optional lifecycle-owned
  `Arc<StrategyRuntimeRegistry>` and injects that same registry into both
  strategy status projections; a product-runtime test proves updates after
  startup are visible. The default desktop composition still leaves the
  registry absent until Pine lifecycle reporting is implemented, so this does
  not claim a Rust strategy production owner.

Verification: `cargo test -p jftrade-api websocket -- --nocapture`; `cargo test -p jftrade-store-settings-file --test interface_settings_contracts`; `go test ./internal/store/settingsfile -run '^TestLiveWebSocketInterfaceSettingsMatchRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-api observability::tests::request_observability_matches_stage9_go_corpus -- --exact`; `go test ./pkg/observability -run '^TestRequestObservabilityMatchesRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-engine --lib product::product_market_data_runtime_status::tests::market_data_runtime_projection_matches_go_status_corpus -- --exact`; `go test ./internal/app/apiserver/status -run '^TestMarketDataRuntimeStatusMatchesRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-engine --lib product::product_strategy_runtime_status::tests::strategy_runtime_projection_matches_go_status_corpus -- --exact`; `go test ./internal/app/apiserver/status -run '^TestStrategyRuntimeStatusMatchesRustMigrationCorpus$' -count=1`; `node --test scripts/lib/tauri-runtime.test.mjs scripts/lib/desktop-release-metadata.test.mjs`; `cargo test -p jftrade-engine --lib product_runtime::tests::product_runtime_without_optional_workers_starts_and_stops_cleanly -- --exact`; `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests -- --nocapture`; `go test ./internal/app/apiserver/servercoretest -run '^TestSystemStatusReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `pnpm run check:rust`.

Current route coverage remains 1 shadow / 133 cutover-test-only / 144
cutover-qualified / 0 remaining / 0 Rust production owner. The ledger retains
`productionOwner=go` and `goRemovalStatus=retained`.
