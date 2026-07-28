# JFTrade 工程改进计划

本轮复核: 2026-07-28 · HEAD `a2bdb66f` · 所有数值由本次实测得出

---

## 0. 规模基线（当前实测）

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

### P1-1 前端组件体量危机：23 个 `.vue` >800 行，scoped CSS 合计 18,290 行

**实测证据**

```
find apps/web/src -name '*.vue' -exec wc -l {} \; | sort -rn | awk '$1>800' | wc -l  → 23 个文件
```

Top 超量组件（template / script / style 分拆实测）：

| 文件 | template | script | style | 总行 |
| --- | ---: | ---: | ---: | ---: |
| `pages/BacktestPage.vue` | 553 | 2,035 | 1,676 | 4,597 |
| `components/StrategyDesignStage.vue` | 339 | 899 | 1,123 | 2,602 |
| `components/research/StockScreenerView.vue` | — | — | — | 2,414 |
| `components/adk-page/ADKChatComposer.vue` | 374 | 711 | 719 | 2,168 |
| `pages/ResearchPage.vue` | — | — | — | 1,357 |
| `components/product/PredictionResearchPanel.vue` | — | — | — | 1,324 |
| `components/workspace/OrderEntryPanel.vue` | — | — | — | 1,308 |
| （另 16 个超过 800 行）| | | | |

**为什么是系统性问题**

初版分析只聚焦 BacktestPage.vue 一个文件。本次实测发现 23 个 `.vue` 超标：

- **style 区占比普遍偏高**：`StrategyDesignStage.vue` style 1,123 行超过 script 899 行；`ADKChatComposer.vue` style 719 行几乎等于 script 711 行。这说明开发者在用 scoped CSS 逐个覆盖公共样式，而非单文件的设计问题。
- **设计 token 层极薄**：全局只有 `apps/web/src/styles/adk-tokens.css` 一个 token 文件，且命名为 `adk-tokens`（ADK 专属，不是全局 token）。Tailwind 配置不在 `apps/web/` 下（`tailwind.config.*` 不存在）。开发者无法从 token 层获得约束。
- **Tailwind + Vuetify 混用**：26 个文件同时出现 Vuetify 组件 (`v-btn`、`v-card`) 和 Tailwind class (`flex`、`grid`、`text-`)，两套样式体系的边界没有文档。

**直接拆组件是错误的操作顺序**：在没有 token 层之前把 BacktestPage.vue 拆成 5 个子组件，只是把 1,676 行 CSS 分散到 5 个文件，根因不变。

**建议步骤**

1. **先明确样式体系职责分工**（1 天设计决策）：Vuetify 负责什么、Tailwind 负责什么、何时允许写 scoped CSS。产出：`docs/frontend/styling-guide.md`。
2. **提取共享 token 到 `apps/web/src/styles/tokens.css`**（1-2 天）：间距、圆角、面板背景、工具栏高度、层级。这一步可消掉跨组件的重复 CSS，惠及所有超量文件。
3. **按业务区块拆组件**（每个大文件 1-2 天）：BacktestPage → 参数表单 / 运行控制 / 结果图表 / 交易明细；StrategyDesignStage → 已有 `strategyVisualBuilder*` 系列，对齐即可。
4. **数据与状态迁到 composable**：BacktestPage script 2,035 行中的数据获取逻辑迁到 composable（`useBacktestRuns.ts` 已有骨架）。

**代价**：步骤 1-2 是先决条件，跳过直接做步骤 3 会在 6 个月内回到原点。

---

### P1-2 错误字符串匹配：7 处需定义哨兵错误（前版本计数有误）

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

**建议**

- 4 处已有 sentinel 的直接改 `errors.Is`（机械修改，无风险）。
- 3 处需新增 sentinel error，定义在相应包的 `errors.go`。
- 存量 2,115 处 `fmt.Errorf`（72.3% 不含 `%w`）：不建议批量修改，只在 `golangci-lint` 的 `errorlint` 启用增量规则，仅对新增代码强制。

---

### P1-3 ADK 子系统战略定位（产品 + 工程共同决策）

**实测证据**

ADK 子系统生产行数 **36,033 行**，核心交易 **29,237 行**，比例 **1.23:1**（较上轮 1.69:1 已明显改善——核心交易侧增长更快，不是 ADK 在缩）。

ADK 测试行数（`pkg/adk` 37,633 + `internal/assistant` 8,762）= **46,395 行**，超过 ADK 自身生产行数 1.29 倍，说明测试覆盖较为充分。

**已确认安全边界**：

```bash
grep -rn 'PlaceOrder\|SubmitOrder\|CancelOrder' pkg/adk --include='*.go' → 无结果
```

ADK 工具集不直接触碰下单接口，交易动作通过策略 runtime 或明确 approval 流程流转。

**仍待回答的业务问题**

| 问题 | 影响 |
| --- | --- |
| ADK session/run/approval 触发数 vs 回测运行数、下单数 | 决定 ADK 是否超配 |
| `pkg/adk` 中的 workflow 编排、child workflow、canvas override、execution lease、goal state 等高复杂度特性，有多少在真实使用？ | 可能存在大量从未被触发的代码路径 |
| ADK 是核心差异化还是辅助功能？ | 决定是否收缩到「工具调用 + 只读查询」最小集 |

**两条可行路径**

- **ADK 是核心差异化**：当前规模合理，但 `pkg/adk` 应改为 `internal/assistant/engine`（无外部复用者，放 `pkg/` 是误导）。
- **ADK 是辅助功能**：36,033 行严重超配，应收缩到最小集，大量删除。

**建议先动作**：埋一周使用数据采点（ADK session 触发次数、approval 使用次数、workflow 触发次数），成本极低，但决定后续所有优先级。**这是本清单里唯一一条不该由工程师单独决定的事项**。

---

### P1-4 broker 抽象漏底：3 处确认，仍只有一个实现

**实测证据（3 处漏底均在原位置）**

```
internal/system/service.go:32-34  futuOpenDHealthFn / futuOpenDInstallGuideFn / resetFutuRuntimeFn（Futu 专名字段）
internal/backtest/data.go:180     bt.NewFutuKLineStore(...) 直接调用
internal/backtest/sync.go:17      注释「创建 Futu 连接」（架构语义泄漏）
```

`pkg/broker` 2,922 行，含 `Broker` 接口、`Registry`、`CapabilityCatalog`、`market_rules`，但实现仍只有 `pkg/futu/adapter.go` 一个。`docs/new-broker-integration-guide.md` 存在却无任何实现验证过这个抽象。

**为什么现在必须决策**：未验证的抽象在接第二个 broker 时几乎必然需要重写（它是照着 Futu 形状长出的）；同时业务层已在绕过抽象，每次修改 backtest 或 system 时都在付「要不要做成 broker-neutral」的决策税。

**二选一（不要维持现状）**

- **若 12 个月内有确定的第二 broker**：立刻用 mock/paper broker 把抽象跑通，同时修复 3 处漏底，让 `Registry` 至少有两个成员。
- **若没有**：把 `pkg/broker` 内移到 `internal/broker`，文档明确标注「单实现抽象，保留是为 CapabilityCatalog 的 UI 驱动能力，不承诺 broker 中立」。同时修复 3 处漏底，删除 `docs/new-broker-integration-guide.md` 或降级为草稿。

---

### P1-5 Pine 前后端双解析：前端 9,714 行 vs 后端 9,311 行，差距持续扩大

**实测证据**

```
前端 Pine 相关（实测 find apps/web/src -name '*pine*' -o -name '*Pine*'）：
strategyPineEditorIntelliSense.ts  2,344 行
strategyVisualBuilderPineParser.ts  2,225 行
strategyVisualBuilderPine.ts        1,355 行
pineV6Workflow.ts                    805 行
pineSourceStructureIndex.ts          638 行
（其余 9 个文件）                    347 行
合计：9,714 行

后端 pkg/strategy/pine（实测）：9,311 行
```

前端 Pine 代码从初版分析时的 3,580 行增长到 9,714 行，已与后端解析器规模相当。`strategyPineEditorIntelliSense.ts` 2,344 行是前端代码库第三大文件（仅次于 `openapi.ts` 12,403 行和 `BacktestPage.vue` 4,597 行）。

**核心风险**：语义漂移。前端解析器认为合法的写法后端可能拒绝；后端支持的语法前端可能静默丢弃。策略是用户核心资产，**静默丢失是最坏的失效模式**。

**建议**

1. **先建共享语料库**（最小成本止血）：一批 Pine 源码 fixture，前后端解析器必须对它们产出一致的结构判定。仓库已有 `pkg/backtest` 的影子语料机制，复用这套模式。
2. **中期方向**：后端把解析结果（结构索引）作为 API 返回给前端，前端不再自己解析，只保留「visual model → Pine 文本」的单向生成。这样前端 Parser 主体可以删掉。
3. **不建议**：把后端解析器编译成 WASM 给前端。收益不抵复杂度。

---

### P1-6 `pkg/` 命名空间失效：多个包应归 `internal/`，空包待删

**实测证据（按外部复用标准评估）**

| 包 | 生产行数 | 应归属 | 理由 |
| --- | ---: | --- | --- |
| `pkg/futu` | 130,241（含 pb 114,008）| 保留 | 架构文档明确复用意图；实现 bbgo `types.Exchange` |
| `pkg/strategy` | 26,590 | 保留（见 P0-1）| 多子系统共用，但见 indicatorruntime 拆解 |
| `pkg/adk` | 24,498 | → `internal/assistant/engine` | 无外部 module 复用者；JFTrade 专属逻辑（见 P1-3）|
| `pkg/bbgo` | 17,253 | 保留（fork 特殊处理）| 见 P0-2；需补 FORK.md |
| `pkg/backtest` | 7,944 | 保留 | 被多个子系统引用 |
| `pkg/broker` | 2,922 | → `internal/broker` | 见 P1-4；无外部复用者 |
| `pkg/market` | 1,684 | → `internal/market` | 无外部复用者 |
| `pkg/researchscreen` | 1,627 | → `internal/research` | 无外部复用者 |
| `pkg/observability` | 497 | → `internal/observability` | 无外部复用者 |
| `pkg/jftsettings` | 233 | → `internal/jftsettings` | 无外部复用者 |
| `pkg/chart` | 25 | 合并到使用方 | 规模不该独立成包 |
| `pkg/besteffort` | 20 | 合并到使用方 | 规模不该独立成包 |
| `pkg/jftradeapi` | 0 | **立即删除** | 空包 |

**为什么值得管**：`pkg/` 隐含「公开 API，变更需谨慎」的契约。名不副实时，要么无谓维护向后兼容，要么规则形同虚设——后者侵蚀所有其他目录约定的可信度。

**建议**：以迁移难度排序，先处理轻量的（`pkg/chart`、`pkg/besteffort` 合并，`pkg/jftradeapi` 删除），再处理中量的（`pkg/researchscreen`、`pkg/market`、`pkg/observability`），最后处理 `pkg/adk` 和 `pkg/broker`（需配合 P1-3 和 P1-4 决策）。

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

### P2-4 前端契约与类型层部分失效

**`@/contracts` 旁路（实测）**

```
grep -rl "generated/openapi" apps/web/src --include='*.ts' --include='*.vue' | grep -v contracts/ | grep -v apiClient → 28 个文件
grep -rl "@/contracts" apps/web/src --include='*.ts' --include='*.vue' → 39 个文件
```

39 个文件正确通过 `@/contracts` 引入 DTO；但另有 28 个文件（21 个 composable + 4 个组件 + 2 个 types 文件 + `apiClient.ts`）直接引用 `@/generated/openapi`，绕过契约层。现有门禁 `check:web-contract-audit` 不强制这一分层（它只检查 wire DTO 不出现在 view-model 层，不检查 import 路径）。

**类型重复（实测）**

`types/view-models/market-data.ts` 和 `types/view-models/market-profile.ts` 手写了与 openapi 生成类型形状一致的接口（`MarketDataCandleDto`、`MarketDataQuoteSnapshotDto`、`MarketProfileDto` 等），而不是使用 `Omit<components["schemas"]["..."]> & {...}` 模式扩展生成类型。`types/client-api.ts` 做法正确，可作为模版。

**建议**

- 把 28 处直接 openapi 引用加入 `check:web-contract-audit` 的检查范围（或新建 `check:web-contract-imports`），给出6个月的迁移窗口逐步修复。
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

## 建议执行顺序（本轮更新）

```
第 0 步  P0-2 补 pkg/bbgo/FORK.md                —— 半天，填供应链盲区（成本最低）
第 1 步  P0-1 拆 indicatorwarmup + 删计算引擎    —— 最高 ROI，一次删 ~6,600 生产 + ~8,000 测试行
第 2 步  P1-2 步骤 1：7 处字符串匹配改哨兵错误   —— 1 天，4 处机械修改 + 3 处新增 sentinel
第 3 步  P1-6 轻量清理：删空包 + 合并微包        —— 半天，pkg/jftradeapi + pkg/chart + pkg/besteffort
第 4 步  P1-4 broker 抽象二选一决策（+执行）
第 5 步  P1-1 前端组件体量：先做样式体系决策和 token 层，再拆组件
第 6 步  P1-3 ADK 使用数据采集 → 战略决策 → pkg/ 归属跟进
```

P1-5（Pine 双解析）和 P2 各条独立于以上顺序，可并行推进。P1-3 的 ADK 采点独立于所有工程工作，尽早开始。

---

## 关于本清单的诚实边界

**本轮已验证并可复现（HEAD `a2bdb66f`）**

- 所有规模数字来自 `find ... | xargs wc -l` 或 `grep -rc` 直接测量。
- `indicatorruntime` 的 5 个外部非测试导入者、4 个实际使用符号、A/B 分组，均经 `rg` import 分析确认。
- `strings.Contains(err)` 实测 8 处（前版本 19 处为误计，已核正）。
- broker 3 处漏底均在原位置确认（`service.go:32-34`、`data.go:180`、`sync.go:17`）。
- `pkg/bbgo/FORK.md` 不存在已确认。
- 前端 23 个 `.vue` >800 行、scoped CSS 合计 18,290 行，为本次新增发现（前版本仅列 BacktestPage 一个文件）。
- ADK 工具集无下单接口（`PlaceOrder`/`SubmitOrder`/`CancelOrder` grep 无结果）已确认。

**未验证**

- P0-1：B 组是否仍服务于实盘指标面板——本轮未发现实时路径调用计算引擎，但建议动手前再确认一次。
- P1-3：ADK 各高级特性（workflow 编排、execution lease、goal state、child workflow）的实际触发率——需埋点数据。
- P1-6：各 `pkg/` 包内移后的 import 路径冲突，需逐包验证 `go build`。

**未覆盖（建议单独专项）**

- **goroutine 生命周期**：实测生产代码有 58 处 goroutine 启动，无系统性泄漏审计。近期有 2 个 goroutine 相关修复 commit，说明这块有真实问题，值得专项分析。
- **SQLite 查询性能与索引**：`internal/store/sqliteconn` 连接层质量良好（WAL + 单写连接 + 只读池），但 query 层未审计慢查询或缺失索引。
- **前端 bundle 体积与运行时性能**：未检查 tree-shaking 效果、路由级 code-splitting 覆盖率、最大依赖体积。
- **Windows 环境已知限制**：7 个包因 symlink 权限测试失败（`datamigration`、`exchangecalendar`、`settingsfile`、`store/trading`、`internal/trading`、`pkg/strategy/pineworker`）；`pineworker` 有时序竞态间歇失败。这些是预存环境限制，非本轮改动回归，最终验收需 Linux CI。

**已确认良好、无需改动**

- `internal/store/sqliteconn/conn.go`：单写连接 + WAL + `synchronous(NORMAL)` + `foreign_keys(ON)` + `busy_timeout(10000)`，仍是仓库工程质量最高的部分之一。
- `check-arch-deps.sh` 153 条规则 0 warning 0 failed（P0 遗产），`servercore-budget.json` 五维度 ratchet 设计良好（防止单一维度腾挪规避）。
- 前端 API 边界：只有 `apiClient.ts` 做裸 `fetch()`（`refetch()` 为 vue-query 方法调用，非 window.fetch）；DTO 只从 `@/contracts` 引入的分层已落地。
- ADK 工具集安全边界：无下单接口，交易动作通过明确 approval 流程流转。
