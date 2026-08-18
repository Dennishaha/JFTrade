# Python 行情 sidecar 排障

本文处理共用 `marketdata-sidecar` 内的 Yahoo Finance 与 AKShare Provider。先在首页/研究页的“行情提供者”状态提示或统一状态接口确认当前 Provider；交易账户和下单问题仍按 Futu/OpenD 链路排查。

## 最短诊断路径

发布版 helper 使用运行时动态分配的 loopback 端口，用户不需要知道或配置端口；helper endpoint 是内部地址，不会出现在用户界面或持久化文件中。

```bash
curl -sS http://127.0.0.1:3000/api/v1/market-data/provider
```

开发者做 standalone helper smoke 时，才使用 CLI 的临时测试端口直接调用 `/healthz` 与 `/providers/{source}/health`；这不是正式运行模式。Windows PowerShell 可以用统一 Provider 状态接口：

```powershell
Invoke-RestMethod http://127.0.0.1:3000/api/v1/market-data/provider
```

判断方式：

- 连接被拒绝：helper 没启动、启动后退出，或开发态 `JFTRADE_MARKETDATA_SIDECAR` 路径无效。
- 发布版提示 helper 缺失：安装包不是对应平台的 `release_assets` 构建，或资产校验失败；重新安装匹配平台的产品包。
- `/healthz` 返回 `200`，JFTrade 仍报不可用：继续检查当前 Provider 的独立健康状态和 helper 日志。
- Provider health 为 `warming` 且查询返回 `503 *_RUNTIME_WARMING`：运行时仍在后台导入；遵循 `Retry-After: 1`。
- Provider health 为 `failed`：查看 `warmup_error`；Yahoo 导入失败不会阻断 AKShare，反之亦然。
- health 为 `ready`，但查询返回 `502`：失败发生在当前上游请求或响应结构转换，不会自动换源。
- AKShare 返回 `503 AKSHARE_POOL_BUSY` 或 `AKSHARE_UPSTREAM_TIMEOUT`：最多四个工作线程已占满或单次调用超过 12 秒；超时线程仍占用其原槽位，待实际返回后才释放。
- 返回 `unsupported_market`、`unsupported_period`、`unsupported_adjustment` 或 `unsupported_capability`：请求超出当前明确支持的能力，不应靠重试解决。`unsupported_adjustment` 表示该 Provider 或品种/周期组合不支持请求的复权方式：yfinance 只支持 `none/forward`，AKShare 仅沪深股票与 ETF 的 `1d/1w/1mo` 支持 `forward/backward`。

## 自动启动后立即退出

发布版由 CPython 3.14.x 构建的 PyInstaller `onedir` helper 提供 Python 运行时，不需要用户安装 Python。有效 bundle 缓存在 `cache/marketdata-sidecar/<bundle-sha256>/`；旧 yfinance 缓存不会被复用或自动删除。源码启动探针只检查 Python 3.11+、`marketdata_sidecar`、FastAPI 和 Uvicorn；yfinance/curl_cffi 与 akshare 的导入成败由各自 Provider health 独立报告，不能阻止另一数据源启动。解释器通过 `JFTRADE_MARKETDATA_DEV_PYTHON`、workspace `.venv` 或 PATH 选择。如果使用独立 frozen helper，可显式提供其绝对路径：

```bash
JFTRADE_MARKETDATA_SIDECAR=/absolute/path/to/marketdata-sidecar-<platform>/marketdata-sidecar-<platform> \
  go run ./cmd/jftrade-api
```

构建开发 helper：

```bash
python3.11 -m pip install --disable-pip-version-check uv==0.12.5
uv sync --locked --project workers/marketdata-sidecar --extra runtime --extra build
pnpm run build:marketdata-sidecar
```

源码模式只检测依赖，不会自动执行 pip、升级或创建虚拟环境。Python 诊断不再显示在 OOBE 或设置页；需要排查源码模式时查看 sidecar 启动日志。

桌面开发启动会优先保留显式 `JFTRADE_MARKETDATA_*` 覆盖；旧 `JFTRADE_YFINANCE_*` 仅在通用变量为空时生效。没有覆盖时检查 `workers/marketdata-sidecar/.venv/bin/python`（Windows 为 `.venv\\Scripts\\python.exe`）和源码目录，最后复用已构建 frozen helper。正式 profile 忽略开发覆盖。

显式切换到 yfinance 或 AKShare 时，JFTrade 会等待对应 Provider 达到 `ready`（最长约 45 秒）。路径不存在、helper 缺失、启动失败、预热失败或超时都会返回 `409 MARKET_DATA_PROVIDER_UPDATE_FAILED` 并恢复原 Provider。若旧 Provider 也是 Python Provider，共用进程会保留；只有从 Futu 本次新启动的进程才会停止。

应用启动时恢复已持久化的 Python Provider 只要求 helper 进程及轻量健康可用，不等待后台预热；主界面可以先进入并显示预热状态。helper 缺失或进程健康失败会保留已配置 Provider 并显示不可用状态，不会回退或持久化 Futu。修复安装或开发路径后，重新选择目标 Provider 以触发受 `ready` 门禁保护的新启动。

## 端口冲突

helper 使用动态 loopback 端口，不存在需要用户处理的固定端口冲突。若日志显示启动时端口分配失败，检查本机 loopback 和进程资源；JFTrade 不会接管其他进程的监听器。

开发态外部 helper 也由 JFTrade 分配端口并通过 `--host 127.0.0.1 --port <动态端口>` 启动，不要手工固定端口。

## Yahoo 上游失败

sidecar 会把网络错误、限流和无法解析的上游响应转换为结构化错误，例如：

```json
{
  "error": {
    "code": "upstream_error",
    "message": "..."
  }
}
```

依次检查：

1. 当前机器能否正常解析和访问 Yahoo Finance。
2. 企业代理、防火墙或 VPN 是否只作用于浏览器，而没有作用于该 Python 进程。
3. 标的是否使用受支持的 JFTrade 标识：`US.AAPL`、`HK.00700`、`SH.600519` 或 `SZ.000001`。`CN` 只是前端聚合分类，必须携带 `SH.` 或 `SZ.` 前缀。
4. 请求是否过于密集。`yfinance` 不提供可依赖的免费服务等级或稳定限额，持续重试可能加重限流。

每次 Yahoo 上游传输调用最多等待 10 秒；JFTrade 设置中的请求总超时还包含 Go 侧安全重试，因此两者不是同一个超时。若持续出现超时，应先排查网络或上游限流，而不是不断放大总超时。

`/healthz` 与 Provider health 都不访问 Yahoo 或 AKShare 行情网络；即使状态为 `ready`，也不能证明外部上游行情可用。

## AKShare 上游失败

AKShare 请求没有 Yahoo/Futu 自动兜底。输入、周期或范围错误返回 `400`，标的不存在返回 `404`，上游或表结构异常返回 `502`，预热、导入失败、线程池饱和或 12 秒截止返回 `503`。目录和全市场 spot 按来源/市场缓存 15 秒并做 singleflight；批量快照按市场目录取数，单次最多 100 个标的，不逐证券请求上游。

排查时确认目标身份属于支持目录。美股指数必须使用 `US..DJI`、`US..SPX` 或 `US..NDX`；恒生系列使用 `HK.800000`、`HK.800100`、`HK.800700`。超出分钟历史保留窗口的 `UNSUPPORTED_RANGE` 是能力边界，不应无限重试。指数与分钟周期的非 `none` 复权同样返回 `UNSUPPORTED_RANGE`。

新闻（`stock_news_em`）、公司行动（`stock_fhps_em`）和指数成分股路由只覆盖沪深标的；美港或非指数标的请求返回 400 `AKSHARE_UNSUPPORTED`。公司行动的冷缓存首次取数可能较慢，sidecar 可能返回 503 并附带 `Retry-After`，稍后重试即可命中缓存。

## 数据看起来不实时

这是预期能力边界：

- yfinance 快照统一按约 15 分钟延迟行情处理。
- 它没有 JFTrade 可依赖的实时推流；collector 会使用轮询，不会建立 Futu `BasicQot` 订阅。
- 它不提供 Level 2 盘口，深度请求会返回明确的不支持错误。
- 美股历史 K 线可包含盘前盘后；港股和沪深使用各自常规交易时段，不能据此推断当前报价是实时数据。
- Yahoo 美股盘外分钟成交量不是可靠的分钟增量：盘前常见全零，盘后可能混入累计量。统一行情 API 会保留盘外 OHLC，但将这些 K 线的 `volume` 返回为 `null`；日成交量请以 `1d` K 线为准。

需要实时推流、订阅状态或 Level 2 时，请切回 Futu OpenD，并确认相应市场数据权限。

## 手工复现 sidecar 契约

以下请求只用于开发/测试中直接验证 helper 边界，不经过 JFTrade 认证；将 `<测试端口>` 替换为本次 standalone helper 启动时选择的临时端口：

```bash
curl http://127.0.0.1:<测试端口>/healthz
curl http://127.0.0.1:<测试端口>/providers/yfinance/health
curl http://127.0.0.1:<测试端口>/providers/akshare/health
curl 'http://127.0.0.1:<测试端口>/providers/yfinance/search?q=Apple&limit=5'
curl http://127.0.0.1:<测试端口>/providers/akshare/snapshot/SH/600519
curl 'http://127.0.0.1:<测试端口>/providers/akshare/candles/US/.SPX?period=1d&limit=5'
curl -X POST http://127.0.0.1:<测试端口>/providers/akshare/snapshots \
  -H 'Content-Type: application/json' \
  -d '{"instrument_ids":["SH.600519","HK.00700"]}'
```

搜索、快照和 K 线会访问真实外部网络。普通自动化测试通过 fixture、mock 和 socket 阻断保持离线；显式联网 smoke 必须由专用环境变量开启，不纳入普通 CI。
