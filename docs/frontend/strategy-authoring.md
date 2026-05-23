# 前端策略设计专题

本文只回答三件事：

- QuickJS 策略定义和可视化流程图现在分别落在哪一层
- Logic Flow、Monaco 和模板生成各自负责什么
- 后续改模板、图块、同步行为时，应该先从哪个文件进入

## 当前设计面

策略工作区现在分成两个横向 tab：

- `/strategy` 默认先进入运行态；这里负责查看实例状态、日志、审计，并对已保存定义做实例化和启停
- 设计态：在固定高度的画布式 SPA 内同时编辑 `visualModel` 和 QuickJS script，页面本身不滚动
- 从运行态点击顶部“设计”会进入已有定义编辑；点击“新增策略”会直接进入设计态的模板选择模式
- 已保存定义列表现在是独立悬浮面板，收起后会完全隐藏，只能通过顶部工具栏按钮重新打开
- 点击“新建”时，设计态会先进入模板选择模式：只保留样板策略选择面板；选中模板后，模板面板自动收起，再回到代码/画布编辑工作区
- 新增草稿和未保存修改离开设计态时，现在会先触发确认流程；页内切回运行态、路由离开和浏览器刷新都会走同一套保护
- Logic Flow 是设计态底层画布，拖拽、连线和节点选择都发生在画布内部；外层 SPA 只负责固定高度和浮层编排
- 画布上方现在合并成单张工具栏卡片，统一承载标题、保存/创建运行实例、显示模式切换、面板开关，以及代码/流程图自动同步状态提示
- 顶部成功/错误提示支持手动关闭，避免持续遮挡工具栏和画布
- 已保存定义列表、样板策略、基本信息、Block Inspector、代码编辑框和元信息都作为悬浮面板叠在画布上；设计态默认停在 `画布` 模式并收起代码工作台，切到 `双栏` 或 `代码` 时才展开代码区，其它工具卡仍通过顶部工具栏按需打开
- 设计态现在用三态显示切换替代旧的“代码编辑框 / 纯代码模式”按钮：`画布`、`双栏`、`代码`
- 切到 `代码` 纯代码模式后，顶部工具栏仍然可以正常打开样板策略、基本信息、元信息和图块详情等悬浮工具卡，不再因为隐藏画布而失效
- Logic Flow 底层画布需要铺满整个设计态 SPA；节点内容的初始居中应避开顶部工具栏、左侧 definitions、右侧代码面板和底部 HUD 的遮挡区
- 画布右下角现在只保留约 18em 宽的横向缩放 HUD，并固定锚在右下角；100% 表示当前安全区下的适应视图，适应动作改成图标按钮；手动“重写代码”和“重置流程图”按钮已经移除
- 这块右下角缩放 HUD 在模板选择态和正常画布编辑态都应保持可见；当右侧代码面板打开时，控件会自动左移，避免被代码面板盖住
- Flow 创建器改成左下角固定的折叠式 launcher；它在设计态浮层里保持最高层级，展开后网格向上弹出，关闭按钮仍留在原始 launcher 位置，并在顶部提供搜索框；展开区有最大高度限制，内部自行滚动，用户直接从这里拖拽图块进入画布
- 模板、基本信息、元信息和 Inspector 这类悬浮工具卡优先以横向平铺条带展示，不再默认做纵向堆叠
- 横向工具卡条带需要固定落在工具栏下方并留出明确间距，避免标题和首行内容被工具栏压住
- 横向工具卡条带当前会和 definitions 面板共享同一垂直高度，避免模板/表单卡片比左侧仓库定义面板明显矮一截
- Logic Flow 组件在设计态使用无外壳模式铺满底层画布；独立组件默认仍保留原有卡片外壳，避免影响其它潜在用法
- 为避免画布节点被顶部工具栏或底部 HUD 压住，设计态会给 Logic Flow 视口保留上下安全区，并在模型重绘后重新对齐视口
- 选中任意流程图节点时，“图块详情”卡片会自动展开；点击画布空白处后，该卡片会随选中态一起隐藏
- Definitions、样板策略、基本信息、元信息和 QuickJS 代码工作台的面板标题现在统一为单层标题，不再拆成 kicker + title 两行

设计态当前支持两种协作方式：

- 图优先：通过 Logic Flow 拖拽图块、改 Inspector 参数，系统自动异步回写 QuickJS
- 码优先：直接在代码区修改 QuickJS 脚本，系统会在防抖和失焦时自动尝试反解回流程图
- 混合模式：无法反解成标准图块的脚本片段会保留为 `codeBlock`，继续留在流程图里和标准块并存

当前约束必须明确：

- 系统会持久化 `script` 和可选 `visualModel`
- visual builder 生成 QuickJS 时，除 lifecycle/helper 的 JSDoc 外，还会给每个可视节点补一段 flow JSDoc，至少携带 `nodeId`、`blockKind`、`nodeText`，`codeBlock` 还会额外带 `codeScope`
- `script -> visualModel` 目前支持常见 QuickJS 策略骨架、内置模板生成的条件分支结构，以及简单 `console.log(expr)` 这类表达式日志；复杂控制流或无法稳定归一化的自定义表达式仍会降级成 `codeBlock`
- 加载已有定义时，如果只有代码没有 `visualModel`，页面会自动尝试反解流程图；如果定义已经保存了 `visualModel`，则优先保留已保存脚本，直到用户继续改图或改代码

## 关键职责分层

### 前端页面与组件

- [../../apps/web/src/pages/StrategyPage.vue](../../apps/web/src/pages/StrategyPage.vue)：策略工作区薄包装器，负责默认进入运行态、协调运行/设计 tab 切换，并在设计态挂载前传递“编辑已有定义 / 新建模板草稿”的入口模式
- [../../apps/web/src/components/StrategyDesignStage.vue](../../apps/web/src/components/StrategyDesignStage.vue)：设计态主体，负责 Logic Flow 画布、悬浮 definitions/code workbench、模板选择、图块详情、同步和保存流程；现在默认以画布态进入并收起代码工作台，同时负责未保存修改签名跟踪，以及页内切换、路由离开、刷新关闭前的离开确认
- [../../apps/web/src/components/StrategyRuntimePanel.vue](../../apps/web/src/components/StrategyRuntimePanel.vue)：运行态主体，负责实例列表、启停控制、日志和审计，并提供“进入设计”和“新增策略模板草稿”两个设计态入口
- [../../apps/web/src/components/StrategyLogicFlowDesigner.vue](../../apps/web/src/components/StrategyLogicFlowDesigner.vue)：Logic Flow 画布封装，负责图块渲染、选择、图结构更新，以及拖拽调整画布高度后的 resize 同步；现在会在原生拖拽调高过程中按帧调用 resize，对安全区执行 fitView + 偏移居中，提供可随右侧代码面板让位的右下角缩放 HUD，并把图块创建器改成左下角折叠式 launcher，展开时网格上弹、关闭入口保留在原始锚点
- [../../apps/web/src/components/MonacoCodeEditor.vue](../../apps/web/src/components/MonacoCodeEditor.vue)：浏览器内 Monaco 包装；支持拖拽调整高度、QuickJS 宿主 API 声明注入、常用 hook/snippet completion，以及 ctx/ctx.kline、runtime host API、模板 helper 和因子变量的 hover 文档，在 jsdom 测试里自动回退到 textarea；显示模式切换或面板隐藏时会跳过已脱离 DOM 的异步初始化，避免浏览器侧挂载报错
- [../../apps/web/src/features/strategyEditorIntelliSense.ts](../../apps/web/src/features/strategyEditorIntelliSense.ts)：策略脚本编辑器的额外类型声明、completion/snippet 定义和 hover 元数据；现在和真实 QuickJS runtime host API 对齐
- [../../apps/web/src/features/strategyVisualBuilder.ts](../../apps/web/src/features/strategyVisualBuilder.ts)：图块目录、内置模板、visualModel 克隆/初始化，以及 script 和 graph 双向转换的统一导出入口
- [../../apps/web/src/features/strategyVisualBuilderParser.ts](../../apps/web/src/features/strategyVisualBuilderParser.ts)：QuickJS -> visualModel 解析器，负责识别常见指标/条件/动作块、模板生成的 guard + condition 分支，以及简单表达式日志，并把无法标准化的代码折叠成 `codeBlock`
- [../../apps/web/src/composables/useDraggable.ts](../../apps/web/src/composables/useDraggable.ts)：设计态悬浮面板拖动能力，基于 transform 偏移，不破坏原有绝对定位基线

### sidecar 持久化与契约

- [../../pkg/jftradeapi/strategy_routes.go](../../pkg/jftradeapi/strategy_routes.go)：`/api/v1/strategy-definitions/*` 路由和实例化入口
- [../../pkg/jftradeapi/strategy_design_store.go](../../pkg/jftradeapi/strategy_design_store.go)：策略定义文件存储，包含 `visualModel` 归一化和落盘
- [../../pkg/jftradeapi/openapi_components.go](../../pkg/jftradeapi/openapi_components.go)：`StrategyDefinition`、`StrategyVisualModel` 等 OpenAPI 契约
- [../../packages/ui-contracts/src/index.ts](../../packages/ui-contracts/src/index.ts)：前后端共享 DTO，前端页面和测试都依赖这里的 `visualModel` 结构

### 运行时

- [../../pkg/strategy/quickjs/strategy.go](../../pkg/strategy/quickjs/strategy.go)：QuickJS 运行时桥接，负责把生成脚本接到 bbgo 生命周期，并暴露 `notify`、`placeOrder`、`cancelOrder`、`getPosition`、`getPositions`、`getRiskState`、`isOperationBlocked`

## 当前内置模板与经典块

当前内置模板包括：

- 逻辑流起步骨架
- 双均线系统
- RSI 反转观察
- MACD 动能观察
- 布林带回归观察
- 突破告警
- 均值回归告警

当前可视化图块语义包括：

- 生命周期块：`onInit`（界面显示为“策略启动”）、`onKLineClosed`
- 均线块：快均线、慢均线、金叉、死叉
- RSI 块：RSI 计算、超买、超卖
- MACD 块：MACD 计算、diff 高于 signal、diff 低于 signal
- 布林带块：布林带计算、收盘价突破上轨、收盘价跌破下轨
- 动作块：日志、通知
- 兜底块：`codeBlock`，用于承载当前不能稳定映射成标准语义块的 QuickJS 代码

这些语义都在 [../../apps/web/src/features/strategyVisualBuilder.ts](../../apps/web/src/features/strategyVisualBuilder.ts) 里定义，并直接决定生成的 QuickJS 结构。

## 同步与编辑器约束

当前同步策略是双向自动的，但能力并不对称：

- `visualModel -> script`：支持，拖拽建块、连线变化和 Inspector 改参数后都会自动异步刷新代码区
- `script -> visualModel`：支持常见 QuickJS 结构、内置模板导出的兄弟分支条件，以及简单 `console.log(expr)` 日志；如果脚本里保留了 visual builder 生成的 flow JSDoc，解析器还会优先用它恢复被重命名的节点标题、稳定 node id，以及多语句 `codeBlock` 的边界；代码区输入防抖到时或编辑器失焦时会自动尝试反解流程图
- 无法反解的代码不会直接丢失，而是会转成 `codeBlock` 节点保留在图里；工具栏会显示当前是否存在 code block 兜底或解析失败
- 已保存且自带 `visualModel` 的定义，打开时仍以现有保存内容为准；只有后续发生图编辑或代码编辑时，才会触发新的自动同步

代码编辑器当前采用两层实现：

- 浏览器运行时使用 Monaco，提供 JavaScript 高亮和基础语言服务
- 单元测试和 jsdom 环境使用 textarea 回退，保持测试稳定和可操作性；失焦事件同样会触发 code -> flow 自动同步
- 策略编辑器额外注入 `notify`、`JFTradeInitContext`、`JFTradeKLineClosedContext` 等声明，并补 `onInit` / `onKLineClosed` snippet completion
- 策略编辑器现在还会给 `ctx`、`ctx.kline.close`、`placeOrder` / `getPosition` / `getRiskState`、模板生成的 `simpleMovingAverage` / `calculateRSI` / `calculateMACD` / `calculateBollingerBands`，以及 `state.closes` / `latestRsi` / `latestMacdDiff` 这类因子运行时变量提供 hover 文档
- `placeOrder` / `cancelOrder` / `getPosition` / `getPositions` / `getRiskState` / `isOperationBlocked` 现在已经接到真实 QuickJS runtime，不再只是编辑器预留
- `getRiskState()` 返回的是 runtime 本地会话能力快照：它反映 executor、账户能力和被阻断的操作列表，不是控制面 `real-trade-risk` 接口的直通镜像
- Logic Flow 画布和策略设计里新增的可视化卡片都需要走主题变量，不应继续写死浅色渐变、白底半透明或扩展库默认白色浮层
- visual builder 生成的 QuickJS 现在会同时输出两类 JSDoc：一类给 hook 和常用 helper 补类型，避免 Monaco 在 `checkJs` 下把参数推成隐式 any；另一类给每个 visual node 写 flow 元标签，供 code -> flow 反解时恢复标题、边界和稳定映射

设计页当前还支持双向跳转：

- 点选画布节点（已有 sourceRange）时，代码编辑器自动滚动并选中对应代码区间
- 在代码编辑器移动光标时，画布自动聚焦到光标所在节点（通过 sourceRange 反查）
- 工具栏同步状态旁显示映射质量「映射 M/N」，表示当前 visualModel 中多少节点有有效 sourceRange

设计页当前还支持两类空间管理：

- 已保存定义列表可完全隐藏，恢复入口只保留在顶部工具栏
- QuickJS 代码工作台支持拖动、外层面板 resize 和纯代码模式；设计态默认先收起该面板，高度/宽度 resize 只保留在外层容器，最大尺寸受动态视口约束，主要编辑区块也都可单独收起
- Logic Flow 画布会在重绘后自动重新对齐视口，减少模板节点落在可视区边缘的情况

因此如果改了编辑器实现，必须同时确认：

- 浏览器里的 Monaco worker 仍然可初始化
- 测试里的 `strategy-script-editor` 仍然是可输入、可断言的 DOM 控件
- 如果改了 QuickJS host API 桥接，还要补跑 [../../pkg/strategy/quickjs/strategy.go](../../pkg/strategy/quickjs/strategy.go) 对应的 Go 测试

## 回归检查

前端策略设计相关改动，优先跑下面这条窄验证：

```bash
npm run typecheck && npm --workspace @jftrade/web run test -- App.strategy.test.ts
```

如果还涉及 QuickJS runtime host API，再补：

```bash
go test ./pkg/strategy/quickjs
```

如果还涉及后端策略定义接口，再补：

```bash
go test ./pkg/jftradeapi
```