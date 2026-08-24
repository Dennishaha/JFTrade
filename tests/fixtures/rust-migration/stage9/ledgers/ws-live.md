# WS Live Group Ledger

- Group: `ws-live`
- Tier: B; this is the live WebSocket route and therefore includes handshake, long-lived event ordering, provider/runtime failure, cancellation, reconnect and close behavior.
- Operations: 1 GET route: `/api/v1/ws/live`.
- Current ownership: `cutover-test-only`; the route is registered only when the explicit product test-cutover profile supplies `WsLiveSnapshotPort`. Go remains the production owner.
- Production owner: Go remains the only production owner of WebSocket transport, live client registry, provider/OpenD lifecycle, subscriptions, notification replay, market ticks and depth update bridges. Rust replay is fixture-only and never connects an external service or writes state.
- Fixture: `tests/fixtures/rust-migration/stage9/ws-live.json`.
- Go reference: `scripts/rust-migration/stage9_ws_live_reference_test.go`.
- Rust replay: `crates/jftrade-engine/src/product_ws_live.rs` and `crates/jftrade-engine/tests/stage9_ws_live.rs`.
- Differential: `scripts/rust-migration/check-stage9-ws-live.mjs`.

## Contract Corpus

| Scenario | Handshake / lifecycle | Events and wire points | Failure / recovery |
| --- | --- | --- | --- |
| `heartbeat-no-origin-with-desktop-protocol` | GET upgrade with no Origin and `jftrade.desktop.v1` offer | Initial heartbeat envelope, source/entity fields, interval and live-client fields | Successful handshake |
| `subscription-event-order-and-normalization` | Subscribe after initial heartbeat | Subscription update heartbeat, console refresh, security details, depth in Go dispatcher order; active/depth normalization and caps | Successful stream |
| `notification-replay-tick-and-deduplication` | Subscribe to one active instrument | Sequence-zero notification replay, heartbeat, tick envelope, broker tag, unchanged `observedAt` dedup | Provider/event replay uses fixture backend only |
| `depth-push-refresh-and-release` | Subscribe depth, emit one update, close client | Initial and pushed `market.depth`, entity ID and resolved-at refresh | Depth callback is released on disconnect |
| `invalid-subscription-closes-without-code-frame` | Malformed subscription has no provider broker | Initial heartbeat only | Go closes underlying connection; close observation is captured |
| `provider-error-cancels-stream` | Provider returns an error during data refresh | Heartbeat frames before failure | Stream is cancelled and closes without an application error event |
| `server-close-cancels-active-client` | Server handler Close while client is active | Initial heartbeat | Cancellation and close observation |
| `client-reconnects-after-disconnect` | Close first client, dial a second client on same handler | One initial heartbeat per connection | Reconnect and per-connection bridge cleanup |
| `origin-forbidden-during-handshake` | Untrusted Origin | No event | Gorilla upgrader plain-text 403 |
| `connection-limit-rejection` | Second client while limit is occupied | First client heartbeat | JSON 503 `LIVE_WS_LIMIT_REACHED` |
| `backend-unavailable-is-not-found` | Handler has no backend | No event | Plain-text 404 |

All dynamic RFC3339 values are canonicalized to `fixture-time` in the Go reference while preserving raw JSON field order. Event envelopes are serialized as ordered structs in the Rust replay; payload maps retain Go-compatible sorted JSON keys.

## Quirks and Three-Way Review

quirk: `go test ./internal/app/apiserver/webaccess -run TestWebSocketUsesCookieSession` cannot reach the test body in this checkout because the generated `docs/swagger` package is absent.
范围: `ws-live` / GET `/api/v1/ws/live` / webaccess harness setup
证据: baseline command output: `internal/app/apiserver/servercore/openapi.go:10: no required module provides package github.com/jftrade/jftrade-main/docs/swagger`; `go test ./internal/api/live/... -run 'TestHandler|TestDispatcher' -count=1` passes.
分类: harness
判定: unresolved
处置: 修复 fixture/harness
风险: medium
owner: 集成分支
后续: regenerate the documented OpenAPI package or run the webaccess auth baseline in the generated-artifact job before route qualification; this worker does not modify generated files.

quirk: the Go handler rejects an untrusted Origin through Gorilla's plain-text upgrader response instead of the standard JSON API error envelope.
范围: `ws-live` / GET `/api/v1/ws/live` / handshake Origin rejection
证据: `origin-forbidden-during-handshake` fixture, Go reference capture and Rust replay.
分类: go-behavior
判定: unresolved
处置: 复刻，待硬切后修复
风险: high
owner: Go / 集成分支
后续: preserve the exact 403 body and content type until the shared transport owner is deliberately changed and a public wire migration is approved.

quirk: malformed subscriptions without `providerBrokerId` and backend/provider errors terminate the WebSocket by closing the underlying connection; no application error event or explicit close frame is observable.
范围: `ws-live` / GET `/api/v1/ws/live` / invalid subscription and provider failure
证据: `invalid-subscription-closes-without-code-frame` and `provider-error-cancels-stream` fixture cases, Go close observations and Rust replay.
分类: go-behavior
判定: unresolved
处置: 复刻，待硬切后修复
风险: high
owner: Go / 集成分支
后续: keep the observed abnormal-closure semantics in replay; do not add close codes, error events or fallback-to-Go behavior during this migration slice.

quirk: the first `writeLiveData` pass runs before a provider subscription is installed, so retained notifications are sent immediately after the initial heartbeat and before subscription-triggered data events.
范围: `ws-live` / GET `/api/v1/ws/live` / notification replay ordering
证据: `notification-replay-tick-and-deduplication` fixture and Go dispatcher `writeInitialEvents`/`writeLiveData` order.
分类: go-behavior
判定: unresolved
处置: 复刻，待硬切后修复
风险: medium
owner: Go / 集成分支
后续: preserve sequence-zero replay order until a separately approved product behavior change.

quirk: invoking `cargo test -p jftrade-engine` with a lib test filter also schedules unrelated stage shadow binaries; in this checkout one such binary can remain running after the filtered product test has passed.
范围: `ws-live` / shared product differential harness
证据: the filtered product test passed, while the package-level cargo process remained alive in `jftrade-stage5-shadow`; `cargo test -p jftrade-engine --lib <filter> -- --exact` exits normally.
分类: harness
判定: unresolved
处置: 修复 fixture/harness
风险: medium
owner: 集成分支
后续: keep the shared differential runner restricted to the engine lib target until package target test behavior is independently fixed; this does not change production code or wire behavior.

quirk: the worker differential script had a blank line at EOF, which made the repository diff gate reject the new harness file.
范围: `ws-live` / `scripts/rust-migration/check-stage9-ws-live.mjs`
证据: `pnpm run check:quick` failed in `check-diff` at line 32 before running affected tests; Go/Rust replay output was already green.
分类: harness
判定: deviated
处置: 修复 fixture/harness
风险: low
owner: 集成分支
后续: fixed by removing the trailing blank line; rerun `check:quick` before commit.

## Integration Wiring Patch Plan

The worker intentionally does not touch shared wiring. The integration branch should apply the smallest patch after cherry-picking this commit:

1. Add a private `WsLiveSnapshotPort`/live replay adapter to the product composition root only when an explicit test-cutover profile supplies a fixture backend; keep the default capability set and production route catalog unchanged.
2. Add one `WsLive` capability/port bit and the single `GET /api/v1/ws/live` route to `crates/jftrade-engine/src/product*.rs`, reusing the existing authenticated loopback WebSocket transport. The adapter must delegate all backend/provider/runtime lifecycle behavior through a narrow consumer-owned port and must not start OpenD, subscribe, publish notifications or write SQLite.
3. Update `tests/fixtures/rust-migration/stage9/route-ownership.json` for this one operation from `remaining` to `cutover-test-only`, add evidence `scripts/rust-migration/check-stage9-ws-live.mjs`, and derive the new total `26 shadow / 149 cutover-test-only / 0 qualified / 103 remaining / 0 Rust production owner`.
4. Extend the shared product differential and route-isolation tests only on the integration branch; preserve Go fallback/owner and add an explicit test proving the route is absent without the injected port.

Do not mark this group `cutover-qualified`: the current Rust shared transport does not yet expose the Go live backend, and the high-risk close/origin quirks remain unresolved.

## Integration Review

- Product wiring now gates the existing authenticated loopback WebSocket handler on an explicit `WsLiveSnapshotPort`; the default profile remains at 48 routes and does not register `/api/v1/ws/live`.
- The authenticated Rust transport now owns one shared typed connection counter and RAII permit used by both its upgrade limit and the `system/status` live projection. This covers only Rust transport concurrency; the Go live client/subscription registry remains the production owner.
- The shared differential runs `TestStage9WSLiveFixtureMatchesCurrentGoOwner` and the product route-isolation test. The standalone Go/Rust replay remains the wire evidence; no provider, OpenD, subscription, notification, or SQLite lifecycle crosses into Rust.
- Three-way review (Go handler/reference, Rust replay, harness): replay matches the captured Go corpus. The plain-text Origin rejection, abnormal close behavior, replay ordering, and missing generated `docs/swagger` webaccess setup remain recorded quirks and block qualification.
