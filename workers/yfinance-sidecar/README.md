# JFTrade yfinance sidecar

这个服务把 Python `yfinance` 封装为仅监听本机的 HTTP 数据源，供 JFTrade
的 Go 进程调用。sidecar 支持 `US`、`HK`、`SH` 和 `SZ` 四个叶子市场；
`CN` 是前端聚合分类，只接受带 `SH.` 或 `SZ.` 的限定代码。响应始终使用
规范化的 JFTrade 标的 ID（例如 `HK.00700`），不会泄漏 Yahoo 的 `.HK`、
`.SS` 或 `.SZ` 后缀。

## 能力边界

- 支持证券搜索、证券详情/基本面、延迟快照和历史 K 线。
- 直查接口会用 Yahoo metadata 复核真实交易所和证券类型；不支持的市场及
  crypto、currency、future 会稳定返回 `404`，不会伪装成其他市场数据。
- K 线支持 `1m`、`5m`、`15m`、`30m`、`1h`、`1d`、`1w`、`1mo`；美股请求
  会包含盘前盘后数据，港股及沪深按各自交易时区请求常规时段数据。
- Yahoo Finance 免费数据不提供 JFTrade 所需的可靠实时推流或 Level 2
  盘口；快照按 15 分钟延迟数据展示。
- Yahoo Finance 接口并非官方稳定 API，调用失败会被转换为结构化的
  `502 upstream_error`，不会向 Go 进程泄漏 Python 异常。
- 每次 Yahoo 上游传输调用最多等待 10 秒，避免 Go 请求取消后后台线程
  长时间占用 sidecar worker。

所有时间字段都是 RFC 3339 UTC 字符串。缺失或非有限数值会返回 `null`；
OHLC 不完整的 K 线会被丢弃，因此响应不会包含非法 JSON 的 `NaN` 或
`Infinity`。

## 安装和启动

需要 Python 3.11 或更高版本：

```bash
cd workers/yfinance-sidecar
python3.11 -m venv .venv
.venv/bin/python -m pip install --upgrade pip
.venv/bin/python -m pip install --editable '.[runtime]'
.venv/bin/python -m yfinance_sidecar.main --host 127.0.0.1 --port 7788
```

源码启动仅用于开发和测试。正式 JFTrade 安装包使用随 Go 二进制嵌入的
PyInstaller 单文件 helper，由应用自动分配 loopback 端口并管理生命周期。
helper 也支持 `--version`：

```bash
./yfinance-sidecar --version
```

服务应只绑定 `127.0.0.1`。可通过以下命令验证进程本身，不会触发 Yahoo
网络请求：

```bash
curl http://127.0.0.1:7788/health
```

## HTTP 契约

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/health` | 进程和 yfinance 版本 |
| GET | `/markets` | 可路由市场及别名 |
| GET | `/search?q=Apple&limit=20` | 证券搜索，`limit` 为 1–100 |
| GET | `/security/{market}/{symbol}` | 证券详情和基本面 |
| GET | `/snapshot/{market}/{symbol}` | 延迟快照 |
| GET | `/candles/{market}/{symbol}` | 历史 K 线 |

K 线参数：

- `period`：上述八种周期之一，默认 `1d`。
- `limit`：返回最近的数据条数，范围 1–1000，默认 200。
- `from` / `to`：可选 RFC 3339 有时区时间；`from` 不得晚于 `to`。

成功响应直接返回资源对象。失败响应统一为：

```json
{
  "error": {
    "code": "unsupported_period",
    "message": "unsupported candle period: 2m"
  }
}
```

调用方应以 HTTP 状态和 `error.code` 分支，不应匹配英文消息。

## 测试

```bash
.venv/bin/python -m pip install --editable '.[runtime,test]'
.venv/bin/pytest
```

To build the release executable, install the pinned build extra and run the
repository build script. The resulting one-file executable is staged under
`internal/yfinanceassets/assets/bin`:

```bash
.venv/bin/python -m pip install --editable '.[runtime,build]'
JFTRADE_YFINANCE_BUILD_PYTHON="$PWD/.venv/bin/python" \
  node ../../scripts/build-yfinance-sidecar.mjs
```

测试使用 `httpx.ASGITransport` 和 yfinance mock，并全局阻止 socket 连接；
不会访问真实 Yahoo Finance。
