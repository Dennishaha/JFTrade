# PineTS Worker 发布与运行清单

本文面向发布、部署和现场排障。当前 Pine 执行路径是 Go 主进程启动 Bun/PineTS gRPC worker pool；Go 仍负责行情、K 线缓存、策略调度、回测撮合、资金曲线、风控、下单和交易所 API。发布打包采用非 SEA 的 Bun bundle，再由 Go `release_assets` 嵌入到 `trading-engine` 二进制；目标机器必须安装 Bun。

## 发布放行条件

发布二进制必须同时满足：

- 公开 `pinets` 包已按 npm lockfile 安装、锁定并记录版本；当前 `pinets@0.9.26` npm license 为 `AGPL-3.0-only`。
- 发布合规材料必须按公开 `pinets` 许可证准备；商业 PineTS 授权计划已取消。
- worker 以真实 PineTS executor 启动，未启用 mock。
- 真实 worker 进程通过 localhost gRPC smoke，覆盖 `HealthCheck` 和 `RunScript`。
- `npm run build:pineworker` 通过 `bun build --target=bun` 生成平台无关的 `worker.js` Bun bundle。
- `go test -tags release_assets ./internal/pineworkerassets -run Test` 通过，确认 embedded asset 选择逻辑可用。
- `go build -tags release_assets -o dist/trading-engine ./cmd/jftrade-api` 后的发布产物必须存在、非空且可执行。
- Go、worker、前端 focused test、coverage、performance gate、PineTS AGPL notice/source-offer check 和 `git diff --check` 通过。

当前仓库已锁定公开 `pinets@0.9.26`，真实非 mock PineTS worker 进程 smoke 已可作为放行依据；完整 strict release gate 已可通过 `npm run check:pinets-release` 在 Windows 和 CI 风格环境中直接运行。

## 运行模式

开发态优先使用外部 worker：

```bash
npm run dev:api:pineworker
```

如果需要手动控制 worker 路径，可以先构建当前平台 worker，再设置环境变量：

```powershell
$env:JFTRADE_PINEWORKER_BUNDLE = (npm run --silent build:pineworker:dev | Select-Object -Last 1)
go run ./cmd/jftrade-api
```

`npm run build:pineworker:dev` 和 `npm run dev:api:pineworker` 都走 Node 入口，不需要 Git Bash 或 WSL。设置 `JFTRADE_PINEWORKER_DEV_ENV_FILE` 时，dev build 会写出 `JFTRADE_PINEWORKER_BUNDLE=...` 和 `JFTRADE_PINEWORKER_RUNTIME=...` 供 VS Code launch 配置读取。

发布态使用 embedded worker：

```bash
npm run build:pineworker
go build -tags release_assets -o dist/trading-engine ./cmd/jftrade-api
```

`npm run build:pineworker` 会先确认 `pinets` 包已安装并输出其 license。未安装 `pinets` 时，脚本会在调用 `bun build` 前失败。`scripts/build-pineworker-assets.sh` 仍保留为兼容入口，但会转发到 Node 版 builder。

`npm run build:frontend-assets` 也使用 Node 入口重建 web dist、复制到 `internal/frontendassets/dist`，并调用 Go zip 工具生成 `dist.zip`；`scripts/build-frontend-assets.sh` 保留为兼容转发器。

`npm run test:pineworker` 和 worker package 的 `npm test` 通过 `scripts/run-bun.mjs` 启动 Bun。该入口会依次检查 `JFTRADE_BUN_BINARY`、`PATH`、`~/.bun/bin/bun(.exe)`，以及 Windows 常见 Bun 安装目录；因此 Windows 用户安装 Bun 后即使当前 shell 的 `PATH` 尚未刷新，也可以直接运行测试。

worker 资产构建采用 Bun bundle 路线：只生成一个平台无关的 `worker.js`，暂存到 `internal/pineworkerassets/assets/bin`，再通过 `release_assets` 构建嵌入 Go。运行时 Go 会释放 bundle 到临时目录、校验 SHA256，并通过目标机器上的 Bun 启动固定数量的 localhost gRPC 子进程，关闭时清理临时 bundle。Go 发布文件不再包含 Bun runtime，因此部署时必须把 Bun 放入 `PATH` 或设置 `JFTRADE_PINEWORKER_RUNTIME`。

API 启动时会先读 `JFTRADE_PINEWORKER_BUNDLE`。未配置外部 bundle 时，`release_assets` 构建会选择内嵌 `worker.js`。若两者都不可用，Pine worker manager 不启动；回测和实盘策略执行会失败返回配置错误，不会回退到旧 Go 执行器。

## 环境变量

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `JFTRADE_PINEWORKER_DISABLED` | `false` | 显式关闭 worker manager。 |
| `JFTRADE_PINEWORKER_BUNDLE` | 空 | 外部 `worker.js` bundle 路径；配置后优先于 embedded asset。 |
| `JFTRADE_PINEWORKER_RUNTIME` | `bun` | Bun 可执行文件路径。 |
| `JFTRADE_PINEWORKER_SHA256` | embedded asset hash 或运行时计算值 | 校验外部 worker bundle。 |
| `JFTRADE_PINEWORKER_WORKERS` | `DefaultWorkerConfig(runtime.NumCPU())` | worker 进程数量。 |
| `JFTRADE_PINEWORKER_HOST` | `127.0.0.1` | worker 监听主机。 |
| `JFTRADE_PINEWORKER_START_PORT` | `50051` | 第一个 worker 端口，后续 worker 递增。 |
| `JFTRADE_PINEWORKER_TEMP_DIR` | 系统临时目录 | 释放 embedded worker 的目录。 |
| `JFTRADE_PINEWORKER_PROTO` | worker 默认 proto 路径 | 传给 Bun worker 的 proto 文件路径。 |
| `JFTRADE_PINEWORKER_PINETS_VERSION` | 空 | 传给 worker 的 PineTS 版本标记。 |
| `JFTRADE_PINEWORKER_REQUEST_TIMEOUT` | worker 默认值 | 单次 `RunScript` 请求超时。 |
| `JFTRADE_PINEWORKER_HEALTH_TIMEOUT` | `5s` | worker 健康检查超时。 |
| `JFTRADE_PINEWORKER_MAX_MESSAGE_BYTES` | worker 默认值 | gRPC 收发消息上限。 |
| `JFTRADE_PINEWORKER_MAX_CANDLES` | worker 默认值 | 单请求 K 线数量上限。 |
| `JFTRADE_PINEWORKER_MAX_DURATION` | performance gate 默认值 | 单次执行最长耗时。 |
| `JFTRADE_PINEWORKER_MAX_DURATION_PER_BAR` | performance gate 默认值 | 单根 K 线最大平均耗时。 |
| `JFTRADE_PINEWORKER_MIN_CANDLES_PER_SEC` | performance gate 默认值 | 最低处理吞吐。 |
| `JFTRADE_PINEWORKER_MAX_PEAK_RSS_BYTES` | performance gate 默认值 | worker 峰值 RSS 上限。 |
| `JFTRADE_PINEWORKER_MOCK` | `false` | 仅测试使用；发布和生产环境不得启用。 |
| `JFTRADE_BUN_BINARY` | 自动探测 | 仅 npm worker 测试/start 辅助入口使用；指定 Bun 可执行文件路径。 |

`JFTRADE_PINEWORKER_MOCK=true` 会让 worker 使用确定性测试 executor。它只能用于 contract、manager 和 gRPC smoke 测试，不能作为生产降级方案。

## 建议 worker 数量

| 场景 | 建议 |
| --- | --- |
| 实盘 | `2` 到 `4` 个 worker，优先稳定低延迟。 |
| 普通回测 | CPU 核心数的一半，给 Go 撮合和系统保留资源。 |
| 参数优化 | 接近 CPU 核心数，并结合队列和 performance gate 观察吞吐。 |

worker 不长期持有全量历史 K 线；K 线主存储仍在 Go 侧。策略实例也不绑定固定 worker，调度由 `WorkerManager` 轮询健康 worker 完成。

## 发布构建步骤

```bash
npm install
npm run test:pineworker
npm run typecheck:pineworker
npm run build:frontend-assets
go test -tags release_assets ./internal/frontendassets -run TestFileSystem
npm run build:pineworker
go test -tags release_assets ./internal/pineworkerassets -run Test
go test ./pkg/strategy/pineworker -run Test -cover
go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem
go build -tags release_assets -o dist/trading-engine ./cmd/jftrade-api
```

也可以直接运行发布验收脚本：

```bash
npm run check:pinets-release
```

严格模式默认输出单文件 `dist/trading-engine`。临时验证其他输出路径时可以设置 `JFTRADE_PINETS_RELEASE_OUT`。

严格模式还会读取 `node_modules/pinets/package.json` 的 `license` 字段并打印出来，供发布记录和合规检查使用；公开 `AGPL-3.0-only` 包不会再因为缺少商业 attestation 被脚本阻塞。
严格模式同时运行 `npm run check:pinets-compliance`，确认 `docs/legal/third-party-notices.md` 记录生产 worker 集成、源码提供入口、上游地址、构建命令和 AGPL-3.0-only 许可证事实。

迁移阶段如果 `pinets` 包缺失导致 strict release 被阻塞，可以使用：

```bash
npm run check:pinets-release -- --allow-blocked
```

发布脚本自身的阻塞/放行分支可以用 stub 测试验证：

```bash
npm run test:pinets-release-check
npm run test:pineworker-asset-build
```

在真实非 mock PineTS worker smoke 通过前，构建产物不应进入正式发布。

## 非 mock smoke

放行前需要手动或自动执行一次真实 worker 进程 smoke：

```bash
unset JFTRADE_PINEWORKER_MOCK
JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE=1 \
  go test ./pkg/strategy/pineworker -run TestWorkerManagerRealPineTSProcessSmoke -v
```

这个测试会编译 Bun worker，以非 mock 模式启动 worker 进程，并通过 `WorkerManager` 发起 localhost gRPC `RunScript`。它要求 `pinets` 已安装；缺少 `pinets` 时测试会失败，表示发布仍被阻塞。命令行或日志中不应出现 `--mock true`。

当前已有的 `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` 只证明 Bun worker、进程生命周期和 gRPC 边界可用；它使用 mock executor，不能替代真实 PineTS smoke。

## 排障顺序

1. 先看 API 日志是否出现 `JFTrade PineTS worker manager started`。
2. 若提示未配置 worker，检查 `JFTRADE_PINEWORKER_BUNDLE` 或是否使用了 `release_assets` 构建。
3. 若提示 checksum 失败，重新计算外部 worker 的 SHA256 或清空 `JFTRADE_PINEWORKER_SHA256` 让启动时按当前二进制计算。
4. 若 worker 启动但请求失败，先调低 worker 数量并检查端口区间是否被占用。
5. 若回测失败为超时或性能门禁，检查 K 线数量、gRPC message size、timeout 和 performance gate。
6. 若实盘没有下单，先确认 worker 返回的是当前已收盘 K 线的 order intent，再看 Go 风控和 notify-only 设置。
