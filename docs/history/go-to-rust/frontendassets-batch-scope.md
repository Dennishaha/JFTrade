# 前端资源包域对齐批次（第 106 批）

本文件记录 Go 基线 `internal/frontendassets`（`FileSystem()` 的 dev/release 构建标签选择、
release 内嵌 `dist.zip` 的内容约束）迁移到 Rust 的逐测试核对结论，涉及
`apps/desktop/src-tauri`（Tauri `frontendDist`/`devUrl` 装配与 `resource_integrity`）、
`scripts/build-frontend-assets.mjs`（发布 staging 与 web 清单）、`scripts/stage-docs.mjs`
（文档页 staging）、`scripts/check-oss-license.mjs`（许可文案闸门）与本次新增的
`scripts/check-web-assets.mjs`（staged 资产闸门）。

## 第一百零六批：internal/frontendassets 收口（4 条）

### 范围与分片

Go 侧 3 个文件（`dev.go`/`release.go`/`load.go`）与 2 个测试文件，本批覆盖 4 条 `missing`：

- P1 `dev_test.go:7`：开发构建不内嵌前端资源（`FileSystem()` = `(nil,false,nil)`）。
- P1 `release_test.go:14`：release 内嵌 `dist/assets` 下划线前缀 chunk。
- P1 `release_test.go:49`：release 内嵌文档与法律声明页（存在、非空、含许可文案）。
- P2 `release_test.go:96`：内嵌文本资产不得含被移除的 Go Pine 运行时引用。

分类：1 条 `[x]`/function_exact + 3 条 partial；`internal/frontendassets` 域内 `missing` 归零。

### 关键事实（本批 recon 实测）

- **锚点例外实测**：`scripts/check-zero-go.mjs::checkZeroGo` 用
  `fs.readFileSync(trackedFile, "utf8")` 读取**每个 tracked 文件**并按行跑
  `activeTextPattern`；该正则含 `internal/(?:frontendassets|marketdataassets|pineworkerassets)`。
  实测 `git grep -n "internal/frontendassets" -- '*.rs'` 为 0 命中，同族
  `internal/pineworkerassets` 的既有 `[x]` 行同样没有代码锚点——本批沿用该规则，不写
  `// Parity:` 锚点，改由清单结论记录例外原因。
- **Rust 无内嵌归档 FS**：Go 用 `//go:embed dist.zip` + `zip.NewReader`；Rust 桌面壳由
  `apps/desktop/src-tauri/tauri.conf.json` 的 `frontendDist=../../web/dist` 在编译期内嵌
  staging 目录（非归档、无 glob 过滤），开发态由 `devUrl=http://127.0.0.1:3003` +
  `beforeDevCommand`（`scripts/dev-tauri-frontend.mjs`）提供外部 dev server 资源。
- **发布输入与内容约束**：`scripts/build-frontend-assets.mjs` 依次执行
  `build:web:generated`、`build:docs:generated`、`stage:docs`、web 生产包 smoke，随后写
  `runtime-assets/web/manifest.json`（逐文件 sha256，schema `jftrade.web-assets.v1`）；
  `scripts/lib/desktop-release-inputs.mjs` 再把该清单作为 release 准备输入锁定。
  文档页来自 VitePress（`docs/legal/license.md`、`docs/legal/third-party-notices.md` →
  `docs/legal/*.html`），文案要点由 `scripts/check-oss-license.mjs` 在源文件上校验。

### 结果

- 全局：4451 = function_exact **1311** + partial **2545** + boundary 573 + module_only 4 +
  missing **18**（前批 1310 / 2542 / 573 / 4 / 22）。
- Rust 测试 3041 → **3042**（新增 1 条）；Node 侧新增闸门脚本 1 个 + 其测试 1 个。
- 锚点对账：1314 唯一引用（已记账 1259、unrecorded **0**、unknown 55、stale **0**）——
  本批按锚点例外不新增锚点；未锚定 function_exact 告警 199 → **200**（新增的 1 条 `[x]` 行）。

### 本批新增/加强的证据

- `apps/desktop/src-tauri/tests/desktop_contracts.rs::desktop_shell_keeps_development_frontend_external_and_embeds_staged_dist_for_release`
  （`dev_test.go:7`）：读取 `tauri.conf.json`，断言开发态外部资源（`devUrl` 为
  `http://127.0.0.1:3003`、`beforeDevCommand` 含 `dev-tauri-frontend.mjs`）与发布态内嵌
  staging 目录（`frontendDist=../../web/dist`、`bundle.active=true`）。
- `scripts/check-web-assets.mjs`（新增，发布闸门，由 `build-frontend-assets.mjs` 在写清单后调用）：
  1) 清单与 `apps/web/dist` 文件集合/摘要一一对应（多、少、摘要不符都失败）；
  2) `assets/_` 前缀 staged 资产必须进入清单（对应 `release_test.go:14`）；
  3) `docs/legal/license.html` 与 `docs/legal/third-party-notices.html` 必须存在、非空并含
     许可要点（对应 `release_test.go:49`）；
  4) `html/js/css/json/txt/map` 文本资产不得含 `pkg/strategy/pineruntime`、
     `BenchmarkPineRuntime`、`BenchmarkRunExecutesPineGoldenMatrix`，按 Go 语义跳过 `docs/`
     （对应 `release_test.go:96`）。
- `scripts/check-web-assets.test.mjs`（新增）：临时目录 fixture 覆盖通过路径与「清单漏下划线
  资产」「摘要不符」「清单声明未 staged 文件」「文档页缺失/为空」「文本资产命中禁用引用」
  「docs 内同名文案不失败」「dist/manifest 不可读」等失败路径。

### 探针（改坏→转红→按字节回滚）

1. `scripts/check-web-assets.mjs`：`name.startsWith("_")` 改成 `startsWith("__")` →
   `node scripts/check-web-assets.test.mjs` 红；回滚后 shasum
   `8a336ce01f105d8ed79c4f7d625b1bc8847d6fc917733a4cbaa8c6b44856dc0f`。
2. 同文件：摘要比较条件追加 `&& false`（不再报摘要不符）→ fixture 红；回滚到同一 shasum。
3. 同文件：清空 `removedRuntimeReferences` → fixture 的禁用引用断言红；回滚到同一 shasum。
4. `apps/desktop/src-tauri/tauri.conf.json`：`devUrl` 改 `http://127.0.0.1:9999` → 新增 Rust
   测试红（`left: 9999 / right: 3003`）；回滚后 shasum
   `181ea787916126a19b7ceb668b5e4a68b32502d49cfb3aae3d26f2d86a6a7c86`。
5. 同文件：`frontendDist` 改 `../../web/dist.zip` → 同一测试红；回滚到同一 shasum。

### 保留差异与边界

- **无 `dist.zip`/`FileSystem()`/`available` 标志**：Go 的运行期
  `(fs.FS, bool, error)` 三态在 Rust 由「Tauri 构建模式一次性决定」替代；`dev_test.go:7`
  以配置契约测试映射为 function_exact，`release_*` 三条因缺少运行期内嵌 FS 记为 partial。
- **文档文案按 Rust 现状**：Rust 仓库的 `docs/legal/third-party-notices.md` 只把 BBGO 作为
  文档快照（`github.com/c9s/bbgo@v1.64.2`）引用，不再分发 bbgo 运行时，因此没有
  `Copyright Suneido Software Corp.`，许可要点改为 MIT/Apache 文本行；闸门按现行文案断言，
  与 go:452dea11 的 bbgo 运行时声明差异属产品差异。
- **`apps/web/dist` 与 `runtime-assets/web/manifest.json` 不入库**：Rust 测试无法离线断言
  「内嵌内容」，故 3 条 release 行没有 Rust 函数级证据；若后续要求 function_exact，需要把
  staging 校验迁入 `apps/desktop`（或引入读取已构建 dist 的显式 live 校验）。

### 发现：HEAD 既有阻塞（非本批引入）

`pnpm run build:docs:generated`（VitePress）在 HEAD 即失败：

```
[plugin vite:vue] docs/history/go-to-rust/assistant-workflow-adk-batch-scope.md:112:81
[plugin vite:vue] docs/history/go-to-rust/test-parity-inventory.md:1309:89
RolldownError: Element is missing end tag.
```

复现与定性证据：把本批重新生成的 `manual-test-mappings.json`/`test-parity-inventory.md`/
`test-parity-report.md` 用 `git stash` 还原到 HEAD 后重跑，仍报同样两个文件、同样 2 个错误
（`HEAD_DOCS_EXIT=1`），说明失败只依赖 HEAD 已提交的文档内容（形如 `go:452dea11:<go 文件>:<行>`
的尖括号占位符被 markdown-it 当作内联 HTML 标签）。影响：`build:frontend-assets:generated`
（CI `ci.yml` 的 “Build desktop Web assets” 与 release workflow 的 “Build release inputs”）
在 docs 阶段即失败，本批新增闸门位于其后的 manifest 步骤，因此本机无法完成端到端装配。
本批不修（会改动其他批次的历史文档与审计脚本生成格式），登记为独立后续项。

本机证据补充：改动前的完整 staging（web + docs，707 文件）在闸门下通过；本次部分重跑
（web 成功、docs 失败）后 dist 与旧清单不一致，闸门精确报出 827 条不一致，其中包含
「下划线资产未打包」与「legal 页缺失」——闸门的失败检测在真实产物上得到验证。

### 后续待办

- 独立修复 docs 构建：审计脚本生成 markdown 时转义尖括号；同时修正
  `assistant-workflow-adk-batch-scope.md` 等历史文档（需先枚举全部同类占位符）。
- 下一批按域余量：`internal/research` 4 → `internal/security/passwordhash` 3 →
  `cmd/jftrade-api` 2 → `cmd/check-go-coverage` 2 → `pkg/besteffort` 2 → 直至 4451 条全部完成。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-desktop --all-targets
  --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop --all-targets --locked
  --no-fail-fast`：29 passed / 0 skipped / 0 failed（含本批新增 1 条）。
- `node scripts/check-web-assets.test.mjs` EXIT=0；`node scripts/check-web-assets.mjs`
  在改动前的完整 staging 上通过（707 文件），在本次 partial staging 上按预期失败。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3042 Rust、`[x]` 1311、
  0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少 function_exact；未锚定 200（本批锚点例外，
  +1）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1314 唯一引用（已记账 1259、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:compatibility` EXIT=0；`pnpm run check:rust:architecture` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2913 tracked files，含本批两个新脚本）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=1：唯一失败阶段 `pnpm run check:rust:static`，其内部
  target-health/architecture/production-policy/fmt/clippy 全部通过，仅 `check:rust:policy`
  （`cargo deny check`）因 RUSTSEC-2026-0285 与 8 条陈旧 advisory 失败——与 `check:rust`
  的既有阻塞同源；同一 run 的其余阶段（nextest 3139 passed / 2 skipped、web coverage 与
  typecheck、pine、Python sidecar、兼容回放）全部通过。
- `pnpm run check:rust` EXIT=1：同上 policy 阻塞（run 停在静态阶段，workspace/all-targets
  不执行），等价测试面由 nextest wrapper 覆盖。**不记为通过**，与干净 HEAD 行为一致。
