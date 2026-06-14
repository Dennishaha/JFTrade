# Pine v6 v1.4 Practical Migration Compatibility

v1.4 targets executable, same-symbol, closed-bar TradingView strategy migration. It does not claim full Pine v6 or TradingView broker-emulator compatibility.

## Current Score

The reproducible score model version is `closed-bar-strategy-v1.4`.

`strategy.pine_spec` emits the capability registry, capability statuses, unsupported IDs, dimension weights, and weighted practical migration score from one source of truth. The current weighted practical migration score is **81.96%**, reported as approximately **82%**.

The v1.4 score model keeps language at 25% and tooling at 10%, raises indicators to 30% and MTF data to 20%, and lowers orders to 15%. This matches the v1.4 goal of practical closed-bar migration coverage while keeping broker-emulator gaps visible.

The score remains an engineering estimate, not TradingView certification.

## v1.4 Additions

- `ta.highestbars`, `ta.lowestbars`, `ta.change`, `ta.mom`, `ta.roc`, `ta.rising`, `ta.falling`, `ta.stdev`, and `ta.variance` are locked into the capability registry and golden examples.
- `ta.barssince` and `ta.valuewhen` are treated as supported closed-bar stateful event helpers.
- `ta.tr(true|false)` is documented and tested with first-bar fallback behavior.
- `request.security` supports same-symbol static-timeframe pure expressions composed from source/source history, supported MTF moving averages, math, comparisons, boolean logic, ternary, `na`, and `nz`.
- Pure-expression `request.security` continues to reject side effects, tuples, dynamic symbol/timeframe, `lookahead_on`, and `gaps_on`.
- v1.4 golden scripts cover window momentum, state events, TR/ATR, and MTF pure expressions.
- `TestPineV14MigrationCorpusGate` raises the migration corpus to at least 80 scripts with at least 20 runnable cases and requires weighted corpus success of at least 82%.

## Important Boundaries

- Current JFTrade spot backtest accounting still does not simulate margin naked-short inventory. Full short broker accounting remains partial.
- `request.security` pure expressions are not a full nested Pine interpreter; unsupported TA, tuple returns, visual calls, order calls, and side effects remain diagnostics.
- Dynamic symbol/timeframe, `lookahead_on`, `gaps_on`, general tuple `request.security`, array/map/matrix, library/import, Pine method/type, dynamic loops, `while`, recursive UDF, OCA, partial fill, tick recalculation, and full intrabar broker-emulator behavior remain out of scope.
- Visual APIs such as `plot`, drawings, and tables continue as warning/no-op compatibility shims.

## Test Gate

Required gates for this release line:

- Existing Go tests and Pine golden examples must remain green.
- `TestPineV13MigrationCorpusGate` remains green so v1.3 coverage is not lost.
- `TestPineV14MigrationCorpusGate` must keep the 80-script corpus weighted score at or above 82%.
- Indicator vectors and expression tests must cover warmup/history-short behavior, state reset, and fixed numeric expectations where applicable.
- Order tests must continue asserting quantity, price, order sequence, and final position for the supported subset.
- The historical absolute JSON benchmark remains visible until separately audited; relative base/head performance gates are the release criterion.
