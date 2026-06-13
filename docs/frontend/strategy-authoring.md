# 前端策略设计专题

本文只回答三件事：

- Pine v6 策略定义和 Logic Flow `visualModel` 分别落在哪一层
- Logic Flow、Monaco 和模板生成各自负责什么
- 后续改模板、图块、同步行为时，应该先从哪个文件进入

## 当前设计面

策略工作区分成两个横向 tab：

- `/strategy` 默认先进入运行态；这里负责查看实例状态、日志、审计，并对已保存定义做实例化和启停。
- 设计态在固定高度的画布式 SPA 内同时编辑 `visualModel` 和 Pine v6 script，页面本身不滚动。
- 从运行态点击顶部“设计”会进入已有定义编辑；点击“新增策略”会直接进入设计态的模板选择模式。
- 已保存定义列表、样板策略、基本信息、Block Inspector、代码编辑框和元信息都作为悬浮面板叠在画布上。
- Logic Flow 是设计态底层画布，拖拽、连线和节点选择都发生在画布内部；外层 SPA 只负责固定高度和浮层编排。
- 顶部工具栏承载标题、保存、创建运行实例、显示模式切换、面板开关，以及 Pine/流程图同步状态提示。
- 设计态使用 `画布`、`双栏`、`代码` 三态显示切换；纯代码模式下仍可打开样板策略、基本信息、元信息和图块详情等悬浮工具卡。
- 新增草稿和未保存修改离开设计态时会触发确认流程；页内切回运行态、路由离开和浏览器刷新都会走同一套保护。

设计态支持两种协作方式：

- 图优先：通过 Logic Flow 拖拽图块、改 Inspector 参数，系统自动异步回写 Pine。
- 码优先：直接在代码区修改 Pine，系统会在防抖和失焦时自动尝试反解回流程图。
- 混合模式：无法反解成标准图块的 Pine 片段会保留为 `pineSnippet`，继续留在流程图里和标准块并存。

当前约束必须明确：

- 系统只持久化 Pine v6 源码和可选 `visualModel`。
- 新建策略定义的 `id` 现在默认生成 GUID；设计页只展示该 ID，不允许手工修改。策略 `version` 仍由系统在每次有意义保存时自动递增。
- 图块语义保持不变；`visualModel -> script` 由前端生成 Pine v6，后端只接收 `sourceFormat: "pine-v6"`。
- 后端不会直接执行文本脚本；Pine 会先解析为策略 IR/规划结果，再由 Go executor 消费。
- 加载旧定义时会统一归一到 `sourceFormat: "pine-v6"` 和 `runtime: "pine-go-plan"`。
- 如果定义已经保存了 `visualModel`，页面优先保留已保存图结构，直到用户继续改图或改代码。

## 关键职责分层

### 前端页面与组件

- [../../apps/web/src/pages/StrategyPage.vue](../../apps/web/src/pages/StrategyPage.vue)：策略工作区薄包装器，负责运行/设计 tab 切换，并在设计态挂载前传递“编辑已有定义 / 新建模板草稿”的入口模式。
- [../../apps/web/src/components/StrategyDesignStage.vue](../../apps/web/src/components/StrategyDesignStage.vue)：设计态主体，负责 Logic Flow 画布、悬浮 definitions/code workbench、模板选择、图块详情、同步和保存流程。
- [../../apps/web/src/components/StrategyRuntimePanel.vue](../../apps/web/src/components/StrategyRuntimePanel.vue)：运行态主体，负责实例列表、启停控制、日志和审计，并提供“进入设计”和“新增策略模板草稿”两个设计态入口。
- [../../apps/web/src/components/StrategyLogicFlowDesigner.vue](../../apps/web/src/components/StrategyLogicFlowDesigner.vue)：Logic Flow 画布封装，负责图块渲染、选择、图结构更新、视口安全区、缩放 HUD、图块创建器和连线断开。
- [../../apps/web/src/components/MonacoCodeEditor.vue](../../apps/web/src/components/MonacoCodeEditor.vue)：浏览器内 Monaco 包装；注册 `pine-v6` 语言、Pine token、缩进规则、completion 和 hover，测试环境回退 textarea。
- [../../apps/web/src/features/strategyPineEditorIntelliSense.ts](../../apps/web/src/features/strategyPineEditorIntelliSense.ts)：Pine 编辑器的 completion、snippet 和 hover 元数据。
- [../../apps/web/src/features/strategyVisualBuilder.ts](../../apps/web/src/features/strategyVisualBuilder.ts)：图块目录、内置模板、visualModel 克隆/初始化，以及 Pine 和 graph 双向转换的统一导出入口。
- [../../apps/web/src/features/strategyVisualBuilderPine.ts](../../apps/web/src/features/strategyVisualBuilderPine.ts)：`visualModel -> Pine` 生成器，负责把图块、连线、条件、下单、保护和指标节点渲染为 Pine。
- [../../apps/web/src/features/strategyVisualBuilderPineParser.ts](../../apps/web/src/features/strategyVisualBuilderPineParser.ts)：`Pine -> visualModel` 解析器，负责恢复常见指标、条件、动作和 `pineSnippet` 兜底节点。
- [../../apps/web/src/composables/useDraggable.ts](../../apps/web/src/composables/useDraggable.ts)：设计态悬浮面板拖动能力，基于 transform 偏移，不破坏原有绝对定位基线。

### sidecar 持久化与契约

- [../../pkg/jftradeapi/strategy_routes.go](../../pkg/jftradeapi/strategy_routes.go)：`/api/v1/strategy-definitions/*` 路由、Pine 校验和实例化入口。
- [../../pkg/jftradeapi/strategy_design_store.go](../../pkg/jftradeapi/strategy_design_store.go)：策略定义文件存储，包含 Pine runtime/sourceFormat 归一化、旧记录迁移、`visualModel` 归一化和落盘。
- [../../pkg/jftradeapi/strategy_catalog_store.go](../../pkg/jftradeapi/strategy_catalog_store.go)：策略实例目录，实例化时编译 Pine、记录 compiled hooks 和 compiled requirements。
- [../../pkg/jftradeapi/openapi_components.go](../../pkg/jftradeapi/openapi_components.go)：`StrategyDefinition`、`StrategyVisualModel` 等 OpenAPI 契约。
- [../../apps/web/src/contracts/index.ts](../../apps/web/src/contracts/index.ts)：前端页面和测试共享的 DTO 与默认模型都在这里，`visualModel` 结构也以这里为准。

### 运行时

- [../../pkg/strategy/pine](../../pkg/strategy/pine)：Pine v6 前端，负责语法解析、诊断、warning 和 lowering 到策略 IR。
- [../../pkg/strategy/dsl](../../pkg/strategy/dsl)：内部旧 DSL/表达式解析器；当前 Pine lowering 后仍复用表达式子语言校验。
- [../../pkg/strategy/ir](../../pkg/strategy/ir)：策略 IR 模型和需求规划，提取指标、账户能力、仓位和资金依赖。
- [../../pkg/strategy/pineruntime](../../pkg/strategy/pineruntime)：Go 策略执行器，直接消费 Pine lowering 后的 IR 语义并执行条件、通知和下单。
- [../../pkg/strategy/indicatorruntime](../../pkg/strategy/indicatorruntime)：指标预计算运行时，为 Pine lowering 后的 executor 提供 MA、RSI、MACD、KDJ、布林带、ATR、CCI、Williams %R 等序列值。

当前和策略 timeUnit 直接相关的运行时约束需要单独记住：

- 对 `day`、`week`、`month` 这类 timeUnit，策略运行时不再把 US/HK/SH/SZ 一刀切成同一个日内分钟常量；`pkg/strategy/indicatorruntime` 现在会按真实交易日窗口计算 time-bound 指标与保护条件。regular-only 时仍只取 regular session；backtest 打开 extended-hours 后，US 的 `ma(..., day/week/month)` / `ema(..., day/week/month)` / `vwma(..., day/week/month)` 等 moving-average，以及 `stopLoss` / `takeProfit` / `trailingStop` 的 `day/week/month` window，都会把 pre/after/overnight bar 一起纳入同一个 trading-period window。
- 对 `2h`、`4h`、`6h`、`12h` 这类 intraday higher-period，backtest store 现在也会按 market session 起点切 bucket：US regular 从 `09:30` 起桶，HK/SH/SZ 会在午休前后分别重置，不再沿用 UTC floor 把不同时段硬拼进同一根 bar。
- warmup 估算已经改成 symbol-aware：常规情况下 US/HK/SH/SZ 会按各自 regular session 分钟数推导预热 bars；backtest 打开 extended-hours 后，US 的 moving-average trading-period window 会按 extended trading day 分钟数放大 warmup。策略详情预览和实盘 seed 目前仍保持 regular 口径。
- backtest store 的 `1d`、`1w`、`1mo` synthetic path 已改成 market-aware trading period bucket：当原生 higher-period K 线缺失时，会按 trading profile 从 sub-daily 或 daily label 合成真实交易日/周/月；regular-only 模式只取 regular session，HK/SH/SZ 会正确跨午休拼接同一交易日。
- 回测页现在提供“是否包含扩展交易时段”的开关，并把它同时带到 sync 与 run 两条链路。对 US 回测，关闭时会同步/读取 regular-only 数据版本，`2h`/`4h`/`6h`/`12h` 的 synthetic intraday bar 与 `1d`/`1w`/`1mo` synthesis 都只统计 regular session；打开时会同步/读取 extended 数据版本，US 会按 pre/regular/after/overnight 的 session-aware bucket 从 `60m` 及以下 sub-daily 数据合成更高周期 bar，而且 backtest Pine indicatorruntime 的 moving-average 与 stop-loss `day/week/month` 窗口都会切到 extended trading-period 口径。SQLite 现在已用紧凑的表级 session tag 区分 `legacy` / `regular` / `extended` 三套版本，regular-only 与 regular+extended 可以并存。

## 当前内置模板与经典块

当前内置模板包括：

- 逻辑流起步骨架
- 双均线系统
- RSI 反转观察
- MACD 动能观察
- KDJ 交叉交易
- 布林带回归观察
- ATR 波动率过滤
- CCI 反转交易
- Williams %R 反转交易
- 突破告警
- 均值回归告警

当前可视化图块语义包括：

- 生命周期块：`onInit`（界面显示为“策略启动”）、`onKLineClosed`
- 均线块：快均线、慢均线、金叉、死叉
- RSI 块：RSI 计算、超买、超卖
- MACD 块：MACD 计算、diff 高于 signal、diff 低于 signal
- KDJ 块：KDJ 计算、金叉、死叉、J 超买、J 超卖
- ATR 块：ATR 计算、高于阈值、低于阈值
- CCI 块：CCI 计算、高于阈值、低于阈值
- Williams %R 块：Williams %R 计算、超买、超卖
- 布林带块：布林带计算、收盘价突破上轨、收盘价跌破下轨
- 交易块：下单；支持 Pine 可表达的固定股数 `shares`、固定金额 `amount`（生成 `qty=amount/close`）和账户权益百分比 `equityPercent`（生成 `qty=(strategy.equity*pct/100)/close`）
- 动作块：日志、通知
- 退出块：基础止损、止盈和追踪止损优先生成 `strategy.exit`；带交易时段窗口的复杂风控当前会明确标为 unsupported
- 兜底块：`pineSnippet`，用于承载当前不能稳定映射成标准语义块的 Pine 片段。
- 旧 `codeBlock` 和旧合并式 `technicalIndicator` 不再支持；打开、保存或反解时应提示用户用 Pine v6 标准图块或 Pine 片段重建。

这些语义都在 [../../apps/web/src/features/strategyVisualBuilder.ts](../../apps/web/src/features/strategyVisualBuilder.ts) 里定义，并直接决定生成的 Pine 结构。

## 同步与编辑器约束

样式隔离还需要额外注意：

- 策略运行/设计组件中的 scoped 样式，避免写 `:global(.tv-main) ...` 这一类“先全局父级再局部后代”的组合选择器。
- 这类写法在当前构建链路下可能被错误降级成直接命中 `.tv-main`，从而把 border/outline 等视觉规则污染到整个 SPA 容器。
- 组件内样式优先直接使用本地类名（例如 `.strategy-*`），确需跨组件时优先使用明确的页面级包装类，不要绑定到全局 shell 容器。

当前同步策略是双向自动的，但能力并不对称：

- `visualModel -> script`：支持，拖拽建块、连线变化和 Inspector 改参数后都会自动异步刷新代码区。
- `script -> visualModel`：支持常见 Pine v6 子集、内置模板导出的条件分支、日志、通知、下单和指标语句；无法稳定归一化的片段会保留为 `pineSnippet`。
- 无法反解的代码不会直接丢失；工具栏会显示当前是否存在 Pine 片段兜底或解析失败。
- 已保存且自带 `visualModel` 的定义不再执行旧模型迁移；旧 `codeBlock` 或旧合并式 `technicalIndicator` 会被拒绝。

## v1.0 主路径与旧路径清理

- Pine 编辑器是策略 authoring 主路径；保存、预览、回测、实例化和运行统一使用 `sourceFormat: "pine-v6"` + `runtime: "pine-go-plan"`。
- 显式非 Pine source/runtime 不再默认替换为 Pine；后端会返回明确错误。
- 旧 `codeBlock` / 旧合并式 `technicalIndicator` 不再作为类型定义、读取兼容或迁移路径保留。
- Pine 反解遇到旧流程图注解会失败；无法标准化但合法的普通 Pine 行仍会落到 `pineSnippet`。
- 内部统计字段已统一为 `pineSnippetCount`。

代码编辑器当前采用两层实现：

- 浏览器运行时使用 Monaco，语言 ID 为 `pine-v6`。
- 单元测试和 jsdom 环境使用 textarea 回退，保持测试稳定和可操作性；失焦事件同样会触发 code -> flow 自动同步。
- Completion 覆盖 `//@version=6`、`strategy(...)`、`if`、订单、日志、通知和常用 `ta.*` 指标函数。
- Hover 覆盖 `close`、`open`、`high`、`low`、`ta.*` 和下单相关 Pine 片段。

设计页当前还支持双向跳转：

- 点选画布节点（已有 sourceRange）时，代码编辑器自动滚动并选中对应代码区间。
- 在代码编辑器移动光标时，画布自动聚焦到光标所在节点（通过 sourceRange 反查）。
- 工具栏同步状态旁显示映射质量「映射 M/N」，表示当前 visualModel 中多少节点有有效 sourceRange。

设计页当前还支持两类空间管理：

- 已保存定义列表可完全隐藏，恢复入口只保留在顶部工具栏。
- 代码工作台支持拖动、外层面板 resize 和纯代码模式；设计态默认先收起该面板，高度/宽度 resize 只保留在外层容器，最大尺寸受动态视口约束，主要编辑区块也都可单独收起。
- Logic Flow 画布会在重绘后自动重新对齐视口，减少模板节点落在可视区边缘的情况。

因此如果改了编辑器实现，必须同时确认：

- 浏览器里的 Monaco worker 仍然可初始化。
- 测试里的 `strategy-script-editor` 仍然是可输入、可断言的 DOM 控件。
- Pine parser、IR planner 和 Go executor 的窄测试仍然通过。

## 回归检查

当前前端策略页测试已拆成两份：

- `App.strategy.runtime.test.ts` 负责运行态面板、实例绑定、日志审计、runtime observation 和定义刷新。
- `App.strategy.test.ts` 负责设计态、Logic Flow、模板草稿和离开保护。

前端策略设计相关改动，优先跑下面这条窄验证：

```bash
npm --workspace @jftrade/web run test -- --run tests/App.strategy.test.ts tests/App.strategy.runtime.test.ts
```

如果改了 Pine 编辑器或图块转换，再补：

```bash
npm --workspace @jftrade/web run typecheck
```

如果还涉及后端策略定义、解析、规划或执行器，再补：

```bash
go test ./pkg/jftradeapi ./pkg/strategy/dsl ./pkg/strategy/pineruntime ./pkg/strategy/ir
```

如果改动会影响共享 Go 运行时，且希望确认“当前策略设计器里各图块”的成本有没有一起回升，再补：

```bash
go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkCasesSmoke$'

go test ./pkg/backtest -run '^$' \
	-bench '^BenchmarkRunExecutesStrategyBlockMatrix$' \
	-benchtime 1x -benchmem

JFTRADE_UPDATE_STRATEGY_BLOCK_BASELINE=1 \
	go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkBaseline$' -count 1

JFTRADE_ENFORCE_STRATEGY_BLOCK_BASELINE=1 \
	go test ./pkg/backtest -run '^TestStrategyBlockBenchmarkBaseline$' -count 1
```

这套矩阵覆盖当前可视化设计里主要的指标、价格判断、日志、通知、下单和风控图块；它不是替代真实链路 benchmark，而是用来快速判断共享执行路径优化是否真的对整组图块都生效。

其中 `TestStrategyBlockBenchmarkBaseline` 会把基线写入或比对 `pkg/backtest/testdata/strategy_block_benchmark_baseline.json`，适合在做共享 replay/runtime 优化后，补一条半自动性能门槛。
