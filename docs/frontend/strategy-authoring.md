# 前端策略设计专题

JFTrade 的策略设计面是 Pine v6 原生源码工作台。当前 UI 不再使用自由连线的 Logic Flow 画布；它以“结构指令 + Pine 代码”双向编辑同一份源码，并把可结构化部分保存为 `PineV6WorkflowDocument`。

## 页面入口

- `/strategy` 重定向到 `/strategy/runtime`。
- `/strategy/runtime` 展示实例、绑定、运行状态、日志与审计。
- `/strategy/design` 编辑已有定义；`/strategy/design?mode=new` 创建新草稿。

运行页与设计页是独立路由。切换时通过 route query 传递入口模式、提示和 definition ID，不在一个页面内维护隐藏 tab 状态。

## 设计工作台

设计页提供三种显示模式：

- `指令`：显示策略定义、声明、诊断、关联实例和源码结构块。
- `双栏`：结构指令与 Pine 编辑器同时显示。
- `代码`：以 Pine 源码编辑为主，仍保留必要的定义信息。

结构指令不是第二套执行模型。`pineSourceStructureIndex` 解析当前 Pine 源码并生成结构块；新增、移动、复制、删除或修改结构块时，操作直接重写对应源码范围。保存时再从源码构建兼容的 workflow snapshot。

当前结构块覆盖 strategy 声明、input、赋值、条件、循环、函数、collection、`request.security`、订单、风控、visual、alert、log 以及无法结构化的 raw 锚点。具体可编辑字段和默认渲染以代码注册表为准，不在文档复制完整清单。

## 单一事实来源

- 可执行事实是保存的 Pine v6 `script`。
- `sourceFormat` 固定为 `pine-v6`，runtime 主路径为 `pine-pinets`。
- `visualModel` 是可选的结构化编辑投影，不能比源码声明更多执行能力。
- PineTS worker 执行脚本并返回 signal、plot、alert、visual output 与 order intent。
- Go 继续负责调度、回测撮合、风险检查、账户状态和券商下单。

无法安全结构化的 Pine 代码必须保留为 raw/source 内容或给出诊断，不能为了让 UI 看起来完整而改写语义。当 Pine 源码无法标准化为可同步的结构模型时，转换必须返回 `ok:false` 和行号诊断，不能伪造可编辑结构块或静默成功。

## 前端代码入口

- [StrategyDesignPage.vue](../../apps/web/src/pages/StrategyDesignPage.vue)：设计路由包装器，解析新建/已有入口并返回运行页。
- [StrategyRuntimePage.vue](../../apps/web/src/pages/StrategyRuntimePage.vue)：运行路由包装器，装配实例面板并进入设计页。
- [StrategyDesignStage.vue](../../apps/web/src/components/StrategyDesignStage.vue)：定义加载、结构指令、Pine 编辑、分析、保存和实例摘要的主工作台。
- [StrategyRuntimePanel.vue](../../apps/web/src/components/StrategyRuntimePanel.vue)：实例列表、绑定、启停、日志和审计入口。
- [PineSourceCodePane.vue](../../apps/web/src/components/PineSourceCodePane.vue)：Pine 源码编辑器与诊断 marker。
- [PineSourceStructureBlockList.vue](../../apps/web/src/components/PineSourceStructureBlockList.vue)：结构指令列表及块操作。
- [pineSourceStructureIndex.ts](../../apps/web/src/features/pineSourceStructureIndex.ts)：源码结构索引与 snapshot 构建入口。
- [pineV6Workflow.ts](../../apps/web/src/features/pineV6Workflow.ts)：workflow block registry、默认策略、归一化和诊断。
- [strategyPineEditorIntelliSense.ts](../../apps/web/src/features/strategyPineEditorIntelliSense.ts)：Monaco completion、snippet 与 hover 元数据。
- [contracts/index.ts](../../apps/web/src/contracts/index.ts)：`PineV6WorkflowDocument`、策略定义和实例 DTO。

## 后端代码入口

- [routes.go](../../internal/api/strategy/routes.go)：策略定义、实例生命周期与 Pine analyze HTTP 路由。
- [service.go](../../internal/strategy/service.go)：稳定业务门面和 DesignStore/CatalogStore/RuntimeManager 端口。
- [design_store.go](../../internal/app/apiserver/servercore/design_store.go)：当前定义持久化实现。
- [catalog_store.go](../../internal/app/apiserver/servercore/catalog_store.go)：当前实例目录持久化实现。
- [strategy_adapters.go](../../internal/app/apiserver/servercore/strategy_adapters.go)：store/runtime 实现到 `internal/strategy` 端口的装配适配。
- [pine](../../pkg/strategy/pine)：Pine 解析、语义诊断与 lowering。
- [pineworker](../../pkg/strategy/pineworker)：PineTS gRPC client、worker manager 与执行契约。
- [indicatorruntime](../../pkg/strategy/indicatorruntime)：Go 侧需求计算和指标序列能力。

## 编辑与保存约束

1. 修改结构块后必须保持源码可解析；结构视图和源码不能各自独立保存。
2. Pine analyze 的 error 会阻止把定义当作可运行策略；warning 不能被 UI 隐藏。
3. 定义 ID、版本和软删除语义由后端 store 归一，不由前端伪造历史版本。
4. 实例绑定引用定义版本；更新定义不会绕过实例刷新与运行状态约束。
5. 新增 Pine public surface 时，同步 parser/semantic、PineTS worker、生成支持快照、Monaco 元数据和回归测试。
6. 不恢复已经移除的 Go 计划运行时、旧自定义 helper syntax 或自由连线 visual block 兼容路径。

## 时间周期与回测边界

- 日/周/月及高周期聚合按 market session 处理，不用固定 UTC bucket 代替交易时段。
- regular-only 与 extended-hours 数据版本可以并存；回测的 sync 与 run 必须使用同一时段选择。
- warmup 由 symbol、timeframe、指标历史依赖和 session 范围共同决定。
- PineTS 负责脚本执行，`conservative-bar-v1` 的订单成交仍由 Go 完成；详见 [回测执行模型](../backtest-execution-model.md)。

## 最低验证

```bash
go test ./internal/strategy ./internal/api/strategy ./pkg/strategy/... -count=1
pnpm --filter @jftrade/web run test -- Strategy
pnpm --filter @jftrade/web run typecheck
```

涉及 public Pine 支持范围时，还要运行生成参考并确认 [Pine v6 支持快照](../reference/generated/pine-v6-support.md) 与代码一致。
