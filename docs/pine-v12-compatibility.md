# Pine v6 v1.2 Practical Migration Compatibility

v1.2 targets executable, same-symbol, closed-bar TradingView strategy migration. It does not claim full Pine v6 or TradingView broker-emulator compatibility.

## Compatibility score

The reproducible model version is `closed-bar-strategy-v1.2`:

| Dimension | Weight | Score |
| --- | ---: | ---: |
| Language and expressions | 25% | 60% |
| Indicators | 25% | 65% |
| MTF data | 15% | 55% |
| Order semantics | 25% | 65% |
| Tooling | 10% | 90% |

The weighted practical migration score is **64.75%**, reported as approximately **65%**. The score and capability status are emitted by `strategy.pine_spec`; they are engineering estimates, not TradingView certification.

## v1.2 additions

- Native closed-bar trailing exits for `trail_points + trail_offset` and `trail_price + trail_offset`, interpreted in market ticks.
- Pending stop-limit orders that activate at the stop and then remain as limit orders.
- Conservative stop-first handling when bracket conditions collide on one bar.
- `ta.linreg`, `ta.obv`, `ta.pivothigh`, `ta.pivotlow`, `ta.kc`, `ta.kcw`, and `ta.alma`.
- Static intraday `request.security` lowering for the new indicators, plus the existing source, history, and moving-average subset.
- Compile-time lowering for `switch` and controlled multi-statement UDFs.
- A single capability registry used by feature IDs and the Pine specification payload.

## Deliberate boundaries

Arrays, maps, matrices, libraries/imports, methods/types, dynamic loops, and `while` remain unsupported. Dynamic symbols/timeframes, side-effect security expressions, `lookahead_on`, and `gaps_on` are rejected. Visual APIs remain warning/no-op behavior.

The order runtime remains closed-bar. It does not infer intrabar paths and does not implement partial fills, OCA, or tick-level recalculation.

## Performance baseline audit

The absolute JSON dated May 30, 2026 was recorded against an older DSL workload. The benchmark matrix now uses `sourceFormat=pine-v6`, so the old numbers are intentionally left visible but are not comparable.

The baseline gate now records a SHA-256 workload fingerprint, source format, sample count, audit commit, and hardware description. Regeneration requires:

```bash
JFTRADE_UPDATE_STRATEGY_BLOCK_BASELINE=1 \
JFTRADE_BENCHMARK_AUDIT_COMMIT="$(git rev-parse HEAD)" \
JFTRADE_BENCHMARK_AUDIT_HARDWARE="<runner and CPU description>" \
go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkBaseline$' -count=1
```

Normal release gating uses same-runner base/head medians. The 20% improvement requirement is enabled only with `JFTRADE_REQUIRE_PINE_BENCHMARK_IMPROVEMENT=1`; compatibility releases otherwise enforce non-regression thresholds.
