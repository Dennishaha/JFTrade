# PineTS Worker 发布与运行清单

本文面向发布、部署和现场排障。当前 Pine 执行路径是 Go 主进程启动 Bun/PineTS gRPC worker pool；Go 仍负责行情、K 线缓存、策略调度、回测撮合、资金曲线、风控、下单和交易所 API。

## 发布放行条件

发布二进制必须同时满足：

- 商业 `pinets` 包已按许可证策略安装、锁定并记录版本。
- worker 以真实 PineTS executor 启动，未启用 mock。
- 真实 worker 进程通过 localhost gRPC smoke，覆盖 `HealthCheck` 和 `RunScript`。
- `scripts/build-pineworker-assets.sh` 生成目标平台 worker 二进制。
- `go test -tags release_assets ./internal/pineworkerassets -run Test` 通过，确认 embedded asset 选择逻辑可用。
- Go、worker、前端 focused test、coverage、performance gate 和 `git diff --check` 通过。

当前仓库还未满足最终发布：`npm ls pinets --workspaces --depth=1` 为空，真实非 mock PineTS worker 进程 smoke 还不能作为放行依据。

## 运行模式

开发态优先使用外部 worker：

```bash
export JFTRADE_PINEWORKER_BINARY=/absolute/path/to/worker-darwin-arm64
go run ./cmd/jftrade-api
```

发布态使用 embedded worker：

```bash
bash scripts/build-pineworker-assets.sh
go build -tags release_assets ./cmd/jftrade-api
```

API 启动时会先读 `JFTRADE_PINEWORKER_BINARY`。未配置外部二进制时，`release_assets` 构建会按当前平台选择内嵌 worker。若两者都不可用，Pine worker manager 不启动；回测和实盘策略执行会失败返回配置错误，不会回退到旧 Go 执行器。

## 环境变量

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `JFTRADE_PINEWORKER_DISABLED` | `false` | 显式关闭 worker manager。 |
| `JFTRADE_PINEWORKER_BINARY` | 空 | 外部 worker 二进制路径；配置后优先于 embedded asset。 |
| `JFTRADE_PINEWORKER_SHA256` | embedded asset hash 或运行时计算值 | 校验外部 worker 二进制。 |
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
bash scripts/build-pineworker-assets.sh
go test -tags release_assets ./internal/pineworkerassets -run Test
go test ./pkg/strategy/pineworker -run Test -cover
go test ./pkg/strategy/pineworker -bench BenchmarkCheckPerformanceGate -run '^$' -benchmem
go build -tags release_assets ./cmd/jftrade-api
```

也可以直接运行发布验收脚本：

```bash
npm run check:pinets-release
```

迁移阶段如果只是要确认除商业 `pinets` 包之外的门禁，可以使用：

```bash
bash scripts/check-pinets-release.sh --allow-blocked
```

发布脚本自身的阻塞/放行分支可以用 stub 测试验证：

```bash
npm run test:pinets-release-check
```

在商业 `pinets` 包未安装前，`npm install` 和真实 worker 运行不能代表最终放行。构建产物也不应进入正式发布。

## 非 mock smoke

放行前需要手动或自动执行一次真实 worker 进程 smoke：

```bash
unset JFTRADE_PINEWORKER_MOCK
export JFTRADE_PINEWORKER_BINARY=/absolute/path/to/worker-linux-x64
go test ./internal/app/apiserver/servercore -run TestResolvePineWorkerRuntimeConfigDefaultsToRealPineTSWorker -v
```

随后用实际 worker 发起 `HealthCheck` 和一段小 K 线 `RunScript` 请求。命令行或日志中不应出现 `--mock true`。

当前已有的 `JFTRADE_PINEWORKER_PROCESS_SMOKE=1 go test ./pkg/strategy/pineworker -run TestWorkerManagerProcessSmokeWithBunWorker -v` 只证明 Bun worker、进程生命周期和 gRPC 边界可用；它使用 mock executor，不能替代真实 PineTS smoke。

## 排障顺序

1. 先看 API 日志是否出现 `JFTrade PineTS worker manager started`。
2. 若提示未配置 worker，检查 `JFTRADE_PINEWORKER_BINARY` 或是否使用了 `release_assets` 构建。
3. 若提示 checksum 失败，重新计算外部 worker 的 SHA256 或清空 `JFTRADE_PINEWORKER_SHA256` 让启动时按当前二进制计算。
4. 若 worker 启动但请求失败，先调低 worker 数量并检查端口区间是否被占用。
5. 若回测失败为超时或性能门禁，检查 K 线数量、gRPC message size、timeout 和 performance gate。
6. 若实盘没有下单，先确认 worker 返回的是当前已收盘 K 线的 order intent，再看 Go 风控和 notify-only 设置。
