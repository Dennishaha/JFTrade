# Pine v6 v2.7 Compatibility

v2.7 continues the closed-bar `strategy(...)` migration track and focuses on collection/timeframe ergonomics that appear in practical scripts.

## Executable Additions

- Array history snapshots now support read-only aggregate/stat calls including `range`, `median`, `mode`, `stdev`, and `variance`.
- `map.keys()` and `map.values()` can be used directly as deterministic arrays in `for...in` loops.
- Matrix read/write coverage is locked in for `rows`, `columns`, `get`, and `set`.
- `timeframe.in_seconds(timeframe?)`, `timeframe.multiplier`, and `timeframe.isseconds` are available in closed-bar expressions.
- `request.security` pure expressions accept supported string/timeframe helpers after lowering.

## Boundaries

- Historical collection mutation remains unsupported.
- Dynamic symbol/timeframe, nested `request.security`, `lookahead_on`, `gaps_on`, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain outside v2.7.
- Deep generic collection typing and full TradingView collection edge behavior are still not promised.

## Evidence

- `TestPineV27MigrationCorpusGate`: at least 1900 scripts and 380 runnable cases.
- Focused coverage includes collection history aggregate, map iteration, matrix read/write, timeframe helpers, and MTF helper expressions.
