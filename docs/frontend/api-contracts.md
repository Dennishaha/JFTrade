# 前端 API 契约边界

前端只有一份线上 wire contract：`docs/swagger/swagger.json` 生成的 `apps/web/src/generated/openapi.ts`。

## 类型分层

- `apps/web/src/contracts/generated/*`：只允许直接导出 OpenAPI schema 或 operation 类型。
- `apps/web/src/contracts/index.ts`：仅作为生成类型的兼容 re-export 入口，不声明 interface、运行时常量或人工 wire shape。
- `apps/web/src/types/view-models/*`：字段缺失、`null`、开放枚举或 UI 语义需要归一化时使用的前端模型。
- `apps/web/src/composables/*Contract.ts`、`*Mappers.ts` 或对应 API 模块：wire → view model 的显式 mapper；必须覆盖缺失字段、`null` 和未知枚举。

`scripts/web-contract-classification.json` 对全部人工类型模块分类，并记录 normalized API model 的 mapper 与边界测试。新增文件没有分类、mapper 或测试时，`pnpm run check:web-contract-audit` 会失败。

## 请求边界

- JSON 业务请求统一使用 `apiGet/apiPost/apiPut/...`；响应类型从生成的 operation 自动推导，调用方不能传入自选 `<T>`。
- SSE 等非 envelope 协议统一使用 `apiRawRequest`，以共享鉴权、CSRF 和认证失效事件。
- `fetch` 只允许出现在 `apps/web/src/composables/apiClient.ts`。

## 生成与门禁

公开 HTTP 契约变更后运行：

```bash
pnpm run generate:docs
pnpm run check:openapi-quality
pnpm run check:web-api-boundary
pnpm run check:web-contract-audit
pnpm run typecheck:web
```

字段审计使用 TypeScript AST 将全部 Swagger schema 与生成类型逐项比较，覆盖字段名、required、nullable、枚举和数组元素类型。协议端点必须按真实 media/status 描述：ADK 流使用 `text/event-stream`，WebSocket 使用 `101`，不能伪装成 JSON envelope。
