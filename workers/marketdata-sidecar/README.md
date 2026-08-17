# JFTrade market-data sidecar

这个仅监听 loopback 的 Python 服务为 JFTrade 同时承载 `yfinance` 和
`AKShare` 行情适配。两个 Provider 拥有相互隔离的懒加载运行时；进程启动和
`/healthz` 都不会导入数据源或访问网络，一个 Provider 初始化失败也不会影响
另一个 Provider。

## 能力边界

- yfinance 保留 US、HK、SH、SZ 的搜索、详情、延迟快照和八种周期 K 线。快照优先
  走 `Ticker.fast_info` 快速路径，字段缺失或出错时回退 `get_info`；K 线支持
  `none/forward` 复权（`forward` 为 Yahoo auto_adjust）；另有新闻与公司行动路由。
- AKShare 支持上述四市场的市场描述、搜索、详情、单/批量快照和 K 线；沪深
  另有 ETF 与上证/深证/中证指数，港股包含 AKShare 指数目录，美股指数限定
  为 DJIA、SPX、NDX。沪深快照填充东财买一/卖一；详情含市值、动态市盈率（f9，
  非 TTM）、市净率（f23）和总股本（沪深 A 股经 `stock_individual_info_em`
  补全，24 小时缓存，失败静默降级）。沪深股票/ETF 的 `1d/1w/1mo` 支持
  qfq/hfq 复权（ETF 经东财接口取数）；另有沪深新闻（`stock_news_em`）、
  分红送转（`stock_fhps_em`）和指数成分股路由，美港标的返回
  `AKSHARE_UNSUPPORTED`。
- AKShare 不提供推流、Level 2、盘前盘后或交易能力。它失败时返回明确错误，
  不会在 sidecar 内回退到 yfinance 或 Futu。
- AKShare 全市场目录缓存 15 秒，并按目录 singleflight。批量快照先按市场各取
  一次目录，再在本地解析最多 100 个标的，不逐标的请求上游。
- AKShare 阻塞调用使用独立的四线程池，每次最多等待 12 秒。四个槽位都被
  在途或已超时但仍未结束的调用占用时立即返回 `AKSHARE_POOL_BUSY`。
- 个股研究能力（profile/financials/analyst/ownership）两个 Provider 均提供：
  yfinance 覆盖 US/HK（info、年度三表、recommendationTrend、holders），
  AKShare 覆盖沪深（东财 F10 个股资料、年度三表、个股研报评级聚合、十大股东）
  与港股公司资料；研究类数据进程内缓存 1 小时。

AKShare 数值使用十进制字符串，避免经 JSON `float` 丢精度。缺失的 volume
和 turnover 保持 `null`；买一/卖一仅在上游提供时填充，未服务市场保持
`null`；沪深以“手”返回的成交量乘以 100 后输出。
时间统一为 RFC 3339 UTC，没有真实行情时间时只填 `observed_at`，不伪造
`quote_at`。K 线默认使用 `adjust=""` 的不复权数据，非法/非有限 OHLC 会被丢弃；
沪深股票/ETF 的日线及以上可使用 qfq/hfq 复权，其余品种与分钟周期的非
`none` 复权返回 400 `UNSUPPORTED_RANGE`。

## 安装和启动

需要 Python 3.11 或更高版本：

```bash
cd workers/marketdata-sidecar
python3.11 -m venv .venv
python3.11 -m pip install --disable-pip-version-check uv==0.12.5
uv sync --locked --extra runtime
uv run --locked --extra runtime marketdata-sidecar --host 127.0.0.1 --port 7788
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
uv sync --locked --extra runtime --extra build
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
| GET | `/providers/{source}/news/{market}/{symbol}?limit=10` | 新闻条目，limit 为 1–50；AKShare 仅沪深 |
| GET | `/providers/{source}/corporate-actions/{market}/{symbol}?from=&to=` | 分红/拆分事件，RFC3339 包含式边界，默认最近两年；AKShare 仅沪深 |
| GET | `/providers/akshare/index-constituents/{market}/{symbol}?limit=200` | 中证/沪深交易所指数成分股，limit 为 1–1000；仅供 assistant 工具使用 |
| GET | `/providers/akshare/rankings?market=&kind=&limit=20` | 沪深/港股涨跌幅、成交额榜单（本地排序目录快照），limit 为 1–100 |
| GET | `/providers/yfinance/rankings?market=US&kind=&limit=20` | 美股 Yahoo 预定义榜单（day_gainers/day_losers/most_actives） |
| GET | `/providers/akshare/industries?kind=industry\|concept` | 东财行业/概念板块列表；成员见 `/industries/{name}/members?limit=100` |
| GET | `/providers/{source}/profile/{market}/{symbol}` | 公司资料分组；yfinance 限 US/HK，AKShare 限沪深/HK |
| GET | `/providers/{source}/financials/{market}/{symbol}?statement=income\|balance\|cashflow` | 年度财务三表，近 4 期；AKShare 限沪深 |
| GET | `/providers/{source}/analyst/{market}/{symbol}` | 分析师评级聚合；yfinance 限 US/HK，AKShare 限沪深（个股研报聚合） |
| GET | `/providers/{source}/ownership/{market}/{symbol}` | 股权结构分组；yfinance 限 US/HK，AKShare 限沪深十大股东 |
| GET | `/providers/akshare/calendar/earnings?begin_date=&end_date=` | 沪深财报披露日历（东财预约披露）；窗口按披露季映射报告期（年报次年 1–4 月、一季报 4 月、中报 7–8 月、三季报 10 月），最多跨 6 个报告期；`event_date` 取实际披露时间、回退首次预约；`market_cap`/`price` 恒 null |
| GET | `/providers/akshare/calendar/dividends?date=` | 沪深单日除权除息日历（东财分红送配）；`payable_date` 恒 null（上游无派息日列） |
| GET | `/providers/akshare/calendar/economic?begin_date=&end_date=` | 百度财经宏观事件日历，窗口 ≤31 天；`event_timestamp` 按 Asia/Shanghai 换算，无时刻的全天事件为 null；前值/预期/公布为数值字符串，不含单位 |
| GET | `/providers/akshare/calendar/ipos` | 沪深新股日历（东财新股申购）；北交所跳过，`issue_price_min`/`issue_price_max` 恒 null |
| GET | `/providers/akshare/macro/indicators` | 宏观指标静态目录（16 个指标，中国/美国分组）；AKShare 1.18.x 无联邦基金利率接口，未收录 |
| GET | `/providers/akshare/macro/indicator-history?indicator_id=&limit=100` | 单指标历史序列，`data_time` 统一 `YYYY-MM`，倒序；未知 `indicator_id` 返回 404 |
| POST | `/providers/yfinance/screen` | 美股自定义筛选（EquityQuery 翻译，仅 US）；body `{"market","conditions":[{"factor_key","min","max"}],"sorts":[{"factor_key","direction"}],"offset","limit"}`；单页 limit 1–250，offset 独立直传；仅接受一个排序键，多个排序键返回 400 `unsupported_kind` |
| POST | `/providers/akshare/screen` | 沪深/港股/美股筛选（CN/SH/SZ/HK/US），复用 15s 全市场现货目录本地过滤排序分页，零新上游请求；US/HK 现货帧由 `akshare_spot_clist` 直连东财 clist 构建（akshare 封装丢弃了 f23 市净率/f115 PE TTM 与 HK 总市值），六个因子全部市场可用；CN 成交量由手换算成股 |
| POST | `/providers/akshare/snapshots` | 最多 100 个 AKShare 批量快照 |

筛选因子目录（两 provider 统一，`in` 枚举条件暂不支持，收到即 400 unsupported_kind）：

| factor_key | yfinance 字段 | akshare 列 | 单位语义 |
| --- | --- | --- | --- |
| `simple.price` | `intradayprice` | 最新价 | 当日最新价 |
| `simple.change_pct` | `percentchange` | 涨跌幅 | % |
| `simple.volume` | `dayvolume` | 成交量 | 股（CN 帧为手，×100 换算；US/HK 帧即股） |
| `simple.market_cap` | `intradaymarketcap` | 总市值 | 盘中市值（US/HK 经 clist 直连补齐） |
| `simple.pe_ttm` | `peratio.lasttwelvemonths` | 市盈率-动态 | TTM/动态市盈率 |
| `simple.pb` | `pricebookratio.quarterly` | 市净率 | 季度市净率 |

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

K 线参数为 `period`、`limit`、`from`、`to`、`before`、`adjustment`。`adjustment`
取 `none`（默认）/`forward`/`backward`；yfinance 仅支持 `forward`，AKShare 仅
沪深股票/ETF 的 `1d/1w/1mo` 支持 `forward`/`backward`，非法取值返回
`unsupported_adjustment`。股票、沪深 ETF/指数支持
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
uv sync --locked --extra runtime --extra test
uv run --locked --extra runtime --extra test pytest
```

测试通过 `httpx.ASGITransport` 和 pandas DataFrame fixture 模拟两个数据源，
并全局阻止 socket 连接；普通测试不会访问 Yahoo Finance 或 AKShare 网络。

真实 Yahoo/AKShare 闭环只通过显式开关手动运行，不进入普通 pytest/CI：

```bash
JFTRADE_MARKETDATA_LIVE_SMOKE=1 \
  uv run --locked --extra runtime --extra test \
  python scripts/marketdata_live_smoke.py --provider all --suite full \
  --report /tmp/marketdata-live-report.json
```

脚本验证两个 Provider 的导入/health、核心行情、研究能力、筛选分页、错误拒绝和
AKShare 31 天经济日历；报告只包含端点、状态、耗时和条目数，不保存原始行情或财务数据。
未设置开关时直接拒绝运行，不会导入数据栈或访问网络。旧的
`JFTRADE_AKSHARE_LIVE_SMOKE=1 python scripts/akshare_live_smoke.py` 入口仍作为
AKShare 全量 smoke 的兼容包装保留。GitHub 手动 workflow 还要求输入
`RUN_LIVE_MARKETDATA` 确认文本，失败不会被转换为跳过。
