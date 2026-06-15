# Pine v6 v2.8 Compatibility

v2.8 keeps the same closed-bar strategy boundary and thickens the object/method semantic layer without loading external TradingView libraries.

## Executable And Semantic Additions

- Object history reads such as `box[1].price` lower to read-only historical object snapshots.
- Pure method chains such as `box.identity().score(2)` lower to nested `object_method(...)` calls and execute in the closed-bar runtime.
- `request.security` pure expressions can include supported object method expressions.
- `export` declarations expose `exportedKind` metadata for exported functions, types, and methods.

## Boundaries

- `library` and `import` remain metadata/diagnostic surfaces; JFTrade does not load external TradingView libraries.
- Method side effects, cross-library method execution, dynamic `request.security`, `lookahead_on`, `gaps_on`, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain unsupported.
- Full Pine overload/type-system parity is not promised in v2.8.

## Evidence

- `TestPineV28MigrationCorpusGate`: at least 2200 scripts and 460 runnable cases.
- Focused coverage includes object history read, method chain lowering/runtime execution, export metadata, and MTF object method expressions.
