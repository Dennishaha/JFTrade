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
| 实时推流 | 支持 | 不支持 | 不支持 |
| Level 2 盘口 | 取决于权限 | 不支持 | 不支持 |
| 盘前盘后 | 支持 | 美股由 Yahoo 实际报价决定 | 不支持 |
| 实盘策略行情 | 支持 | 禁止 | 禁止 |
| 外部依赖 | Futu OpenD `>= 10.9.6908` | 与 AKShare 共用内置 Python helper | 与 yfinance 共用内置 Python helper；固定 `akshare==1.18.81` |

AKShare 目录包含沪深股票与 ETF、美港通用证券、上证/深证/中证完整指数目录、港股行情指数，以及具有快照闭环的 `US..DJI`、`US..SPX`、`US..NDX`。恒生系列继续规范为 `HK.800000`、`HK.800100`、`HK.800700`；其他港股指数使用 `HK.<AKShare code>`，中证指数对外使用 `SH.<code>`。美港目录不能从 AKShare 明确判断品种时，`securityType` 保持 `null`；重复身份无法唯一判定时返回歧义错误，不按名称猜测。

AKShare 沪深股票、ETF、指数及美港通用证券支持 `1m/5m/15m/30m/1h/1d/1w/1mo`；美股 `5m` 至 `1h` 由一分钟数据确定性聚合，美港指数的周/月线由日线按交易所时区聚合。超出上游保留窗口会返回 `UNSUPPORTED_RANGE`，不会伪装为空结果。历史查询统一不复权；沪深以“手”报告的成交量转换为股数。不存在的 bid、ask、volume、turnover 和真实报价时间保持 `null`。

Yahoo Finance 接口不是官方稳定 API，也没有实时性或可用性承诺。JFTrade 会把缺失值安全映射为 `null`，并把上游失败转换为结构化错误，但无法消除上游限流、字段变化或临时不可用。

Yahoo 的美股盘外分钟数据只作为价格样本使用：上游盘前成交量通常为零，盘后还可能把截至当时的累计成交量放进单根分钟 K，不能解释为该分钟增量。JFTrade 因此把 Yahoo 美股盘前、盘后分钟 K 的 `volume` 统一标记为 `null`，价格 K 仍保留；成交量柱和量价指标只使用成交量有效的常规时段 K。日成交量直接读取 Yahoo 日 K，不从盘外分钟 K 聚合。Futu OpenD 的每根 K 线成交量不受此规则影响。

Yahoo 的 `postMarketPrice` / `postMarketTime` 是 Yahoo Provider 下的盘后价格与实际报价时间，不会用 Futu 数值覆盖。JFTrade 使用当前生效的交易所日历校验报价所属交易日和盘前/盘后窗口，并据此分类分钟 K 线；周末、节假日和早收市边界不由 Python helper 或前端硬编码。行情卡片分别展示实际“报价时间”和日历计划“截止时间”，两者均以交易所时区为主。Futu `PreAfterMarketData` 不携带独立时间戳，因此不会把 BasicQot 常规更新时间冒充盘外报价时间。

## 内置 helper

桌面版和带 `release_assets` 的 `cmd/jftrade-api` 会嵌入目标平台的 PyInstaller `onedir` helper（可执行文件及其依赖目录）。JFTrade 首次启用时把它原子发布到设置目录下的 `cache/marketdata-sidecar/<bundle-sha256>/`，逐文件校验摘要、类型和权限；后续启动完整校验后复用，不再重复写文件。损坏只重建当前摘要目录，缓存不可写时降级到权限受限的临时目录。切回 Futu 或退出时停止 helper 进程，但保留有效缓存；成功启动后尽力清理超过 7 天的旧摘要。helper 不会无限自动重启，也不会监听公网。

设置页不提供行情 Provider 分类，也不提供 host、port、enabled、timeout 或 Python 路径配置。发布版及 frozen helper 自带 Python 3.14；源码开发模式仍可在“设置 → 依赖项管理”查看 Python 3.11+ 与运行模块状态，但解释器只通过环境变量、workspace `.venv` 或 PATH 自动选择。

## Provider 切换与 Futu 退订

helper 的 `/healthz` 只报告进程状态；`/providers/yfinance/health` 与 `/providers/akshare/health` 分别报告独立的 `warming`、`ready` 或 `failed` 状态。两个运行时按需独立懒加载，一方导入失败不会拖垮另一方，进程启动和健康检查都不访问外网。预热中的数据请求返回 `503 <PROVIDER>_RUNTIME_WARMING` 和 `Retry-After: 1`；导入失败、线程池饱和或上游超时也返回结构化 `503`。

切换到 Yahoo 或 AKShare 时，JFTrade 会先启动并检查共用 helper，随后原子提交逻辑 Provider、清理旧行情缓存并让 collector 改走 15 秒轮询。Yahoo 与 AKShare 之间切换复用同一进程；激活失败会恢复旧 Provider 并保留进程。由 Futu 首次启动 helper 后激活失败时，只停止本次新启动的进程；成功切回 Futu 后才停止 helper。Futu OpenD 不允许物理订阅建立后不足一分钟就退订，因此旧 Futu demand 会立即归零，尚未满足最短持有时间的订阅进入 `pending_unsubscribe`。

collector 每 250 毫秒推进一次非活跃 Futu 清理；每条订阅从 OpenD 确认建立的时间起满一分钟后才发送退订。退订暂时失败会沿用行情订阅的退避策略继续重试，不会把已经成功的 Yahoo 切换回滚成 Futu。若在到期前切回 Futu，仍符合当前真实 demand 的物理订阅会直接复用，不会重复占用订阅额度。

`pnpm run desktop:dev` 默认直接使用 `workers/marketdata-sidecar/.venv` 和源码目录；只有显式 helper、开发 venv 均不可用时才复用已构建的 frozen helper，启动脚本不会隐式运行 PyInstaller。没有可用运行时会立即给出安装命令。独立运行 `cmd/jftrade-api` 时，可通过绝对路径指定本地 helper：

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

运行中的实盘策略不会被静默迁移到延迟行情：切换 Provider 前必须先停止全部实盘策略。yfinance 或 AKShare 激活期间不能启动实盘策略；回测不受此限制。

## 开发与测试

sidecar 测试会 mock 两个上游并阻止真实 socket 连接：

```bash
python -m pytest workers/marketdata-sidecar/tests
```

Go 侧 Provider、运行时切换与统一行情 service 的测试分别位于：

- `internal/integration/yfinance`
- `internal/integration/akshare`
- `internal/app/apiserver/marketdataapp`
- `internal/marketdata`

公开设置或行情 HTTP 契约发生变化后，仍需执行 `pnpm run generate:docs`。
