# Market-Data Subscription Mutation Group Ledger

- Group: `market-data-subscription-mutation`
- Tier: A; subscription acquisition, release, heartbeat, clear, and prediction lease mutations change live demand or upstream subscription state.
- Operations: 6 routes; 55 frozen request cases / 55 HTTP requests.
- Worker status: complete as a test-only leaf and fixture package. Integration must register the group as `cutover-test-only`; this worker deliberately does not edit `route-ownership.json` or shared product assembly.
- Production owner: Go remains the sole owner of subscription registry, demand reconciliation, prediction eligibility, Provider/OpenD lifecycle, lease state, persistence, cleanup, and user-visible live updates.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-subscription-mutation.json` (55 cases).
- Go reference: `scripts/rust-migration/stage9_market_data_subscription_mutation_reference_test.go`.
- Rust leaf: `crates/jftrade-engine/src/product_market_data_subscription_mutation_{port,api,routes}.rs`.
- Rust replay: `crates/jftrade-engine/tests/stage9_market_data_subscription_mutation.rs`.
- Differential: `node scripts/rust-migration/check-stage9-market-data-subscription-mutation.mjs`.
- Current ownership records are `cutover-test-only` for all six routes;
  `productionOwner=go` and `goRemovalStatus=retained` remain unchanged. This
  group is not registered in the default shadow profile and still requires the
  explicit mutation test port plus durable-owner/recovery evidence before any
  qualification or production-owner discussion.

## Operation Contract

| Method | Path | Request/response and observable branches |
| --- | --- | --- |
| DELETE | `/api/v1/market-data/prediction/contracts/{code}/subscriptions/{leaseId}` | Empty-body lease release; success is `{released:true}`; blank lease is `400 BAD_REQUEST`, unknown lease is idempotent success, upstream failure is `502 BROKER_FEATURE_FAILED`. The path `code` is not used to look up the lease. |
| DELETE | `/api/v1/market-data/subscriptions` | Optional `consumerId` query; success returns the post-clear snapshot with `cleared:true`; cancellation/service failure is `500 SUBSCRIPTION_FAILED`. A blank query value clears all non-managed web subscriptions. |
| POST | `/api/v1/market-data/prediction/contracts/{code}/subscriptions` | JSON `dataTypes`; code and data types are normalized by Go; success returns a lease and provider attribution; malformed JSON is `400`; invalid data types are `400`; ineligible account is `403`; unavailable broker is `409`; upstream/canceled failure is `502`. |
| POST | `/api/v1/market-data/subscriptions` | JSON consumer/instrument request; success returns a subscription snapshot; non-Futu `providerBrokerId` uses the zero-state polling fallback; malformed/invalid requests are `400`; service/cancellation failure is `500`. |
| POST | `/api/v1/market-data/subscriptions/heartbeat` | JSON consumer request; success returns the snapshot; non-Futu provider uses the zero-state polling fallback; malformed/blank consumer is `400`; service/cancellation failure is `500`. |
| POST | `/api/v1/market-data/subscriptions/release` | JSON consumer and optional first target; success returns the post-release snapshot with `released:true`; non-Futu provider uses the zero-state polling fallback; malformed/invalid requests are `400`; service/cancellation failure is `500`. |

## Three-Way Review

The Go owner reference, frozen fixture, and Rust replay were compared at the raw method/path/query/body boundary. The fixture contains 24 successful responses, 21 `400` responses, one `403`, one `409`, four `500`, and four `502`; no retry header is emitted by these current handlers. Rust forwards every valid JSON POST and every DELETE unchanged to the injected port, while malformed POST bodies are rejected at the leaf with the route-specific Go message. The port is fixture-only and never starts a provider, opens SQLite, or mutates subscription state.

## Reviewed Quirks

quirk: Gin/Go JSON binding consumes the first JSON value and ignores trailing JSON, so `{"consumerId":"chart","instruments":[]} {}` follows the normal required-field path instead of the malformed-body path.
范围: `market-data-subscription-mutation` / POST acquire, release, heartbeat, and prediction acquire
证据: Go cases `acquire-trailing-json`, `release-trailing-json`, `heartbeat-trailing-json`, `prediction-acquire-trailing-json`; frozen fixture; Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Rust leaf changed to accept a decodable first JSON value and reproduce the Go observable result; preserve until the Go owner is cut over.
风险: medium
owner: Go until cutover; Rust leaf reviewed by worker
后续: Do not replace this with strict whole-body JSON validation without an approved wire-contract change.

quirk: A JSON `null` body binds to the zero-value request and reaches required-field validation, while malformed bytes use the route-specific `invalid ... request` message.
范围: `market-data-subscription-mutation` / all four POST body-bound routes
证据: `*-null-body` and `*-malformed-json` fixture cases, Go reference, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce; the Rust port receives valid `null` and leaves business validation to the Go-owned adapter projection.
风险: medium
owner: Go until cutover
后续: Keep null/omitted/malformed spellings as separate corpus cases.

quirk: A non-Futu `providerBrokerId` is trimmed and lower-cased in a zero-state `snapshot-poll-fallback` response and does not create or remove a Futu subscription lease.
范围: `market-data-subscription-mutation` / POST acquire, release, heartbeat
证据: `acquire-polling-fallback`, `release-polling-fallback`, and `heartbeat-polling-fallback`; Go service/handler; Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce; no Rust subscription registry or provider fallback was introduced.
风险: high
owner: Go until provider/subscription owner cutover
后续: Preserve provider selection and no-state-change fencing in the eventual adapter.

quirk: Acquire filters instruments with blank market or symbol before validation; if all entries are filtered, it emits the generic `consumerId and instruments are required` error rather than the lower-level market/symbol message.
范围: `market-data-subscription-mutation` / POST `/api/v1/market-data/subscriptions`
证据: `acquire-all-instruments-invalid`, Go `subscriptionInstruments`, frozen fixture, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce; do not normalize or validate the request independently in the Rust leaf.
风险: medium
owner: Go until cutover
后续: Keep the raw body in the adapter contract and preserve binding order.

quirk: Release uses only the first instrument target and ignores additional targets; the remaining targets stay active.
范围: `market-data-subscription-mutation` / POST `/api/v1/market-data/subscriptions/release`
证据: `release-only-first-target`, Go `subscriptionReleaseTarget`, frozen post-release snapshot, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce until a separately approved public contract change; Rust does not expand the operation to batch release.
风险: high
owner: Go until cutover
后续: Add an explicit batch-release contract before changing this behavior.

quirk: A percent-encoded blank `consumerId` on DELETE is trimmed to empty and therefore clears all non-managed web subscriptions.
范围: `market-data-subscription-mutation` / DELETE `/api/v1/market-data/subscriptions`
证据: `clear-blank-consumer-means-all`, Go query binding and registry clear behavior, frozen fixture, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce; keep query text opaque to the Rust port.
风险: high
owner: Go until cutover
后续: Include encoded whitespace and omitted query cases in hard-cut transport review.

quirk: Prediction acquire upper-cases and prefixes the contract code with `US.`, trims/upper-cases data types, removes duplicates, sorts them, and selects depth only for the single `ORDER_BOOK` type.
范围: `market-data-subscription-mutation` / POST prediction subscription
证据: `prediction-acquire-normalizes-types`, `prediction-acquire-order-book`, provider-call evidence, Go prediction service, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce in the Go-owned adapter; the Rust leaf only preserves the captured response and raw request.
风险: high
owner: Go until prediction capability cutover
后续: Keep eligibility, capability selection, and normalization outside the pure Rust leaf.

quirk: Prediction release keys state only by trimmed `leaseId`; the path contract code is ignored, and releasing an unknown lease is idempotent success.
范围: `market-data-subscription-mutation` / DELETE prediction lease
证据: `prediction-release-code-does-not-rebind-lease`, `prediction-release-unknown-is-idempotent`, `prediction-release-blank-lease`, Go `ReleasePredictionSubscription`, frozen fixture, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Reproduce until the Go owner is cut over; do not add code/lease consistency checks in Rust.
风险: high
owner: Go until cutover
后续: Treat lease persistence, restart recovery, and idempotency as release gates.

quirk: Market-data cancellation errors map to `500 SUBSCRIPTION_FAILED`; prediction provider/cancellation errors map to `502 BROKER_FEATURE_FAILED`, while missing broker and eligibility are prioritized as `409` and `403` respectively.
范围: all six routes / cancellation and provider/error precedence
证据: canceled cases, prediction provider failure, missing broker, ineligible account, Go error mapping, frozen fixture, Rust replay.
分类: go-behavior
判定: intended compatibility behavior
处置: Preserve exact status/code/message precedence; do not improve errors in the migration slice.
风险: high
owner: Go until cutover; integration owns transport review
后续: Add cancellation fencing and restart/recovery evidence before qualification.

quirk: The first Rust replay implementation rejected trailing JSON even though Go accepted the first value; this was a Rust implementation difference, fixed before the differential was accepted.
范围: `market-data-subscription-mutation` / Rust leaf body gate
证据: initial Rust replay failure on the four trailing-json cases, Go fixture output, updated Rust `body_starts_with_json_value`, final replay.
分类: rust-implementation
判定: deviated then fixed
处置: Fix Rust to match Go; retain the regression fixture and test.
风险: medium
owner: worker
后续: Keep the first-value decoder behavior covered in every future body-bound mutation group.

quirk: Focused handler fixtures execute below shared authentication middleware; `401` is therefore not synthesized in this corpus, while Rust leaf tests cover unknown-route `404` and missing-port `503`.
范围: all six routes / transport boundary
证据: Go reference router setup, shared transport contract tests, Rust fail-closed tests.
分类: harness
判定: intended
处置: Keep auth in the shared transport owner and do not invent operation-level `401` cases here.
风险: medium
owner: integration branch
后续: Include auth/CSRF and authenticated loopback evidence in integration cutover review.

quirk: Production durable idempotency, production lease persistence, cross-process cancellation fencing, crash/restart recovery, and one authoritative subscription owner are not proven by the isolated test adapter.
范围: entire Tier A group / all six mutations
证据: existing Go owner ledger; the new `cfg(test)` SQLite adapter and integration replay use an isolated `market_data_test_*` schema and temporary database, not the Go production schema or Provider/OpenD runtime.
分类: ownership
判定: unresolved
处置: Block `cutover-qualified`; keep Go as sole production owner and resolve only with the production owner matrix and serial integration/release gates.
风险: release-blocker
owner: integration plus Go owner
后续: Require durable owner matrix, backup/restore, crash recovery, duplicate-request, cancel/timeout, and no-double-write evidence before any owner switch.

## Wave Closeout (2026-08-26)

- Added `cfg(test)`-only `MarketDataSubscriptionMutationSqliteTestCutoverPort`. It uses a temporary isolated SQLite file and test-prefixed tables for consumer subscriptions, prediction leases, IDs, and events; it never opens production SQLite, connects Provider/OpenD, reconciles live demand, or emits user-visible updates.
- Added coverage for event-failure transaction rollback, acquire/release/heartbeat/clear, prediction lease allocation and release, one-winner concurrent release fencing, close/reopen persistence, and independent integration-level replay. The product-level replay also exercised all six routes through the explicit test-cutover profile and verified settings bytes were unchanged.
- Passed: `cargo test -p jftrade-engine --test stage9_market_data_subscription_mutation -- --nocapture` (8 tests), `cargo test -p jftrade-engine --lib market_data_subscription_mutations_sqlite_test_cutover_replays_transport_and_restart -- --nocapture`, the dedicated Go/Rust differential, `pnpm run check:quick`, and `pnpm run check:rust`.
- This closes the local isolated durability rehearsal evidence only. All six operations remain `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`; production schema compatibility, cross-process fencing, live Provider/OpenD lifecycle, backup/restore, release/security gates, and hard-cut evidence remain open.

## Qualification Blockers

- The six target records are now recorded as `cutover-test-only` with the fixture, Go reference, Rust replay, authenticated loopback rehearsal, Rust product replay, dedicated differential, and shared product differential evidence. They retain `productionOwner=go` and `goRemovalStatus=retained`.
- The Rust leaf is wired only behind an authenticated explicit test-cutover port and remains absent from the default product profile; no Rust production owner was added.
- Provider/OpenD lifecycle, subscription demand reconciliation, prediction eligibility, SQLite/durable lease state, duplicate-request semantics, cancellation fencing, restart recovery, four-platform release/signing/security/SBOM, and hard-cut remain release gates.

## Authenticated Product Rehearsal

- Go loopback rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestMarketDataSubscriptionMutationRehearsalPreservesBrowserBoundaryAndRecoversAcrossRestart$' -count=1` passed for all six routes. It verified private bearer and internal-proxy fencing, browser Cookie/Origin/Referer/CSRF forwarding, success and provider-error mapping, timeout, request cancellation, Rust crash fail-closed behavior, Go rollback, Go restart recovery, and unchanged settings bytes.
- Rust product replay: `cargo test -p jftrade-engine --lib market_data_subscription_mutations -- --nocapture` passed with unauthorized/CSRF-failure fencing, unavailable/provider-failure mapping, all six route projections, request path/query forwarding, explicit test-cutover registration, restart, and unchanged settings bytes. The injected port is a fixture boundary and does not create a provider, lease registry, SQLite write, or user-visible side effect.
- The rehearsal proves transport and test-cutover isolation only. It does not prove durable lease persistence, authoritative demand reconciliation, idempotency/transaction semantics, cancellation fencing in the live owner, or crash recovery of Provider/OpenD and subscription state.

## Integration Handoff

- Add the three Rust source files to the product composition only behind the explicit `MarketDataSubscriptionMutationPort` test-cutover capability; do not alter the default profile. Keep the independent integration replay in the test suite.
- Supply a Go-owned adapter that returns the complete wire projection for the raw request and preserves `500/502/403/409` error precedence. It must not duplicate subscription state or call a second provider owner.
- Before any qualification or owner switch, add production-owner durable state, idempotency, cross-process cancellation fencing, Provider/OpenD lifecycle, backup/restore, and crash/restart recovery evidence; the isolated adapter is not a substitute and this group remains `cutover-test-only` until those gates close.
- Keep `GET /api/v1/market-data/subscriptions` in its existing read group; this mutation worker intentionally rejects GET.
