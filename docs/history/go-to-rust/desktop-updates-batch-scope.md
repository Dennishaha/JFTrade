# 桌面更新检查批次（第 114 批）

本文件记录 Go 基线 `cmd/jftrade-desktop/desktop_updates_test.go` 的 2 条测试迁移到 Rust 的逐条
核对结论。本批结论是**一升一边界**：`:33`（开发态不检查更新）用本批新增的 headless 测试收口为
`function_exact`；`:9`（客户端抓取 GitHub release 列表并挑选最新稳定版）判为**不适用/边界保留**，
因为 Rust 把“选择最新稳定版”的所有权移到了发布侧 + 签名 updater feed，客户端不存在可断言的
release 列表过滤代码。涉及 `apps/desktop/src-tauri/src/native_tests.rs`（本批新增 1 条测试）与
`apps/desktop/src-tauri/src/profile.rs`（探针改动，已按字节回滚）。

## 第一百一十四批：desktop_updates 收口（2 条）

### 范围与分片

Go 侧 1 文件 2 条：

- P1 `desktop_updates_test.go:9`：`TestDesktopUpdateServiceSelectsLatestStableDesktopRelease`
  （原 `missing`；httptest 提供 release 列表，期望 User-Agent 为
  `JFTrade-Desktop/1.2.0`、忽略 `desktop-v9.0.0` 与非稳定版本、选到 `1.4.0` 及其 release URL）。
- P1 `desktop_updates_test.go:33`：`TestDesktopUpdateServiceDisabledForDevelopment`
  （原 `partial`；`enabled=false` 时 Check 不报错、`Available=false`、`CurrentVersion` 原样返回）。

分类：1 条 `[x]`/`function_exact`（`:33`），1 条 `[~]`/`boundary`（`:9`，不适用/边界保留）；
全仓 `missing` 3 → 2（余 `pkg/chart`、`scripts/archive_frontend_assets_test.go`）。

### 关键事实（本批 recon 与实测）

- **Go 机制**（`cmd/jftrade-desktop/desktop_updates.go`）：release 构建（
  `profile.UpdateChecksEnabled=true`，dev 为 false）启动 15 秒后检查一次、之后每 24 小时检查；
  `Check()` 抓取 GitHub releases API（`Accept: application/vnd.github+json`、
  `User-Agent: JFTrade-Desktop/<current>`，15s 超时），过滤 `draft`、`prerelease`、非 `v` 前缀与
  非法 semver，取最大稳定版本，把 `Available/LatestVersion/ReleaseURL/PublishedAt/Notes` 交给前端；
  只提示不下载安装。
- **Rust 机制**：`NativeUpdaterConfig` 要求 HTTPS endpoint 与签名公钥成对配置
  （`apps/desktop/src-tauri/src/native_notification_updater.rs`，`Disabled`/`Unconfigured`/`Ready`
  三态）；release 构建在启动检查里用签名 updater 拉取 feed
  （`native_lifecycle.rs:148-166`），有更新时 `emit(DESKTOP_UPDATE_AVAILABLE_EVENT, result)`，
  前端 `DesktopUpdateBanner.vue` 复用同一事件展示并调用签名安装（Tauri）或打开 release 链接
  （非 Tauri 后端）。`Update.raw_json` 只用来兜底取 `releaseUrl`（`html_url`）。
- **选择所有权在发布侧**：`scripts/check-desktop-release-policy.mjs` 的 updater 必需项包含
  endpoint 与公钥，`.github/workflows/desktop-release.yml` 在构建时注入两者；feed 版本被
  `scripts/release/check-signed-updater-artifact.mjs`（`/^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)$/`）
  与 `scripts/verify-tauri-release-artifacts.mjs:43` 强制为**数值稳定 semver**，因此“只提供稳定版”
  由发布侧保证；`apps/desktop/src-tauri` 内不存在任何 `tag_name`/`prerelease`/`draft` 解析。
- **渠道门禁一致**：`profile.rs` 中 development 渠道 `update_checks_enabled=false`、release 渠道为
  true，与 Go 的 `UpdateChecksEnabled` 一一对应；`native_lifecycle.rs` 只在开关为真时才 spawn
  自动检查任务。

### 逐条结论

- `desktop_updates_test.go:33` →
  `apps/desktop/src-tauri/src/native_tests.rs::tests::development_channel_skips_release_checks_while_the_release_channel_runs_them`：
  断言 development 渠道 `update_checks_enabled=false`、release 渠道为 true，且把开发态开关喂给
  `NativeUpdaterConfig::from_values` 得到 `Disabled`（配合既有
  `updater_requires_complete_https_release_configuration` 的三态断言）。生命周期层在开关为假时
  根本不 spawn 检查，facade 显式调用时 `Disabled` 分支返回 `no_update(current)`
  （`available=false`、无错误）。
- `desktop_updates_test.go:9` → **不适用/边界保留**：Rust 的等价能力（release 构建得知有新稳定版、
  展示版本与 release 链接、可签名安装）由签名 feed 承担，客户端不做 release 列表过滤与最大版本
  选择。复现差异：未配置签名 feed 的构建调用 `desktop_update_check` 返回
  `DESKTOP_UPDATE_NOT_CONFIGURED`，不会像 Go 那样降级去读公开 release 列表。若产品后续决定恢复
  “未配置签名 feed 也提示公开 release”，修复位置为 `apps/desktop/src-tauri`（新增 feed 客户端），
  回归要求：headless fixture/mock server 覆盖 draft/prerelease/非 `v` tag 过滤、最大稳定 semver、
  `User-Agent: JFTrade-Desktop/<current>`，且不得引入未签名的下载/安装路径。

### 探针记录（破 → 红 → 按字节回滚）

- 把 development 渠道的 `update_checks_enabled` 由 `false` 改成 `true`
  （`apps/desktop/src-tauri/src/profile.rs`）：
  `development_channel_skips_release_checks_while_the_release_channel_runs_them` 转红
  （`development builds never poll for desktop releases`）。
- 探针按字节回滚，回滚后 shasum 与探测前一致：
  - `apps/desktop/src-tauri/src/profile.rs`：`457b009bad204e458a70a19857619e53bef1da7d85b2794d074f6fc7e47ecd7d`
  - `apps/desktop/src-tauri/src/native_tests.rs`：`76cf4793ba87ed37ddd44133b778dc70a941e6f6737e08c409c98c67cad3f928`

### 保留差异

- **客户端无 release 列表解析**：Go 的 draft/prerelease/tag 前缀/semver 过滤在 Rust 没有对应代码；
  该判断由发布侧（签名 feed + 数值 semver 门禁）承担，客户端只消费单一 feed。
- **`currentVersion` 取值不同**：Go 用构建信息里的版本，dev 构建为字面量 `dev`；Rust 取 Tauri
  `package_info().version`（真实包版本）。`:33` 的断言落在“开发渠道不轮询 + 配置 Disabled +
  无可用更新”，不复制 `dev` 占位字符串。
- **未配置 feed 的行为更严格**：Go 无需任何配置即可读公开 feed；Rust 未配置签名 feed 时
  `DESKTOP_UPDATE_NOT_CONFIGURED` 失败关闭，避免出现无法签名安装的“幽灵更新”。
- **检查节奏**：Go 为启动后 15 秒首查 + 每 24 小时轮询；Rust 目前只在启动时检查一次
  （`native_lifecycle.rs` 的 setup 中单次 `update_check()`）。长时间常驻的桌面会话在 Rust 下不会
  自动获知新版本——这是本批**登记的后续待办**（Go 源文件中的 `startDesktopUpdateChecks`
  定时器没有对应 Go 测试，故不在本批两条清单行的断言范围内）。

### 后续待办

- 下一批：`pkg/chart/chart_type_test.go:5`（`TestNormalizeChartType`，`missing` 2 → 1）。
- 再下一批：`scripts/archive_frontend_assets_test.go:11`（`missing` 1 → 0，随后转入 2544 条
  partial 与 573 条 boundary 的逐域收口）。
- 本批登记的独立项：为 Rust 桌面补“周期性更新检查”（对应 Go `startDesktopUpdateChecks` 的
  24 小时节奏），复用同一签名 updater 与 `jftrade:desktop-update:available` 事件，不新增未签名
  网络路径；需要为其设计可测试的调度接缝（注入时钟/间隔）。
- 既有独立项：审计脚本生成的 markdown 含尖括号占位符，`build:docs:generated` 在
  markdown-it/Vue 判为未闭合 HTML 标签处失败（与本批无关）。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-desktop --all-targets
  --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop --all-targets --locked
  --no-fail-fast`：30 passed / 0 skipped / 0 failed（含本批新增 1 条）。
- 探针转红并已按字节回滚（development 渠道开关翻转），回滚后两个文件 shasum 与探测前一致。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3065 Rust、`[x]`
  1327 → 1328、`missing` 3 → 2、`partial` 2545 → 2544、`boundary` 572 → 573、0 破坏引用、
  0 条 `[x]` 缺少 function_exact；未锚定 function_exact 保持 202（既有基线）、partial 无解析引用 7
  （既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1329 唯一引用（已记账 1273 →
  1274；unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility:desktop-runtime`
  EXIT=0（3 平台 profile、6 链接用例、10 条 facade 命令、4 个事件）；`pnpm run check:generated`
  EXIT=0 且未改写工作树；`node scripts/check-zero-go.mjs` EXIT=0（2927 tracked files）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（本次 target-health 通过，未触发 clean）。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`）报
  RUSTSEC-2026-0285 与陈旧 advisory 告警，与本仓既有基线同源。**不记为通过**；该 run 在静态
  阶段停止，等价测试面由上面的 nextest 结果覆盖。
