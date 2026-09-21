# 交易所日历设置域对齐批次（第 105 批）

本文件记录 Go 基线 `internal/jftsettings`（共享设置类型包：`ExchangeCalendarSettings`
自定义 `UnmarshalJSON`、`errorNotificationsEnabledSet` 标记、`WithErrorNotificationsEnabledSet`
辅助方法）迁移到 Rust 的逐测试核对结论，涉及
`crates/jftrade-settings/src/exchange_calendar.rs`（类型与归一化）与
`crates/jftrade-store-settings-file/src/lib.rs`（settings.json 读写与字段校验）。

## 第一百零五批：internal/jftsettings 收口（5 条）

### 范围与分片

`internal/jftsettings` 共 3 个 Go 文件，本批覆盖其中 2 个测试文件的 5 条 `missing`：

- P1 `types_test.go` 3：旧配置缺字段时默认开启通知（:8）、显式 false 保留并清标记（:21）、
  畸形 JSON 拒绝（:39）。
- P2 `exchange_calendar_settings_validation_test.go` 2：显式 true 被接受（:8）、对象内
  非布尔值拒绝（:18）。

分类：5 条 `[x]`/function_exact；`internal/jftsettings` 域内 `missing` 归零。

### 关键事实（本批 recon 实测）

- Go 语义（`git show go:internal/jftsettings/types.go` 复核）：自定义 `UnmarshalJSON` 用
  `ErrorNotificationsEnabled *bool` 覆盖别名解码结果——指针非 nil 时取该值并置
  `errorNotificationsEnabledSet=true`；指针为 nil（字段缺失或 `null`）时把值补成 true、
  标记保持 false。因此「缺字段」与「显式 true」在 Go 里值相同、标记不同。
- 该标记在冻结基线里的**唯一生产消费者**是
  `internal/store/settingsfile/calendar.go:70`：`if !input.ErrorNotificationsEnabledSet() && !input.ErrorNotificationsEnabled`
  才回退到默认值（true），随后 `WithErrorNotificationsEnabledSet(true)`；其余命中全部是
  该包自身测试与 `rollback_test.go:72` 的构造调用（`git grep -n "ErrorNotificationsEnabledSet\|errorNotificationsEnabledSet" go`）。
- Rust 语义：`ExchangeCalendarSettings` 的 serde 容器 `default` 从
  `ExchangeCalendarSettings::default()`（`error_notifications_enabled = true`）取缺失字段，
  `normalize_exchange_calendar_settings` 完全不触碰该字段；store 侧
  `StoredExchangeCalendarSettings.error_notifications_enabled: Option<bool>` +
  `unwrap_or(true)` 在读取时一次完成「缺省即开启、显式 false 保留」。
- 逐形态对照（缺字段 / `true` / `false` / `null`）：Go 归一化后结果分别为
  true / true / false / true，Rust 读取结果相同；`"enabled"` 这类类型错误与截断 JSON
  两侧都失败（本批两条 store 测试锁定）。

### 结果

- 全局：4451 = function_exact **1310** + partial 2542 + boundary 573 + module_only 4 +
  missing **22**（前批 1305 / 2542 / 573 / 4 / 27）。
- Rust 测试 3036 → **3041**（本批新增 5 条，1 条对应 1 行）。
- 锚点对账：1309 → **1314** 唯一引用（已记账 1254 → **1259**、unrecorded **0**、
  unknown 55、stale **0**）；未锚定 function_exact 告警保持 **199**（本批 5 条 `[x]`
  行全部带 `go:452dea11:...` 锚点）。

### 本批新增的测试

`crates/jftrade-settings/src/exchange_calendar.rs`（3 条）：

- `legacy_settings_without_error_notifications_field_default_to_enabled`（`types_test.go:8`）：
  解码 `{"autoRefreshEnabled":true,"refreshIntervalHours":12}`，断言通知开关为 true、其余
  字段按默认取值（12 小时保留），且序列化后 `errorNotificationsEnabled` 仍写出 true。
- `explicit_error_notifications_false_survives_normalization`（`types_test.go:21`）：解码
  `{"errorNotificationsEnabled":false}` 后断言 false 保留，`normalize_exchange_calendar_settings`
  不会重新打开显式 false（Go 标记守卫要防的行为），序列化仍输出 false。
- `explicit_error_notifications_true_keeps_calendar_defaults`（validation `:8`）：解码
  `{"errorNotificationsEnabled":true}` 后断言开关为 true，且 `autoRefreshEnabled=true`、
  `refreshIntervalHours=24`（解码显式字段不清零其余默认值），序列化输出 true。

`crates/jftrade-store-settings-file/tests/settings_file_contracts.rs`（2 条）：

- `truncated_exchange_calendar_document_is_rejected_at_open`（`types_test.go:39`）：截断文档
  `{"exchangeCalendars":{"manualOverrides":` 在 `SettingsFileStore::open` 与 `open_read_only`
  两个入口都以 decode 错误失败，不降级为空设置。
- `non_boolean_error_notifications_value_is_rejected_at_open`（validation `:18`）：
  `{"exchangeCalendars":{"errorNotificationsEnabled":"enabled"}}` 被 `decode exchangeCalendars`
  拒绝，字符串不会被强制转换为 bool。

### 探针（改坏→转红→按字节回滚）

1. `crates/jftrade-settings/src/exchange_calendar.rs`：把 `Default` 的
   `error_notifications_enabled` 改成 false →
   `legacy_settings_without_error_notifications_field_default_to_enabled` 红
   （`assertion failed: settings.error_notifications_enabled`）；显式 true 用例保持绿
   （显式字段覆盖默认值），说明两条测试各自钉住不同路径。回滚后 shasum
   `5ec6e37d3533bd4704c08c162f1b9e6bb930d2d3b9be1792ceee05a4d40b4737`。
2. 同文件：在 `normalize_exchange_calendar_settings` 开头加
   `settings.error_notifications_enabled = true;` →
   `explicit_error_notifications_false_survives_normalization` 红
   （`assertion failed: !normalized.error_notifications_enabled`）；回滚到同一 shasum。
3. `crates/jftrade-store-settings-file/src/lib.rs`：把 `load_document` 的
   `serde_json::from_slice` 失败改成 `Ok(Map::new())`（畸形文档降级为空设置）→
   `truncated_exchange_calendar_document_is_rejected_at_open` 红；回滚后 `git diff` 对该文件
   为空（字节级还原）。
4. 同文件：`validate_supported_fields` 末尾改为跳过
   `validate_field::<StoredExchangeCalendarSettings>` →
   `non_boolean_error_notifications_value_is_rejected_at_open` 红；回滚后同样无 diff。

### 保留差异与边界

- Go 的 `errorNotificationsEnabledSet` 标记与 `ErrorNotificationsEnabledSet()`/
  `WithErrorNotificationsEnabledSet()` 辅助 API 在 Rust **没有对应物**：Rust 用
  「`Default` 缺省 true + store 边界 `Option<bool>`」表达同一决策，默认化在读取时完成，
  归一化阶段不再需要来源判定。该标记在 Go 中唯一的生产消费面就是上述归一化守卫，
  逐形态对照后两侧结果一致，因此记为结构差异而非行为缺口。
- Go 对「程序化构造的零值 false（标记未置位）」会强制回 true；Rust 没有零值构造路径
  （`..Default::default()` 即 true），只有显式写出 `false` 才会保存 false——这与 Go
  「显式来源才保留 false」的意图一致，同样是结构差异。
- `internal/jftsettings` 中除交易所日历外的类型（`FutuIntegrationConfig`、
  `SecuritySettings`、`MCPServerSettings`、`PineWorkerSettings` 等）已在前序批次
  （`settings-internal-batch-scope.md`、`storage-settingsfile-batch-scope.md`）覆盖，
  本批不重复。

### 后续待办

- 下一批按域余量：`internal/frontendassets` 4（锚点例外：路径命中 `scripts/check-zero-go.mjs`
  禁用文本模式，不写代码锚点）→ `internal/research` 4 → `internal/security/passwordhash` 3 →
  `cmd/jftrade-api` 2 → `cmd/check-go-coverage` 2 → `pkg/besteffort` 2 → 直至 4451 条全部完成。
- 若后续批次引入「按来源区分显式/缺省」的新消费面（例如设置页要展示“用户是否显式关闭通知”），
  Rust 需要把 set-ness 提升为结构字段或 wire DTO 字段；当前无此需求，故不提前引入。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-settings
  -p jftrade-store-settings-file --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-settings -p jftrade-store-settings-file
  --all-targets --locked --no-fail-fast`：69 passed / 0 skipped / 0 failed（含本批 5 条新测试）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3041 Rust、`[x]` 1310、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 199（本批 5 条新 `[x]`
  行全部带 `// Parity:` 锚点）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1314 唯一引用（已记账 1259、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2910 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（affected 计划、contracts、target-health、nextest、fmt/clippy、
  兼容回放与 desktop 检查全绿；`.rcgu.o` 计数 24594 < 50000，未触发清理）。
- `pnpm run check:rust` EXIT=1：唯一阻塞点仍是 `pnpm run check:rust:policy`（`cargo deny check`）
  的 RUSTSEC-2026-0285（rustls 0.23.44，修复需 >=0.23.45）与 8 条
  `warning[advisory-not-detected]` 陈旧 ignore；run 停在静态阶段、未执行 workspace/all-targets
  阶段，等价测试面由上面的 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致。
