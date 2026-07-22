# PineTS contract audit

Scope: current mainline PineTS execution contract. The completed hard-cut process is retained in Git history rather than duplicated here.

This audit tracks the hard cut from the former Go Pine runtime to the PineTS worker boundary. It separates trading contracts from visual Pine outputs so frontend authoring does not imply that every PineTS visual feature is also a tradeable JFTrade flow block.

## Contract matrix

| Surface | Status | Current contract |
| --- | --- | --- |
| `pkg/backtest.Run` | Intentional break | Direct Go Pine execution is disabled and returns an error. Production callers must use `RunWithPineWorker` with an explicit Pine worker runner. |
| Backtest service/API | Compatible through dependency injection | API startup injects a configured `pineworker.WorkerManager`; service-level execution fails fast when no worker runner is configured instead of falling back to Go Pine. |
| Strategy definition runtime | Migration compatible | `runtime=pine-pinets` is the current value. `pine-go-plan` is accepted only as a migration alias and normalized to `pine-pinets`. |
| Strategy source format | Compatible | `sourceFormat=pine-v6` remains the only supported source format. |
| Pine worker proto | Additive compatible | Existing fields remain unchanged. Order intents add `parent_id`、`atomic_group_id`、`oco_group_id`、`reduce_only`; live requests add session operation and expected revision, responses add session revision. |
| Backtest result model | Compatible | Go remains authoritative for fills, trades, equity, metrics, and result collection. PineTS worker supplies order intents, visual outputs, and upstream strategy metrics for inspection. |
| Live order path | Compatible with new authority split | PineTS worker produces current-bar order intents; Go still performs risk checks, notifications, broker reads, and order placement. |
| ADK/spec payload | Migration compatible | Public spec/runtime surfaces advertise `runtime=pine-pinets`; legacy runtime text is limited to explicit normalization surfaces. |

## Live incremental worker contract

Production live Pine opens one stateful Worker session per strategy instance and symbol. `open` performs the complete historical warmup once and returns revision 1 without emitting historical orders. Each later closed bar is sent through `append` with the caller's expected revision; PineTS appends the candle to the existing runtime, executes only the new global bar indices, and returns delta plots/events/order intents plus the next revision. `close` retires the pinned Worker and releases its runtime slot.

The session validates immutable source, symbol, timeframe and params, requires strictly increasing candle times, serializes appends, and invalidates itself after an execution failure. A stale revision cannot be replayed into the same state. Injected legacy runners that do not implement the session opener retain the full-history path for compatibility, but the bundled production Worker advertises `live-session-v1` and uses the stateful path.

This protocol removes the repeated full-history calculation from the long-running production path without truncating history or changing `var`、series history、order state and global `bar_index` semantics. It intentionally uses PineTS's append/iteration hooks pinned to the tested PineTS version; upgrading PineTS must rerun the real incremental-state regression before changing the bundled Worker.

## Atomic protective-order contract

A same-bar entry and its protective exit are emitted with one atomic group. The exit references the parent entry, is reduce-only, and a limit-plus-stop bracket carries one OCO group. Go expands a dual-price exit into separate limit and stop legs, preflights the entire bar before any order side effect, and submits the complete group only through `PineWorkerAtomicOrderExecutor`.

An execution backend implementing that interface promises all-or-none acceptance, child activation only after the parent fill, OCO sibling cancellation and reduce-only enforcement at match time. A backend that cannot make all four promises is not allowed to emulate the group with sequential `SubmitOrders`: the complete group is rejected before the entry is placed. The current Futu live adapter does not claim this atomic capability, so same-bar protective groups fail closed rather than opening an unprotected position. This protocol is narrower than general TradingView OCA/partial-fill parity, which remains outside the current broker-emulator score.

## PineTS capability alignment

| Capability | PineTS `0.9.28` | Worker contract | Frontend support |
| --- | --- | --- | --- |
| Numeric plots | Supported through `plots` | `plots` and `outputs` preserve numeric series | Monaco suggests `plot`; rendering remains a separate chart concern. |
| Alerts | `alert()` / `alertcondition()` events | `alerts` carries normalized alert events | Monaco suggests `alert` and `alertcondition`; flow notify blocks still generate `alert()`. |
| Shapes and chars | `plotshape`, `plotchar`, `plotarrow` | Captured as `visual_outputs` when PineTS returns visual payloads | Monaco suggests common shape/char calls; no trading flow block is created for them. |
| Drawing objects | `label`, `line`, `box`, `polyline`, `linefill` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests common constructors; visual model does not treat drawings as orders. |
| Tables | `table.*` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests `table.new`; flow blocks stay trading-focused. |
| Strategy orders | `strategy.entry/order/exit/close/cancel` state | Normalized to `orderIntents`, then executed by Go | Flow blocks cover the JFTrade tradeable subset. |
| Strategy metrics | `buy_and_hold_pnl`, `buy_and_hold_per_gain`, `strategy_outperformance` in PineTS `0.9.28` strategy state | Captured as optional `strategy_metrics` with presence flags so zero values remain distinguishable | No dedicated display yet; Go backtest metrics remain authoritative for JFTrade results. |
| Integer division and UDF history | `int / int` truncates toward zero; user-function return paths support `src[len]`, `close[len]`, and tuple-return computed history access in PineTS `0.9.28` | Passed through unchanged by the worker adapter and guarded by real PineTS executor tests | Reflected through normal plot/order outputs; no dedicated UI needed. |
| Arrays/maps/matrices | PineTS namespace support | Available to PineTS script execution; not a separate API payload | Monaco suggests common constructors/helpers; flow blocks expose only limited read-only collection stats. |

## Production-call audit

- Current production backtest and live paths must not call `pkg/backtest.Run` for Pine execution.
- `RunWithPineWorker`, `pineworker.Client`, and `pineworker.WorkerManager` are the supported execution boundaries.
- `pine-go-plan` must not be presented as selectable runtime. It is only valid in normalization shims and this compatibility audit.
- Generated support snapshots should not cite deleted Go runtime execution tests as current runtime proof unless the test still exists in the checkout.

## Frontend boundary

- Monaco is allowed to expose PineTS syntax and visual APIs because those scripts can run in the worker.
- Strategy visual flow blocks remain a JFTrade trading authoring surface. They should cover the standard order/condition/indicator/risk/parameter path, not every PineTS visual object.
- Visual Pine outputs should be rendered from worker/API `plots`, `alerts`, `visual_outputs`, and `strategy_metrics` contracts when the product adds chart/result rendering for them.
