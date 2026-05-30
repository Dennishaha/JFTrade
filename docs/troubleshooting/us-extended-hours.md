# 美股盘外时段显示

本文回答一个问题：为什么盘前、盘后、夜盘卡片看起来和常规盘不同，以及出问题时先查哪几个字段。

## 当前约定

- 盘中：主卡片显示实时价
- 非盘中：主卡片显示最近一次 regular session 收盘价
- 盘外涨跌幅基准：`lastClosePrice`
- 盘前、盘后、夜盘的活动卡片优先使用 live tick；HTTP 快照只作兜底和补全
- 回测页只在“当前标的属于 US 且当前周期支持 extended replay”时展示扩展时段开关；不生效时直接隐藏，不再显示禁用态切换
- 回测页的预热 K 线预览会按当前选中的标的、周期和扩展时段口径一起推导，避免 UI 预览与实际回测 warmup 不一致
- 回测结果页里的时间统一按当前浏览器本地时间展示，格式固定为 `YYYY-MM-DD HH:mm:ss`
- 如果回测使用前复权或后复权，结果图里的价格来自该复权口径下的已闭合历史 K 线；不要直接和实时 snapshot 的 `afterMarket` / `overnight` 报价比较，后者通常是不复权的最新盘后成交

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

## 回测侧的日历边界

当前 backtest/store/indicator 的时段语义可以处理两类情况，但精度边界不同：

- 周末：可以处理。US session 分类在运行时会把周六直接判为 closed，周日只在 20:00 ET 之后进入 overnight；regular session window 也会直接跳过周六周日。所以本地 SQLite 如果周末没有 bar，replay 和 higher-period aggregation 会自然跨过去，不会因为周末缺口报错。
- 不定休市、法定节假日、半日市：当前只能“容忍没有 bar”，还不能“识别这一天本来就不开市”。也就是说，如果上游同步结果里这一天没有任何 bar，回测会正常跳过；但当前代码没有交易所 holiday calendar，US session 判定仍主要是 weekday + clock time 规则，warmup 估算也仍按每周 5 个 trading day、每月 20 个 trading day 估算。

这意味着当前实现的实际行为是：

- 对完整休市日：只要 SQLite 中没有这天的 bar，replay 本身通常是安全的。
- 对需要主动判断“今天是不是 holiday”的路径：当前并不严格准确，`ClassifyMarketSession` / trading-period label 不能单靠自身识别 weekday holiday。
- 对半日市、临时停市：当前也没有单独的 session 窗口模型，默认仍按常规时段或 extended 时段固定边界处理。

如果后面要把 holiday/half-day 也做成语义正确，而不只是 replay 容错，入口应放在 `pkg/futu/trading_profile.go` 和 `pkg/futu/quote_snapshot.go` 这一层，引入交易所 calendar，再把 warmup 和 aggregation 共用到同一套日历定义。

## 需要避免的旧表述

- 不要把“盘前盘后卡片跟 HTTP 刷新”直接判定为 bug
- 不要把“最近盘中收盘”和“上个交易日收盘”混成同一个概念