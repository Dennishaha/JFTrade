# API 端点生命周期

本文档是 HTTP API 端点的治理清单：哪些端点处于废弃/兼容状态、谁在用、何时可以删。
契约层面的强制约束由代码和测试保证（见下文「硬性门禁」），本表是人工维护的决策记录。

## 生命周期状态

| 状态 | 含义 | 对外表现 |
|---|---|---|
| `active` | 正常维护的端点 | swagger 正常展示 |
| `deprecated` | 仍可用，但有替代端点；新调用方不得使用 | swagger 标记 `@Deprecated`，响应头 `Deprecation: true` + `Link: <替代端点>; rel="successor-version"` |
| `tombstone` | 功能已移除，仅保留兼容壳告知调用方 | 恒返 410 Gone，swagger 标记 `@Deprecated` |
| `removed` | 端点删除 | 路由不存在，走统一 404 |

迁移规则：

1. `active → deprecated`：需要在本表登记替代端点与废弃日期，OpenAPI 标记和 Rust transport deprecation middleware 同时落地。
2. `deprecated → removed/tombstone`：至少经过一个发布版本的兼容窗口，并用请求观测数据（`requestObservabilityMiddleware` 记录每个请求的 method+path+status）确认无活跃调用方。
3. 删除时同步清理：路由、swagger 注解、前端 `generated/openapi.ts` 引用、本表条目。

## 硬性门禁

以下检查在 CI 中强制执行，不依赖本表：

- `pnpm run check:route-contracts`：从 OpenAPI 与 Rust 实际注册目录构造集合，逐条核对 278 条路由的 path、method 与认证策略。
- `pnpm run check:openapi-quality`：写请求、schema、operationId 和公开错误面满足中立 OpenAPI 规则。
- `pnpm run check:generated`：`contracts/openapi/openapi.json`、Web 类型和 reference 文档在临时目录生成并逐字节比较。
- `pnpm run check:rust:production-policy`：生产 route assembly 不允许 forced registration、fallback 或 synthetic success。

## 当前 deprecated / tombstone 端点

当前没有 deprecated 或 tombstone 端点。2026-07-24 的严格契约清理已删除旧 execution preview、`data-migration` 别名和 ADK skill PUT 路由；这些 URL 统一返回 404。

## 无前端调用但保留的端点

以下端点没有 Web UI 直接调用，但属于有意保留的对外面，**不应**按「死接口」删除：

| 分组 | 端点 | 保留原因 |
|---|---|---|
| 能力目录 | `GET /alerts/price`、`GET /alerts/option-events`、`/watchlists/remote`、`/brokers/{id}/quote|securities|klines`、`/execution/buying-power`、`/research/technical-indicators/{id}` 等 | 进入 Rust broker capability catalog 和 route manifest，供 ADK 工具、MCP 客户端和外部 sidecar 使用 |
| ADK 兼容面 | `POST /adk/chat`（非流式）、`GET /adk/optimization-tasks/{taskId}` | ADK/MCP 客户端使用；前端只用流式和列表接口 |
| 批量运维 | `POST /system/exchange-calendars/refresh`、`POST /system/exchange-calendars/probe`（全市场批量版） | 运维入口，前端只调按市场版本 |

新增此类端点时，请在本表登记保留原因，避免后续审计误判。
