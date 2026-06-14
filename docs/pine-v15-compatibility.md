# Pine v6 v1.5 Practical Migration Compatibility

v1.5 targets executable, same-symbol, closed-bar TradingView strategy migration. It does not claim full Pine v6 or TradingView broker-emulator compatibility.

## Current Score

The reproducible score model version is `closed-bar-strategy-v1.5`.

`strategy.pine_spec` emits the capability registry, statuses, unsupported IDs, dimension weights, and weighted practical migration score from one source of truth. The current weighted practical migration score is **87.03%**, reported as approximately **87%**.

The v1.5 model weights language at 23%, indicators at 32%, MTF data at 25%, orders at 10%, and tooling at 10%. This reflects the v1.5 focus on common closed-bar migration scripts while keeping broker-emulator gaps visible.

The score remains an engineering estimate, not TradingView certification.

## v1.5 Additions

- `request.security` pure expressions now cover common TA combinations for RSI, MACD member reads, ATR, Bollinger member reads, and Supertrend member reads on same-symbol static timeframes.
- Direct helper coverage includes `ta.range` and `ta.mode`, plus locked planner/runtime tests for mixed common TA migration cases.
- Cross/state migration samples combine `ta.crossover`, `ta.crossunder`, `ta.cross`, `ta.barssince`, and `ta.valuewhen`.
- Static `for` lowering supports an unconditional `break` / `continue` subset.
- v1.5 golden scripts cover MTF common TA, cross/state strategies, and static loop control.
- `TestPineV15MigrationCorpusGate` raises the migration corpus to at least 100 scripts with at least 28 runnable cases and requires weighted corpus success of at least 87%.

## Important Boundaries

- `request.security` remains same-symbol and static-timeframe only. Dynamic symbol/timeframe, `lookahead_on`, `gaps_on`, side effects, order calls, alert/log calls, and general tuple security remain unsupported diagnostics.
- MTF common TA is implemented through synthetic indicator bindings, not a general nested Pine interpreter.
- Static loop control covers unconditional `break` / `continue`; dynamic loops, `while`, conditional control-flow lowering, recursive UDF, nested UDF, method/type, library/import, array/map/matrix remain out of scope.
- Full TradingView broker emulator behavior, OCA, partial fill, tick recalculation, intrabar path inference, and spot margin naked-short accounting remain out of scope.
- Visual APIs such as `plot`, drawings, and tables continue as warning/no-op compatibility shims.

## Test Gate

- Existing Go tests, frontend tests, TypeScript typecheck, build, golden scripts, and benchmark smoke gates must remain green.
- `TestPineV13MigrationCorpusGate`, `TestPineV14MigrationCorpusGate`, and `TestPineV15MigrationCorpusGate` must all pass.
- Indicator tests must cover warmup/history-short behavior, state reset where applicable, tuple/member field reads, and fixed numeric expectations.
- Order tests continue asserting quantity, price, order sequence, and final position for the supported subset.
- The historical absolute JSON benchmark remains visible until separately audited; relative base/head performance gates are the release criterion.
