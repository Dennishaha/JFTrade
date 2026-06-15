# Pine v6 v2.5 Compatibility

v2.5 keeps JFTrade's closed-bar `strategy(...)` migration model and adds high-frequency Pine helper surfaces without changing broker-emulator scope.

## Executable Additions

- `array` runtime coverage expands to `abs`, `binary_search_leftmost`, `binary_search_rightmost`, `percentrank`, `percentile_nearest_rank`, `percentile_linear_interpolation`, `stdev`, `variance`, and `covariance`.
- `str.length`, `str.contains`, `str.pos`, `str.substring`, `str.replace`, `str.upper`, `str.lower`, and `str.format` lower to deterministic runtime helpers.
- `time_close` is available as a closed-bar timestamp; `timeframe.change(static_tf)` is supported for static minute/hour/day/week/month buckets.

## Boundaries

- Array statistics require numeric values; empty arrays return `na` except existing `sum` behavior.
- `timeframe.change` supports only static JFTrade timeframes already accepted by the Pine runtime.
- Dynamic `request.security`, `lookahead_on`, `gaps_on`, nested request calls, broker emulator parity, OCA, partial fill, and intrabar tick recalculation remain unsupported.

## Evidence

- `TestPineV25MigrationCorpusGate`: at least 1450 scripts and 260 runnable cases.
- `strategy.pine_spec` reports the latest product version and includes v2.5 capability rows.
