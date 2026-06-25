# Pine v6 v1.6 Practical Migration Compatibility

v1.6 targets executable, same-symbol, closed-bar TradingView `strategy(...)` migration. It keeps the focus on practical analysis, backtest, and runtime execution instead of full TradingView editor, drawing, or broker-emulator parity.

## Release Score

The v1.6 reproducible score model version was `closed-bar-strategy-v1.6`.

At the v1.6 checkpoint, `strategy.pine_spec` emitted a weighted practical migration score of **92.24%**, reported as approximately **92%**.

The v1.6 model weights language at 15%, indicators at 35%, MTF data at 38%, orders at 2%, and tooling at 10%. Broker-emulator gaps remain visible as unsupported order capabilities, but they are intentionally not allowed to dominate the v1.6 practical language-migration score.

The score remains an engineering estimate, not TradingView certification.

## v1.6 Additions

- `request.security` tuple assignment supports a white-listed same-symbol/static-timeframe subset such as `[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])`.
- MTF tuple assignment supports common multi-return indicators lowered through existing runtime objects: `ta.macd`, `ta.bb`, `ta.supertrend`, and `ta.kc`.
- Tuple aliases are normalized through the same expression path as ordinary assignments, including member aliases, history/source lowering, ternary, and math namespace replacement.
- Planner and runtime indicator binding now recognize the internal MTF MACD and Bollinger forms produced by `request.security(..., ta.macd/ta.bb(...))` lowering.
- `TestPineV16MigrationCorpusGate` raises the migration corpus to at least 130 scripts with at least 40 runnable cases and requires weighted corpus success of at least 92%.

## Important Boundaries

- `request.security` remains same-symbol and static-timeframe only.
- Tuple support is a whitelist, not general Pine tuple/array execution.
- Dynamic symbol/timeframe, `lookahead_on`, `gaps_on`, nested `request.security`, side effects, order calls, alert/log calls inside MTF expressions, collections, imports, methods, and user-defined types remain diagnostics.
- Full TradingView broker emulator behavior, OCA, partial fill, tick recalculation, intrabar path inference, and spot margin naked-short accounting remain out of scope.
- Visual APIs such as `plot`, drawings, and tables continue as warning/no-op compatibility shims.

## Test Gate

- `TestPineV13MigrationCorpusGate`, `TestPineV14MigrationCorpusGate`, `TestPineV15MigrationCorpusGate`, and `TestPineV16MigrationCorpusGate` must all pass.
- Required package checks include `pkg/strategy/pine`, `pkg/strategy/pinespec`, `pkg/strategy/ir`, `pkg/strategy/pineruntime`, `pkg/strategy/indicatorruntime`, and `pkg/backtest`.
- API and frontend checks remain part of release validation when Pine editor/spec surfaces change.
