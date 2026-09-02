# JFTrade ADK 架构

JFTrade 的 ADK 集成在现有 sidecar 内提供 Agent 控制面，不嵌入 Google ADK 自带 Web UI。生产前端使用 `/adk` 页面和右侧 AI 助手面板。

## 产品定位与使用观测

ADK 是 JFTrade 的核心差异化能力，不是可以随时裁掉的辅助模块。当前保留 workflow 编排、child workflow、execution lease、goal state、approval 和工具幂等完整能力；实现由 `jftrade-assistant`、`jftrade-engine` 与 Rust SQLite stores 持有。

`GET /api/v1/adk/metrics` 在现有运行指标上增加本机滚动 7 日使用窗口，包括 run、session、approval 和 workflow invocation，并同时返回 workflow definition/trigger 的启用数。工具指标还记录输出字节数、最大输出、截断次数、错误数、可重试错误数和稳定错误码分布，不记录原始工具输出。设置页展示近 7 日 ADK 运行和 Workflow 调用，用于发版复盘功能接受度。这些指标只聚合本地 SQLite 中的业务记录，不上传用户数据，也不将“低频使用”自动等同于“应删除”。

## 后端边界

- `crates/jftrade-assistant`：provider、agent、session、run、approval、skill 与 workflow 领域规则和窄 port。
- `crates/jftrade-api`：对外提供 `/api/v1/adk/*` 的 JSON/SSE transport。
- `crates/jftrade-engine`：构造并持有 ADK store/session/artifact、工具目录、workflow/task runtime 与本机 MCP listener；跨领域能力通过 production port 注入。
- `crates/jftrade-store-sqlite`：ADK、session 和 artifact 的 schema、事务与唯一 writer lease。

领域 crate 不向 transport 暴露具体 Store 或 MCP server；`jftrade-engine` 只向 API 注入 Assistant ports、状态/配置动作、broker-neutral workflow 事件和幂等 shutdown handle。

实际执行链使用 ADK Go v2：

- Agent：每次执行通过 `llmagent.New` 从 JFTrade Agent 定义构建。
- Runner：聊天、工具循环和审批恢复通过 `runner.Run` 驱动。
- Workflow Agent：当前对外工作模式是 `chat`、`loop`。`loop` 使用 JFTrade parent/child run facade、执行计划投影和 runtime task toolset 推进目标；公开 `task` 模式已经移除，旧的 `sequential`、`parallel` 和 `task` 请求值不再作为运行模式接收。
- Session：使用 ADK `session/database` 持久化事件；执行真相源是独立的 ADK session SQLite，不再从 JFTrade 历史消息回灌。`adk-session.db` schema 不兼容时会按 v2 结构重建，旧 ADK 原始对话事件不迁移。
- Tool：JFTrade `ToolRegistry` 中经 Agent 白名单和权限模式筛选后的工具直接构造成 ADK 原生 `FunctionTool`。清洗后的 JSON Schema 由原生工具在业务 handler 前严格校验，审批策略通过原生 `RequireConfirmation` 声明；执行租约、幂等、心跳、审计和结果投影仍由产品控制面负责。工具是否执行由 Provider 返回的 function call 决定，后端不再按关键词或 `<execute-tool>` 文本标签做本地工具选择兜底。
- HITL：需要审批的工具使用 ADK `RequireConfirmation`、`RequestConfirmation` 和 `adk_request_confirmation` 协议。模型需要用户做方案决策时使用自动注入的 long-running tool `interaction.request_user`，Run 进入 `PENDING_INPUT`，回答通过原 function-call ID 恢复。原始 ADK workflow `RequestInput` 仍不是公开产品入口；非 `interaction.request_user` 的 requested-input 事件继续返回 `ADK_INPUT_UNSUPPORTED`。
- Model：所有 Provider 统一通过 ADK Go v2.2 原生 `openaimodel` 调用 OpenAI-compatible Responses API。Provider 的 BaseURL、请求超时、默认请求头和 SSRF 防护继续生效；薄 adapter 只负责思考字段映射和工具名的线上安全字符适配/恢复。Agent 必须显式绑定启用状态的 Provider，且该 Provider 必须配置 API Key。不再提供 Chat Completions 分支、本地确定性模型回复或 Provider 不可用时的本地文本兜底。

Run usage 直接消费 ADK Go v2.2 最终、非 partial 事件上的 `UsageMetadata`：按事件 author 归属到 parent 或对应 child run，以事件 ID 去重，并把 prompt/candidate tokens 累加到已有 `tokensIn` / `tokensOut`，每个有效 usage 事件计为一次 `modelCalls`。审批或用户输入恢复从持久化 Run 的累计值继续，不重置历史统计。Responses model 提供该元数据；Provider 没有返回元数据时这些字段保持零值。

公共思考等级固定为 `low`、`medium`、`high`、`xhigh`、`max` 五档。Agent 可以选择一个默认等级，也可以留空表示模型默认；会话覆盖为空表示跟随 Agent，不是一个额外的等级。有效优先级是会话覆盖、Agent 默认、模型默认。Provider 的 `reasoningConfig` 声明支持的公共等级、请求 JSON 点路径和实际枚举值；请求字段默认是 `reasoning.effort`，映射默认均为空。未配置映射表示该 Provider 不支持显式推理等级。

Responses 薄 adapter 注入映射后的请求字段，不经过通用 GenAI `ThinkingConfig`。普通对话、目标规划、子 Agent、循环任务和最终汇总使用各自 Agent/Provider 解析出的等级。Run 在启动时私有持久化公共等级、请求字段和值，审批或用户输入恢复继续使用该快照；公开 Run 响应只返回公共等级。Provider 不支持 Agent 或会话要求的等级时立即返回清晰错误，不静默降级。Provider 健康检查和上下文压缩同样走 Responses，但不发送显式推理字段。

`POST /api/v1/adk/providers/{providerId}/test` 接受可选的 `mode`。缺失或 `quick` 时验证连通性、工具能力和一个代表档位（优先 `medium`，否则按公共顺序取第一个已配置档位）；`full` 时串行验证全部映射并保留逐档结果。无映射时两种模式都不发送推理请求。完整验证会产生额外模型调用，控制台在执行前提示耗时和费用。

普通 Agent 可以编辑全部既有配置；内置 `jftrade-default` 也提供编辑入口，但只允许修改 Provider、覆盖模型和默认思考等级。其名称、指令、工具、技能、审批、记忆、工作模式和启用状态继续由后端保护，且不能删除。启动时会把这些受保护字段同步到当前内置模板，因此旧数据库立即获得最新版策略，同时保留用户选择的 Provider、Model 和 Reasoning Effort。

JFTrade 的 Run、Approval、Audit 和前端 SSE 是产品控制面，不替代 ADK Go v2 的 Agent、Runner、Session 或 Tool 执行语义。本次切换后不再为历史会话或旧 skill 数据提供兼容恢复逻辑。

聊天入口约定：

- `/api/v1/adk/chat`：同步 JSON chat。
- `/api/v1/adk/chat/stream`：SSE 流式 chat。

### 普通 chat 的端到端完成策略

内置默认助手对目标明确的普通 `chat` 必须在同一个 Run 中连续完成诊断、结论和直接相关的可执行方案。安全、只读且能从当前证据推断的后续分析、检查单或计算不能以“是否继续”“想先看哪部分”或“如果需要我可以继续”推迟到下一轮；多个直接服务原始意图的安全分支应采用推荐默认值或合并覆盖。自定义 Agent 和 `loop` 目标模式保持各自配置与终态语义。

`interaction.request_user` 只允许三类真正阻塞边界：`missing_required_context`（缺少用户独有的必要信息）、`material_tradeoff`（存在无法合并的重大取舍）和 `scope_boundary`（继续会越过权限或任务范围）。每次调用必须提交 `decisionKind` 和非空 `blockingReason`；可选下一步、是否继续、先看哪部分都不是合法暂停原因。实际写操作继续使用 Approval，不能用输入问题替代授权。

为防止提示策略遗漏，内置默认助手的普通 `chat` 在终态写入前可以执行一次有界、无工具的完成度复核。只有本轮至少两个只读工具都成功结束、已有非空回复，且没有待输入、待审批、失败、超时、未知副作用或降级终态时才调用；复核沿用本 Run 的 Provider/Model，但不发送显式推理等级，最长 20 秒、最大约 1200 输出 token。复核只看到原始请求、最近一次输入回答、当前回复和工具名称/状态，不接收原始工具结果，也不能获取新事实或执行动作。

复核只接受严格的 `complete` / `append` 结构。仅当 `append` 置信度不低于 `0.85` 且续篇非空、不超过 6000 字符时，才通过现有增量文本通道追加到同一 Run，并把合并后的全文保存为同一个最终消息；低置信度、超时、Provider 错误或解析失败全部 fail-open，保留原回复并正常完成。恢复路径不会重新写入已经关闭的旧 SSE，而是把合并全文持久化后由现有 Run/Session 刷新展示。

审计记录不保存原始对话：输入暂停记录 `decisionKind`；完成度复核记录 `complete`、`append`、`skipped` 或 `failed`、稳定 reason code、耗时和是否追加；同一会话在完成后 10 分钟内只发送 `继续`、`continue` 或 `go on` 时，记录前一 Run ID，作为提前收尾治理指标。

运行时文件默认位于 `var/jftrade-api/`：

- `adk.db`
- `adk-session.db`
- `backups/*.bak`，数据库启动前滚动备份，单库保留最近 3 份
- `secrets/adk-secrets.json`，权限 `0600`
- `adk/skills/`

可用环境变量覆盖：

- `JFTRADE_ADK_DB`
- `JFTRADE_ADK_SESSION_DB`
- `JFTRADE_ADK_SECRETS`
- `JFTRADE_ADK_SKILLS_DIR`

## 跨进程执行租约与工具幂等

每个实际执行中的 Run 都必须先在 `adk_run_leases` 取得持久租约。租约记录 executor owner、心跳时间、过期时间和 fencing token；默认租期为 30 秒、每 10 秒续租。启动恢复、超时扫描、审批继续、用户输入继续和目标恢复遇到其他进程的有效租约时不会接管或把 Run 误判为孤儿。租约过期后的接管会提升 fencing token；携带执行租约的 Run 状态写入会在同一个 SQLite 写事务内校验 token，旧进程的心跳、Run 写入和工具结果提交均会失败。

产品工具的每次实际调用以 Run 和 GO-ADK function-call ID 为稳定身份写入 `adk_tool_invocations`。完成结果会持久化并在同一调用恢复时直接重放；执行中的调用有独立心跳，并同时受 Run fencing token 约束。Tool descriptor 的 `idempotencyMode` 有三种值：

- `replay_safe`：无副作用的读取；调用租约过期后允许安全接管。`read*` permission 默认使用此模式。
- `keyed`：工具必须读取 `ToolInvocationIdempotencyKey(ctx)`，并把该 key 传给外部系统或与副作用一起持久化；运行时会校验工具确实读取过 key，否则把结果标记为未知。调用租约过期后允许使用同一 key 接管。
- `fail_closed`：默认写入模式。执行进程在结果持久化前失联，或写工具返回无法证明无副作用的错误时，调用进入 `INDETERMINATE`，自动恢复会停止并报告结果未知，禁止盲目重放。

因此，当前保证的是跨进程单执行者、旧执行者 fencing、已完成结果重放，以及不具备幂等能力的写操作失败关闭。严格的“崩溃后仍自动完成 exactly-once”只适用于明确声明 `keyed` 且下游真正按该 key 去重的工具；普通写工具发生结果未知时需要业务侧对账，系统不会把“可能重复执行”伪装成成功恢复。

## 权限模式

- `approval`：默认模式。低风险读取自动执行；中风险及以上工具、安装 skill、保存策略、运行优化、工作流管理和工作流启动等动作进入审批。
- `less_approval`：减少普通写入和优化类动作的审批；实盘下单与撤单仍逐次审批，并不会被此模式绕过。
- `all`：内部/外部读取和普通写入尽量自动执行；`live_trading` 仍逐次审批，实际交易还必须通过交易系统自身的实盘开关、风控和熔断。

## 工具访问范围

- `all`：声明当前运行时中符合权限模式的全部工具。
- `selected`：只声明 Agent 明确选择且符合权限模式的工具；空列表保持为空，不再隐式代表全部工具。
- `none`：不向模型声明任何工具，包括 JFTrade 产品工具、ADK memory 工具、Skill toolset 和 artifact 加载工具；记忆持久化配置可以保留，但该 Agent 不能主动调用记忆工具。

## 内置 Tools

当前内置 tools 覆盖（完整目录仍以运行时 `tools.search`/`GET /api/v1/adk/tools` 返回的 descriptor、schema 和 Required Skill 为准）：

- 用户交互：`interaction.request_user`（Registry 内置工具；需要在 Agent 工具清单中启用）
- 系统运维：`system.status`、`system.futu_opend`、`system.runtime_dependencies`、`plugins.catalog`
- 行情：`market.capabilities`、`market.providers`、`market.provider.select`、`market.search`、`market.instrument_profile`、`market.subscriptions`、`market.snapshot`、`market.snapshots`、`market.candles`、`market.intraday`、`market.ticks`、`market.depth`、`market.broker_queue`、`market.capital_flow`、`watchlist.list`、`watchlist.remote.list`、`watchlist.remote.modify`、`alerts.price.list/set`
- 账户与风控：`portfolio.accounts`、`portfolio.overview`、`portfolio.positions`、`portfolio.summary`、`account.orders`、`broker.orders`、`broker.fills`、`broker.cash_flows`、`broker.fees`、`broker.margin_ratios`、`risk.state`、`risk.events`、`execution.order_events`
- 衍生品与预测：`derivatives.option_chain/screen/analysis/events`、`derivatives.warrants`、`derivatives.futures`、`prediction.discover/snapshot/depth/history/combo_eligible/combo_quote`、`alerts.option_event.list/set`
- 研究：`research.instrument`、`research.financials`、`research.valuation`、`research.analyst`、`research.ownership`、`research.corporate_actions`、`research.short_interest`、`research.news`、`research.screen_catalog`、`research.screen`、`research.calendar`、`research.macro`、`research.rankings`、`research.institutions`、`research.industry`、`research.technical_indicators`
- 工作流等待与任务：`workflow.wait`、`tasks.list/create/update/delete`、`memory.list/remember/forget`
- 工作流定义：`workflows.list/get/create/update/delete/run`
- 工作流触发器：`workflow_triggers.list/get/create/update/delete/run`
- 工作流运行：`workflow_runs.list/get/wait`；`workflow_runs.wait` 在服务端按间隔等待，超时返回当前状态和下一次建议轮询间隔，避免模型忙轮询。
- 策略：`strategy.definitions`、`strategy.definition_versions.list/get`、`strategy.pine_spec`、`strategy.validate_pine`、`strategy.research_backtest`、`strategy.save_draft`、`strategy.save_definition`、`strategy.update_instance_mode`、`strategy.instantiate`、`strategy.instance_start`、`strategy.instance_stop`、`strategy.instance_refresh_definition`、`strategy.instance_risk.update`、`strategy.instance_activity`、`strategy.optimize`
- 回测：`backtest.runs`、`backtest.result_view`、`backtest.kline_sync_status`、`backtest.cancel`
- 外部：`http.fetch`（支持 `maxBytes`，响应返回 `finalUrl`、`bytes` 和 `truncated`）

`http.fetch` 允许公网 HTTP/HTTPS，默认阻止本机、私网、link-local、multicast 和 metadata IP，且限制响应大小；它不是本机文件或 Shell 工具。

`interaction.request_user` 只用于上文三类无法从工具或已有上下文消除的阻塞边界。模型必须在一次调用中集中提供当前决策阶段的全部问题；每题必须提供 2 到 3 个选项，可通过 `allowOther` 允许方案外自由输入。同一个 Run 可以在回答并恢复后再次提问，但任一时刻只允许一个待回答请求，不能并行提问。每轮问题、答案和状态都保存在 Run 的 `inputRequests` 历史中；刷新、切换会话或服务重启后仍可恢复。回答恢复时注入的 function response 除 `requestId` 和 `answers` 外还携带 `originalRequest`（原始请求全文）和 `continuationInstruction`（继续完成原始请求的指令），回答只解除阻塞，不代表任务完成。`POST /api/v1/adk/runs/{runId}/input-response` 对每个请求只消费一次有效回答，完全相同的重试幂等，不同的第二次回答返回冲突。

`watchlist.list` 是只读工具：不指定 group 时返回本地分组摘要，指定 group 后按 market、query、cursor/limit 返回成员、来源和最近导入状态。它默认 `includeQuotes=false`，不会触发券商导入或行情订阅；完整参数和数据边界见 [自选系统](watchlist.md)。

组合查询按上下文大小分层：`portfolio.accounts` 只做 live account discovery；`portfolio.overview` 返回逐账户持仓/订单数量和 partial 状态；`portfolio.positions` 返回逐账户持仓明细；需要资金、持仓和订单完整载荷时才使用兼容工具 `portfolio.summary`。所有层级都保留 `accountId + tradingEnvironment + market` 选择事实，不跨账户聚合，也不会把发现失败或部分失败解释成“没有资产”。

工作流管理 tools 复用工作流 Studio 的业务 Service、校验、脱敏和审计。工作流与轻量控制面工具由内置 `jftrade-workflow-management` Skill 提供使用说明；已通过 Agent 白名单和权限筛选的工具从构建开始就会声明给模型，`tools.search` 也会返回这些工具。`load_skill` 只按需加载工作规范和资源；Required Skill 是操作规范关联，不是运行时工具解锁门禁。自定义 Agent 仍须绑定该 Skill 才能读取说明，并至少授权其中一个可用工具。

`update` 使用补丁语义，未提供字段保持不变；列表返回紧凑摘要，`get` 返回完整资源。创建、更新、删除和运行只在 `approval` 模式请求确认，在 `less_approval` 与 `all` 模式直接执行。已进入审批流程的工作流调用可以在后续 invocation 恢复，不要求重新加载 Skill。

`workflows.run` 和 `workflow_triggers.run` 会先持久化 `QUEUED` 运行日志并立即返回；agent 对已知 `logId` 优先使用 `workflow_runs.wait`，未知或需要分页时使用 `workflow_runs.get`、`workflow_runs.list`，不受单次工具调用 30 秒上限影响。只有可解析的普通交互会话能够启动工作流；工作流来源会话禁止再次启动工作流，以避免递归和跨工作流环路。

Webhook secret 不进入模型上下文或 ADK 工具记录。tools 可以查看、启停和编辑已有 Webhook 触发器的非密钥元数据，但不能创建 Webhook 触发器、重置或读取 secret；这些操作仍只通过工作流 Studio/API 完成。

策略内置 skill 已拆分为 `jftrade-strategy-research` 和 `jftrade-strategy-publish`。前者用于临时研究回测、不可变历史版本比较与结果查看，不写入策略定义；后者用于用户明确要求的保存、发布、历史版本恢复、实例模式调整和已保存定义优化。策略 tools 同样按 Agent 白名单和权限在构建时声明，Skill 只提供各自的操作规范和资源。`strategy.validate_pine`、`strategy.definition_versions.list/get`、`backtest.runs` 和 `backtest.kline_sync_status` 由两条流程共享；研究或发布专属工具的说明仍归对应 Skill。历史版本读取只返回不可变快照；恢复必须由用户明确要求，重新校验后通过受审批的 `strategy.save_definition` 以相同 definitionId 创建新版本，不能原地修改历史。旧的 `jftrade-strategy` 不再作为内置 skill 同步。

ADK 发起研究回测或策略优化前会先检查本地 K 线覆盖，并把指标 warmup 纳入检查范围。覆盖不足时自动启动历史数据同步，工具返回 `syncing_data` 和同步 `taskId`，不会提前创建回测 run；skill 使用 `backtest.kline_sync_status` 等待完成后，以相同参数重试原回测工具。同步失败、取消或完成后覆盖仍不足时停止自动重试并返回原因。

回测与策略研究的 `marketDataProvider` 是单次请求覆盖值：任务开始时冻结当前默认回测提供者，数据准备、同步、排队和所有候选运行沿用同一值，不会被并发的全局切换串改。`market.providers` 只返回静态能力和当前激活提供者健康；`market.provider.select` 修改实时或回测默认值，所有权限模式都逐次审批并在激活失败时回滚。yfinance、AKShare 是轮询历史数据源，不提供实时策略流。回测结果、同步状态、`backtest.runs` 和结果视图都保留 provider、图表类型、标的类型、时段、费用和执行模型。

`market.candles` 支持 `sessions`、`beforeTime` 和 `adjustment`，游标与时间范围不能同时出现；`research.screen_catalog` 和 V2 `research.screen` 共享版本化因子目录、归一化、分页、限流和列投影；`research.calendar` 接受市值、期权量、IV、IV Rank 和 IV Percentile 等筛选参数。策略实例工具共用策略 Service 生命周期，启动前会拒绝未知/不健康/不支持实时流的提供者、缺少明确账户的 live 绑定和已满的 Pine Worker。

## Skill 运行时

- Skill 真相源是文件系统中的 `adk/skills/<skill-name>/SKILL.md` 目录树，直接使用 ADK 原生 `skill.NewFileSystemSource` + `skilltoolset`。
- Agent 绑定的是 skill 目录名；模型通过 `list_skills`、`load_skill`、`load_skill_resource` 按需读取说明和资源。
- ADK Go v2 的原生 `skilltoolset` 提供 Skill 指令和资源，不负责产品工具装配；JFTrade 在构建 Agent 时按工具白名单和权限模式过滤业务工具，并把它们作为原生 FunctionTool 声明。`load_skill` 不再维护额外的工具解锁状态。
- `SKILL.md` 使用 ADK 原生 frontmatter：`name`、`description`、`allowed-tools`、`metadata`。
- 不再保留产品级 `enabled` 开关或 Skill 数据库存储；`allowed-tools` 用于校验 Skill 是否能在当前 agent/权限模式下使用，不能绕过 agent 工具白名单。
- 外部 Skill 只提供工作规范与资源目录，不执行任意代码；安装时限制文件大小并阻止不安全主机与文件路径引用。

## API 访问权限

- Tauri 桌面端使用每进程临时能力凭证，无需用户输入密码或 Key。
- Web 默认关闭。用户在桌面设置中开启后，所有 `/api/v1/adk/*` 以及交易、策略、回测、设置和插件 API 都要求 Web 密码会话。
- 浏览器以 Web 访问密码调用 `POST /api/v1/auth/login` 后获得 `HttpOnly`、`SameSite=Strict` 会话；会话默认 12 小时过期。
- cookie 写请求必须来自配置的 GUI Origin，并携带登录或 session 状态接口返回的 `X-CSRF-Token`。
- 不再提供持久 Admin Key、Bearer 管理员旁路或 `/api/v1/auth/token`。外部脚本若通过可选 Web 入口调用，必须走同一密码会话和 CSRF 规则。
- CORS 只回显配置的 GUI/API Origin；缺失 `Origin` 不再被视为可信请求。

Provider 默认允许局域网和本机模型地址，但始终拒绝 link-local、multicast、未指定地址以及云 metadata 地址。每次连接和重定向都会重新解析并校验目标地址，且不使用环境 HTTP 代理。

## 本机 MCP Server

JFTrade 可作为本机 MCP Server，使用 `github.com/modelcontextprotocol/go-sdk v1.7.0` 提供无状态 Streamable HTTP transport。该服务默认关闭，在“设置 → 智能体 → MCP 服务”中启用；默认端点为 `http://127.0.0.1:6697/mcp`。

- 仅绑定 `127.0.0.1`，不提供 stdio、局域网或公网监听。即使选择“无 Token”，也仍只接受本机连接。
- 默认使用 Bearer Token 鉴权。Token 只在生成或重置响应中显示一次；设置文件只保存不可逆校验值。重置后旧 Token 会立即失效。
- 服务使用无状态 Streamable HTTP，仅接受 `POST`，不依赖 `Mcp-Session-Id`；`GET`、`DELETE` 返回 `405`，并保留 SDK 的 localhost Host 防护。
- 仅公开经过固定白名单审核的只读工具：系统、行情、账户与风险、策略读取和回测读取工具。交易、写入、HTTP 抓取、Agent/Skill/任务/记忆管理工具不会出现在 `tools/list` 中。
- 客户端可读取 `jftrade://runtime/status` JSON 资源；内容只包含脱敏的运行时、Provider 摘要、Agent、Skill 和工具目录，不包含 API Key。资源支持订阅，运行时工具目录变化会发送 `tools/list_changed` 和 `resources/updated` 通知。

通用 MCP 客户端配置示例：

```json
{
  "mcpServers": {
    "jftrade": {
      "type": "streamable-http",
      "url": "http://127.0.0.1:6697/mcp",
      "headers": {
        "Authorization": "Bearer <MCP_TOKEN>"
      }
    }
  }
}
```

使用无 Token 模式时删除 `headers`。服务设置可通过 `GET` / `PUT /api/v1/settings/adk/mcp` 读取或更新，`POST /api/v1/settings/adk/mcp/token/reset` 生成新的 Token。

## Run 与优化任务

- Run 支持 `RUNNING`、`PENDING_APPROVAL`、`COMPLETED`、`FAILED`、`DENIED`、`CANCELLED`、`TIMED_OUT`。
- 多审批 Run 只有在全部批准后才执行写工具；任一拒绝会终止其余待执行动作。
- `POST /api/v1/adk/runs/{runId}/cancel` 可取消运行中、等待审批或等待用户回答的 Run。
- `strategy.research_backtest` 使用临时 Pine 脚本启动研究回测，不保存策略定义；异步未完成时可短暂调用 `workflow.wait` 后用 `backtest.result_view` 分片查看摘要、蜡烛、交易、日志或错误。
- `strategy.optimize` 会为候选策略定义创建真实异步回测，并通过 `/api/v1/adk/optimization-tasks/*` 查询或取消。
- `/api/v1/adk/audit` 和 `/api/v1/adk/metrics` 提供审计记录与基础运行指标。

## 工作模式

JFTrade 的非 chat 工作模式不替代 ADK Go v2 的 Agent、Runner、Session、Tool 或 HITL 执行语义。每个 Agent 可配置默认工作模式，聊天请求也可以临时覆盖。当前后端只接受 `chat`、`loop`；历史 `sequential`、`parallel`、`task` 作为请求覆盖值会被拒绝。

- `chat`：默认单轮对话，完全复用现有执行链。
- `loop`：按最大轮次推进目标；遇到审批、失败、超时、取消或轮次上限会停止。

目标模式使用 `workflow.task.*` 工具维护内部 TODO/DAG 数据，必要时委派子智能体或继续下一轮。普通用户 agent 不直接获得 workflow planner 控制面工具；目标模式中的 task 数据只作为产品层观察和推进状态，不代表公开 `task` 工作模式。

workflow 父 run 会保存 `workMode`、`objective`、`childRunIds`、`iteration`、`workflowPlan` 和 `workflowStatus`，用于前端观察与取消；实际工具调用、审批记录和审批恢复仍属于触发工具的 child run，不合并回 parent run。目标步骤会投影到 `adk_tasks`，便于在 Settings 的工作流观察页查看任务、依赖和关联 run；task payload 保留 `order`、`modeHint`、`agentRole`、`plannerStepId`、`planSource`、`workflowMode`、`objective` 和 planner warnings，作为产品层 DAG/provenance 观察数据，不替代 ADK Go v2 Session / Runner / Agent tree 的执行语义。

## 前端入口

- `/adk`：Provider、Agent、Skill、会话、审批和运行记录工作台。
- 右侧 AI 助手：调用 `/api/v1/adk/chat/stream`，与 `/adk` 页面共享相同的运行、工具和终态失败展示语义。

workflow UI 是产品层投影，不改变 ADK Go v2 的执行语义。`/adk` 页面和右侧 AI 助手会在输入框上方按“待审批、子智能体、执行计划、输入框”的顺序显示紧凑队列；执行计划来自 parent run 的 `workflowPlan`，子智能体等同 workflow child run，审批队列聚合当前会话的 parent/child pending approvals。child view 只用于观察 child timeline 和处理审批，不允许直接向 child run 追加新用户消息。

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

ADK Go v2.2.0 的能力审计按“原生机制存在”与“JFTrade 产品语义可等价替换”分开判断：

| 能力 | v2.2 原生机制 | JFTrade 等价性要求与结论 |
| --- | --- | --- |
| workflow graph | `workflow.Workflow`、`workflowagent`、静态/动态节点 | 原生图已用于底层执行，但不表达 run lease、parent/child run、计划投影、图指纹与 SSE；保留产品控制面。 |
| resume | `Workflow.Resume`、`RequestInput`、按 interrupt ID 恢复 | 原生 round-trip 与事件顺序由 `adk22regression` 锁定；工具审批、`interaction.request_user`、恢复前裁剪和失败投影仍经 JFTrade adapter。 |
| tool confirmation | `RequireConfirmation`、`adk_request_confirmation` | 已原生化工具确认协议；请求顺序、审批持久化、异步恢复、取消和审计继续由 JFTrade 回归测试保护。 |
| session | `session.Service` 与 `session/database` | 已使用原生 database service；JFTrade wrapper 只负责 SQLite 连接、schema 校验、备份/维护和关闭，重启恢复由 `session_sqlite_test.go` 锁定。 |
| artifact | `artifact.Service`，内存与 GCS 实现 | 本地工作台需要 SQLite、版本、维护和重启恢复，v2.2 没有等价 SQLite 实现；保留实现该原生接口的 JFTrade adapter。 |
| memory | `memory.Service`，内存与 Vertex AI 实现 | JFTrade 需要本地 workspace/agent scope、排序与既有 CRUD；保留实现原生接口的本地 adapter，不迁移到云服务。 |
| plugin | Runner `PluginConfig` 与生命周期 callbacks | 能提供 hook，但不能替代 run lease、父子运行、计划、审批/输入状态、SSE、调度、审计和指标；只保留现有窄 plugin 使用，不把产品控制面改写为 plugin。 |

原生化判定必须同时覆盖恢复结果、审批状态、事件顺序、SQLite 重启、取消与父子运行。单项存在原生 API 或上游单元测试，不构成删除 JFTrade 控制面的依据；每个可替换项必须在独立提交中先增加等价回归，再移除对应胶水。

- JFTrade 的 `adk_sessions` / `adk_messages` 仍作为前端列表与最终消息投影视图使用，但不再是执行真相源。
- 目标模式、parent/child run、审批队列、执行计划和 child view 都是 JFTrade 产品层投影。
- ADK Go v2.2 的 `workflowagent.Config` 不能传入 workflow 并发选项，也只识别原生 `RequestInput` 恢复。JFTrade 暂时保留薄 workflow agent adapter，以维持 `WithMaxConcurrency`、工具审批响应、invocation 回退和恢复前会话裁剪；后续只有在原生入口能够等价表达这些语义时才移除。
- Workflow task 工具是内部控制面工具，用于维护目标推进 TODO/DAG，不作为公开 task 模式或直接调用 child agent 的兼容层。
- Provider tool calling 是工具执行的唯一入口；业务工具在构建 Agent 时按白名单和权限筛选，并始终声明给模型。`load_skill` 只加载原生 Skill 指令和资源，不再动态解锁业务工具。后端保留权限、审批、审计和投影控制面，并把 ADK Go v2 confirmation 与 `interaction.request_user` long-running 事件分别投影到 JFTrade Approval、InputRequest 和 timeline；其他 requested-input 事件仍直接失败为 `ADK_INPUT_UNSUPPORTED`。
- Optimization task、Run/Audit 展示、前端 SSE 和审批列表都属于 JFTrade 产品控制面，而不是 ADK Go v2 自带控制面。

## 验证

```bash
cargo test -p jftrade-assistant -p jftrade-engine -p jftrade-store-sqlite --all-targets
pnpm run typecheck:web
pnpm run build:web
```
