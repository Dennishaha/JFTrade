# JFTrade ADK 架构

JFTrade 的 ADK 集成在现有 sidecar 内提供 Agent 控制面，不嵌入 Google ADK 自带 Web UI。生产前端使用 `/adk` 页面和右侧 AI 助手面板。

## 后端边界

- `pkg/adk`：独立 ADK 包装层，保存 provider、agent、session、run、approval、skill，并适配 `google.golang.org/adk`。
- `internal/api/assistant`：对外提供 `/api/v1/adk/*` 的 JSON/SSE transport。
- `internal/assistant.Service`：封装 session、run、approval、provider、agent、
  skill、observability 与 optimization 业务门面。
- `internal/app/apiserver/servercore/adk_runtime.go`：把 JFTrade 内部资源注册为 ADK tools，避免通过 HTTP 回环调用自身。

实际执行链使用 GO-ADK：

- Agent：每次执行通过 `llmagent.New` 从 JFTrade Agent 定义构建。
- Runner：聊天、工具循环和审批恢复通过 `runner.Run` 驱动。
- Workflow Agent：当前对外工作模式是 `chat`、`task`、`loop`。`task`/`loop` 会先由内部 Planner 生成结构化 plan，再编译为固定 GO-ADK workflow agent tree；旧的 `sequential`、`parallel` 请求值不再作为运行模式接收。
- Session：使用 ADK `session/database` 持久化事件；执行真相源是独立的 ADK session SQLite，不再从 JFTrade 历史消息回灌。
- Tool：JFTrade `ToolRegistry` 中的工具会包装为 ADK Function Tool，并由 Runner 调用；工具是否执行由 Provider 返回的 tool/function call 决定，后端不再按关键词或 `<execute-tool>` 文本标签做本地工具选择兜底。
- HITL：需要审批的工具使用 ADK `RequestConfirmation` 和 `adk_request_confirmation` 协议。原始 function-call ID 与 confirmation-call ID 持久化到审批记录；只有保留完整 confirmation 上下文的 `PENDING_APPROVAL` run 会在服务重启后继续恢复，普通 `RUNNING` run 会标记为 orphaned/failed。
- Model：Provider 通过 ADK `model.LLM` 适配器调用 OpenAI-compatible `/chat/completions`；Agent 必须显式绑定启用状态的 Provider，且该 Provider 必须配置 API Key。不再提供本地确定性模型回复或 Provider 不可用时的本地文本兜底。

JFTrade 的 Run、Approval、Audit 和前端 SSE 是产品控制面，不替代 GO-ADK 的 Agent、Runner、Session 或 Tool 执行语义。本次切换后不再为历史会话或旧 skill 数据提供兼容恢复逻辑。

聊天入口约定：

- `/api/v1/adk/chat`：同步 JSON chat。
- `/api/v1/adk/chat/stream`：SSE 流式 chat。

运行时文件默认位于 `var/jftrade-api/`：

- `adk.db`
- `adk-session.db`
- `backups/*.bak`，数据库启动前滚动备份，单库保留最近 3 份
- `secrets/adk-secrets.json`，权限 `0600`
- `secrets/admin.key`，单管理员密钥，权限 `0600`
- `adk/skills/`

可用环境变量覆盖：

- `JFTRADE_ADK_DB`
- `JFTRADE_ADK_SESSION_DB`
- `JFTRADE_ADK_SECRETS`
- `JFTRADE_ADK_SKILLS_DIR`

## 权限模式

- `approval`：默认模式。读内部/外部资源自动执行；安装 skill、保存策略、运行优化等写动作进入审批。
- `sandbox_auto`：允许沙盒内自动执行优化和草稿类动作；live/real 策略仍不自动启动。
- `high_auto`：允许更多自动化控制面动作；实盘交易工具未开放，未来也必须复用现有风控和熔断。

## 内置 Tools

当前内置 tools 覆盖：

- 系统：`system.status`、`system.futu_opend`、`plugins.catalog`
- 行情：`market.subscriptions`、`market.snapshot`、`market.candles`
- 账户：`portfolio.summary`、`account.orders`
- 策略：`strategy.definitions`、`strategy.pine_spec`、`strategy.validate_pine`、`strategy.save_draft`、`strategy.save_definition`、`strategy.update_instance_mode`、`strategy.optimize`
- 回测：`backtest.runs`
- 外部：`http.fetch`

`http.fetch` 允许公网 HTTP/HTTPS，默认阻止本机、私网、link-local、multicast 和 metadata IP，且限制响应大小。

## Skill 运行时

- Skill 真相源是文件系统中的 `adk/skills/<skill-name>/SKILL.md` 目录树，直接使用 ADK 原生 `skill.NewFileSystemSource` + `skilltoolset`。
- Agent 绑定的是 skill 目录名；模型通过 `list_skills`、`load_skill`、`load_skill_resource` 按需读取说明和资源。
- `SKILL.md` 使用 ADK 原生 frontmatter：`name`、`description`、`allowed-tools`、`metadata`。
- 不再保留产品级 `enabled` 开关、Skill 数据库存储、或 “Skill.Tools 与 Agent.Tools 取交集” 的旧规则。
- 外部 Skill 只提供工作规范与资源目录，不执行任意代码；安装时限制文件大小并阻止不安全主机与文件路径引用。

## API 管理员权限

- 所有 `/api/v1/adk/*` 以及交易、策略、回测、设置和插件敏感 API 都要求管理员认证。
- 浏览器调用 `POST /api/v1/auth/login` 后获得 `HttpOnly`、`SameSite=Strict` 会话；会话默认 12 小时过期。
- cookie 写请求必须来自配置的 GUI Origin，并携带登录或 session 状态接口返回的 `X-CSRF-Token`。
- CLI 使用 `Authorization: Bearer <管理员密钥>`，不需要 CSRF。`/api/v1/auth/token` 返回 `410 Gone`。
- CORS 只回显配置的 GUI/API Origin；缺失 `Origin` 不再被视为可信请求。

Provider 默认允许局域网和本机模型地址，但始终拒绝 link-local、multicast、未指定地址以及云 metadata 地址。每次连接和重定向都会重新解析并校验目标地址，且不使用环境 HTTP 代理。

## Run 与优化任务

- Run 支持 `RUNNING`、`PENDING_APPROVAL`、`COMPLETED`、`FAILED`、`DENIED`、`CANCELLED`、`TIMED_OUT`。
- 多审批 Run 只有在全部批准后才执行写工具；任一拒绝会终止其余待执行动作。
- `POST /api/v1/adk/runs/{runId}/cancel` 可取消运行中或等待审批的 Run。
- `strategy.optimize` 会为候选策略定义创建真实异步回测，并通过 `/api/v1/adk/optimization-tasks/*` 查询或取消。
- `/api/v1/adk/audit` 和 `/api/v1/adk/metrics` 提供审计记录与基础运行指标。

## 工作模式

JFTrade 的非 chat 工作模式使用 GO-ADK 原生 workflow agents 执行，不替代 GO-ADK 的 Agent、Runner、Session、Tool 或 HITL 执行语义。每个 Agent 可配置默认工作模式，聊天请求也可以临时覆盖。当前后端只接受 `chat`、`task`、`loop`；历史 `sequential`、`parallel` 会在迁移时归一到 `chat`，作为请求覆盖值会被拒绝。

- `chat`：默认单轮对话，完全复用现有执行链。
- `task`：先由内部 Planner `LlmAgent` 通过 `workflow.plan.*` 工具生成任务 plan，再创建对应 task/child run，并按已校验步骤执行。
- `loop`：Planner 生成目标推进步骤后，按最大轮次推进目标；遇到审批、失败、超时、取消或轮次上限会停止。

Planner 阶段只负责产出结构化 plan，不直接启动业务子智能体，也不暴露给普通用户 agent。后端会校验 `order` 和 `dependsOn` 形成的 DAG，并在落库时把 planner 内部依赖映射为真实 `adk_tasks` 依赖；若 Planner 未调用 `finish`、产物为空、依赖非法、循环依赖或无法映射到当前工作模式，本次 workflow 直接失败并记录原因。执行阶段不动态修改 `SubAgents`，而是把已校验的 plan 编译成固定 ADK agent tree，让顺序、并行和循环调度交给 GO-ADK workflow agents。

workflow 父 run 会保存 `workMode`、`objective`、`childRunIds`、`iteration`、`workflowPlan` 和 `workflowStatus`，用于前端观察与取消；实际工具调用、审批记录和审批恢复仍属于触发工具的 child run，不合并回 parent run。工作流步骤会投影到 `adk_tasks`，便于在 Settings 的工作流观察页查看任务、依赖和关联 run；task payload 额外保留 `order`、`modeHint`、`agentRole`、`plannerStepId`、`planSource`、`workflowMode`、`objective` 和 planner warnings，作为产品层 DAG/provenance 观察数据，不替代 GO-ADK Session / Runner / Agent tree 的执行语义。

## 前端入口

- `/adk`：Provider、Agent、Skill、会话、审批和运行记录工作台。
- 右侧 AI 助手：调用 `/api/v1/adk/chat/stream`，与 `/adk` 页面共享相同的运行、工具和终态失败展示语义。

workflow UI 是产品层投影，不改变 GO-ADK 的执行语义。`/adk` 页面和右侧 AI 助手会在输入框上方按“待审批、子智能体、执行计划、输入框”的顺序显示紧凑队列；执行计划来自 parent run 的 `workflowPlan`，子智能体等同 workflow child run，审批队列聚合当前会话的 parent/child pending approvals。child view 只用于观察 child timeline 和处理审批，不允许直接向 child run 追加新用户消息。

## ADK 聊天与审批前端交互约定

这部分是回归保护规则，修改 `/adk` 页面、右侧 AI 助手、审批队列或运行轨迹时必须优先遵守。

- 工具调用失败、run 超时、run 取消或审批拒绝都属于业务终态。调用方应收到正常的终态 `ChatResponse` / SSE `final`，并从 `run.status`、`run.failureReason`、`run.errorCode` 与 `toolCalls[].error` 读取失败信息；不要把这类场景当成传输层错误。
- 只有请求体非法、Agent/Session 前置校验失败、Agent 未绑定可用 Provider、Provider 未配置 API Key、运行时不可用、SSE 不支持，或流式连接在没有终态结果时中断，才应该返回 HTTP 错误或 SSE `error` 事件。

- 已经展示给用户的 assistant 文本不能被后续 SSE、run snapshot、final response 或工具进度覆盖掉；最终响应只能补齐、归一或追加新内容，不能用 `preToolContent` 或 final reply 的差异直接清空已渲染内容。
- 工具调用期间的进度、审批状态和后续模型输出必须是增量式呈现；如果模型先输出文字、再调用工具、再继续输出文字，前面已经出现的文字仍要保留在聊天记录中。
- 同一次会话中的多轮工具调用不能被前端合并成一个“已调用 N 个工具”的单一摘要。工具调用应按后端 run snapshot 中的顺序稳定追加展示：先出现 2 个就先展示 2 个，之后又出现 4 个就继续追加 4 个。
- 工具调用展示可以折叠单个工具详情，但折叠粒度必须是单个调用或一次明确的调用批次，不能把不同时间发生的调用压扁成同一个不可区分的卡片。
- 前端批准或拒绝审批后，请求只负责提交审批决议并刷新/轮询 run 状态；审批接口不应等待被批准工具和后续模型执行全部完成后才返回。
- 审批失败必须在前端明确提示后端错误信息，包括 `ADK_APPROVAL_RESOLVE_FAILED`、`SQLITE_BUSY` 等可诊断错误，不能静默中断或误提示“请先在 ADK 审批队列里确认”。

## 当前非原生 ADK 边界

- JFTrade 的 `adk_sessions` / `adk_messages` 仍作为前端列表与最终消息投影视图使用，但不再是执行真相源。
- 动态 workflow planning、parent/child run、审批队列、执行计划和 child view 都是 JFTrade 产品层投影；底层 `task`/`loop` 执行由 GO-ADK workflow agents 驱动。
- Planner 工具是内部控制面工具，用于把用户目标转成固定 agent tree，不作为运行期动态创建 agent 或直接调用 child agent 的机制。
- Provider tool calling 是工具执行的唯一入口；后端保留权限、审批、审计和投影控制面，但不再本地猜测用户意图并主动插入工具调用。
- Optimization task、Run/Audit 展示、前端 SSE 和审批列表都属于 JFTrade 产品控制面，而不是 GO-ADK 自带控制面。

## 验证

```bash
go test ./internal/assistant ./internal/api/assistant ./internal/app/apiserver/servercore ./pkg/adk
npm --workspace @jftrade/web run typecheck
npm --workspace @jftrade/web run build
```
