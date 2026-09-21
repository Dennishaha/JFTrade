# pineworker 资产包域对齐批次（第 102 批）

本文件记录 Go 基线 `internal/pineworkerassets`（PineTS worker 嵌入资产的选择、元数据与
平台无关文件名，含 `!release_assets` / `release_assets` 两套构建）迁移到 Rust 的逐测试
核对结论，涉及 `apps/desktop/src-tauri/src/native_resource_integrity.rs`（开发态/发布态
bundle 选址与 `required_asset`）、`apps/desktop/src-tauri/src/resource_integrity.rs`
（发布 runtime 清单摘要校验）、`crates/jftrade-integration-pine/src/asset.rs`
（`PineBundle` 元数据校验与物化）、`scripts/build-pineworker-assets.mjs`
（打包产物名）与 `scripts/check-zero-go.mjs`（锚点例外约束）。

## 第一百零二批：internal/pineworkerassets 收口（7 条）+ 锚点例外

### 范围与分片

`internal/pineworkerassets` 共 7 条 `missing`，三个文件：

- P2 `asset_selection_boundaries_test.go` 4：`selectFromFS` 返回嵌入 bundle 元数据（:13）、
  缺失/空 bundle 视为不可用（:30）、非预期读错误原样传播（:52）、`isMissingAsset` 只认
  不存在类错误（:63）。
- P2 `assets_dev_test.go` 1：`!release_assets` 构建下无嵌入资产时 `Select` 返回不可用（:7）。
- P2 `assets_release_test.go` 1：`release_assets` 构建下已 staged 资产返回同名/同内容/同摘要
  （:13；未 staged 时 `t.Skip`）。
- P2 `assets_test.go` 1：`BundleName()` 与平台无关，恒为 `worker.mjs`（:5）。

### 关键事实（本批 recon 实测）

- Go 包结构：`assets.go`（`Asset{Name,Data,SHA256}`、`Select`→`selectFromFS`、`BundleName`、
  `isMissingAsset`）、`assets_dev.go`（`assetFS()` 返回空 `emptyAssetFS`）、
  `assets_release.go`（`//go:embed assets/bin/*` + `fs.Sub`）、`emptyfs.go`；`binDir="bin"`、
  `workerBundleName="worker.mjs"`。
- 冻结基线核对：`git ls-tree -r --name-only go -- internal/pineworkerassets` 命中 7 个测试文件；
  `internal/assets_worker` 在基线中**不存在**（0 行清单）。Rust 代码里两条 `// Parity:` 锚点
  （`crates/jftrade-integration-pine/src/asset.rs:105/:118`）仍指向该退役路径，属 reconcile
  的 unknown go line 55 中的 2 条。
- 锚点例外：`scripts/check-zero-go.mjs` 的 `activeTextPattern` 明确把
  `internal/(frontendassets|marketdataassets|pineworkerassets)/` 列为活动根禁止文本，因此本批
  **一律不写代码锚点**，退役包路径也无法机械改写进 Rust 注释；function_exact 无锚点告警
  193 → 199（+6，全部为本批新增 `[x]` 行）。
- Rust 侧没有嵌入式 `fs.FS` 选层：开发态从仓库 `var/pineworker/worker.mjs`、发布态从
  `runtime/pineworker/worker.mjs` 选址，缺失即 `NativeError::MissingAsset`（fail closed，
  本批把这两条内联字面量收敛为 `PINE_WORKER_BUNDLE_FILE_NAME` + `pine_worker_bundle_fallback`）；
  发布态另由 `verify_release_resources` 按 runtime 清单逐项 `sha256` 校验（缺失→`Read`、
  摘要不符→`Hash`、路径逃逸→`UnsafePath`）。
- 打包侧同一名字面量在 Node 与 Tauri 配置：`scripts/build-pineworker-assets.mjs`
  `const outputName = "worker.mjs"`、`tauri.conf.json` 把 `runtime-assets/pine/worker.mjs`
  映射到 `runtime/pineworker/worker.mjs`、`prepare-tauri-release-runtime.mjs` 把它写进清单；
  Node 侧测试只断言「平台无关 Node bundle（--platform node --format esm）」，不断言文件名。

### 结果

- 7 条全部给出结论：**6 条 `[x]`/function_exact + 1 条 `boundary`**，
  `internal/pineworkerassets` 域内 `missing` 归零。
- 全局：4451 = function_exact **1297** + partial 2539 + boundary **572** + module_only 4 +
  missing **39**（前批 1291 / 2539 / 571 / 4 / 46）。
- Rust 测试 3027 → **3030**（本批新增 3 条，另加强 1 条既有测试）。
- 锚点对账：anchors 1300（已记账 1245、unrecorded **0**、unknown 55、stale **0**）——
  本批为锚点例外，代码锚点无变化。

### 本批新增/加强的测试

Rust（3 条新增 + 1 条加强，`apps/desktop/src-tauri`）：

- `native_tests.rs::development_pine_runtime_without_staged_bundle_reports_unavailable_asset`
  （`assets_dev_test.go:7`）：开发态仓库根缺少 `var/pineworker/worker.mjs` 时
  `retained_runtime_config` 返回 `MissingAsset{name:"PineTS worker bundle"}`，不伪造空 bundle。
- `native_tests.rs::pine_worker_bundle_file_name_is_platform_independent`（`assets_test.go:5`）：
  断言常量等于 `worker.mjs`、不含路径分隔符，且开发态/发布态两条派生路径都以该名结尾。
- `resource_integrity.rs::rejects_missing_staged_resource_instead_of_serving_without_it`
  （`asset_selection_boundaries_test.go:52`）：清单声明 `runtime/pineworker/worker.mjs` 但文件
  缺失时返回 `Read{path}` 并放弃启动，而不是当作不可用继续。
- `resource_integrity.rs::accepts_exact_resources_and_rejects_tampering`（加强，
  `assets_release_test.go:13`）：清单固件扩展为同时包含 node 与 `runtime/pineworker/worker.mjs`，
  断言精确匹配通过、篡改 bundle 后返回 `Hash{path=…/runtime/pineworker/worker.mjs}`。

生产代码变更（最小化，仅去重字面量）：`native_resource_integrity.rs` 新增
`PINE_WORKER_BUNDLE_FILE_NAME` 常量与 `pine_worker_bundle_fallback` 选择函数，
`retained_runtime_config` 改用它生成开发态/发布态默认路径（行为不变）。

### 探针（改坏→转红→按字节回滚）

1. `crates/jftrade-integration-pine/src/asset.rs`：把 `verify()` 的摘要比较改成恒假 →
   `test_select_from_fs_treats_missing_and_empty_bundles_as_unavailable` 与
   `checksum_is_verified_before_worker_asset_is_written` 同时红；回滚后 shasum
   `0eeba93efbebcf16d2abb8d2043d9dd615db39842acd0404dd21d2047b86454a`。
2. `apps/desktop/src-tauri/src/native_resource_integrity.rs`：把 `required_asset` 的
   `!path.is_file()` 判定改成恒假 → 新 dev 测试红（拿到 `Ok` 配置而不是缺资产错误）；回滚后
   shasum `1b2c50b83363fe0568a4b47b8ae072ccd0ad0d3f55ff94401077afb03df92102`。
3. `apps/desktop/src-tauri/src/resource_integrity.rs`：把 `if actual != entry.sha256` 改成
   恒假 → `accepts_exact_resources_and_rejects_tampering` 红（读错误用例不受影响，仍绿）；
   回滚后 shasum `76658877f933a4095e48790234db7cca5ab06a0a5a36a2dc242ca5577c1acafc`。
4. 同文件：把 `sha256_file(&path)?` 改成 `unwrap_or_else(|_| entry.sha256.clone())` 吞掉读错误
   → `rejects_missing_staged_resource_instead_of_serving_without_it` 红；回滚后 shasum 同 3。

### 保留差异与边界

- `isMissingAsset`（`asset_selection_boundaries_test.go:63`）是 Go 侧的文件系统错误分类
  shim：`errors.Is(fs.ErrNotExist)` 之后还按 `"file does not exist"`/`"no such file"`
  **字符串兜底**，以兼容某些平台 embed/fs 返回的未包装错误。Rust 宿主栈没有这一层，缺失判定
  由 `Path::is_file()` 与 `File::open`/`sha256_file` 的 `io::Error` 按类型承担，不存在需要按
  文案解析的来源；按文案判错迁入 Rust 属反模式，故记为 boundary 保留。
- 无 `fs.FS` 注入点：Go 能在单测里注入读失败的 FS；Rust 的等价可注入边界是 runtime 清单声明
  的资源文件读取（:52 结论），其余路径（`PineBundle::verify/materialize`）只在调用方给出
  bundle 后运行，读层由打包/清单阶段承担。
- Go 的 release 测试带 `t.Skip` 分支（未 staged 即跳过）；Rust 是「未声明不校验、声明即强
  校验」，没有跳过路径，证据强度更高但不与 Go 的跳过语义一一对应。
- Rust 没有 `Asset` 结构：元数据是 `PineBundle{file_name,bytes,sha256}` 与 runtime 清单
  `{resource,sha256}` 两种形态，故 :13 与 `assets_release_test.go:13` 分别落在两个 owner。
- `crates/jftrade-integration-pine/src/asset.rs` 的两条锚点仍写着退役目录名
  `internal/assets_worker`（reconcile unknown 55 中的 2 条）；因活动根禁止出现
  `internal/pineworkerassets` 文本，无法机械改写，保留为已登记的老锚点。

### 后续待办

- 下一批按域余量：`internal/desktop` 6 → `internal/retry` 6 → `internal/jftsettings` 5 →
  `internal/frontendassets` 4（锚点例外）→ `internal/research` 4 → …（同一锚点例外规则）。
- 待办：评估删除 `asset.rs` 两条退役路径锚点（本批已把对应证据落到 JSON），或在下一次锚点
  专项批次里统一处理 unknown go line 的 55 条。
- 待办：`scripts/build-pineworker-assets.test.mjs` 可补一条断言，直接锁定打包产物名
  `worker.mjs`（当前只断言平台无关构建参数），使 Node 侧与 Rust 常量形成双侧证据。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-desktop --all-targets
  --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop -p jftrade-integration-pine
  --all-targets --locked --no-fail-fast`：64 passed / 1 skipped / 0 failed（桌面壳 25 条含本批
  2 条新测试，pine 集成 39 条）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3030 Rust、`[x]` 1297、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 199（↑6，本批锚点
  例外）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1300 唯一引用（已记账 1245、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2907 tracked files）；`pnpm run check:ai-context`
  EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（affected 计划：policy/contracts/target-health、desktop nextest、
  rust fmt/clippy、compatibility desktop-runtime、desktop 全套脚本 48/48）。
- `pnpm run check:rust` EXIT=1：唯一阻塞点是 `pnpm run check:rust:policy`（`cargo deny check`）
  的 RUSTSEC-2026-0285（rustls 0.23.44，修复需 >=0.23.45）与 8 条
  `warning[advisory-not-detected]` 陈旧 ignore；run 停在静态阶段、未执行 workspace/all-targets
  阶段，等价测试面由上面的 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致
  （target-health 本次通过，未触发产物清理）。
