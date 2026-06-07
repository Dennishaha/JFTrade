# JFTrade ADK 架构

JFTrade 的 ADK 集成在现有 sidecar 内提供 Agent 控制面，不嵌入 Google ADK 自带 Web UI。生产前端使用 `/adk` 页面和右侧 AI 助手面板。

## 后端边界

- `pkg/adk`：独立 ADK 包装层，保存 provider、agent、session、run、approval、skill，并适配 `google.golang.org/adk`。
- `pkg/jftradeapi/adk_routes.go`：对外提供 `/api/v1/adk/*` 和兼容 `/api/v1/assistant/chat`。
- `pkg/jftradeapi/adk_runtime.go`：把 JFTrade 内部资源注册为 ADK tools，避免通过 HTTP 回环调用自身。

实际执行链使用 GO-ADK：

- Agent：每次执行通过 `llmagent.New` 从 JFTrade Agent 定义构建。
- Runner：聊天、工具循环和审批恢复通过 `runner.Run` 驱动。
- Session：使用 ADK `session/database` 持久化事件；执行真相源是独立的 ADK session SQLite，不再从 JFTrade 历史消息回灌。
- Tool：JFTrade `ToolRegistry` 中的工具会包装为 ADK Function Tool，并由 Runner 调用。
- HITL：需要审批的工具使用 ADK `RequestConfirmation` 和 `adk_request_confirmation` 协议。原始 function-call ID 与 confirmation-call ID 持久化到审批记录；只有保留完整 confirmation 上下文的 `PENDING_APPROVAL` run 会在服务重启后继续恢复，普通 `RUNNING` run 会标记为 orphaned/failed。
- Model：Provider 通过 ADK `model.LLM` 适配器调用 OpenAI-compatible `/chat/completions`；未配置 Provider 时使用本地确定性模型执行测试和降级流程。

JFTrade 的 Run、Approval、Audit 和前端 SSE 是产品控制面，不替代 GO-ADK 的 Agent、Runner、Session 或 Tool 执行语义。本次切换后不再为历史会话或旧 skill 数据提供兼容恢复逻辑。

聊天入口约定：

- `/api/v1/adk/chat`：同步 JSON chat。
- `/api/v1/adk/chat/stream`：SSE 流式 chat。
- `/api/v1/assistant/chat`：保留给历史调用方的兼容 JSON 入口，不再承担流式协议。

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
- 策略：`strategy.definitions`、`strategy.save_draft`、`strategy.optimize`
- 回测：`backtest.runs`
- 外部：`http.fetch`

`http.fetch` 允许公网 HTTP/HTTPS，默认阻止本机、私网、link-local、multicast 和 metadata IP，且限制响应大小。

## Skill 运行时

- Skill 真相源是本地 `adk/skills/<skill-name>/SKILL.md` 目录树，直接使用 ADK 原生 `skill.NewFileSystemSource` + `skilltoolset`。
- Agent 绑定的是 skill 目录名；模型通过 `list_skills`、`load_skill`、`load_skill_resource` 按需读取说明和资源。
- `SKILL.md` 使用 ADK 原生 frontmatter：`name`、`description`、`allowed-tools`、`metadata`。
- 不再保留产品级 `enabled` 开关、Skill 数据库存储、或 “Skill.Tools 与 Agent.Tools 取交集” 的旧规则。
- 外部 Skill 只提供工作规范与资源目录，不执行任意代码；安装时限制文件大小并阻止不安全主机与本地路径引用。

## API 管理员权限

- 所有 `/api/v1/adk/*` 以及交易、策略、回测、设置和插件敏感 API 都要求管理员认证。
- 浏览器调用 `POST /api/v1/auth/login` 后获得 `HttpOnly`、`SameSite=Strict` 会话；会话默认 12 小时过期。
- cookie 写请求必须来自配置的 GUI Origin，并携带登录或 session 状态接口返回的 `X-CSRF-Token`。
- CLI 使用 `Authorization: Bearer <管理员密钥>`，不需要 CSRF。`/api/v1/auth/token` 返回 `410 Gone`。
- CORS 只回显配置的 GUI/API Origin；缺失 `Origin` 不再被视为本地请求。

Provider 默认允许局域网和本机模型地址，但始终拒绝 link-local、multicast、未指定地址以及云 metadata 地址。每次连接和重定向都会重新解析并校验目标地址，且不使用环境 HTTP 代理。

## Run 与优化任务

- Run 支持 `RUNNING`、`PENDING_APPROVAL`、`COMPLETED`、`FAILED`、`DENIED`、`CANCELLED`、`TIMED_OUT`。
- 多审批 Run 只有在全部批准后才执行写工具；任一拒绝会终止其余待执行动作。
- `POST /api/v1/adk/runs/{runId}/cancel` 可取消运行中或等待审批的 Run。
- `strategy.optimize` 会为候选策略定义创建真实异步回测，并通过 `/api/v1/adk/optimization-tasks/*` 查询或取消。
- `/api/v1/adk/audit` 和 `/api/v1/adk/metrics` 提供审计记录与基础运行指标。

## 前端入口

- `/adk`：Provider、Agent、Skill、会话、审批和运行记录工作台。
- 右侧 AI 助手：调用 `/api/v1/adk/chat`，失败时保留本地兜底回答。

## 当前非原生 ADK 边界

- JFTrade 的 `adk_sessions` / `adk_messages` 仍作为前端列表与最终消息投影视图使用，但不再是执行真相源。
- Optimization task、Run/Audit 展示、前端 SSE 和审批列表都属于 JFTrade 产品控制面，而不是 GO-ADK 自带控制面。

## 验证

```bash
go test ./pkg/adk ./pkg/jftradeapi
npm --workspace @jftrade/web run typecheck
npm --workspace @jftrade/web run build
```
