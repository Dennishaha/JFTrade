# JFTrade 活动路线图

更新时间：2026-08-19。

本文是仓库内唯一的活动计划入口，只记录尚未完成且仍值得推进的工作。已经落地的设计应写入对应专题文档；一次性迁移过程、发布冻结说明和验收日志由 Git 提交与发布 tag 保留，不继续作为维护文档存在。

## Go/Wails → Rust/Tauri 完整迁移

完整范围、迁移守则、依赖选择、九阶段步骤和性能/资源门禁见 [architecture/go-to-rust-migration.md](architecture/go-to-rust-migration.md)。活动路线只保留阶段状态：

- [ ] 阶段 1：Rust 工程、authenticated loopback Tonic health bridge、依赖治理、affected/完整门禁和四目标 CI 基础已落地且本地 cross-check 通过；等待首次上游原生四平台矩阵后关闭。
- [x] 阶段 2：共享领域模型、codec 与 SQLite 只读 differential 本地工作包完成；Go 保持唯一生产 owner。
- [x] 阶段 3：`conservative-bar-v1` 回测纯计算核心、三方 differential、取消/超时恢复、owner 回退演练和本机 release 资源门禁完成；生产 Pine replay 仍由 Go 拥有。
- [x] 阶段 4：行情 Provider、PineTS/Python helper 生命周期与 Futu/OpenD adapter 本地工作包、三方 differential 和 release replay 资源基线完成；真实 live、发布资产/原生平台资格仍阻断产品切流，Go 保持唯一生产 owner。
- [x] 阶段 5：交易/策略/通知 shadow、订单/成交/持仓状态、三方 differential、零 dispatch 与 release replay 资源基线完成；只读 OpenD、显式小范围 live 和持久化恢复仍阻断产品切流，Go 保持唯一写 owner。
- [ ] 阶段 6：Assistant 使用 Rig 迁移。
- [ ] 阶段 7：Rust API/control plane 接管产品流量。
- [ ] 阶段 8：Wails → Tauri 桌面迁移。
- [ ] 阶段 9：删除 Go/Wails 并发布 Rust 大版本。

已完成的阶段 2/3/4/5 本地工作包同样不改变公开 HTTP/OpenAPI、SSE/WS、Wails bindings、SQLite schema 或产品运行入口；阶段 4 对 retained worker 的鉴权扩展默认关闭且保持 Go 启动兼容，阶段 5 Rust 输出固定无副作用。Go/Wails 仍是唯一生产 owner；阶段完成事实和后续生产切换条件以迁移专题账本为准。

## AI 开发效率治理

本轮按 P0-P3 分阶段硬切，验收重点是上下文一致性、反馈速度和热点包的可维护规模。公开 HTTP、OpenAPI、SSE/WS、Wails bindings、SQLite schema 与 `pkg/*` API 保持不变。

### P0 上下文统一

- [x] 根 `AGENTS.md` 作为事实源，`CLAUDE.md` 通过导入复用。
- [x] 为 apiserver、assistant、Web、Futu、worker 增加局部指令和 `scripts/module-map.json`。
- [x] 增加 `check:ai-context`，阻止旧 `pkg/jftradeapi` 等路径回流。
- [x] 移除未引用根目录 HTML inspection 资产及其第三方声明。

### P1 反馈环与门禁

- [x] 增加 `check:quick`、`test:affected`、`check:generated`、`check:all`。
- [x] 生成器支持统一输出根，契约只读检查使用临时目录。
- [x] Go/Web 文件长度和 servercore/assistant 预算改为只减不增的 ratchet；`servercore-budget.json`/`assistant-budget.json` 记录目标值，`scripts/tighten-budgets.mjs` 提供只降不升的自动收紧。
- [x] 测试命名门禁拒绝覆盖率数字和空泛后缀。

### P2 后端热点

- [x] 将 servercore 收缩为 HTTP/frontend shell 与 composition root，迁移 webaccess、liveapp、tradingapp、marketdataapp；Futu provider 装配与 HTTP adapter 下沉到 marketdataapp/futuapp，broker/status 访问器改为窄函数；Server 私有编排/选项/路由辅助方法降为包级函数，有效方法面 88→59（生产 3324 行、Server 方法 14）。
- [x] servercore 黑盒 HTTP/契约测试迁至 `internal/app/apiserver/servercoretest`（公开 SidecarHandler 驱动，仅保留依赖私有 router/stores/runtime 的用例在包内），测试行 14991→8947。
- [x] 将 assistant/engine 拆为 model、persistence、providers、workflowruntime、skillsruntime；子包目录和外部依赖门禁已建立（外部直接依赖 engine 为 0）。共享 DTO/状态/归一化已下沉 `internal/assistant/model`（含单调可排序时间戳、输入校验、composer 归一化、SessionProjection 与 session timeline 投影、workflow plan 投影/编译、planner draft 归一化与 step sanitize、planner 提示/步骤 schema、任务工具 schema、workflow canvas 编译、workflow 子任务/最终合成指令与 observation 匹配、workflow 暂停/工具错误分类/任务摘要、goal decision 状态机、goal 提示构建、任务工具描述、runner 会话/审批/输入/生命周期状态辅助、run 终态投影（status/error code/audit kind/usage 结算/tool 摘要/optimization ID/failed-completed chat run）、审批结果摘要与用户可见错误映射、input request 追加/最新归一化等执行支撑状态）；RunLease/ToolInvocationClaim、SQLite lease/claim、chat 幂等性（UUID 指纹 + 原子 claim）、ADK artifact service、audit/maintenance/notice/workflow store、Provider/Agent/Session CRUD、Skill/OptimizationTask/Task/Memory CRUD、Run/approval 存储与 approvalStage 事务、composer/handoff/context/input store 已全部迁入 `engine/persistence`，根包 `Store` 仅保留 composition 壳与 nil 语义；safe HTTP/legacy reasoning/OpenAI Responses adapter、OpenAI chat client/消息归一化/流式聚合、Google-compatible ADK model 与 ADK memory adapter 已迁入 `engine/providers`，根包仅保留委托壳与工具描述转换；skill registry、builtin 目录与安装/解压管线已迁入 `engine/skillsruntime`（tool schema 同包），根包仅保留公开委托壳与既有测试语义。硬切接缝已建立：根包新增 `WorkflowExecution` 执行接口（Run/FailParent/ResumeLoopWorkflow/ReconcileWorkflowChildren/CompleteResumedWorkflow/ResumeADKGoalWorkflow/WorkflowTasks/PersistWorkflowTasks/RunPlannedGoogleADKWorkflow/WorkflowResponse），`Runtime.SetWorkflowExecutor` 支持装配期注入，`WorkflowRequest/WorkflowStep/WorkflowGoalDecision/AssistantExecutionResult` 已导出，`NewWorkflowExecutor` 提供默认实现构造入口；编排面 9 个执行方法已直接导出（无桥接壳），googleADKExecution 的执行方法面已导出（Run/PendingApprovals/ToolContextForRun/ResultForRun/SetInputRequests/DetachDeltaSink/WorkflowRunObserved/RunNeedsFinalSynthesis/RunHasPostToolText/SetRunIDByAgentName/HasFinalReplyForRun，`toolExecutionContext` 同步导出为 `ToolExecutionContext`）；`WorkflowExecutorRuntime` 服务接口已建立（22 个导出编排方法 + `Store`/`RegisterWorkflowExecution`/`WithWorkflowChildLock`/`RunExecutionInFlight`/`ModelsListTool`），`WorkflowExecutor` 已改为只依赖该接口，`PendingInputRequests` 接受 `WorkflowExecutionHandle`（含 session/appName/sessionID/agent/tracked-call 访问器），`NewGoogleADKTaskExecution`/`NewGoogleADKWorkflowExecution`/`RunGoogleADKWorkflowChildFinalSynthesis` 全部走句柄接口；装配层已显式注入 `workflowruntime.NewWorkflowExecutor(runtime)`，并有端到端注入测试（真实 Store + loop ChatStream 走注入的执行器并返回哨兵错误）；39 个 engine→model 纯委托壳已删除并内联为 `jfadkmodel.*` 调用（含任务工具/planner schema），纯函数测试已开始迁出根包（workflow plan 展示/边界、goal decision/提示/任务摘要、planner 参数解析、approval run 过滤改测 model 公开函数）。engine 根包当前 14825 生产行 / 33004 测试行（预算已收紧至 14825/33004，均为只降不升的 ratchet；runner_chat.go 与 runner_lifecycle.go 已低于 800 行，过期文件长度例外已清除）。剩余结构性工作：planner/workflow 执行器已复制到 `engine/workflowruntime`（executor_workflow/child/task/task_tools/plan/approval 约 2000 行，真实 Store + fake OpenAI provider 的 loop 端到端冒烟测试通过），装配层已切换为注入该实现；google runner、runner/session orchestration 仍在 engine 根包；根包执行器白盒测试已从 21 个文件 / 86 处引用收缩为 3 个刻意保留的文件 / 3 处（native task graph 终态持久化、`newBareGoogleADKExecution` + `RegisterWorkflowExecution` 句柄注册、`activeMu`/`activeRuns` blockers），其余 runner 编排用例已改为经 `runtime.workflowExecutor()` 注入缝传入执行器。已迁至 workflowruntime 的组：workflow_helpers、child-failure、goal-turn、task-state、goal-terminal、taskset-done、taskset-biz(toolset 部分)、task-tools-persistence、execution-failure、workflow-persistence、execution-persistence（raw session 故障场景通过注入 failGetSessionService 等价复现）、child-finalization、task-limit、child-lifecycle、reconcile-ignore、approval-persistence、reconcile-executor、resume-executor、finalization-contracts、task-tools、goal-resume-failure、goal-state、approval-recovery、executor-boundary-branches、persistence-propagation-closeout、persistence-failure、goal-pause/response、run-child-approval、models-list-tool；`ToolExecutionContext`/`RunStartOptions` 已下沉 model 并导出字段，因此尚未删除根包默认实现。执行器契约下沉已完成第一步：`WorkflowRequest`、`AssistantExecutionResult`、`WorkflowStore` 窄接口与 `WorkflowExecution`/`WorkflowExecutorRuntime`/`WorkflowExecutionHandle` 三个契约接口已迁入 model，根包改为别名并新增 WorkflowStore() 访问器，执行器代码改经 WorkflowStore() 访问持久层；`ErrUserGoalPauseRequested`、`ErrorFromSerializedADKText`、`GoogleADKWorkflowChildName`、`HydrateRunExecutionResult`、`NewWorkflowMapFunctionTool(s)`、`WorkflowMapToolSpec`、`PendingApprovalsOnly`、`ReusedChatRequestError` 的规范实现均已下沉 model（根包保留委托壳），engine 生产行 15017→14825、测试行 33006→33004。删除根包执行器副本的最后一步仍受 import 环约束：workflowruntime 门面仍依赖 engine 根包的 `Runtime`/`Store` 具体类型与构造入口，根包测试二进制无法导入 workflowruntime，因此需先把执行器契约下沉到 model/共享层、由 engine 根包改为依赖 workflowruntime 后，才能移除根包默认实现并清零上述 3 处引用。迁入前提是保持现有“外部消费者经 workflowruntime 门面访问 engine、外部直接依赖 engine 为 0”的约束：执行实现只能放在 workflowruntime（依赖 engine 根包），engine 根包通过自身定义的执行接口 + 装配期注入调用，且需要把依赖私有 Runtime 内部的方法导出、并把引擎根包内直接调用执行器的测试迁移到外部测试包或新包后再删除根包实现——这是多步硬切，不能在一次改动内完成。已完成最后硬切：门面保留在 `internal/assistant/engine/workflowruntime`（目录/包名是生成 OpenAPI 定义名的组成部分，不可移动），执行器实现整体迁至新叶子包 `internal/assistant/engine/workflowexec`（生产代码只依赖 model）；根包执行器副本（workflow.go、workflow_child.go、workflow_task.go 执行器部分、workflow_task_tools.go、workflow_approval.go 执行器方法）已删除，根包 `WorkflowExecutor{` 引用清零，`workflowExecutor()` 改为注入必填并在未装配时返回错误；`NewGoogleADKTaskExecution` 改由执行器注入 `WorkflowTaskToolset`（模型接口增加 `adktool.Toolset` 参数），`PruneInterruptedGoalWorkflowToolCalls`/`InterruptedGoalWorkflowToolCall` 下沉 model；根包启动 reconcile 在装配执行器后执行（构造期未装配时标记待办，`SetWorkflowExecutor` 触发）；根包白盒引用已迁移/删除（native task graph 终态改测 workflowruntime 导出方法、RegisterWorkflowExecution 契约保留在根包、activeMu blockers 测试随根包 toolset 删除并由 workflowruntime 等价用例覆盖），pending-input 终态测试迁至 workflowruntime。engine 生产行 14825→12691、测试行 33004→32898（预算 ratchet 已收紧至 12691/32898），外部直接依赖 engine 保持 0，核心 engine 测试约 15 秒（目标 <20 秒已达成）。

### P3 Web、worker 与 CI

- [x] 拆分 Web 超限 composable、Pine IntelliSense、Visual Builder 和巨型测试；手写 src ≤800 行、测试 ≤1200 行，预算例外为 0。
- [x] 拆分 PineTS executor（799 行）与 Python market-data provider（akshare_identity/catalog/search/candles/quotes/conversion/upstream，provider 门面 68 行），消除 Python 3.14 弃用警告噪声；marketdata-sidecar Python 3.14 venv 全量测试 138 项通过（无 warning 噪声）；US/HK 分钟聚合确定性测试与 CN 用例一样固定 `_utc_now`，不再随 5 天保留窗口漂移。
- [x] 将 CI/desktop workflow 重复步骤收敛为 composite action/reusable workflow，并同步桌面发布门禁测试。
- [x] Web diff coverage 门禁恢复全绿：为 ADK 运行/审批、Research 布局与控制器、StockScreener 预设补充边界测试，并清理 Visual Builder 拆分文件中被规范化保证恒有值、实际不可达的 `??` 兜底分支（PineStatements 分支 145→56、PineIndicatorExpressions 165→58、PineParserExpressions 176→152），门禁阈值不变。

### 验收指标

- [x] `check:generated` 只读且不修改工作树（实测 13.8 秒）。
- [x] `check:quick` 典型单领域小于 30 秒：架构门禁改为单次 `go list` 快照后从约 22 秒降至 3.2 秒，`go vet ./...` 改为按受影响模块收敛；assistant 受影响测试加 `-p=4` 后完整命令序列实测 28.7 秒（engine 单包约 14–17 秒），典型 trading 约 8 秒、web 约 12 秒。已实测不修改工作树。
- [x] Go 全量回归小于 60 秒：实测 55.2 秒（webaccess 测试改为复用缓存哈希并强制 Futu provider，消除每用例 Python sidecar 冷启动；此前 64.1 秒/104.8 秒）。
- [x] Web 全量回归小于 90 秒：`test:web` 改用 `--fileParallelism --maxWorkers=4`，实测 65.8 秒（353 文件 / 2216 测试；此前顺序执行 141–145 秒）。
- [x] servercore 单包测试小于 30 秒：实测 20.9 秒（不包含已迁出的 servercoretest 包；当前机器负载敏感）。
- [x] servercore 生产代码低于 3500 行：已达成（3324 行）；测试低于 9000 行：已达成（8947 行，已从 14991 行下降并锁定 ratchet）；有效方法低于 60：已达成（59）。
- [x] assistant engine 外部直接依赖文件不超过 10 个：已达成（0）；核心 engine 测试低于 20 秒：已达成（本轮实测约 14–17 秒，当前机器负载敏感）。
- [x] 每个硬切独立提交：按用户确认的 7 阶段分组完成（P0 上下文 / P1 门禁 / P1 测试命名 / P2 后端 / P3 Web / P3 Worker+Python / P3 CI），每个提交独立可编译。
- [x] 工作树在任何 `check:*` 后保持干净：提交后 `check:all` 全绿且 `git status --porcelain` 为空；`check:quick`、`test:affected`、`check:generated` 复核均只读。
