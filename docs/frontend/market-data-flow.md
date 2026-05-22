# 前端行情数据流

本文回答两个问题：

- 前端模块之间如何分工
- 历史 candles、snapshot 和实时 tick 分别从哪里进入、在哪里合并

## 入口与依赖
- 历史 K 线：`/api/v1/market-data/candles/*`
- 快照：`/api/v1/market-data/snapshots/*`
- 证券基础信息 / typed security details：`/api/v1/market-data/securities/*`
- 实时 tick：`/api/v1/ws/live`

这些接口都由 sidecar 提供，不是 bbgo 原生 `/api/*`。

`snapshot`、`security details` 以及 candles 查询响应的共享 DTO 当前统一收敛在 `packages/ui-contracts`，前端 composable 主要负责状态编排，不再重复定义 rich security details 结构。

## 主要模块

| 模块 | 作用 |
| --- | --- |
| `AppShell.vue` | 接收 live websocket 事件并分发到前端状态层 |
| `marketDataQuery.ts` | 负责页面查询、订阅与初始装载 |
| `marketDataRealtime.ts` | 负责实时状态机，是分桶与累计的主编排层 |
| `marketDataRealtime*` 系列 | 分别处理 bucket、bar price、bar volume 等局部状态 |
| `WatchlistPanel.vue` | 消费 snapshot + security details，渲染价格卡片与证券详情区 |
| `charting/kline.ts` | 把历史与实时叠加为最终图表数据 |

## 合并顺序

```text
页面打开
  -> 请求 candles + snapshot + security details
  -> 初始化当前图表与当前桶 seed
  -> 建立 /api/v1/ws/live
  -> tick 到来后进入 realtime controller
  -> overlayRealtimeTickCandle 产出最终图表序列
```

`security details` 当前主要供 `WatchlistPanel.vue` 展示通用证券摘要，以及 equity / option / warrant / future / trust / index / plate 等 typed block。

当前价格面板会在初次查询后维持约 1 秒一次的后台轮询，同时刷新 `snapshot` 与 `security details`；实时 tick 仍优先驱动价格与当前 candle，后台轮询主要用于补齐 typed details 和时段切换等非 tick 字段。

## 时间字段流向

| 字段 | 当前职责 |
| --- | --- |
| `snapshot.at` | 上游快照时间，不能直接当唯一分桶依据 |
| `snapshot.observedAt` | 前端实时分桶统一时间参考 |
| `candle.at` | 业务分桶键 |
| `candle.displayAt` | 图表展示时间 |

## 维护建议

- 改数据请求逻辑，先看 `marketDataQuery.ts`
- 改 security details 结构或前后端字段契约，先看 `packages/ui-contracts`
- 改实时状态机，先看 `marketDataRealtime.ts`
- 改价格面板的 typed block 展示，先看 `WatchlistPanel.vue`
- 改最终渲染时间或 candle 叠加，先看 `charting/kline.ts`

如果你准备同时改时间字段和图表展示，先同步阅读 [kline-guardrails.md](kline-guardrails.md)。