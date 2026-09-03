# PineTS diagnostic engine

JFTrade 的生产 Pine 标识为 `sourceFormat=pine-v6`、`runtime=pine-pinets`。`pinets-shadow`
是保留在协议中的诊断引擎名称，用于能力说明和 MCP/spec 输出；它不是第二个生产执行 owner，
也不会新增公开 HTTP 端口。

## 当前职责

- `workers/pineworker` 运行固定版本的 PineTS，产出信号、图形、告警和 order intents。
- `crates/jftrade-integration-pine` 负责 worker 启动、鉴权、请求、超时和关闭。
- `crates/jftrade-backtest` 与 `crates/jftrade-engine` 负责撮合、成交、资金曲线、风控、持久化和下单。
- `scripts/pinets-worker.mjs` 只保留诊断用 newline-delimited JSON 协议，不是生产回测 fallback。

`JFTRADE_PINETS_MODE=shadow` 或 `community-agpl` 只影响诊断输出；生产 Pine worker 仍由
Rust composition 明确注入。`JFTRADE_PINETS_WORKER_PATH` 仅用于开发测试覆盖 worker 路径。

## 回归入口

Rust MCP/spec contract replay：

```bash
pnpm run test:pinets-shadow-corpus
```

真实 PineTS 进程 smoke：

```bash
pnpm run smoke:pinets-backtest
```

后一个命令通过 Rust `PineProcess` 启动 bundled Node worker，验证 authenticated readiness，
执行原生 PineTS `RunScript`，检查 metadata/plots，并在结束时停止子进程。普通回归使用固定
fixture，不读取操作者数据库，也不连接真实行情或交易账户。

冻结 fixture 中的历史 shadow/来源描述只用于兼容 replay；不得用旧实现更新 fixture，
也不得把诊断引擎结果解释为生产撮合或实盘授权。
