# System status read audit

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

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
  rejection. `OpenDFrameReader` and `decode_quote_push` now read unsolicited
  frames from the same session and decode BasicQot/KL/OrderBook payloads with
  Go-compatible retType/S2C drop and proto2 required-field semantics;
  lifecycle ingestion records stream recovery only for the active generation
  and rejects stale frames. `OpenDManagedSession` now provides the missing
  single-reader boundary: exact protocol/serial matches wake concurrent pending
  RPCs, every other frame becomes a generation-tagged unsolicited event, peer
  EOF fans out to waiters, timed-out requests cannot receive late responses,
  and local close shuts down and joins the reader exactly once. Local mock TCP
  tests cover response/push interleaving, same-serial wrong protocols,
  out-of-order concurrent responses, EOF, late responses and idempotent close.
  `OpenDInitializedSession` now performs `InitConnect` once over that managed
  session and is the shared authenticated boundary for `OpenDTcpProbe` and
  `OpenDSubscriptionExecutor`. It preserves the observed Go connection-role
  quirk: the short-lived health probe sends `RecvNotify=false`, while the
  long-lived subscription data session sends `RecvNotify=true`. A
  single-connection data-session mock interleaves a BasicQot push with
  `GetGlobalState`, then executes Qot_Sub without a second handshake or
  competing socket reader. `OpenDSessionEventPump::poll_once` now provides a
  bounded, test-only single-step consumer: timeout is idle, active pushes enter
  the generation-fenced lifecycle, malformed known pushes are silently dropped
  like the Go stream handler, active peer close requests reconnect once, and stale/local/lifecycle-first
  shutdown events have no retry side effect. It owns no thread, dial, retry,
  subscription replay or router mutation. `jftrade-marketdata` now also exposes
  a synchronous `SnapshotPollExecutor` seam with the Go collector's one-second
  cadence, 1.5-second freshness gate, recorder-owned 5/10/20/30-second retry
  window, normalized/deduplicated demand, requested-tick filtering and stale
  generation discard. Query failure preserves existing cache entries, explicit
  non-positive policy values retain the Go defaults, and the query remains
  injected with no timer, task or transport ownership.
  `OpenDBasicQuoteExecutor` now supplies the Qot_GetBasicQot 3004 wire query on
  the same managed session. It canonicalizes/deduplicates SecurityList, requires
  an active lifecycle-owned BASIC subscription, fences session/lifecycle
  generations, reuses required-field decoding, preserves rejection/incomplete
  errors, and accepts successful absent/empty `S2C` as Go's empty result without
  implicit subscribe. Its `query_ticks` seam maps valid regular
  BasicQot rows into the current Fixed8 collector `Tick`, preserving normalized
  market identity, last-row-wins behavior, zero-price drop, caller observation
  time and provider generation without mutating cache or recorder. The collector
  volume model now uses arbitrary-precision `DecimalText` while retaining the
  existing numeric JSON representation, so finite fractional `hpVolume` no
  longer truncates; Stage 4 differential remains byte-compatible. Full
  extended-session/time projection still requires a frozen Go/Rust corpus. The
  query now uses the Go collector's 900ms per-call deadline through the managed
  pending-RPC timeout; a timed-out serial is removed before a late response can
  reach the old waiter.
  `OpenDBasicQuoteExecutor::query_with_retry` now provides a bounded,
  test-only replay seam: at most two attempts, a fresh 900ms deadline per
  attempt, no backoff, and retries only for managed-session Closed/IO/timeout
  errors. The reconnect callback must return a new authenticated session and
  replay the already-approved subscriptions; generation fences run before and
  after the callback, and no implicit Qot_Sub is issued by the query. A Go
  `withRetryingClient` recovery-policy test and a two-socket Rust mock cover
  the successful replay and prove the dead session is not reused. Delayed
  fallback, reconnect orchestration and default product composition remain
  open; the callback is not wired into the ProviderRouter or product runtime.
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

## Current slice quirk review

quirk: A successful OpenD 3004 response with absent `S2C` or an empty quote
list was initially treated as a query failure, while Go returns an empty slice
without entering the collector retry path.
范围: `system-status-read` / Qot_GetBasicQot response mapping
证据: Go `pkg/futu/opend/quotes.go`; Rust
`basic_quote_query_requires_subscription_and_maps_success_rejection_and_empty`.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust 使其匹配 Go；successful absent/empty payloads now return an
empty result, while present-but-incomplete proto2 rows remain errors.
风险: medium
owner: Rust integration branch
后续: retain absent/empty/incomplete cases through production composition.

quirk: `OpenDSubscriptionLifecycle::execute_action` originally fenced only the
lifecycle callback generation and could send Qot_Sub through an executor whose
managed session belonged to an older generation.
范围: `system-status-read` / subscription replay and reconnect fencing
证据: Rust `lifecycle_rejects_stale_executor_before_qot_sub_io` local TCP
regression.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust；require lifecycle, callback, and managed-session generation to
match before encoding or writing any Qot_Sub request.
风险: high
owner: Rust integration branch
后续: retain the no-I/O regression when the test coordinator becomes a real
composition-owned coordinator.

quirk: A stale snapshot-poll caller could report `Fresh` when the cache already
contained a fresh sample from the newer active generation, obscuring the stale
caller boundary.
范围: `system-status-read` / snapshot-poll generation fencing
证据: Rust
`stale_caller_is_rejected_even_when_the_active_generation_cache_is_fresh`.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust；closed and active-generation guards now run before freshness,
while active-generation freshness still precedes cadence/retry like Go.
风险: medium
owner: Rust market-data / 集成分支
后续: preserve ordering when the synchronous seam gains a lifecycle-owned task.

quirk: Explicit zero or negative Rust snapshot-poll policy values initially
removed cadence/freshness fencing instead of retaining the Go collector defaults.
范围: `system-status-read` / `SnapshotPollExecutor::new`
证据: Go `CollectorOptions`/`durationOr`; Rust regression
`non_positive_policy_values_retain_collector_defaults`.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust 使其匹配 Go；non-positive values now retain 1s/1.5s defaults.
风险: medium
owner: Rust integration branch
后续: retain the regression through default product composition.

quirk: Go `shopspring/decimal.NewFromFloat` panics for a NaN or infinite
BasicQot price, while the Rust mapper rejects the external row without panicking.
范围: `system-status-read` / OpenD 3004 BasicQot-to-collector mapping
证据: Go `pkg/futu/raw_decimal.go`, `pkg/futu/quote_snapshot_nonfinite_corpus_test.go`,
fixture `tests/fixtures/rust-migration/stage9/basic-quote-nonfinite.json`,
shopspring decimal v1.4.0 source, and Rust
`non_finite_price_corpus_matches_go_failure_boundary_and_rust_rejection`.
分类: go-behavior
判定: unresolved
处置: Rust 暂时 fail closed；建立 Go/Rust malformed-float corpus 后决定是否把
Go crash behavior 列为硬切后修复，未完成前阻断完整 mapper qualification.
风险: high
owner: 集成分支
后续: hard cut 前完成三方复核；Go corpus currently records the historical
panic while Rust intentionally rejects all three values, and no external OpenD
data may trigger a Rust panic.

quirk: The first Rust mapper could not preserve Go's finite fractional
`hpVolume` because the collector model exposed `Tick.volume` as `i64`.
范围: `system-status-read` / BasicQot high-precision volume
证据: Go `quoteSnapshotFromBasicQotAt`; Rust `DecimalText` numeric serializer;
`preserves_fractional_high_precision_volume`; Stage 4 differential.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust 使其匹配 Go；`Tick.volume` now stores arbitrary-precision
`DecimalText` and serializes as the existing JSON number, with no silent truncation.
风险: high
owner: Rust market-data / 集成分支
后续: retain fractional/large-number codec tests and Stage 4 differential through
production composition.

quirk: A failed or replay-pending Qot_Sub record was briefly exposed as an
active BasicQot subscription, and an unsubscribe failure was not deferred.
范围: `system-status-read` / OpenD subscription lifecycle
证据: Rust `failed_or_replayed_subscriptions_are_not_active_until_success`,
`failed_unsubscribe_is_deferred_until_its_retry_window`, and the reconnect
replay test.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust；active status now requires a successful subscribe in the
current generation, failed unsubscribe retains active state and its retry
window, and replay records stay pending until replay success.
风险: medium
owner: Rust integration branch
后续: preserve these fences when the lifecycle is connected to the production
coordinator.

quirk: A successful BasicQot push with an explicit empty `s2c.basic_quotes`
list was briefly projected as a successful empty push.
范围: `system-status-read` / OpenD 3005 push decoder
证据: Rust `rejected_empty_and_unknown_pushes_are_dropped_like_go`.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust；empty BasicQot lists now drop without clearing quote/stream
failure state.
风险: low
owner: Rust integration branch
后续: retain the empty-list case in the protocol corpus.

quirk: A direct fresh-cache lookup could expose a tick from a prior provider
generation after demand/session reconfiguration, even though snapshot polling
already fenced its freshness check.
范围: `system-status-read` / `TickCache` direct reads and snapshot fallback
证据: Rust `cache_rejects_stale_generation_and_classifies_freshness` and the
generation-aware lookup/require-fresh regression; `SnapshotPollExecutor` now
uses `lookup_for_generation`.
分类: rust-implementation
判定: deviated then fixed
处置: 修复 Rust；additive generation-aware lookup treats a mismatched sample
as missing and preserves the existing generation-agnostic API for compatibility.
风险: medium
owner: Rust market-data / 集成分支
后续: use the generation-aware methods at every provider/session-owned read
boundary before production composition; keep the legacy methods only for
callers whose cache is independently cleared on provider switch.

Test-only composition evidence: `basic_quote_query_feeds_snapshot_poll_cache_with_generation_fencing`
now drives a local framed OpenD BasicQot response through the authenticated
managed session, `OpenDBasicQuoteExecutor::query_ticks`, and the broker-neutral
`SnapshotPollExecutor`. The test proves a successful tick is committed to the
generation-fenced cache, a fresh cache suppresses a second query, and a
generation change rejects the stale poll without touching OpenD. The mock
server has no external network or production lifecycle; this does not wire a
timer, retry worker, reconnect coordinator, ProviderRouter, default product
composition, or production owner.

Test-only runtime projection evidence: `system_status_reflects_fixture_opend_poll_and_reconnect_lifecycle`
injects a fixture-only session coordinator into the product status port. It
reconciles a generation, marks the stream connected, executes a fenced snapshot
poll into `TickCache`, observes the connected/refresh projection through the
authenticated `/api/v1/system/status` handler, then simulates peer EOF and a
reconnect. The degraded projection exposes the stream failure, and the next
generation clears the failure after another fenced poll. This is a product
composition rehearsal only: the coordinator has no socket, timer, retry task,
ProviderRouter activation, default desktop registration, or production owner;
the default product composition still leaves the explicit OpenD seam unused.

Composition seam evidence: `OpenDSessionCoordinator` is now a public,
explicitly injectable integration boundary rather than a `cfg(test)`-only
module. It exposes topology-authorized reconcile, bounded push polling,
generation-fenced BasicQot snapshot polling into a caller-owned `TickCache`,
and idempotent close. `ProductRuntimeConfig::with_opend_session_coordinator`
can retain that authenticated session, derive the same recorder for
`/api/v1/system/status`, expose it through `ProductRuntimeHandle`, and close it
without allowing a ProviderRouter to be composed at the same time. The default
desktop profile still leaves the field absent: no timer/thread, ProviderRouter
activation, real OpenD dial, or production owner is introduced.

quirk: Promoting the coordinator API without adding a default timer preserves
the single-owner boundary; callers must explicitly drive `poll_once` and
`poll_snapshot`, while the runtime owns shutdown after injection.
范围: `system-status-read` / OpenD coordinator composition
证据: Rust `OpenDSessionCoordinator::public_coordinator_polls_basic_quotes_into_a_generation_fenced_cache`,
`ProductRuntimeConfig::with_opend_session_coordinator`, and
`ProductRuntimeError::ConflictingMarketDataOwners`.
分类: rust-implementation
判定: intended
处置: 保留显式 composition seam；在 production cutover 前补 timer/task、dynamic demand source、ProviderRouter activation、reconnect backoff and release recovery evidence.
风险: high
owner: Rust integration / engine composition
后续: hard cut 前完成 runtime task ownership and end-to-end OpenD fixture/live differential; until then keep Go owner and default field absent.

Runtime-task evidence: `OpenDSessionRuntime` now owns the explicit polling
thread, shared `TickCache`, dynamic demand source, bounded `poll_once` /
`poll_snapshot` cadence, reconnect counters, error projection, and joined
shutdown. `ProductRuntimeConfig::with_opend_session_runtime` is the only
composition path that enables it; the handle exposes a controlled demand
update method and rejects a task config without an authenticated coordinator.

quirk: The task is still opt-in and synchronous at the integration boundary;
it does not activate ProviderRouter or change default desktop behavior.
范围: `system-status-read` / OpenD runtime task and dynamic demand
证据: Rust `runtime_task::tests::runtime_task_updates_dynamic_demand_and_shuts_down_its_coordinator`,
`product_runtime::tests::opend_runtime_task_requires_explicit_session_composition`,
and `ProductRuntimeHandle::set_market_data_opend_demand`.
分类: rust-implementation
判定: intended
处置: 保留显式 runtime task seam；在 production cutover 前补真实 OpenD fixture/live differential、backoff/recovery、ProviderRouter activation 和 release recovery evidence.
风险: high
owner: Rust integration / engine composition
后续: hard cut 前证明 runtime task 与唯一 market-data owner 的 end-to-end lifecycle；直到那时继续保持 Go owner。

Verification: `cargo test -p jftrade-marketdata --lib -- --nocapture`; `cargo test -p jftrade-integration-futu --lib -- --nocapture`; `cargo clippy -p jftrade-integration-futu --all-targets -- -D warnings`; `go test ./pkg/futu -run '^(TestWithClientReplayPolicyForRecoverableErrors|TestQuoteSnapshotNonFinitePriceCorpusRecordsGoFailureBoundary)$' -count=1`; `cargo test -p jftrade-api websocket -- --nocapture`; `cargo test -p jftrade-store-settings-file --test interface_settings_contracts`; `go test ./internal/store/settingsfile -run '^TestLiveWebSocketInterfaceSettingsMatchRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-api observability::tests::request_observability_matches_stage9_go_corpus -- --exact`; `go test ./pkg/observability -run '^TestRequestObservabilityMatchesRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-engine --lib product::product_market_data_runtime_status::tests::market_data_runtime_projection_matches_go_status_corpus -- --exact`; `go test ./internal/app/apiserver/status -run '^TestMarketDataRuntimeStatusMatchesRustMigrationCorpus$' -count=1`; `cargo test -p jftrade-engine --lib product::product_strategy_runtime_status::tests::strategy_runtime_projection_matches_go_status_corpus -- --exact`; `go test ./internal/app/apiserver/status -run '^TestStrategyRuntimeStatusMatchesRustMigrationCorpus$' -count=1`; `node --test scripts/lib/tauri-runtime.test.mjs scripts/lib/desktop-release-metadata.test.mjs`; `cargo test -p jftrade-engine --lib product_runtime::tests::product_runtime_without_optional_workers_starts_and_stops_cleanly -- --exact`; `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests -- --nocapture`; `go test ./internal/app/apiserver/servercoretest -run '^TestSystemStatusReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `pnpm run check:rust`.

Current route coverage remains 1 shadow / 118 cutover-test-only / 159
cutover-qualified / 0 remaining / 0 Rust production owner. The ledger retains
`productionOwner=go` and `goRemovalStatus=retained`.

## Explicit OpenD provider bridge (2026-08-25)

Rust now has an opt-in `OpenDProviderRuntime` composition boundary. It probes
OpenD, maps the probe to broker-neutral `HealthStatus`, registers and explicitly
activates one Futu descriptor in a supplied `ProviderRouter`, acquires the
initial demand, and starts `OpenDSessionRuntime` against the router's exact
recorder, demand snapshot and `TickCache` handles. The runtime task rejects a
recorder mismatch and ignores direct `set_demand` when the router is its demand
owner, preventing a second demand/cache lifecycle. Startup failure deactivates
the provider and releases the bridge demand. Product runtime exposes this only
through `with_opend_provider_runtime`; default desktop composition still leaves
the field absent, does not probe/connect OpenD, and does not activate a
ProviderRouter. No route ownership or production owner changed.

quirk: The explicit provider bridge performs a short health probe before opening
the long-lived push session, preserving the existing Go-compatible probe/data
connection role split while avoiding duplicate ProviderRouter or recorder state.
范围: `system-status-read` / Futu OpenD provider composition
证据: `OpenDProviderRuntime`, `OpenDSessionRuntime::start_with_provider_router`,
`ProviderRouter::cache_handle`, and router/runtime task unit tests.
分类: rust-implementation
判定: intended
处置: retain as explicit composition; do not call from the default profile.
风险: high
owner: Rust integration / engine composition
后续: connect real provider lifecycle only after live OpenD differential,
reconnect/backoff, release recovery and owner qualification are complete.

## Provider health feedback (2026-08-25)

`OpenDSessionRuntime` now feeds the active provider slot from the same recorder
used by the product status port. For active demand, stream/quote errors or a
closed session map to `ProviderReadiness::Failed`; an active connected
generation maps back to `Ready`; a connected-but-not-yet-ready generation maps
to `Warming`. Empty demand leaves the initial provider health unchanged, so the
bridge does not manufacture a failure while idle. The feedback is only wired
by `start_with_provider_router` and remains absent from the default desktop
profile. Router/runtime tests cover failure and recovery without a real OpenD.

quirk: Provider health is derived from the recorder rather than independently
probing or writing a second lifecycle state, preserving one generation/error
source and avoiding provider/router double ownership.
范围: `system-status-read` / OpenD provider health feedback
证据: `sync_provider_health` and
`runtime_task::tests::provider_health_sync_replays_recorder_failure_and_recovery`.
分类: rust-implementation
判定: intended
处置: retain explicit feedback; qualify only after real OpenD reconnect/live
differential and release recovery evidence.
风险: high
owner: Rust integration / engine composition
后续: keep Go production owner and route shadow until all closeout gates pass.

## Bounded reconnect backoff (2026-08-25)

`OpenDSessionRuntimeConfig` now carries positive initial and maximum reconnect
delays. The runtime task applies a bounded exponential delay after a coordinator
error, resets the failure streak after a successful reconnect or healthy
iteration, and never spins on a closed or unreachable session. Zero durations
use safe defaults and a maximum below the initial delay is normalized upward.
The pure delay boundary test covers cap and overflow behavior, while an
authenticated mock TCP trace covers peer close, one failed reconnect handshake,
the minimum backoff interval, successful replay and the runtime reconnect count;
no real OpenD is dialed by this slice.

quirk: backoff is task-local scheduling state rather than a second recorder or
provider lifecycle. The coordinator remains the sole generation/reconnect owner.
范围: `system-status-read` / OpenD runtime reconnect scheduling
证据: `reconnect_delay_is_bounded_and_recovers_from_zero_or_overflowing_inputs`,
`runtime_task_backoff_replays_after_a_failed_reconnect_attempt` and the explicit
runtime task config.
分类: rust-implementation
判定: intended
处置: retain opt-in backoff; qualify only with live OpenD reconnect, release
recovery, and owner-gate evidence.
风险: high
owner: Rust integration / engine composition
后续: exercise bounded recovery against authenticated live/mocked reconnect
traces before any ProviderRouter production activation or Go owner switch.

## Provider bridge shutdown fencing (2026-08-25)

`OpenDProviderRuntime::shutdown` and its Drop path now join/close the runtime
first, release the bridge-owned demand consumer, and then deactivate the active
provider. A router regression proves the active provider and bridge demand are
both cleared; the router's managed-consumer guard remains authoritative when
other owners are present. This closes an in-process state-leak path but does not
qualify real OpenD or change the production owner.

quirk: provider descriptors remain registered as static catalog entries after
deactivation; only active runtime state and demand are fenced away.
范围: `system-status-read` / explicit OpenD provider bridge shutdown
证据: `provider_runtime::tests::release_and_deactivate_clears_bridge_owned_router_state`.
分类: rust-implementation
判定: intended
处置: retain shutdown fencing; require live OpenD, release recovery and
cross-process owner evidence before production activation.
风险: high
owner: Rust integration / engine composition
后续: exercise shutdown/restart against authenticated live and release traces.

## Provider bridge startup rollback (2026-08-25)

Provider bridge registration, activation and initial demand acquisition now run
as one rollback-aware composition step. A failed demand consumer or instrument
validation releases the bridge demand (if any) and deactivates only the provider
activated by this bridge; an activation failure before ownership is established
does not touch an existing active provider. The regression uses a ready router
and an empty consumer id to prove the active provider and demand are cleared.

quirk: static provider registration remains in the router catalog after rollback;
only the failed bridge's active runtime state is fenced.
范围: `system-status-read` / explicit OpenD provider bridge startup
证据: `provider_runtime::tests::provider_configuration_rolls_back_activation_when_demand_validation_fails`.
分类: rust-implementation
判定: intended
处置: retain rollback fencing; require live OpenD and cross-process owner evidence
before production activation.
风险: high
owner: Rust integration / engine composition
后续: exercise provider start failure/retry against authenticated release traces.
