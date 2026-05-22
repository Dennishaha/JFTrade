# 美股盘外时段显示

本文回答一个问题：为什么盘前、盘后、夜盘卡片看起来和常规盘不同，以及出问题时先查哪几个字段。

## 当前约定

- 盘中：主卡片显示实时价
- 非盘中：主卡片显示最近一次 regular session 收盘价
- 盘外涨跌幅基准：`lastClosePrice`
- 盘前、盘后、夜盘的活动卡片优先使用 live tick；HTTP 快照只作兜底和补全

## session 判定优先级

当前优先顺序是：

1. Futu 扩展行情块
2. 市场时间规则
3. 最后才是本地时间兜底

所以排查时不要先假设“前端只按本地时钟猜 session”。

## 优先检查的字段

- `lastClosePrice`
- `preMarket`
- `afterMarket`
- `overnight`
- `session`

如果这些字段都不存在，再往前查 sidecar 的 snapshot 生成链路。

## 常见异常

### 盘外价格更新慢

先查 live tick 是否正常，再看 HTTP 快照。正常预期是 live tick 优先，HTTP 只是兜底。

### 涨跌幅基准不对

先查 `lastClosePrice`，不要直接拿当前 snapshot 的 `price` 或前一个可见 bar 代替。

### 节假日或半日市 session 错误

优先检查扩展行情块是否已返回，而不是直接按本地时间判断。

## 需要避免的旧表述

- 不要把“盘前盘后卡片跟 HTTP 刷新”直接判定为 bug
- 不要把“最近盘中收盘”和“上个交易日收盘”混成同一个概念