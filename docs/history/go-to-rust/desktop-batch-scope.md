# 桌面运行时路径与通知策略域对齐批次（第 103 批）

本文件记录 Go 基线 `internal/desktop`（打包态数据目录解析、当前平台路径、缺失 home 的
错误、通知匹配归一化、系统通知转发策略与 macOS 通知元数据）迁移到 Rust 的逐测试核对结论，
涉及 `apps/desktop/src-tauri/src/{profile.rs,native_lifecycle.rs,native_tests.rs}`、
`apps/desktop/src-tauri/tests/desktop_contracts.rs`、
`crates/jftrade-settings/src/notifications.rs` 与
`apps/desktop/src-tauri/src/native_notification_updater.rs`（通知投递边界）。

## 第一百零三批：internal/desktop 收口（6 条）

### 范围与分片

`internal/desktop` 共 6 条 `missing`，两个文件：

- P1 `runtime_path_test.go` 2：三平台数据目录矩阵（:8）、当前平台真实解析（:41）。
- P1 `runtime_path_matching_test.go` 2：home 缺失必须报错（:5）、空白/归一化匹配边界（:13）。
- P1 `notification_policy_test.go` 2：系统通知转发决策表（:10）、通知元数据（:35）。

分类：5 条 `[x]`/function_exact + 1 条 boundary；`internal/desktop` 域内 `missing` 归零。

### 关键事实（本批 recon 实测）

- Go 侧：`runtime_path.go` 的 `ProductDataDir()`/`productDataDir(goos,home,config,getenv)` 按
  darwin/windows/linux 三分支选择 `JFTrade`/`jftrade` 基目录并 trim 输入；`notification_policy.go`
  的 `ShouldForwardSystemNotification` 按 enabled/mode(all|custom|important)/levels/categories
  决策，另有 `NotificationThreadID`（category→source→`jftrade.system`）与
  `NotificationInterruptionLevel`（error→timeSensitive、warn→active、其余→passive）。
- Rust 侧对应 owner：`apps/desktop/src-tauri/src/profile.rs::product_data_dir`（同形三分支 +
  `join_path`）、`native_lifecycle.rs::platform_paths`（HOME/USERPROFILE + `cfg!(target_os)`）、
  `crates/jftrade-settings/src/notifications.rs::{should_forward_system_notification,
  matches_value}`（由 engine 的 projector/API 调用）。
- 通知元数据在 Rust 无 owner：`TauriNotificationPort` 经 tauri-plugin-notification 2.4.0 的
  桌面实现只转发 title/body/icon/sound（builder 的 group 等字段在桌面分支被忽略；crate 内
  `threadIdentifier`/`interruptionLevel` 只出现在 iOS Swift 源码），
  `ProductNotificationRequest` 也只有 title/body/sound_enabled。
- 桌面 crate 是 `#![forbid(unsafe_code)]` + edition 2024，测试不能 `env::set_var`；Go 用
  `t.Setenv` 清空 HOME 的用例需要注入点才能稳定复刻。

### 结果

- 全局：4451 = function_exact **1302** + partial 2539 + boundary **573** + module_only 4 +
  missing **33**（前批 1297 / 2539 / 572 / 4 / 39）。
- Rust 测试 3030 → **3035**（本批新增 5 条）。
- 锚点对账：1300 → **1305** 唯一引用（已记账 1245 → **1250**、unrecorded **0**、
  unknown 55、stale **0**）；未锚定 function_exact 告警保持 **199**（本批 5 条 `[x]` 行全部带锚点）。

### 本批新增/加强的测试

Rust（5 条新增，均带 `// Parity:` 锚点）：

- `apps/desktop/src-tauri/tests/desktop_contracts.rs::product_data_dir_covers_every_platform_base_directory`
  （`runtime_path_test.go:8`）：按 Go 表逐条断言 macOS（显式 config / 空 config 回退）、
  Windows（LOCALAPPDATA / 回退 config）、Linux（XDG_DATA_HOME / 回退 `~/.local/share`）与
  带空格 home 的归一化，并额外固定 Rust 的失败关闭分支（两处 base 皆空 →
  `ProfileError::MissingDataDirectory`）。
- `native_tests.rs::product_data_dir_uses_the_current_platform_base_directory`
  （`runtime_path_test.go:41`）：在真实环境上走 `platform_paths()` + `product_data_dir`，
  断言当前平台目录以 `/JFTrade` 或 `/jftrade` 结尾。
- `native_tests.rs::platform_paths_report_missing_home_environment`
  （`runtime_path_matching_test.go:5`）：注入式环境闭包断言「两键皆缺」「HOME 为空串」都返回
  `MissingHome`，且 HOME 空时回退 USERPROFILE。
- `crates/jftrade-settings/src/notifications.rs::should_forward_system_notification_covers_disabled_all_custom_and_unknown_modes`
  （`notification_policy_test.go:10`）：逐条覆盖 disabled/all/custom-levels/custom-categories/
  未命中/未知模式/带空格 levels 七种输入。
- `crates/jftrade-settings/src/notifications.rs::blank_values_never_match_and_padded_candidates_match_case_insensitively`
  （`runtime_path_matching_test.go:13`）：空值与空候选不匹配（与 Go 的 `value==""` 早退一致）、
  `ERROR` 命中 ` error `。

生产代码变更（行为不变，仅加测试缝）：`native_lifecycle.rs::platform_paths` 拆出
`platform_paths_from(getenv)`，保留 `env::var_os` 的 OsString 语义与空值过滤，
`platform_paths()` 以 `|key| env::var_os(key)` 调用它。

### 探针（改坏→转红→按字节回滚）

1. `apps/desktop/src-tauri/src/profile.rs`：把 Darwin 基目录后缀改成 `JFTradeData` →
   `product_data_dir_covers_every_platform_base_directory` 与
   `product_data_dir_uses_the_current_platform_base_directory` 同时红；回滚后 shasum
   `457b009bad204e458a70a19857619e53bef1da7d85b2794d074f6fc7e47ecd7d`。
2. `apps/desktop/src-tauri/src/native_lifecycle.rs`：删掉空 HOME 过滤 `filter(...is_empty())`
   → `platform_paths_report_missing_home_environment` 红；回滚后 shasum
   `59339b056a2fb23e911578ec178fcf03824157155ebef76dc70d39a8dc164c12`。
3. `crates/jftrade-settings/src/notifications.rs`：未知 mode 分支从 `false` 改成 `true` →
   `should_forward_system_notification_covers_...` 红；回滚后 shasum（本批最终）
   `f53ffc10f0cf94cdf8c484ec88c7db8fbfacaf7adec395109e3853fadb3020ee`。
4. 同文件：去掉 `matches_value` 的空值守卫后，**第一次探针没有转红**——因为原用例
   `matches_value("", ["error"])` 在没有守卫时同样为 false（Go 的同名用例也有这个盲区）。
   据此给测试补了一条 Go 语义等价、能钉住守卫的断言 `!matches_value("", [""])`，重跑探针后
   该测试转红，再按字节回滚。这是本批发现的“用例看似覆盖、实际没钉住实现”的实例。

### 保留差异与边界

- `notification_policy_test.go:35`（`NotificationThreadID`/`NotificationInterruptionLevel`）记为
  boundary：这两个 helper 产出的是 macOS UNUserNotification 的线程标识与打断级别，
  Rust 桌面链走 tauri-plugin-notification 的跨平台实现，只能携带 title/body/icon/sound；
  复刻需要 `objc2-user-notifications` 这类不安全 FFI（桌面 crate 禁止 unsafe）并改动通知
  wire 结构，属「无 Rust owner」。不迁实现、不新增表面 API。
- 路径分隔符：Go 用 `filepath.Join` 生成平台原生分隔符，Rust `join_path` 统一以 `/` 连接并
  原样保留调用方前缀（Windows 形如 `C:\...\Local/JFTrade`，Windows 接受混合分隔符）。
- Windows 缺 base：Go 退化成相对目录 `JFTrade`，Rust 返回 `MissingDataDirectory` 失败关闭
  （已写入本批测试，属有意的强化差异）。
- home 回退：Go 在 Unix 只读 HOME，Rust 三平台都做 HOME→USERPROFILE 回退（更宽松）；
  错误类型与文案也不同（`NativeError::MissingHome` / `ProfileError::MissingHome`）。
- 空白匹配：Go 的 `normalize` 比较会让「纯空白值 + 空候选」匹配成功，Rust 先 trim 再判空，
  空白值永不匹配（更严格的失败关闭）。
- 通知决策输入形态：Go 收 `live.Event`，Rust 收 projector 拆出的 level/category 字符串。

### 后续待办

- 下一批按域余量：`internal/retry` 6 → `internal/jftsettings` 5 →
  `internal/frontendassets` 4（锚点例外）→ `internal/research` 4 → …，直到 4451 条全部完成。
- 待办：`apps/desktop/src-tauri/src/native_notification_updater.rs` 可在插件升级到支持
  `threadIdentifier` 的版本后重开 `notification_policy_test.go:35` 的边界结论。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-desktop -p jftrade-settings
  --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop -p jftrade-settings
  --all-targets --locked --no-fail-fast`：78 passed / 0 skipped / 0 failed。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3035 Rust、`[x]` 1302、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 199（本批 5 条新 `[x]`
  行全部带 `// Parity:` 锚点）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1305 唯一引用（已记账 1250、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2908 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0：affected 计划为 policy/contracts/target-health、
  nextest（jftrade-desktop + jftrade-engine + jftrade-settings + jftrade-store-settings-file）
  1842 passed / 0 failed、rust fmt/clippy、compatibility desktop-runtime、desktop 脚本层。
- `pnpm run check:rust` EXIT=1：唯一阻塞点仍是 `pnpm run check:rust:policy`（`cargo deny check`）
  的 RUSTSEC-2026-0285（rustls 0.23.44，修复需 >=0.23.45）与 8 条
  `warning[advisory-not-detected]` 陈旧 ignore；run 停在静态阶段、未执行 workspace/all-targets
  阶段，等价测试面由上面的 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致。
