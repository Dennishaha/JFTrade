# PineTS Hard-Cut Migration Plan

> Goal: replace the Go Pine runtime with PineTS workers, remove the already-built Go TradingView parity path, and keep Go as the trading, risk, order, and backtest authority.

## Current Decision

- Runtime target: `sourceFormat=pine-v6` + `runtime=pine-pinets`.
- Legacy runtime: `pine-go-plan` is migration-only and must not remain a selectable execution path.
- License assumption: PineTS commercial license is available before the worker is shipped in release binaries.
- Execution authority: PineTS computes Pine outputs and order intents; Go remains authoritative for backtest matching, equity curves, live risk, and order placement.
- Release shape: one Go binary embeds platform-specific Bun/PineTS worker binaries and starts them as localhost gRPC child processes.
- File-size guardrail: new or materially rewritten files must stay under 1200 lines.

## Progress Tracker

| Phase | Status | Evidence / Exit Criteria |
| --- | --- | --- |
| 0. Plan and guardrails | Done | This document exists; coverage and performance gates are documented; focused verification is recorded below. |
| 1. Pine worker contract | Done | `pkg/strategy/pineworker` owns `pine-pinets` constants, request/response shapes, order intent schema, worker defaults, validation, and perf gate helpers. |
| 1.1 Runtime ID normalization | Done | Server-side definition/catalog normalization emits `pine-pinets` and migrates old `pine-go-plan`; focused servercore tests pass. |
| 2. Proto contract | Done | `pkg/strategy/pineworker/proto/pineworker.proto` mirrors the Go contract and compiles through `protoc`. |
| 3. Worker PoC | Done | Bun worker core validates requests, adapts custom OHLCV data to the PineTS constructor shape, normalizes plots/logs/order intents, exposes a gRPC server boundary, and has Bun tests. Real PineTS dependency wiring remains blocked on commercial license and package-lock policy. |
| 4. gRPC bridge | In progress | Go worker client abstraction, generated Go gRPC transport, and Bun gRPC server boundary are covered by fake/bufconn/Bun tests. Real JS gRPC dependencies, process-level end-to-end tests, and worker packaging remain. |
| 5. Worker manager | In progress | Go `WorkerManager` starts fixed worker specs, assigns ports, dials transports, round-robins healthy workers, restarts failed health checks, drains on shutdown, and exposes snapshots. Binary extraction launcher, gRPC dialer, API-server lifecycle wiring, embedded asset selection, and gated process smoke coverage are implemented. Real JS gRPC dependency lock policy remains before always-on process smoke. |
| 6. Backtest integration | In progress | `pkg/backtest` has a Pine worker adapter, replay planner, command executor, replay pump, and `RunWithPineWorker`; `internal/backtest.Service` defaults to the Pine worker path and API startup injects a configured `WorkerManager` from `JFTRADE_PINEWORKER_BINARY`. Missing worker config now fails fast instead of falling back to Go runtime. |
| 7. Live integration | Not started | Bar-close live flow calls worker, applies Go risk, places orders through broker APIs, and records runtime observation. |
| 8. Hard removal | Not started | `pkg/strategy/pineruntime`, Go TradingView parity extensions, self-built support matrix docs, and old UI toggles are removed. |
| 9. Packaging | In progress | `scripts/build-pineworker-assets.sh` builds platform Bun worker binaries into `internal/pineworkerassets/assets/bin`; Go selects the matching embedded asset under `release_assets` and falls back to external env config in development. A gated process smoke test can compile and run the mock Bun worker through real gRPC when JS gRPC deps are installed. Real PineTS dependency lock policy remains. |
| 10. Acceptance | Not started | Focused Go/web/worker tests pass; performance gates pass on golden scripts; docs reflect `pine-pinets` as the only Pine runtime. |

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

## Worker PoC Boundary

The first Bun worker slice lives under `workers/pineworker` and intentionally avoids adding `pinets` to root lockfiles until the commercial license and package-management policy are finalized.

- `NativePineTSExecutor` dynamically imports `pinets` and constructs `new PineTS(candles)` for custom OHLCV execution.
- `runScriptWithPineTS` validates requests before dispatch and maps both validation/runtime failures into worker error responses.
- Adapter normalization currently covers plots, outputs, logs, warnings, diagnostics, metadata, and normalized order intents.
- `startWorkerGrpcServer` dynamically consumes `@grpc/grpc-js` and `@grpc/proto-loader`, registers health/analyze/run handlers, and enforces gRPC send/receive message limits.
- `DeterministicPineTSExecutor` exists only for fast contract tests; it must not become a production fallback.

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
- Current manager tests cover fixed port allocation, round-robin dispatch, health-check restart, failed restart reporting, startup cleanup, shutdown cleanup, snapshot state, binary checksum, process cleanup, dialer creation, and a gated Bun process smoke path.
- Real PineTS dependency lock policy and always-on process smoke remain in packaging/manager follow-up slices.

## Backtest Integration Boundary

`pkg/backtest.PineWorkerBacktestAdapter`, `PineWorkerReplayPlanner`, `PineWorkerCommandExecutor`, `PineWorkerReplayPump`, and `RunWithPineWorker` are the backtest-facing contracts for worker execution.

- It forces `RunScriptRequest.Mode` to `backtest` and maps worker/transport errors into backtest errors.
- It converts worker `OrderIntent` values into `WorkerOrderCommand` records with Go-side side, order type, quantity, limit/stop, comments, alerts, bar index, and time.
- The replay planner converts `types.KLine` to worker candles, builds `RunScriptRequest`, copies params, applies default job IDs, validates returned command bar indexes, fills missing command times from the source candle, and groups commands by bar index/open time.
- The command executor resolves session markets, submits market/limit/stop commands through bbgo `SubmitOrders`, tracks created orders by Pine id/client id, and maps `cancel`/`cancel_all` to bbgo `CancelOrders`.
- The replay pump validates replay candle order, feeds each K-line into bbgo matching, then executes that bar's close-generated worker commands so they are eligible for later-bar matching.
- `RunWithPineWorker` loads the same K-line store and bbgo backtest exchange, collects replay K-lines for worker planning, routes worker intents through Go matching, and uses the existing result collector for trades/equity/metrics without instantiating `pkg/strategy/pineruntime`.
- `internal/backtest.Service` accepts a `WithPineWorkerRunner` dependency and its default runner now requires that Pine worker dependency instead of calling `bt.Run`.
- API server startup no longer injects `bt.Run`; it injects a started Pine worker manager only when `JFTRADE_PINEWORKER_BINARY` is configured and otherwise leaves service-level fail-fast behavior in place.
- Quantity-percent commands currently fail fast until Go-side position sizing is wired, because Go remains authoritative for account/position state.
- Current tests cover entry, exit, cancel-all, default entry quantity, unsupported intents, transport errors, worker errors, replay request construction, replay K-line collection, params propagation, command grouping, invalid bar indexes, worker timeout propagation, market/limit order submission, cancel/cancel-all, unsupported sizing, submit/cancel error propagation, replay shape validation, missing/extra bars, consume-before-command ordering, an end-to-end `RunWithPineWorker` smoke through Go matching, service-level fail-fast when no Pine worker runner is configured, and API startup wiring for configured/absent worker managers.
- Direct non-service calls to `pkg/backtest.Run` and `pkg/strategy/pineruntime` remain until CLI/live callers are migrated and hard removal lands.

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

1. Finish `pkg/strategy/pineworker` contract and tests.
2. Add worker proto mirroring the Go contract.
3. Finish Bun gRPC worker server around the worker core.
4. Add process-level worker smoke tests with real JS gRPC dependencies or a locked dependency policy.
5. Add embedded platform worker asset selection and process-level smoke tests.
6. Route backtest replay through `PineWorkerBacktestAdapter` and Go matching.
7. Update live runtime manager after backtest correctness and performance gates are stable.
8. Delete Go Pine runtime/parity surfaces and update docs/UI.

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
| 2026-06-29 | `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` | Skipped in current environment; `@grpc/grpc-js` and `@grpc/proto-loader` are not installed |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -run Test -cover` | Pass, 86.1% statement coverage |
| 2026-06-29 | `go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem` | Pass, ~5.85 ns/op, 0 B/op, 0 allocs/op |
| 2026-06-29 | `go test ./internal/pineworkerassets ./internal/app/apiserver/servercore -run 'TestBinaryName\|TestSelectForPlatform\|TestResolvePineWorkerRuntimeConfig\|TestServerStartsConfiguredPineWorkerManagerAndStopsOnClose\|TestServerStartsEmbeddedPineWorkerManager\|TestServerBacktestDoesNotFallbackToGoRuntimeWithoutPineWorker'` | Pass |
| 2026-06-29 | `go test ./internal/backtest -run 'TestServiceDefaultBacktest\|TestStartQueuesRunAndExecutesWithInjectedRunner\|TestStartScriptQueuesResearchRunWithoutStrategyProvider\|TestResultView'` | Pass |
| 2026-06-29 | `go test ./pkg/backtest -run 'TestRunWithPineWorker\|TestCollectPineWorkerReplayKLines\|TestPineWorkerReplayPump\|TestPineWorkerCommandExecutor\|TestPineWorkerReplay\|TestBuildPineWorkerBacktestRequest\|TestPineWorkerBacktestAdapter\|TestCommandsFromOrderIntents\|TestCommandFromOrderIntent'` | Pass |
| 2026-06-29 | `npm run test:pineworker && npm run typecheck:pineworker` | Pass |
| 2026-06-29 | `wc -l pkg/strategy/pineworker/process_smoke_test.go docs/pinets-hardcut-migration.md` | Pass; largest touched file 260 lines, below 1200 |
