# 前端行情与 K 线入口

本文是入口页，只回答三件事：

- 当前前端行情链路怎么走
- 改 K 线逻辑前必须记住哪些不变量
- 细节应该去看哪篇专题，而不是继续把所有边界条件堆在一页里

## 当前数据链路

```text
历史 candles: /api/v1/market-data/candles/*
实时 tick:    /api/v1/stream/live
快照:         /api/v1/market-data/snapshots/*

apps/web
  -> AppShell / MarketPage
  -> marketDataQuery / marketDataRealtime
  -> charting/kline.ts

当前图表指标层除了主图 MA/EMA 和副图 Volume/MACD/KDJ 外，还支持副图 ATR、CCI、Williams %R；这些展示指标只在前端图表层做可视化计算，不参与策略运行时判定。
```

前端只负责在“后端已给出正确当前桶 seed”的前提下继续做实时累计，不负责猜测当前桶的真实 open/high/low。

## 必须牢记的不变量

1. `candle.at` 是分桶键，只能表示桶起点，不能被当前秒数覆盖
2. `candle.displayAt` 是展示时间，分钟/小时 K 应显示桶结束点，而不是 `observedAt` 当前秒数
3. `snapshot.observedAt` 是实时分桶统一时间参考，价格和成交量必须共用它
4. `snapshot.volume` 是累计量，不是当前桶量柱；当前桶量柱应来自 `barVolume` 或当前桶 candle.volume
5. 首屏若已拿到当前未收盘桶，前端必须沿用该桶的 open/high/low/volume 作为 seed
6. 同一周期内高低点必须累计，不能每次只用最新价覆盖
7. 历史数据过旧时，不能用实时快照强行补一根跨大时间洞的伪 candle

## 常见误判

- 不要为了显示当前时刻把 `candle.at` 改成 `observedAt`
- 不要把活动 candle 的 `displayAt` 写成当前秒数
- 不要用上一根 close 猜当前桶 open/high/low
- 不要把 `snapshot.volume` 直接当作分钟量柱

## 专题导航

- [frontend/market-data-flow.md](frontend/market-data-flow.md)：前端与 sidecar 的行情数据流、模块职责、时间字段流向
- [frontend/kline-guardrails.md](frontend/kline-guardrails.md)：K 线不变量、回归风险与改动前检查单

## 代码锚点

- [../apps/web/src/layout/AppShell.vue](../apps/web/src/layout/AppShell.vue)
- [../apps/web/src/composables/marketDataQuery.ts](../apps/web/src/composables/marketDataQuery.ts)
- [../apps/web/src/composables/marketDataRealtime.ts](../apps/web/src/composables/marketDataRealtime.ts)
- [../apps/web/src/charting/kline.ts](../apps/web/src/charting/kline.ts)
- [../pkg/jftradeapi/market_routes.go](../pkg/jftradeapi/market_routes.go)
- [../pkg/jftradeapi/market_live.go](../pkg/jftradeapi/market_live.go)

- `apps/web/tests/kline.test.ts`
  - 覆盖分钟/小时 K 展示桶结束点、历史最后一根不被实时新桶吞掉、日线、周线、分钟线分桶，以及 stale history gap 抑制。

- `apps/web/tests/KlineChart.test.ts`
  - 覆盖分钟 K 图表 series 时间使用桶结束展示时间。

- `apps/web/tests/useConsoleData.klineRealtime.test.ts`
  - 覆盖实时事件 `event.at` 已跨分钟、但 `snapshot.at` 仍停在旧分钟时，前端仍应开启新桶。
  - 覆盖同一周期内多 tick 的 open/high/low/close 累计，以及 1m、5m 跨周期拆分。
  - 覆盖历史查询已经带回当前桶时，后续实时 tick 必须复用该当前桶的真实 open/high/low，而不是回退到上一根 close。
  - 覆盖首屏并行加载时，最新量柱必须复用当前桶 candle.volume，而不是 snapshot 累计 volume。

- `pkg/futu/exchange_test.go`
  - 覆盖 Futu intraday 历史 K 标签时间减一个周期成为内部桶起点，以及日线不前移。
  - 覆盖查询窗口命中当前时间桶时，`Qot_GetKL` 当前未收盘 K 线会并入历史结果，供前端直接复用当前桶真实 OHLC。

## 相关实现文件

- `apps/web/src/charting/kline.ts`
- `apps/web/src/composables/useConsoleData.ts`
- `apps/web/src/pages/MarketPage.vue`
- `apps/web/src/components/workspace/LightweightChart.vue`
- `pkg/futu/exchange.go`
