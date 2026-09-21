# Futu Proto 生成域对齐批次

本文件记录 Go 基线 `cmd/generate-futu-proto`（上游 proto 校验和校验、
`go_package` 重写、protoc 流水线与仓库摘要校验）迁移到 Rust
`crates/jftrade-integration-futu`（`build.rs` 用 prost 编译冻结 proto、
`tests/futu_proto_frozen_inputs.rs` 冻结产物守卫）的逐测试核对结论。

## 第九十六批：`cmd/generate-futu-proto` 全域收口（18 条）

### 范围与分片

`cmd/generate-futu-proto/**` 剩余 **18 条 `missing`**（5 个文件），分三片：
P0 manifest 与 generator 8 条（`manifest_test.go` 5、`generator_test.go` 3）→
P1 repository_verify 3 条 → P2 CLI 与 rewrite 7 条（`main_test.go` 4、`rewrite_test.go` 3）。

### 关键事实（本批 recon 实测）

- 该 Go 包是**退场 owner**：仓库零 Go 源码，`cmd/generate-futu-proto` 已删除，生成器
  不再存在；Rust 侧由 `crates/jftrade-integration-futu/build.rs` 用 prost 在构建期编译
  冻结的 `proto/futu/*.proto`。
- 两个冻结证据文件被保留：`scripts/futu-proto-10.9.6908.sha256`（184 条上游 proto 摘要）
  与 `scripts/futu-proto-generated-10.9.6908.digest`（`proto a7b3cc04… 184` / `pb 141cf7a1… 185`）。
  本批实测：proto 树按 Go 的 `digestRepositoryTree` 算法（相对名 + `0x00` + 内容 + `0x00`，
  按名排序累计 sha256）重算 = `a7b3cc0480dd6fc6247bfc187dfba240a6fa8a92acdee692e2099781fe4c1ec9`，
  与冻结 digest **逐字节一致**（184/184）。
- manifest 记录的 184 个摘要与树内容**全部不同**：manifest 描述上游（重写前）输入，
  树是重写后的暂存产物（注入 `option go_package`、CRLF 归一化、去行尾空白）。
  因此「按 manifest 校验树」不成立，上游输入校验路径随下载步骤一起退场（`manifest_test.go:70` 记 boundary）。
- Rust 冻结树逐文件满足 Go 重写的后置条件（本批逐条实测 184/184）：唯一 `option go_package`、
  取值 = `github.com/jftrade/jftrade-main/pkg/futu/pb/<pkg 小写去下划线>;<同名>`、无 `\r`、无行尾空白。

### 结果

- 18 条全部给出结论：**3 条 `[x]`/`function_exact` + 6 条 `partial` + 9 条 `boundary`**。
- `cmd/generate-futu-proto/**` 归零：18 条无一 `missing`。
- 全局：4451 = function_exact **1262** + partial **2519** + boundary **551** + module_only 4 +
  missing **115**（前批为 1259 / 2513 / 542 / 4 / 133），Rust 测试 2962 → **2968**。
- 锚点对账：anchors 1254 → **1262**（本批 8 个新锚点全部已记账），unrecorded 保持 **0**。

### 本批新增测试与生产改动

新增 `crates/jftrade-integration-futu/tests/futu_proto_frozen_inputs.rs`（6 条），
把「生成器退场后仍然重要的契约」固化成守卫：

- `frozen_proto_tree_matches_the_recorded_repository_digest`（`[x]`）：重算 proto 树摘要 = 冻结 digest，
  并在临时目录构造漂移树断言 `checked-in OpenD generated outputs do not match`。
- `frozen_proto_names_and_extensions_match_the_checksum_manifest`（`[x]`）：树平铺、全 `.proto`、
  名字集合与 manifest 完全一致；另复现非 `.proto` 扩展名、子目录、名字不匹配三类失败。
- `frozen_digest_parser_rejects_malformed_entries`（`[x]`）：复刻 Go digest grammar，
  拒绝 NUL 摘要、缺 `pb` 条目、重复条目、计数为 0。
- `frozen_manifest_parser_rejects_invalid_entries`、`frozen_manifest_names_match_the_checked_in_proto_tree`
  （`partial` 证据）：复刻 checksum manifest grammar（注释/空行、2 字段、64 位小写 hex、裸名唯一）、
  排序名字列表与树对照、空 manifest 拒绝、missing/unexpected 聚合文案。
- `frozen_proto_tree_keeps_the_generator_rewrite_postconditions`（`partial` 证据）：184/184 逐文件断言
  重写后置条件（canonical go_package、无 CRLF、无行尾空白）。

生产改动仅一处且为锁定文件边缘：`crates/jftrade-integration-futu/Cargo.toml` 增加
`sha2.workspace = true`（dev-dependency，工作区已锁定 `=0.11.0`），`Cargo.lock` 相应增加一行依赖边，
无版本变化、无新增第三方来源。测试夹具用 Cargo 自带的 `CARGO_TARGET_TMPDIR`，未引入 `tempfile`。

探针（改坏 → 转红 → 按字节回滚并 `shasum -a 256` 校验一致）：

1. 给 `proto/futu/Common.proto` 追加注释 → `frozen_proto_tree_matches_the_recorded_repository_digest`
   转红（记录摘要 `a7b3cc04…` vs 实际不同）→ 回滚后 `Common.proto` 摘要 `bdbd02a8…` 前后一致。
2. 新建 `proto/futu/ZZProbe.proto` → `frozen_proto_names_and_extensions_match_the_checksum_manifest`
   与 `frozen_manifest_names_match_the_checked_in_proto_tree` 同时转红（`filenames do not match`）→
   删除探针文件后 6 条守卫全绿。

另有 1 次开发期红→修（非人工探针）：manifest 解析最初只用 `Path::file_name` 判裸名，
`nested\Common.proto` 子例在 Unix 下漏判，补显式 `/` 与 `\` 拒绝后转绿。

### 新增 `[x]` 锚点（3 条）

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `repository_verify_test.go:12 TestRepositoryDigestDetectsGeneratedOutputDrift` | `tests/futu_proto_frozen_inputs.rs::frozen_proto_tree_matches_the_recorded_repository_digest` | `[x]`：proto 树摘要 = 冻结 digest，漂移树报同一错误文案；pb 半边随 Go 产物退场。 |
| `repository_verify_test.go:41 TestRepositoryDigestRejectsUnexpectedFilesAndProtoNames` | `...::frozen_proto_names_and_extensions_match_the_checksum_manifest` | `[x]`：扩展名/平铺/名字集合三类失败与 manifest 一致性。 |
| `repository_verify_test.go:59 TestParseRepositoryDigestValidation` | `...::frozen_digest_parser_rejects_malformed_entries` | `[x]`：digest grammar 与四类坏输入拒绝。 |

### 保留差异候选（保持 `partial`/`boundary` 的理由）

- `partial` 的 6 条（`manifest_test.go:15/25/38/60`、`generator_test.go:60`、`rewrite_test.go:12`）：
  行为层由守卫覆盖（grammar、排序名字、聚合差异、使用前先校验、重写后置条件），
  但 Go 的错误分类文本、返回类型、生成器流水线位置与重写分支（replace/insert、`no package`）
  在 Rust 没有运行入口，不构成 function_exact。
- `boundary` 的 9 条：`manifest_test.go:70`（上游输入校验，随下载步骤退场）、
  `generator_test.go:18/46`（protoc 流水线、暂存目录与 `pkg/futu/pb` 注册文件全部不存在，
  prost 在构建期生成到 `OUT_DIR`）、`main_test.go:14/29/37/44`（CLI 参数、`-help`、退出码无入口）、
  `rewrite_test.go:35/42`（`no package` 分支与 `.pb.go` 目录归档无入口）。

### 后续待办

- 全量清单仍余 **115 条 missing**，下一批按域余量：`internal/system`（14）→
  `cmd/generate-pineworker-proto`（12）→ `internal/live`（12）→ `internal/productfeatures`（12）→
  `pkg/observability`（10）→ `cmd/internal`（9）→ `internal/pineworkerassets`（7，锚点例外）→
  `internal/desktop`（6）→ `internal/retry`（6）→ 其余小片。
- 若未来把 proto 摘要校验接入 `pnpm run check:generated` 或独立 CI 门禁，可把本批 3 条 `[x]`
  证据从测试提升为门禁证据；当前守卫已随工作区测试运行。
- `scripts/futu-proto-10.9.6908.sha256` 的语义（上游重写前摘要）已在守卫注释与本文档中说明，
  不要再把它当成树校验和。

验证：`cargo fmt --all -- --check`；`cargo clippy -p jftrade-integration-futu -p jftrade-settings -p jftrade-engine -p jftrade-api -p jftrade-store-sqlite --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-futu -p jftrade-settings -p jftrade-engine -p jftrade-api -p jftrade-store-sqlite --all-targets --locked --no-fail-fast`（**2546 passed / 1 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2968 Rust** / **1262 `[x]`**；missing 115、partial 2519、boundary 551、module_only 4；`OK: 1262 function_exact mappings cite existing workspace tests`、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 196、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（anchors **1262**；already recorded 1173、**unrecorded 0**、unknown go line 55、stale 34）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0）；`pnpm run check:generated`（EXIT=0）；
`node scripts/check-zero-go.mjs`（2898 tracked files / 0 release artifact）；`pnpm run check:ai-context`（EXIT=0）；`git diff --check`。

`pnpm run check:quick` **EXIT=1**，唯一失败项与本批 diff 无关且与 `check:rust` 同源：本批改动
`Cargo.toml`/`Cargo.lock` 触发 rust 全量 affected 计划，因此把长期存在的 `check:rust:policy`
`advisories FAILED`（**RUSTSEC-2026-0285**，rustls 0.23.44）一并纳入 quick。同次运行的
target-health、static（fmt+clippy）、architecture、production-policy、workspace 测试
（**3085 passed / 2 skipped**）、契约检查、web/desktop 与 Node 套件（366+10+1 文件）全部通过。

`pnpm run check:rust` **未通过（EXIT=1）**，唯一失败项不在本批 diff 范围内，保留原始证据不记为通过：
`check:rust:policy` 的 `cargo deny check` advisories 失败于 **RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；
根 `Cargo.toml` 精确锁定 `=0.23.44`），另有 8 条 `warning[advisory-not-detected]`（陈旧 ignore）。
同一次运行中 target-health、architecture、production-policy、`cargo fmt --check` 与 clippy 均通过；
本批仅改 `Cargo.lock` 的一行依赖边（sha2 0.11.0，dev-dependency），未升级任何依赖。

运维记录：首次 `check:quick` 因 `check:rust:target-health` 报 `target/debug/deps` 中间 `.rcgu.o`
超过 5 万而失败（长时间 nextest 运行的副产物）；按仓库约定确认无 cargo 进程后执行
`pnpm run clean:rust:artifacts`（`cargo clean`，移除 139088 个文件 / 39.4GiB 可再生产物），
target-health 恢复通过后重跑 quick。
