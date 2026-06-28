# JFTrade Pine v6 v0.8 发布收口说明

> Historical note: this v0.8 closeout predates the PineTS hard cut. Its `pine-go-plan` references describe the old release line only; current development uses `runtime=pine-pinets` and follows [pinets-hardcut-migration.md](pinets-hardcut-migration.md).

## 当前定位

v0.8 锁定的是 JFTrade 可执行 Pine v6 策略子集，不追 TradingView Pine v6 全量语言或完整 broker emulator。主路径为 `sourceFormat=pine-v6` + `runtime=pine-go-plan`。

已纳入本次收口的能力包括：

- Pine parser/runtime：`strategy(...)`、`if/else`、`var`/`varip`/`const` 兼容、重赋值、历史引用、单表达式 UDF、静态 for、input 默认值、barstate/session/time/math/string。
- 指标与 MTF：source-aware MA/RSI/CCI/stdev/rolling window、Donchian、Williams %R、Bollinger、SAR、MTF source 与 MTF MA。
- 订单与退出：`qty_percent`、`strategy.order`、`close_all`、pending stop/limit、bracket exit、`cancel`/`cancel_all`。
- 集成与前端：`strategy.pine_spec` 输出 `supportMatrix`、`compatibilityLayers`、`unsupportedPatterns`、`goldenScripts`；流程图只接受可标准化的 Pine v6 语义块，无法标准化的 Pine 行返回行号诊断。

## 明确不支持

- `array.*`、`map.*`、`matrix.*`、`library/import`、Pine type/method、多语句或递归 UDF。
- 动态 for、`break`/`continue`、任意表达式 MTF、`lookahead_on`/`gaps_on`。
- OCA、partial fill、intrabar broker emulator、entry/order stop-limit 组合、exit trail 与 bracket 混用。
- TradingView 视觉对象只做 warning/no-op 兼容，不参与执行。

## 迁移注意

- 旧 `codeBlock` 和 `pineSnippet` 不再作为 visual model 兼容路径；Pine 反解无法标准化时整体失败并提示继续在 Pine 工作台编辑。
- 旧合并式 `technicalIndicator` 块不再迁移或只读保留；新建、模板、打开和生成路径只接受标准 Pine visual blocks。
- close/默认源指标继续保留 legacy key，例如 `ma:SMA:20`、`rsi:14`、`cci:20`；volume、hlc3 等新增 source 必须走 source-aware key，避免指标串线。
- 此 v0.8 迁移说明已被 v1.0 主路径取代：显式旧 source/runtime 与旧 visual model 现在会被拒绝，不再自动替换。

## 黄金脚本与验收

`strategy.pine_spec` 的 `goldenScripts` 字段是 v0.8 黄金脚本表，覆盖：

- 均线交叉
- RSI/CCI/Williams/Bollinger
- Donchian breakout、volume MA、SAR
- MTF source/MA
- qty_percent、strategy.order、pending、bracket、cancel
- UDF + static for

发布前必跑：

```bash
go test ./pkg/strategy/... ./internal/app/apiserver/servercore ./internal/api/strategy ./internal/strategy ./pkg/backtest/...
npm run test:web
npm run typecheck:web
```

建议补跑：

```bash
go test ./pkg/backtest -run Pine
npm run test:web -- strategyVisualBuilderPine.test.ts
```

通过标准：全量测试通过；黄金脚本全部 `AnalyzeScript OK` 并能完成 requirements planning；既有 Pine backtest 回归无 runtime error；前端不可识别 Pine 行不会回写为 visual snippet，而是给出同步失败诊断。
