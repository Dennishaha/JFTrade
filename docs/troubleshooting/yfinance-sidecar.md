# yfinance sidecar 排障

本文只处理内置 Yahoo Finance 行情 Provider。先在首页/研究页的“行情提供者”状态提示或统一状态接口确认 Provider 状态；交易账户和下单问题仍按 Futu/OpenD 链路排查。

## 最短诊断路径

发布版 helper 使用运行时动态分配的 loopback 端口，用户不需要知道或配置端口；helper endpoint 是内部地址，不会出现在用户界面或持久化文件中。

```bash
curl -sS http://127.0.0.1:3000/api/v1/market-data/provider
```

开发者做 standalone helper smoke 时，才使用 CLI 的临时测试端口直接调用 `/health`；这不是正式运行模式。Windows PowerShell 可以用统一 Provider 状态接口：

```powershell
Invoke-RestMethod http://127.0.0.1:3000/api/v1/market-data/provider
```

判断方式：

- 连接被拒绝：helper 没启动、启动后退出，或开发态 `JFTRADE_YFINANCE_SIDECAR` 路径无效。
- 发布版提示 helper 缺失：安装包不是对应平台的 `release_assets` 构建，或资产校验失败；重新安装匹配平台的产品包。
- helper `/health` 返回 `200`，JFTrade 仍报不可用：检查 Provider 状态中的最后错误和 helper 进程日志。
- `/health` 的 `runtime_state=warming`，查询返回 `503 YFINANCE_RUNTIME_WARMING`：本地进程已就绪，重型依赖仍在后台预热；客户端应遵循 `Retry-After: 1`，通常无需人工处理。
- `/health` 的 `runtime_state=failed`：后台预热失败，查看可选 `warmup_error` 和桌面日志定位缺失资产或导入错误。
- `/health` 为 `ready`，但查询返回 `502 upstream_error`：本地进程与运行时正常，失败发生在 Yahoo 网络请求或上游响应转换。
- 返回 `unsupported_market`、`unsupported_period` 或 `unsupported_capability`：请求超出当前明确支持的能力，不应靠重试解决。

## 自动启动后立即退出

发布版由 PyInstaller `onedir` helper（可执行文件及其依赖目录）提供 Python 运行时，不需要用户安装 Python。有效 bundle 持久缓存到设置目录下的 `cache/yfinance-sidecar/<bundle-sha256>/`；遇到篡改、符号链接、摘要或权限不匹配时只重建当前摘要，缓存不可写才退回临时目录。源码开发模式会检查 Python 3.11+ 以及 `yfinance_sidecar`、FastAPI、Uvicorn、yfinance、curl_cffi；可以在“设置 → 依赖项管理”保存解释器路径，也可以通过环境变量覆盖。如果使用独立 frozen helper，可显式提供其绝对路径：

```bash
JFTRADE_YFINANCE_SIDECAR=/absolute/path/to/yfinance-sidecar-<platform>/yfinance-sidecar-<platform> \
  go run ./cmd/jftrade-api
```

构建开发 helper：

```bash
python -m pip install --editable "workers/yfinance-sidecar[runtime,build]"
pnpm run build:yfinance-sidecar
```

源码模式只检测依赖，不会自动执行 pip、升级或创建虚拟环境。保存新的 Python 路径后，设置页会立即重新探测；当前 Yahoo helper 不会被中断，新路径在下一次 helper 启动、Provider 切换或应用重启时生效。

桌面开发启动会优先保留显式 `JFTRADE_YFINANCE_SIDECAR` 或源码环境覆盖；没有覆盖时检查已保存的 Python 路径，再检查 `workers/yfinance-sidecar/.venv/bin/python`（Windows 为 `.venv\\Scripts\\python.exe`）和 `workers/yfinance-sidecar/src`，最后复用已构建 frozen helper。启动脚本不会自动构建 helper；全部不可用时快速退出并给出安装命令。正式 profile 忽略开发覆盖。`onedir` helper 不需要启动时解压整个运行时，JFTrade 仍会等待 `/health` 就绪。

显式切换到 yfinance 时，JFTrade 会等待 helper 的 `/health` 达到 `runtime_state=ready`（最长约 45 秒）。路径不存在、helper 缺失、启动失败、预热失败或超时都会返回 `409 MARKET_DATA_PROVIDER_UPDATE_FAILED`，停止本次新进程，并恢复原来的 Provider。

应用启动时恢复已持久化的 yfinance 只要求 helper 进程和 `/health` 可用，不等待后台预热；主界面可以先进入，行情状态显示“Yahoo 预热中”。helper 缺失或进程健康失败仍会回退并持久化 Futu。修复安装或开发路径后，再切回 yfinance 以触发一次受 ready 门禁保护的新启动。

## 端口冲突

helper 使用动态 loopback 端口，不存在需要用户处理的固定 yfinance 端口冲突。若日志显示启动时端口分配失败，检查本机 loopback 和进程资源；JFTrade 不会接管其他进程的监听器。

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

`/health` 不访问 Yahoo，也不同步导入 pandas、numpy、yfinance；它证明本地进程可响应，并通过 `runtime_state` 区分预热阶段。即使状态为 `ready`，也不能证明 Yahoo 上游行情可用。

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
curl http://127.0.0.1:<测试端口>/markets
curl 'http://127.0.0.1:<测试端口>/search?q=Apple&limit=5'
curl http://127.0.0.1:<测试端口>/snapshot/US/AAPL
curl 'http://127.0.0.1:<测试端口>/candles/US/AAPL?period=1d&limit=5'
curl http://127.0.0.1:<测试端口>/snapshot/HK/0700
curl 'http://127.0.0.1:<测试端口>/candles/SH/600519?period=1d&limit=5'
```

搜索、快照和 K 线会访问真实 Yahoo 网络。自动化测试不要使用这些请求；仓库测试通过 mock 和 socket 阻断保证普通 CI 不依赖外部行情。
