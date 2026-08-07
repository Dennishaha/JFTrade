# JFTrade market-data sidecar

这个仅监听 loopback 的 Python 服务为 JFTrade 同时承载 `yfinance` 和
`AKShare` 行情适配。两个 Provider 拥有相互隔离的懒加载运行时；进程启动和
`/healthz` 都不会导入数据源或访问网络，一个 Provider 初始化失败也不会影响
另一个 Provider。

## 能力边界

- yfinance 保留 US、HK、SH、SZ 的搜索、详情、延迟快照和八种周期 K 线。
- AKShare 支持上述四市场的市场描述、搜索、详情、单/批量快照和 K 线；沪深
  另有 ETF 与上证/深证/中证指数，港股包含 AKShare 指数目录，美股指数限定
  为 DJIA、SPX、NDX。
- AKShare 不提供推流、Level 2、盘前盘后或交易能力。它失败时返回明确错误，
  不会在 sidecar 内回退到 yfinance 或 Futu。
- AKShare 全市场目录缓存 15 秒，并按目录 singleflight。批量快照先按市场各取
  一次目录，再在本地解析最多 100 个标的，不逐标的请求上游。
- AKShare 阻塞调用使用独立的四线程池，每次最多等待 12 秒。四个槽位都被
  在途或已超时但仍未结束的调用占用时立即返回 `AKSHARE_POOL_BUSY`。

AKShare 数值使用十进制字符串，避免经 JSON `float` 丢精度。缺失的 bid、ask、
volume 和 turnover 保持 `null`；沪深以“手”返回的成交量乘以 100 后输出。
时间统一为 RFC 3339 UTC，没有真实行情时间时只填 `observed_at`，不伪造
`quote_at`。K 线使用 `adjust=""` 的不复权数据，非法/非有限 OHLC 会被丢弃。

## 安装和启动

需要 Python 3.11 或更高版本：

```bash
cd workers/marketdata-sidecar
python3.11 -m venv .venv
.venv/bin/python -m pip install --upgrade pip
.venv/bin/python -m pip install --editable '.[runtime]'
.venv/bin/python -m marketdata_sidecar.main --host 127.0.0.1 --port 7788
```

CLI 名为 `marketdata-sidecar`，默认端口仍为 `7788`。兼容入口
`yfinance-sidecar` 暂时保留，但二者都运行同一个通用进程：

```bash
marketdata-sidecar --version
curl http://127.0.0.1:7788/healthz
```

正式安装包使用随 Go 二进制嵌入的 PyInstaller `onedir` helper，产物位于
`internal/marketdataassets/assets/bin/<platform>`。构建使用：

```bash
.venv/bin/python -m pip install --editable '.[runtime,build]'
JFTRADE_MARKETDATA_BUILD_PYTHON="$PWD/.venv/bin/python" \
  node ../../scripts/build-marketdata-sidecar.mjs
```

PyInstaller spec 使用 `JFTRADE_MARKETDATA_BINARY_NAME` 指定二进制名；旧
`JFTRADE_YFINANCE_BINARY_NAME` 仅作为低优先级兼容别名。运行时的通用
`JFTRADE_MARKETDATA_*` 配置同样优先于旧 `JFTRADE_YFINANCE_*` 配置。

## HTTP 契约

通用和命名空间路由：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 仅检查 sidecar 进程，不触发 Provider 预热 |
| GET | `/providers/{source}/health` | `source` 为 `yfinance` 或 `akshare`；触发该运行时懒加载 |
| GET | `/providers/{source}/markets` | Provider 可路由市场与交易时段 |
| GET | `/providers/{source}/search?q=...&limit=20` | 搜索，limit 为 1–100 |
| GET | `/providers/{source}/security/{market}/{symbol}` | 详情与品种支持周期 |
| GET | `/providers/{source}/snapshot/{market}/{symbol}` | 延迟快照 |
| GET | `/providers/{source}/candles/{market}/{symbol}` | 历史 K 线 |
| POST | `/providers/akshare/snapshots` | 最多 100 个 AKShare 批量快照 |

原有 `/health`、`/markets`、`/search`、`/security/...`、`/snapshot/...` 和
`/candles/...` 继续作为 yfinance 兼容路由，返回内容与对应的
`/providers/yfinance/...` 一致。

批量请求和响应：

```json
{
  "instrument_ids": ["US.AAPL", "HK.00700", "US..SPX"]
}
```

```json
{
  "entries": [],
  "errors": [
    {
      "instrument_id": "US.MISSING",
      "code": "instrument_not_found",
      "message": "instrument not found: US.MISSING"
    }
  ]
}
```

K 线参数为 `period`、`limit`、`from`、`to`、`before`。股票、沪深 ETF/指数支持
`1m/5m/15m/30m/1h/1d/1w/1mo`；美港指数只支持 `1d/1w/1mo`。
美股 5–60 分钟线由 1 分钟数据按纽约交易所时区确定性聚合，美港指数周/月线
由日线按交易所时区聚合。1 分钟请求超出最近五天返回
`400 UNSUPPORTED_RANGE`，不会伪装成空结果。

`before` 是严格排除式游标：返回的 K 线都早于它。当且仅当已确认仍有更早的有效 K 线时，响应才设 `has_more=true`，并以当前页最早 K 线时间作为 `next_before`。`from`/`to` 为包含式范围边界，不能与 `before` 同时使用；范围查询总是终点页（`has_more=false`）。游标超过最早可用历史或短周期保留边界时，服务返回空页与 `has_more=false`，不将其当作上游故障。

所有失败都使用统一 envelope：

```json
{
  "error": {
    "code": "AKSHARE_UPSTREAM_ERROR",
    "message": "AKShare search failed"
  }
}
```

输入、周期和区间错误为 400，标的不存在为 404，上游/结构错误为 502，运行时
预热、失败、超时或线程池饱和为 503。数据路由遇到运行时预热时会附带
`Retry-After: 1`。

## 测试

```bash
.venv/bin/python -m pip install --editable '.[runtime,test]'
.venv/bin/pytest
```

测试通过 `httpx.ASGITransport` 和 pandas DataFrame fixture 模拟两个数据源，
并全局阻止 socket 连接；普通测试不会访问 Yahoo Finance 或 AKShare 网络。

真实 AKShare 闭环只通过显式开关手动运行，不进入普通 pytest/CI：

```bash
JFTRADE_AKSHARE_LIVE_SMOKE=1 \
  .venv/bin/python scripts/akshare_live_smoke.py
```

脚本验证 AKShare 导入/health，并真实调用 US.AAPL 的 search、snapshot 和日 K；
未设置开关时只输出 `SKIP`，不会导入 AKShare 或访问网络。
