# 前端 K 线实时合成说明

本文档只描述前端如何把历史 candles 和 websocket 实时 tick 合成为图表上的 K 线，目的是避免重复踩到“新时间桶没有收盘、而是继续重绘旧桶”的坑。

## 数据流

1. 历史 K 线来自 `/api/v1/market-data/candles/:market/:symbol`。
2. 实时 tick 来自 websocket `/api/v1/ws/live` 的 `market-data.tick` 事件。
3. `apps/web/src/layout/AppShell.vue` 把最新 tick 交给 `applyMarketDataTickEvent`。
4. `apps/web/src/composables/useConsoleData.ts` 负责把 tick 写入 `marketDataSnapshot`，并计算当前桶成交量。
5. 非 tick 实时 tick 还会写入前端实时 candle 缓存，同一周期内更新同一根 candle，跨周期时清除上一根的实时展示时间并新开一根。
6. `apps/web/src/pages/MarketPage.vue` 和 `apps/web/src/components/workspace/LightweightChart.vue` 使用 `overlayRealtimeTickCandle` 把历史 candles 和实时 snapshot 合成为最终图表数据。

## 时间字段约定

- `snapshot.at`
  - 来自后端快照载荷本身。
  - 表示上游行情快照时间。
  - 不能假设它一定和 websocket 消息送达时间同步推进到下一个分钟桶。

- `snapshot.observedAt`
  - 前端用于实时分桶和渲染的统一时间。
  - 如果后端 tick 已提供 `snapshot.observedAt`，前端直接使用。
  - 如果后端没有提供，前端必须回退到外层 websocket 事件的 `event.at`。

核心原则：实时成交量分桶、snapshot 落库、tick 图展示，必须使用同一套 observedAt 逻辑。不要让一条链路看 `event.at`，另一条链路看 `snapshot.at`。

- `candle.at`
  - 前端 candle 的稳定分桶键。
  - 历史 K 线必须保持后端返回的时间，不允许前端统一 `+1` 个周期。
  - 非 tick 实时 K 线必须保存周期起点，例如 `10:50:00`。
  - 不能用当前 tick 时间覆盖它，否则同一周期内每个 tick 都会变成不同 candle，或者让一根 candle 看起来跨越多个周期。

- `candle.displayAt`
  - 图表展示/悬浮时间。
  - 只有活动中的实时非 tick candle 可以显式设置为当前 `observedAt`，例如 `10:50:30`。
  - 历史 candle 和已完成实时 candle 没有 `displayAt` 时，图表必须直接使用 `at`，不能自动按周期右移。

## 分桶规则

- `tick`
  - 一条实时 tick 对应一根图表 candle。
  - 时间直接使用 `observedAt`。

- `1m` 到 `1h`
  - 先把 `observedAt` 截断到对应周期起点。
  - 如果当前周期桶已存在，更新这根 candle。
  - 如果当前周期桶不存在，但只比最后一根历史 candle 新一个合理范围内的桶，则新开一根 candle。
  - 如果历史数据已经过旧，禁止跨很大的时间洞直接补一根“未来 candle”。

- 非 tick 实时叠加显示
  - 当前前端对活动中的非 tick 实时 candle 使用 `observedAt` 作为显示时间。
  - 例子：1m 实时快照落在 `10:50:30`，分桶仍按 `10:50:00` 归属，但图上悬浮时间显示为 `10:50:30`。
  - 5m、15m、1h、1d、1w 遵循同一原则：`at` 是周期起点，`displayAt` 只表达活动实时 tick 的观察时间。
  - 这个显示时间写入 `displayAt`，不改变 `at` 分桶键。
  - 当 `observedAt` 进入下一周期时，前端必须清除上一根活动 candle 的 `displayAt`，让它恢复按 `at` 展示，并新开下一周期 candle。
  - 同一时间桶内连续 tick 的 `open/high/low/close` 需要在前端累计，不能每次只用最新价重算，否则高低点会丢失。

- `1d` / `1w`
  - 未收盘的当日或当周 candle 可以继续更新。
  - 一旦进入新日或新周，必须落到新的周期起点，不能继续复用上一根历史 candle。

## 已知回归点

### 1. 新分钟没有收盘，而是继续重绘旧分钟

触发方式：

- websocket 外层 `event.at` 已经进入新分钟。
- 但 `snapshot.at` 仍停留在旧分钟。
- 前端把 tick 写入 `marketDataSnapshot` 时没有补上 `observedAt`。

结果：

- 成交量分桶可能已经按新分钟计算。
- 图表叠加却仍按旧分钟覆盖上一根 candle。
- 最终表现为“柱子没有收盘，一直在本根 K 线上重绘”。

修复要求：

- 在 `applyMarketDataTickEvent` 中显式写入 `observedAt`。
- 所有实时分桶逻辑统一走 `resolveMarketDataTickObservedAt`。

### 2. 新桶成交量和价格属于不同时间桶

触发方式：

- 成交量分桶用 `event.at`。
- 价格叠加用 `snapshot.at`。

结果：

- 价格已经被画到旧桶。
- 成交量却被记到新桶或相反。

修复要求：

- 成交量和价格必须共享同一个 observedAt。

### 2.5. 实时叠加时间被写成固定周期起点

触发方式：

- 修改实时 overlay 逻辑时，把活动实时 candle 的显示时间重新写成桶起点或下一个周期边界。

结果：

- 前端最新一根重绘 candle 无法反映当前时刻，只会固定停在周期边界标签上。
- 如果同时没有保存同一周期内的高低点累计，图形还会继续出现高低点丢失。

修复要求：

- 保留非 tick 实时叠加使用 `observedAt` 作为活动 candle 显示时间。
- 保留内部实时分桶键仍按真实周期起点计算，显示时间写到 `displayAt`。
- 为当前活动桶持续累计 open/high/low/close，而不是每次只覆盖最新 close。

### 2.6. 历史 candles 被整体加一个周期

触发方式：

- 图表层为了让 1m 悬浮时间看起来像收盘时间，对没有 `displayAt` 的历史 candle 统一返回 `at + period`。

结果：

- 历史 K 线整体右移。
- 最新实时 candle 和历史 candle 在图表时间轴上撞到同一时间点时，会被去重逻辑吞掉一根。
- 问题会影响所有非 tick 周期，不应该只按 1m 局部修补。

修复要求：

- 历史 candle 和已完成 candle 的展示时间一律使用 `at`。
- 只有活动实时 candle 可以显式携带 `displayAt`。
- `resolveKlineCandleDisplayAt` 不能根据 `period` 自动右移。

### 2.7. 用 observedAt 覆盖 candle.at

触发方式：

- 为了让悬浮时间显示当前时刻，直接把 `candle.at` 改成 `observedAt`。

结果：

- `mergeDisplayCandles` 和历史合并会按不同 `at` 把同一分钟 tick 当成不同 candle。
- Market 页可能出现最新 K 线范围从某个整分钟一直延伸到当前秒数，跨分钟也不拆分。

修复要求：

- `at` 只能表示桶起点。
- 当前时刻展示写入 `displayAt`。
- 非 tick 实时 tick 要写入前端 candle 缓存，跨桶时清除旧桶的 `displayAt` 并新开新桶。

### 3. 历史 candles 已过旧，却被实时快照强行接续

触发方式：

- 历史 candles 停在很久以前。
- 实时 snapshot 直接落到当前时间的新桶。

结果：

- 图上会出现一根跨越大时间洞的伪造实时 candle。

修复要求：

- 继续保留 `resolveRealtimeBucketStart` 里的 stale gap 抑制规则。

## 修改 K 线逻辑前的检查单

1. 先确认你改的是历史数据合并、实时 tick 落库，还是图表渲染层，不要混着动。
2. 只要修改实时 tick 相关逻辑，就检查 `snapshot.at` 和 `observedAt` 的语义是否仍然一致。
3. 只要修改分桶逻辑，就检查日线、周线、分钟线、stale gap 这几条边界。
4. 如果你发现一个函数自己在兜底时间，确认别的调用链是不是也在各自兜底，避免再次分叉。

## 回归测试

- `apps/web/tests/kline.test.ts`
  - 覆盖全周期历史 candle 不自动右移、日线、周线、分钟线分桶，以及 stale history gap 抑制。

- `apps/web/tests/useConsoleData.klineRealtime.test.ts`
  - 覆盖 websocket `event.at` 已跨分钟、但 `snapshot.at` 仍停在旧分钟时，前端仍应开启新桶。
  - 覆盖同一周期内多 tick 的 open/high/low/close 累计，以及 1m、5m 跨周期拆分。

## 相关实现文件

- `apps/web/src/charting/kline.ts`
- `apps/web/src/composables/useConsoleData.ts`
- `apps/web/src/pages/MarketPage.vue`
- `apps/web/src/components/workspace/LightweightChart.vue`