# Assistant 局部指令

- `internal/assistant` 暴露 Assistant 业务契约；`assembly` 负责跨域工具投影和 ADK/MCP 生命周期；`engine` 只实现运行时基础设施。
- 共享 DTO 与状态常量放在 `internal/assistant/model`；业务层和 transport 直接 import `model` 或 `engine/workflowruntime`，不再直接依赖 `engine` 根包。
- Assistant 不得依赖 `internal/app`、`internal/api`、具体 store、integration 或 HTTP transport。
- 共享 DTO 放在无环的 model/contracts 层；不要让 API DTO、ADK 类型和 SQLite 行模型互相泄漏。
- 工具调用、审批、输入请求、超时和取消都是可观察业务终态；不能把它们粗暴转换为 transport error。
- 最小验证：`go test ./internal/assistant/... -count=1`、对应 `internal/api/assistant` 测试。
