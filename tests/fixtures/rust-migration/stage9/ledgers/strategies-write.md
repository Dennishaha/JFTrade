# Strategies Runtime Write Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `strategies-write`
- Tier: A: seven strategy-instance mutation/control operations change catalog or live runtime state.
- Operations: 7: `PUT /api/v1/strategies/{instanceId}`, `PUT /api/v1/strategies/{instanceId}/runtime-risk`, `POST /api/v1/strategies/{instanceId}/pause`, `POST /api/v1/strategies/{instanceId}/stop`, `POST /api/v1/strategies/{instanceId}/start`, `POST /api/v1/strategies/{instanceId}/refresh-definition`, and `DELETE /api/v1/strategies/{instanceId}`.
- Current route-ownership status after integration: all seven are
  `cutover-test-only` only when the explicit `StrategyRuntimeWritePort` is
  injected; Go remains the production owner. Current coverage is `1 shadow /
  118 cutover-test-only / 159 cutover-qualified / 0 remaining / 0 Rust
  production owner`.
- Production owner: Go remains the only owner of the strategy catalog, runtime manager, PineTS lifecycle, subscriptions, activity/notification side effects, and SQLite writes. Rust is a leaf replay boundary only.
- Rust boundary: `product_strategy_runtime_write_port.rs` accepts a complete consumer-owned mutation port. The isolated adapter in `product_strategy_runtime_write_test_cutover.rs` is compiled only for tests and persists fixture catalog/runtime projections; it does not open the Go strategy database or register a default-profile, broker/OpenD, PineTS, notification, task, listener, or production-owner path.
- Fixture: `tests/fixtures/rust-migration/stage9/strategies-write.json` (35 cases / 36 requests; success, malformed/null/trailing JSON, 400/404/500/502, repeated pause, cancellation, timeout, compensation, and all seven routes).
- Go reference: `scripts/rust-migration/stage9_strategies_write_reference_test.go` uses only temporary Gin handlers and recording fakes; it does not open production stores or start a runtime.
- Rust leaf test: `crates/jftrade-engine/tests/stage9_strategies_write.rs`.
- Differential: `node scripts/rust-migration/check-stage9-strategies-write.mjs`.

## Contract ledger

| Method | Path | Go observable behavior | Failure/error precedence |
| --- | --- | --- | --- |
| PUT | `/api/v1/strategies/{instanceId}` | Binds `InstanceBinding`, ignores unknown JSON fields, accepts JSON `null` as a zero-value binding, and the current Gin JSON binder accepts the first value when trailing JSON follows. | Binding errors are `400 BAD_REQUEST` before catalog access; catalog `NotFound`/`Busy` map to `404`/`400`; generic store errors map to `500 STRATEGY_FAILED` with the route fallback message. |
| PUT | `/api/v1/strategies/{instanceId}/runtime-risk` | Binds `RuntimeRiskSettings` with the same unknown/null/trailing-value behavior and delegates the complete risk projection. | Malformed input is `400 BAD_REQUEST` before the catalog; `NotFound`/generic failures map to `404`/`500 STRATEGY_FAILED`. |
| POST | `/api/v1/strategies/{instanceId}/pause` | Transitions the catalog to `PAUSED`, then stops the runtime and refreshes the live market stream in the service layer. Repeated requests repeat the transition/stop side effects. | Catalog failure is mapped before runtime stop; `NotFound`/`Busy` map to `404`/`400`. An upstream error follows the shared runtime-start failure code. |
| POST | `/api/v1/strategies/{instanceId}/stop` | Transitions to `STOPPED`, then stops the runtime and refreshes the live market stream. Request body is ignored. | Catalog failure is mapped before runtime stop; `NotFound`/`Busy` map to `404`/`400`, generic failure to `500 STRATEGY_FAILED`. |
| POST | `/api/v1/strategies/{instanceId}/start` | Looks up the instance, validates startability, starts the runtime with the request context, transitions to `RUNNING`, and stops the runtime if the transition fails. Request body is ignored. | Missing/not-startable are `404`/`400`; Pine worker capacity is a `400` localized busy message; runtime cancellation/timeout and generic start failures are `502 STRATEGY_RUNTIME_START_FAILED`; transition failure preserves the same rollback/error mapping. |
| POST | `/api/v1/strategies/{instanceId}/refresh-definition` | Delegates instance-definition lookup and refresh to the catalog. Request body is ignored. | `NotFound` is `404`; generic refresh failure is `500 STRATEGY_FAILED`. |
| DELETE | `/api/v1/strategies/{instanceId}` | Delegates deletion to the catalog. Request body is ignored. | `NotFound`/`Busy` map to `404`/`400`; generic repository failure is `500 STRATEGY_FAILED`. |

## Three-way quirks and qualification gaps

quirk: `PUT` binding and runtime-risk handlers accept JSON `null` as a zero-value Go struct and preserve the first JSON value when a second value trails it.
范围: `strategies-write` / both PUT routes
证据: Go fixture cases `update-null-body-zero-value`, `update-trailing-json-first-value-wins`, `runtime-risk-null-body-zero-value`, and `runtime-risk-trailing-json-first-value-wins`; Rust leaf replay and boundary-input assertions; dedicated differential output.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: preserve binder semantics in every production adapter; decide whether the compatibility exception is retained after hard-cut.

quirk: An upstream error from the pause transition is returned as `502 STRATEGY_RUNTIME_START_FAILED`, reusing the shared mapper's start-failure code for a pause route.
范围: `strategies-write` / POST `/api/v1/strategies/{instanceId}/pause`
证据: Go fixture case `pause-upstream-uses-start-failure-code`; Rust port error replay; dedicated differential output.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go until cutover
后续: keep the exact code/message precedence in the rehearsal; review the user-visible code only after the Go owner is no longer authoritative.

quirk: A canceled or deadline-exceeded start context is mapped to `502 STRATEGY_RUNTIME_START_FAILED` with the raw context error message instead of a client-cancellation/timeout status.
范围: `strategies-write` / POST `/api/v1/strategies/{instanceId}/start`
证据: Go fixture cases `start-context-cancelled-maps-gateway` and `start-timeout-maps-gateway`; Rust leaf replay; dedicated differential output.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go until cutover
后续: retain the mapping for wire compatibility; add real request cancellation, runtime join, and recovery evidence before qualification.

quirk: Repeating `pause` requests produces two successful catalog transitions and two runtime-stop calls; no idempotency key or duplicate suppression is observable in this route slice.
范围: `strategies-write` / POST `/api/v1/strategies/{instanceId}/pause`
证据: Go fixture case `pause-repeat-replays-side-effects` records two transition/stop observations; Rust leaf receives two mutation calls; differential is green.
分类: go-behavior
判定: unresolved
处置: 复刻 captured behavior; block qualification until repeated-request, transaction, restart, and owner-fencing semantics are reviewed.
风险: release-blocker
owner: Go/integration branch
后续: resolve with durable runtime/catalog replay before any owner switch; do not infer idempotency from the leaf port.

## Verification and handoff

The authenticated Go loopback rehearsal now proves private Bearer/internal
protocol fencing, browser Cookie/Origin/Referer/CSRF forwarding, all seven
operations, duplicate pause forwarding, 502 runtime failure, timeout,
cancellation, Rust crash fail-closed behavior, safe Go rollback/restart through
malformed-input validation before the real catalog owner, and unchanged
settings bytes. The Rust product replay proves default/no-port isolation,
browser 401/403 fencing, unavailable/error recovery, all seven route
projections, duplicate pause forwarding, restart, and no settings mutation.
Both use injected ports and temporary settings; neither opens the production
strategy store, starts PineTS, connects Provider/OpenD, changes subscriptions,
or emits activity/notification/task side effects.

- Go fixture/reference: fixture generation passed with `JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES=1`, followed by a no-update drift replay.
- Rust leaf and durable test-cutover replay: 8 tests passed, including all 35 fixture cases, exact seven-route inventory, malformed-input precedence, JSON binder compatibility, read isolation, unavailable-port fencing, concurrent repeated pause transitions, start-transaction rollback, running-delete guard, and close/reopen persistence.
- Dedicated differential: passed with `node scripts/rust-migration/check-stage9-strategies-write.mjs`.
- `go test ./internal/app/apiserver/servercoretest -run '^TestStrategiesWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1 -timeout=300s` — passed.
- `cargo test -p jftrade-engine --lib strategy_runtime_write_product -- --nocapture` — passed; explicit-registration, authenticated failure/recovery/restart, and isolated SQLite product restart tests passed.
- The full Stage 9 product differential passed with 220 Rust product library tests plus the integration and supporting-package batches.
- `pnpm run check:quick` — passed for the five affected files, including the
  full `servercoretest`, all-target Rust tests, Clippy, architecture checks,
  and generated-contract drift check.
- `pnpm run check:rust` — passed, including target health, layout, rustfmt,
  workspace Clippy/tests, Stage 2-9 differential, and supporting packages.
- Final route coverage, both migration JSON validations, and `git diff --check`
  — passed.

The group remains `cutover-test-only`. Production catalog/runtime owner fencing,
cross-process repeated-request policy, cancellation/timeout joins, real PineTS and
subscription recovery, activity/notification/task isolation, four-platform
release/signing, security/SBOM, backup/restore, and hard-cut gates remain open.
