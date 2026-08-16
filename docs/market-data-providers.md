# 行情数据源

JFTrade 的行情查询与交易执行是两个独立边界。运行时提供 Futu OpenD、Yahoo Finance（`yfinance`）和 AKShare（`akshare`）三个行情 Provider；首页/研究页的“行情提供者”菜单默认使用 yfinance。账户、持仓、订单与真实下单仍只走已配置的 Futu OpenD broker。

新安装默认使用 Yahoo Finance。yfinance 与 AKShare 都适合美股、港股和沪深的证券搜索、延迟快照与历史 K 线分析，不应当作实时交易报价。已有明确选择的 `futu`、`yfinance` 或 `akshare` 会保留；任何 Python 上游失败都会作为当前 Provider 的结构化错误返回，不会静默回退到 Yahoo 或 Futu。

## 能力对比

| 能力 | Futu OpenD | Yahoo Finance（`yfinance`） | AKShare（`akshare`） |
| --- | --- | --- | --- |
| 支持市场 | 由 OpenD 权限和 JFTrade 映射决定 | `US`、`HK`、`SH`、`SZ` | `US`、`HK`、`SH`、`SZ` |
| 证券搜索与详情 | 支持 | 支持 | 支持股票、沪深 ETF 和限定指数目录 |
| 行情快照 | 支持，时效取决于行情权限 | 延迟 HTTP，按需轮询 | 延迟 HTTP，按需轮询；批量目录快照最多 100 个标的 |
| 历史 K 线 | 支持 | 八个全局周期 | 品种级 `supportedPeriods` 为权威；美港指数仅 `1d/1w/1mo` |
| 复权历史 K 线 | 支持 `none/forward/backward` | 支持 `none/forward`（`forward` 走 Yahoo auto_adjust）；`backward` 返回 400，非法值返回 400 `unsupported_adjustment` | 支持 `none/forward/backward`（qfq/hfq），仅沪深股票与沪深 ETF 的 `1d/1w/1mo`；指数与分钟周期仅 `none` |
| 回测历史同步 | 支持 `none/forward/backward` | 支持 `none/forward`；`1m` 7 天、`5m/15m/30m` 60 天、`1h` 730 天 | 支持 `none/forward/backward`；`1m` 5 天，美股全部分钟周期 5 天 |
| 基本面字段 | 不支持 | 支持 market_cap、trailing_pe、shares_outstanding | 支持 market_cap、trailing_pe（东财 f9 动态市盈率，非 TTM）、shares_outstanding；沪深 A 股总股本经 `stock_individual_info_em` 补全，24 小时缓存，失败静默降级为仅现货字段。sidecar 还返回 price_to_book（f23），但 Go 侧当前不投影进 SecurityDetails |
| 快照买一/卖一 | 支持 | 取决于 Yahoo 上游字段，缺失保持 `null` | 支持（东财买一/卖一）；未服务市场保持 `null` |
| 新闻与公司行动 | 不支持 | 支持；公开 API `GET /api/v1/market-data/news/{market}/{symbol}` 与 `/api/v1/market-data/corporate-actions/{market}/{symbol}` | 支持，新闻与分红/送转仅覆盖沪深；同一公开 API，美港返回 400 |
| 指数成分股 | 不支持 | 不支持 | 支持中证指数（如 `SH.000300`）与沪深交易所指数（如 `SH.000001`）；仅经 assistant 工具 `market.index_constituents` 提供，无公开 HTTP API |
| 实时推流 | 支持 | 不支持 | 不支持 |
| Level 2 盘口 | 取决于权限 | 不支持 | 不支持 |
| 盘前盘后 | 支持 | 美股由 Yahoo 实际报价决定 | 不支持 |
| 实盘策略行情 | 支持 | 禁止 | 禁止 |
| 外部依赖 | Futu OpenD `>= 10.9.6908` | 与 AKShare 共用内置 Python helper | 与 yfinance 共用内置 Python helper；固定 `akshare==1.18.91` |

AKShare 目录包含沪深股票与 ETF、美港通用证券、上证/深证/中证完整指数目录、港股行情指数，以及具有快照闭环的 `US..DJI`、`US..SPX`、`US..NDX`。恒生系列继续规范为 `HK.800000`、`HK.800100`、`HK.800700`；其他港股指数使用 `HK.<AKShare code>`，中证指数对外使用 `SH.<code>`。美港目录不能从 AKShare 明确判断品种时，`securityType` 保持 `null`；重复身份无法唯一判定时返回歧义错误，不按名称猜测。

AKShare 沪深股票、ETF、指数及美港通用证券支持 `1m/5m/15m/30m/1h/1d/1w/1mo`；美股 `5m` 至 `1h` 由一分钟数据确定性聚合，美港指数的周/月线由日线按交易所时区聚合。回测 Provider descriptor 会在任务启动前声明并校验这些滚动窗口：普通 key（如 `1m`）作用于所有市场，`US:5m` 形式的 key 优先覆盖特定市场。历史查询默认不复权（`adjustment=none`）；AKShare 沪深股票与 ETF 的 `1d/1w/1mo` 额外支持 `forward`/`backward`（qfq/hfq，ETF 经东财接口取数），指数与分钟周期的非 `none` 复权返回 400。沪深以“手”报告的成交量转换为股数。不存在的 volume、turnover 和真实报价时间保持 `null`。

## 历史 K 线分页

历史 K 线的 `before` 是严格排除式游标：后续页中的每根 K 线都必须早于它。`pagination.nextBefore` 始终等于当前页最早的 K 线时间，且只有 Provider 已确认还存在更早的有效 K 线时才会返回 `hasMore=true`。调用方必须以此分页元数据为准，不能根据当前页的返回条数推断是否还有历史数据。

`from`/`to` 是包含式边界，与 `before` 不可同时使用；所有显式范围查询都是有界查询，回应 `hasMore=false`。游标到达 Provider 最早可用数据或短周期保留边界时，会正常返回空页与 `hasMore=false`，而不是报错；上游、鉴权和响应格式故障仍按错误处理。

Yahoo Finance 接口不是官方稳定 API，也没有实时性或可用性承诺。JFTrade 会把缺失值安全映射为 `null`，并把上游失败转换为结构化错误，但无法消除上游限流、字段变化或临时不可用。

yfinance 快照优先读取 `Ticker.fast_info` 快速路径（15 秒缓存与 singleflight 不变），字段缺失或出错时才回退到 `get_info`；约 15 分钟延迟的契约不变。yfinance 历史 K 线的 `adjustment=forward` 映射为 Yahoo auto_adjust；`backward` 不支持，返回 400 `unsupported_time_range`；非法取值返回 400 `unsupported_adjustment`。

## 新闻与公司行动

yfinance 与 AKShare 都提供 `GET /api/v1/market-data/news/{market}/{symbol}?limit=`（`limit` 1–50，默认 10）和 `GET /api/v1/market-data/corporate-actions/{market}/{symbol}?from=&to=`（RFC3339 包含式边界，默认最近两年）。新闻条目含 `title`、`link`、`publisher`、`published_at`（RFC3339，可为 `null`）与 `summary`；公司行动事件含 `kind`（`dividend`/`split`）、`ex_date`（YYYY-MM-DD）、`amount`（每股，可空）与 `ratio`（可空），按 `(ex_date, kind)` 排序。AKShare 两项仅覆盖沪深：新闻经 `stock_news_em`（上游约 10 条），公司行动经 `stock_fhps_em`（半年报期 0630/1231，冷缓存可能较慢并返回 503 与 `Retry-After`）；美港请求返回 400 `AKSHARE_UNSUPPORTED`。Futu 不支持这两项，会返回明确的不支持错误。assistant 对应只读工具为 `market.news` 与 `market.corporate_actions`，都经过当前活跃 Provider；指数成分股另有 assistant 工具 `market.index_constituents`（仅 SH/SZ，经 AKShare），刻意不暴露公开 HTTP API。

控制台"资讯"与个股研究"公司行动"界面始终走 broker product-feature 管线（查询式 `/api/v1/market-data/news`、`/api/v1/research/corporate-actions/{instrumentId}`）。当活跃（或显式指定）Provider 为 yfinance/AKShare 时，`internal/productfeatures` 的 facade 会把这两个 feature 委托给上述 Provider 能力并投影为 `broker.FeatureResult`（新闻保留 `title/link/publisher/publishedAt/summary`；公司行动投影 `exDate` 与合成 `statement`，Futu 专有的公告日/进度/登记日等列为空），前端无需按 Provider 分支；assistant 的 `research.news`/`research.corporate_actions` 工具走同一路径同样生效。AKShare 的美港请求在该管线下返回 409 不支持错误。

Yahoo 的美股盘外分钟数据只作为价格样本使用：上游盘前成交量通常为零，盘后还可能把截至当时的累计成交量放进单根分钟 K，不能解释为该分钟增量。JFTrade 因此把 Yahoo 美股盘前、盘后分钟 K 的 `volume` 统一标记为 `null`，价格 K 仍保留；成交量柱和量价指标只使用成交量有效的常规时段 K。日成交量直接读取 Yahoo 日 K，不从盘外分钟 K 聚合。Futu OpenD 的每根 K 线成交量不受此规则影响。

Yahoo 的 `postMarketPrice` / `postMarketTime` 是 Yahoo Provider 下的盘后价格与实际报价时间，不会用 Futu 数值覆盖。JFTrade 使用当前生效的交易所日历校验报价所属交易日和盘前/盘后窗口，并据此分类分钟 K 线；周末、节假日和早收市边界不由 Python helper 或前端硬编码。行情卡片分别展示实际“报价时间”和日历计划“截止时间”，两者均以交易所时区为主。Futu `PreAfterMarketData` 不携带独立时间戳，因此不会把 BasicQot 常规更新时间冒充盘外报价时间。

Futu 的可见标的若 `BasicQot` 订阅因行情权限、不支持或订阅额度已满而无法建立，JFTrade 不会自动切换到 Yahoo/AKShare，也不会启动 Python helper。它会在同一 OpenD 连接上用 `Qot_GetStaticInfo` 获取 Stock ID，并用 `Qot_StockScreen` 读取延迟快照补全价格和可用的昨收；该路径不会创建 `Qot_Sub`，结果短暂缓存 15 秒。回退标的继续参与快照轮询，但不会加入实时推送流；订阅状态会标为 `fallback`，行情状态和自选价格旁会显示黄色的降级提示。盘口、K 线和实盘策略仍要求原生 Futu 订阅。

## 内置 helper

桌面版和带 `release_assets` 的 `cmd/jftrade-api` 会嵌入目标平台的 PyInstaller `onedir` helper（可执行文件及其依赖目录）。JFTrade 首次启用时把它原子发布到设置目录下的 `cache/marketdata-sidecar/<bundle-sha256>/`，逐文件校验摘要、类型和权限；后续启动完整校验后复用，不再重复写文件。损坏只重建当前摘要目录，缓存不可写时降级到权限受限的临时目录。全局行情、回测任务等消费者通过 Provider 租约共享 helper；切回 Futu 后只有最后一个 Python Provider 租约释放才停止进程，应用退出则统一取消任务并回收。helper 不会无限自动重启，也不会监听公网。

设置页不提供行情 Provider 分类，也不提供 host、port、enabled、timeout 或 Python 路径配置。发布版及 frozen helper 自带 Python 3.14；源码开发模式仍可在“设置 → 依赖项管理”查看 Python 3.11+ 与运行模块状态，但解释器只通过环境变量、workspace `.venv` 或 PATH 自动选择。

## Provider 切换与 Futu 退订

helper 的 `/healthz` 只报告进程状态；`/providers/yfinance/health` 与 `/providers/akshare/health` 分别报告独立的 `warming`、`ready` 或 `failed` 状态。两个运行时按需独立懒加载，一方导入失败不会拖垮另一方，进程启动和健康检查都不访问外网。预热中的数据请求返回 `503 <PROVIDER>_RUNTIME_WARMING` 和 `Retry-After: 1`；导入失败、线程池饱和或上游超时也返回结构化 `503`。

切换到 Yahoo 或 AKShare 时，JFTrade 会先启动并检查共用 helper，随后原子提交逻辑 Provider、清理旧行情缓存并让 collector 改走 15 秒轮询。Yahoo 与 AKShare 之间切换复用同一进程；激活失败会恢复旧 Provider 并保留进程。由 Futu 首次启动 helper 后激活失败时，只停止本次新启动的进程；成功切回 Futu 后才停止 helper。Futu OpenD 不允许物理订阅建立后不足一分钟就退订，因此旧 Futu demand 会立即归零，尚未满足最短持有时间的订阅进入 `pending_unsubscribe`。

collector 每 250 毫秒推进一次非活跃 Futu 清理；每条订阅从 OpenD 确认建立的时间起满一分钟后才发送退订。退订暂时失败会沿用行情订阅的退避策略继续重试，不会把已经成功的 Yahoo 切换回滚成 Futu。若在到期前切回 Futu，仍符合当前真实 demand 的物理订阅会直接复用，不会重复占用订阅额度。

## 回测模块 Provider

回测使用独立的 `backtestMarketDataProvider` 设置，不跟随全局行情 Provider。旧设置文件首次启动时会把当时的 `activeMarketDataProvider` 复制一次；之后两者独立切换。`GET/PUT /api/v1/settings/backtest-market-data-provider` 返回当前选择和三个 Provider descriptor。写入前会准备并检查目标 Provider，失败保持旧选择。

同步请求和回测启动请求都不接受逐次 Provider 覆盖。请求被接受时固定模块当前 Provider；同步去重、覆盖检查、进度、运行状态和结果都携带 `marketDataProvider`。切换设置不会打断已接受任务，新任务立即使用新选择。历史缓存 schema v3 以 `provider + symbol + interval + adjustment + session` 隔离数据；v2 缓存由数据库管理页的备份、确认重建、重启流程统一重建，独立的 `backtest-runs.db` 不受影响。

前端会按 descriptor 在同步或回测前拦截超出历史窗口的组合；后端仍执行同一校验，避免旧客户端绕过。前端的复权（rehabType）选项由当前 Provider descriptor 声明的 `priceAdjustments` 决定（yfinance 为 `none/forward`，AKShare 为 `none/forward/backward`），未声明时回退为 `none`，切换 Provider 后失效的选择重置为 `none`；K 线缓存本身已按 rehabType 分区，无 schema 变更。Go 侧还会在执行前预检拒绝 AKShare 分钟周期加复权这类不支持组合。缓存缺失但请求范围有效时，“开始回测”会先同步当前范围并在同步完成后重试一次。Provider 返回零根 K 线时同步任务标记失败，不再以“完成”掩盖无数据结果；已失败运行的真实错误会直接显示在默认报告页。

Wails 原生桌面开发不会自动选择、构建或启动 Python helper。需要在开发/测试环境显式提供 `JFTRADE_MARKETDATA_SIDECAR`，或先执行发布资产准备命令；没有可用运行时会沿用现有明确错误。独立运行 `cmd/jftrade-api` 时，可通过绝对路径指定本地 helper：

```bash
JFTRADE_MARKETDATA_SIDECAR=/absolute/path/to/marketdata-sidecar-darwin-arm64/marketdata-sidecar-darwin-arm64 \
  go run ./cmd/jftrade-api
```

开发和测试使用 `JFTRADE_MARKETDATA_SIDECAR` 指定 frozen helper，或以 `JFTRADE_MARKETDATA_DEV_PYTHON` 与 `JFTRADE_MARKETDATA_DEV_PYTHONPATH` 指定源码命令。旧 `JFTRADE_YFINANCE_*` 名称仍作为低优先级兼容别名；新旧同时存在时始终使用通用名称。未提供覆盖时依次检查 workspace `.venv`、PATH 和平台常见路径；正式 profile 忽略全部开发覆盖。构建命令为 `pnpm run build:marketdata-sidecar`，兼容脚本 `build:yfinance-sidecar` 本次仍保留；构建解释器必须是 CPython 3.14.x。

## 验证

正常运行时 helper endpoint 是 JFTrade 的内部动态地址，不会写入配置或暴露给前端。开发 API 可以查询统一状态接口：

```bash
curl http://127.0.0.1:3000/api/v1/market-data/provider
```

helper 的健康路由不访问外部网络；只有开发者做 standalone smoke 时才直接调用它。JFTrade 对外行情接口仍是统一的 `/api/v1/market-data/*`，前端不会直接调用 Python sidecar。

如果 Provider 状态不可用或 helper 启动失败，请按 [Python 行情 sidecar 排障](./troubleshooting/marketdata-sidecar.md) 检查嵌入资产、运行时与上游网络。

## 持久化配置

普通用户无需配置 Python helper。新安装的 `settings.json` 只记录默认 Provider：

```json
{
  "activeMarketDataProvider": "yfinance"
}
```

yfinance 与 AKShare 由同一应用运行时管理，不持久化 Python、包安装、连接地址或端口配置；旧 `runtimeDependencies.pythonBinaryPath` 会被忽略。JFTrade 不自动执行 pip、升级或创建虚拟环境，也不迁移或删除旧 `cache/yfinance-sidecar`；新缓存独立写入 `cache/marketdata-sidecar/<sha>`。

运行中的实盘策略不会被静默迁移到延迟行情：切换 Provider 前必须先停止全部实盘策略。live 与 notify-only 都要求当前全局 Provider 声明 `streamingCandles=true`；首期只有 Futu 满足。回测则可独立选择三种历史 Provider。

## 开发与测试

sidecar 测试会 mock 两个上游并阻止真实 socket 连接：

```bash
uv sync --locked --project workers/marketdata-sidecar --extra runtime --extra test
uv run --locked --project workers/marketdata-sidecar --extra runtime --extra test pytest workers/marketdata-sidecar/tests
```

Go 侧 Provider、运行时切换与统一行情 service 的测试分别位于：

- `internal/integration/yfinance`
- `internal/integration/akshare`
- `internal/app/apiserver/marketdataapp`
- `internal/marketdata`

公开设置或行情 HTTP 契约发生变化后，仍需执行 `pnpm run generate:docs`。
