# PineTS contract audit

Baseline: `fcf74cecfabab5a998c73345af3eefe78bde7021`.

This audit tracks the hard cut from the former Go Pine runtime to the PineTS worker boundary. It separates trading contracts from visual Pine outputs so frontend authoring does not imply that every PineTS visual feature is also a tradeable JFTrade flow block.

## Contract matrix

| Surface | Status | Current contract |
| --- | --- | --- |
| `pkg/backtest.Run` | Intentional break | Direct Go Pine execution is disabled and returns an error. Production callers must use `RunWithPineWorker` with an explicit Pine worker runner. |
| Backtest service/API | Compatible through dependency injection | API startup injects a configured `pineworker.WorkerManager`; service-level execution fails fast when no worker runner is configured instead of falling back to Go Pine. |
| Strategy definition runtime | Migration compatible | `runtime=pine-pinets` is the current value. `pine-go-plan` is accepted only as a migration alias and normalized to `pine-pinets`. |
| Strategy source format | Compatible | `sourceFormat=pine-v6` remains the only supported source format. |
| Pine worker proto | Additive compatible | Existing response fields remain unchanged; `alerts` and `visual_outputs` are appended after the original fields. |
| Backtest result model | Compatible | Go remains authoritative for fills, trades, equity, metrics, and result collection. PineTS worker supplies order intents and visual outputs. |
| Live order path | Compatible with new authority split | PineTS worker produces current-bar order intents; Go still performs risk checks, notifications, broker reads, and order placement. |
| ADK/spec payload | Migration compatible | Public spec/runtime surfaces advertise `runtime=pine-pinets`; legacy runtime text is limited to migration/history notes. |

## PineTS capability alignment

| Capability | PineTS `0.9.26` | Worker contract | Frontend support |
| --- | --- | --- | --- |
| Numeric plots | Supported through `plots` | `plots` and `outputs` preserve numeric series | Monaco suggests `plot`; rendering remains a separate chart concern. |
| Alerts | `alert()` / `alertcondition()` events | `alerts` carries normalized alert events | Monaco suggests `alert` and `alertcondition`; flow notify blocks still generate `alert()`. |
| Shapes and chars | `plotshape`, `plotchar`, `plotarrow` | Captured as `visual_outputs` when PineTS returns visual payloads | Monaco suggests common shape/char calls; no trading flow block is created for them. |
| Drawing objects | `label`, `line`, `box`, `polyline`, `linefill` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests common constructors; visual model does not treat drawings as orders. |
| Tables | `table.*` helpers | Captured as `visual_outputs` when returned by the worker result | Monaco suggests `table.new`; flow blocks stay trading-focused. |
| Strategy orders | `strategy.entry/order/exit/close/cancel` state | Normalized to `orderIntents`, then executed by Go | Flow blocks cover the JFTrade tradeable subset. |
| Arrays/maps/matrices | PineTS namespace support | Available to PineTS script execution; not a separate API payload | Monaco suggests common constructors/helpers; flow blocks expose only limited read-only collection stats. |

## Production-call audit

- Current production backtest and live paths must not call `pkg/backtest.Run` for Pine execution.
- `RunWithPineWorker`, `pineworker.Client`, and `pineworker.WorkerManager` are the supported execution boundaries.
- `pine-go-plan` must not be presented as selectable runtime. It is only valid in normalization shims and historical docs.
- Generated support snapshots should not cite deleted Go runtime execution tests as current runtime proof unless the test still exists in the checkout.

## Frontend boundary

- Monaco is allowed to expose PineTS syntax and visual APIs because those scripts can run in the worker.
- Strategy visual flow blocks remain a JFTrade trading authoring surface. They should cover the standard order/condition/indicator/risk/parameter path, not every PineTS visual object.
- Visual Pine outputs should be rendered from worker/API `plots`, `alerts`, and `visual_outputs` contracts when the product adds chart rendering for them.
