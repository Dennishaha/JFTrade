# Pine v6 v2.2 Compatibility

v2.2 keeps JFTrade's closed-bar `strategy(...)` model and raises the practical migration corpus score to 98.24.

## Executable Additions

- Structured AST lowering now consumes the indentation tree while preserving the existing IR shape.
- General tuple literal/destructure supports 2 to 8 aliases, including `_` discard.
- `request.security` supports 2 to 8 element pure-expression tuples for static same-symbol timeframes.
- Dynamic `for`, `while`, `break`, and `continue` execute in the closed-bar runtime with bounded nesting and per-bar iteration limits.
- Pure UDT constructor and single-expression method calls execute for side-effect-free object subsets.
- Reassignment invalidates compile-time value aliases so loop state such as `count := count + 1` remains dynamic.

## Boundaries

- `request.security` still requires `syminfo.tickerid`, a static timeframe, default `gaps_off`, and default `lookahead_off`.
- Dynamic symbol/timeframe, nested request calls, side effects inside request expressions, and `lookahead_on`/`gaps_on` remain unsupported.
- Full Pine tuple/array interop, nested collections, copy/sort/slice APIs, and complete collection coverage remain outside v2.2.
- `library` and `import` stay metadata/diagnostic only.
- Method side effects, full overload/type-system parity, inheritance-like patterns, and cross-library methods remain unsupported.
- Broker emulator parity, partial fills, OCA, tick recalculation, and intrabar path simulation remain a separate roadmap.

## Evidence

- `TestPineV22MigrationCorpusGate`: at least 420 scripts and 110 runnable cases.
- Current gate result: parse 97.06%, compile 97.06%, run 100.00%, weighted 98.24.
- `strategy.pine_spec` reports `productVersion=v2.2` and score model `closed-bar-strategy-v2.2`.
