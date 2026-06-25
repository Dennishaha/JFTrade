# Pine v6 v0.8 RC Checklist

## Release Scope

v0.8 RC 锁定 JFTrade 可执行 Pine v6 策略子集；不扩展 TradingView 全量 v6 语言面。主路径为 `sourceFormat=pine-v6` 与 `runtime=pine-go-plan`。

本次发布范围：

- Parser/runtime：Pine metadata、结构化 diagnostics、UDF、静态 for、历史引用、barstate/session/time/math/string。
- Indicator runtime：source-aware MA/RSI/CCI/stdev、rolling window、SAR、MTF source、MTF MA、warmup/key parity。
- JFTrade API/ADK：`strategy.pine_spec`、`strategy.validate_pine`、保存定义、回测、实例启动/暂停/停止、compiled hooks/requirements。
- Frontend：Pine editor/intellisense、Pine ↔ visual model 标准块；无法标准化的 Pine 行反解失败，旧 visual block 被明确拒绝。
- Docs/typegen：Pine v6 支持矩阵、兼容层清单、黄金脚本、生成类型与发布说明。

## Dirty Worktree Grouping

v0.8 发布主线文件：

- `pkg/strategy/pine/*`、`pkg/strategy/ir/*`、`pkg/strategy/indicatorbinding/*`、`pkg/strategy/indicatorruntime/*`、`pkg/strategy/pineruntime/*`、`pkg/strategy/pinespec/*`
- `pkg/backtest/runner_test.go`
- `internal/app/apiserver/servercore/adk_runtime_test.go`、`internal/app/apiserver/servercore/strategy_pine_routes_test.go`、`internal/app/apiserver/servercore/strategy_runtime_manager_trading_test.go`
- `apps/web/src/features/strategyPineEditorIntelliSense.ts`、`apps/web/src/features/strategyVisualBuilderPine.ts`、`apps/web/src/features/strategyVisualBuilderPineParser.ts`
- `apps/web/tests/strategyVisualBuilderPine.test.ts`、`apps/web/tests/adkToolVisualizations.test.ts`

v0.9 兼容迁移前置文件：

- `apps/web/src/features/strategyVisualBuilderCatalog.ts`
- `apps/web/src/features/strategyVisualBuilderIndicatorBlock.ts`
- `apps/web/src/features/strategyVisualBuilderIndicatorShortcut.ts`
- `apps/web/src/features/strategyVisualBuilderNodePresentation.ts`
- `apps/web/src/features/strategyVisualBuilderShared.ts`
- `apps/web/src/components/strategy-stage/*`

文档与生成产物：

- `docs/release-pine-v08-closeout.md`
- `docs/pine-v08-rc-checklist.md`
- `docs/pine-v08-acceptance-matrix.md`
- `docs/frontend/strategy-authoring.md`
- `docs/reference/generated/types.md`

## RC Must-Haves

- `strategy.pine_spec` 输出 `supportMatrix`、`compatibilityLayers`、`unsupportedPatterns`、`goldenScripts`。
- `goldenScripts` 覆盖 MA、RSI/CCI/Williams/Bollinger、Donchian/volume MA/SAR、MTF、orders/exits、UDF/static for。
- close/default source legacy key 保留；非 close/default source 使用 source-aware key。
- Pine 反解无法标准化的行返回行号诊断，不生成 `pineSnippet` 或新 `codeBlock`。
- 旧 `codeBlock` 和旧合并 `technicalIndicator` 不再兼容读取；打开、保存或生成 Pine 时返回明确错误。
- 保存、预览、回测、实例化、运行以 `pine-v6` + `pine-go-plan` 为主路径。

## Freeze Rules

- v0.8 RC 不再扩新 Pine built-ins。
- 只修测试失败、文档错误、key parity、source/runtime 主路径和明显兼容回归。
- 不回滚当前 dirty worktree 中既有 Pine v6 成果；只做增量整理、补测和发布说明。

## Verification

必跑：

```bash
go test ./pkg/strategy/... ./internal/app/apiserver/servercore ./internal/api/strategy ./internal/strategy ./pkg/backtest/...
npm run test:web
npm run typecheck:web
```

专项：

```bash
go test ./pkg/backtest -run Pine
npm run test:web -- strategyVisualBuilderPine.test.ts
```
