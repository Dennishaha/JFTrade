# protogen 工具链域对齐批次（第 101 批）

本文件记录 Go 基线 `cmd/internal/protogen`（Go 代码生成工具的目录替换、文件复制、仓库根
探测与 protoc/插件工具链准备）迁移到 Rust 的逐测试核对结论，涉及
`apps/desktop/src-tauri/src/native_resource_integrity.rs`（开发态仓库根探测）、
`crates/jftrade-engine`（data-management 批量替换与回滚、插件产物安装）、
`crates/jftrade-integration-{pine,marketdata-helper}`（资产物化）、
`scripts/compatibility/audit_test_parity.py`（Rust 测试总数口径）与
`.github/actions/setup-rust/action.yml` + `scripts/check-rust-toolchain-bootstrap.test.mjs`
（固定 protoc 工具链引导）。

## 第一百零一批：protogen 收口（9 条）+ 锚点漂移清理

### 范围与分片

`cmd/internal/protogen` 共 9 条 `missing`，三个文件：

- P1 `files_test.go` 4：`CopyFile` 复制内容（:13）、`ReplaceDirectories` 全量替换（:24）、
  替换失败全量回滚（:39）、`ExitCode` 解包包装错误（:55）。
- P1 `repository_test.go` 2：向上查找仓库根（:12）、缺失 marker 时失败（:23）。
- P1 `tools_test.go` 3：安装缺失插件（:15）、拒绝错误 protoc 版本（:65）、
  env helper 大小写不敏感（:74）。

顺带完成第 100 批登记的机械清理：`parity_anchor_reconcile.py` 报的 43 条 stale 锚点
（第 97–99 批把测试模块抽到同目录 `*_tests.rs` 后，`rust_entry` 仍写生产文件名或用
crate 路径引用）。

### 关键事实（本批 recon 实测）

- Go 的 protogen 是**构建期 Go 工具**：`CopyFile`（建父目录 + 按源权限位复制 + 拒绝非普通
  文件）、`ReplaceDirectories`（先备份所有 target，再逐个 `os.Rename` 装入，失败时删除已装
  目标并恢复全部备份）、`FindRepoRoot`（向上找 `go.mod`）、`PrepareToolchain`（校验
  `libprotoc 34.1`、解析 GOPATH、缺插件时 `go install`、PATH 前置、安装时清空 GOFLAGS）、
  `ExitCode`（`errors.As` 穿透包装取 `ExitCode() int`，否则 1）。
- Rust 仓库零 Go 源码，不生成 Go 绑定：等价能力由各自的 Rust/Node owner 承接——
  资产物化与安装路径（父目录 + 临时文件 + 原子 rename + 回滚）、data-management 批量
  替换与逐字节回滚、开发态仓库根探测、CI 固定工具链引导、Node `spawnChecked` 退出码契约。
- Go 的 marker 是 `go.mod`；Rust 侧对应 marker 是 `Cargo.toml` 普通文件 +
  `workers/pineworker` 目录（`native_resource_integrity.rs::development_repository_root`
  原本就是向上查找，但起止路径写死为 `CARGO_MANIFEST_DIR`，且没有针对该契约的测试）。

### 结果

- 9 条全部给出结论：**2 条 `[x]`/function_exact + 5 条 `partial` + 2 条 `boundary`**，
  `cmd/internal/protogen` 域内 `missing` 归零。
- 全局：4451 = function_exact **1291** + partial **2539** + boundary 571 + module_only 4 +
  missing **46**（前批 1289 / 2534 / 569 / 4 / 55）。
- Rust 测试口径修正后为 **3027**（原 3005 未统计 `apps/desktop/src-tauri` 的 22 条；
  本批新增 2 条后应为 3007 + 既有 20 = 3027，见「审计脚本修正」）。
- 锚点对账：anchors 1298 → **1300**（已记账 1200 → **1245**、unrecorded 保持 **0**、
  **stale 43 → 0**）。

### 本批新增/加强的测试

Rust（2 条，`apps/desktop/src-tauri/src/native_tests.rs`）：

- `repository_root_walks_up_from_a_nested_directory_to_the_workspace_markers`（:12）：
  `repository_root_from(workspace/one/two)` 解析到 workspace 根；`one/Cargo.toml` 是**目录**
  且 `one/workers/pineworker` 存在时不得误命中（marker 必须是普通文件）；marker 目录自身
  解析到自己。
- `repository_root_rejects_a_tree_without_the_workspace_markers`（:23）：只有 `Cargo.toml`、
  缺 `workers/pineworker` 的树从根与嵌套子目录启动都返回
  `NativeError::MissingRepositoryRoot`（fail closed，不退化成返回起始目录）。

Node（1 条，`scripts/lib/windows-command-resolution.test.mjs`）：

- `spawnChecked reports the child exit code and falls back to 1 without a status`（:55 证据）：
  子进程退出码 42 原样传播；命令不可启动时返回 1 且只报错一次（与 Go「普通失败→1」一致）。

Python（1 条，`scripts/compatibility/test_audit_test_parity.py`）：

- `test_desktop_shell_tests_are_counted_in_the_workspace_total`：断言审计快照把
  `apps/desktop/src-tauri` 的 `#[test]` 计入总数并归到 `other` 域。

### Rust 功能改动（含先红后改）

- **`repository_root_from` 参数化**（`apps/desktop/src-tauri/src/native_resource_integrity.rs`）：
  把内联在 `development_repository_root` 里的 `ancestors().find(...)` 抽成
  `repository_root_from(start: &Path)`，`development_repository_root` 传入
  `CARGO_MANIFEST_DIR`。marker 判定从「`Cargo.toml` 是文件」保持不变，并首次为该契约补测试。
- **审计脚本口径修正**（`scripts/compatibility/audit_test_parity.py`）：新增
  `_rust_source_files()`/`_rust_crate_name()`，`extract_rust_tests()` 除 `crates/**/*.rs`
  外也扫描 `apps/desktop/src-tauri/**/*.rs`（与 `_workspace_rust_files()` 的引用解析口径
  一致），并把 Tauri 壳 crate 命名为 `apps/desktop/src-tauri`。修正前报告少算 22 条桌面测试，
  与「可解析引用集合」自相矛盾。

### 机械清理（锚点漂移 → stale 归零）

- 逐条把 43 条 stale 行的 `rust_entry` 补上锚点所在文件的显式引用（保留原入口）：
  能定位到具体测试的补 `<file>::<test>`（37 条，例如
  `crates/jftrade-api/tests/transport_contracts.rs::desktop_missing_static_assets_do_not_use_spa_fallback`），
  锚点写在生产符号或整块测试模块（`//!` 头）上的补 `<file>`（6 条，例如
  `crates/jftrade-store-sqlite/src/maintenance_rebuild_safety_tests.rs`）。
- 清理只改 `rust_entry` 字符串，不动任何判定结论：`[x]` 保持 1291，`stale` 归零，
  `unrecorded` 保持 0；`function_exact` 未锚定告警从 196 降到 **193**（37 条修复行里含
  若干 `[x]` 行，其锚点与清单从此互相印证）。

### 探针（改坏 → 转红 → 按字节回滚）

1. `repository_root_from` 的 `Cargo.toml.is_file()` 改成 `exists()` →
   `repository_root_walks_up_from_a_nested_directory_to_the_workspace_markers` 转红
   （解析到 marker 形状的 `one/` 目录）→ 回滚，`native_resource_integrity.rs` 恢复
   `f5f6a6db334285fc36042c4105fcf065a7d26c900a30e6c752b54d7e1984a4bb` 后转绿。
2. `ok_or(NativeError::MissingRepositoryRoot)` 换成 `MissingAsset` →
   `repository_root_rejects_a_tree_without_the_workspace_markers` 转红
   （错误类型与文案不符）→ 同法回滚并核对同一 shasum。
3. `extract_rust_tests()` 去掉 `apps/desktop/src-tauri/**/*.rs` 扫描 →
   `test_desktop_shell_tests_are_counted_in_the_workspace_total` 转红
   （`native_shell_test` 未被计数）→ 回滚，`audit_test_parity.py` 恢复
   `e16a48ec75c4f68520097b2acf34945bc94f750f42d00a0f59d8926496b3b8fa`。

### 保留差异（记录在 partial/boundary 结论里）

- **目录替换**：Go 以「目录」为单位备份/装上/回滚；Rust 的批量 owner 是 data-management
  启动批（descriptor 文件集 + `-wal/-shm/-journal` 边车，逐文件快照、逆序恢复、保留迁移
  备份），插件安装路径只覆盖单 target。没有 `Replacement{Source,Target}` 目录 API。
- **文件复制**：Rust 无 `CopyFile(source,target)` 通用 helper；父目录创建 + 精确字节 + 原子
  rename 分散在资产物化（helper/pine）、插件产物安装与 Node 打包脚本里，权限位来自
  `std::fs::copy` 或显式 chmod。
- **工具链准备**：Rust 不装 Go 插件（`protoc-gen-go*`、GOPATH、GOFLAGS 无对应对象）；
  固定工具链由 CI 引导脚本提供（Rust 1.97.1、protoc 34.1 各平台归档 + sha256、
  cargo-deny 0.20.2），版本校验以 `grep -Fx "libprotoc 34.1"` fail closed，
  由 `scripts/check-rust-toolchain-bootstrap.test.mjs`（policy 层）断言。
- **env helper**：Go 的 `EqualFold` 环境变量折叠是 `[]string` 手工搬运的产物；
  Rust/Node 分别依赖 `std::env` 与 `process.env` 的平台语义（Windows 不区分、Unix 区分），
  没有也不应引入折叠 helper。
- **退出码**：Go 用错误链解包 `ExitCode()`；Node `spawnChecked` 用返回值传递
  「子进程状态，取不到则 1」，Rust 托管子进程只观察 `ExitStatus`。

### 后续待办

- `partial` 无解析引用 7 条、无断言 `function_exact` 2 条维持既有基线告警；本批未新增。
- 第 97–99 批 `#[path]` 搬迁产生的 stale 锚点已全部归零；后续若继续搬迁测试模块，需同步
  在写回时补 `<file>::<test>` 形式，避免再次出现文件级 stale。
- 下一批按域余量：`internal/pineworkerassets` 7（锚点例外）→ `internal/desktop` 6 →
  `internal/retry` 6 → `internal/jftsettings` 5 → `internal/frontendassets` 4（锚点例外）→ …

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-desktop --all-targets
  --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop -p jftrade-engine
  --all-targets --locked --no-fail-fast`：1772 passed / 0 skipped / 0 failed（170s），
  其中桌面壳 22 条（含本批 2 条新测试）。
- `pnpm run test:scripts -- policy`：124 passed / 0 failed（含
  `check-rust-toolchain-bootstrap.test.mjs` 与新增的 `spawnChecked` 退出码用例；
  `test:scripts -- compatibility` 里的 python 用例含本批新增审计单测）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3027 Rust、`[x]` 1291、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 193（↓3）、
  partial 无解析引用 7、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1300 唯一引用（已记账 1245、
  unrecorded 0、unknown 55、**stale 0**）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2906 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=1：唯一失败阶段是 `pnpm run check:rust:static`，其内部
  `check:rust:policy`（`cargo deny check`）因 RUSTSEC-2026-0285（rustls 0.23.44，修复需
  >=0.23.45）与 8 条 `warning[advisory-not-detected]` 陈旧 ignore 失败；其余并行阶段
  （含 pinets 合规、web、契约与脚本层）通过。
- `pnpm run check:rust` EXIT=1，阻塞点同上（policy），run 停在静态阶段、未执行
  workspace/all-targets 阶段；等价测试面由上面的 nextest wrapper 覆盖。**两者均不记为
  通过**，与干净 HEAD 行为一致（target-health 本次 21176 个 `.rcgu.o`，低于 5 万阈值，
  未触发清理）。
