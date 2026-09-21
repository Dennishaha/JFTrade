# System 域对齐批次（第 97 批）

本文件记录 Go 基线 `internal/system`（System 状态投影、真实交易控制面读取、
Futu/OpenD 健康、交易所日历委托、运行时依赖与存储概览）迁移到 Rust
`crates/jftrade-engine`（`product_api_system_status`、`product_system_control_read_tests`、
`product_system_write_product_tests`、`product_production_assembly_tests`、
`real_trade_control`）的逐测试核对结论。

## 第九十七批：`internal/system` 全域收口（14 条）

### 范围与分片

`internal/system/**` 剩余 **14 条 `missing`**（2 个文件），分三片：
P0 状态与真实交易控制面 6 条（`service_test.go` 前 6：status 字段/动态 provider、
默认前端契约、风险快照读取、typed-nil 归一、控制面委托与不可用边界）→
P1 Futu/OpenD 健康与重置 3 条（`service_test.go` 后 2 + 观测默认不可用）→
P2 观测/日历/存储/依赖 5 条（`service_status_defaults_test.go` 全部）。

### 关键事实（本批 recon 实测）

- **发现并修复真实功能差异**：Go 的 `/api/v1/system/status` 每个请求都从持久化
  execution 设置读取 `defaultTradingEnvironment`（`servercore/trading_adapters.go`
  的 `defaultTradingEnvironment(s)` → `store.ExecutionSettings().DefaultTradingEnvironment`，
  仅 store 为 nil 时才回落 `SIMULATE`），而 Rust `system_status()` 把该字段**硬编码为
  `"SIMULATE"`**。用户在设置里把默认环境改成 `REAL` 后，Go 状态正确、Rust 状态错误。
- Go 的六个真实交易控制命令（update/disable runtime risk、activate/release kill switch、
  activate/release hard stop）是六个独立注入回调；Rust 合并为单一 `SystemWritePort`
  的七种 operation（含 `manual-retry`），唯一写入 owner 不变。
- Go 的若干「未注入回调」占位分支（install guide 空 map、order snapshot 空 map、
  RuntimeDependencies 静态 satisfied、日历 `accepted:false + reason`）只在
  `system.NewService()` 无回调时出现；真实 Go 服务器始终注入回调
  （`server_calendar.go` 未配置日历时应答 `{"accepted": false}` 且**无 reason**），
  因此这些分支属于 Rust 组合根 owner 退场后的边界，不强行迁移。
- Rust 状态投影的注入缝是 `ProductConfig` 的端口（`resource`/`calendar_manager`/
  `market_data_runtime_status_port`/`system_read_snapshot_port`），不再是 `system.Option` 回调表。

### 结果

- 14 条全部给出结论：**10 条 `[x]`/`function_exact` + 3 条 `partial` + 1 条 `boundary`**。
- `internal/system/**` 归零：14 条无一 `missing`。
- 全局：4451 = function_exact **1272** + partial **2522** + boundary **552** + module_only 4 +
  missing **101**（前批 1262 / 2519 / 551 / 4 / 115），Rust 测试 2968 → **2976**。
- 锚点对账：anchors 1262 → **1272**（本批 10 个新锚点全部已记账），unrecorded 保持 **0**。

### 本批新增测试与生产改动

生产改动一处（P0 功能修复）：`crates/jftrade-engine/src/product_api_system_status.rs`
每次请求读取 `settings.execution.settings()` 得到归一化的 `defaultTradingEnvironment`
（读取失败回落 `ExecutionSettings::default()` = `SIMULATE`），与 Go 的 store 语义一致。

本批新增测试 8 条、给既有测试补锚点 2 处（共 10 个新锚点）：

- `system_status_reports_the_persisted_default_trading_environment`（`[x]`，新增）：
  设置 `REAL` 启动 → `/system/status` 得 `apiPort`=实际绑定端口、`defaultTradingEnvironment=REAL`；
  `PUT /api/v1/settings/execution` 改回 `SIMULATE` 后再读即 `SIMULATE`（逐请求动态）。
- `system_status_and_control_reads_follow_the_configured_risk_snapshot`（`[x]`，新增）：
  磁盘控制面（realTradingEnabled/killSwitch/RUNTIME 源/限值 12.5·2500/一条 risk event）
  驱动 status 与 kill-switch、risk-limits、risk-events 读取投影。
- `default_control_reads_match_the_frontend_contract`（`[x]`，新增）：
  默认 approvals/killSwitch/riskLimits/riskEvents 的**键集**（排序后精确比较）与值，
  含 `requiredConfirmationText`、`maxApprovalAgeMs=300000`、`not_configured`、`approvalMode=none`，
  以及不得出现 `enabled`/`approvals`/`pendingCount`/`active`/`events` 等旧键。
- `empty_control_state_serializes_empty_entry_slices`（`[x]`，新增）：
  空控制态与缺失控制面文件两种输入下，四个投影的 `entries` 都是 `[]`（typed-nil 归一）。
- `system_status_embeds_injected_live_market_data_calendar_and_request_summaries`（`[x]`，新增）：
  recorder + 真实 CalendarManager + 一次 503 系统读失败 → 观测四段（live/marketdata/
  exchangeCalendars/requests）在同一响应里可见。
- `system_status_default_request_observability_matches_the_go_baseline`（`[x]`，新增）：
  全新产品的 requests 段等于 Go 默认基线（750ms / low / 两个空窗口 / OpenD 计数 0）。
- `storage_overview_and_control_defaults_expose_empty_frontend_slices`（`[x]`，新增）：
  存储概览四个空数组 + hard stops / hard stop events / kill switch events 默认形状。
- `production_system_read_defaults_opend_health_to_the_unavailable_reason`（`[x]`，新增）：
  未启用集成时 OpenD 健康恰为 `{reason,status}` 两键，不伪造 runtime/diagnosis。
- `system_status_matches_go_stable_fields_without_claiming_migration_ownership`、
  `system_write_product_replays_browser_boundary_failure_recovery_and_restart`
  （`[x]`，补锚点）：分别补 `service_test.go:13` 与 `service_test.go:212` 锚点。
- `partial` 证据沿用已存在测试：日历路由委托与 fail-closed、资源依赖探测、
  OpenD 启用分支诊断。

探针（改坏 → 转红 → 字节回滚）：把 `defaultTradingEnvironment` 改回硬编码 `"SIMULATE"`
→ `system_status_reports_the_persisted_default_trading_environment` 转红
（`left "SIMULATE" / right "REAL"`）→ 按字节回滚，回滚后文件
shasum `9935fdcb949fba39101936f8c58ecb5eb06bafe1d56211fd2f4582818273020e` 与修复版一致。

### 保留差异（记录在 `partial`/`boundary` 结论里）

- 日历：Rust 只在存在 `CalendarManager` 端口时注册日历路由（owned_routes 48→54），
  未装配时返回错误信封（与未注册路由一致）而非 Go 的 `200 + {"accepted": false, reason}`；
  生产组合根始终装配 manager。
- Futu/OpenD：没有可注入的 health/reset 回调与 `ctx` 透传断言；重置语义落在
  `POST /api/v1/system/futu-opend/manual-retry`（未配置 trade runtime 时 503 fail-closed）。
- 运行时依赖：无「回调透传」与静态 satisfied 未配置分支，始终执行真实探测。
- 观测：live 段是共享传输指标（无 `connected=3` 注入缝），请求窗口由真实 503 请求写入。

### 后续待办

- 下一批：`cmd/generate-pineworker-proto`（12）→ `internal/live`（12）→
  `internal/productfeatures`（12）→ `pkg/observability`（10）→ `cmd/internal`（9）→
  `internal/pineworkerassets`（7，锚点例外）→ `internal/desktop`（6）→ `internal/retry`（6）→ 其余小片。
- 若产品需要「默认交易环境」变更即时可见于桌面状态栏，本批修复已提供逐请求语义；
  后续若把 execution 设置接入缓存，必须保留该动态性（回归测试已就位）。

验证：`cargo fmt --all`（EXIT=0）；`cargo clippy -p jftrade-engine -p jftrade-integration-futu -p jftrade-settings -p jftrade-api -p jftrade-store-sqlite --all-targets --locked`（EXIT=0，无告警）；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-integration-futu -p jftrade-settings -p jftrade-api -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**2554 passed / 1 skipped**，EXIT=0）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2976 Rust** / **1272 `[x]`**；missing 101、partial 2522、boundary 552、module_only 4；`OK: 1272 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors **1272**；already recorded 1183、**unrecorded 0**、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0）；`node scripts/check-zero-go.mjs`（2900 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`（EXIT=0）。

`pnpm run check:quick` **EXIT=0**：policy/contracts/target-health、`cargo-nextest -p jftrade-desktop -p jftrade-engine`
（**1755 passed / 0 skipped**）、fmt、clippy、7 组 compatibility replay、pineworker 与 desktop/Node 套件（48/48）全部通过。

`pnpm run check:rust` **EXIT=1**，唯一失败项与本批 diff 无关且是既有基线失败，保留原始证据不记为通过：
target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 全部通过后，`check:rust:policy`
的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；根 `Cargo.toml`
精确锁定 `=0.23.44`），另有 **8 条** `warning[advisory-not-detected]`（陈旧 ignore）；run 在 policy 处中止，
因此该次运行的 workspace 测试阶段未执行（本批的 crate 级证据以上面的 2554 条 nextest 为准）。本批未改动
`Cargo.toml`/`Cargo.lock`。
