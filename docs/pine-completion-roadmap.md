# Pine v6 Completion Roadmap

> Status: superseded for implementation planning by [PineTS Hard-Cut Migration Plan](pinets-hardcut-migration.md). Keep this document as the compatibility boundary reference only; do not use it to justify new Go Pine runtime or TradingView full-parity work.

## Summary

JFTrade should not claim blanket 100% TradingView Pine v6 parity. The active direction is PineTS worker execution for the JFTrade executable Pine v6 subset, with Go retaining trading, risk, order, and backtest authority. The practical compatibility boundary is therefore split into two explicit tracks:

- **Executable strategy completion**: closed-bar, same-symbol Pine v6 strategies that can compile, plan, backtest, and run through PineTS workers in JFTrade.
- **Full parity boundary**: TradingView-only behavior that remains parseable, diagnosable, or documented as out of scope.

The current `strategy.pine_spec` baseline is `ProductVersion v4.0`. It already goes beyond the older v1.7 95% roadmap: collection/map/matrix subsets, pure object/method subsets, expanded MTF expressions, semantic declarations, native public-surface diagnostics, MTF diagnostic matrix/preflight checks, advanced language boundary diagnostics, generated support snapshots, broker emulator boundary decisions, and large corpus gates are represented in the public spec. The next work should keep that broad surface auditable rather than expanding the score by hiding unsupported TradingView-only behavior.

## Completion Definition

JFTrade Pine v6 is considered complete for the executable strategy track when all of the following are true:

1. Every claimed public Pine form has parser, semantic, planner, PineTS worker, spec, editor hint, and regression-test coverage.
2. `strategy.pine_spec` is the single source of truth for supported, warning-only, and unsupported capabilities.
3. Unsupported Pine v6 behavior fails with stable diagnostic codes and line ranges, not silent fallback.
4. Visual builder output uses only Pine v6 native public syntax; internal helper keys remain runtime protocols only.
5. Backtest and live warmup use the same requirement derivation for indicators, MTF, stateful series, collections, and object history.
6. The migration corpus is versioned, reproducible, and gates any public score increase.

JFTrade is **not** complete for full TradingView parity until these separate boundaries are implemented or explicitly product-scoped out: cross-symbol `request.security`, dynamic requests, full library/import execution, complete Pine type/method overload resolution, all visual object APIs, intrabar tick recalculation, OCA groups, partial fills, and TradingView broker emulator edge cases.

## Current Baseline

- Pine public surface: native `ta.*`, `input.*`, `math.*`, `str.*`, `timeframe.*`, `strategy.*`, and static same-symbol `request.security`.
- Runtime protocol: internal requirement keys such as `ma:*`, `security_source:*`, `bollinger:*`, `anchored_vwap:*`, and collection/object state keys remain private planning/worker protocol details.
- Visual authoring: standardizable blocks only; complex scripts stay in the Pine workbench.
- MTF model: one native subscription plus upward aggregation; no lower-timeframe reconstruction and no cross-symbol live subscription in this track.
- Execution model: closed-bar strategy runtime, not full TradingView intrabar broker emulation.

## Phase 1: Spec Reconciliation

Goal: make the public support matrix match the implementation exactly.

- Audit `pkg/strategy/pinespec/spec.go` against parser, semantic, planner, PineTS worker behavior, and frontend completion lists.
- Split each capability into `supported`, `warningOnly`, `diagnosticOnly`, or `outOfScope`.
- Replace prose-only unsupported descriptions with stable diagnostic IDs for dynamic MTF, broker emulator gaps, library/import, visual APIs, and unsupported built-ins.
- Add a generated markdown snapshot so docs cannot drift from `strategy.pine_spec`.
- Remove any remaining public docs or examples that expose JFTrade helper syntax as user-authored Pine.

Acceptance:

- `strategy.pine_spec` and generated docs list the same capability statuses.
- `rg` finds internal helper names only in lowering/runtime/tests that explicitly assert private protocol behavior.

## Phase 2: Public Surface Closure

Goal: finish native Pine entry points before adding more runtime behavior.

- Verify every executable `ta.*`, `math.*`, `str.*`, `timeframe.*`, `input.*`, and `strategy.*` form has semantic signatures and Monaco completion/hover metadata.
- Add negative diagnostics for JFTrade-only public calls such as `ma(...)`, `security_source(...)`, `bollinger(...)`, `kdj(...)`, and `anchored_vwap(...)` when typed directly in Pine.
- Keep tuple aliases consistent across `ta.macd`, `ta.bb`, `ta.dmi`, `ta.supertrend`, `ta.kc`, and MTF tuple assignment.
- Make KDJ a generated Pine expression pattern only, not a public helper.

Acceptance:

- Public scripts and templates contain no user-facing internal helper calls.
- Direct helper calls return clear "use Pine v6 native syntax" diagnostics.

## Phase 3: MTF Completion

Goal: make static same-symbol `request.security` predictable and well bounded.

- Centralize timeframe normalization for parser, planner, runtime, warmup, visual builder, and diagnostics.
- Expand pure-expression MTF only where source/history/TA/collection/object dependencies can be planned deterministically.
- Keep dynamic symbol/timeframe, nested request, side effects, order calls, visual calls, `lookahead_on`, and `gaps_on` as explicit diagnostics.
- Add warmup parity tests for backtest and live startup across intraday, day, week, month, and extended-hours scenarios.

Acceptance:

- `1m + D/W/M` indicators use trading-period aggregation, not fixed bar counts.
- A lower-than-native timeframe request fails before runtime execution.

## Phase 4: Language And State Hardening

Goal: stabilize the advanced language subset already listed in v3.0.

- Lock supported array/map/matrix operations with parser, semantic, runtime, history, and benchmark coverage.
- Lock pure UDT constructor, field access, field reassignment, method receiver, method chain, and object history semantics.
- Define exact loop limits for static expansion, bounded runtime loops, `break`, and `continue`.
- Keep recursive UDF, nested function declarations, side-effect methods, deep generic overloads, and cross-library resolution as diagnostics.

Acceptance:

- Collection/object history behavior is deterministic across replay and live warmup.
- Corpus gates cover both successful execution and unsupported diagnostic cases.

## Phase 5: Broker Boundary

Goal: make order semantics honest and testable.

- Document the current closed-bar order model separately from Pine language compatibility.
- Add dedicated tests for `process_orders_on_close`, stop-limit activation, bracket priority, trailing exits, reversals, pyramiding, `allow_entry_in`, cancel behavior, commission, and slippage.
- Keep OCA, partial fills, margin liquidation, multi-symbol portfolio accounting, and intrabar recalculation out of the Pine completion score unless a broker-emulator track is opened.

Acceptance:

- Support score cannot be inflated by excluding order gaps silently; gaps are either in a separate broker score or explicitly out of scope.

## Phase 6: Corpus And Benchmark Gates

Goal: make "completion" reproducible.

- Version the migration corpus by capability family and expected outcome.
- Add minimum runnable and diagnostic-case counts per phase.
- Add performance baselines for new TA, MTF, collection, object-history, and loop-runtime families.
- Record score changes in docs only after CI passes parser, planner, runtime, frontend, and benchmark gates.

Recommended gates:

- Go: `go test ./pkg/strategy/indicatorbinding ./pkg/strategy/ir ./pkg/strategy/pine ./pkg/strategy/pineworker ./pkg/strategy/indicatorruntime ./pkg/strategy/pinespec`
- Web: `npm -w @jftrade/web run test -- strategyVisualBuilderPine StrategyStageOverlayDeck strategyAuthoringDocs`
- Typecheck: `npm -w @jftrade/web run typecheck`
- Full CI before score bumps: `go test ./...`, `npm run test:web`, `npm run build:web`, `git diff --check`
- Benchmarks: targeted `go test -bench` suites for any new runtime family.

## Proposed Milestones

- **v3.1 Public Surface Lock**: spec reconciliation, native-only examples, helper-call diagnostics, editor metadata parity.
- **v3.2 MTF Lock**: centralized timeframe normalization, warmup parity, pure-expression MTF diagnostic matrix.
- **v3.3 Advanced Language Lock**: collection/object/history semantics, loop limits, and stable UDF/loop boundary diagnostics.
- **v3.4 Completion Gate**: reproducible score, generated support snapshot docs, benchmark thresholds, no undocumented public behavior.
- **v4.0 Broker Emulator Decision**: complete; TradingView broker-emulator parity is formally outside JFTrade executable Pine v6 completion and tracked as a separate trading-runtime boundary.

## Open Decisions

- Whether the product should ever market "100% Pine v6" or only "100% of JFTrade executable Pine v6 strategy subset".
- Whether cross-symbol `request.security` requires multi-subscription live architecture or remains permanently unsupported.
- Whether visual APIs should stay warning-only/no-op or become a separate chart-rendering feature.
- Broker emulator parity now belongs in a separate trading-runtime score, not the JFTrade executable Pine v6 compatibility score.
