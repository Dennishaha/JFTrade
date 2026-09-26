# Market-Data 资产选择与缓存领域对齐批次

本文件记录 Go 基线 `internal/marketdataassets`（嵌入式平台 onedir 资产选择、私有目录物化、
内容寻址缓存与保留期清理）迁移到 Rust 的逐测试核对结论。Rust 侧对应入口：

- `crates/jftrade-integration-marketdata-helper`（保留的 host-side 侧车适配层：
  `AssetBundle` 校验/原子物化、`HelperProcess` 生命周期）；
- `crates/jftrade-engine`（运行时资源描述与桌面包 helper 解析：
  `product_resources`、`desktop_marketdata_helper`）；
- crates 之外的打包与资源校验：`apps/desktop/src-tauri/src/native_resource_integrity.rs`
  （`release_marketdata_helper_path`、`marketdata_helper_command`）、
  `apps/desktop/src-tauri/src/resource_integrity.rs`（`verify_release_resources`）、
  `scripts/build-marketdata-sidecar.mjs`、`scripts/lib/desktop-release-inputs.mjs`、
  `scripts/lib/materialize-directory-symlinks.mjs`。

## 第九十一批：`internal/marketdataassets` 全域收口（36 条）

### 范围与分片

`internal/marketdataassets/**` 剩余 **36 条 `missing`**（5 个文件），分两片：

- P0 资产选择与缓存边界 29 条（`asset_selection_boundaries_test.go` 15、
  `cache_test.go` 14）；
- P1 dev/release 资产生成与清单 7 条（`assets_dev_test.go` 3、
  `assets_release_test.go` 3、`assets_test.go` 1）。

### 结果

- 36 条全部给出结论：**3 条 `[x]`/`function_exact` + 24 条 `partial` + 9 条 `boundary`**。
- `internal/marketdataassets/**` 归零：36 条 = 3 `[x]` + 24 `partial` + 9 `boundary`，0 `missing`。
- 全局：4451 = function_exact **1221** + partial 2444 + boundary 541 + module_only 4 +
  missing **241**（前批为 1218 / 2420 / 532 / 4 / 277），Rust 测试 2933 → **2936**。
- 本批无生产实现变更：三个新增 Rust 回归测试全部落在
  `crates/jftrade-integration-marketdata-helper/src/asset.rs` 的既有 `AssetBundle` 契约上。

### 锚点例外（本批新增约定）

`internal/marketdataassets`、`internal/frontendassets`、`internal/pineworkerassets` 是已退休包，
`scripts/check-zero-go.mjs` 会在活动根（`.github/`、`apps/`、`crates/`、`scripts/`、`workers/`）
的文本里拒绝这三个路径字样。因此本批三条新 `[x]` **不写代码侧 `// Parity:` 锚点**，
证据改由上表、`manual-test-mappings.json` 与本文件承担；审计会把这些行计入
“function_exact 无锚点”告警（基线 193 → 196）。后续同域批次沿用该例外，不修改 `check-zero-go` 规则。

### 新增 `[x]` 证据（无代码锚点，理由见下节）

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `asset_selection_boundaries_test.go:204 TestMaterializeAssetRejectsDigestChanges` | `jftrade-integration-marketdata-helper/src/asset.rs::rejects_a_bundle_whose_bytes_no_longer_match_the_digest` | `[x]`：字节与记录摘要不一致时拒绝物化且不落盘（Go 断言 err 非空、available=false、materialized=nil）；差异仅在错误表达（Rust `AssetError::ChecksumMismatch{expected,actual}` vs Go 文案 `SHA256 changed`）。 |
| `asset_selection_boundaries_test.go:218 TestMaterializeAssetRejectsInvalidBundlePath` | `jftrade-integration-marketdata-helper/src/asset.rs::rejects_escaping_asset_names_before_writing` | `[x]`：`../sidecar` 越界名与 `nested/sidecar` 非裸名都在写盘前被拒绝、目录内无任何产物；差异仅在错误类型/文案（Rust `AssetError::InvalidName` vs Go `invalid market-data sidecar bundle file path`）。 |
| `cache_test.go:15 TestMaterializeCachedAssetReusesVerifiedContent` | `jftrade-integration-marketdata-helper/src/asset.rs::reuses_a_published_asset_without_rewriting_it` | `[x]`：二次物化复用已发布文件（同一路径、字节不变、inode 与 mtime 不变），对应 Go 的“第二次返回同一 Path 且 mtime 未被重写”；Go 末条 Cleanup no-op 断言在 Rust 结构上成立（`materialize` 无删除路径），测试同样断言复用后文件仍存在。 |

### 先红后改探针（证明新测试是真实守卫）

- 摘除 `AssetBundle::materialize` 的复用分支 → `reuses_a_published_asset_without_rewriting_it`
  转红（`a reused asset keeps the published file instead of republishing it`，inode 362203123 != 362203125）。
- 让 `verify()` 跳过摘要比较并放宽名称校验 → `rejects_a_bundle_whose_bytes_no_longer_match_the_digest`
  与 `rejects_escaping_asset_names_before_writing` 同时转红。
- 探针按字节回滚后 5 条 `asset::tests` 全绿，`git diff` 只剩新增测试行（102 insertions，0 deletions）。
  本批无生产行为修复需求，故未改实现。

### 保留差异候选（保持 partial 的理由）

- **P0 资产选择（15 条中的 12 条）**：Go 从 build-tag 区分的 embed FS 选出整个 PyInstaller
  onedir 包（可执行 + `lib/*` 多文件、按排序路径+字节计算 SHA256、拒绝 symlink 条目），
  并用 `(Asset, available, error)` 三元组表达“本构建未嵌入资产”。Rust 没有 onedir 资产模型：
  `AssetBundle` 是单文件内容寻址适配器，release 侧车直接作为 Tauri 资源目录随包分发
  （`runtime/marketdata/marketdata-sidecar-<platform>-<arch>/<name>`），一致性由
  `verify_release_resources` 的清单 SHA256 覆盖，缺资产以 `MissingAsset`/`MissingExecutable` fail-closed 表达。
- **平台命名推导（`TestBinaryNameForUsesGoPlatformNames`、`TestBinaryNameUsesCurrentRuntime`）**：
  Go 暴露可传 `goos/goarch` 的 `BinaryNameFor`；Rust 用 `cfg!(target_os/target_arch)` 编译期推导单目标名，
  且该逻辑位于 `apps/desktop`（crates 之外），审计 `rust_entry` 只解析 `crates/**`，
  故只能记 `partial` 并把 Node 侧断言（`scripts/lib/desktop-release-inputs.test.mjs`）写进结论。
- **P1 缓存与保留期（`cache_test.go` 7 条 boundary）**：`cachedAssetRetention`(7 天)、
  `pruneCachedAssets`/`PruneCached`、`ensurePrivateCacheRoot`、`removeInvalidCacheTarget`、
  内容寻址目录与非常规文件/权限形状校验在 Rust 没有对应实现——release 侧车不从缓存解包，
  这些机制随部署模型一并消失，按“不适用/需保留边界”登记，不强行迁移。
- **清理句柄（`TestCleanupHandlesNilAndAlreadyCleanedAssets`）**：Rust 不返回可清理句柄，
  资源目录所有权归打包流程，nil/重复清理不可达。
- **`isMissingAsset` 文案匹配（`TestIsMissingAssetRecognizesOnlyNotFoundErrors`）**：Rust 使用
  `ErrorKind::NotFound`/`Path::is_file` 判定，不复制 “file does not exist”/“no such file” 字符串匹配；
  意图等价、实现不同。
- **dev/release 构建开关（`assets_dev_test.go`、`assets_release_test.go`）**：Go 用 `release_assets`
  build tag + `DevelopmentOverridesAllowed()`；Rust 用一条解析链（`JFTRADE_MARKETDATA_SIDECAR`
  → `JFTRADE_MARKETDATA_DEV_PYTHON`/venv → release 资源目录），打包阶段由
  `scripts/build-marketdata-sidecar.mjs`（Node 测试：`build stages one host-specific PyInstaller onedir asset`）
  与 `scripts/lib/materialize-directory-symlinks.mjs`（Node 测试：
  `materializes internal file and directory symlinks as regular bundle entries`、
  `rejects bundle symlinks that escape the output directory`）保证 macOS loader 布局为常规文件。

验证：`cargo fmt --all`；`cargo clippy -p jftrade-integration-marketdata-helper --all-targets --locked`；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-marketdata -p jftrade-datamanagement --all-targets --locked --no-fail-fast`（90 passed / 0 skipped）；
`node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-marketdata-helper -p jftrade-marketdata -p jftrade-datamanagement -p jftrade-engine --all-targets --locked --no-fail-fast`（**1808 passed / 0 skipped**）；
`python3 scripts/compatibility/audit_test_parity.py`（4451 Go / **2936 Rust** / **1221 `[x]`**；missing 241、partial 2444、boundary 541、module_only 4；`OK: 1221 function_exact mappings cite existing workspace tests`、0 破坏引用、0 条 `[x]` 缺少 function_exact；未锚定 193、7 条 partial 无解析引用、2 条无断言为前批基线告警）；
本批未新增代码锚点，审计“function_exact 无锚点”告警由基线 193 变为 **196**（新增 3 条按上节例外处理）；
`python3.12 scripts/compatibility/parity_anchor_reconcile.py`（1219 唯一锚点引用，1124 已记账，
未记账 6 条仍为 `internal/settings` 前批基线）；`node scripts/check-zero-go.mjs`（2893 tracked files / 0 release artifact，通过）；
`pnpm run check:rust:architecture`；`pnpm run check:compatibility`（EXIT=0）；`pnpm run check:ai-context`；`git diff --check`。

`pnpm run check:quick` 最终 **EXIT=0**（计划：policy、contracts、target-health、nextest `-p jftrade-desktop -p jftrade-engine -p jftrade-integration-marketdata-helper` **1760 passed / 0 skipped**、format:rust:check、clippy、check:desktop）。
首次 `check:quick` 有两处中间失败，均已定位并复验，原始证据保留：
① `check:rust:target-health` 报 `target/debug/deps` 超过 5 万个 `.rcgu.o` —— 确认无 cargo/rustc 进程后执行 `pnpm run clean:rust:artifacts`（`cargo clean` 删除 126753 个文件）再重跑；
② 工作区并发跑时 `product_adk_model_runtime_takeover_tests.rs::replay_safe_tools_re_execute_on_takeover_and_deduplicate_subsequent_claims` 报
`claim 1: LeaseLost("run run-replay-safe lease fencing token 1 is no longer current")`；单独运行该测试 **1 passed**，重跑整条 `check:quick` 亦通过，属并发饱和下的既有 flaky 家族，与本批 diff 无关。

`pnpm run check:rust` **EXIT=1**，唯一失败项为环境级 `check:rust:policy`（`cargo deny check` → advisories FAILED），保留原始证据不记为通过：
**RUSTSEC-2026-0285**（rustls 0.23.44，修复 >=0.23.45；根 `Cargo.toml` 精确锁定 `=0.23.44`，解除需一次显式的依赖升级批次并复核 `deny.toml`），
同次输出另有 8 条 `warning[advisory-not-detected]`（deny.toml 陈旧 ignore 条目）为警告而非错误。
该次运行中 `check:rust:target-health`、`check:rust:architecture`、`check:rust:production-policy`、`format:rust:check`、`check:clippy` 均已通过。

## 第九十二批：P1 缓存安全与并发物化收口（10 条）

本批先处理高风险缓存差异，读取 Go `cache_test.go` / `assets_release_test.go` 实现并核对 Rust `AssetBundle` owner。新增 Rust 回归覆盖：

- `cache_test.go:45:TestMaterializeCachedAssetRepairsTamperAndSymlink` → `repairs_tampered_asset_and_symlink_targets`：单文件篡改与符号链接目标会被校验后原子修复；onedir 依赖树与权限仍是 partial。
- `cache_test.go:83:TestMaterializeCachedAssetPublishesConcurrently`、`:118:TestPublishCachedAssetAcceptsOnlyAValidConcurrentWinner` → `concurrent_materialization_publishes_one_verified_file`：并发物化收敛到同一已校验目标；多文件 staging/无效赢家仍是 partial。
- `cache_test.go:159:TestMaterializeCachedAssetPrunesOnlyExpiredDigests`、`:185:TestPruneCachedAssetsIgnoresMissingCacheRoot` → `prunes_only_expired_cache_directories_and_ignores_missing_root`：缺失根不创建、过期目录清理、当前项保留；Rust retention 由调用方传入，仍是 partial。
- `cache_test.go:193:TestMaterializeCachedAssetRejectsUnsafeCacheRoot` → `rejects_a_cache_root_that_is_a_file`：普通文件 cache root fail-closed；私有模式和 onedir 形状仍是 partial。
- `assets_release_test.go:72:TestMaterializeCachedReleaseAssetReusesBundleAndFallsBack`：复用/修复/根目录校验已有 Rust owner；release 资源目录没有 Go 的临时 fallback，保留 partial。

先红后修探针：移除 cache-root 类型检查、并发赢家复用或篡改重写分支时，对应 helper nextest 会失败；恢复后 `jftrade-integration-marketdata-helper` 26/26 通过。该批没有把多文件 onedir 行为升级为 exact。
