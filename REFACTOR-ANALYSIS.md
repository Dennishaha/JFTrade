# JFTrade 工程与业务优化分析清单

初版分析:2026-07-26 · 基线 commit `3482f994`
本轮复核:2026-07-28 · 复核 commit `6eb2e22b` · 所有数值为本次实测

---

## 本轮结论摘要

**P0 三条全部闭环,门禁已从 warning 升级为硬失败。** 复核确认这不是纸面完成:`check-arch-deps` 153 条规则 0 warning 0 failed,`servercore` 预算门禁把生产行数/方法面/字段数五个维度全部下压到当前实测值,`check:test-names` 全仓通过且 allowlist 为 0。

**复核发现的 3 个 P0' 遗留问题已在本轮闭环:**

| 编号 | 问题 | 性质 |
| --- | --- | --- |
| P0'-1 | 测试归位与 servercore 测试行硬预算 | ✅ 已完成：17,582 / 18,000 行 |
| P0'-2 | Windows 契约审计路径分隔符 | ✅ 已完成：source/classification/adapter/test key 统一 POSIX |
| P0'-3 | 干净检出 preflight 缺生成物 | ✅ 已完成：preflight 首步生成 docs，ci-local 单次生成后检查漂移 |

**P1-4 已完成验证,原假设需要修正:** `indicatorruntime` 不是僵尸包,但其中 **6,632 行计算引擎在生产路径上不可达**(仅测试可达)。这仍是本清单投入产出比最高的一条,但动作从"删包"改为"拆包 + 删一半"。

---

## 0. 规模基线

| 维度 | 基线 `3482f994` | 当前 `6eb2e22b` | 变化 |
| --- | ---: | ---: | ---: |
| Go 生产 | 950 文件 / 280,238 行 | 1,017 文件 / 298,431 行 | +18,193 |
| Go 测试 | 838 文件 / 192,616 行 | 878 文件 / 197,538 行 | +4,922 |
| 前端 `apps/web/src` | 353 文件 / 127,742 行 | 383 文件 / 134,956 行 | +7,214 |
| 前端测试 | 291 文件 / 96,041 行 | 299 文件 / 100,816 行 | +4,775 |

扣除 `pkg/futu/pb` 生成代码与 `pkg/bbgo` fork(17,253 行)后,手写 Go 生产代码约 15.1 万行。

注意:P0 三轮改造期间生产代码净增 1.8 万行 —— 说明**重构与功能开发并行**,`servercore` 的行数下降来自搬迁而非净删除。P0'-1 已通过测试归位与硬预算收尾。

---

## P0 已闭环(归档) —— 复核实测

三条 P0 的原始分析基于 `3482f994`,不回写历史数据。下表是本次实测的闭环结果。

### P0-1 `servercore` God Package

| 指标 | 基线 | 首轮改造 | 上次自评 | **本次实测** | 门禁上限 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 生产文件 | 96 | — | — | **59** | — |
| 生产行数 | 20,668 | 7,718 | 6,622 | **6,559** | 6,560 |
| 显式 `*Server` 方法 | 200 | 72 | 63 | **46** | 46 |
| `serverApplication` 方法 | — | 107 | 73 | **73** | 73 |
| 有效方法面(含嵌入提升) | 200 | 179 | 136 | **119** | 119 |
| 聚合字段数 | 33 | 52 | 26 | **26** | 26 |
| 禁止的直接 import | 3 warning | 0 | 0 | **0(硬失败)** | 0 |

生产行数比基线降 68%,有效方法面降 41%。`scripts/servercore-budget.json` 把五个维度全部钉在当前实测值,回退即失败。`check-arch-deps.sh` 结果 **153 passed / 0 warnings / 0 failed**,`pkg/futu`、`pkg/adk`、`pkg/backtest` 三族的生产与测试 import 全部为硬失败,无测试逃逸口。

ADK 生产代码在 `servercore` 只剩 **53 行**(`adk_runtime.go` 24 行 + `adk_workflow.go` 29 行),纯装配入口。计算引擎与生命周期已进入 `internal/assistant/assembly`(4,562 行)。

servercore 测试现在受 `testLinesMax: 18000` 硬门禁约束；ADK、Pine、Futu、settings 测试已在 owner package 运行。

### P0-2 前端 API 类型双真相

| 事实 | 基线 | **本次实测** |
| --- | ---: | ---: |
| `src/generated/openapi.ts` | 7,603 行 | **12,403 行** |
| 引用生成物的文件数 | 7 | **39** |
| 绕过 `apiClient` 的裸 `fetch()` | 17 处 | **1 处**(即 `apiClient.ts` 自身) |
| `src/types/` | 12 行,形同虚设 | `client-api.ts` + `view-models/` 10 个域文件 |

wire DTO 只从 `@/contracts` 引入、view model 只从 `@/types` 引入的分层已落地。四个门禁 `check:web-api-boundary`、`check:web-contract-index`、`check:web-contract-audit`、`check:openapi-quality` 全部进入 `preflight`。

裸 `fetch` 从 17 处收敛到 1 处是这一条最实的安全收益 —— 鉴权头、CSRF、`WEB_AUTH_REQUIRED_EVENT` 不再有缺失路径。

Windows 路径回归测试覆盖 source key 与 test-file key；契约门禁已在本机 Windows 通过。

### P0-3 测试命名与断言密度

| 指标 | 基线 | **本次实测** |
| --- | ---: | ---: |
| 违反命名政策的测试文件 | 88 | **0** |
| `test-name-allowlist.txt` 豁免条目 | — | **0** |
| 无可识别断言的测试 | 未度量(旧正则永远退出 0) | **6**(5 legacy + 1 具名豁免) |

`check:test-names` 覆盖全仓(不再只查增量),基线从 `origin/main` tree 按当前规则推导,无法通过"同时新增文件和豁免条目"绕过。断言检查改为 Go AST 分析,识别 `testing.T` 失败调用、testify 与跨文件 helper;普通函数调用不再自动合格。

剩余 6 个缺口:
- 5 个 legacy(`internal/api/marketdata`、`internal/api/research`、`internal/app/apiserver/lifecycle`、`pkg/bbgo/types`、`pkg/strategy/pineworker`)
- 1 个具名豁免(`servercore/strategy_runtime_nil_boundaries_test.go`,"不 panic"边界契约,理由已登记)

---

## P0' —— P0 改造遗留已闭环（2026-07-28 实测）

### P0'-1 `servercore` 测试归位与预算

**闭环证据**

P0'-1 将 ADK 路由/SSE 测试迁到 `internal/api/assistant`，工具/策略/产品/workflow/maintenance 测试迁到 `internal/assistant/assembly`，Pine 路由与 worker 测试迁到 `internal/api/strategy`/`internal/strategy/pineruntime`，Futu runtime 测试迁到 `internal/app/apiserver/futuapp`，ADK settings 测试迁到 `internal/api/settings`。`adk_assembly_compat_test.go` 已删除，迁移后的测试直接构造 owner runtime、adapter、coordinator 和 ports。

servercore 测试行从复核时的 25,457 降到 **17,582**，并由 `testLinesMax: 18000` 硬失败门禁锁定:

| 阶段 | 生产行 | 测试行 | 比值 |
| --- | ---: | ---: | ---: |
| 基线 `3482f994` | 20,668 | 35,416 | 1.71 |
| **本轮闭环** | 6,559 | 17,582 | **2.68** |

owner package 测试均已通过，servercore 本身也通过 `go test ./internal/app/apiserver/servercore -count=1`。测试/生产比从 3.88 降到 2.68，且后续回退会由预算门禁阻断。

---

### P0'-2 Windows 契约审计路径规范化

**闭环证据**

`scripts/lib/web-contract-audit.mjs` 提供统一 `normalizeRelativePath()`，主审计脚本及分类表、source、adapter、boundary-test 比较全部先规范为 `/`。Node 回归测试同时覆盖 Windows 反斜杠 source key 和 test-file key；本机 `pnpm run check:web-contract-audit` 通过。

---

### P0'-3 独立 preflight 生成契约产物

**闭环证据**

`scripts/run-test-layer.mjs` 抽出共享 `preflightChecks`：`preflight = generate:docs + checks`；`ci-local = generate:docs + git diff + audit/license + checks + 后续门禁`，不会重复生成。`docs/testing-strategy.md` 已说明 preflight 会刷新工作树生成物，而 ci-local 仍用 `git diff` 拦截未提交契约漂移。本机执行 `pnpm run generate:docs` 成功，相关 Node 门禁和 Web contract audit 均通过。

---

## P1 —— 明显收益

### P1-4 `indicatorruntime`:原假设已验证并修正(**当前最高优先级**)

> 初版分析把这条列为"疑似 9,193 行僵尸代码,需先验证再动手"。**本轮已完成验证,结论需要修正。**

**验证方法与结果**

1. **外部 import 者:5 个非测试文件**(`internal/backtest/run.go`、`internal/strategy/liveruntime/pineworker_live.go`、`internal/api/strategy/routes.go`、`pkg/backtest/pineworker_runner.go`、`pkg/backtest/runner.go`)—— 全部在活跃调用链上,**不是僵尸包**。
2. **外部实际使用的符号只有 4 个**,全部是预热 K 线数量计算:
   - `RuntimeOptions`
   - `WarmupBarsFromScriptForSymbol`
   - `WarmupBarsFromScriptForSymbolWithOptions`
   - `WarmupBarsFromPlanForSymbolWithOptions`
3. **包共导出 10 个函数 + 2 个类型。** `IndicatorEngine` 与 `NewIndicatorEngineForPlan` / `NewIndicatorEngineForPlanWithOptions` **只被本包测试调用**(`indicator_runtime_state_test.go`、`warmup_script_test.go`),无任何生产调用者。
4. **`IndicatorEngine` 是计算机器的唯一入口。** 包内非测试文件中,只有 `indicator_engine.go` 自己引用 `IndicatorEngine`;`warmup.go` 与 `spec_parse_keys.go` 完全不触碰 `kdjSeries` / `macdSeries` / `snapshotSeriesCache` 等计算符号。

**结论:包可以按外部可达性一分为二**

| 分组 | 文件数 | 生产行数 | 生产可达性 |
| --- | ---: | ---: | --- |
| A:spec 解析 + requirements + 预热计算 | 12 | **2,561** | 活跃(4 个外部符号的实现) |
| B:指标计算引擎(`calc_*` / `state_*` / `snapshot_*` / `indicator_engine` / `indicator_runtime*` / `stoploss` / `trading_window_ma` / `series_limit` 等) | 46 | **6,632** | **仅测试可达** |
| 包测试 | — | 8,617 | 其中大部分服务于 B |

这与架构文档一致:`docs/README.md` 声明"Go 主进程不再维护自研 Pine 执行 runtime",PineTS 是唯一执行路径。B 组正是自研 runtime 的计算内核 —— 执行路径切走后,只有预热计算这一小块被留用。

**为什么不能直接删文件**

已实测:直接移除 B 组文件后 `go build ./...` 失败,因为 `indicatorRequirements`(A 组的 `spec_parse.go`、`spec_query.go`、`trading_period.go`、`mtf_validation.go` 都用)与 `snapshotSeriesCache`、`kdjSeries` 等类型定义交叉分布在两组文件里。包内耦合是拆分的真实障碍,不是可以靠删文件绕过的。

**建议做法**(顺序不可调换)

| 步 | 动作 | 验证 |
| --- | --- | --- |
| 1 | 新建 `pkg/strategy/indicatorwarmup`,迁入 A 组 12 个文件与其私有类型(`indicatorRequirements` 等) | `go build ./...` 通过 |
| 2 | 5 个外部调用点改指向新包(只有 4 个符号) | `go build ./...` + `go test ./internal/backtest ./pkg/backtest ./internal/strategy/liveruntime ./internal/api/strategy` |
| 3 | 确认 `indicatorruntime` 已无任何非测试 import,**整包删除**(6,632 行生产 + 约 8,000 行测试) | `rg -l 'strategy/indicatorruntime' --glob '!*_test.go'` 为空 |
| 4 | 跑全量回归与覆盖率门禁 | `pnpm run test:go`、`pnpm run test:coverage` |

**预期产出:一次删除约 6,600 行生产代码 + 约 8,000 行测试代码**,且预热逻辑获得一个名副其实的包名。这是本清单里投入产出比最高的一条。

**风险与前置确认**

- 必须确认 B 组是否仍服务于"实时指标计算"(非回测路径)。本轮验证显示 `internal/strategy/liveruntime/pineworker_live.go` 只用 `WarmupBarsFrom*`(A 组),**未发现实时路径使用计算引擎**,但建议在动手前再确认一次实盘指标面板的数据来源。
- 覆盖率门禁:删除后 `pkg/strategy` 系的覆盖率分母变小,需重跑 `test:coverage` 确认仍满足普通包 ≥85%。

**同时处理的小包**:`pkg/strategy/expression` 21 行、`pkg/chart` 25 行、`pkg/besteffort` 20 行 —— 这种规模不应独立成包,合并到使用方(`expression` 只被 `pkg/strategy/pine` 的 2 个文件引用)。

---

### P1-1 ADK/AI 助手子系统体量是核心交易业务的 1.7 倍(产品重心失衡,已缓解但仍显著)

**证据(本次实测)**

| 子系统 | 生产行数 | 测试行数 |
| --- | ---: | ---: |
| **ADK/助手**:`pkg/adk` 24,498 + `internal/assistant` 8,194 + `internal/api/assistant` 3,341 + `servercore/adk_*` 53 | **36,086** | **47,229** |
| **核心交易业务**:`internal/trading` 4,937 + `internal/api/trading` 1,270 + `pkg/broker` 2,922 + `internal/marketdata` 2,913 + `internal/strategy` 7,269 + `internal/backtest` 1,982 | **21,293** | — |

比例从基线 2.13:1 降到 **1.69:1**(生产行)—— `internal/strategy` 从 2,215 增长到 7,269 行,核心业务侧增长更快,失衡有所缓解。但 ADK 含测试合计约 83,000 行 Go,绝对量仍然显著,且测试量(47,229)已超过自身生产量的 1.3 倍。

**这是业务判断,不是工程缺陷**,需要明确回答:

- README 定位是「面向 Futu OpenD 的交易研发控制台」。ADK 占这么大工程投入,是刻意的战略选择还是逐步漂移?
- ADK 有完整的 workflow 编排、approval 流程、execution lease、goal state、child workflow、canvas override。这套复杂度对应的用户场景是什么?有多少是真实使用的?
- 若 ADK 是核心差异化 → 不该埋在 `pkg/adk`,应有顶层模块地位与独立演进节奏(见 P1-5)。
- 若 ADK 是辅助功能 → 36,086 行严重超配,应考虑收缩到「工具调用 + 只读查询」最小集。

**建议**:先用一个季度的真实使用数据(ADK session/run/approval 触发数 vs 回测运行数、下单数)回答上面的问题。埋点成本很低,但答案决定后续所有优先级。**这是本清单里唯一一条不该由工程师单独决定的事项**。

---

### P1-2 broker 抽象只有一个实现,且抽象已在业务层漏底

**证据(现状未变)**

- `pkg/broker` **2,922 行**,含 `Broker` 接口、`Registry`、`CapabilityCatalog`、`Descriptor`、`market_rules`、`research_contracts`。
- 实现仍只有一个:`pkg/futu/adapter.go` 的 `futuAdapter`。
- 抽象漏底点(需复核是否仍存在):`internal/system/service.go` 的 Futu 专名字段、`internal/backtest/data.go` 的 `bt.NewFutuKLineStore(...)` 直接调用、`internal/backtest/sync.go` 的「创建 Futu 连接」。

**为什么是问题**

教科书式 speculative generality:为「将来可能支持第二个 broker」付了 2,922 行抽象 + capability catalog + `docs/new-broker-integration-guide.md` 的成本,但抽象从未被第二个实现验证过。未被验证的抽象在真正接第二个 broker 时几乎必然要重写(它是照着 Futu 的形状长出来的),同时业务层已经在绕过它。

**建议做法**(二选一,不要维持现状)

- **若 12 个月内有明确的第二 broker**:立刻用一个最小的 mock/paper broker 实现把抽象跑通,让 `Registry` 至少有两个成员,同时清理三处漏底。
- **若没有**:把 `pkg/broker` 内移到 `internal/broker`,文档明确记为「单实现抽象,保留是为了 capability catalog 的 UI 驱动能力,不承诺 broker 中立」。诚实标注比维持虚假的中立承诺更有价值 —— 后者让每个新功能都付「要不要做成 broker-neutral」的决策税。

---

### P1-3 Pine 语法在前后端各实现一遍(差距已扩大)

**证据**

| 位置 | 基线 | **本次实测** |
| --- | ---: | ---: |
| 前端 Pine 相关文件合计 | 3,580 | **9,714** |
| 后端 `pkg/strategy/pine` 生产 | 9,311 | **9,311** |

前端 Pine 相关代码从 3,580 行增长到 9,714 行,现已与后端解析器规模相当。除 `strategyVisualBuilderPine.ts` / `strategyVisualBuilderPineParser.ts` 外,新增了 `pineSourceStructure*` 系列 7 个文件、`pineV6Workflow.ts`、`strategyPineEditorIntelliSense.ts`。

**为什么是问题**:语义漂移。前端解析器认为合法的写法后端可能拒绝;后端支持的语法前端可视化编辑器可能识别不了并静默丢弃用户代码。策略是用户的核心资产,**静默丢失是最坏的失效模式**。前端解析代码翻了 2.7 倍意味着这个风险面在扩大,而不是收敛。

**建议**

1. **先建共享语料库**(最小成本止血,不改架构):一批 Pine 源码 fixture,前后端解析器必须对它们产出一致的结构判定。仓库已有 `pkg/backtest` 的 `TestPinetsShadowCorpusReport` 影子语料机制,复用这套模式。
2. 中期方向:后端把解析结果(结构索引)作为 API 返回给前端,前端不再自己解析,只保留「visual model → Pine 文本」的单向生成。
3. 不建议:把后端解析器编译成 WASM 给前端。收益不抵复杂度。

---

### P1-5 `pkg/` 与 `internal/` 的划分标准已失效

`docs/architecture/backend-coding-standards.md` 规定「只有需要被其他 Go module 复用的稳定能力才放入 `pkg/*`」。实测 `pkg/` 下 13 个包,**没有一个有外部复用者**(本仓库是唯一 module):

| 包 | 生产行数 | 应归属 |
| --- | ---: | --- |
| `pkg/futu` | 130,241(含 `pb` 生成 114,008) | 保留(架构文档明确复用意图) |
| `pkg/strategy` | 26,590 | 保留;但见 P1-4(其中 9,193 行待拆解) |
| `pkg/adk` | 24,498 | `internal/assistant/`(见 P1-1) |
| `pkg/bbgo` | 17,253 | 上游 fork,**见下方供应链提示** |
| `pkg/backtest` | 7,944 | 保留 |
| `pkg/broker` | 2,922 | `internal/broker/`(见 P1-2) |
| `pkg/market` | 1,684 | 保留 |
| `pkg/researchscreen` | 1,627 | `internal/research/` |
| `pkg/observability` / `pkg/jftsettings` / `pkg/chart` / `pkg/besteffort` | 497 / 233 / 25 / 20 | `internal/`;后两个直接合并到使用方 |
| `pkg/jftradeapi` | 0 | **空包,应删除** |

**为什么值得管**:`pkg/` 隐含「公开 API,变更需谨慎」的契约。名不副实时,团队要么无谓地为内部代码维护向后兼容,要么发现规则是假的从而不再相信任何目录约定 —— 后者更糟,它侵蚀所有其他约定的可信度。

**供应链盲区(建议优先处理)**:`pkg/bbgo` 17,253 行上游 fork,被 **117 个非测试文件**引用,但**没有任何文档说明 fork 自哪个版本、改了什么、如何同步上游安全更新**,`pkg/bbgo/FORK.md` 不存在(已确认)。建议补一份记录基线 commit 与本地改动清单 —— 这是低成本高价值的一条,半天可完成。

---

### P1-6 `BacktestPage.vue` 4,597 行,其中 1,676 行是 scoped style

**证据(基本未变)**

| 部分 | 基线 | **本次实测** |
| --- | ---: | ---: |
| 文件总行 | 4,569 | **4,597** |
| `<script setup>` 区 | 2,006 | **2,035** |
| `<style>` 区 | 1,675 | **1,676** |

**1,676 行 scoped CSS 是比 2,035 行 script 更强的信号** —— 项目同时引入 Vuetify 4 与 Tailwind 4 两套样式体系,却仍需为单个页面手写 1,676 行 CSS。说明设计系统没建立,或两套体系在打架、开发者用 scoped CSS 逐个覆盖。

初版分析后又新增了 `feat: 优化回测工作台UI`、`feat: 优化全局拖动条样式` 两个 commit,但 style 区只减少 1 行 —— 印证样式层在持续付成本而没有结构性改善。

**建议**

1. **先解决样式体系重叠**(根因,优先于拆组件):明确 Vuetify 负责什么、Tailwind 负责什么、什么时候才允许写 scoped CSS。抽出共享 token(间距、圆角、层级、面板/工具栏样式)。这一步能消掉大部分重复 CSS 且惠及所有页面。
2. 再按业务区块拆组件:参数表单 / 运行控制 / 结果图表 / 交易明细 / 指标面板。
3. 2,035 行 script 里的数据获取与状态迁到 composable(已有 `useBacktestRuns.ts`,可扩展)。

**代价**:步 1 是设计决策 + 2-3 天;步 2-3 约一周。步 1 不做直接做步 2,只是把 1,676 行 CSS 分散到 5 个文件里。

---

### P1-7 错误处理不一致(**已恶化**)

**证据**

| 指标 | 基线 | **本次实测** | 变化 |
| --- | ---: | ---: | --- |
| `fmt.Errorf` 调用(生产) | 1,893 | **2,115** | +222 |
| 其中使用 `%w` 包装 | 541(28.6%) | **585(27.7%)** | 占比下降 |
| `strings.Contains(err...)` 字符串匹配错误 | 8 | **19** | **+11** |

**为什么是问题**:72% 的错误不可 unwrap,调用方无法用 `errors.Is` 判定错误类型,只能靠字符串 —— 那 19 处 `strings.Contains(err)` 就是这个缺陷的直接产物,且**从 8 处涨到 19 处,说明缺陷在扩散**。错误分类不可靠会直接影响 API 层错误码映射准确性,而 `docs/testing-strategy.md` 把「fail-closed 风控和权限拒绝」列为必须完整枚举的契约面。

**建议**

1. 先修那 19 处 `strings.Contains(err)` —— 数量可控、风险明确,为每处定义哨兵错误。**优先级比基线时更高**,因为增长趋势已确认。
2. 审计忽略 `Rollback`/`Exec` 返回值的位置,**只有事务回滚和写操作是真问题**(`Close` 的读路径忽略通常可接受)。
3. 新增代码要求 `%w`:用 `golangci-lint` 的 `errorlint` 增量启用(仅对改动代码)。存量约 1,530 处不建议批量改。

---

## P2 —— 长期整理

### P2-1 前端模块组织已到平铺极限(部分已改善)

| 目录 | 基线 | **本次实测** | 状态 |
| --- | ---: | ---: | --- |
| `components/` 根目录 `.vue` | 28 | **28** | 未改善,无归类 |
| `components/domain/` | 6 个空目录 | **6 个域,已填充** | ✅ 已改善 |
| `composables/` | 96 文件 / 23,395 行 | **106 文件 / 25,787 行** | 继续增长,仍全平铺 |
| `features/` | 41 文件 / 16,721 行 | **42 文件 / 16,743 行** | 基本持平 |
| `features/strategyVisualBuilder*` | 25 | **23** | 略降 |

`components/domain/` 从空壳变成了 6 个填充好的域(`account`、`market-data`、`runtime`、`shared`、`strategy`、`watchlist`)—— 初版指出的「未完成重构」已完成。但 `components/` 根目录仍有 28 个未归类 `.vue`,`composables/` 106 个文件全平铺。

**建议**:把 `components/` 根目录剩余 28 个按同样方式归入 `domain/`(路径已经建好,边际成本很低);`features/` 的 23 个 `strategyVisualBuilder*` 前缀文件重组为 `features/strategy-builder/`,并给每个域一个 `index.ts` 作为唯一对外出口(便于用 lint 规则禁止跨域深引用)。

### P2-2 状态管理策略需要显式化

无 Pinia。**106 个 composables 中仅 7 个文件使用 `useQuery`/`useMutation`**(基线为 31 处调用点),绝大部分跨组件状态靠 composable 里的模块级 `ref` 单例。这在当前规模下能跑,但:模块级 ref 在测试间会串状态(需手动 reset);没有 devtools 可观测;「谁拥有这份状态」只能靠读代码。

**建议**:写一份 `docs/frontend/state-management.md` 明确约定(什么状态用 vue-query、什么用模块单例、什么用 provide/inject),比引入 Pinia 更实际。项目已引入 `@tanstack/vue-query` 但只有 7 个文件在用,这个「引入了但没用起来」的状态本身需要一个决策:要么推广,要么明确它只服务特定场景。

### P2-3 测试执行成本

Go 测试 197,538 行 + 前端 100,816 行,其中 Go 测试有 **71 处 `time.Sleep`**(基线 69,略增)。`test:preflight` 串行执行 13 个步骤(含三套覆盖率)。

**建议**:审计 71 处 sleep,改为条件等待/channel 同步(sleep 是测试不稳定与慢速的双重来源);`preflight` 中无依赖的步骤并行化 —— 前 8 个检查(test-policy / test-names / test-quality / servercore-budget / 四个契约门禁)相互无依赖,可并行。

### P2-4 `scripts/` 90 个文件,`package.json` 89 个 npm script

| 指标 | 基线 | **本次实测** |
| --- | ---: | ---: |
| `scripts/` 文件数 | 50 | **90** |
| npm script 数 | 70+ | **89** |
| 其中 `test:desktop-*` | 12 | **9** |

构建/发布逻辑的复杂度已超过一个独立子项目。`test:desktop` 一条命令串起 11 个子步骤。

**建议**:合并同类项(desktop 发布相关的 9 个测试脚本可合成一个带子命令的入口),并为 `scripts/lib/` 建立最小文档。注意 `scripts/` 自身有 10 个 `*.test.mjs`(通过 `test:test-policy` 运行)—— 门禁工具有测试是好事,但也说明这块已经复杂到需要自己的测试套件了。

---

## 建议执行顺序(本轮更新)

P0 与 P0' 已闭环，后续按「先做高 ROI 删除 → 再处理架构与产品决策」推进。不要并行开工。

```
第 0 步  P1-4 拆 indicatorwarmup + 删计算引擎  —— 最高 ROI,一次删约 6,600 生产 + 8,000 测试
第 1 步  P1-5 补 pkg/bbgo/FORK.md              —— 半天,填供应链盲区
第 2 步  P1-7 步骤 1(19 处字符串匹配错误)    —— 1 天,趋势已确认在恶化
第 3 步  P1-6 步骤 1(样式体系决策)           —— 设计决策 + 2-3 天
第 4 步  P1-2 broker 抽象二选一决策
```

P1-1 的 ADK 使用数据采集独立于以上工程顺序，由产品与工程共同决定。

---

## 关于本清单的诚实边界

**本轮已验证并可复现:**

- 所有 P0/P0' 数值来自本轮工作树实测（基线 commit `6eb2e22b`）。
- P0 三条的闭环状态由实际门禁退出码确认:`check-arch-deps`(153/0/0)、`check-servercore-budget`(五维度全部贴线)、`check-test-names`(全仓通过,0 豁免)、`check-test-quality`(6 个缺口全部已登记)。
- P0'-2 的 Windows 回归已通过，source/classification/adapter/test key 均统一为 POSIX 分隔符。
- P0'-3 的 `generate:docs` 已作为 preflight 第一步，ci-local 单次生成后仍执行契约漂移检查。
- P1-4 的外部符号使用面、`IndicatorEngine` 的测试-only 可达性、A/B 分组行数,均经 import 分析与实际 `go build` 移除实验确认。

**未验证:**

- P1-4 的 B 组是否仍服务于实盘指标面板 —— 本轮未发现实时路径使用计算引擎,但建议动手前再确认一次实盘指标数据来源。
- P1-2 的三处「抽象漏底」是否仍在原位置 —— 沿用初版分析结论,未逐个复核行号。

**未覆盖:**

- 并发正确性(goroutine 泄漏、锁粒度)。注意近期有两个相关 commit(`b1187163` 追踪 goal-resume goroutine、`edc12dae` 补覆盖率),说明这块有真实问题被发现过,值得单独做一轮分析。
- SQLite 查询性能与索引;前端 bundle 体积与运行时性能。

**已知本地测试限制(Windows):**

本轮在 Windows 11 验证时发现 7 个包的测试因 symlink 权限失败(`internal/app/apiserver/datamigration` / `internal/store/exchangecalendar` / `internal/store/settingsfile` / `internal/store/trading` / `internal/trading` / `pkg/strategy/pineworker`)。已在 HEAD 复现,属预存 Windows 环境限制,非本轮改动回归。`pineworker` 另有时序竞态(`TestWorkerManagerReadinessFailuresCloseTransportAndRespectCancellation` 间歇失败 "unhealthy transport closes = 2, want 1")。上述失败导致完整 Go 覆盖率门禁无法在本地完成;已通过的其他门禁(`arch-deps` / `servercore-budget` / `lint` / `vet` / `test-policy` / 契约门禁 / 独立包测试)充分但需 Linux CI 最终验收。

**已确认良好、无需改动:**

- SQLite 连接层(`internal/store/sqliteconn/conn.go`):单写连接 + 独立只读池 + WAL + `synchronous(NORMAL)` + `foreign_keys(ON)` + `busy_timeout(10000)`,是本项目工程质量最高的部分之一。
- `check-arch-deps.sh` 已从 108 条规则扩展到 **153 条**,且 warning 全部升级为硬失败。初版分析的核心批评是「规范的强制力止步于 warning」—— **这一点已经被修正**,是 P0 改造最有长期价值的产出。
- `servercore-budget.json` 的五维度 ratchet 设计得当:同时约束行数、两个 receiver 的方法数、嵌入后有效方法面和聚合字段数,无法通过单一维度腾挪规避。
