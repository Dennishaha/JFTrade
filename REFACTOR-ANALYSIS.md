# JFTrade 工程改进计划

本轮复核: 2026-07-29 · 治理前基线 HEAD `a2bdb66f`；P0/P1 已提交至 `9330a0a0`，P2 完成事实按当前工作区复测并记录在各治理项中

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

### P2-1 前端状态管理显式化（✅ 已完成）

**P2 治理前实测**：无 Pinia；112 个 composable 文件全部平铺于 `composables/` 根目录（更早基线 106，P1 组件拆分后净增 6 个），其中 7 个文件使用 `useQuery`/`useMutation`/`useInfiniteQuery`。状态 owner 和模块 singleton 的准入规则尚未显式记录。

**完成决策**：不引入 Pinia。`docs/frontend/state-management.md` 已明确六类 owner：后端资源归 Vue Query，页面交互归 feature composable，组件树协作归 typed provide/inject，可分享导航状态归 router，跨路由纯客户端协调才允许受控 singleton，单组件临时状态保持局部。

**完成实现**

- 明确 Vue Query 只管理服务器状态，统一 query key、mutation invalidation、SSE/WS cache 更新和轮询边界；不把它扩张为所有 composable 的替代品。
- 当前真正的模块级共享状态收口为 `brokerProviderSelection` 与 `marketProfiles` 两个受控 singleton；前者已有测试 reset，后者新增 `resetMarketProfilesForTests` 和 generation guard，reset 后旧异步请求不能回写状态。
- 文档明确 singleton 必须有 owner、reset/dispose、陈旧请求保护和 readonly/action 边界；页面 composable 在函数内建状态，不因少传一层 prop 升级为全局状态。
- Query 测试、singleton reset、typed context 隔离和 timer/并发测试约定已形成可审查规则。

**验证**：market profile reset/旧请求隔离测试、Web typecheck 及前端全量测试通过。

---

### P2-2 前端目录按 owner 收口（✅ 已完成）

| 治理前目录 | 文件数 | 状态 |
| --- | ---: | --- |
| `components/` 根 `.vue` | 28 | 未归类，`domain/` 已有 6 个域目录但根目录未清理 |
| `composables/` 根 | 112 | 全平铺，无子目录（治理前基线 106） |
| `features/strategyVisualBuilder*` | 22 个文件 | 功能相关但未收进子目录 |
| `features/pineSourceStructure*` | 7 个文件 | 同上，与视觉构建器并列平铺 |

**完成结果**

| 目录 | 完成后 | 约束 |
| --- | ---: | --- |
| `components/` 根 `.vue` | **0** | 归入 auth、app-shell、settings、backtest、shared、strategy-design、strategy-runtime 及既有 domain 目录 |
| `composables/` 根 `.ts` | **0** | 112 个实现归入 11 个 owner 域，另有 11 个域级 `index.ts` |
| `features/strategy-builder/` | 23 个文件 | 域外只允许从 `index.ts` 导入 |
| `features/pine-structure/` | 7 个文件 | 域外只允许从 `index.ts` 导入 |

- 全仓 import 与测试路径同批迁移，不保留旧路径转发壳；旧 feature 路径与两个 feature 的域外深引均为 0。
- `check:web-component-budget` 已扩展为组件体量 + 模块布局门禁：阻止根组件、根 composable、旧平铺 feature 和跨域深引回归，并要求两个 feature 入口始终存在。
- composable 使用显式 `@/composables/<domain>/<module>` 路径，使 owner 与依赖对象可见；域级 index 记录审查后的公共目录，不建立根级兼容出口。

**验证**：Web typecheck 通过；共享 Pine/策略 feature 18 个测试文件、128 项测试通过；组件迁移定向测试通过；模块布局门禁实测 211 个组件、0 例外。

---

### P2-3 测试执行稳定性与成本（✅ 已完成）

```
time.Sleep in tests  → 63 处（P2 治理前实测；更早基线 70，P1 期间减少 7 处）
legacy 无有效断言测试 → 未发现明确登记的 5 处（原有 grep 模式未匹配到，建议重新核查）
```

**时长分布（P2 治理前复核）**

| 时长 | 出现次数 | 备注 |
| --- | ---: | --- |
| 10ms | 29 | 大多是 ADK engine 并发等待，时长极短 |
| 20ms | 11 | ADK engine input/store 测试 |
| 5ms / 2ms / 25ms | 6 | 辅助等待 |
| 50ms | 3 | 边界 |
| **100ms** | **2** | `internal/assistant/engine/input_continuation_failure_recovery_test.go:192`；`pkg/strategy/pineworker/process_smoke_test.go:151`——两处均属进程启动/恢复路径，可评估换 channel 等待 |
| **2s** | **1** | `pkg/futu/opend/client_test.go:222`——OpenD 客户端重连等待，是真实网络协议行为，建议豁免 |

`test:preflight` 治理前串行执行 **17 个步骤**（run-test-layer.mjs 中 preflightChecks 数组实测）。前 10 个静态检查（test-policy / test-names / test-quality / servercore-budget / openapi-quality / web-api-boundary / web-contract-index / web-contract-audit / web-openapi-imports / web-component-budget）相互无依赖，并行化节省空间最大。

**完成结果**

1. 两处目标 100ms sleep 已清零。ADK 恢复测试同步验证 foreign lease 与本地 continuation claim；Pine worker 在 `Start` 健康检查后直接执行带 10 秒 deadline 的请求，不再轮询猜测进程就绪。非 bbgo fork 的 `time.Sleep` 从 63 降至 61；OpenD 2s 真实重连行为与 bbgo fork 测试保持不动。
2. preflight 前 10 个静态检查改为并行 stage，输出按声明顺序缓冲展示，汇总所有失败与 stderr；后续 lint/vet/coverage/typecheck/arch-deps 保持串行。测试验证任一并行失败都会阻止后续阶段，且不会丢失其他失败。
3. 原登记的 5 个无有效断言测试已逐一核查：3 个无业务价值空桩删除，Connectivity 补真实状态/channel 断言，剩余 2 个合法 effect-only 契约以具体原因登记豁免。质量门禁当前 0 legacy、2 个有效豁免。

**验证**：ADK lease 恢复测试连续 10 次通过；Pine worker mock 进程 smoke、相关 Go 包与 scripts policy 通过；并行调度/失败传播测试通过。

---

### P2-4 前端契约与类型层（✅ 已完成）

**`@/contracts` 旁路（P2 治理前实测）**

```
origin/main：直引 42 个文件；扣除 contract/client/test 基础设施后，生产消费者 30 个
P1 收口工作区：直引 37 个文件；基础设施仍为 12 个，生产消费者降至 25 个
rg -l "@/contracts" apps/web/src --glob '*.ts' --glob '*.vue' → 41 个文件
```

**完成结果**

- 25 个历史生产消费者及 1 个相对路径旁路全部改为从 `@/contracts` 获取具体领域 alias；legacy allowlist 已清空。
- `contracts/generated` 扩展为 broker、market-data、observability、research、settings、strategy、system、trading、watchlist 等窄 alias 模块；不向业务代码暴露完整 `components["schemas"]`。
- `apiClient` 是唯一保留的 `paths` 基础设施边界；checker 已同步其新路径。门禁实测 **0 legacy consumer、16 个受控基础设施文件**，新增直引或 stale allowlist 都会失败。
- `types/view-models/market-data.ts` 与 `market-profile.ts` 已用生成类型的 `Omit`/领域 alias 扩展，只重写 nullability、精确业务对象等真实 view-model 差异，不再逐字段复制 wire DTO。
- contract classification 中 15 个 adapter 路径已同步 P2-2 新目录；OpenAPI 字段等价、contract index 和 normalized adapter 审计继续有效。

**验证**：OpenAPI checker、checker 5 项单测、Web 与 contract typecheck、19 个契约测试文件 / 114 项测试、447 个 Swagger schema 字段审计均通过。

---

### P2-5 前端 bundle 量化与预算（✅ 已完成）

**当前实测状态**

| 依赖 | 原评估风险 | 当前实测状态 |
| --- | --- | --- |
| `monaco-editor ^0.56.0` | 可能 eager import 进入主 chunk | ✅ 已确认安全：`MonacoCodeEditor.vue` 顶层全为 `import type`（不产生运行时依赖）；`monaco-editor` 模块由 `await import("monaco-editor")` 动态加载；组件本身随策略设计路由 chunk 懒加载，不进入主 chunk |
| `mermaid ^11.16.0` | 可能直接进入主 chunk | ✅ 已确认安全：`useADKWorkspacePresentation.ts` 使用 `import("mermaid")` 动态加载，仅在 ADK 工作区首次渲染时触发 |
| `acorn ^8.17.0` | 运行时 JS parser 依赖 | ✅ 已从 `@jftrade/web` 直接依赖移除；源码直接 import 为 0。lockfile 中同版本仅由 `vue-router -> mlly` 间接使用 |

路由级 code splitting：12 个路由均为 `() => import(...)` 动态引入 ✓。

**完成结果**

- 新增 `pnpm run build:web:report` / `check:web-bundle`，按 gzip level 9 量化 `index.html` 初始依赖图、初始 CSS、最大异步 JS 与控制台全部 JS；`ci-local` 复用既有 release asset 构建，不重复 build。
- P2 基线：首屏 JS 403.8 KiB gzip、首屏 CSS 115.0 KiB、最大异步 JS 1,448.8 KiB、全部控制台 JS 5,030.6 KiB；预算保留约 8–11% headroom 并阻止无解释增长。
- Monaco `editor.api`/worker、Mermaid core 和 Cytoscape 被显式禁止进入初始图；当前最大异步文件为 Monaco TypeScript worker，保持按需加载。
- `docs/frontend/bundle-budget.md` 记录命令、基线、预算升级规则和依赖结论；报告脚本有合成资产单测并已纳入 scripts 完整性测试。

**验证**：当前 release asset 报告通过全部预算和重依赖初始图检查。

---

### P2-6 `scripts/` 测试入口与维护边界（✅ 已完成）

**P2 治理前实测（相较更早基线）**

```
scripts/ 文件数：87（治理前 79，净增 8 个；含 25 个 .test.mjs，治理前 22）
package.json scripts：93 条（治理前 89，净增 4 条）
CI ci.yml 步骤：214 条（治理前 212，净增 2 条；以 `- name:`/`uses:`/`run:` 计）
scripts/lib/ 文件数：13（5 个 .mjs + 3 对 .test.mjs + 其他），仍无 README
```

**完成结果**

- 新增参数式统一入口 `pnpm run test:scripts [-- <suite>]`，当前提供 all、policy、desktop、api-release、pinets-release、pineworker-assets、pineworker-dev、pine-benchmark、web-bundle 九组命名 suite。
- desktop suite 聚合原 8 个 release metadata/artifact/input、Linux、Wails、签名与 dev desktop 测试；删除 7 个 granular package aliases，并同步 ci-local、GitHub CI 与 desktop-release workflow。加入 bundle 报告命令后根 package scripts 仍由 93 降至 **89**。
- 注册完整性测试扫描所有 `scripts/**/*.test.mjs`，任何新增但未进入 `all`/命名 suite 的脚本测试都会失败；当前根 `scripts/` 有 89 个文件、27 个 `.test.mjs`，测试增长不再意味着 CI 静默漏跑。
- 新增 `scripts/lib/README.md`，记录 13 个 `.mjs` 模块的职责、稳定 exports、调用方和测试约定；公共 helper 修改不再只靠文件名猜测影响面。
- `test:test-policy` 通过 `test:scripts -- policy` 表达脚本自身门禁，desktop 与 release workflow 使用同一个受测入口。

**验证**：`test:scripts` 全量 84 项、desktop 8 项、test-policy 67 项及统一入口的 fail-closed/去重/完整注册测试均通过。

---

## P0/P1/P2 完成记录

```
✅ P0-2  pkg/bbgo/FORK.md 供应链可追溯
✅ P0-1  indicatorwarmup 拆分并删除旧 Go 指标执行引擎
✅ P1-1  样式体系 + token/primitive + 23 个历史例外全部职责拆分 + 组件预算清零
✅ P1-2  7 处错误身份改为 sentinel/errors.Is + 增量 errorlint
✅ P1-3  ADK 核心差异化定位 + 滚动 7 日使用观测 + internal 硬切
✅ P1-4  broker 3 处漏底修复 + 单实现边界文档化
✅ P1-5  Pine 前后端共享结构语料与 preflight 门禁
✅ P1-6  pkg/adk、pkg/jftsettings 内移 + 保留包重审 + namespace 硬切门禁
✅ P2-1  状态 owner 规则 + Vue Query/singleton/context 边界
✅ P2-2  components/composables/features 按域归档 + 模块布局门禁
✅ P2-3  固定等待清理 + preflight 静态检查并行 + 无断言测试清零
✅ P2-4  OpenAPI 历史直引清零 + view-model 基于生成类型扩展
✅ P2-5  bundle gzip 基线 + 重依赖懒加载预算 + acorn 直接依赖移除
✅ P2-6  scripts 统一测试入口 + 完整注册门禁 + lib 维护文档
```

---

## 关于本清单的诚实边界

**本轮已验证并可复现（P0 基线 + P1 提交 + P2 工作区）**

- 所有规模数字来自 `find ... | xargs wc -l` 或 `grep -rc` 直接测量。
- `indicatorruntime` 的 5 个外部非测试导入者、4 个实际使用符号、A/B 分组，均经 `rg` import 分析确认。
- `strings.Contains(err)` 实测 8 处（前版本 19 处为误计，已核正）。
- broker 3 处漏底均在原位置确认（`service.go:32-34`、`data.go:180`、`sync.go:17`）。
- `pkg/bbgo/FORK.md` 治理前不存在、当前已补齐并通过 bbgo 包回归。
- 前端组件预算实测 211 个 SFC，所有 effective lines 均不超过 800；冻结例外为 0，本地 `<style src>` 一并计入后的 effective scoped CSS 为 18,100 行。
- ADK 工具集无下单接口（`PlaceOrder`/`SubmitOrder`/`CancelOrder` grep 无结果）已确认。
- ADK 运行指标、broker-neutral 注入、8 场景 Pine 共享语料、旧 package import 归零、顶层公开包集合、OpenAPI 直引、前端体量和目录布局均有自动化测试/结构门禁。
- P2 的 OpenAPI 历史消费者为 0；根 Vue/composable 文件为 0；bundle 四项 gzip 指标、重依赖初始图和 scripts 测试注册均有可复现门禁。

**待持续观测（不阻塞 P2 收口）**

- ADK 高级特性的真实采用率需要随后续本地 7 日窗口累积；当前已有可量化数据面，不再是无采点状态。
- 前端体量存量债务已清零；后续不得新增 >800 effective lines 的 SFC、预算例外或提高 scoped CSS 基线。

**未覆盖（建议单独专项）**

- **goroutine 生命周期**：实测生产代码有 58 处 goroutine 启动，无系统性泄漏审计。近期有 2 个 goroutine 相关修复 commit，说明这块有真实问题，值得专项分析。
- **SQLite 查询性能与索引**：`internal/store/sqliteconn` 连接层质量良好（WAL + 单写连接 + 只读池），但 query 层未审计慢查询或缺失索引。
- **前端运行时性能**：bundle 与路由 code splitting 已建立静态基线，但尚未用真实桌面会话量化首屏解析、长任务、内存和交互延迟。
- **Windows 环境已知限制**：7 个包因 symlink 权限测试失败（`datamigration`、`exchangecalendar`、`settingsfile`、`store/trading`、`internal/trading`、`pkg/strategy/pineworker`）；`pineworker` 有时序竞态间歇失败。这些是预存环境限制，非本轮改动回归，最终验收需 Linux CI。

**已确认良好、无需改动**

- `internal/store/sqliteconn/conn.go`：单写连接 + WAL + `synchronous(NORMAL)` + `foreign_keys(ON)` + `busy_timeout(10000)`，仍是仓库工程质量最高的部分之一。
- `check-arch-deps.sh` 168 项检查 0 warning 0 failed，`servercore-budget.json` 五维度 ratchet 设计良好（防止单一维度腾挪规避）。
- 前端 API 边界：只有 `composables/shared/apiClient.ts` 做裸 `fetch()`（`refetch()` 为 vue-query 方法调用，非 window.fetch）；OpenAPI 历史生产直引债务已从 30 清零，新增代码必须经 `@/contracts`。
- ADK 工具集安全边界：无下单接口，交易动作通过明确 approval 流程流转。
