# Pine v6 v1.7 Compatibility Roadmap

v1.7 targets approximately 95% practical migration coverage for closed-bar `strategy(...)` scripts.

## Current Score

The reproducible score model version is `closed-bar-strategy-v1.7`.

`strategy.pine_spec` emits the capability registry, statuses, unsupported IDs, dimension weights, and weighted practical migration score from one source of truth. The current weighted practical migration score is **95.13%**, reported as approximately **95%**.

The v1.7 model weights language at 12%, indicators at 30%, MTF data at 48%, orders at 0%, and tooling at 10%. Broker-emulator gaps remain explicit unsupported capabilities, but the practical language-migration score keeps them on the separate broker-emulator roadmap.

## Scope

- Introduce a formal Pine parser transition layer under `pkg/strategy/pine/parser` or by splitting the current `ast.go` path.
- Let existing lowering consume structured AST nodes first, while preserving current IR and runtime contracts.
- Add a semantic pass for variable scope, coarse const/simple/series typing, tuple destructuring, and built-in function signature diagnostics.
- Extend diagnostics and editor hints in `apps/web/src/features/strategyPineEditorIntelliSense.ts` and `apps/web/src/components/MonacoCodeEditor.vue`.

## Current Progress

- `AnalyzeScript` now builds a semantic summary from the structured AST when `IncludeAST` or `IncludeSemantic` is requested.
- The semantic summary exposes symbols, global/block scope, coarse value kinds, tuple bindings, and recognized function calls.
- The pass emits early diagnostics for duplicate tuple aliases and known built-in signature mismatches.
- The `/strategy-pine/analyze` route returns semantic data together with AST output, giving the frontend a stable transition surface for richer diagnostics.

## Boundaries

- Runtime and IR should remain mostly stable; only add fields to `pkg/strategy/ir/model.go` when required by a proven migration case.
- Dynamic symbol/timeframe `request.security`, full collections, methods/types/libraries, and TradingView broker emulator parity remain out of v1.7.

## Gate

- Add `TestPineV17MigrationCorpusGate`.
- Corpus size: at least 170 scripts.
- Runnable size: at least 55 scripts.
- Weighted corpus score: at least 95%.
- v1.3 through v1.6 corpus gates must stay green.
