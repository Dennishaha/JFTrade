# 行情数据源

JFTrade 的行情查询与交易执行是两个独立边界。运行时内置 Futu OpenD 和 Yahoo Finance（`yfinance`）两种行情 Provider；首页/研究页的“行情提供者”菜单默认使用内置 yfinance，并负责切换到 Futu OpenD。账户、持仓、订单与真实下单仍只走已配置的 Futu OpenD broker。

新安装默认使用 Yahoo Finance（`yfinance`），适合美股、港股和沪深的证券搜索、延迟快照与历史 K 线分析，不应当作实时交易报价。已有明确选择的 `futu` 或 `yfinance` 会保留；旧版 yfinance 连接配置会在加载时清理并回退到 Futu。

## 能力对比

| 能力 | Futu OpenD | Yahoo Finance（`yfinance`） |
| --- | --- | --- |
| 支持市场 | 由 OpenD 权限和 JFTrade 映射决定 | `US`、`HK`、`SH`、`SZ`；前端将 `SH`/`SZ` 聚合为“沪深”，`CN` 必须带 `SH.` 或 `SZ.` 前缀 |
| 证券搜索与详情 | 支持 | 支持 |
| 行情快照 | 支持，时效取决于行情权限 | 支持；统一标记为约 15 分钟延迟 |
| 历史 K 线 | 支持 | 支持 `1m`、`5m`、`15m`、`30m`、`1h`、`1d`、`1w`、`1mo` |
| 实时推流 | 支持 | 不支持；JFTrade 只按需轮询 |
| Level 2 盘口 | 取决于权限 | 不支持 |
| 盘前盘后 | 支持 | 仅美股在 Yahoo 实际提供报价时展示盘前/盘后；港股和沪深只使用本地交易时段 |
| 外部依赖 | Futu OpenD `>= 10.9.6908` | 发布版内置 PyInstaller helper；不要求用户安装 Python |

Yahoo Finance 接口不是官方稳定 API，也没有实时性或可用性承诺。JFTrade 会把缺失值安全映射为 `null`，并把上游失败转换为结构化错误，但无法消除上游限流、字段变化或临时不可用。

## 内置 helper

桌面版和带 `release_assets` 的 `cmd/jftrade-api` 会嵌入目标平台的 PyInstaller 单文件 helper。JFTrade 在启用内置 yfinance Provider 时自动释放到权限受限的临时目录，分配动态 loopback 端口，探测 `/health` 后注入内部 Provider endpoint；切回 Futu 或退出应用时停止进程并删除临时文件。helper 缺失、启动失败或健康探测失败会回退并持久化 Futu；helper 不会无限自动重启，也不会监听公网。

设置页不提供行情 Provider 分类，也不提供 Python、host、port、enabled 或 timeout 配置。首页/研究页的“行情提供者”菜单只展示可用 Provider，并负责切换到 Futu OpenD。

`pnpm run desktop:dev` 会优先复用当前平台的已构建 helper；如果 helper 不存在且仓库内的 `workers/yfinance-sidecar/.venv` 可用，启动脚本会自动构建并通过 `JFTRADE_YFINANCE_SIDECAR` 注入桌面进程。没有该虚拟环境时，可先按 sidecar README 安装 `[runtime,build]` 依赖。独立运行 `cmd/jftrade-api` 时，可通过绝对路径指定本地 helper：

```bash
JFTRADE_YFINANCE_SIDECAR=/absolute/path/to/yfinance-sidecar-darwin-arm64 \
  go run ./cmd/jftrade-api
```

该环境变量仅用于开发和测试，不写入 `settings.json`，也不会出现在用户界面。构建 helper 的依赖和 PyInstaller spec 位于 `workers/yfinance-sidecar`；目标平台构建命令为 `pnpm run build:yfinance-sidecar`。

## 验证

正常运行时 helper endpoint 是 JFTrade 的内部动态地址，不会写入配置或暴露给前端。开发 API 可以查询统一状态接口：

```bash
curl http://127.0.0.1:3000/api/v1/market-data/provider
```

helper 的 `/health` 不访问 Yahoo 网络；只有开发者做 standalone smoke 时才直接调用它。此时使用 helper CLI 的临时测试端口，不要把该地址写入 `settings.json`。JFTrade 对外行情接口仍是统一的 `/api/v1/market-data/*`，前端不会直接调用 Python sidecar。

如果 Provider 状态不可用或 helper 启动失败，请按 [yfinance sidecar 排障](./troubleshooting/yfinance-sidecar.md) 检查嵌入资产、开发态路径和上游网络。

## 持久化配置

普通用户无需配置 yfinance。新安装的 `settings.json` 只记录默认 Provider：

```json
{
  "activeMarketDataProvider": "yfinance"
}
```

如果检测到旧版顶层 `yfinance` 配置块，加载时会删除该块、强制选择 Futu 并持久化；yfinance 仅作为内置默认 Provider 由运行时管理。`JFTRADE_YFINANCE_SIDECAR` 是开发环境变量，不属于持久化配置。

运行中的实盘策略不会被静默迁移到延迟行情：切换 Provider 或重配当前 yfinance 前必须先停止全部实盘策略。yfinance 激活期间也不能启动实盘策略；回测不受此限制。

## 开发与测试

sidecar 测试会 mock `yfinance` 并阻止真实 socket 连接：

```bash
python -m pytest workers/yfinance-sidecar/tests
```

Go 侧 Provider、运行时切换与统一行情 service 的测试分别位于：

- `internal/integration/yfinance`
- `internal/app/apiserver/marketdataapp`
- `internal/marketdata`

公开设置或行情 HTTP 契约发生变化后，仍需执行 `pnpm run generate:docs`。
