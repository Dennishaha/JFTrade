# Pine v6 v2.4 Compatibility

v2.4 keeps JFTrade's closed-bar `strategy(...)` migration model and raises the practical migration corpus score to 100.00 on the v2.4 positive corpus.

## Executable Additions

- `array` runtime coverage expands to `from`, `concat`, `join`, `sort`, `sort_indices`, `binary_search`, `median`, `mode`, and `range`.
- `map` runtime coverage expands to `copy`, `keys`, and `values`; keys and values are emitted in stable key-string order for reproducible backtests.
- `order.ascending` and `order.descending` are supported for deterministic collection sorting; nil values sort last.
- `request.security` supports `ta.stoch(source, high, low, length)` for same-symbol static timeframes.
- Static `for` loops with conditional `break` or `continue` fall back to bounded runtime loop execution.
- Persistent `var` UDT object fields can be reassigned with `:=` and keep their closed-bar state across bars.
- Object method expressions support named/default arguments in ordinary expressions and same-symbol static MTF pure expressions.

## Boundaries

- `request.security` still requires `syminfo.tickerid`, a static timeframe, default `gaps_off`, and default `lookahead_off`.
- Dynamic symbol/timeframe, nested request calls, side effects inside request expressions, and `lookahead_on`/`gaps_on` remain unsupported.
- Collection sorting supports deterministic number/string/bool/nil subsets; nested collections, deep generic parity, and the full Pine collection surface remain outside v2.4.
- `library` and `import` stay metadata/diagnostic only.
- Method side effects, complete overload/type-system parity, and cross-library methods remain unsupported.
- Broker emulator parity, partial fills, OCA, tick recalculation, and intrabar path simulation remain a separate roadmap.

## Evidence

- `TestPineV24MigrationCorpusGate`: at least 1250 scripts and 220 runnable cases.
- Current gate result: parse 100.00%, compile 100.00%, run 100.00%, weighted 100.00.
- `strategy.pine_spec` reports `productVersion=v2.4` and score model `closed-bar-strategy-v2.4`.
