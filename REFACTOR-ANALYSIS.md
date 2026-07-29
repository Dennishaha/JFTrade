# JFTrade 工程改进计划

本轮复核: 2026-07-29 · 治理前基线 HEAD `a2bdb66f`；P1 完成事实按当前工作区复测并记录在各治理项中

---

## 0. 治理前规模基线（HEAD `a2bdb66f` 实测）

| 维度 | 数值 | 备注 |
| --- | ---: | --- |
| Go 生产文件 | 1,017 | 含 `docs/swagger/docs.go`（30,553 行生成物）|
| Go 生产行数 | 320,794 | 同上；扣除 pb 生成（114,008）+ swagger 生成后手写约 17.6 万行 |
| Go 测试文件 | 881 | |
| Go 测试行数 | 195,809 | |
| 前端 `apps/web/src` | 383 文件 / 134,956 行 | |
| 前端测试 `apps/web/tests` | 299 文件 / 100,816 行 | |

**子系统对比（实测，不含测试）**

| 子系统 | 生产行数 |
| --- | ---: |
| ADK/助手：`pkg/adk` 24,498 + `internal/assistant` 8,194 + `internal/api/assistant` 3,341 | **36,033** |
| 核心交易：`internal/trading` 4,937 + `internal/api/trading` 1,270 + `internal/marketdata` 2,913 + `internal/strategy` 7,269 + `internal/backtest` 1,982 + `pkg/backtest` 7,944 + `pkg/broker` 2,922 | **29,237** |
| ADK : 核心交易 比例 | **1.23 : 1** |

> 比例较上一轮（1.69:1）已收窄——`internal/strategy` 从 2,215 行增长到 7,269 行，核心交易侧增速更快。

---

## P0 —— 架构完整性与最高 ROI 清理

### P0-1 `indicatorruntime` 清理（✅ 已完成）

**拆分前实测证据（HEAD `a2bdb66f`）**

```
find pkg/strategy/indicatorruntime -name '*.go' -not -name '*_test.go' | xargs wc -l → 9,193 行
find pkg/strategy/indicatorruntime -name '*_test.go' | xargs wc -l            → 8,617 行
```

外部非测试导入者（5 个文件）：
```
internal/strategy/liveruntime/pineworker_live.go
internal/backtest/run.go
internal/api/strategy/routes.go
pkg/backtest/pineworker_runner.go
pkg/backtest/runner.go
```

**这 5 个文件实际使用的外部符号只有 4 个**，全部是预热 K 线数量计算：
- `RuntimeOptions`
- `WarmupBarsFromScriptForSymbol`
- `WarmupBarsFromScriptForSymbolWithOptions`
- `WarmupBarsFromPlanForSymbolWithOptions`

**`IndicatorEngine`（计算引擎唯一入口）及其 46 个 `calc_*` / `state_*` / `snapshot_*` 文件，无任何生产调用者**，仅被本包自身测试引用。这与架构文档一致：「Go 主进程不再维护自研 Pine 执行 runtime」—— PineTS 是唯一执行路径，自研引擎早已切出，只是代码还在。

**完成结果**

| 项目 | 拆分前 | 完成后 | 净变化 |
| --- | ---: | ---: | ---: |
| 生产代码 | `indicatorruntime` 9,193 行 | `indicatorwarmup` 1,974 行（9 文件） | **-7,219 行** |
| 测试代码 | 8,617 行 | 1,237 行（8 文件） | **-7,380 行** |

最终迁移的是预热逻辑的精确最小闭包：requirements/config、严格需求解析与排序、固定周期校验、预热计算、`RuntimeOptions` 和 interval 分钟换算。原分析列入 A 组的 `trading_period.go`、`session.go`、`spec_keys.go`、`spec_query.go` 实际也只服务旧计算/快照引擎，已随旧包删除。

| 步 | 完成状态 | 验证 |
| --- | --- | --- |
| 1 | ✅ 新建 `pkg/strategy/indicatorwarmup`，迁入精确最小闭包及对应测试 | 包覆盖率 95.88% |
| 2 | ✅ 5 个外部调用点全部改指向新包 | 定向测试与 `go build ./...` 通过 |
| 3 | ✅ `pkg/strategy/indicatorruntime` 整包删除 | 非测试 import 为 0，hard-cut 审计防止回归 |
| 4 | ✅ 全量回归与覆盖率门禁 | `pnpm run test:go`、`pnpm run test:coverage` 通过 |

生产链核验结果也已明确：策略实盘信号由 PineTS 生成；普通 K 线技术指标由浏览器端 TypeScript 计算；两条生产路径均不使用 Go `IndicatorEngine`。

**同时处理的小包**：`pkg/strategy/expression` 已合并到唯一使用方 `pkg/strategy/pine`。原分析对另外两个包的“唯一使用方”假设不成立：`pkg/chart` 有 15 个生产导入文件，`pkg/besteffort` 有 68 个生产导入文件，均横跨多个业务域，继续作为共享包保留。

---

### P0-2 `pkg/bbgo/FORK.md` 可追溯性（✅ 已完成）

**治理前实测证据**

```bash
ls pkg/bbgo/FORK.md → DOES NOT EXIST
grep -rl "jftrade-main/pkg/bbgo" --include='*.go' . | grep -v '_test.go' | wc -l → 117 个非测试文件
```

`pkg/bbgo` 有 17,253 行 Go 代码（含 `pkg/bbgo/types` 13,387 行），被 117 个生产文件导入。**没有任何文档说明 fork 自哪个上游 commit、本地改了什么、如何跟踪上游安全更新**。

**为什么是 P0**：这是供应链盲区。上游如果发布了安全修复，本项目无法感知是否受影响；code review 也无法判断哪些改动是"我们加的"还是"上游本来就有的"。17,253 行代码对外表现为完全不透明的黑盒。

**完成结果**：已新建 `pkg/bbgo/FORK.md`，确认上游基线为 `c9s/bbgo v1.64.2`、commit `816670adaa14e95d61697d2c2a81975fd90fdff3`，并记录证据链、初始本地差异、后续 patch stack、月度及发版前安全检查、选择性移植和完整重新基线流程。`go test ./pkg/bbgo/... -count=1` 已通过。

---

## P1 —— 明显收益

### P1-1 前端组件体量治理（✅ 已完成）

**收口前后实测**

```
收口前：22 个源码行数超过 800 的 SFC；按 <style src> 有效体量计为 23 个冻结例外
收口后：0 个 effective lines 超过 800 的 SFC；0 个冻结例外；最大 effective lines = 800
```

23 个历史例外已全部按真实职责拆为业务子组件、typed controller、composable 或纯 TypeScript helper，而不是仅把 CSS 外移。代表性结果：

| 原组件 | 收口前 effective lines | 收口后 effective lines | 主要拆分边界 |
| --- | ---: | ---: | --- |
| `pages/BacktestPage.vue` | 3,611 | 488 | 配置、运行记录、报告区块与页面状态控制器 |
| `components/StrategyDesignStage.vue` | 2,602 | 654 | 画布、参数、预览、诊断与 Pine helper |
| `components/research/StockScreenerView.vue` | 2,414 | 231 | 工具栏、预设、条件构建、结果、对话框与 controller |
| `components/adk-page/ADKChatComposer.vue` | 2,168 | 210 | 输入、目标、队列、上下文、左右控制区与 composable |
| `pages/ResearchPage.vue` | 1,357 | 496 | 研究视图编排与 typed controller |
| `components/product/PredictionResearchPanel.vue` | 1,324 | 679 | 筛选、详情、交易上下文与 composable |
| `components/workspace/OrderEntryPanel.vue` | 1,308 | 450 | 下单状态机、风控确认、最大可交易量与反馈轮询 |
| `components/StrategyRuntimePanel.vue` | 1,178 | 796 | 布局、展示投影与刷新逻辑 |
| `components/product/OptionComboBuilder.vue` | 848 | 800 | 组合草稿与执行请求 helper；本地外置 scoped CSS 仍计入有效体量 |

**治理结果**

1. **样式职责固化**：`docs/frontend/styling-guide.md` 明确 Vuetify、Tailwind、全局 primitive 和 scoped CSS 的边界；`styles/tokens.css` 与 `styles/components.css` 提供统一 token 和 `jf-panel` primitives。
2. **组件职责收敛**：回测、策略设计、研究、ADK、交易、运行时、图表、期权和风险页面均保留编排职责；交互区块进入子组件，状态机与格式化逻辑进入 `.ts`/composable。
3. **公开行为保持**：原 props、emits、关键 DOM 结构和测试依赖的 setup state 名称保持；期权与预测研究同时迁出两处 OpenAPI 直引，统一通过 `@/contracts`。
4. **存量债务清零**：`check:web-component-budget` 继续把本地 `<style src>` 计入有效体量，并阻止新增 >800 行 SFC、例外或 scoped CSS 增长。预算从 23 个例外降为零，effective scoped CSS 从 18,383 行降至 18,100 行。

**完成验证**：组件门禁实测 211 个组件、0 个冻结例外、18,100 行 effective scoped CSS；`test:preflight` 全量通过，其中 Web 298 个测试文件 / 1,896 项测试全部通过（97.91% statements、89.32% branches），相对 `origin/main` 的逐文件覆盖门禁 0 failure，Go 业务覆盖率 97.01%、增量覆盖率 96.66%，worker 83 项测试、三套 typecheck 与 168 项架构依赖检查均通过。

---

### P1-2 错误身份治理：7 处已改为 sentinel + `errors.Is`

**实测证据**（HEAD `a2bdb66f`，仅生产代码）

```
grep -rn 'strings.Contains(err' --include='*.go' --exclude='*_test.go' . | grep -v REFACTOR
```

共 8 处，其中 1 处可豁免：

| 文件 | 位置 | 问题 |
| --- | --- | --- |
| `internal/pineworkerassets/assets.go:47` | `"file does not exist" / "no such file"` | ✅ 豁免：OS 跨平台文件错误，无法用 sentinel |
| `internal/api/assistant/catalog.go:57` | `"invalid task status"` | 需定义 sentinel error |
| `internal/api/assistant/catalog.go:238` | `"used by agent"` | 需定义 sentinel error |
| `pkg/adk/google_exec.go:276` | `adktool.ErrConfirmationRequired.Error()` | 已有 sentinel，直接改用 `errors.Is` |
| `pkg/adk/google_runner_resume.go:76` | 字符串 `"no function call event found..."` | 需定义 sentinel error |
| `pkg/adk/workflow_task.go:619` | `errUserGoalPauseRequested.Error()` | 已有 sentinel，直接改用 `errors.Is` |
| `pkg/adk/event_projection.go:370` | `adktool.ErrConfirmationRequired.Error()` | 已有 sentinel，直接改用 `errors.Is` |
| `pkg/adk/event_projection.go:375` | `adkworkflow.ErrNodeInterrupted.Error()` | 已有 sentinel，直接改用 `errors.Is` |

**修正说明**：前版本计数为 19，实测只有 8 处（之前的统计误包含了 `REFACTOR-ANALYSIS.md` 本身的引用行和测试文件）。但趋势风险仍然存在：6 处集中在 `pkg/adk`，说明 ADK 子系统的错误处理风格不一致。

**完成结果**

- 新增 `ErrInvalidTaskStatus`、`ErrProviderInUse` 和 GO-ADK 恢复事件缺失 sentinel；provider/task 产生端使用 `%w`，HTTP 状态映射使用 `errors.Is`。
- confirmation、workflow interruption 和 goal pause 在 GO-ADK FunctionResponse/持久化文本边界统一恢复 sentinel 身份，业务调用点不再各自匹配文本。集中恢复器保留原错误链和原始 `Error()` 文本。
- 文件系统例外先使用 `errors.Is(err, fs.ErrNotExist)`，只在跨平台系统文本不统一时保留带注释的兼容兜底。
- 新增 AST 回归测试防止 ADK 生产代码重新使用 `strings.Contains(...sentinel.Error())`；新增 `lint:go:errorlint`，以 diff base 对新代码强制错误包装/比较规则，已接入 preflight 和 GitHub CI。

当前精确的 `strings.Contains(err.Error(), ...)` 只剩 2 个受控边界：`internal/pineworkerassets` 的跨平台 OS 文本兜底，以及 `internal/assistant/engine/errors.go` 对已序列化 GO-ADK 错误的集中恢复。`pnpm run lint:go:errorlint` 已通过（0 issues）。

---

### P1-3 ADK 战略定位：已确认为核心差异化并建立 7 日使用观测

**实测证据**

内移后 ADK engine 生产代码为 24,605 行，`internal/assistant` 其余 service/assembly/workflow 生产代码为 8,346 行；对应测试为 37,788 + 8,762 = **46,550 行**。测试量继续高于核心实现，workflow、approval、lease 和恢复路径不是无回归保护的黑盒。

**已确认安全边界**：

```bash
grep -rn 'PlaceOrder\|SubmitOrder\|CancelOrder' internal/assistant/engine --include='*.go' → 无结果
```

ADK 工具集不直接触碰下单接口，交易动作通过策略 runtime 或明确 approval 流程流转。

**决策结果**

| 问题 | 本轮决策 |
| --- | --- |
| ADK 是核心差异化还是辅助功能？ | 定位为核心差异化，保留 workflow、child workflow、execution lease、goal state、approval 和工具幂等完整能力 |
| 复杂特性是否被真实使用？ | 不在无数据时删减；用滚动 7 日 session/run/approval/workflow 指标做发版复盘 |
| 实现是否是对外稳定 Go API？ | 不是；已硬切到 `internal/assistant/engine`，不保留 `pkg/adk` 兼容壳 |

**完成实现**：`/api/v1/adk/metrics` 新增 `runs.last7Days`、`approvals.last7Days`、session 总量/近 7 日、workflow definition/trigger 启用数、invocation 总量/近 7 日及 status/triggerType 分布，并返回明确的 `measurementWindow`。设置页同步展示“近 7 日 ADK 运行”和“近 7 日 Workflow”；OpenAPI、前端 mapper 和契约测试已同步。指标只聚合本地 SQLite 记录，不上传用户数据。

---

### P1-4 broker 抽象漏底：3 处已修复，单实现边界已明确

**治理前实测证据（3 处漏底均在原位置）**

```
internal/system/service.go:32-34  futuOpenDHealthFn / futuOpenDInstallGuideFn / resetFutuRuntimeFn（Futu 专名字段）
internal/backtest/data.go:180     bt.NewFutuKLineStore(...) 直接调用
internal/backtest/sync.go:17      注释「创建 Futu 连接」（架构语义泄漏）
```

`pkg/broker` 2,922 行，含 `Broker` 接口、`Registry`、`CapabilityCatalog`、`market_rules`，但实现仍只有 `pkg/futu/adapter.go` 一个。`docs/new-broker-integration-guide.md` 存在却无任何实现验证过这个抽象。

**决策结果**：未来 12 个月没有已承诺的第二 broker，因此不用 mock 伪造“已验证中立”。但进一步的生产 importer 审计发现 `pkg/broker` 被 77 个生产文件使用，其导出类型还被 `pkg/futu` 和 `pkg/researchscreen` 的公开 API 直接暴露；直接内移会让保留的公开包泄漏不可导入的 `internal` 类型。因此保留 `pkg/broker` 作为共享 adapter/capability DTO，但明确不承诺多 broker 中立。

**完成实现**

- `internal/system.Service` 的注入字段已改为 `brokerRuntimeHealthFn`、`brokerInstallGuideFn`、`resetBrokerRuntimeFn`，对应选项为 `WithBrokerRuntimeHealth`、`WithBrokerInstallGuide`、`WithResetBrokerRuntime`；Futu 专用 HTTP 路由和响应名保持不变。
- `internal/backtest` 不再直接构造 `NewFutuKLineStore`；K 线覆盖检查通过窄 callback 注入，具体 SQLite/Futu 命名 store 收口到 `internal/store/backtest`。
- `internal/backtest/sync.go` 的 Futu 专名注释已改为 broker-neutral 语义。
- `docs/new-broker-integration-guide.md` 已降级为单实现草案，明确当前只有 Futu/OpenD，第二 adapter 接入时必须重新验证抽象。

---

### P1-5 Pine 前后端双解析：共享结构语料与双端门禁已落地

**实测证据**

```
前端 Pine 相关（实测 find apps/web/src -name '*pine*' -o -name '*Pine*'）：
strategyPineEditorIntelliSense.ts  2,344 行
strategyVisualBuilderPineParser.ts  2,225 行
strategyVisualBuilderPine.ts        1,355 行
pineV6Workflow.ts                    805 行
pineSourceStructureIndex.ts          638 行
（其余 10 个文件）                 2,347 行
合计：9,714 行

后端 pkg/strategy/pine 生产 Go（实测）：9,329 行
```

前端 Pine 代码从初版分析时的 3,580 行增长到 9,714 行，已与后端解析器规模相当。`strategyPineEditorIntelliSense.ts` 2,344 行是前端代码库第三大文件（仅次于 `openapi.ts` 12,403 行和 `BacktestPage.vue` 4,597 行）。

**核心风险**：语义漂移。前端解析器认为合法的写法后端可能拒绝；后端支持的语法前端可能静默丢弃。策略是用户核心资产，**静默丢失是最坏的失效模式**。

**完成结果**

1. 新增 `tests/fixtures/pine-structure-corpus.json`，同一批 Pine 源码由 Go `pkg/strategy/pine` 和前端 `strategyVisualBuilderPineParser` 直接消费；检验后从 3 个简单样例扩充为 8 个业务场景。
2. Go 侧从 lowering 后 IR 递归核对 `let/if/order/exit/cancel/log/notify`、分支首节点、订单/退出字段和风险 metadata；前端同时核对 18 类 visual block、具体语义签名、控制边与嵌套深度，并执行 `source → visual → Pine → visual` 往返。语料覆盖参数、状态重赋值、MTF、派生/集合序列、指标条件、时间/交易时段、多订单、部分平仓、撤单、风险元数据、四类退出、通知和日志；退出 ID 与 `from_entry` 也会在往返中保留。
3. 新增防退化元断言，关键 statement kind、visual block 或业务维度被删除都会失败；`pnpm run test:pine-structure-corpus` 已接入 preflight，并运行全部共享语料与元断言。
4. 边界已写入 `docs/frontend/strategy-authoring.md`：Go 是完整语法/语义/lowering 权威，前端只是可视化往返子集，不将它扩张为第二套完整 AST。

后端直接返回结构索引仍是可选的长期简化方向，但它不是本次 P1 止血门禁的验收前置；WASM 方案继续不采用。

---

### P1-6 `pkg/` 命名空间：硬切完成，保留包已按公开契约重新审计

原评估把“仓库外复用者”作为唯一判据，忽略了保留公开包之间的导出类型契约。本轮按生产 importer 和导出签名重新审计，结果如下：

| 分类 | 完成决策 | 依据 |
| --- | --- | --- |
| 稳定执行能力 | 保留 `pkg/futu`、`pkg/backtest`、`pkg/strategy` | sidecar、回测、策略 runtime 交叉使用；`pkg/futu` 实现 bbgo exchange 契约 |
| 上游 fork | 保留 `pkg/bbgo` | 通过 `FORK.md` 管理基线、patch stack 和安全更新 |
| 共享公开类型 | 保留 `pkg/broker`、`pkg/market`、`pkg/researchscreen`、`pkg/observability` | 它们被 `pkg/futu`、`pkg/backtest` 或 `pkg/strategy` 直接导入/暴露；单独内移会破坏公开签名 |
| 窄 helper | 保留 `pkg/chart`、`pkg/besteffort` | 生产调用面分别是 15 和 68 个文件，不存在“唯一使用方”可供合并 |
| 仓库私有实现 | `pkg/adk` → `internal/assistant/engine`；`pkg/jftsettings` → `internal/jftsettings` | JFTrade 专属，无外部 module 契约；全仓 import 同一次硬切，不留 alias/转发壳 |
| 旧空门面 | `pkg/jftradeapi` 保持删除 | HTTP 装配、transport 和 service 已有明确 `internal` 归属 |

**治理闭环**

- 新增 `docs/architecture/public-package-policy.md`，明确新增、保留、内移和破坏性变更规则。
- `scripts/check-arch-deps.sh` 新增硬切门禁：旧目录必须不存在，旧 Go import 必须为零；经审计保留的 10 个顶层 `pkg/*` 形成精确 allowlist，新增包或陈旧条目都会失败。
- `internal/assistant/engine` 的持久化依赖只允许 `internal/store/sqliteconn` 和 `sqliteschema`；其他 assistant 包仍禁止依赖具体 store，该 allowlist 由架构门禁强制。
- 旧 `pkg/adk`、`pkg/jftsettings` 目录不存在，仓内旧 import 为 0；`check:arch-deps` 实测 168 项通过、0 warning、0 failed。

---

## P2 —— 长期整理

### P2-1 前端状态管理未显式化

无 Pinia。106 个 composable 文件全部平铺于 `composables/` 根目录，其中实测**只有 3 个文件使用 `useQuery`/`useMutation`**（前版本计 7 处，差异可能因计数单位不同，但绝对值仍然偏低）。绝大部分跨组件状态靠 composable 里的模块级 `ref` 单例实现。

**存在的问题**：模块级 ref 在测试间会串状态（需手动 reset）；无 devtools 可观测；「谁拥有这份状态」只能靠读代码。

**建议**：写一份 `docs/frontend/state-management.md` 明确约定（什么状态用 vue-query、什么用模块单例、什么用 provide/inject），而不是引入 Pinia。已引入 `@tanstack/vue-query` 但使用极少——需要一个决策：要么系统性推广，要么明确它只服务特定场景（避免「引入了又不用」的认知负担）。

---

### P2-2 前端目录组织已到平铺极限

| 目录 | 文件数 | 状态 |
| --- | ---: | --- |
| `components/` 根 `.vue` | 28 | 未归类，`domain/` 已有 6 个域目录但根目录未清理 |
| `composables/` 根 | 106 | 全平铺，无子目录 |
| `features/strategyVisualBuilder*` | 23 个文件 | 功能相关但未收进子目录 |

**建议**

- `components/` 根目录 28 个文件按已有的 `domain/` 六分法归入（路径已建好，边际成本低）。
- `features/strategyVisualBuilder*` 和 `features/pineSourceStructure*` 分别收入 `features/strategy-builder/` 和 `features/pine-structure/`，给每个域加 `index.ts`（便于 lint 规则禁止跨域深引用）。
- `composables/` 106 个文件按关注点分组到子目录（market-data、strategy、backtest、adk、settings 等），不必一次完成，随新增文件渐进迁移。

---

### P2-3 测试执行稳定性与成本

```
time.Sleep in tests  → 70 处（实测）
legacy 无有效断言测试 → 5 处（已登记，见旧版 P0-3 遗留）
```

70 处 `time.Sleep` 是测试不稳定与执行慢的双重来源。`test:preflight` 串行执行 13 个步骤（含三套覆盖率），其中前 8 个检查相互无依赖。

**建议**

1. 审计 70 处 sleep，优先处理时长 >50ms 的，改为 channel 同步或条件等待。
2. `preflight` 中 `test-policy`、`test-names`、`test-quality`、`servercore-budget`、四个契约门禁相互无依赖，可并行执行（Node `Promise.all`）。
3. 5 处 legacy 无断言测试：每处补一个有意义的断言，或明确注释说明「不 panic」是合法的验证目标（参考已有豁免格式）。

---

### P2-4 前端契约与类型层：直引已冻结，历史债务待迁移

**`@/contracts` 旁路（复核后实测）**

```
origin/main：直引 42 个文件；扣除 contract/client/test 基础设施后，生产消费者 30 个
当前工作区：直引 37 个文件；基础设施仍为 12 个，生产消费者降至 25 个
rg -l "@/contracts" apps/web/src --glob '*.ts' --glob '*.vue' → 41 个文件
```

附件所称“28 → 30”不是相同口径的 Git 对比；相对 `origin/main`，P1 改造前后均为 42 个总直引、30 个生产消费者，并没有净新增。本次仍主动把 ADK、回测、client envelope、两个 mapper 测试、期权组合与预测研究迁到 `@/contracts`，当前生产债务降至 25 个。

新增 `check:web-openapi-imports`：`contracts/generated/*`、`apiClient.ts` 和契约等价性测试是明确基础设施，其余 25 个历史消费者进入 shrink-only allowlist。新直引、allowlist 相对 merge-base 增长或已迁移后的 stale 条目都会失败；门禁已接入 test-policy、preflight、ci-local 和 GitHub CI。

**类型重复（实测）**

`types/view-models/market-data.ts` 和 `types/view-models/market-profile.ts` 手写了与 openapi 生成类型形状一致的接口（`MarketDataCandleDto`、`MarketDataQuoteSnapshotDto`、`MarketProfileDto` 等），而不是使用 `Omit<components["schemas"]["..."]> & {...}` 模式扩展生成类型。`types/client-api.ts` 做法正确，可作为模版。

**剩余建议**

- 在门禁保护下继续把 27 处历史直引逐域迁入 `@/contracts`，每次迁移同步删除 allowlist 条目。
- 重写 `types/view-models/market-data.ts` 和 `market-profile.ts`，改用生成类型的 Omit/Pick 扩展模式，消除 DTO 字段级重复。

---

### P2-5 前端 bundle 风险（未量化，需专项核查）

**实测信号**

| 依赖 | 风险 | 状态 |
| --- | --- | --- |
| `monaco-editor ^0.56.0` | 最大 bundle 成本，~2MB+压缩后 | `MonacoCodeEditor.vue` (991行) 作为普通组件导入，是否在路由分割之外被 eager import 未确认 |
| `mermaid ^11.16.0` | ~2MB 未压缩 | 未找到懒加载 import，可能直接进入主 chunk |
| `acorn ^8.17.0` | JS parser，属于 **运行时依赖**（非 devDependency）| 可能用于 Pine 表达式解析；生产包中携带 JS 解析器值得确认必要性 |

路由级 code splitting 已全部就位（12 个路由均为 `() => import(...)` 动态引入），但 monaco 和 mermaid 若被任何 eager import 的路径引用，仍会进入主 chunk。

**建议**：运行一次 `pnpm --filter @jftrade/web build --report`（或 `rollup-plugin-visualizer`），确认各 chunk 的体积分布，再决定是否需要手动 lazy-import monaco/mermaid。

---

### P2-6 `scripts/` 复杂度已超出可维护阈值

```
scripts/ 文件数：79（含 22 个 .test.mjs——门禁工具有了自己的测试套件）
package.json scripts：89 条
CI ci.yml 步骤：212 条（以 `- name:`/`uses:`/`run:` 计）
```

**建议**

- 合并同类项：desktop 发布相关的多条测试脚本合成一个带子命令的入口（参数式调用，减少 npm script 数量）。
- `scripts/lib/` 下的公共函数缺乏文档；`lib/*.mjs` 文件数已达需要一份 `scripts/lib/README.md` 的规模。
- 22 个 `.test.mjs` 说明 scripts 自身已有独立演进生命周期，考虑给 `scripts/` 建一个独立的 CI 检查目标（目前通过 `test:test-policy` 运行，但名称不直观）。

---

## P0/P1 完成记录

```
✅ P0-2  pkg/bbgo/FORK.md 供应链可追溯
✅ P0-1  indicatorwarmup 拆分并删除旧 Go 指标执行引擎
✅ P1-1  样式体系 + token/primitive + 23 个历史例外全部职责拆分 + 组件预算清零
✅ P1-2  7 处错误身份改为 sentinel/errors.Is + 增量 errorlint
✅ P1-3  ADK 核心差异化定位 + 滚动 7 日使用观测 + internal 硬切
✅ P1-4  broker 3 处漏底修复 + 单实现边界文档化
✅ P1-5  Pine 前后端共享结构语料与 preflight 门禁
✅ P1-6  pkg/adk、pkg/jftsettings 内移 + 保留包重审 + namespace 硬切门禁
```

除修复附件指出的 P2-4 OpenAPI 直引退步风险、加入收缩型门禁外，其余 P2 项保持独立，本次未扩大处理。

---

## 关于本清单的诚实边界

**本轮已验证并可复现（P0 基线 + P1 工作区）**

- 所有规模数字来自 `find ... | xargs wc -l` 或 `grep -rc` 直接测量。
- `indicatorruntime` 的 5 个外部非测试导入者、4 个实际使用符号、A/B 分组，均经 `rg` import 分析确认。
- `strings.Contains(err)` 实测 8 处（前版本 19 处为误计，已核正）。
- broker 3 处漏底均在原位置确认（`service.go:32-34`、`data.go:180`、`sync.go:17`）。
- `pkg/bbgo/FORK.md` 治理前不存在、当前已补齐并通过 bbgo 包回归。
- 前端组件预算实测 211 个 SFC，所有 effective lines 均不超过 800；冻结例外为 0，本地 `<style src>` 一并计入后的 effective scoped CSS 为 18,100 行。
- ADK 工具集无下单接口（`PlaceOrder`/`SubmitOrder`/`CancelOrder` grep 无结果）已确认。
- ADK 运行指标、broker-neutral 注入、8 场景 Pine 共享语料、旧 package import 归零、顶层公开包集合、OpenAPI 直引和前端体量均有自动化测试/结构门禁。

**待持续观测（不阻塞 P1 收口）**

- ADK 高级特性的真实采用率需要随后续本地 7 日窗口累积；当前已有可量化数据面，不再是无采点状态。
- 前端体量存量债务已清零；后续不得新增 >800 effective lines 的 SFC、预算例外或提高 scoped CSS 基线。

**未覆盖（建议单独专项）**

- **goroutine 生命周期**：实测生产代码有 58 处 goroutine 启动，无系统性泄漏审计。近期有 2 个 goroutine 相关修复 commit，说明这块有真实问题，值得专项分析。
- **SQLite 查询性能与索引**：`internal/store/sqliteconn` 连接层质量良好（WAL + 单写连接 + 只读池），但 query 层未审计慢查询或缺失索引。
- **前端 bundle 体积与运行时性能**：未检查 tree-shaking 效果、路由级 code-splitting 覆盖率、最大依赖体积。
- **Windows 环境已知限制**：7 个包因 symlink 权限测试失败（`datamigration`、`exchangecalendar`、`settingsfile`、`store/trading`、`internal/trading`、`pkg/strategy/pineworker`）；`pineworker` 有时序竞态间歇失败。这些是预存环境限制，非本轮改动回归，最终验收需 Linux CI。

**已确认良好、无需改动**

- `internal/store/sqliteconn/conn.go`：单写连接 + WAL + `synchronous(NORMAL)` + `foreign_keys(ON)` + `busy_timeout(10000)`，仍是仓库工程质量最高的部分之一。
- `check-arch-deps.sh` 168 项检查 0 warning 0 failed，`servercore-budget.json` 五维度 ratchet 设计良好（防止单一维度腾挪规避）。
- 前端 API 边界：只有 `apiClient.ts` 做裸 `fetch()`（`refetch()` 为 vue-query 方法调用，非 window.fetch）；OpenAPI 直引生产债务已从 30 降至 25 并由 shrink-only allowlist 冻结，新增代码必须经 `@/contracts`。
- ADK 工具集安全边界：无下单接口，交易动作通过明确 approval 流程流转。
