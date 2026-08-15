# Worker 局部指令

- `workers/pineworker` 是固定 PineTS `0.9.31` 的 Node ESM gRPC worker，只产出信号、图形和 order intents。
- `pinetsExecutor.ts` 只保留 session orchestration；静态 route 预检、结果压缩和 source 归一化分别由 `pinetsStaticPreflight.ts`、`pinetsResult.ts`、`pinetsSource.ts` 持有。
- `workers/marketdata-sidecar` 是本地 Python helper；普通测试禁止真实网络，使用 ASGI transport 和 fixture。
- Go 负责撮合、成交、资金曲线、风控和券商下单；不要把这些职责放入 worker。
- 最小验证：`pnpm --filter @jftrade/pineworker run test`、`pnpm --filter @jftrade/pineworker run typecheck`、`workers/marketdata-sidecar/.venv/bin/python -m pytest workers/marketdata-sidecar/tests`。
