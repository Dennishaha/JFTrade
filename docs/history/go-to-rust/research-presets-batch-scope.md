# 研究预设域对齐批次（第 107 批）

本文件记录 Go 基线 `internal/research`（`errors.go`、`presets.go`、`presets_test.go`）的
4 条 `missing` 测试迁移到 Rust 的逐条核对结论，涉及
`crates/jftrade-engine/src/product_production_ports_research.rs`（生产端口
`ProductionResearchPresetPort`）、`crates/jftrade-engine/src/product_research_preset_port.rs`
（读端口与 `ResearchPresetReadSnapshotError`）、
`crates/jftrade-engine/src/product_research_preset_write_port.rs`
（写端口与 `ResearchPresetWritePortError`）、
`crates/jftrade-engine/src/product_wire_research.rs`（wire 失败映射）与
`crates/jftrade-store-sqlite/src/research_preset.rs`（`ResearchPresetStore`，WriterLease 唯一写者）。

## 第一百零七批：internal/research 预设收口（4 条）

### 范围与分片

Go 侧 3 个文件、4 条 `missing`：

- P1 `presets_test.go:85`：`TestServiceCreateListGetAndDeletePreset`（CRUD 往返与 id/name 归一化）。
- P1 `presets_test.go:111`：`TestServiceUpdatePresetMergesFieldsAndEnforcesRevision`（字段合并与 revision fence）。
- P1 `presets_test.go:146`：`TestServiceRejectsUnavailableAndInvalidPresetOperations`（非法输入失败关闭）。
- P2 `presets_test.go:202`：`TestServicePropagatesRepositoryFailures`（仓储故障必须可见）。

分类：4 条 `[x]`/function_exact；`internal/research` 域内 `missing` 归零。

### 关键事实（本批 recon 与实测）

- **Go Service 没有 Rust 对应对象**：Go 的 `Service` 包装 `ScreenPresetRepository` 接口，
  nil service/nil repository → `ErrUnavailable`；Rust 生产组合根用
  `ProductionResearchPresetPort { store: Arc<ResearchPresetStore> }` 同时实现读端口
  （`read` 覆盖 list/get 两条路由）与写端口（`mutate` 覆盖 Create/Update/Delete），
  不存在 nil 端口形态，因此 `ErrUnavailable` 的“依赖缺失”语义落在
  `ResearchPresetReadSnapshotError::Unavailable` / `ResearchPresetWritePortError::Unavailable`
  上，nil-service 分支以 per-op invalid 断言替代。
- **本批修复的真实分歧**：读路径原先直接把 URL 段当 id 查库，带空白或百分号编码的 id
  （Go 测试用 `" preset "`）会落到 NotFound。现改为 `percent_decode_str` 解码后 `trim`，
  空 id 提前返回 `Invalid("preset id is required")`；写路径 update 分支同样改为 trim 后判空。
  该修复使 `ResearchPresetReadSnapshotError::Invalid` 首次在生产路径被构造，
  随之移除了该枚举上原有的 `#[expect(dead_code, ...)]`（否则 `-D warnings` 会报
  `unfulfilled_lint_expectations`）。
- **归一化 owner 下移**：Go 在 Service 层 trim 名称并调用 `researchscreen.NormalizeDefinitionV2`；
  Rust 在端口层做同样归一化（名称按 Unicode 字符数 ≤80），definition 归一化复用
  `jftrade_research::normalize_definition_v2`，`querySchemaVersion` 固定 2，
  revision fence 由 `ResearchPresetStore::replace_revision` 在 SQLite 事务内执行。
- **错误归类**：`map_research_preset_store_error` 把 `NotFound`/`Conflict`/`Incompatible`
  分别映射为 `NotFound`/`Conflict`/`Invalid`，其余（Open/Configure/Schema/Query/Lock/WriterLease）
  映射为 `Unavailable(<store error>)`；读路径经
  `product_wire_research.rs::research_preset_read_snapshot_failure` 变成 API 失败，
  不会退化为空列表。

### 逐条结论

- `presets_test.go:85` → `crates/jftrade-engine/src/product_research_preset_tests.rs::preset_ports_round_trip_create_list_get_and_delete_with_trimmed_ids`：
  在临时 SQLite 上建 `research_screen_presets` schema 后跑完整往返——create `"  美股价值  "`
  归一化为 `美股价值`、`querySchemaVersion=2`、`revision=1`；list 返回该条；get 用
  `"%20<id>%20"` 仍命中；delete 用 `" <id> "` 后原 id 读取转 NotFound。
- `presets_test.go:111` → `::definition_only_updates_keep_the_stored_name_and_enforce_revision`：
  建 `旧名称` 后做 definition-only 更新，断言 `revision=2`、`querySchemaVersion=2`
  且名称保留（未被清空）；`expectedRevision` 落后时返回 `Conflict` 且库中版本不变。
- `presets_test.go:146` → `::invalid_preset_operations_fail_closed_before_the_store_is_touched`：
  对 create（空名、81 字符名、`querySchemaVersion=1`）、get/delete/update（空白 id）、
  update（无字段、`expectedRevision=0`）逐项断言 `Invalid` 前缀，最后断言列表仍为空，
  即非法调用没有写入任何条目。
- `presets_test.go:202` → `::store_failures_surface_as_unavailable_instead_of_silent_success`：
  删除 `research_screen_presets` 表制造基础设施故障后，list/get 必须返回非空 `Unavailable`，
  create/update/delete 必须失败关闭；任一操作静默成功（例如列表退化成 `[]`）都会转红。

### 探针记录（破 → 红 → 按字节回滚）

- 读路径移除 percent-decode（改回 `let preset_id = id;`）：往返测试转红
  （`get preset by percent-encoded padded id: NotFound`）。
- update 合并分支把 `current.preset.name.clone()` 改为 `String::new()`：
  definition-only 测试转红（`preset is not normalized for the expected revision`）。
- 删除 update 的 `if !has_name && !has_definition` 守卫：非法输入测试转红
  （no-field update 落到仓储报 NotFound 而非 validation）。
- list 分支改为 `self.store.list().unwrap_or_default()`：仓储故障测试转红
  （列表静默返回空）。

四个探针回滚后 `product_production_ports_research.rs` shasum 恒为
`3ef832494c8d5b274c1accbe2656eeae7b2ee79e6f036c57db00f6e64d89996e`；
`product_research_preset_port.rs` 为
`7505191c558d664fb6ec916cbea8ad58951c9ed2d0c12303fc94361043eee074`（含 dead_code expect 移除）。

### 保留差异

- Go 用 nil Service/Repository 表达不可用；Rust 端口恒持有 `Arc<Store>`，无 nil 态，
  以 `Unavailable` 变体表达依赖故障。
- Go 把仓储 error 原样透传给调用方；Rust 按端口契约归类为 `Unavailable(<store error>)`
  再映射为 API 失败，错误文本不逐字等同，但“失败必须可见、不得退化为空成功”一致。
- Go 用 `*string` 区分“未提供/提供空值”，Rust 端口按 JSON 缺键与 `null` 判定
  `has_name`/`has_definition`。
- Go 的 `Service` 是独立对象；Rust 拆为读端口（快照 JSON）与写端口（mutation 枚举），
  分别服务 GET 与 POST/PUT/DELETE 路由。

### 后续待办

- 下一批按域余量：`internal/security/passwordhash` 3 → `cmd/jftrade-api` 2 →
  `cmd/check-go-coverage` 2 → `pkg/besteffort` 2 → `internal/app/apiserver/servercoretest` 1 →
  `internal/app/apiserver/webaccess` 1 → `cmd/jftrade-desktop` 1 → `pkg/chart` 1 →
  `scripts/archive_frontend_assets_test.go` 1 → 直至 4451 条全部完成。
- 既有独立项：修复审计脚本生成 markdown 时的尖括号占位符（`<go 文件>:<行>`），使
  `build:docs:generated` 不再被 markdown-it 判为未闭合 HTML 标签。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-research
  -p jftrade-store-sqlite -p jftrade-engine --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-research
  -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`：1944 passed / 0 skipped /
  0 failed（含本批新增 4 条；单测聚焦运行 4/4 passed）。
- 四个探针各自转红并已按字节回滚（读路径 go 无 decode、update 合并空名、删除
  empty-update 守卫、list 吞仓储错误），回滚后 shasum 与探测前一致。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3046 Rust、`[x]` 1315
  （+4）、`missing` 18 → 14、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少
  function_exact；未锚定 200（本批 4 条均写锚点，未增加）、partial 无解析引用 7
  （既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1318 唯一引用
  （已记账 1263，+4；unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2914 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0。首跑曾因 `check:rust:target-health` 报
  `.rcgu.o` 89454 个（≥50000）失败，确认无 cargo/rustc 进程后执行
  `pnpm run clean:rust:artifacts` 再跑全绿；本批工作树测得的 quick 计划不含
  `check:rust:static` 阶段。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`），
  报 RUSTSEC-2026-0285 漏洞与 8 条陈旧 `advisory-not-detected` 告警，与本仓既有基线
  同源（第 106 批已在干净 HEAD 用 `git stash` 复现同一失败）。**不记为通过**；该 run
  停在静态阶段，等价测试面由上面的 nextest 结果覆盖。
