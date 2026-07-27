# JFTrade 工程与业务优化分析清单

分析日期:2026-07-26 · 基线 commit `3482f994` · 所有数据为实测

---

## 0. 规模基线(先看清盘子有多大)

| 维度 | 生产 | 测试 | 测试/生产 |
| --- | ---: | ---: | ---: |
| Go 总计 | 950 文件 / 280,238 行 | 838 文件 / 192,616 行 | 0.69 |
| Go 扣除生成与 fork | ≈149,000 行 | 192,616 行 | **1.29** |
| 前端 `apps/web/src` | 353 文件 / 127,742 行 | 291 文件 / 96,041 行 | 0.75 |
| PineTS worker | 21 文件 / 5,547 行 | — | — |
| 构建脚本 `scripts/` | 50 个 / 3,903 行 | — | — |

扣除项:`pkg/futu/pb` 生成代码 114,008 行、`pkg/bbgo` fork 17,253 行。

**第一个信号:手写 Go 生产代码 14.9 万行,测试 19.3 万行,测试比生产多 30%。** 这不是"测试写得好"的表现,后面 P0-3 会给出原因。

---

## P0 —— 阻碍演进,建议优先处理

### P0 改造复核与证据闭环（2026-07-27）

本节以暂存改造、后续修正后的工作树和实际门禁为准。下面原始 P0-1～P0-3 保留为 `3482f994` 基线分析，不回写历史数据。

| 项目 | 基线 | 首轮暂存改造 | 本次闭环结果 | 状态 |
| --- | ---: | ---: | ---: | --- |
| `servercore` 生产行 | 20,668 | 7,718 | **6,622** | 规格内拆解闭环，余量继续受控 |
| 显式 `*Server` 方法 | 200 | 72 | **63** | 不能单独代表真实方法面 |
| `serverApplication` 方法 | 不适用 | 107 | **73** | 已下降并设 ratchet |
| `*Server` 有效方法面 | 200 | 179 | **136** | 较基线下降 32% |
| `Server` + application 字段 | 非同口径 33 | 52 | **26**（8 + 18） | store/runtime/lifecycle 均已聚合 |
| 禁止的生产/测试直接 import | 3 个 warning | 0 | **0，硬失败** | 完成 |

#### P0-1：六个规格步骤闭环，但不把“闭环”解释为无需继续治理

- 持久化资源由 `internal/app/apiserver/stores.Handle` 聚合。应用根只保留一个具名 store 字段，handle 内部维持成功打开顺序、失败回滚、降级 fallback 和幂等逆序关闭。
- 运行时资源由 `internal/app/apiserver/runtimes.Handle` 按启动根、可缺省运行时和可重置运行时分组；handle 在发布资源前登记关闭函数，拒绝并立即关闭 shutdown 后到达的资源。关闭顺序明确拆成 consumer、业务 service、provider 三段，Pine runner 切换和替换释放仍在同一同步边界内。`serverApplication` 不再平铺 14 个 runtime 字段、ownership flag、`sync.Once` 或 Pine mutex。
- ADK/MCP 生命周期与跨域工具投影均进入 `internal/assistant/assembly`，业务模型契约归 `internal/assistant`，具体 Store/Runtime/ToolRegistry 构造只由 `internal/assistant/testkit` 提供给跨包测试。生产文件 `servercore/adk_adapter.go` 和 assembly 的宽泛 ADK re-export 均已删除。
- Pine worker 与 live runtime 分别由 `internal/strategy/pineruntime`、`internal/strategy/liveruntime` 持有；`servercore` 对 `pkg/futu`、`pkg/adk`、`pkg/backtest` 根包及其全部子包的生产和测试直接 import 均为硬失败。
- Futu 的 broker 选择、generation reset、健康/安装/运行时投影进入 `internal/app/apiserver/futuapp.Coordinator`。协议转换和 OpenD 连接仍归 `internal/integration/futu`，跨边界测试通过 `internal/integration/futu/testkit` 使用语义 fixture，不再把 protobuf 暴露给 `servercore`。
- `futu_runtime.go` 已由 application coordinator 取代；`pineworker_runtime.go` 和 `adk_runtime.go` 的少量残留仅是装配/类型别名入口。
- 预算门禁同时约束生产行数、两个 receiver 的方法数、嵌入后有效方法面、聚合字段数和禁止 import 文件集合；当前预算直接下压到 `6,622 / 63 / 73 / 136 / 26`，不得通过调高阈值解决回退。

附件报告只统计显式 `*Server` receiver，忽略了嵌入 `serverApplication` 后提升的方法。首轮真实可达方法面是 `72 + 107 = 179`，本次为 `63 + 73 = 136`；因此仍不能声称“只剩 63 个方法”。P0-1 的 stores、runtimes、ADK、Pine、Futu 和硬门禁六个规格步骤已经闭环，但 `servercore` 仍有 6,622 行和 136 个有效方法，后续应在当前 ratchet 下持续下沉，而不是把 P0 闭环包装成 God package 已经消失。

`check-arch-deps` 同时检查 `servercore` 的生产 import、`TestImports` 和 `XTestImports`，并按包族匹配根包及后代；直接依赖 `pkg/futu/*`、`pkg/adk/*`、`pkg/backtest/*` 都会硬失败，不再保留测试文件逃逸口。当前结果是 `153 passed, 0 warnings, 0 failed`。

#### P0-2：生成契约、归一模型和 UI 模型重新分层

- `@/types` 不再转导 `@/contracts`；wire DTO 只从 `@/contracts` 引入，view model 只从 `@/types` 引入，来源在 import 处可见。
- 手写 `WebSession`、success/error envelope 改为由生成 schema 派生；API mapper 的 wire 输入补上生成类型依赖。
- `RequestBodyFor` 成为共享生成请求体类型，ADK SSE POST 不再手写 request body；原始协议请求仍复用统一认证、CSRF 与地址解析边界。
- raw SSE 错误会解析标准 error envelope，只有 `WEB_AUTH_REQUIRED`、`WEB_ACCESS_DISABLED`、`REMOTE_WEB_ACCESS_DISABLED` 触发登录事件；`CSRF_FAILED`、`ORIGIN_FORBIDDEN` 等 401/403 不再被误判为会话失效。
- `contractsModularization.test.ts` 进入独立 `tsconfig.type-tests.json`，根 `typecheck` 会实际编译类型断言，不再只由 Vitest 运行时收集。
- 契约审计当前覆盖 444 个 Swagger schema、8 个 generated alias module、206 个 normalized API declaration、6 个 UI declaration 和 2 个 client infrastructure declaration，并要求归一化 mapper 具有生成依赖与测试。

附件中的“236 降至 52”比较了不同目录/分类口径，不能作为完成率。P0-2 的完成依据应是 wire 来源唯一、mapper 边界可追踪、raw transport 安全语义一致、类型断言进入 CI，以及字段级审计通过；按这些条件，本轮核心双真相问题已经闭环。

#### P0-3：门禁现在反映断言，而不是“调用过代码”

- 任意大小写、PascalCase 或分隔形式的 `coverage` 文件名，以及 `c95`、`c_98` 等数字缩写，现在都由全仓门禁管理。检查器仍从 merge-base Git tree 按当前规则推导历史上限，不信任旧规则生成的清单，也无法通过同时新增文件和豁免条目绕过。
- 历史违规测试文件均已改为描述业务行为的名称，工作树违规项和 `scripts/test-name-allowlist.txt` 豁免项均为 0；P0 不再遗留测试命名盲区。
- 原正则报告把任意函数调用、`Skip`、`Sleep` 都当成可观察效果，且永远退出 0，不能称为断言密度门禁。现已改为 Go AST 分析：识别 `testing.T` 失败调用、testify 和跨文件 assertion helper，报告全仓并对新增缺口硬失败。
- 当前 AST 报告有 8 个未识别断言的测试：7 个属于 merge-base 历史，1 个“不 panic”边界测试有逐测试、带理由的显式例外；不存在新增未豁免缺口。普通函数调用不再自动合格，失效或理由不足的例外会失败。

测试总行数净增不能单独证明质量下降，文件名包含 `coverage` 也不能证明测试无价值。删测、重命名和拆包必须以业务断言、重复性与重构阻力为证据；本轮没有机械删除有效测试，也没有把 P1 的 `indicatorruntime` 审计扩进 P0。

#### 本轮验收命令

`check:servercore-budget`、`check:arch-deps`、四个 Web/OpenAPI 契约门禁、`check:test-names`、`check:test-quality`、`test:test-policy`、根 `typecheck`、Web 全量测试，以及 `application`/`runtimes`/`futuapp`/`stores`/`assistant`/`assistant/assembly`/`servercore` Go 测试均纳入验收；最终结论以这些命令和 `test:preflight` 的实际退出状态为准。

---

### P0-1 `servercore` 是事实上的 God Package,且是唯一的架构告警来源

**证据**

| 指标 | 数值 |
| --- | ---: |
| 生产文件 | 96 个 |
| 生产行数 | 20,668 |
| 测试行数 | 35,416 |
| 单包合计 | **56,084 行** |
| `Server` 结构体依赖字段 | 33 个(分 3 个嵌入 struct) |
| `func (s *Server)` 方法数 | **200 个** |

`internal/app/apiserver/servercore/server.go:64` 的 `Server` 嵌入了 `serverStores`(9 个 store)、`serverRuntimes`(14 个 runtime)、`serverFacades`(11 个 service),再加 12 个基础设施字段。`server_assembly.go` 的注释写得很清楚很专业,但注释解决不了「一个类型持有 33 个依赖、暴露 200 个方法」这个事实。

`bash scripts/check-arch-deps.sh` 结果:**108 passed, 3 warnings, 0 failed**,而 3 个 warning 全部指向同一个包:

```
⚠️ servercore still imports pkg/futu
⚠️ servercore still imports pkg/adk
⚠️ servercore still imports pkg/backtest
```

**为什么是问题**

1. 任何新功能都倾向于在这里加一个字段 + 一个方法,因为「所有依赖都在手边」。这是熵增的正反馈回路 —— 越大越吸引新代码。
2. 200 个方法意味着没有任何单元可以被独立测试。35,416 行测试里有大量是为了绕过装配复杂度而写的 setup 样板。
3. `Server` 是包内所有文件的共享可变状态,`pineWorkerMu`、`closeOnce` 这类同步原语和 `apiPort`、`desktopMode` 这类配置混在一个结构体里,关闭顺序和并发边界只能靠人肉记忆。
4. 三个 warning 是**已知但被降级为警告的债务** —— warning 不阻断 CI,所以它永远不会被还。

**建议做法**(按可独立交付的顺序)

| 步 | 动作 | 产出 |
| --- | --- | --- |
| 1 | 把 `serverStores` 提成独立包 `internal/app/apiserver/stores`,`OpenStores(paths) (*Stores, error)` + `Close()` | 消除 9 个字段,存储生命周期可独立测试 |
| 2 | 把 `serverRuntimes` 拆成 `runtimes` 包,按生命周期分组(启动即建 / 懒启动 / 可重置) | 消除 14 个字段,明确关闭顺序 |
| 3 | ADK 装配(`adk_*.go` + `mcp_server.go`,3,559 行)整体移到 `internal/assistant/assembly`,`servercore` 只保留一个 `assistant.Runtime` 接口 | 消除 `pkg/adk` warning |
| 4 | `pineworker_runtime.go`(788 行)+ `pineworker_live.go`(347 行)移到 `internal/strategy/pineruntime` | 消除 `pkg/backtest` warning |
| 5 | `futu_runtime.go`(481)+ `notify_futu.go`(361)+ `watchlist_futu.go`(488)移到 `internal/integration/futu` | 消除 `pkg/futu` warning |
| 6 | 三个 warning 清零后,把 `check-arch-deps.sh` 的 `warn_direct_import` 改成 `check_no_import`(硬失败) | **防止回退** —— 这步不做,前 5 步会慢慢被还原 |

**代价**:步 1-2 各约 1-2 天;步 3-5 各 2-4 天(主要成本在测试迁移);步 6 半小时。可以分 6 个 PR 独立合入。

**关键**:第 6 步是整个重构唯一有长期价值的部分。只做 1-5 不做 6,一年后会回到原点。

---

### P0-2 前端 API 类型存在双真相,OpenAPI 生成链路基本空转

**证据**

| 事实 | 数值 |
| --- | ---: |
| `src/generated/openapi.ts` 体积 | 7,603 行 |
| 引用它的文件数 | **7 个** |
| `src/contracts/index.ts` 手写 `interface`/`type` | **236 个** |
| 其中真正基于 `components[...]` 派生的 | **23 处** |
| 使用 `apiClient` 的文件 | 56 个 |
| 绕过 `apiClient` 直接 `fetch()` 的调用点 | **17 处** |

`apps/web/src/composables/apiClient.ts:1-66` 的类型体操写得相当漂亮 —— 从 `paths` 推导 `JsonRequestBody` 和 `ResponseDataFor`,理论上能做到端点级类型安全。但 `contracts/index.ts` 里 236 个手写 interface(`WatchlistGroup`、`ApiSuccessEnvelope` 等)才是组件实际消费的类型。

**为什么是问题**

后端改一个 DTO 字段 → `generate:contracts` 更新 `openapi.ts` → **前端不会有任何编译错误**,因为组件读的是手写的 `contracts/index.ts`。整条 `generate:openapi → generate:api-types → openapi-baseline.json` 的基础设施(以及 CI 里的契约漂移检查)保护的是「生成物与后端一致」,却没有保护「前端与生成物一致」。契约防线在最后一米断了。

这是 P0 不是 P1,因为:它让一套已经建好、且每个 PR 都在跑的昂贵基础设施产生虚假的安全感。团队会以为契约是被守住的。

**建议做法**

1. 先量化伤害:写一次性脚本,把 `contracts/index.ts` 的 236 个 interface 与 `openapi.ts` 的 `components["schemas"]` 做字段级 diff,产出「已漂移清单」。**这一步的输出决定后面投多少资源**,不要跳过。
2. 对每个能对上的类型,改成 `export type WatchlistGroup = components["schemas"]["WatchlistGroup"]`。对不上的(纯前端 view model,如 `ArchitectureCard`、`RoadmapPhase`、`ConsolePanel`)迁到 `src/types/`,与 API 契约物理分离 —— 目前 `src/types/` 只有 12 行,形同虚设。
3. 17 处裸 `fetch()` 收敛到 `apiClient`,否则鉴权头、CSRF、`WEB_AUTH_REQUIRED_EVENT` 在这些路径上是缺失的(**这条同时是安全问题**,建议先单独查一遍这 17 处是否涉及写操作)。
4. 加 CI 检查:`contracts/index.ts` 中直接声明 API DTO 形状的 interface 数量不得增长。

**代价**:步 1 半天;步 2 按漂移量,估 2-5 天;步 3 一天;步 4 半天。

---

### P0-3 88 个测试文件违反项目自己的命名政策,反映的是"为覆盖率写测试"

**证据**

`docs/testing-strategy.md` 明文规定:「新建或重命名的测试文件不得使用 `coverage_98`、`c95` 等覆盖率数字名称」。实测违反数:**88 个**。

| 包 | 违规文件数 | 测试/生产比 |
| --- | ---: | ---: |
| `pkg/adk` | 21 | 37,523 / 24,448 = **1.53** |
| `pkg/strategy/pine` | 17 | — |
| `pkg/strategy/indicatorruntime` | 12 | — |
| `internal/app/apiserver/servercore` | 11 | 35,416 / 20,668 = **1.71** |
| `pkg/backtest` | 6 | 12,345 / 7,942 = 1.55 |

文件名样本:`coverage_98_business_helpers_test.go`、`coverage_95_snapshot_window_test.go`、`runtime_symbol_coverage_98_test.go`、`request_object_c95_test.go`。

**为什么是问题 —— 这不是命名洁癖**

文件名是意图的诚实记录。`coverage_98_business_helpers_test.go` 这个名字承认了:**这个文件存在的目的是把覆盖率从 97% 推到 98%,不是为了验证某个业务行为**。`docs/testing-strategy.md` 开篇写「覆盖率是发现未验证行为的信号,不是业务正确性的替代品」—— 但 88 个文件名证明实践正好相反。

后果是复合的:

1. **测试成为重构的阻力而非保障。** 按行覆盖写的测试与实现细节强耦合。P0-1 拆 `servercore` 时,35,416 行测试里相当一部分会因为「内部函数没了」而失败,却不是因为行为变了。这直接抬高了 P0-1 的价格。
2. **门禁被规避而非满足。** `check:test-names` 只检查相对 base 的新增文件,88 个历史文件永久豁免。规则存在但不生效。
3. **真实的验证空白被掩盖。** 覆盖率 98% 且门禁全绿,但没人知道哪些业务分支是被真正断言的。

**建议做法**

1. **不要做无意义的批量改名。** 改名不改内容 = 把问题藏起来。
2. 从最高价值处切入:P0-1 要动 `servercore`,就在动之前先处理它的 11 个违规文件 —— 逐个判断,能对应到业务行为的重命名并补断言,纯行覆盖填充的**直接删除**。删测试听起来吓人,但一个只执行代码不断言行为的测试,其价值为负(它消耗 CI 时间并阻碍重构)。
3. 把 `check:test-names` 的范围从「新增文件」扩到「全仓」,用一份显式的 allowlist 冻结剩余历史文件,allowlist 只能减不能增。这样债务变成可见且单调递减的。
4. 覆盖率门禁增加「断言密度」维度:统计每个测试文件的 `t.Fatal`/`t.Error`/`want` 出现次数,对断言密度过低的文件告警。行覆盖 + 断言密度双指标比单一行覆盖诚实得多。

**代价**:步 2 与 P0-1 合并进行,增量成本低;步 3 半天;步 4 一天。

---

## P1 —— 明显收益

### P1-1 ADK/AI 助手子系统体量是核心交易业务的 2.1 倍(产品重心失衡)

**证据**

| 子系统 | 后端生产行数 |
| --- | ---: |
| **ADK/助手** (`pkg/adk` 24,448 + `internal/assistant` 3,530 + `internal/api/assistant` 2,244 + `servercore/adk_*` 3,559) | **33,781** |
| **核心交易业务** (`internal/trading` 4,890 + `api/trading` 956 + `pkg/broker` 2,915 + `marketdata` 2,913 + `strategy` 2,215 + `backtest` 1,959) | **15,848** |

比例 **2.13 : 1**。前端同样:ADK 相关文件 21,006 行,占 `src` 的 16.4%。加上测试,ADK 子系统合计约 61,000 行 Go —— 是整个项目手写 Go 的 **41%**。

**这是业务判断,不是工程缺陷**,但需要明确回答:

- README 定位是「面向 Futu OpenD 的**交易研发控制台**」。一个交易工作台把 41% 的工程投入放在 AI 助手上,是刻意的战略选择,还是逐步漂移的结果?
- ADK 有完整的 workflow 编排、approval 流程、execution lease、goal state、child workflow、canvas override(从 `pkg/adk` 的文件名可读出)。这套复杂度对应的用户场景是什么?有多少是真实使用的?
- 如果 ADK 是核心差异化 → 它不该埋在 `pkg/adk` 里,应该有自己的顶层模块地位和独立演进节奏。
- 如果 ADK 是辅助功能 → 33,781 行的实现规模严重超配,应考虑收缩到「工具调用 + 只读查询」的最小集。

**建议**:在动任何代码之前,先用一个季度的真实使用数据(ADK session 数、run 数、approval 触发数 vs 回测运行数、下单数)回答上面的问题。埋点成本很低,但这个答案决定了后续所有优先级。这是本清单里**唯一一条不该由工程师单独决定**的事项。

---

### P1-2 broker 抽象只有一个实现,且抽象已在业务层漏底

**证据**

- `pkg/broker` 2,915 行,含 `Broker` 接口、`Registry`、`CapabilityCatalog`、`Descriptor`、`market_rules`、`research_contracts` 等完整抽象。
- 实现只有一个:`pkg/futu/adapter.go:61` 的 `futuAdapter`。
- 抽象已漏底:
  - `internal/system/service.go:32-34` —— 三个 Futu 专名字段 `futuOpenDHealthFn`、`futuOpenDInstallGuideFn`、`resetFutuRuntimeFn`
  - `internal/backtest/data.go:180` —— `bt.NewFutuKLineStore(...)` 直接调用
  - `internal/backtest/sync.go:17` —— 注释即为「创建 Futu 连接」

**为什么是问题**

这是教科书式的 speculative generality:为「将来可能支持第二个 broker」付了 2,915 行抽象 + capability catalog + `docs/new-broker-integration-guide.md` 的成本,但抽象从未被第二个实现验证过 —— 而未被验证的抽象,在真正接第二个 broker 时几乎必然要重写(因为它是照着 Futu 的形状长出来的)。同时业务层已经在绕过它了。

**建议做法**(二选一,不要维持现状)

- **若路线图上 12 个月内有明确的第二 broker**:立刻用一个最小的 mock/paper broker 实现把抽象跑通,让 `Registry` 至少有两个成员。抽象不被第二个实现拉扯过,就不是抽象,只是一层间接。同时清理上面三处漏底。
- **若没有**:把 `pkg/broker` 内移到 `internal/broker`,在文档里明确记为「单实现抽象,保留是为了 capability catalog 的 UI 驱动能力,不承诺 broker 中立」。诚实标注比维持一个虚假的中立承诺更有价值 —— 后者会让每个新功能都付「要不要做成 broker-neutral」的决策税。

---

### P1-3 Pine 语法在前后端各实现一遍

**证据**

| 位置 | 行数 |
| --- | ---: |
| 前端 `strategyVisualBuilderPine.ts` + `strategyVisualBuilderPineParser.ts` | 3,580 |
| 后端 `pkg/strategy/pine` | 9,311 |

架构文档说「前端生成 Pine,后端统一解析」。但前端有 2,225 行的 `PineParser` —— 说明前端不只生成,也解析(为了从 Pine 源码反推 visual model)。同一套 Pine v6 语法有两个独立实现。

**为什么是问题**:语义漂移。前端解析器认为合法的写法,后端可能拒绝;后端支持的语法,前端可视化编辑器可能识别不了并静默丢弃用户代码。策略是用户的核心资产,静默丢失是最坏的失效模式。

**建议**:
1. 先建立一个**共享语料库** —— 一批 Pine 源码 fixture,前后端解析器都必须对它们产出一致的结构判定。这是最小成本的止血,不需要改架构。(注意仓库已有 `pkg/backtest` 的 `TestPinetsShadowCorpusReport` 影子语料机制,可复用这套模式)
2. 中期方向:后端把解析结果(结构索引)作为 API 返回给前端,前端不再自己解析。前端只保留「visual model → Pine 文本」的单向生成。
3. 不建议做的:把后端解析器编译成 WASM 给前端用。收益不抵复杂度。

---

### P1-4 `pkg/strategy/indicatorruntime` 疑似 9,193 行僵尸代码

**证据**

| 包 | 生产行数 | 被非测试文件引用数 |
| --- | ---: | ---: |
| `pkg/strategy/indicatorruntime` | **9,193** | **5** |
| `pkg/strategy/pineengine` | 437 | 4 |
| `pkg/strategy/indicatorbinding` | 432 | 6 |
| `pkg/strategy/expression` | **21** | 2 |
| `pkg/strategy/ir` | 1,929 | 25 |

`docs/README.md` 声明「Go 主进程不再维护自研 Pine 执行 runtime」,PineTS 是唯一执行路径。但 `indicatorruntime` 有 9,193 行生产代码 + 12 个 `c98`/`coverage_95` 测试文件,只被 5 个非测试文件引用。

**建议**:
1. 用 `go build` + 逐个删除法验证(或看这 5 个引用点是否在活跃调用链上)。如果确认是旧自研 runtime 的遗留,**删除**,这是本清单里投入产出比最高的一条 —— 一次删除约 9,000 行生产 + 数千行测试。
2. `pkg/strategy/expression` 21 行、`pkg/chart` 25 行、`pkg/besteffort` 20 行 —— 这种规模的包不应该独立存在,合并到使用方。
3. **注意**:删之前确认 `indicatorruntime` 是否仍服务于"实时指标计算"(非回测路径)。文档只说了 Pine 执行,没说指标运行时。这条必须先验证再动手。

---

### P1-5 `pkg/` 与 `internal/` 的划分标准已失效

`docs/architecture/backend-coding-standards.md` 规定「只有需要被其他 Go module 复用的稳定能力才放入 `pkg/*`」。实测 `pkg/` 下 12 个包,**没有一个有外部复用者**(本仓库是唯一 module):

| 包 | 生产行数 | 应归属 |
| --- | ---: | --- |
| `pkg/bbgo` | 17,253 | 上游 fork,应独立 vendor 目录或明确标注 fork 状态与同步策略 |
| `pkg/adk` | 24,448 | `internal/assistant/` |
| `pkg/broker` | 2,915 | `internal/broker/`(见 P1-2) |
| `pkg/researchscreen` | 1,627 | `internal/research/` |
| `pkg/observability` / `pkg/jftsettings` / `pkg/chart` / `pkg/besteffort` | 497/233/25/20 | `internal/` |
| `pkg/futu` / `pkg/strategy` / `pkg/backtest` / `pkg/market` | — | 保留(架构文档明确其复用意图) |

**为什么值得管**:`pkg/` 隐含「公开 API,变更需谨慎」的契约。当它名不副实时,团队要么无谓地为内部代码维护向后兼容,要么发现规则是假的从而不再相信任何目录约定 —— 后者更糟,因为它侵蚀所有其他约定的可信度。

**特别提示 `pkg/bbgo`**:17,253 行的上游 fork 被 164 个文件引用,但没有任何文档说明它 fork 自哪个版本、改了什么、如何同步上游安全更新。这是一个**供应链盲区**,建议优先补一份 `pkg/bbgo/FORK.md` 记录基线 commit 与本地改动清单。

---

### P1-6 `BacktestPage.vue` 4,569 行,其中 1,675 行是 scoped style

**证据**

| 部分 | 行数 |
| --- | ---: |
| `<script setup>` | 2,006 |
| `<template>`(多段合计) | ~700 |
| `<style>` | **1,675** |
| 顶层 `const`/`function` 声明 | 200 |

**1,675 行 scoped CSS 是比 2,006 行 script 更强的信号** —— 项目同时引入了 Vuetify 4 和 Tailwind 4 两套样式体系,却仍需为单个页面手写 1,675 行 CSS。说明:要么设计系统没建立,要么两套体系在打架,开发者用 scoped CSS 逐个覆盖。

`git log` 显示最近两个 commit 正是「优化回测工作台UI」「优化全局拖动条样式」—— 印证了样式层持续在付成本。

**建议**:
1. **先解决样式体系的重叠**(这是根因,优先于拆组件):明确 Vuetify 负责什么、Tailwind 负责什么、什么时候才允许写 scoped CSS。抽出共享的 token(间距、圆角、层级、面板/工具栏样式)。这一步能消掉大部分重复 CSS,且惠及所有页面。
2. 再按业务区块拆组件:参数表单 / 运行控制 / 结果图表 / 交易明细 / 指标面板。
3. 2,006 行 script 里的数据获取与状态迁到 composable(已有 `useBacktestRuns.ts` 930 行,可扩展)。

**代价**:步 1 是设计决策 + 2-3 天;步 2-3 约一周。步 1 不做直接做步 2,只是把 1,675 行 CSS 分散到 5 个文件里。

---

### P1-7 错误处理不一致

**证据**

| 指标 | 数值 |
| --- | ---: |
| `fmt.Errorf` 调用 | 1,893 |
| 其中使用 `%w` 包装 | 541(**28.6%**) |
| `errors.Is`/`errors.As` 调用 | 204 |
| `strings.Contains(err...)` 字符串匹配错误 | **8** |
| `_ =` 忽略 `Close`/`Write`/`Exec`/`Rollback` 返回值 | 54 |

**为什么是问题**:71% 的错误不可 unwrap,意味着调用方无法用 `errors.Is` 判定错误类型,只能靠字符串 —— 那 8 处 `strings.Contains(err)` 就是这个缺陷的直接产物。错误分类不可靠会直接影响 API 层的错误码映射准确性,而 `docs/testing-strategy.md` 把「fail-closed 风控和权限拒绝」列为必须完整枚举的契约面。

**建议**:
1. 先修那 8 处 `strings.Contains(err)` —— 数量少、风险明确,半天可完成。为每处定义哨兵错误。
2. 54 处忽略 `Rollback`/`Exec` 返回值中,**只有事务回滚和写操作是真问题**(`Close` 的读路径忽略通常可接受)。挑出写路径的逐个处理。
3. 新增代码要求 `%w`,可用 `golangci-lint` 的 `wrapcheck` 或 `errorlint` 增量启用(仅对改动代码)。存量 1,352 处不建议批量改。

---

## P2 —— 长期整理

### P2-1 前端模块组织已到平铺极限

| 目录 | 文件数 | 行数 | 问题 |
| --- | ---: | ---: | --- |
| `components/`(根目录) | **28 个 .vue** | — | 无归类,含 2,562 行的 `StrategyDesignStage.vue` |
| `components/domain/` | 6 | **0** | 空壳目录,说明有一次未完成的重构 |
| `composables/` | 96 | 23,395 | 全平铺,最大 `useADKPageChatState.ts` 1,645 行 |
| `features/` | 41 | 16,721 | 靠前缀分组:`strategy*` 25 个、`pine*` 8 个、`adk*` 6 个 |

`features/` 用 25 个 `strategyVisualBuilder*` 前缀文件模拟目录结构,是「该建目录了」的明确信号。建议按域重组为 `features/strategy-builder/`、`features/pine-structure/`、`features/adk-workflow/`,并给每个域一个 `index.ts` 作为唯一对外出口(便于用 lint 规则禁止跨域深引用)。`components/domain/` 空目录要么用起来要么删掉。

### P2-2 状态管理策略需要显式化

无 Pinia。96 个 composables 中仅 31 处 `useQuery`/`useMutation`,20 处 `provide`/`inject`。绝大部分跨组件状态靠 composable 里的模块级 `ref` 单例。这在当前规模下能跑,但:模块级 ref 在测试间会串状态(需要手动 reset);没有 devtools 可观测;「谁拥有这份状态」只能靠读代码。建议至少写一份 `docs/frontend/state-management.md` 明确约定(什么状态用 vue-query、什么用模块单例、什么用 provide/inject),比引入 Pinia 更实际。

### P2-3 测试执行成本

Go 测试 192,616 行 + 前端 96,041 行,其中 Go 测试有 **69 处 `time.Sleep`**。`test:preflight` 串行执行 7 个步骤(含三套覆盖率)。建议:审计 69 处 sleep,改为条件等待/channel 同步(sleep 是测试不稳定与慢速的双重来源);`preflight` 中无依赖的步骤并行化。

### P2-4 `scripts/` 50 个脚本 3,903 行

`package.json` 有 **70+ 个 npm script**,其中 12 个是 `test:desktop-*`。构建/发布逻辑的复杂度已接近一个独立子项目。建议合并同类项(desktop 发布相关的 8 个测试脚本可合成一个带子命令的入口),并为 `scripts/lib/` 建立最小文档。

---

## 建议执行顺序

不要并行开工。推荐路径:

```
第 1 步  P0-2 步骤 1(量化契约漂移)        —— 半天,产出决定后续投入
第 2 步  P1-4(验证并删除 indicatorruntime) —— 投入产出比最高,可能一次删 9k 行
第 3 步  P0-3 步骤 3(全仓 test-names + allowlist 冻结) —— 让债务可见且单调递减
第 4 步  P0-1 步骤 1-6(servercore 拆解)    —— 最大工程量,但第 3 步先做能降低阻力
第 5 步  P0-2 步骤 2-4(契约收口)
第 6 步  P1-6 步骤 1(样式体系决策)
```

**P1-1(ADK 重心)独立于以上所有步骤,且应最先启动数据采集** —— 它的答案会改变第 4 步中 ADK 部分的拆解方式(是提升为一等模块,还是收缩)。

---

## 关于本清单的诚实边界

- 所有数值均为实测,命令可复现。
- **未验证**:P1-4 中 `indicatorruntime` 是否真为僵尸代码 —— 我只统计了引用数,没有追调用链,动手前必须验证。
- **未覆盖**:并发正确性(goroutine 泄漏、锁粒度)、SQLite 查询性能与索引、前端 bundle 体积与运行时性能、安全面(那 17 处裸 fetch 只标注了风险未逐个检查)。这些需要单独的深入分析,不适合在概览层面下结论。
- **已确认良好、无需改动的部分**:SQLite 连接层(`internal/store/sqliteconn/conn.go:66-118`)配置得当 —— 单写连接 + 独立只读池 + WAL + `synchronous(NORMAL)` + `foreign_keys(ON)` + `busy_timeout(10000)`,是本项目工程质量最高的部分之一;`check-arch-deps.sh` 的 108 条规则设计精良;分层规范文档本身写得清晰可执行。**问题不在于缺少规范,而在于规范的强制力止步于 warning。**
