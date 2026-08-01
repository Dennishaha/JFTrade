# 美股盘外时段显示

本文回答一个问题：为什么盘前、盘后、夜盘卡片看起来和常规盘不同，以及出问题时先查哪几个字段。

## 当前约定

- 盘中：主卡片显示实时价
- 非盘中：主卡片显示最近一次 regular session 收盘价
- 盘外涨跌幅基准：`lastClosePrice`
- 盘前、盘后、夜盘的活动卡片优先使用 live tick；HTTP 快照只作兜底和补全
- `quoteTime` 是 Provider 实际报价时间；`sessionStartAt` / `sessionEndAt` 是交易日历计划边界，前端不得互相替代
- 行情卡片按交易所时区显示实际报价时间和计划截止时间，浏览器本地时间只作为提示信息
- 回测页只在“当前标的属于 US 且当前周期支持 extended replay”时展示扩展时段开关；不生效时直接隐藏，不再显示禁用态切换
- 回测页的预热 K 线预览会按当前选中的标的、周期和扩展时段口径一起推导，避免 UI 预览与实际回测 warmup 不一致
- 回测结果页里的时间统一按当前浏览器本地时间展示，格式固定为 `YYYY-MM-DD HH:mm:ss`
- 如果回测使用前复权或后复权，结果图里的价格来自该复权口径下的已闭合历史 K 线；不要直接和实时 snapshot 的 `afterMarket` / `overnight` 报价比较，后者通常是不复权的最新盘后成交

## session 判定优先级

当前 session 由 Go 进程中生效的 `pkg/market/calendar` 解析器统一判定。Yahoo 的 `marketState` 和 Python helper 固定时钟窗口都不是 session 事实来源；Futu/Yahoo 扩展行情块只有在对应日历窗口内才作为盘外行情展示。

## 优先检查的字段

- `lastClosePrice`
- `preMarket`
- `afterMarket`
- `overnight`
- `session`
- `quoteTime`
- `tradingDate`
- `exchangeTimezone`
- `sessionStartAt` / `sessionEndAt`

如果这些字段都不存在，再往前查 sidecar 的 snapshot 生成链路。

## 常见异常

### 盘外价格更新慢

先查 live tick 是否正常，再看 HTTP 快照。正常预期是 live tick 优先，HTTP 只是兜底。

### 涨跌幅基准不对

先查 `lastClosePrice`，不要直接拿当前 snapshot 的 `price` 或前一个可见 bar 代替。

### 节假日或半日市 session 错误

优先检查 `/api/v1/system/exchange-calendars/status` 的有效来源和覆盖范围，再核对扩展行情的 `tradingDate` 与 `sessionEndAt`。早收市日的盘后截止会随日历调整，不固定为 20:00 ET。

## 回测侧的日历边界

当前 session 分类、warmup 和 higher-period aggregation 共用 `pkg/market/calendar` 的日历解析器：

- 内置规则覆盖周末、已知完整休市日和美股半日市等基础日历。
- sidecar 的 `internal/exchangecalendar.Manager` 按“人工覆盖 -> 已启用远端快照 -> 内置规则兜底”的顺序解析，并在服务装配时注入 `pkg/market`。
- `pkg/market/session.go` 的 session、regular window 和 trading-period 计算都使用同一解析器，回测与指标聚合不再各自维护工作日假设。

排查日历问题时先看 `/api/v1/system/exchange-calendars/status` 返回的 `effectiveSource`、`effectiveMode` 和覆盖范围。远端数据不可用但允许 fallback 时会继续使用内置规则；超出人工、远端和内置规则覆盖范围的特殊休市，以及临时停牌，仍不能仅靠计划日历推断。

## 需要避免的旧表述

- 不要把“盘前盘后卡片跟 HTTP 刷新”直接判定为 bug
- 不要把“最近盘中收盘”和“上个交易日收盘”混成同一个概念
