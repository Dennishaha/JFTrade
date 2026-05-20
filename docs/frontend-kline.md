# 前端 K 线实时合成说明

本文档只描述前端如何把历史 candles 和 websocket 实时 tick 合成为图表上的 K 线，目的是避免重复踩到“新时间桶没有收盘、而是继续重绘旧桶”的坑。

## 数据流

1. 历史 K 线来自 `/api/v1/market-data/candles/:market/:symbol`。后端在 `pkg/futu` 里把 Futu intraday 历史 K 的收盘标签时间减去一个周期，归一化为内部桶起点；当查询窗口覆盖当前时间桶时，还会额外调用 Futu `Qot_GetKL` 把当前未收盘 K 线并入返回结果。
2. 实时 tick 来自 websocket `/api/v1/ws/live` 的 `market-data.tick` 事件。
3. `apps/web/src/layout/AppShell.vue` 把最新 tick 交给 `applyMarketDataTickEvent`。
4. `apps/web/src/composables/useConsoleData.ts` 负责把 tick 写入 `marketDataSnapshot`，并计算当前桶成交量。
5. 非 tick 实时 tick 还会写入前端实时 candle 缓存，同一周期内更新同一根 candle，跨周期时清除上一根的实时展示时间并新开一根。
6. `apps/web/src/pages/MarketPage.vue` 和 `apps/web/src/components/workspace/LightweightChart.vue` 使用 `overlayRealtimeTickCandle` 把历史 candles 和实时 snapshot 合成为最终图表数据。

补充原则：前端实时叠加只负责在“已经拿到当前桶真实 seed”的前提下继续累计当前桶后续 tick。页面如果在当前桶中途打开，当前桶的 open/high/low 不能再从上一根 close 猜出来，否则一定会和稍后回来的真实历史 K 线对不上。

## 时间字段约定

- `snapshot.at`
  - 来自后端快照载荷本身。
  - 表示上游行情快照时间。
  - 不能假设它一定和 websocket 消息送达时间同步推进到下一个分钟桶。

- `snapshot.observedAt`
  - 前端用于实时分桶和渲染的统一时间。
  - 如果后端 tick 已提供 `snapshot.observedAt`，前端直接使用。
  - 如果后端没有提供，前端必须回退到外层 websocket 事件的 `event.at`。

- `snapshot.volume`
  - 来自快照接口的累计成交量语义，不是当前 K 线时间桶的成交量。
  - 这个字段可以用于行情概览，但不能直接拿来画分钟/小时 K 线附图里的最新量柱。

- `snapshot.barVolume`
  - 前端当前桶成交量字段。
  - websocket tick 到来后，由实时状态机按当前桶增量计算。
  - 页面首屏加载时，如果历史 candles 已经带回当前未收盘桶，前端必须先用这根当前桶 candle 的 volume 把 `barVolume` seed 出来，不能回退到 `snapshot.volume`。

核心原则：实时成交量分桶、snapshot 落库、tick 图展示，必须使用同一套 observedAt 逻辑。不要让一条链路看 `event.at`，另一条链路看 `snapshot.at`。

- `candle.at`
  - 前端 candle 的稳定分桶键。
  - 历史 K 线必须使用后端归一化后的桶起点。Futu intraday 历史 K 的原始标签如果是 `10:55:00`，后端返回给前端的 `at` 应是 `10:54:00`。
  - 非 tick 实时 K 线必须保存周期起点，例如 `10:50:00`。
  - 不能用当前 tick 时间覆盖它，否则同一周期内每个 tick 都会变成不同 candle，或者让一根 candle 看起来跨越多个周期。

- `candle.displayAt`
  - 前端展示时间，可以作为图表 x 轴时间，但不能作为分桶键。
  - 活动中的实时非 tick 分钟/小时 K 线必须显式设置为桶结束点，而不是当前 `observedAt` 秒数。
  - 例子：当前 `10:55:30` 属于 `10:55:00-10:56:00` 这根 1m K，`at` 是 `10:55:00`，`displayAt` 是 `10:56:00`。
  - 已收盘的分钟/小时 K 线如果没有显式 `displayAt`，图表展示时间从自己的 `at + period` 派生；这只是渲染坐标，不能改写 `at`。

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
  - 分桶用 `observedAt` 截断后的周期起点，展示用桶结束点。
  - 例子：1m 实时快照落在 `10:55:30`，分桶按 `10:55:00` 归属，图上显示为 `10:56:00`。
  - 5m、15m、1h 遵循同一原则：`at` 是周期起点，展示时间是周期结束点。
  - 这个展示时间写入 `displayAt`，不改变 `at` 分桶键。
  - 当 `observedAt` 进入下一周期时，前端必须保留上一根 candle 自己的桶结束展示时间，并新开下一周期 candle。
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

### 2.5. 实时叠加时间被写成当前 observedAt 秒数

触发方式：

- 修改实时 overlay 逻辑时，把活动实时 candle 的显示时间写成当前 tick 的 `observedAt`。

结果：

- 最新 1m K 在 `10:55:30` 被画到 `10:55:30`，上一根在 `10:55:00`，导致最新 K 视觉上超过它应该占用的周期。
- 如果同时没有保存同一周期内的高低点累计，图形还会继续出现高低点丢失。

修复要求：

- 保留内部实时分桶键仍按真实周期起点计算。
- 活动 candle 的 `displayAt` 必须是桶结束点，例如 `10:55:30` 所在 1m K 显示为 `10:56:00`。
- 为当前活动桶持续累计 open/high/low/close，而不是每次只覆盖最新 close。

### 2.6. 改写历史 candle.at 作为展示时间

触发方式：

- 为了让历史 K 显示在收盘点，把历史 candle 的 `at` 存储值改成 `at + period`。

结果：

- 历史数据身份被改写，后续实时合并找不到正确的桶。
- 最新实时 candle 可能和历史 candle 在内部键上错位，出现吞蜡烛或重复蜡烛。
- 问题会影响所有分钟/小时 K，不应该只按 1m 局部修补。

修复要求：

- `at` 只能保存原始桶时间。
- 分钟/小时 K 的收盘点展示只能在 `resolveKlineCandleDisplayAt` 这类展示函数里派生。
- 实时活动 candle 可以显式携带桶结束点 `displayAt`，但不能携带当前秒数。

### 2.6.1. Futu 历史 intraday 时间没有减周期

触发方式：

- 后端把 Futu 历史 K 的 `time_key` / timestamp 直接当成内部桶起点。
- 对 intraday K 来说，这个时间实际用于收盘标签展示。

结果：

- 历史最后一根 K 的展示时间被前端再加一个周期。
- 最新实时 K 的桶结束展示时间和历史最后一根撞到同一个图表时间。
- 图表层按展示时间去重时，历史最后一根可能被吞掉。

修复要求：

- 在 `pkg/futu` 入口把 intraday 历史 K 的标签时间减去一个周期，作为 `types.KLine.StartTime`。
- 日线、周线等日级及以上 K 线不做这个前移。
- 前端只根据归一化后的 `at` 派生展示时间，不再猜测 Futu 原始时间语义。

### 2.7. 用 observedAt 覆盖 candle.at

触发方式：

- 为了让悬浮时间显示当前时刻，直接把 `candle.at` 改成 `observedAt`。

结果：

- `mergeDisplayCandles` 和历史合并会按不同 `at` 把同一分钟 tick 当成不同 candle。
- Market 页可能出现最新 K 线范围从某个整分钟一直延伸到当前秒数，跨分钟也不拆分。

修复要求：

- `at` 只能表示桶起点。
- 桶结束展示写入 `displayAt`。
- 非 tick 实时 tick 要写入前端 candle 缓存，跨桶时保留旧桶自己的桶结束展示时间并新开新桶。

### 2.8. 用 observedAt 当前秒数作为图表 x 轴坐标

触发方式：

- 图表适配层排序或生成 lightweight-charts 数据时，把活动 candle 的 `displayAt` 写成 `observedAt` 当前秒数。

结果：

- 上一根完成 1m candle 显示 `10:55:00`。
- 最新活动 1m candle 被画到 `10:55:30`，而不是应显示的 `10:56:00`。
- 最新 candle 看起来超过了它应该占用的周期。

修复要求：

- candlestick、volume、MACD、KDJ 的数据点时间可以使用展示时间，但展示时间必须是桶结束点。
- `displayAt` 不能保存当前秒数；内部排序合并仍必须以 `at` 作为业务身份。

### 2.9. 页面中途打开时，用上一根 close 猜当前桶 open/high/low

触发方式：

- 历史接口只返回到上一根已收盘 K 线。
- 页面在当前桶中途打开，前端本地还没有见过当前桶前半段 tick。
- `useConsoleData.ts` 用 `lastHistoricalCandle.close` 作为当前桶 open，再用页面打开后看到的 tick 去累计 high/low。

结果：

- 当前未收盘 K 线在前端看起来可以动，但 open/high/low/close 只反映“页面打开后”观察到的片段。
- 一旦后端后续把真实当前桶或下一次查询结果返回，前端自绘 K 线价格就会和历史 K 线对不上。

修复要求：

- 当前查询窗口覆盖现在时，后端必须把 `Qot_GetKL` 返回的当前未收盘 K 线一并返回给前端。
- 前端实时状态机如果发现当前桶已经存在，必须沿用这个桶的 open/high/low/volume 作为 seed，只继续更新 close 和后续增量。
- `lastHistoricalCandle.close` 只能作为纯前端降级兜底，不能再当作正确语义依赖。

### 2.10. 首屏把 snapshot.volume 当成当前量柱

触发方式：

- 页面首屏并行请求 snapshot 和 candles。
- candles 已经返回当前未收盘桶，但 `marketDataSnapshot` 还没有实时状态机算出的 `barVolume`。
- 图表叠加层回退使用 `snapshot.volume`，而这个字段实际是当日累计成交量。

结果：

- 最新一根 K 线的量柱在页面刚加载时异常巨大。
- 等第一笔 tick 到来并算出 `barVolume` 后，量柱又恢复正常，所以问题会表现成“首屏闪一下就好了”。

修复要求：

- 在 `loadMarketDataQuery` 首屏合成 `marketDataSnapshot` 时，如果当前桶 candle 已存在，先把这根 candle 的 open/high/low/volume seed 到 `barOpen/barHigh/barLow/barVolume`。
- 非 tick K 线附图量柱只能用 `barVolume` 或当前桶 candle.volume，不能直接用累计快照量。

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
  - 覆盖分钟/小时 K 展示桶结束点、历史最后一根不被实时新桶吞掉、日线、周线、分钟线分桶，以及 stale history gap 抑制。

- `apps/web/tests/KlineChart.test.ts`
  - 覆盖分钟 K 图表 series 时间使用桶结束展示时间。

- `apps/web/tests/useConsoleData.klineRealtime.test.ts`
  - 覆盖 websocket `event.at` 已跨分钟、但 `snapshot.at` 仍停在旧分钟时，前端仍应开启新桶。
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