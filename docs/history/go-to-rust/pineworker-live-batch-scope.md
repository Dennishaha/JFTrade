# PineTS 生成器与 live 域对齐批次（第 98 批）

本文件记录 Go 基线 `cmd/generate-pineworker-proto`（PineTS proto 生成流水线）
与 `internal/live`（订阅归一化、客户端注册表、重放发布器、宿主通知投递）
迁移到 Rust 的逐测试核对结论，涉及 `crates/jftrade-integration-pine`、
`crates/jftrade-api` 与 `crates/jftrade-engine`。

## 第九十八批：两域收口（24 条）

### 范围与分片

两域各 12 条 `missing`，共 24 条：

- P0 `cmd/generate-pineworker-proto`：`generator_test.go` 5（输出替换、退出码传播、
  行数限额、同名碰撞、生成前输入校验）、`output_test.go` 3（平铺、碰撞、行数语义）、
  `main_test.go` 4（CLI 解析、非法参数、help、退出码）。
- P1 `internal/live`：`client_test.go` 3（订阅归一化、注册表活跃标的、快照隔离与合并）、
  `publisher_test.go` 5（序号窗口、UTC 归一、过期淘汰、关闭一次、并发发布）、
  `lifecycle_boundaries_test.go` 3（nil 边界、Start 错误、Start/Close 竞态）、
  `notification_delivery_test.go` 1（宿主通知结果显式化）。

### 关键事实（本批 recon 实测）

- PineTS proto 生成器与第 96 批的 Futu 生成器同类：Go 侧流程
  （`verifyPineworkerInputs` → protoc `--go_out/--go-grpc_out` 到暂存目录 →
  按 basename 平铺 → 行数限额 → 替换 `pkg/strategy/pineworker/pineworkerpb`）
  在 Rust 中不存在；`crates/jftrade-integration-pine/build.rs` 用 tonic-prost
  在构建期把 `proto/pineworker` 编译进 `OUT_DIR`，仓库不保留生成的 `.pb.go`。
- 冻结输入恰为三个文件：`pineworker.proto`、`pineworker_common.proto`、
  `pineworker_types.proto`（与 Go 的 `pineworkerProtoFiles` 一致），全部平铺、
  同属 package `jftrade.strategy.pineworker.v1`；本批实测整树 digest
  （相对名 + 0x00 + 内容 + 0x00，按名排序累计 sha256）= `7c9a329c…`（3 个文件）。
- `internal/live` 的订阅归一化在 Rust 有两处 owner：`crates/jftrade-api` 的
  `LiveHub`（活跃标的、provider、订阅释放）与 `crates/jftrade-engine` 的
  `normalized_subscriptions`（完整表：security details 与 depth 夹取 `[1,50]`、
  去重排序、provider 小写、consoleRefresh 透传）。
- Go 的 `ReplayPublisher`（自增序号 + retention/capacity 重放窗口 + `After(seq)`）
  在 Rust 被**架构替换**：`LiveHub` 使用有界 broadcast，慢消费者先收到
  `live.resync` 控制事件（`reason=broadcast_lagged`、`action=resubscribe`、
  `droppedEvents`）再继续接收新事件，客户端按 resync 重新同步而非按序号补发。

### 结果

- 24 条全部给出结论：**4 条 `[x]`/`function_exact` + 5 条 `partial` + 15 条 `boundary`**。
  - `cmd/generate-pineworker-proto`（12）= 1 `[x]` + 2 partial + 9 boundary，域内 `missing` 归零。
  - `internal/live`（12）= 3 `[x]` + 3 partial + 6 boundary，域内 `missing` 归零。
- 全局：4451 = function_exact **1276** + partial **2527** + boundary **567** + module_only 4 +
  missing **77**（前批 1272 / 2522 / 552 / 4 / 101），Rust 测试 2976 → **2983**。
- 锚点对账：anchors 1272 → **1278**（本批 6 个新锚点全部已记账），unrecorded 保持 **0**。

### 本批新增测试

新增 7 条测试，其中 4 条对应 `[x]` 行：

- `pineworker_proto_inputs_are_verified_before_any_generation`（`[x]`）：
  输入集合恰为记录的三文件、`build.rs` 引用全部输入并 `compile_protos`，
  临时目录删掉一个输入后复现 `missing Pineworker proto file: <path>`。
- `pineworker_proto_tree_matches_the_frozen_digest`：整树 digest 等于冻结值、恰好 3 个
  平铺 `.proto`、均非空（编辑/删除/新增文件即转红）。
- `pineworker_proto_tree_keeps_the_canonical_package_and_imports`：每个文件声明
  `syntax = "proto3"` 与 canonical package、每个 import 都能从仓库根解析，
  且 `execution.rs`/`mock_worker.rs` 的 `include_proto!` 指向同一 package。
- `subscription_normalization_matches_the_go_table`（`[x]`）：用 Go 的同一张表断言
  `normalized_subscriptions` 的 active/security/depth/provider/consoleRefresh 结果。
- `live_hub_snapshot_is_isolated_and_subscriptions_are_replaced`：快照是拥有值（改动不回写）、
  后一次订阅替换而非合并、断开后清空。
- `live_hub_shutdown_is_idempotent_and_rejects_new_sessions`：重复 `begin_shutdown`/`mark_stopped`
  保持状态、shutdown 信号置真、shutdown/stopped 后 `try_connect` 返回 None。
- `system_notification_delivery_keeps_the_host_outcome_explicit`（`[x]`）：
  `/api/v1/settings/system-notifications/test` 在投递成功与设置过滤两条路径上
  分别返回 `delivered/status/message` 的显式结果。

### 探针（改坏 → 转红 → 按字节回滚）

三次探针全部字节复原（shasum 记录于 `/tmp/b98_probe.shasum`）：

1. `proto/pineworker/pineworker.proto` 追加注释 → digest 守卫转红
   （`0527ebc0…` ≠ `7c9a329c…`）→ 回滚后 `c2e25d5b…` 与探针前一致。
2. `normalized_subscriptions` 的 depth 上界 `50` 改为 `100` →
   `subscription_normalization_matches_the_go_table` 转红（多出 `US.TME|100`）→
   回滚后 `product_ws_live.rs` 恢复 `21e49c72…`。
3. 通知过滤文案改为 `notification filtered` → 通知投递测试转红 →
   回滚后 `product_api.rs` 恢复 `99d1dbee…`。

### 保留差异（记录在 `partial`/`boundary` 结论里）

- PineTS 生成器：无 protoc runner、无退出码、无暂存目录替换、无 basename 平铺、
  无生成行数预算、无 CLI；这些语义随 Go 命令退场，由「冻结输入 + 构建期编译」
  的不变式替代。
- live 重放：Rust 无服务端重放窗口与全局序号，改为「有界广播 + 显式 resync」；
  因此「序号连续无丢失」与「过期淘汰」两类断言不迁移。
- `live.Updated()` 的唤醒合并、`Start(source)`/`Close` 的来源回调、nil 接收者语义
  在 Rust 没有对应对象（缺失 owner 一律 fail-closed 或以 `Option`/RAII 表达）。

### 后续待办

- 下一批：`internal/productfeatures`（12）→ `pkg/observability`（10）→ `cmd/internal`（9）→
  `internal/pineworkerassets`（7，锚点例外）→ `internal/desktop`（6）→ `internal/retry`（6）→
  `internal/jftsettings`（5）→ `internal/frontendassets`（4，锚点例外）→ `internal/research`（4）→
  `internal/security`（3）→ 其余小片，直到 4451 条清单全部完成。
- 若未来需要「断线重连后补齐事件」，必须在客户端侧实现按 resync 重新拉取，
  不要把服务端重放窗口重新引入 LiveHub（会与有界广播的丢弃语义冲突）。

`pnpm run check:quick` **EXIT=0**：target-health、policy/contracts、`cargo-nextest -p jftrade-desktop -p jftrade-engine`
（**1853 passed / 1 skipped**）、fmt、clippy、7 组 compatibility replay、pineworker 与 desktop/Node 套件全部通过。
首次 quick 因长时间 nextest 运行后 `target/debug/deps` 的中间 `.rcgu.o` 达到 60941 而失败；确认无 cargo/rustc
进程后执行 `pnpm run clean:rust:artifacts`（`cargo clean`，移除 137566 个文件 / 37.9GiB 可再生产物），
target-health 恢复后重跑 quick 通过。

`pnpm run check:rust` **EXIT=1**，唯一失败项与本批 diff 无关且是既有基线失败，保留原始证据不记为通过：
target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 全部通过后，`check:rust:policy`
的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；根 `Cargo.toml`
精确锁定 `=0.23.44`），另有 **8 条** `warning[advisory-not-detected]`（陈旧 ignore）；run 在 policy 处中止，
因此该次运行的 workspace 测试阶段未执行（本批的 crate 级证据以上面的 1833 条 nextest 为准）。本批未改动
`Cargo.toml`/`Cargo.lock`。

验证：`cargo fmt --all`（EXIT=0）；`cargo clippy -p jftrade-integration-pine -p jftrade-engine -p jftrade-api --all-targets --locked`（EXIT=0，无告警）；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine -p jftrade-engine -p jftrade-api --all-targets --locked --no-fail-fast`（**1833 passed / 1 skipped**，EXIT=0）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2983 Rust** / **1276 `[x]`**；missing 77、partial 2527、boundary 567、module_only 4；`OK: 1276 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors **1278**；already recorded 1189、**unrecorded 0**、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0）；`node scripts/check-zero-go.mjs`（2901 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`（EXIT=0）。

