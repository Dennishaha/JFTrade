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

1. `active → deprecated`：需要在本表登记替代端点与废弃日期，swagger 注解和 `httpserver.Deprecated` 中间件同时落地。
2. `deprecated → removed/tombstone`：至少经过一个发布版本的兼容窗口，并用请求观测数据（`requestObservabilityMiddleware` 记录每个请求的 method+path+status）确认无活跃调用方。
3. 删除时同步清理：路由、swagger 注解、前端 `generated/openapi.ts` 引用、本表条目。

## 硬性门禁

以下检查在 CI 中强制执行，不依赖本表：

- `TestOpenAPICoversRegisteredAPIRoutes`（servercore）：所有已注册路由必须出现在 OpenAPI 契约中，无豁免。
- `TestOpenAPIDocumentsWritableRequestBodies`：写操作的请求体必须是 typed DTO。
- `TestOpenAPISpecStable`：契约快照与 `tests/fixtures/openapi-baseline.json` 一致（有意修改时用 `UPDATE_OPENAPI_SNAPSHOT=1` 更新）。
- CI `Verify tracked contract artifacts are up to date`：`apps/web/src/generated/openapi.ts` 与 swagger 注解同步。

## 当前 deprecated / tombstone 端点

| 端点 | 状态 | 废弃日期 | 替代端点 | 删除条件 |
|---|---|---|---|---|
| `POST /api/v1/execution/orders/preview` | deprecated | 2026-07-19 | `POST /api/v1/execution/previews` | 兼容窗口后无观测调用 |
| `GET /api/v1/settings/data-migration/databases` | deprecated | 2026-07-19 | `GET /api/v1/settings/data-management/databases` | 同上 |
| `POST /api/v1/settings/data-migration/databases/rebuild` | deprecated | 2026-07-19 | `POST /api/v1/settings/data-management/databases/rebuild` | 同上 |
| `PUT /api/v1/adk/skills/{skillId}` | tombstone (410) | 2026-07-19 | 在 agent 上直接绑定技能 | 兼容窗口后删除路由与注解 |

## 无前端调用但保留的端点

以下端点没有 Web UI 直接调用，但属于有意保留的对外面，**不应**按「死接口」删除：

| 分组 | 端点 | 保留原因 |
|---|---|---|
| 能力目录 | `GET /alerts/price`、`GET /alerts/option-events`、`/watchlists/remote`、`/brokers/{id}/quote|securities|klines`、`/execution/buying-power`、`/research/technical-indicators/{id}` 等 | 进入 broker capability catalog（`TestCapabilityCatalogAPISurfacesAreRegistered` 强制路由存在），供 ADK 工具、MCP 客户端和外部 sidecar 使用 |
| ADK 兼容面 | `POST /adk/chat`（非流式）、`GET /adk/optimization-tasks/{taskId}` | ADK/MCP 客户端使用；前端只用流式和列表接口 |
| 批量运维 | `POST /system/exchange-calendars/refresh`、`POST /system/exchange-calendars/probe`（全市场批量版） | 运维入口，前端只调按市场版本 |

新增此类端点时，请在本表登记保留原因，避免后续审计误判。
