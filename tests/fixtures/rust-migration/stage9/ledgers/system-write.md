# System Write Group Ledger

- Group: `system-write`
- Tier: A: real-trade safety controls and OpenD runtime reset are state-changing operations. The Rust side is a test-only rehearsal leaf; it is not a production owner.
- Dynamic baseline before this worker: `278 baseline / 26 shadow / 228 cutover-test-only / 0 cutover-qualified / 24 remaining / 0 Rust production owner`.
- Routes in this group: 7 system mutations. The integration branch now registers
  them only behind the explicit `SystemWritePort`; ownership is
  `cutover-test-only`, while Go remains the production owner.
- Go owner: `internal/api/system`, `internal/system.Service`, `internal/trading.RealTradeControlPlane`, `internal/app/apiserver/futuapp` and the production composition root. Go continues to own OpenD, real-trade state, persistence, broker safety decisions, notifications, and the formal HTTP/Wails entry points.
- Rust rehearsal owner: `crates/jftrade-engine/src/product_system_write_port.rs` behind an injected `SystemWritePort` used only by `crates/jftrade-engine/tests/stage9_system_write.rs`. The leaf has no SQLite, broker, OpenD, notification, task, or persistence capability.
- Fixture: `tests/fixtures/rust-migration/stage9/system-write.json` (`47` cases, `68` requests).
- Go reference: `scripts/rust-migration/stage9_system_write_reference_test.go`.
- Differential: `scripts/rust-migration/check-stage9-system-write.mjs`.

## Route contract

All successful and failed responses use the Go envelope with a normalized fixture timestamp and `Content-Type: application/json; charset=utf-8`. The Rust leaf preserves the timestamp field only as an injected replay value; production transport must continue to source it at the transport boundary.

| Method | Path | Request/body behavior | Go observable success and failure behavior |
| --- | --- | --- | --- |
| DELETE | `/api/v1/system/real-trade-risk-limits` | Optional JSON object. Empty body and `null` become a zero command; the first JSON value is decoded and trailing values are ignored. | `200` with the returned `RealTradeRiskSnapshot`; malformed/non-object JSON is `400 BAD_REQUEST`; callback/service failures are `409 REAL_TRADE_CONTROL_FAILED`. |
| POST | `/api/v1/system/futu-opend/manual-retry` | Body is not decoded or inspected. | Calls `ResetFutuRuntime` and always returns `200` with `{"accepted":true}`; the body and request cancellation do not change this result. |
| POST | `/api/v1/system/real-trade-hard-stops` | Required JSON object. `null` is accepted as a zero-value command; unknown fields and trailing values are ignored. | `200` with the returned snapshot; empty/malformed/non-object JSON is `400 BAD_REQUEST`; callback/service failures are `409 REAL_TRADE_CONTROL_FAILED`. |
| POST | `/api/v1/system/real-trade-hard-stops/{hardStopId}/release` | Optional JSON object. The path ID is trimmed before the service call and cannot be replaced by body fields. | Blank trimmed ID is `400 BAD_REQUEST`; empty/`null` body succeeds; malformed/non-object JSON is `400`; callback/service failures are `409`. |
| POST | `/api/v1/system/real-trade-kill-switch/activate` | Required JSON object. `null` is accepted as a zero-value command; unknown fields and trailing values are ignored. | `200` with the returned snapshot; shape errors are `400`; callback/service failures are `409 REAL_TRADE_CONTROL_FAILED`. |
| POST | `/api/v1/system/real-trade-kill-switch/release` | Optional JSON object. Empty/`null` body succeeds; first JSON value wins. | `200` with the returned snapshot; malformed/non-object JSON is `400`; callback/service failures are `409 REAL_TRADE_CONTROL_FAILED`. |
| PUT | `/api/v1/system/real-trade-risk-limits` | Required JSON object. `null` is accepted as a zero-value command; unknown fields and trailing values are ignored. | Handler validation runs before the service: non-positive provided limits and enabling without a positive quantity/notional limit are `400`; callback/service failures are `409`. |

The direct Go handlers pass `c.Request.Context()` to every real-trade control callback. A callback returning `context.Canceled` or `context.DeadlineExceeded` is still projected as `409 REAL_TRADE_CONTROL_FAILED`, with the Go error text in the message. Manual retry does not receive the request context and remains accepted after cancellation.

## Evidence and three-way quirk review

The Go reference generated the fixture from the live Gin handlers and injected `Service` callbacks. The Rust replay consumes the same fixture through the leaf port, compares status, headers, envelope, decoded command and call count, and is run by the dedicated differential script. The following records were reviewed against all three legs after the replay passed.

### Quirks

quirk: Required and optional JSON bodies have different EOF behavior; `null` becomes a zero-value command; the first JSON value is accepted while trailing JSON and unknown object fields are ignored.
范围: `system-write` / POST hard-stop activate, POST kill-switch activate, PUT risk update; DELETE risk, POST hard-stop release, POST kill-switch release
证据: Go fixture cases `*-required-body-errors`, `*-null-and-trailing`, `*-malformed-before-port`; `system_write_fixture_replays_go_wire_for_all_seven_routes`; `check-stage9-system-write.mjs`
分类: go-behavior
判定: intended
处置: Rust leaf reproduces the Go boundary; retain the behavior until any post-hard-cut API change is explicitly approved.
风险: medium
owner: Go owner / Rust integration adapter
后续: hard-cut 前保持 fixture and differential coverage.

quirk: `POST /api/v1/system/futu-opend/manual-retry` ignores malformed or arbitrary body bytes and still returns `200 {"accepted":true}`. It also ignores an already-canceled request context because the handler does not pass context to the reset callback.
范围: `system-write` / POST `/api/v1/system/futu-opend/manual-retry`
证据: Go fixture cases `manual-retry-ignores-malformed-and-repeats` and `manual-retry-canceled-still-accepted`; Rust manual-retry replay and `system_write_port_unavailable_maps_to_fail_closed_503`; dedicated differential output.
分类: go-behavior
判定: intended
处置: Reproduce observable Go behavior in the explicit test-cutover adapter; do not silently add cancellation or payload validation in this migration slice.
风险: medium
owner: Go system/futuapp owner
后续: review as a separate safety/API change before any production owner cutover.

quirk: Control callback errors, including cancellation and deadline errors, are mapped to HTTP `409 REAL_TRADE_CONTROL_FAILED` instead of a transport-specific `499`/`504` response.
范围: `system-write` / all six real-trade control mutations
证据: Go fixture cases `*-control-failure`, `*-canceled`, and `*-deadline`; Rust error replay; dedicated differential.
分类: go-behavior
判定: intended
处置: Preserve the error status/message in the Rust test port; the integration adapter must forward cancellation to the Go owner without retrying or dispatching a second mutation.
风险: high
owner: Go system/trading owner / integration branch
后续: hard-cut safety review must explicitly approve the fail-closed 409 contract and cancellation fencing.

quirk: Runtime risk validation runs in the HTTP handler before the service callback. Non-positive provided values and `realTradingEnabled=true` without a positive quantity or notional limit never reach the control owner.
范围: `system-write` / PUT `/api/v1/system/real-trade-risk-limits`
证据: Go fixture `risk-update-validation-errors` has `portCalls=false` for all three requests; Rust validation-before-port test; dedicated differential.
分类: go-behavior
判定: intended
处置: Keep validation and error messages byte-compatible; do not delegate invalid values to a Rust domain implementation in this slice.
风险: high
owner: Go API owner / Rust integration adapter
后续: hard-cut checklist must retain the validation-before-owner fence.

quirk: The hard-stop release path parameter is trimmed and takes precedence over any body content; an encoded whitespace-only ID returns `400 hard stop id is required` before the port is called.
范围: `system-write` / POST `/api/v1/system/real-trade-hard-stops/{hardStopId}/release`
证据: Go fixture `hard-stop-release-success-trimmed-path` and `hard-stop-release-blank-id`; Rust path replay and observation comparison; dedicated differential.
分类: go-behavior
判定: intended
处置: Decode/trim only at the transport edge and pass the path ID separately from the optional command body.
风险: high
owner: Go API owner / Rust integration adapter
后续: retain path/query differential before hard-cut.

quirk: Repeated activation/release/update requests are forwarded independently; there is no route-level idempotency or duplicate suppression in the Go handlers.
范围: `system-write` / all real-trade mutation paths
证据: Go fixture cases `*-repeated`, whose observations contain two owner calls; Rust replay compares both calls; dedicated differential.
分类: go-behavior
判定: intended
处置: Reproduce the observable forwarding behavior while keeping the Rust route test-only; do not invent an idempotency key or local state.
风险: release-blocker
owner: Go trading/system owner / integration branch
后续: A-tier hard-cut requires an explicit duplicate-request, rollback, and recovery decision before qualification.

quirk: The Go owner serializes integral JSON float values such as `2500.0` as `2500`; an initial Rust `f64` serializer emitted `2500.0`.
范围: `system-write` / runtime risk command observation
证据: Go fixture `risk-update-null-and-trailing`; Rust replay initially failed on `2500.0` vs `2500`, then passed after `serialize_go_optional_float`; dedicated differential.
分类: rust-implementation
判定: deviated
处置: Fixed in `product_system_write_port.rs` by a Go-compatible optional-float serializer; no Go change.
风险: medium
owner: Rust worker
后续: retain integer/fractional numeric golden cases and re-run if command DTOs change.

quirk: A missing injected Rust test port is fail-closed as `503 SYSTEM_WRITE_UNAVAILABLE`; the Go service's unset callback path instead returns `409 REAL_TRADE_CONTROL_FAILED`, while a production Rust route is absent without explicit port wiring.
范围: `system-write` / all seven routes, test-cutover registration boundary
证据: Go `Service` nil-control behavior and existing system route failure tests; Rust `system_write_leaf_fails_closed_after_shape_validation` and `system_write_port_unavailable_maps_to_fail_closed_503`; dedicated leaf harness.
分类: harness
判定: intended
处置: Keep the Rust route unregistered without the injected port. The integration adapter must map an actual Go-owner failure as the fixture says and must never expose a portless production route.
风险: release-blocker
owner: integration branch
后续: resolve in the shared wiring/product route-isolation tests before any ownership status changes.

## Owner and fencing

- No default profile, read-only shadow catalog, public listener, SQLite path, OpenD socket, broker command, notification, or production owner was changed.
- The injected port is the only mutation capability in the Rust leaf. It returns captured Go-owned data or an explicit error and has no method for persistence or side effects.
- The Rust route must be registered only when `ProductConfig::test_cutover` carries the system write port. Without it, the route must remain absent; with an unavailable adapter, it must fail closed and never retry Go.
- Go remains the sole writer and safety decision owner. Any future owner switch must be one composition-root choice with rollback/fencing evidence; no dual dispatch is allowed.

## Integration hook

The shared integration applied the smallest patch to:

1. Add an optional `SystemWritePort` to the product test-cutover port set and system route capability.
2. Dispatch exact method/path matches through this leaf before the existing read routes, preserving shared auth/access-surface and raw-response wire handling.
3. Add system write route-isolation tests proving default absence, port-unavailable fail-closed behavior, read/write method isolation, and no changes to the 26 shadow routes.
4. Update `route-ownership.json` and the shared stage9 coverage/differential ledger for the seven operations only; they now remain `cutover-test-only`.

The shared files are now integrated on the parent branch; generated contracts and
the default production profile remain unchanged.

## Verification

- `go test scripts/rust-migration/stage9_system_write_reference_test.go -run '^TestStage9SystemWriteFixtureMatchesCurrentGoOwner$' -count=1` — passed.
- `cargo test -p jftrade-engine --test stage9_system_write -- --nocapture` — passed; 9 tests, including 47-case/68-request fixture replay.
- `node scripts/rust-migration/check-stage9-system-write.mjs` — passed.
- Product route isolation, Rust layout, focused Clippy, rustfmt, and the unified
  Stage 9 product differential — passed.
- `git diff --check` — passed.

The current dynamic route gate is `26 shadow / 242 cutover-test-only / 0
qualified / 10 remaining / 0 Rust production owner`; unique owner, recovery,
release, security, and hard-cut gates remain open.
