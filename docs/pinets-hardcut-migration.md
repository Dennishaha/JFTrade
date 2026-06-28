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
| 5. Worker manager | In progress | Go `WorkerManager` starts fixed worker specs, assigns ports, dials transports, round-robins healthy workers, restarts failed health checks, drains on shutdown, and exposes snapshots. Binary extraction launcher and gRPC dialer are implemented; real embedded worker assets and process-level smoke tests remain. |
| 6. Backtest integration | Not started | Backtest calls PineTS worker for intents, then Go produces trades, order book, equity curve, drawdown, and metrics. |
| 7. Live integration | Not started | Bar-close live flow calls worker, applies Go risk, places orders through broker APIs, and records runtime observation. |
| 8. Hard removal | Not started | `pkg/strategy/pineruntime`, Go TradingView parity extensions, self-built support matrix docs, and old UI toggles are removed. |
| 9. Packaging | Not started | `bun build --compile` creates platform workers; Go embeds and releases matching binaries with checksum validation. |
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
- Current manager tests cover fixed port allocation, round-robin dispatch, health-check restart, failed restart reporting, startup cleanup, shutdown cleanup, snapshot state, binary checksum, process cleanup, and dialer creation.
- Real embedded worker assets, platform selection, dependency lock policy, and process-level smoke tests remain in packaging/manager follow-up slices.

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
6. Route backtest through the worker behind the new `pine-pinets` runtime.
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
