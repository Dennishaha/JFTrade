# 前端资产归档批次（第 116 批）

本文件记录 Go 基线 `scripts/archive_frontend_assets_test.go` 最后一条 `missing` 行的核对结论。
本批结论是**不适用/边界保留**：Go 的 `archiveFrontendAssets` 是给旧 Go/Wails 二进制做**确定性
zip 内嵌**的构建脚本，Rust/Tauri 侧不存在同类归档函数；等价打包保证由 Node staging 闸门
（`scripts/build-frontend-assets.mjs` + `scripts/check-web-assets.mjs`）与
`tauri.conf.json` 的 `frontendDist` 编译期内嵌承担。本批无 Rust 生产代码与测试变更，只落清单、
scope 文档与审计产物；全仓 `missing` 由此**归零**。

## 第一百一十六批：scripts 归档脚本收口（1 条）

### 范围与分片

Go 侧 1 文件 1 条：

- P2 `scripts/archive_frontend_assets_test.go:11`：
  `TestArchiveFrontendAssetsPreservesRelativePathsAndTimestamp`（原 `missing`；temp dir 里放
  `root.txt` 与 `nested/child.txt`，断言 zip 恰好 2 个条目、名字为 `nested/child.txt` 与
  `root.txt`（相对 POSIX、WalkDir 字典序）、每个条目 `Modified == time.Unix(0,0).UTC()`、
  内容逐字节相等）。

分类：1 条 `[~]`/`boundary`；全仓 `missing` 1 → **0**（4451 行清单首次全部有结论）。

### 关键事实（本批 recon 与实测）

- **Go 机制**（`scripts/archive_frontend_assets.go`）：`package main` 的构建脚本，
  `-src`/`-dst` 必填；`os.MkdirAll(filepath.Dir(dst))` → `os.Create(dst)` → `zip.NewWriter`；
  `archiveFrontendAssets` 用 `filepath.WalkDir` 遍历，目录跳过，其余走
  `archiveFrontendAssetFile`：`filepath.Rel` 求相对路径，header 固定
  `Name=filepath.ToSlash(relPath)`、`Method=zip.Deflate`、`Modified=fixedArchiveTimestamp`
  （`time.Unix(0,0).UTC()`），内容 `io.Copy` 原样写入；非普通文件返回
  `unsupported file type in frontend assets`。deferred `Close` 只走 `besteffort.LogError`。
- **Rust/Tauri 侧没有归档容器**：控制台由 Vite 产出 `apps/web/dist`，
  `scripts/build-frontend-assets.mjs` 依次跑 `build:web:generated`、`build:docs:generated`、
  `stage:docs`、`test:production-bundle`，然后 `writeWebManifest` 写
  `runtime-assets/web/manifest.json`：`{schemaVersion: "jftrade.web-assets.v1", files: [{path, sha256}]}`，
  `path` 是相对 POSIX 路径（`file.slice(dir.length + 1).split("\\").join("/")`），
  `filesBelow` 递归收集后 `sort()`，摘要为文件内容 sha256。
- **闸门在 Node 侧**：`scripts/check-web-assets.mjs::verifyStagedWebAssets` 双向核对清单与 dist
  文件集合（`staged web asset is missing from the release manifest` /
  `release manifest declares an asset that is not staged`）、逐文件比对 sha256
  （`release manifest digest differs for the staged asset`）、强制 `assets/` 下划线 chunk 入清单
  （`staged bundle asset with a leading underscore is not packaged`）、校验 `docs/legal` 页面存在
  非空且含许可文本、扫描被移除的 Go Pine 运行时引用；fixture 由
  `scripts/check-web-assets.test.mjs` 覆盖（本机 EXIT=0）。发布侧
  `scripts/lib/desktop-release-inputs.mjs` 把 `runtime-assets/web/manifest.json` 列为必需输入，
  `.github/workflows/desktop-release.yml` 先跑 `pnpm run build:frontend-assets:generated` 再
  `write-desktop-release-input-manifest`。
- **内嵌方式**：`apps/desktop/src-tauri/tauri.conf.json` 的 `frontendDist` 指向 `../../web/dist`，
  由 Tauri 在构建期把 staging 目录直接打进桌面包（无 zip、无 glob 过滤）；
  `crates/**` 内没有任何读取 web manifest 的生产代码——`crates/jftrade-api` 的 `AssetBundle`
  只是内存表（`ports.rs` 的 `AssetBundle::new/get/spa_index`），生产控制台由 Tauri 壳提供。

### 逐条结论

`scripts/archive_frontend_assets_test.go:11` → **不适用/边界保留**。Go 的 4 个断言与 Rust/Node
现有保证逐项对应如下：

| Go 断言 | Rust/Node 等价保证 | 证据位置 |
| :--- | :--- | :--- |
| 条目数 = 普通文件数（目录跳过） | 清单与 dist 文件集合双向一一对应，任一方向缺失即失败 | `verifyStagedWebAssets` |
| 相对 POSIX 路径且按路径排序 | `writeWebManifest`/`stagedFiles` 双处 POSIX 归一 + 显式 sort | `build-frontend-assets.mjs`、`check-web-assets.mjs` |
| 每个条目 `Modified` 固定为 Unix(0) | 无归档容器、无 mtime 概念；确定性改由逐文件 sha256 内容寻址承担 | `manifest.json` 的 `files[].sha256` |
| 内容逐字节保留（child/root） | 摘要相等即内容相等，另加文档页文本要求 | `verifyStagedWebAssets`、fixture |

无对应实现的一项：Go 对非普通文件报 `unsupported file type in frontend assets`；Node 侧
`filesBelow` 只收 `entry.isFile()`，其余条目静默跳过——属**保留差异**（见下节）。

### 复现与保留差异

- **复现（fixture 级）**：`node scripts/check-web-assets.test.mjs` EXIT=0，覆盖清单漏项、
  摘要不符、文档页缺失/为空、遗留运行时引用、dist/manifest 不可读等失败面。
- **复现（工作树级）**：在未重新 staging 的工作树上直接跑 `node scripts/check-web-assets.mjs`
  会失败，报 `assets/_plugin-vue_export-helper-BTyRynMl.js` 未入清单与 `docs/legal/*.html` 缺失。
  成因是重构后的 `apps/web/dist` 覆盖在旧 `manifest.json` 之上（`check:quick` 会重建 dist），
  叠加既有 docs 构建阻塞——`pnpm run build:docs:generated` 在
  `docs/history/go-to-rust/assistant-workflow-adk-batch-scope.md`（vitepress 报 `Element is missing
  end tag.`）处失败，导致 `stage:docs` 从未产出 legal 页面。发布流水线按顺序重新生成两者，
  因此该现象是本地陈旧产物，不是功能差异；本批据实记录，不改动 dist/manifest（均为未入库产物）。
- **无 zip / 无 mtime / 无 Deflate**：Rust 不再需要“固定时间戳”这一可重现性手段，因为发布输入是
  内容寻址清单而非归档字节；压缩交由 Tauri 打包阶段。
- **non-regular 文件不再报错**：Go 显式拒绝，Node 静默跳过（清单校验只覆盖 `isFile()` 条目）。
  现实输入是 Vite/VitePress 产物，出现 symlink 等条目时会直接反映为清单缺失/多余。
- **校验语言与位置不同**：Go 在 `scripts/*.go`（同进程 zip）里断言；Rust 仓库的等价断言在
  `scripts/*.mjs`（Node）与 Tauri 构建配置里，`crates/**` 无可迁移的 Rust 函数。

### 后续待办

- 若产品要把该校验上移到 Rust（本行才可能升级为 `function_exact`）：修复位置为
  `apps/desktop/src-tauri/src/resource_integrity.rs`（在既有 `jftrade.tauri-runtime.v1` 校验旁新增
  `jftrade.web-assets.v1` 校验）或 `apps/desktop/src-tauri/build.rs` 的构建期校验；回归要求：
  headless fixture 覆盖相对 POSIX 路径、排序稳定、摘要不符、清单缺项/多项、下划线 chunk、
  非普通文件拒绝，并在发布流水线调用。
- 既有独立项（本批补齐复现器）：`docs/history/go-to-rust` 多份 markdown 含 `<name>`/`<text>` 之类
  尖括号占位符，`pnpm run build:docs:generated` 因此在 vitepress/Vue 解析阶段失败；修复需按文件
  转义或改用 backtick，属独立文档批次。
- 承第 114 批的独立项：为 Rust 桌面补“周期性更新检查”（对应 Go `startDesktopUpdateChecks` 的
  24 小时节奏），复用签名 updater 与 `jftrade:desktop-update:available` 事件。
- 下一批（第 117 批）：`missing` 已归零，转入 **2544 条 `partial`** 的逐域收口。按 partial 密度
  排序为 api_transport 478、strategy_pine 465、assistant_workflow 447、other 311、
  backtest_calendar 262、storage_sqlite 178、marketdata_quotes 155、trading_broker 105、
  futu_opend 104、settings_watchlist 39；按计划中的 P0（公开 API、交易/资金安全、唯一写入所有权）
  优先，第 117 批取 **trading_broker** 域第一片：`internal/trading/execution_test.go` 的 17 条
  partial（成交/撤单/资金与幂等边界），逐条升级为 `function_exact` 或给出 boundary 结论，
  随后依次 `order_updates_test.go`(15)、`execution_combo_lifecycle_test.go`(12)、
  `pkg/broker/broker_test.go`(8) 等 22 个文件分片推进。

验证：
- `node scripts/check-web-assets.test.mjs` EXIT=0（`staged web asset checks behave as expected`）。
- `node scripts/check-web-assets.mjs` EXIT=1（工作树陈旧 dist/manifest 与 docs 未 staging，见上节复现；
  非本批功能差异，未改产物）。
- `pnpm run build:docs:generated` EXIT=1（vitepress 在 `assistant-workflow-adk-batch-scope.md`
  报 `Element is missing end tag.`；HEAD 既有阻塞，本批记录复现器）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3067 Rust、`missing` 1 → 0、
  boundary 573 → 574、`[x]` 1329 不变、0 破坏引用；未锚定 function_exact 202、partial 无解析引用 7、
  无断言 2（均既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1330 唯一引用（已记账 1275、
  unrecorded 0、unknown 55、stale 0）不变（boundary 行不写锚点）。
- `node scripts/check-zero-go.mjs` EXIT=0（2929 tracked files）；`pnpm run check:ai-context` EXIT=0
  （6 modules / 8 instruction files）；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（纯文档/映射批次：compatibility replay 19 passed / 0 failed、
  scripts 59 tests OK、actionlint 10 workflow files；无 Rust 编译面变更）。
