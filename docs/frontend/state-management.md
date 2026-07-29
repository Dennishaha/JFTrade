# 前端状态管理约定

JFTrade 前端不引入 Pinia。当前状态规模可以由 Vue 组件状态、feature
composable、`provide/inject` 和 TanStack Vue Query 清晰表达；再增加一套全局
store 只会制造第二套缓存与生命周期规则。本约定的目标不是统一 API，而是让每份
状态只有一个明确 owner。

## 选择规则

| 状态种类 | owner | 使用方式 | 典型场景 |
| --- | --- | --- | --- |
| 后端资源快照、列表和异步状态 | QueryClient | `useQuery` / `useInfiniteQuery` / `useMutation` | 回测记录、自选、运行依赖、设置读写 |
| 单个页面或组件树的交互状态 | 页面 feature composable | 在 composable 函数内创建 `ref` / `computed`，由页面调用一次并下传 | 回测筛选、下单草稿、研究面板选择 |
| 只服务一棵组件子树的协作状态 | 最近共同祖先 | typed `provide/inject` context | 策略设计工作台、复杂编辑器子面板 |
| 可分享、可刷新后恢复的导航状态 | Vue Router | route params/query | 当前 tab、可链接的筛选和对比对象 |
| 需要跨路由存续的纯客户端协调状态 | 明确命名的 singleton composable | 模块级 `ref`，必须有 reset 和测试 | 当前 broker 选择、市场 profile 缓存 |
| 单组件临时状态 | 组件 | `<script setup>` 内局部 `ref` | dialog 开关、hover、输入焦点 |

判断顺序：先问数据是否来自后端；再问刷新后是否应保留；最后才判断是否真的需要
跨路由共享。不要为了少传一层 prop 把局部状态提升成模块单例。

## TanStack Vue Query 的边界

Vue Query 专门管理服务器状态，而不是替代所有 composable：

- GET/列表查询用稳定、可序列化的 query key；key 必须包含会改变结果的 broker、
  account、market、instrument、分页或筛选参数。
- 写操作使用 mutation；成功后通过 `invalidateQueries` 或精确 `setQueryData` 更新
  唯一缓存，禁止同时维护一份模块级 `ref` 镜像。
- SSE/WebSocket 推送可以精确更新 Query cache；高频逐 tick 图表状态仍由行情
  feature 自己拥有，避免把 Query cache 当事件总线。
- 轮询优先使用 query 的 `refetchInterval`、enabled 和取消能力；只有流式协议、
  生命周期编排或非资源型任务才保留专用 polling composable。
- DTO 必须先经过 `@/contracts` 与 mapper 边界，Query cache 不直接成为页面 view
  model 的隐式 wire contract。

当前 `useBacktestRuns`、`useWatchlist`、`useWatchlistImport` 和设置页资源查询是可复用
范例。新建服务器资源 composable 时，应优先沿用这些模式。

## Feature composable

页面级 composable 在函数调用内创建状态，每个页面实例拥有自己的生命周期。它可以
组合 Query、路由和局部状态，但不应偷偷导出模块级可变对象。命名使用
`use<Feature>`，返回值按 `state`、derived state 和 actions 组织，并在页面卸载时
清理 timer、listener、socket 或 AbortController。

当同一 feature 被多个子组件共同使用时，在页面根调用一次并用 props/emits 下传；
层级过深且消费者稳定时才使用 typed context。context key 与 `provide`/`inject`
helper 放在对应 feature 目录，不放入全局 `composables/shared`。

## 模块级 singleton 的准入条件

模块级 `ref` 只允许用于确实需要跨路由共享、且不属于服务器缓存的数据。新增前必须
同时满足：

1. 文档或代码注释写明 owner、生命周期和持久化策略；
2. 导出幂等的 reset/dispose 方法，测试在 `afterEach` 中调用；
3. 异步结果有 generation/token 防止旧请求覆盖新状态；
4. 不复制 Query cache、路由或 localStorage 已经拥有的数据；
5. 对外只暴露 readonly state 和显式 action。

`brokerProviderSelection` 与 `marketProfiles` 是现有受控 singleton。若同类状态继续
增长到需要跨模块 action、plugin 或时间旅行调试，再以实际需求评估 Pinia；文件数量
本身不是引入 store 的理由。

## 测试要求

- Query 测试为每个用例创建新的 `QueryClient`，关闭 retry，并在卸载后 clear。
- singleton 测试必须 reset；禁止依赖 Vitest 文件执行顺序或前一个用例残留。
- timer、轮询和 debounce 使用 fake timers 或可注入时钟；并发完成使用可观察条件，
  不用固定 `sleep` 猜测调度时机。
- context/composable 测试验证 owner 边界：两个独立 mount 不应共享页面局部状态。

## 目录与 import

composable 按业务关注点放入 `composables/<domain>/`；消费者使用完整的
`@/composables/<domain>/<module>` 路径，让依赖对象保持可见，并避免有副作用的
composable 被宽 barrel 意外带入。各域 `index.ts` 只记录经过审查的公共目录，不建立
根级兼容出口。`shared/` 只容纳无业务 owner 的通用能力，不能作为暂存目录。
`features/strategy-builder` 与 `features/pine-structure` 属于纯逻辑域，消费者统一从域
入口导入，禁止深引内部文件。服务器 DTO 从 `@/contracts` 获取，不直接引用
`@/generated/openapi`。
