# 覆盖度门禁域对齐批次（第 110 批）

本文件记录 Go 基线 `cmd/check-go-coverage`（Go 覆盖度门禁工具）的 2 条 `missing` 测试迁移到
Rust 的逐条核对结论。该工具随 Go 运行时一并删除，仓库现有的等价 owner 是 Node 质量门禁
`scripts/check-web-diff-coverage.mjs` 与其脚本测试 `scripts/check-web-diff-thresholds.test.mjs`。

## 第一百一十批：cmd/check-go-coverage 收口（2 条，均为 partial）

### 范围与分片

Go 侧 5 个源文件 + 5 个测试文件，本批覆盖 2 条 `missing`：

- P1 `changed_lines_analysis_test.go:71`：`TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements`
  （纯改名 diff 只登记新路径、lines 为空，不得虚增变更语句）。
- P1 `profile_analysis_test.go:109`：`TestAnalyzeProfilesRejectsEmptyBusinessCoverage`
  （空 profile 与“无业务语句”两种输入必须失败关闭）。

分类：2 条 partial（等价实现存在且已被脚本测试覆盖，但没有 Rust 实现/测试，审计只认
`*.rs::测试`，故不满足 function_exact）；`cmd/check-go-coverage` 域内 `missing` 归零。

### 关键事实（本批 recon 与实测）

- **Go 语义**：`parseChangedGoLines` 解析 `git diff` 文本，纯改名（`similarity index 100%` +
  `rename from/to`）时 `files={internal/new.go}`、`lines` 为空；`analyzeProfiles` 在
  `len(profiles)==0` 时报 `coverage profile contains no source files`，在
  `business.total==0`（所有语句都命中 `exclusionRules`，如 generated/vendored/tooling）时报
  `coverage profile contains no business statements`（`coverage.go:178-181`、`221-223`）。
- **Rust 侧无等价实现（实测 grep）**：`rg -n "find-renames|--unified=0|name-status" crates --type rust`
  0 命中；仓库没有 llvm-cov/覆盖度分析 owner，Rust 门禁只跑测试与静态检查。
- **等价 owner 一（diff 解析）**：`scripts/check-web-diff-coverage.mjs::changedWebSourceLines`
  先用 `git diff --name-status -z --diff-filter=ACMR --find-renames`（:144-151）取改动文件，status
  以 `R` 开头时回带 `previousPath`（:188-199），再用 `--unified=0 --find-renames` 解析 hunk 头
  （:221-232）得到新增行号；只有行号集合非空才登记该文件（:173-176）。纯 `git mv` 没有任何
  hunk → 文件不入 changed，等价于 Go 的“不虚增变更语句”。
- **等价 owner 二（缺失数据失败关闭）**：同一文件的 `checkWebDiffCoverage` 在改动文件找不到
  覆盖条目且改动行看起来可执行时，push `missingCoverage` 并使 `passed=false`（:86-101），
  渲染为 `missing coverage entry for executable changed code`（:497），对应 Go 第一类
  “没有数据即失败” 的守卫。
- **实测证据**：`node scripts/check-web-diff-thresholds.test.mjs` EXIT=0。其中
  `assertStagedRenameDiff`（:56-84）在临时仓库 `git mv` 后断言 `changedWebSourceLines` 为空，
  随后只改第 4 行再断言恰好为 `[[新路径, Set([4])]]`；:37 与 :43 断言两条 missing coverage
  entry 失败路径。该文件属于 `test-scripts` 的 policy 套件（`pnpm run test-scripts -- policy`，
  check:quick 会执行）。

### 逐条结论

- `changed_lines_analysis_test.go:71` → partial：改名语义由 `changedWebSourceLines` 的
  `--find-renames` + hunk 解析承载，脚本测试 `assertStagedRenameDiff` 给出“纯改名零变更、
  改一行只报一行”的双向断言。保留差异：Go 返回 `files`+`lines` 全局映射（新路径仍在 files、
  lines 为空），Node 直接省略零变更文件；Go 覆盖 `.go` 全仓，Node 只覆盖 `apps/web/src` 下的
  `ts/tsx/vue`。
- `profile_analysis_test.go:109` → partial：Go 的两类 fail-closed 守卫中，“缺数据”一类由
  `missing coverage entry` 失败承载（:37/:43 测试）；“无业务语句”一类在 Node 侧**无等价物**：
  `changedFiles.size===0` 时直接 `passed: true`（:73-80），属 fail-open，与 Go 相反。该差异已
  登记为后续项（建议补“存在改动文件但覆盖报告无任何源文件条目 → 失败”的守卫与脚本测试），
  本批不擅自改动 CI 门禁语义。

### 保留差异与后续项

- fail-open vs fail-closed：Go 覆盖工具在“无可测量输入”时失败，Node web 门禁在“无 web 改动”
  时通过；这是设计取向差异而非缺陷复现，但会漏掉“报告为空却存在改动文件”的场景。
- 覆盖面差异：Node 门禁只评估 web 源码与变更行覆盖阈值；Go 工具评估全仓 Go 包的业务/关键
  域覆盖率并输出分域报告，二者阈值模型不同（`isCriticalWebPath` vs `criticalDomains`）。
- 后续项：为 web 门禁补空报告守卫并加脚本测试；若要 function_exact 收口，需要在 Rust 侧引入
  diff 解析/覆盖率分析实现，当前无此 owner。

### 锚点例外

`cmd/check-go-coverage` 不在 `scripts/check-zero-go.mjs` 的禁用文本内，但审计只扫描 Rust 文件，
Node 脚本无法携带 `// Parity:` 锚点；本批 2 条 partial 行按 `internal/frontendassets` 先例登记
锚点例外，未锚定 function_exact 计数保持不变（202）。

验证：
- 本批无 Rust 生产代码/测试改动，因此以脚本门禁与其测试作为证据面；`cargo fmt --all -- --check`
  EXIT=0（未改动 Rust 文件，fmt 仅作回归确认）。
- `node scripts/check-web-diff-thresholds.test.mjs` EXIT=0：覆盖 missing coverage entry 两条失败
  路径（:37、:43）与纯改名/单行改动双向断言（`assertStagedRenameDiff`，:56-84）。
- grep 证据：`rg -n "find-renames|--unified=0|name-status" crates --type rust` 0 命中（Rust 侧无
  diff 解析/覆盖分析实现）；Node 侧命中 `scripts/check-web-diff-coverage.mjs:144/147/169/223`。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3052 Rust、`[x]` 保持 1320、
  `missing` 9 → 7（本批 2 条判 partial）、0 破坏引用、0 重复 rust_entry、0 条 `[x]` 缺少
  function_exact；未锚定 function_exact 保持 202（本批无新锚点）、partial 无解析引用 7（既有基线，
  本批行不含可解析引用故不计入）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1321 唯一引用（已记账 1266、
  unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `node scripts/check-zero-go.mjs` EXIT=0（2918 tracked files，含本批新文档）；
  `pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0（含 `test:scripts -- policy` 套件，覆盖本批引用的脚本测试）。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`），报
  RUSTSEC-2026-0285 漏洞与陈旧 advisory 告警，与本仓既有基线同源。**不记为通过**；该 run 停在
  静态阶段，测试面由上面的脚本测试与既有 nextest 结果覆盖。
