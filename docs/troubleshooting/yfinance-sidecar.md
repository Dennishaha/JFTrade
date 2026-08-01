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
- `/health` 正常，但查询返回 `502 upstream_error`：本地进程正常，失败发生在 Yahoo 网络请求或上游响应转换。
- 返回 `unsupported_market`、`unsupported_period` 或 `unsupported_capability`：请求超出当前明确支持的能力，不应靠重试解决。

## 自动启动后立即退出

发布版由 PyInstaller `onedir` helper（可执行文件及其依赖目录）提供 Python 运行时，不需要用户安装 Python。`pnpm run desktop:dev` 会自动复用或构建当前平台 helper，并注入目录内可执行文件的绝对路径；如果使用独立 `cmd/jftrade-api`，仍需手工提供目录内可执行 helper：

```bash
JFTRADE_YFINANCE_SIDECAR=/absolute/path/to/yfinance-sidecar-<platform>/yfinance-sidecar-<platform> \
  go run ./cmd/jftrade-api
```

构建开发 helper：

```bash
python -m pip install --editable "workers/yfinance-sidecar[runtime,build]"
pnpm run build:yfinance-sidecar
```

桌面开发启动会默认使用 `workers/yfinance-sidecar/.venv/bin/python`（Windows 为 `.venv\\Scripts\\python.exe`）构建；也可以通过 `JFTRADE_YFINANCE_BUILD_PYTHON` 指定构建 Python。`onedir` helper 不需要启动时解压整个运行时，JFTrade 仍会等待 `/health` 就绪。

显式切换到 yfinance 时，JFTrade 会等待 helper 的 `/health`（最长约 45 秒）。路径不存在、helper 缺失、启动失败或健康探测失败都会返回 `409 MARKET_DATA_PROVIDER_UPDATE_FAILED`，停止本次新进程，并恢复原来的 Provider。

应用启动时恢复已持久化的 yfinance 若 helper 缺失会回退并持久化 Futu，不会让首页继续使用失效的数据源。修复安装或开发路径后，再切回 yfinance 以触发一次受健康门禁保护的新启动。

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

`/health` 不访问 Yahoo，因此它只能证明本地进程和依赖已加载，不能证明上游行情可用。

## 数据看起来不实时

这是预期能力边界：

- yfinance 快照统一按约 15 分钟延迟行情处理。
- 它没有 JFTrade 可依赖的实时推流；collector 会使用轮询，不会建立 Futu `BasicQot` 订阅。
- 它不提供 Level 2 盘口，深度请求会返回明确的不支持错误。
- 美股历史 K 线可包含盘前盘后；港股和沪深使用各自常规交易时段，不能据此推断当前报价是实时数据。

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
