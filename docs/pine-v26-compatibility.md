# Pine v6 v2.6 Compatibility

v2.6 continues the closed-bar `strategy(...)` migration track and thickens the executable collection/object language foundation.

## Executable Additions

- Array `for...in` loops are executable, including `for value in arr` and `for [index, value] in arr`; loop body `break` and `continue` use the existing bounded runtime loop model.
- Read-only collection history snapshots are supported for common array reads such as `arr[1].get(0)`, `arr[1].size()`, `arr[1].first()`, and `arr[1].last()`.
- Inline collection constructors can be used in pure expressions and object constructors, for example `Box.new(array.new_float())`.
- UDT fields can hold collection references, and object field collection methods such as `box.values.push(close)` and `box.values.size()` execute in closed-bar runtime.

## Boundaries

- Direct map iteration remains unsupported; use deterministic `m.keys()` or `m.values()` arrays before iterating.
- Historical collection snapshots are read-only. Mutating `arr[1]` remains unsupported.
- Method side effects, cross-library methods, dynamic `request.security`, `lookahead_on`, `gaps_on`, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain outside v2.6.
- `library`, `import`, and `export` remain metadata/diagnostic surfaces, not external code loading.

## Evidence

- `TestPineV26MigrationCorpusGate`: at least 1650 scripts and 310 runnable cases.
- `strategy.pine_spec` reports `productVersion=v2.6` and score model `closed-bar-strategy-v2.6`.
