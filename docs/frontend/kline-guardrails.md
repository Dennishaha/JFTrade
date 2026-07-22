# K 线防跑偏约束

本文收敛最容易让后续 AI 跑偏的规则。改 K 线前先过一遍，不要直接从 UI 现象倒推出错误修法。

## 10 条高优先级约束

1. `candle.at` 只能表示桶起点，绝不能被 `observedAt` 当前秒数覆盖
2. `candle.displayAt` 只用于展示，分钟/小时 K 的活动 candle 应显示桶结束点
3. `snapshot.observedAt` 是价格和成交量分桶的统一时间来源
4. `snapshot.volume` 是累计成交量，不是当前桶量柱；实时状态机只接受顶层 `cumulativeVolume` / `volumeDelta`
5. 首屏若已返回当前桶 candle，必须用该 candle 的 OHLCV 初始化当前桶状态
6. 同一桶内必须累计 high/low，不能每次仅覆盖最新 close
7. Futu intraday 历史时间语义是收盘标签，后端需先归一化为桶起点
8. 日线/周线的跨周期规则与分钟线不同，不能用同一套偷懒逻辑
9. 历史数据过旧时，必须保留 stale gap 抑制，不能补伪造实时 candle
10. 改动任一时间兜底逻辑时，要检查其他链路是否也在各自兜底，避免再次分叉

图表边界的 `RealtimeKlineSnapshot` 故意不具名暴露 `volume`。缺少可信 `barVolume` 时，已有桶保留自身成交量，新桶使用 `0`；禁止恢复任何 `snapshot.volume` fallback。

## 6 个高频回归点

### 新分钟没收盘，继续重绘旧分钟

通常是 `event.at` 已进新桶，但 `snapshot.observedAt` 没有正确写回，导致实时分桶仍覆盖旧 candle。

### 价格和成交量属于不同时间桶

通常是一条链路用 `event.at`，另一条链路用 `snapshot.at` 或其他时间字段，最终 price 和 volume 不同步。

### 活动 candle 被画在当前秒数

通常是把 `displayAt` 写成当前 `observedAt` 秒数，而不是桶结束点。

### 历史 candle.at 被拿去当展示时间改写

这会破坏桶身份，后续历史与实时合并会错位，出现吞 candle 或重复 candle。

### 页面中途打开时，用上一根 close 猜当前桶

这只适合最低级别兜底，不是正确语义。正确做法是后端返回当前未收盘桶 seed，前端只继续累计。

### 首屏把 snapshot.volume 当作当前量柱

这是累计量和分桶量的概念混淆，典型表现是首屏量柱异常大，第一笔 tick 后又恢复。

## 改动前检查单

1. 这次改的是后端历史返回、前端实时落库，还是图表渲染层
2. 价格和成交量是否仍共享同一套 `observedAt`
3. `at` 与 `displayAt` 的职责是否仍然分离
4. 首屏当前桶 seed 是否仍优先来自历史当前桶，而不是前端猜值
5. 是否保留了 stale gap 抑制和跨日/跨周边界
6. live event 是否仍同时区分 `cumulativeVolume` 与 `volumeDelta`，且没有把 `snapshot.volume` 接回量柱计算

## 相关实现

- [../apps/web/src/composables/marketDataRealtime.ts](../apps/web/src/composables/marketDataRealtime.ts)
- [../apps/web/src/composables/marketDataQuery.ts](../apps/web/src/composables/marketDataQuery.ts)
- [../apps/web/src/charting/kline.ts](../apps/web/src/charting/kline.ts)
- [../internal/api/marketdata/routes.go](../internal/api/marketdata/routes.go)
- [../internal/marketdata/service.go](../internal/marketdata/service.go)
