# PineTS contract audit

Scope: current mainline PineTS execution contract. The completed hard-cut process is retained in Git history rather than duplicated here.

This audit describes the Rust-owned PineTS worker boundary. It separates trading contracts from visual Pine outputs so frontend authoring does not imply that every PineTS visual feature is also a tradeable JFTrade flow block.

## Contract matrix

| Surface | Status | Current contract |
| --- | --- | --- |
| Rust Pine integration | Required production boundary | `jftrade-integration-pine::PineProcess` owns worker startup, authentication, request/response, timeout and shutdown. Missing worker composition fails closed. |
| Backtest service/API | Compatible through dependency injection | Rust engine startup injects a configured Pine port; service-level execution fails fast when no worker is configured instead of falling back to another Pine runtime. |
| Strategy definition runtime | Current | `runtime=pine-pinets` is the current value. Historical runtime aliases are fixture-only compatibility data, not selectable production runtimes. |
| Strategy source format | Compatible | `sourceFormat=pine-v6` remains the only supported source format. |
| Pine worker proto | Additive compatible | Existing fields remain unchanged. Order intents add `parent_id`、`atomic_group_id`、`oco_group_id`、`reduce_only`; live requests add session operation and expected revision, responses add session revision. |
| Backtest result model | Compatible | Rust is authoritative for fills, trades, equity, metrics and result collection. PineTS supplies order intents, visual outputs and upstream strategy metrics for inspection. |
| Live order path | Single-owner | PineTS produces current-bar order intents; Rust performs risk checks, notifications, broker reads and order placement. |
| ADK/spec payload | Current | Public spec/runtime surfaces advertise `runtime=pine-pinets`; historical runtime text is limited to immutable compatibility fixtures. |

## Live incremental worker contract

Production live Pine opens one stateful Worker session per strategy instance and symbol. `open` performs the complete historical warmup once and returns revision 1 without emitting historical orders. Each later closed bar is sent through `append` with the caller's expected revision; PineTS appends the candle to the existing runtime, executes only the new global bar indices, and returns delta plots/events/order intents plus the next revision. `close` retires the pinned Worker and releases its runtime slot.

The session validates immutable source, symbol, timeframe and params, requires strictly increasing candle times, serializes appends, and invalidates itself after an execution failure. A stale revision cannot be replayed into the same state. Test adapters that do not implement the session opener may use a fixture-backed full-history path, but the bundled production Worker advertises `live-session-v1` and uses the stateful path.

This protocol removes the repeated full-history calculation from the long-running production path without truncating history or changing `var`、series history、order state and global `bar_index` semantics. It intentionally uses PineTS's append/iteration hooks pinned to the tested PineTS version; upgrading PineTS must rerun the real incremental-state regression before changing the bundled Worker.

## Atomic protective-order contract

A same-bar entry and its protective exit are emitted with one atomic group. The exit references the parent entry, is reduce-only, and a limit-plus-stop bracket carries one OCO group. Rust expands a dual-price exit into separate limit and stop legs, preflights the entire bar before any order side effect, and submits the complete group only through the atomic execution port.

An execution backend implementing that interface promises all-or-none acceptance, child activation only after the parent fill, OCO sibling cancellation and reduce-only enforcement at match time. A backend that cannot make all four promises is not allowed to emulate the group with sequential `SubmitOrders`: the complete group is rejected before the entry is placed. The current Futu live adapter does not claim this atomic capability, so same-bar protective groups fail closed rather than opening an unprotected position. This protocol is narrower than general TradingView OCA/partial-fill parity, which remains outside the current broker-emulator score.

## PineTS capability alignment

| Capability | PineTS `0.9.31` | Worker contract | Frontend support |
| --- | --- | --- | --- |
| Numeric plots | Supported through `plots` | `plots` and `outputs` preserve numeric series | Monaco suggests `plot`; rendering remains a separate chart concern. |
| Alerts | `alert()` / `alertcondition()` events | `alerts` carries normalized alert events | Monaco suggests `alert` and `alertcondition`; flow notify blocks still generate `alert()`. |
| Shapes and chars | `plotshape`, `plotchar`, `plotarrow` | Captured as `visual_outputs` when PineTS returns visual payloads | Monaco suggests common shape/char calls; no trading flow block is created for them. |
| Drawing objects | `label`, `line`, `box`, `polyline`, `linefill` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests common constructors; visual model does not treat drawings as orders. |
| Tables | `table.*` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests `table.new`; flow blocks stay trading-focused. |
| Strategy orders | `strategy.entry/order/exit/close/cancel` state | Normalized to `orderIntents`, then executed by Rust | Flow blocks cover the JFTrade tradeable subset. |
| Strategy metrics | `buy_and_hold_pnl`, `buy_and_hold_per_gain`, `strategy_outperformance` in PineTS `0.9.31` strategy state | Captured as optional `strategy_metrics` with presence flags so zero values remain distinguishable | No dedicated display yet; Rust backtest metrics remain authoritative for JFTrade results. |
| Integer division and UDF history | `int / int` truncates toward zero; user-function return paths support `src[len]`, `close[len]`, and tuple-return computed history access in PineTS `0.9.31` | Passed through unchanged by the worker adapter and guarded by real PineTS executor tests | Reflected through normal plot/order outputs; no dedicated UI needed. |
| Arrays/maps/matrices | PineTS namespace support | Available to PineTS script execution; not a separate API payload | Monaco suggests common constructors/helpers; flow blocks expose only limited read-only collection stats. |
| Extended tickers | `ticker.heikinashi`, `ticker.standard`, `ticker.inherit`, `chart.is_*` | Current symbol only; aggregate standard bars before HA conversion, and refresh secondary contexts incrementally in live sessions | Workspace, backtest, and strategy bindings select standard K line or Heikin Ashi independently. |

## Extended ticker usage

The chart type controls the primary OHLC series seen by a Pine script. A standard chart exposes standard OHLC; a Heikin Ashi chart exposes derived OHLC and sets `chart.is_heikinashi` to `true`. `chart.is_standard` is the corresponding standard-chart flag.

```pine
//@version=6
strategy("Standard chart")
isStandard = chart.is_standard
signal = ta.crossover(close, ta.sma(close, 20))
```

```pine
//@version=6
strategy("Heikin Ashi chart")
haTicker = ticker.heikinashi(syminfo.tickerid)
haClose = request.security(haTicker, "60", close)
signal = chart.is_heikinashi and haClose > haClose[1]
```

When a strategy runs on a Heikin Ashi chart but needs raw standard prices, use `ticker.standard()`:

```pine
//@version=6
strategy("HA signal with standard reference")
standardClose = request.security(ticker.standard(), "60", close)
signal = chart.is_heikinashi and close > standardClose
```

Only the current base symbol is accepted. External symbols, dynamic ticker expressions, and non-HA synthetic chart types remain unsupported. Pine signals and indicators can read Heikin Ashi data, but JFTrade always executes fills, PnL, and risk checks from standard OHLC.

## Production-call audit

- Current production backtest and live paths must use the Rust Pine integration port.
- `jftrade-integration-pine::PineProcess` and the engine composition are the supported execution boundaries.
- Historical runtime aliases must not be presented as selectable runtimes.
- Generated support snapshots must cite current Rust/Node tests; deleted runtime tests are history, not current proof.

## Frontend boundary

- Monaco is allowed to expose PineTS syntax and visual APIs because those scripts can run in the worker.
- Strategy visual flow blocks remain a JFTrade trading authoring surface. They should cover the standard order/condition/indicator/risk/parameter path, not every PineTS visual object.
- Visual Pine outputs should be rendered from worker/API `plots`, `alerts`, `visual_outputs`, and `strategy_metrics` contracts when the product adds chart/result rendering for them.
