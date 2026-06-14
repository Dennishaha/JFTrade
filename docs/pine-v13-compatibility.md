# Pine v6 v1.3 Practical Migration Compatibility

v1.3 targets executable, same-symbol, closed-bar TradingView strategy migration. It does not claim full Pine v6 or TradingView broker-emulator compatibility.

## Current Score

The reproducible score model version is `closed-bar-strategy-v1.3`.

`strategy.pine_spec` now emits the capability registry, capability statuses, unsupported IDs, dimension weights, and weighted practical migration score from one source of truth. The current weighted practical migration score is **75.18%**, reported as approximately **75%**.

The score remains an engineering estimate, not TradingView certification.

## v1.3 Additions

- `strategy.risk.allow_entry_in(strategy.direction.all|long|short)`.
- Pine-style `strategy.entry` reversal quantity calculation: reverse entries submit close-old-position plus new-entry quantity.
- Close-only behavior when `allow_entry_in` blocks a reverse entry direction.
- `ta.cmo`, `ta.tsi`, `ta.correlation`, `ta.dev`, `ta.median`, `ta.percentile_linear_interpolation`, `ta.percentile_nearest_rank`, `ta.percentrank`, and `ta.swma`.
- Static intraday `request.security` lowering for the v1.3 indicator set.
- `math.avg` and `math.round_to_mintick`.
- Registry-driven capability scoring and `SupportedFeatureIDs`.
- v1.3 golden script and a 60-script migration corpus gate with parse, compile, and runnable subset coverage.

## Important Boundaries

- Current JFTrade spot backtest accounting still does not simulate margin naked-short inventory. The Pine runtime computes reversal quantities, but full short broker accounting remains partial.
- `request.security` remains limited to same-symbol static timeframe source/source history, source-aware MA, and supported static intraday advanced indicators.
- Dynamic symbol/timeframe, `lookahead_on`, `gaps_on`, side-effect expressions, and general tuple `request.security` remain unsupported.
- `array`, `map`, `matrix`, `library/import`, Pine method/type, dynamic loops, `while`, recursive UDF, OCA, partial fill, tick recalculation, and full intrabar broker-emulator behavior remain out of scope.
- Visual APIs such as `plot`, drawings, and tables continue as warning/no-op compatibility shims.

## Test Gate

Required gates for this release line:

- Existing Go tests and Pine golden examples must remain green.
- `TestPineV13MigrationCorpusGate` must keep the 60-script corpus weighted score at or above 75%.
- Indicator vectors must cover warmup/history-short behavior and fixed audited numeric expectations.
- Order tests must assert submitted quantity and final behavior, not only absence of runtime errors.
- The historical absolute JSON benchmark remains visible until separately audited; relative base/head performance gates are the release criterion.
