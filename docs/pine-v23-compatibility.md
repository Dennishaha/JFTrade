# Pine v6 v2.3 Compatibility

v2.3 keeps JFTrade's closed-bar `strategy(...)` migration model and raises the practical migration corpus score to 99.13.

## Executable Additions

- `array` runtime coverage expands to `copy`, `slice`, `reverse`, `fill`, `includes`, `indexof`, `lastindexof`, `min`, `max`, `avg`, and `sum`.
- `matrix` runtime coverage expands to `fill`, `copy`, `reshape`, `add_row`, `add_col`, `remove_row`, and `remove_col`.
- Collection-returning operations such as `array.copy`, `array.slice`, `matrix.copy`, `matrix.remove_row`, and `matrix.remove_col` are tracked as collection variables for later method-style calls.
- Pure UDT support now accepts named constructor arguments, named/default method arguments, and controlled multi-statement pure method bodies.
- Local object fields can be reassigned with `:=`; persistent `var` object field reassignment remains diagnostic-only.
- `request.security` pure expressions can include supported collection reads and pure object field/method reads for same-symbol static timeframes.
- MTF purity validation now runs before `strategy.*` variable normalization so strategy state cannot leak into higher-timeframe expressions.

## Boundaries

- `request.security` still requires `syminfo.tickerid`, a static timeframe, default `gaps_off`, and default `lookahead_off`.
- Dynamic symbol/timeframe, nested request calls, side effects inside request expressions, and `lookahead_on`/`gaps_on` remain unsupported.
- Collection sorting, nested collections, deep generic parity, and the full Pine collection surface remain outside v2.3.
- `library` and `import` stay metadata/diagnostic only.
- Method side effects, complete overload/type-system parity, and cross-library methods remain unsupported.
- Broker emulator parity, partial fills, OCA, tick recalculation, and intrabar path simulation remain a separate roadmap.

## Evidence

- `TestPineV23MigrationCorpusGate`: at least 520 scripts and 140 runnable cases.
- Current gate result: parse 98.54%, compile 98.54%, run 100.00%, weighted 99.13.
- `strategy.pine_spec` reports `productVersion=v2.3` and score model `closed-bar-strategy-v2.3`.
