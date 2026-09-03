# WS Live Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `ws-live`
- Tier: B; this is the live WebSocket route and therefore includes handshake, long-lived event ordering, provider/runtime failure, cancellation, reconnect and close behavior.
- Operations: 1 GET route: `/api/v1/ws/live`.
- Current ownership: `cutover-test-only`; the route is registered only when the explicit product test-cutover profile supplies `WsLiveSnapshotPort`. Go remains the production owner.
- Production owner: Go remains the only production owner of WebSocket transport, provider/OpenD lifecycle, market-data demand, notification replay, market ticks and depth update bridges. The Rust test-cutover transport owns only its ephemeral client connection/subscription registry; it never connects an external service or writes state.
- Fixture: `tests/fixtures/rust-migration/stage9/ws-live.json`.
- Go reference: `scripts/rust-migration/stage9_ws_live_reference_test.go`.
- Rust replay: `crates/jftrade-engine/src/product_ws_live.rs` and `crates/jftrade-engine/tests/stage9_ws_live.rs`.
- Differential: `scripts/rust-migration/check-stage9-ws-live.mjs`.

## Product transport evidence

- The Axum WebSocket boundary preserves the Go upgrader's plain-text
  `404 page not found` and `Forbidden` responses for an unavailable route and
  rejected Origin. Generic API routes retain their JSON Origin error mapping;
  only `/api/v1/ws/live` defers Origin rejection to the WebSocket handler.
- Product tests cover explicit-port registration, default-profile isolation,
  101 handshake, subscription normalization and permit release, connection
  limit `503 LIVE_WS_LIMIT_REACHED`, rejected Origin, and unavailable-route
  plain-text behavior. The route remains test-cutover-only and no
  Provider/OpenD or durable backend is connected.
- The Go fixture/reference, Rust leaf replay, API transport tests, Rust
  product tests, and full Stage 9 product differential all pass. No production
  owner or default profile changes.

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

## Integration wiring and qualification state

The explicit `WsLiveSnapshotPort` product wiring and route isolation are now
present. The default profile remains at 48 routes and does not register
`/api/v1/ws/live`; the test-cutover profile adds only this route. The group
must remain `cutover-test-only`: Rust still has no live backend, Provider/OpenD
demand reconciliation, notification bridge, market tick/depth source, or
durable subscription owner. The plain-text Origin and abnormal-close quirks
remain recorded Go compatibility behavior, and the generated swagger setup,
real-provider recovery, release, security, backup/restore, and hard-cut gates
remain open.

## Integration Review

- Product wiring now gates the existing authenticated loopback WebSocket handler on an explicit `WsLiveSnapshotPort`; the default profile remains at 48 routes and does not register `/api/v1/ws/live`.
- The authenticated Rust transport now owns one shared typed connection registry and client-scoped RAII permit used by both its upgrade limit and the `system/status` live projection. Its effective limit comes from the read-only Go-compatible interface settings projection. The actual handler consumes Go-compatible subscribe messages, projects a sorted/deduplicated active-instrument union and removes it on disconnect. This is ephemeral test-cutover transport state only; Go remains the production owner and Rust does not reconcile Provider/OpenD demand.
- The shared differential runs `TestStage9WSLiveFixtureMatchesCurrentGoOwner`, authenticated Go rehearsals, route isolation, and a real loopback 101/subscribe/status/disconnect test. The standalone Go/Rust replay remains the full event-wire evidence; no Provider/OpenD demand reconciliation, notification bridge, market tick/depth source or SQLite lifecycle crosses into Rust.
- Three-way review (Go handler/reference, Rust replay, harness): replay matches the captured Go corpus. The plain-text Origin rejection, abnormal close behavior, replay ordering, and missing generated `docs/swagger` webaccess setup remain recorded quirks and block qualification.
