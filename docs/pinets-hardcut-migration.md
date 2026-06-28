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
| 2. Worker PoC | Not started | Bun worker can run PineTS on custom OHLCV data and return plots, diagnostics, logs, warnings, and order intents. |
| 3. gRPC bridge | In progress | Proto contract exists and compiles through `protoc`; Go client, worker server, health check deadlines, max-message policy, and error mapping remain. |
| 4. Worker manager | Not started | Go starts N embedded workers, assigns ports, checks health, restarts crashes, drains on shutdown, and exposes status. |
| 5. Backtest integration | Not started | Backtest calls PineTS worker for intents, then Go produces trades, order book, equity curve, drawdown, and metrics. |
| 6. Live integration | Not started | Bar-close live flow calls worker, applies Go risk, places orders through broker APIs, and records runtime observation. |
| 7. Hard removal | Not started | `pkg/strategy/pineruntime`, Go TradingView parity extensions, self-built support matrix docs, and old UI toggles are removed. |
| 8. Packaging | Not started | `bun build --compile` creates platform workers; Go embeds and releases matching binaries with checksum validation. |
| 9. Acceptance | Not started | Focused Go/web/worker tests pass; performance gates pass on golden scripts; docs reflect `pine-pinets` as the only Pine runtime. |

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

## Contract Shape

The Go contract layer starts in `pkg/strategy/pineworker` and later maps 1:1 to protobuf.

- `RuntimeID`: `pine-pinets`.
- `LegacyRuntimeID`: `pine-go-plan`, accepted only for migration normalization.
- `RunScriptRequest`: job/script identity, source, symbol, timeframe, candles, params, and mode.
- `RunScriptResponse`: outputs, order intents, plots, logs, warnings, diagnostics, worker metadata, and optional error.
- `OrderIntent`: normalized representation of Pine `entry/order/exit/close/close_all/cancel/cancel_all`.
- `PerformanceGate`: max duration, max per-candle duration, max RSS, max payload, and min candles/sec thresholds.

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
3. Build a Bun worker PoC with custom candle input and PineTS output extraction.
4. Add Go client and fake worker tests before touching production runtime.
5. Route backtest through the worker behind the new `pine-pinets` runtime.
6. Update live runtime manager after backtest correctness and performance gates are stable.
7. Delete Go Pine runtime/parity surfaces and update docs/UI.

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
