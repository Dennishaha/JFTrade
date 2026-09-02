# PineTS Worker 发布与排障

## 当前边界

- `workers/pineworker` 是 Node ESM gRPC worker，固定 `pinets@0.9.31`。
- `crates/jftrade-integration-pine` 持有 Rust gRPC client、process/readiness、pool 和 backtest execution adapter。
- worker 只输出信号、图形和 order intents；Rust engine 负责撮合、成交、资金曲线、风控和下单。
- protobuf 规范位于 `proto/pineworker`，Rust build script 与 Node runtime 消费同一中立契约。
- 发布资产准备到 `runtime-assets/pine`，再由 Tauri resources 打包。

## 日常验证

```bash
pnpm run test:pineworker
pnpm run typecheck:pineworker
cargo test -p jftrade-integration-pine --all-targets
pnpm run build:pineworker
pnpm run smoke:pinets-backtest
```

`smoke:pinets-backtest` 做三件事：构建真实 `worker.mjs`、由 Rust `PineProcess` 启动它、通过带 Bearer token 的 gRPC readiness 和 native PineTS `RunScript` 后再验证 bounded shutdown。缺少 `pinets`、Node runtime、proto 或 bundle 会 fail closed。

完整 release gate：

```bash
pnpm run check:pinets-release
pnpm run prepare:tauri-release
pnpm run check:tauri-release-runtime
pnpm run check:zero-go
```

## 运行配置

- `JFTRADE_PINEWORKER_BUNDLE`：开发/测试时指定绝对 `worker.mjs`。
- `JFTRADE_PINEWORKER_RUNTIME` 或 `JFTRADE_NODE_BINARY`：Node executable。
- `JFTRADE_PINEWORKER_PROTO`：Pine worker 主 proto。
- `JFTRADE_PINEWORKER_TOKEN`：至少 32 字符的受管内部 token。

正式 Tauri 产品从受管 resources 注入路径，不依赖用户全局 Node 或工作目录。外部手工 worker 只允许开发/测试显式配置。

## 常见失败

1. readiness timeout：检查 Node 路径、bundle/proto 是否存在、端口是否被占用和 worker stderr。
2. unauthenticated：确认 Rust client 与 worker 使用同一 token，且 token 未出现在命令行或日志。
3. protocol decode：确认 Rust/Node 都消费 `proto/pineworker`，并运行 `check:generated`/integration tests。
4. native PineTS 失败：确认精确版本 `0.9.31`、sourceFormat/runtime 和请求 candle 边界。
5. shutdown 遗留：检查 `PineProcess` kill-on-drop、stop timeout、pool cancellation 和 Tauri runtime resource report。
6. 发布包缺资产：先运行显式 prepare，再检查 Tauri bundle resource manifest 和 SHA-256。

`0.29.0` 发布还要求最终 Tauri bundle、candidate inputs 与 SBOM/provenance 通过零 Go/Wails 扫描；本地 worker smoke 不能替代四平台签名安装、升级/回滚或 post-release smoke。
