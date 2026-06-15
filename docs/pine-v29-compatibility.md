# Pine v6 v2.9 Compatibility

v2.9 keeps the closed-bar `strategy(...)` migration track and closes practical object-history gaps around pure methods and MTF diagnostics.

## Executable And Diagnostic Additions

- Object history snapshots can be pure method receivers, for example `box[1].score(factor=2)`.
- Pure method chains keep named/default argument ordering, including `box.identity().score(offset=1, factor=2)`.
- `request.security(syminfo.tickerid, static_tf, expr)` accepts pure object history field and method expressions such as `box[1].price` and `box[1].score(2)`.
- Unsupported `request.security` calls now report distinct diagnostic codes for dynamic symbol, dynamic timeframe, nested request, side effect, `lookahead_on`, and `gaps_on`.

## Boundaries

- Historical object snapshots remain read-only.
- Method side effects, cross-library method execution, dynamic symbol/timeframe execution, nested `request.security`, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain unsupported.
- Full Pine overload/type-system parity is still not promised in v2.9.

## Evidence

- `TestPineV29MigrationCorpusGate`: at least 2500 scripts, 540 runnable cases, and weighted score >= 99.93.
- Focused coverage includes object history method receiver lowering/runtime execution, named/default method chains, MTF object history expressions, and request.security diagnostic codes.
