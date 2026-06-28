# PineTS Hard-Cut Migration Plan

> Goal: replace the Go Pine runtime with PineTS workers, remove the already-built Go TradingView parity path, and keep Go as the trading, risk, order, and backtest authority.

## Current Decision

- Runtime target: `sourceFormat=pine-v6` + `runtime=pine-pinets`.
- Legacy runtime: `pine-go-plan` is migration-only and must not remain a selectable execution path.
- License assumption: PineTS commercial license is available before the worker is shipped in release binaries.
- Execution authority: PineTS computes Pine outputs and order intents; Go remains authoritative for backtest matching, equity curves, live risk, and order placement.
- Release shape: Bun SEA / Bun single-file executable workers are built with `bun build --compile`, embedded into one Go `trading-engine` binary, and started as localhost gRPC child processes.
- File-size guardrail: new or materially rewritten files must stay under 1200 lines.

## Progress Tracker

| Phase | Status | Evidence / Exit Criteria |
| --- | --- | --- |
| 0. Plan and guardrails | Done | This document exists; coverage and performance gates are documented; focused verification is recorded below. |
| 1. Pine worker contract | Done | `pkg/strategy/pineworker` owns `pine-pinets` constants, request/response shapes, order intent schema, worker defaults, validation, and perf gate helpers. |
| 1.1 Runtime ID normalization | Done | Server-side definition/catalog normalization emits `pine-pinets` and migrates old `pine-go-plan`; focused servercore tests pass. |
| 2. Proto contract | Done | `pkg/strategy/pineworker/proto/pineworker.proto` mirrors the Go contract and compiles through `protoc`. |
| 3. Worker PoC | Done | Bun worker core validates requests, adapts custom OHLCV data to the PineTS constructor shape, normalizes plots/logs/order intents, exposes a gRPC server boundary, and has Bun tests. |
| 4. gRPC bridge | Done | Go worker client abstraction, generated Go gRPC transport, Bun gRPC server boundary, static JS gRPC runtime deps, and mock process smoke are covered. |
| 5. Worker manager | Done | Go `WorkerManager` starts fixed worker specs, assigns ports, dials transports, round-robins healthy workers, restarts failed health checks, drains on shutdown, and exposes snapshots. Binary extraction launcher, gRPC dialer, API-server lifecycle wiring, embedded asset selection, and Bun mock process smoke coverage are implemented. |
| 6. Backtest integration | Done | `pkg/backtest` has a Pine worker adapter, replay planner, command executor, replay pump, and `RunWithPineWorker`; `internal/backtest.Service` defaults to the Pine worker path and API startup injects a configured `WorkerManager` from `JFTRADE_PINEWORKER_BINARY`. Missing worker config now fails fast instead of falling back to Go runtime. |
| 7. Live integration | Done | Bar-close live flow now builds Pine worker `live` requests, filters current-bar order intents, applies Go risk/notification/order placement, records runtime observation/errors, and does not fall back to Go Pine runtime. |
| 8. Hard removal | Done | Public Pine spec/runtime payloads now emit `pine-pinets`; direct `pkg/backtest.Run` no longer imports or executes the Go Pine runtime and fails fast; current architecture, performance, and completion docs now point to the PineTS worker boundary; the old Go Pine runtime package has been deleted. |
| 9. Packaging | Blocked for release | `scripts/build-pineworker-assets.sh` checks commercial PineTS package/license attestation before building platform Bun SEA / single-file executable workers with `bun build --compile` into `internal/pineworkerassets/assets/bin`; Go selects the matching embedded asset under `release_assets` and falls back to external env config in development. Mock process smoke compiles and runs through real gRPC. Release packaging remains blocked on the commercial `pinets` package/license and real PineTS process smoke. |
| 10. Acceptance | Blocked for release | Focused Go/web/worker tests, mock worker process smoke, coverage, performance gate, file-size checks, and web typecheck pass. `scripts/check-pinets-release.sh` automates the release gates and runs `TestWorkerManagerRealPineTSProcessSmoke` in strict mode once `pinets` is installed. Final release acceptance still depends on the real PineTS package/license smoke passing through the Bun SEA packaged worker path. |

## Runtime Boundary

Go owns:

- market data ingestion and K-line persistence
- strategy instances, parameter combinations, and scheduling
- backtest replay, matching, fills, trades, equity, and metrics
- live risk controls and order submission
- broker and exchange APIs
- worker pool lifecycle and observability

PineTS worker owns:

- Pine Script parsing/execution through PineTS
- Pine input/default resolution supplied by request params
- plots, debug logs, warnings, diagnostics, alerts
- `strategy.*` call extraction into normalized order intents

PineTS worker must not be the source of truth for final trades, live orders, account state, or risk decisions.

## Release Blockers

- The commercial `pinets` package is not installed in the current workspace; `npm ls pinets --workspaces --depth=1` reports empty.
- Public `pinets@0.9.26` currently reports `AGPL-3.0-only`; release acceptance requires a recorded commercial license approval and `JFTRADE_PINETS_COMMERCIAL_LICENSE_ACK=1`.
- Production worker startup defaults to the native PineTS executor; mock mode requires explicit `JFTRADE_PINEWORKER_MOCK=true` or `--mock true` and is test-only.
- Release binaries must not ship until a real PineTS worker process smoke passes without mock mode.
- The real PineTS package/license decision must be recorded before embedding worker assets in release builds.
- Release and operator acceptance is tracked in [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md).

## Worker PoC Boundary

The first Bun worker slice lives under `workers/pineworker` and intentionally avoids adding `pinets` to root lockfiles until the commercial license and package-management policy are finalized.

- `NativePineTSExecutor` dynamically imports `pinets` and constructs `new PineTS(candles)` for custom OHLCV execution.
- `runScriptWithPineTS` validates requests before dispatch and maps both validation/runtime failures into worker error responses.
- Adapter normalization currently covers plots, outputs, logs, warnings, diagnostics, metadata, and normalized order intents.
- `startWorkerGrpcServer` uses `@grpc/grpc-js` and `@grpc/proto-loader`, registers health/analyze/run handlers, and enforces gRPC send/receive message limits.
- `DeterministicPineTSExecutor` exists only for fast contract tests; it must not become a production fallback.
- `scripts/check-pinets-release.sh` runs the PineTS release acceptance gates and treats a missing `pinets` workspace dependency as a release blocker.

## Contract Shape

The Go contract layer starts in `pkg/strategy/pineworker` and later maps 1:1 to protobuf.

- `RuntimeID`: `pine-pinets`.
- `LegacyRuntimeID`: `pine-go-plan`, accepted only for migration normalization.
- `RunScriptRequest`: job/script identity, source, symbol, timeframe, candles, params, and mode.
- `RunScriptResponse`: outputs, order intents, plots, logs, warnings, diagnostics, worker metadata, and optional error.
- `OrderIntent`: normalized representation of Pine `entry/order/exit/close/close_all/cancel/cancel_all`.
- `PerformanceGate`: max duration, max per-candle duration, max RSS, max payload, and min candles/sec thresholds.

## Go Client Boundary

`pkg/strategy/pineworker.Client` is transport-neutral so backtest/live integration can depend on a stable Go API before generated gRPC stubs exist.

- `Transport` is the swappable boundary for real gRPC, fake workers, and future in-process tests.
- `Client.RunScript` validates requests before dispatch, enforces max message bytes, applies context deadlines, maps transport/worker errors, fills missing metadata, and checks performance gates.
- `GRPCTransport` maps between protobuf messages and the local Go contract, with bufconn coverage for `HealthCheck` and `RunScript`.
- Production integrations must use the client rather than calling gRPC stubs directly.

## Worker Manager Boundary

`pkg/strategy/pineworker.WorkerManager` owns lifecycle and scheduling policy while keeping process launch and transport dialing injectable.

- `WorkerLauncher` is the future seam for extracting embedded Bun worker binaries and starting child processes.
- `TransportDialer` is the future seam for localhost gRPC clients.
- `BinaryWorkerLauncher` verifies SHA256, writes the selected worker binary to a temp executable, starts it with address/worker/proto/message-limit args, and stops/removes it.
- `GRPCDialer` creates localhost gRPC transports with send/receive message limits.
- API startup reads `JFTRADE_PINEWORKER_*` environment settings, starts the worker manager when a worker binary is configured, injects it into backtest service, and stops it during `Server.Close`.
- `internal/pineworkerassets` selects platform-specific embedded worker binaries under `release_assets`; API startup uses external `JFTRADE_PINEWORKER_BINARY` first, then embedded assets.
- Current manager tests cover fixed port allocation, round-robin dispatch, health-check restart, failed restart reporting, startup cleanup, shutdown cleanup, snapshot state, binary checksum, process cleanup, dialer creation, and a gated Bun mock process smoke path through real gRPC.
- Real PineTS dependency lock policy and non-mock process smoke remain release blockers.

## Bun SEA Packaging Boundary

The release packaging direction is Bun SEA / Bun single-file executable workers plus one embedded Go release binary.

- `scripts/build-pineworker-assets.sh` compiles one worker executable per target platform with `bun build --compile --target=...`.
- Generated worker executables are staged under `internal/pineworkerassets/assets/bin` and embedded only for `release_assets` builds.
- `go build -tags release_assets -o dist/trading-engine ./cmd/jftrade-api` produces the single published `trading-engine` binary.
- At runtime, Go extracts the matching embedded worker executable to a temporary directory, verifies SHA256, starts a fixed localhost gRPC worker pool, and removes the temp executable on shutdown.
- Development can still override the embedded asset with `JFTRADE_PINEWORKER_BINARY` for local worker iteration.
- This packaging path does not change ownership boundaries: PineTS workers calculate signals/plots/debug/order intents, while Go remains authoritative for matching, risk, orders, equity, and exchange APIs.

## Backtest Integration Boundary

`pkg/backtest.PineWorkerBacktestAdapter`, `PineWorkerReplayPlanner`, `PineWorkerCommandExecutor`, `PineWorkerReplayPump`, and `RunWithPineWorker` are the backtest-facing contracts for worker execution.

- It forces `RunScriptRequest.Mode` to `backtest` and maps worker/transport errors into backtest errors.
- It converts worker `OrderIntent` values into `WorkerOrderCommand` records with Go-side side, order type, quantity, limit/stop, comments, alerts, bar index, and time.
- The replay planner converts `types.KLine` to worker candles, builds `RunScriptRequest`, copies params, applies default job IDs, validates returned command bar indexes, fills missing command times from the source candle, and groups commands by bar index/open time.
- The command executor resolves session markets, submits market/limit/stop commands through bbgo `SubmitOrders`, tracks created orders by Pine id/client id, and maps `cancel`/`cancel_all` to bbgo `CancelOrders`.
- The replay pump validates replay candle order, feeds each K-line into bbgo matching, then executes that bar's close-generated worker commands so they are eligible for later-bar matching.
- `RunWithPineWorker` loads the same K-line store and bbgo backtest exchange, collects replay K-lines for worker planning, routes worker intents through Go matching, and uses the existing result collector for trades/equity/metrics without instantiating the former Go Pine runtime.
- `internal/backtest.Service` accepts a `WithPineWorkerRunner` dependency and its default runner now requires that Pine worker dependency instead of calling `bt.Run`.
- API server startup no longer injects `bt.Run`; it injects a started Pine worker manager only when `JFTRADE_PINEWORKER_BINARY` is configured and otherwise leaves service-level fail-fast behavior in place.
- Quantity-percent commands currently fail fast until Go-side position sizing is wired, because Go remains authoritative for account/position state.
- Current tests cover entry, exit, cancel-all, default entry quantity, unsupported intents, transport errors, worker errors, replay request construction, replay K-line collection, params propagation, command grouping, invalid bar indexes, worker timeout propagation, market/limit order submission, cancel/cancel-all, unsupported sizing, submit/cancel error propagation, replay shape validation, missing/extra bars, consume-before-command ordering, an end-to-end `RunWithPineWorker` smoke through Go matching, service-level fail-fast when no Pine worker runner is configured, and API startup wiring for configured/absent worker managers.
- Direct `pkg/backtest.Run` no longer executes the Go Pine runtime; it fails fast and points callers to `RunWithPineWorker`. Live bar-close execution is routed through Pine worker order intents.
- Public Pine spec payloads, generated support snapshots, and current frontend authoring docs now advertise `runtime=pine-pinets`; `pine-go-plan` remains only as a migration alias or historical release note.
- Frontend strategy definition saves, runtime-panel display, and strategy page test fixtures now use shared `pine-pinets` runtime identity helpers. `StrategyRuntimePanel.vue` and `strategyPageTestUtils.ts` have both been split below the 1200-line guardrail.
- Current maintenance docs no longer recommend the old Go Pine runtime, direct backtest runner, or TradingView full-parity roadmap; historical release notes may still describe their original release line.

## Live Integration Boundary

`internal/app/apiserver/servercore` now keeps live K-line aggregation, broker account refresh, risk evaluation, notifications, and order placement in Go while delegating Pine execution to the configured `pineworker.WorkerManager`.

- `strategyRuntimePineWorkerLive` builds `ModeLive` requests from warmup + closed candles, copies supported instance params, and sends the script/source/symbol/timeframe to the worker.
- The live path only executes worker order intents for the just-closed bar, preventing historical replayed intents from submitting duplicate live orders.
- Worker `entry/order/exit/close` intents are mapped into bbgo submit orders and then passed through the existing notify-only or live order executors, preserving Go risk controls and broker APIs.
- Worker errors are recorded as runtime errors and persisted in runtime observation; Go does not fall back to the former Go Pine runtime.
- Pine semantics such as default percent sizing, pyramiding, and script-level order decisions now belong to PineTS worker output. Go live execution requires explicit worker-sized quantities and fails fast for quantity-percent intents until live position sizing is implemented in the worker contract.

## Coverage Gates

- New Go packages must have focused table tests for normalization, validation, defaulting, error mapping, and performance gate decisions.
- Worker adapter code must test both success and malformed PineTS responses.
- Backtest integration must cover at least:
  - MA cross entry/close
  - RSI or CCI signal
  - Bollinger or Donchian signal
  - market order, stop/limit, bracket exit, cancel
  - params changing output
  - worker timeout and worker error
- Live integration must cover:
  - notify-only mode
  - real-trade risk block
  - real-trade accepted order intent
  - worker crash between bars
- UI removal must update tests that referenced enum execution, external-series readiness, and Go Pine support boundary.

## Performance Gates

Initial gates are intentionally conservative until real worker benchmarks exist:

- 10k candles single run: no more than 2x the recorded Go Pine baseline for the same golden script.
- 100k candles single run: must complete without worker restart and without unbounded RSS growth.
- Payload overhead: request/response size must stay below configured gRPC max message size; oversized jobs must fail before worker dispatch.
- Parameter optimization: worker pool throughput must improve with workers until CPU saturation; regressions above 20% require investigation.
- Live bar close: p95 worker round-trip should stay below 250 ms for the configured live warmup window.

Each benchmark update must record:

- script name and hash
- candle count and timeframe
- worker count
- duration and candles/sec
- request/response bytes
- worker RSS peak
- baseline commit or fixture version

## Removal Policy

Hard-cut means:

- remove the Go Pine runtime execution path instead of keeping a long-term fallback
- remove Go TradingView parity expansion code and generated parity docs
- keep only migration shims needed to read existing definitions and rewrite runtime to `pine-pinets`
- fail fast for unsupported old runtimes after migration normalization
- do not add new Go Pine language semantics while this migration is active

## Next Engineering Slices

1. Final hard-cut audit: keep `pine-go-plan` only in migration shims and historical docs; reject new current-code or current-doc occurrences.
2. Acceptance verification: rerun focused Go, worker, frontend, coverage, performance, file-size, and `git diff --check` gates from a clean worktree.
3. Packaging decision: install/lock the commercial `pinets` package, disable mock mode, build worker assets, and pass a non-mock process smoke before release.
4. Release cleanup: after the license/package decision is complete, update final release notes against the operator checklist in [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md).

## Verification Log

| Date | Check | Result |
| --- | --- | --- |
| 2026-06-29 | `go test ./pkg/strategy/pineworker` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.2% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.97 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l docs/pinets-hardcut-migration.md pkg/strategy/pineworker/*.go` | Largest new file 208 lines, below 1200 |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy\|TestStrategyRuntimeFromParamsMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyAppliesDefaults\|TestStrategyDefinitionEndpoints'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~10.17 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineWorkerProtoCompilesAndExposesContract -count=1` | Pass; `proto/pineworker.proto` compiles and exposes health/analyze/run methods |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.2% statement coverage after proto contract tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.02 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `bun test workers/pineworker/src` | Pass, 9 tests cover request validation, adapter normalization, error mapping, mock execution, and PineTS constructor integration |
| 2026-06-29 | `npx tsc --noEmit -p workers/pineworker/tsconfig.json` | Pass |
| 2026-06-29 | `wc -l workers/pineworker/package.json workers/pineworker/tsconfig.json workers/pineworker/src/*.ts` | Largest worker file 192 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.5% statement coverage after Go client tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~10.71 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy\|TestStrategyRuntimeFromParamsMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyAppliesDefaults\|TestStrategyDefinitionEndpoints'` | Pass |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | Temp `protoc --go_out --go-grpc_out pkg/strategy/pineworker/proto/pineworker.proto` before split | Blocked for commit: generated `pineworker.pb.go` was 1267 lines, above the 1200-line file guardrail |
| 2026-06-29 | Split proto temp codegen with `pineworker.proto`, `pineworker_types.proto`, and `pineworker_common.proto` | Pass for guardrail; generated files were 78, 197, 639, and 699 lines |
| 2026-06-29 | `scripts/gen-pineworker-proto.sh` | Pass; generated Go protobuf/gRPC files and enforced 1200-line limit |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 87.6% statement coverage after gRPC transport and mapping tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.15 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./pkg/strategy/pineworker/pineworkerpb` | Pass |
| 2026-06-29 | `npm run test:pineworker` | Pass, 14 tests cover worker validation, adapter normalization, PineTS constructor integration, proto mapping, and Bun gRPC server boundary |
| 2026-06-29 | `npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l workers/pineworker/package.json workers/pineworker/tsconfig.json workers/pineworker/src/*.ts` | Largest worker file 192 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover && go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, 87.6% statement coverage and ~6.29 ns/op; run after codegen because `scripts/gen-pineworker-proto.sh` recreates `pineworkerpb` |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy\|TestStrategyRuntimeFromParamsMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyAppliesDefaults\|TestStrategyDefinitionEndpoints'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 88.5% statement coverage after WorkerManager lifecycle tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.98 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `scripts/gen-pineworker-proto.sh` | Pass; run before Go tests when generated files may be absent or stale |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage after binary launcher and gRPC dialer tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.07 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy\|TestStrategyRuntimeFromParamsMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyAppliesDefaults\|TestStrategyDefinitionEndpoints'` | Pass |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestCommandsFromOrderIntents\|TestCommandFromOrderIntentDefaultsEntryQuantity\|TestCommandFromOrderIntentRejectsUnsupportedIntent\|TestPineWorkerBacktestAdapter'` | Pass; worker order intent to backtest command adapter covered |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~11.92 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass; replay planner request construction, params propagation, command grouping, range validation, and worker error propagation covered |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.06 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/backtest/pineworker_replay.go pkg/backtest/pineworker_replay_test.go pkg/backtest/pineworker_adapter.go pkg/backtest/pineworker_adapter_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 207 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass; command executor submit/cancel/cancel-all, unsupported sizing, error propagation, replay planner, and adapter covered |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.01 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/backtest/pineworker_command_executor.go pkg/backtest/pineworker_command_executor_test.go pkg/backtest/pineworker_replay.go pkg/backtest/pineworker_replay_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 214 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass; replay pump ordering, shape validation, missing/extra bars, command errors, command executor, replay planner, and adapter covered |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.98 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/backtest/pineworker_replay_pump.go pkg/backtest/pineworker_replay_pump_test.go pkg/backtest/pineworker_command_executor.go pkg/backtest/pineworker_command_executor_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 220 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass; `RunWithPineWorker` smoke validates worker intents through Go matching and result collection |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~8.04 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/backtest/pineworker_runner.go pkg/backtest/pineworker_runner_test.go pkg/backtest/pineworker_replay_source.go pkg/backtest/pineworker_replay_source_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 262 lines, below 1200 |
| 2026-06-29 | `go test ./internal/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView'` | Pass; service default now fails fast without a Pine worker runner and uses `RunWithPineWorker` when configured |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~8.05 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l internal/backtest/service.go internal/backtest/service_result_view.go internal/backtest/service_pineworker_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 1146 lines, below 1200 |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker\|TestBacktestRouteAcceptsExplicitMarketAndCode\|TestEnqueueBacktestUsesPineInitialCapitalWhenRequestOmitsBalance'` | Pass; API startup wires configured Pine worker managers, stops them on close, and does not fall back to Go runtime when no worker is configured |
| 2026-06-29 | `go test ./internal/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView'` | Pass |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.75 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l internal/app/apiserver/servercore/server.go internal/app/apiserver/servercore/pineworker_runtime.go internal/app/apiserver/servercore/pineworker_runtime_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 702 lines, below 1200 |
| 2026-06-29 | `go test ./internal/pineworkerassets ./internal/app/apiserver/servercore -run 'TestBinaryName\|TestSelectForPlatform\|TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker'` | Pass; embedded asset selection and API startup fallback order covered |
| 2026-06-29 | `go test -tags release_assets ./internal/pineworkerassets -run Test` | Pass; release asset package compiles with staged asset directory |
| 2026-06-29 | `bash -n scripts/build-pineworker-assets.sh build-release.sh start.sh` | Pass |
| 2026-06-29 | `go test ./internal/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView'` | Pass |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.16 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l internal/pineworkerassets/*.go internal/app/apiserver/servercore/pineworker_runtime.go internal/app/apiserver/servercore/pineworker_runtime_test.go scripts/build-pineworker-assets.sh docs/pinets-hardcut-migration.md` | Pass; largest touched file 393 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run 'TestWorkerManagerProcessSmokeWithBunWorker\|TestWorkerManagerStartStopAndSnapshot\|TestGRPCTransport\|TestGRPCDialer\|TestBinaryWorkerLauncher\|TestClientRunScript'` | Pass; gated process smoke is present and skipped by default |
| 2026-06-29 | `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` | Skipped in earlier environment; `@grpc/grpc-js` and `@grpc/proto-loader` were not installed |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.85 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./internal/pineworkerassets ./internal/app/apiserver/servercore -run 'TestBinaryName\|TestSelectForPlatform\|TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker'` | Pass |
| 2026-06-29 | `go test ./internal/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView'` | Pass |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/strategy/pineworker/process_smoke_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 260 lines, below 1200 |
| 2026-06-29 | `npm install` | Pass; workspace now includes `workers/pineworker`, installed static JS gRPC runtime deps. npm reported 7 existing audit findings and local ignored `package-lock.json` was regenerated but not committed by policy |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass after static gRPC imports and proto include-dir fix |
| 2026-06-29 | `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` | Pass; Bun compiled mock worker process served real gRPC through `WorkerManager` |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.98 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./internal/pineworkerassets ./internal/app/apiserver/servercore -run 'TestBinaryName\|TestSelectForPlatform\|TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker'` | Pass |
| 2026-06-29 | `go test ./internal/backtest ./pkg/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView\|TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `wc -l package.json workers/pineworker/package.json workers/pineworker/src/main.ts workers/pineworker/src/grpcServer.ts workers/pineworker/src/grpcServer.test.ts pkg/strategy/pineworker/process_smoke_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 276 lines, below 1200 |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestStrategyRuntime\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker'` | Pass; live runtime now builds Pine worker `live` requests, filters current-bar intents, preserves notify/live risk/order paths, and records worker errors |
| 2026-06-29 | `go test ./internal/backtest ./pkg/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView\|TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.864 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l internal/app/apiserver/servercore/strategy_runtime_manager.go internal/app/apiserver/servercore/strategy_runtime_pineworker_live.go internal/app/apiserver/servercore/strategy_runtime_broker_bridge.go internal/app/apiserver/servercore/strategy_runtime_manager_test.go internal/app/apiserver/servercore/strategy_runtime_manager_test_helpers_test.go internal/app/apiserver/servercore/strategy_runtime_manager_trading_test.go internal/app/apiserver/servercore/server.go internal/app/apiserver/servercore/test_helpers_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 1196 lines, below 1200 |
| 2026-06-29 | `npm run generate:reference` | Pass; generated Pine support snapshot now reports runtime `pine-pinets` |
| 2026-06-29 | `go test ./pkg/strategy/pinespec -run Test` | Pass; pinespec no longer imports Go Pine runtime for public runtime ID |
| 2026-06-29 | `rg -n "pineruntime|pine-go-plan" pkg/strategy/pinespec docs/reference/generated/pine-v6-support.md docs/frontend/strategy-authoring.md` | Pass; no Go runtime import or legacy runtime ID remains in current public spec docs |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run 'TestPineSpecTool\|TestAnalyze\|TestStrategyDefinition\|TestNormalizeStrategyRuntimeUsesPineTSAndMigratesLegacy\|TestStrategyRuntimeFromParamsMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyMigratesLegacyRuntime\|TestStrategyCatalogNormalizeStrategyAppliesDefaults'` | Pass; API/ADK surfaces consume `pine-pinets` spec runtime |
| 2026-06-29 | `go test ./pkg/adk -run 'TestBuiltin\|TestSkills\|TestStore'` | Pass; built-in strategy skill resources remain valid after pinespec split/runtime update |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.968 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l pkg/strategy/pinespec/spec.go pkg/strategy/pinespec/golden_examples.go pkg/strategy/pinespec/spec_test.go docs/pinets-hardcut-migration.md docs/frontend/strategy-authoring.md docs/reference/generated/pine-v6-support.md` | Pass; split `spec.go` from 1313 to 984 lines, all touched files below 1200 |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit locks public spec docs away from `pine-go-plan`/`pineruntime` and allowed only the temporary direct backtest runner import |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage after hard-cut audit |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.564 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l pkg/strategy/pineworker/hardcut_audit_test.go pkg/strategy/pineworker/types.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 293 lines, below 1200 |
| 2026-06-29 | `npm --prefix apps/web test -- App.strategy.test.ts adkToolVisualizations.test.ts` | Pass, 13 tests; strategy definition save/ADK visualization fixtures use `pine-pinets` |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Blocked by pre-existing `src/composables/useADKPageChatState.ts(1237,5): Type 'number' is not assignable to type 'Timeout'` |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.873 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l apps/web/src/components/StrategyDesignStage.vue apps/web/src/components/strategy-runtime/strategyDefinitionPayload.ts apps/web/src/components/strategy-runtime/strategyRuntimeIdentity.ts apps/web/tests/App.strategy.test.ts apps/web/tests/adkToolVisualizations.test.ts docs/pinets-hardcut-migration.md` | Pass; largest touched file 1200 lines, at guardrail |
| 2026-06-29 | `npm --prefix apps/web test -- App.strategy.test.ts adkToolVisualizations.test.ts App.strategy.runtime.test.ts` | Pass, 14 tests; runtime panel and strategy save coverage remain green after PineTS identity/helper split |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Blocked only by pre-existing `src/composables/useADKPageChatState.ts(1237,5): Type 'number' is not assignable to type 'Timeout'` |
| 2026-06-29 | `rg -n "pine-go-plan" apps/web/src -g '*.ts' -g '*.vue'` | Pass; only `strategyRuntimeIdentity.ts` keeps the legacy migration alias |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~10.00 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | `wc -l apps/web/src/components/StrategyRuntimePanel.vue apps/web/src/components/strategy-runtime/useStrategyRuntimeInstanceEditor.ts apps/web/src/components/strategy-runtime/strategyRuntimePanel.css docs/pinets-hardcut-migration.md` | Pass; largest touched file 1064 lines, below 1200 |
| 2026-06-29 | `npm --prefix apps/web test -- App.strategy.test.ts App.strategy.runtime.test.ts adkToolVisualizations.test.ts` | Pass, 14 tests; strategy page mock API and fixture defaults now emit `pine-pinets` |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Blocked only by pre-existing `src/composables/useADKPageChatState.ts(1237,5): Type 'number' is not assignable to type 'Timeout'` |
| 2026-06-29 | `rg -n "pine-go-plan" apps/web/tests apps/web/src -g '*.ts' -g '*.vue'` | Pass; only `strategyRuntimeIdentity.ts` keeps the legacy migration alias |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~8.487 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | `wc -l apps/web/tests/strategyPageTestUtils.ts apps/web/tests/strategyPageMockApi.ts apps/web/tests/strategyPageTestState.ts apps/web/tests/strategyPageScriptFixtures.ts apps/web/tests/strategyPageAnalyzeMock.ts docs/pinets-hardcut-migration.md` | Pass; largest touched file 963 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now also rejects `pine-go-plan` in frontend source/tests except the migration alias helper |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage after frontend legacy-runtime audit |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.948 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l pkg/strategy/pineworker/hardcut_audit_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 317 lines, below 1200 |
| 2026-06-29 | `go test ./pkg/backtest` | Pass; direct `Run` is disabled and Pine worker backtest path remains covered |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; audit no longer allows `pkg/backtest/runner.go` to import `pkg/strategy/pineruntime` |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~8.931 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l pkg/backtest/runner.go pkg/backtest/runner_hardcut_test.go pkg/backtest/pine_costs_test.go pkg/backtest/test_helpers_test.go pkg/strategy/pineworker/hardcut_audit_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 321 lines, below 1200 |
| 2026-06-29 | `rg -n "pkg/strategy/pineruntime\|pine-go-plan\|pkg/backtest\\.Run($\|[^W[:alnum:]_])" docs/architecture.md docs/troubleshooting/backtest-performance.md docs/pine-completion-roadmap.md docs/frontend/strategy-authoring.md` | Pass; current maintenance docs no longer expose legacy Go Pine runtime guidance |
| 2026-06-29 | `go test ./pkg/backtest` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; audit now checks current maintenance docs in addition to public spec and frontend surfaces |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.415 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `git rm -r pkg/strategy/pineruntime` | Pass; deleted the former Go Pine runtime package and its package-local tests/benchmarks |
| 2026-06-29 | `rg -n "pkg/strategy/pineruntime\|pineruntime" --glob '*.go' --glob '*.md' --glob '*.ts' --glob '*.vue'` | Pass; only hard-cut audit deny-list strings and historical migration log entries remain |
| 2026-06-29 | `go test ./pkg/strategy/...` | Pass; strategy packages compile and test without the former Go Pine runtime package |
| 2026-06-29 | `go test ./pkg/backtest ./pkg/strategy/pineworker -run Test` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; audit now also requires the former Go Pine runtime package directory to be absent |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.504 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; audit now restricts `pine-go-plan` to migration shims and historical docs while ignoring untracked `var/` runtime cache |
| 2026-06-29 | `go test ./internal/strategy -run TestDefinitionViewJSONRemainsFlat -v` | Pass; flat JSON fixture now uses `pine-pinets` |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.656 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l internal/strategy/types_test.go pkg/strategy/pineworker/hardcut_audit_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 335 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | `npm run test:pineworker` | Pass, 14 Bun worker tests |
| 2026-06-29 | `npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `go test ./internal/pineworkerassets ./internal/app/apiserver/servercore -run 'TestBinaryName\|TestSelectForPlatform\|TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker\|TestStrategyRuntime'` | Pass |
| 2026-06-29 | `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` | Pass; Bun compiled mock worker served real gRPC through `WorkerManager` |
| 2026-06-29 | `go test ./internal/backtest ./pkg/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView\|TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent\|TestRunDirectGoPineBacktestRemoved'` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/...` | Pass |
| 2026-06-29 | `npm --prefix apps/web test -- App.strategy.test.ts App.strategy.runtime.test.ts adkToolVisualizations.test.ts` | Pass, 14 tests |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover && go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, 86.1% statement coverage and ~8.149 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Pass after adding a browser timer compatibility declaration for DOM/Node timer overloads |
| 2026-06-29 | `wc -l docs/pinets-hardcut-migration.md pkg/strategy/pineworker/hardcut_audit_test.go internal/strategy/types_test.go apps/web/src/types/browser-timers.d.ts` | Pass; largest touched file 341 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | `npm ls pinets --workspaces --depth=1` | Empty; release packaging remains blocked until the commercial `pinets` package/license is available |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run TestResolvePineWorkerRuntimeConfig -v` | Pass; production worker config defaults to non-mock mode and mock requires explicit opt-in |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~8.937 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass, 14 Bun worker tests and TypeScript check |
| 2026-06-29 | `wc -l docs/pinets-hardcut-migration.md internal/app/apiserver/servercore/pineworker_runtime_test.go pkg/strategy/pineworker/hardcut_audit_test.go apps/web/src/types/browser-timers.d.ts` | Pass; largest touched file 409 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | Added [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md) | Pass; release/operator checklist now documents env vars, embedded asset flow, mock restriction, and non-mock smoke requirement |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now covers the PineTS worker release checklist |
| 2026-06-29 | `go test ./internal/app/apiserver/servercore -run TestResolvePineWorkerRuntimeConfig -v` | Pass; runtime config still defaults to real PineTS worker mode |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~7.510 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass, 14 Bun worker tests and TypeScript check |
| 2026-06-29 | `wc -l docs/troubleshooting/pinets-worker-release.md docs/README.md docs/troubleshooting.md docs/pinets-hardcut-migration.md pkg/strategy/pineworker/hardcut_audit_test.go` | Pass; largest touched file 367 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | `npm ls pinets --workspaces --depth=1` | Empty; release remains blocked until the commercial `pinets` package/license is installed and locked |
| 2026-06-29 | Added `scripts/check-pinets-release.sh` and `npm run check:pinets-release` | Pass; strict mode fails while `pinets` is missing, `--allow-blocked` runs current Go/worker gates and skips release asset build |
| 2026-06-29 | `bash scripts/check-pinets-release.sh --allow-blocked` | Pass in blocked mode; confirms missing `pinets`, runs runtime-config test, hard-cut audit, Pine worker coverage/performance gates, Bun worker tests, and worker typecheck |
| 2026-06-29 | `bash scripts/check-pinets-release.test.sh` | Pass; release script strict, blocked, and unblocked branches are covered with command stubs |
| 2026-06-29 | Updated `.github/workflows/backtest-performance-gate.yml` | Pass; removed deleted Go Pine runtime/golden benchmark references and added the PineTS worker performance gate |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now rejects stale Go Pine performance workflow references |
| 2026-06-29 | `wc -l .github/workflows/backtest-performance-gate.yml scripts/check-pinets-release.sh scripts/check-pinets-release.test.sh package.json docs/troubleshooting/pinets-worker-release.md docs/pinets-hardcut-migration.md pkg/strategy/pineworker/hardcut_audit_test.go` | Pass; largest touched file 377 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | Added `TestWorkerManagerRealPineTSProcessSmoke` | Pass; gated by `JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE=1`, requires installed `pinets`, starts a non-mock Bun/PineTS worker process, and is wired into strict `scripts/check-pinets-release.sh` before release asset build |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run 'TestWorkerManagerRealPineTSProcessSmoke\|TestWorkerManagerProcessSmokeWithBunWorker' -v` | Pass with both process smoke tests skipped by default env gates |
| 2026-06-29 | `bash scripts/check-pinets-release.sh --allow-blocked` | Pass in blocked mode; missing `pinets` skips the real PineTS process smoke and release asset build |
| 2026-06-29 | Added `scripts/build-frontend-assets.sh` and wired it into `scripts/check-pinets-release.sh` | Pass; local embedded frontend assets are rebuilt from current web output and no longer contain removed Go Pine runtime package or stale benchmark references |
| 2026-06-29 | `go test -tags release_assets ./internal/frontendassets -run TestFileSystem -v` | Pass; release frontend asset tests now reject removed Go Pine runtime and stale performance benchmark strings |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Pass after rebuilding frontend assets from current source |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now requires release frontend asset auditing |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.440 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l scripts/archive_frontend_assets.go scripts/build-frontend-assets.sh package.json internal/frontendassets/release_test.go pkg/strategy/pineworker/hardcut_audit_test.go` | Pass; largest touched file 276 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | Replaced frontend legacy runtime label with `PineTS migration alias` | Pass; user-visible runtime labels no longer present `pine-go-plan` as a supported Go runtime |
| 2026-06-29 | `npm --prefix apps/web test -- strategyRuntimeIdentity.test.ts App.strategy.runtime.test.ts App.strategy.test.ts adkToolVisualizations.test.ts` | Pass, 16 tests |
| 2026-06-29 | `npm --prefix apps/web run typecheck` | Pass |
| 2026-06-29 | `rg -n "Legacy Go Pine\|pine-go-plan" apps/web/src apps/web/tests -g '*.ts' -g '*.vue'` | Pass; only `strategyRuntimeIdentity.ts` keeps the legacy migration alias constant |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~6.586 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | Updated `.github/workflows/ci.yml` | Pass; CI now installs Bun and runs `npm run test:pineworker` plus `npm run typecheck:pineworker` |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass, 14 Bun worker tests and TypeScript check |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now requires CI to exercise PineTS worker gates |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.864 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `wc -l .github/workflows/ci.yml pkg/strategy/pineworker/hardcut_audit_test.go` | Pass; largest touched file 296 lines, below 1200 |
| 2026-06-29 | `git diff --check` | Pass |
| 2026-06-29 | Updated `.github/workflows/ci.yml` | Pass; CI now builds embedded frontend assets with `npm run build:frontend-assets` and runs `go test -tags release_assets ./internal/frontendassets -run TestFileSystem` |
| 2026-06-29 | Updated `.github/workflows/ci.yml` | Pass; CI now runs `npm run test:pinets-release-check` so strict, blocked, and unblocked release-check branches stay covered |
| 2026-06-29 | `npm view pinets version license dist-tags --json` | Blocked for release; public `pinets@0.9.26` reports `AGPL-3.0-only`, so commercial license attestation is required before release |
| 2026-06-29 | Added shared `scripts/lib/pinets-license.sh` gate | Pass; release-check and worker asset build scripts now block missing package/license and public AGPL packages before release asset generation |
| 2026-06-29 | Split `internal/pineworkerassets` dev/release tests | Pass; dev builds still verify missing assets are unavailable while `release_assets` builds verify staged worker binaries return data and SHA256 |
| 2026-06-29 | Updated `scripts/check-pinets-release.sh` | Pass; strict unblocked release acceptance now builds `go build -tags release_assets -o dist/trading-engine ./cmd/jftrade-api` after worker asset generation and release asset tests |
| 2026-06-29 | Aligned release output name | Pass; `scripts/check-pinets-release.sh` now defaults to the single-file `dist/trading-engine` release artifact and supports `JFTRADE_PINETS_RELEASE_OUT` for test output isolation |
| 2026-06-29 | Updated `scripts/check-pinets-release.sh` | Pass; release acceptance now runs web tests, web typecheck, and `git diff --check` in addition to worker, coverage, performance, and release asset gates |
| 2026-06-29 | Documented Bun SEA packaging direction | Pass; plan and release checklist now require Bun `build --compile` single-file workers embedded into the Go `release_assets` `trading-engine` binary |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now requires Bun SEA packaging docs and `bun build --compile` release asset construction |
| 2026-06-29 | `wc -l docs/pinets-hardcut-migration.md docs/troubleshooting/pinets-worker-release.md pkg/strategy/pineworker/hardcut_audit_test.go && git diff --check` | Pass; largest touched file 414 lines before this log entry, below 1200, and no whitespace errors |
| 2026-06-29 | Updated `scripts/check-pinets-release.sh` | Pass; strict release acceptance now verifies `dist/trading-engine` exists, is non-empty, and is executable after `go build -tags release_assets` |
| 2026-06-29 | `bash scripts/check-pinets-release.test.sh` | Pass; stub coverage now includes successful artifact creation and failure when the release artifact is missing |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run TestPineTSHardCutDoesNotExposeGoPineRuntime -v` | Pass; hard-cut audit now requires the release artifact sanity gate and operator checklist wording |
