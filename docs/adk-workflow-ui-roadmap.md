# ADK Workflow UI 队列化改造路线图

## Summary

本次改造把 `/adk` 对话页和右侧 dock 助手的输入框上方升级成 Codex 风格的紧凑工作区。权限审批、子智能体/child run、执行计划都用类似消息发送队列的折叠条展示，默认不打断主对话流，用户需要时可展开、滚动、进入子视图。

最终布局顺序固定为：权限审批队列、子智能体队列、执行计划队列、输入框。审批优先级最高；三类队列都只在当前会话有对应数据时显示。

## Key Changes

- 新增共享队列组件族：`ADKQueuePanel`、`ADKWorkflowQueuePanel`、`ADKChildRunQueuePanel`、`ADKApprovalQueuePanel`。
- 新增共享状态层 `useADKWorkflowQueueState`，维护最近一次 workflow plan、child run 快照、审批队列和当前 child view。
- child run 快照由 parent run 的 `childRunIds` 与 `workflowPlan[].childRunId` 推导，并用现有 `GET /api/v1/adk/runs/{runId}` 刷新。
- `ADKChatThread` 不再渲染可操作 approval card；审批操作统一迁移到队列。
- child view 在原页面或 dock 内切换，不新开路由；主 timeline 按 child run id 过滤，composer 禁止直接向 child run 发消息。

## UI Rules

- 队列默认折叠，只显示标题、数量、最高优先状态和前 1 条摘要。
- 展开后最多显示约 10 条高度，超过后内部滚动。
- 状态色统一：`TODO` muted、`IN_PROGRESS/RUNNING` info、`BLOCKED/PENDING_APPROVAL` warning、`DONE/COMPLETED` success、失败类 error。
- 空队列不展示；有 pending approval 时审批队列展示 warning 摘要但仍默认折叠。

## Scope

- v1 “子智能体”等同 workflow child run，不引入真正多 Agent 协作模型。
- v1 child view 只做观察和审批，不允许直接追加用户消息。
- child timeline 优先使用现有 session timeline 按 run id 过滤；不新增后端 run timeline API。
- `/adk` 页面和右侧 dock 助手同步显示三类队列与 child drill-down。
