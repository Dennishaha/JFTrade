# JFTrade Pine v6 v0.8 发布收口说明

## 当前定位

v0.8 锁定的是 JFTrade 可执行 Pine v6 策略子集，不追 TradingView Pine v6 全量语言或完整 broker emulator。主路径为 `sourceFormat=pine-v6` + `runtime=pine-go-plan`。

已纳入本次收口的能力包括：

- Pine parser/runtime：`strategy(...)`、`if/else`、`var`/`varip`/`const` 兼容、重赋值、历史引用、单表达式 UDF、静态 for、input 默认值、barstate/session/time/math/string。
- 指标与 MTF：source-aware MA/RSI/CCI/stdev/rolling window、Donchian、Williams %R、Bollinger、SAR、MTF source 与 MTF MA。
- 订单与退出：`qty_percent`、`strategy.order`、`close_all`、pending stop/limit、bracket exit、`cancel`/`cancel_all`。
- 集成与前端：`strategy.pine_spec` 输出 `supportMatrix`、`compatibilityLayers`、`unsupportedPatterns`、`goldenScripts`；流程图无法标准化的 Pine 行保留为 `pineSnippet`。

## 明确不支持

- `array.*`、`map.*`、`matrix.*`、`library/import`、Pine type/method、多语句或递归 UDF。
- 动态 for、`break`/`continue`、任意表达式 MTF、`lookahead_on`/`gaps_on`。
- OCA、partial fill、intrabar broker emulator、entry/order stop-limit 组合、exit trail 与 bracket 混用。
- TradingView 视觉对象只做 warning/no-op 兼容，不参与执行。

## 迁移注意

- 旧 `codeBlock` 只作为历史 visual model 只读兼容；新的 Pine 反解兜底统一生成 `pineSnippet`。
- 旧合并式 `technicalIndicator` 块保留导入解析；新建和模板路径应使用 `getTechnicalIndicator` + `technicalIndicatorCondition`。
- close/默认源指标继续保留 legacy key，例如 `ma:SMA:20`、`rsi:14`、`cci:20`；volume、hlc3 等新增 source 必须走 source-aware key，避免指标串线。
- 历史非 Pine source/runtime 后续只做默认 Pine 替换或只读展示，不再扩展旧 runtime。

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
go test ./pkg/strategy/... ./pkg/jftradeapi/... ./pkg/backtest/...
npm --workspace @jftrade/web run test
npm run typecheck
```

建议补跑：

```bash
go test ./pkg/backtest -run Pine
npm --workspace @jftrade/web run test -- strategyVisualBuilderPine.test.ts
```

通过标准：全量测试通过；黄金脚本全部 `AnalyzeScript OK` 并能完成 requirements planning；既有 Pine backtest 回归无 runtime error；前端不可识别 Pine 行保留为 `pineSnippet`。
