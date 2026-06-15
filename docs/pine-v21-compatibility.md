# Pine v6 v2.1 Compatibility

v2.1 keeps JFTrade's closed-bar `strategy(...)` execution model and raises the practical migration corpus score to 97.02.

## Executable Additions

- Persistent core collection runtime:
  - array: constructor, get/set, push/pop, shift/unshift, insert/remove, first/last, size, clear
  - map: constructor, get/put/remove, contains, size, clear
  - matrix: constructor, get/set, rows, columns
- Collection references survive across closed bars when declared with `var`; aliases share the same collection object.
- `ta.bbw(source, length, mult)` and `ta.cog(source, length)`.
- Closed-bar anchored VWAP for `ta.vwap(source, timeframe.change("D"|"W"|"M"))`.
- Static same-symbol `request.security` pure expressions receive a structured AST safety pass after lowering.

## Boundaries

- `request.security` still requires `syminfo.tickerid`, a static timeframe, default `gaps_off`, and default `lookahead_off`.
- Dynamic symbol/timeframe, side effects, nested requests, and general request tuples remain unsupported.
- Full collection copying, sorting, slicing, nested collection generics, and the complete Pine collection API remain outside v2.1.
- `type`, `method`, `library`, and `import` remain parse/semantic metadata only.
- Broker emulator parity remains a separate roadmap.

## Evidence

- `TestPineV21MigrationCorpusGate`: at least 250 scripts and 70 runnable cases.
- Current gate result: parse 95.03%, compile 95.03%, run 100.00%, weighted 97.02.
- `strategy.pine_spec` reports `productVersion=v2.1` and score model `closed-bar-strategy-v2.1`.
