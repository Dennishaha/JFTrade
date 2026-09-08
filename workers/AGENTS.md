# Worker 局部指令

继承根 [AGENTS.md](../AGENTS.md)。只运行所修改 worker 的局部测试；跨 worker/Rust 契约变化同时验证对应消费者。以下命令从仓库根目录运行。

## PineTS worker

- `pineworker/src/main.ts` 是 Node ESM gRPC 入口；PineTS 精确版本以 `pineworker/package.json` 和根锁文件为准。
- `pinetsExecutor.ts` 只保留 session orchestration；静态预检、结果压缩、source 归一化分别由 `pinetsStaticPreflight.ts`、`pinetsResult.ts`、`pinetsSource.ts` 持有。
- 只产出信号、图形和 order intents，不接管 Rust 的撮合、成交、资金曲线、风控和券商下单。
- 修改前读 [PineTS 契约](../docs/pinets-contract-audit.md)；资产或进程生命周期变更再读 [worker 发布排障](../docs/troubleshooting/pinets-worker-release.md)。不手改打包后的 `worker.mjs`。

```bash
pnpm --filter @jftrade/pineworker run test <test-file>
pnpm run typecheck:pineworker
```

## Python market-data helper

- 入口为 `marketdata-sidecar/src/marketdata_sidecar/main.py`；能力、环境准备和内部 HTTP 契约见 [helper README](marketdata-sidecar/README.md)，产品侧 owner 见 [行情数据源](../docs/market-data-providers.md)。
- yfinance/AKShare 运行时必须隔离；不在 helper 内做跨 Provider 静默回退，不承担交易职责。
- 普通测试通过 ASGI transport 和 fixture 模拟上游，并阻止真实网络。保留 `pyproject.toml` / `uv.lock` 的锁定环境，不依赖某台机器的 `.venv/bin/python` 路径。

```bash
uv run --locked --project workers/marketdata-sidecar --extra runtime --extra test pytest workers/marketdata-sidecar/tests/<test-file>
pnpm run check:python
```

局部验证后运行 `pnpm run check:quick`；涉及 Rust 消费者的实现变更按根指令追加 Rust 门禁。
