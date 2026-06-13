# Pine v6 v0.8 Acceptance Matrix

## Coverage Model

每个 v0.8 golden script 至少要被一个测试层覆盖。新增 Pine 能力必须同步更新 parser lowering、IR planner、runtime/expression lookup、`strategy.pine_spec`、前端提示或 round-trip 测试。

覆盖层含义：

- Analyze：`strategypine.AnalyzeScript` 返回 OK。
- Requirements：`strategyir.PlanRequirements` 输出预期 indicator keys。
- Backtest：`pkg/backtest` Pine 专项能完整运行且无 runtime error。
- Frontend：Pine ↔ visual model round-trip 或新建路径测试。
- ADK/spec：`strategy.pine_spec` payload 暴露对应能力。

## Golden Scripts

| Golden script | 能力 | Analyze | Requirements | Backtest | Frontend | ADK/spec |
| --- | --- | --- | --- | --- | --- | --- |
| `golden-ma-cross` | EMA/SMA、crossover、entry | `TestGoldenExamplesAnalyzeAndPlan` | `ma:EMA:8`、`ma:SMA:21` | `TestRunUsesTradingViewDefaultQuantityForNVDAStylePine` | `strategyVisualBuilderPine.test.ts` MA round-trip | `goldenScripts` |
| `golden-oscillators-bands` | RSI、CCI、Williams %R、Bollinger | `TestGoldenExamplesAnalyzeAndPlan` | `rsi:14`、`cci:20`、`williamsr:14`、`bollinger:20:2` | benchmark/backtest indicator-heavy script | Bollinger/Williams alias tests | `indicatorFunctions` |
| `golden-donchian-volume-sar` | highest/lowest、volume MA、SAR | `TestGoldenExamplesAnalyzeAndPlan` | `highest:high:20`、`lowest:low:20`、`ma:SMA:10:volume`、`sar:0.02:0.02:0.2` | Donchian、volume MA、SAR backtests | source-aware MA round-trip | `supportMatrix` |
| `golden-mtf-source-ma` | request.security source/source[n]、MTF EMA | `TestGoldenExamplesAnalyzeAndPlan` | `security_source:15m:close`、`security_source:15m:close:1`、`ma:EMA:3:15m:hlc3` | MTF intraday backtest | timeframe MA round-trip | `supportMatrix` |
| `golden-orders-exits` | qty_percent、strategy.order、pending、bracket、cancel | `TestGoldenExamplesAnalyzeAndPlan` | no indicator key required | qty_percent、strategy.order、close_all、pending/bracket/cancel backtests | order forms parse test | `orderModes` |
| `golden-udf-static-for` | expression UDF、static for、history、input.int | `TestGoldenExamplesAnalyzeAndPlan` | `ma:EMA:3` | UDF/static for backtest | Pine snippet fallback for non-visual lines | `unsupportedPatterns` |

## Compatibility Matrix

| Compatibility item | Expected behavior | Coverage |
| --- | --- | --- |
| close legacy indicator keys | close/default source keeps legacy keys; volume/hlc3/open/high/low use source-aware keys | `TestPlanRequirementsPreservesLegacyCloseKeysAndSourceAwareKeys` |
| `pineSnippet` fallback | unsupported Pine lines and old codeBlock annotations become `pineSnippet` in Pine reverse parsing | `strategyVisualBuilderPine.test.ts` snippet tests |
| legacy `codeBlock` visual model | old visual model still reads and renders a deprecation log; custom JS is not emitted | `strategyVisualBuilderPine.test.ts` legacy codeBlock test |
| legacy unified `technicalIndicator` | known old blocks auto-migrate to `getTechnicalIndicator` + `technicalIndicatorCondition`; unknown old blocks remain read-only | `strategyVisualBuilderMigration.test.ts` fixture matrix |
| ADK/spec payload | `supportMatrix`、`compatibilityLayers`、`unsupportedPatterns`、`goldenScripts` survive the tool layer | `TestADKStrategyPineSpecToolReturnsStructuredPayload` |

## Remaining Gaps Before v1.0

- No full TradingView broker emulator parity: OCA、partial fill、intrabar execution remain unsupported.
- No arrays/maps/matrices/library/import support.
- Frontend remains a visual editing layer for common strategies, not a complete Pine IDE.
- Internal `codeBlockCount` naming is intentionally retained until v1.0 cleanup to avoid low-value API churn.
