# Pine v6 v1.0 Main Path

v1.0 confirms JFTrade Pine v6 as the primary strategy path for the executable Pine subset. It does not claim full TradingView Pine v6 compatibility.

## Scope

- Strategy definition, preview, backtest, instantiation, runtime lifecycle, and ADK tooling use `sourceFormat=pine-v6` and `runtime=pine-go-plan`.
- New authoring paths generate Pine v6 script, standard Pine visual blocks, or `pineSnippet` fallbacks.
- Golden scripts cover MA cross, RSI/CCI/Williams/Bollinger, Donchian, volume MA, SAR, MTF, qty_percent, pending/bracket/cancel, UDF, and static for.
- Unsupported TradingView features such as arrays, maps, matrices, library/import, dynamic requests, and full broker emulator semantics remain explicit diagnostics or out of scope.

## Legacy Cleanup

- `codeBlock` and unified `technicalIndicator` are not valid v1.0 visual blocks.
- Legacy visual models are rejected instead of auto-migrated.
- Explicit non-Pine source/runtime records are rejected instead of normalized to default Pine.
- `pineSnippet` remains the supported fallback for legal Pine statements that cannot be represented by standard visual blocks.
- The reverse parser reports `pineSnippetCount` for fallback snippets.

## Gate

Before changing Pine capabilities, update parser lowering, IR planning, runtime/expression lookup, `strategy.pine_spec`, editor IntelliSense or docs, and at least one regression layer.

Recommended v1.0 validation:

```bash
go test ./pkg/strategy/... ./internal/app/apiserver/servercore ./internal/api/strategy ./internal/strategy ./pkg/backtest/...
go test ./internal/app/apiserver/servercore -run 'StrategyDefinition|StrategyPine|Runtime|Backtest'
npm run test:web
npm run typecheck:web
```
