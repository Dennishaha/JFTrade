# Watchlists Remote Write Group Ledger

- Group: `watchlists-remote-write`
- Tier: A mutation / external broker state change; this worker remains rehearsal-only.
- Operations: 1
  - `POST /api/v1/watchlists/remote`
- Current status: `remaining`; integration has not registered a Rust route and has not changed `route-ownership.json`.
- Go owner: `internal/api/productfeatures/routes.go`, `internal/productfeatures/service.go`, `pkg/broker/router.go`, and the selected broker adapter remain the only production handler, capability-resolution, OpenD, and remote-watchlist mutation owner.
- Rust boundary: `product_watchlist_remote_write_port.rs` binds the exact POST path and delegates broker resolution plus the external mutation to a consumer-owned test port. It has no OpenD, broker SDK, SQLite, remote registry, notification, or default-profile registration.
- Fixture: `tests/fixtures/rust-migration/stage9/watchlists-remote-write.json`
- Go reference: `scripts/rust-migration/stage9_watchlists_remote_write_reference_test.go`
- Differential: `scripts/rust-migration/check-stage9-watchlists-remote-write.mjs`
- Rust behavior test: `crates/jftrade-engine/tests/stage9_watchlists_remote_write.rs`

## Contract ledger

| Method | Path | Request and success wire | Error branches and precedence |
| --- | --- | --- | --- |
| POST | `/api/v1/watchlists/remote` | The generic Go customization handler accepts a JSON object or JSON `null`, reads the first `brokerId`/`accountId` query values, and sends `featureId=watchlist.remote.modify`, `action=modify`, and the decoded payload to the selected `CustomizationService`. Success is HTTP 200 with `{ok:true,data,timestamp}` and `Content-Type: application/json; charset=utf-8`; `data.provider` is overwritten with the selected broker attribution. | Empty, malformed, trailing, or non-object JSON is HTTP 400 `BAD_REQUEST` / `invalid request body` before capability resolution. Missing broker, unavailable capability, or missing `CustomizationService` is HTTP 409 `BROKER_CAPABILITY_UNAVAILABLE`. Provider 4xx is preserved as `PROVIDER_REQUEST_FAILED`; rate limiting is HTTP 429 `MARKET_SNAPSHOT_RATE_LIMITED` with rounded-up `Retry-After`; other broker/provider errors are HTTP 502 `BROKER_FEATURE_FAILED`. A missing Rust test port is fail-closed HTTP 503 `WATCHLIST_REMOTE_WRITE_UNAVAILABLE` and is not a Go production response. |

The fixture contains 19 cases covering explicit/default broker selection, repeated query values, null/empty/object/malformed/array bodies, capability and adapter failures, provider 4xx/5xx, rate limiting, cancellation/deadline errors, duplicate forwarding, and failure recovery. It normalizes only the dynamic HTTP envelope and provider timestamps after validating RFC3339Nano.

## Owner and fencing evidence

- `POST /api/v1/watchlists/remote` is not registered by this worker. The default Rust profile, production Go router, OpenD, broker registry, remote watchlist state, and all external writes remain unchanged.
- The Rust port returns no second broker owner and has no persistence or notification side effect. An integration adapter must explicitly own capability resolution and call the current Go/test fixture boundary before any test-cutover registration.
- Repeated requests are replayed as two independent broker calls, matching the Go handler's lack of an idempotency key or local write fence. The leaf also replays one broker failure followed by a successful next request without adding retry behavior inside Rust.
- The Rust test asserts the exact one-route inventory, malformed-body precedence, missing-port fencing, all fixture envelopes, action payload states, repeated forwarding, and failure recovery.

## Quirks and three-way review

quirk: JSON `null` is accepted by Gin's `map[string]any` binding, becomes a nil `CustomizationAction.Payload`, still reaches the broker mutation, and can return HTTP 200.
范围: `watchlists-remote-write` / POST `/api/v1/watchlists/remote`
证据: Go reference `null-body-nil-payload`, fixture call trace, and Rust `null_and_empty_object_preserve_go_payload_states` plus fixture replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后评估是否收紧 body contract
风险: medium
owner: Go / 集成分支
后续: hard-cut 前保留 differential；任何收紧必须作为单独公开契约变更。

quirk: An empty JSON object is accepted and forwarded as an empty payload; `omitempty` removes the payload field from the action trace even though the handler reached the broker.
范围: `watchlists-remote-write` / POST `/api/v1/watchlists/remote`
证据: Go reference `empty-object-payload`, fixture `payloadState=empty_object`, and Rust fixture replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后评估
风险: medium
owner: Go / 集成分支
后续: preserve payload-state evidence through test-cutover and any live differential.

quirk: Repeating the same POST does not deduplicate or fence the external mutation; Go resolves and applies it twice, while a failed call does not prevent a later request from reaching the broker.
范围: `watchlists-remote-write` / POST `/api/v1/watchlists/remote`
证据: Go reference `repeated-write-is-forwarded-twice` and `failed-write-recovers-on-next-request`, fixture apply counts/action traces, and Rust `remote_watchlist_write_leaf_replays_failure_recovery_and_duplicate_forwarding`.
分类: go-behavior
判定: intended
处置: 复刻，待硬切前补充 broker-side idempotency/replay acceptance evidence
风险: high
owner: Go / 集成分支
后续: release-blocker until duplicate/retry semantics and external broker recovery are explicitly accepted for cutover.

quirk: A cancelled request or an expired deadline returned by the broker adapter maps through the default Go branch to HTTP 502 `BROKER_FEATURE_FAILED` with the raw context error, rather than a transport-specific 499/504.
范围: `watchlists-remote-write` / POST `/api/v1/watchlists/remote`
证据: Go reference `cancelled-request-defaults-to-broker-failure` and `deadline-request-defaults-to-broker-failure`, fixture envelopes, and Rust error replay.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复或明确接受
风险: high
owner: Go / 集成分支
后续: release-blocker until cancellation, timeout, and remote broker partial-commit behavior are separately reviewed.

quirk: A 2.5 second snapshot-rate-limit error becomes `Retry-After: 3` because Go rounds the duration up to whole seconds in `writeQueryError`.
范围: `watchlists-remote-write` / POST `/api/v1/watchlists/remote`
证据: Go reference `generic-rate-limit-retry-after`, fixture header, and Rust `RateLimited` mapping.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后评估
风险: low
owner: Go / 集成分支
后续: retain exact header value in any cutover differential.

quirk: The handler and provider attribution use request-time timestamps, so only the envelope and `provider.resolvedAt`/`provider.asOf` fields are canonicalized to the fixed fixture clock.
范围: all 19 fixture cases
证据: Go reference timestamp validation/normalization, fixture, and Rust fixed-timestamp replay.
分类: fixture
判定: intended
处置: 修复 fixture/harness canonicalization；不放宽 timestamp format validation
风险: low
owner: 集成分支
后续: live differential must validate format, monotonic/clock source, and field presence separately.

## Verification record

- Go owner fixture drift test: passed (`go test scripts/rust-migration/stage9_watchlists_remote_write_reference_test.go -run '^TestStage9WatchlistsRemoteWriteFixtureMatchesCurrentGoOwner$' -count=1`).
- Rust leaf/behavior test: passed (8 tests, `cargo test -p jftrade-engine --test stage9_watchlists_remote_write -- --nocapture`).
- Dedicated differential: passed (`node scripts/rust-migration/check-stage9-watchlists-remote-write.mjs`).
- Differential script syntax: passed (`node --check scripts/rust-migration/check-stage9-watchlists-remote-write.mjs`).
- Narrow Go owner regression packages: passed (`go test ./internal/api/productfeatures ./internal/productfeatures ./pkg/broker -count=1`).
- Narrow Rust clippy: passed (`cargo clippy -p jftrade-engine --test stage9_watchlists_remote_write -- -D warnings`).
- Rust formatting check: passed (`cargo fmt --all -- --check`).
- Shared product wiring, `route-ownership.json`, unified product differential, `check:quick`, full `check:rust`, and `check:generated` are intentionally not run or claimed by this worker because they belong to the integration branch or are outside the requested file boundary.

## Handoff state

- Group: `watchlists-remote-write`
- Tier: A
- Operation count: 1 (19 fixture cases)
- Status: leaf/replay verified; integration registration and ownership status remain unchanged (`remaining`, Go owner retained).
- Next qualification action: integration must add the explicit mutation test port and route isolation, then complete durable external-write, cancellation/timeout, duplicate/retry, recovery, release, and hard-cut evidence before any owner change.
