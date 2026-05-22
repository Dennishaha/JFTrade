# 前端行情数据流

本文回答两个问题：

- 前端模块之间如何分工
- 历史 candles、snapshot 和实时 tick 分别从哪里进入、在哪里合并

## 入口与依赖

前端当前主要依赖三类数据：

- 历史 K 线：`/api/v1/market-data/candles/*`
- 快照：`/api/v1/market-data/snapshots/*`
- 实时 tick：`/api/v1/ws/live`

这些接口都由 sidecar 提供，不是 bbgo 原生 `/api/*`。

## 主要模块

| 模块 | 作用 |
| --- | --- |
| `AppShell.vue` | 接收 live websocket 事件并分发到前端状态层 |
| `marketDataQuery.ts` | 负责页面查询、订阅与初始装载 |
| `marketDataRealtime.ts` | 负责实时状态机，是分桶与累计的主编排层 |
| `marketDataRealtime*` 系列 | 分别处理 bucket、bar price、bar volume 等局部状态 |
| `charting/kline.ts` | 把历史与实时叠加为最终图表数据 |

## 合并顺序

```text
页面打开
  -> 请求 candles + snapshot
  -> 初始化当前图表与当前桶 seed
  -> 建立 /api/v1/ws/live
  -> tick 到来后进入 realtime controller
  -> overlayRealtimeTickCandle 产出最终图表序列
```

## 时间字段流向

| 字段 | 当前职责 |
| --- | --- |
| `snapshot.at` | 上游快照时间，不能直接当唯一分桶依据 |
| `snapshot.observedAt` | 前端实时分桶统一时间参考 |
| `candle.at` | 业务分桶键 |
| `candle.displayAt` | 图表展示时间 |

## 维护建议

- 改数据请求逻辑，先看 `marketDataQuery.ts`
- 改实时状态机，先看 `marketDataRealtime.ts`
- 改最终渲染时间或 candle 叠加，先看 `charting/kline.ts`

如果你准备同时改时间字段和图表展示，先同步阅读 [kline-guardrails.md](kline-guardrails.md)。