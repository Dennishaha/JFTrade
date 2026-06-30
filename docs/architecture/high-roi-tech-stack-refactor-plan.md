# 高收益技术栈重构计划

本文拆分的是当前技术栈中最值得做、收益最高、允许部分重构的优化项。目标不是替换现有主栈，而是把 JFTrade 已经形成的方向做硬：

- Go/Gin sidecar 继续作为唯一 API 后端入口。
- Vue 3 + Vite + Vuetify 继续作为本地控制台前端。
- PineTS worker、Futu OpenD、ADK runtime 继续作为核心能力边界。
- 重构重点放在契约、状态、实时事件、性能、领域组件和可观测性。

## 总体原则

1. 每个 slice 必须能独立合并、独立验证。
2. 先改“边界”和“基础设施”，再改页面体验。
3. 不做框架替换式迁移，不把稳定的 REST API 一次性迁到 RPC。
4. 所有重构必须保留现有 `/api/v1/*` 控制台契约。
5. 交易、行情、回测、ADK 相关改动必须按业务语义补测试，不只验证类型通过。

## 推荐顺序

| 顺序 | Slice | 收益 | 风险 | 推荐先验收范围 |
| --- | --- | --- | --- | --- |
| 1 | OpenAPI 类型生成与 typed API client | 高 | 低 | Settings + Strategy definitions |
| 2 | Server-state 迁到 TanStack Query | 高 | 中 | Settings + Backtest runs + Strategy definitions |
| 3 | 统一实时事件模型 | 高 | 中高 | 行情 live stream + 通知 + backtest sync task |
| 4 | 高吞吐 UI 性能约束 | 高 | 中 | ADK timeline + Backtest results |
| 5 | JFTrade 领域组件层 | 中高 | 中 | Workspace + Strategy runtime |
| 6 | 结构化日志与链路观测 | 高 | 中 | API request + OpenD + ADK run + Pine worker |

## 当前收口状态

本次变更按一个阶段性提交收口，不再按 slice 拆分提交。为了避免后续误判完成度，当前状态按“已落地 / 仍保留边界”记录如下：

- Slice 1 已落地 OpenAPI TS 生成、typed API wrapper 与第一批 settings/strategy/backtest 调用迁移；由于当前 Swagger envelope 的 `data` 仍是 `unknown`，调用方仍需要显式声明响应类型，尚不是从 OpenAPI 自动推导完整响应 DTO。
- Slice 2 已落地 TanStack Query client、query key 规范、Settings、Strategy definitions 与 Backtest runs 的第一批 server-state 迁移；Pinia 仍只保留控制台客户端状态。
- Slice 3 已落地后端原生 live event envelope、前端严格 envelope 解析、事件去重/乱序保护和 market/notification/backtest reducer；前端不再接受旧 top-level live payload。`payload` 内仍保留原业务字段，避免覆盖行情源、通知来源等字段如 `source`。
- Slice 4 已落地 ADK timeline/tool trace bounded rendering、Mermaid lazy import、Backtest 大结果分页/裁剪和不可变结果 `markRaw`；这属于窗口化渲染，不是全量虚拟滚动框架。
- Slice 5 已落地 Workspace 与 Strategy runtime 的第一批领域组件，组件通过 props 输入，不直接发请求。
- Slice 6 已落地 request observability、统一字段、`importance` 分级、SystemPage 摘要和 OpenD/ADK/backtest/Pine worker 关键链路日志；OpenD subscribe 细粒度 span 与 ADK tool 级 span 可作为后续增量。

## Slice 1: OpenAPI 类型生成与 typed API client

### 目标

把现有 Swagger/OpenAPI 产物变成前端可用的类型来源，减少手写请求、字段漂移和重复 DTO 定义。

### 当前痛点

- 后端已有 `go generate ./cmd/jftrade-api` 和 `docs/swagger/swagger.json`，但前端仍以手写 `fetch` 和手写类型为主。
- 接口数量已经覆盖 settings、market-data、strategy、backtest、ADK、portfolio、execution，字段漂移风险会持续升高。
- API 错误 envelope、分页参数、status 字段等语义在不同 composable 中容易重复实现。

### 建议方案

1. 增加 OpenAPI TS 类型生成脚本。
   - 新增根脚本：`generate:api-types`
   - 输入：`docs/swagger/swagger.json`
   - 输出建议：`apps/web/src/generated/openapi.ts`
2. 收口 `apps/web/src/composables/apiClient.ts`。
   - 保留现有 `buildApiUrl`、认证、错误处理。
   - 新增 typed `apiGet`、`apiPost`、`apiPut`、`apiDelete` 包装。
   - 统一解析 JFTrade API envelope。
3. 先迁移低风险接口。
   - `GET/PUT /api/v1/settings/*`
   - `GET/POST/PUT/DELETE /api/v1/strategy-definitions`
   - `GET /api/v1/backtests`

### 主要文件

- `package.json`
- `scripts/generate-api-types.mjs`
- `apps/web/src/generated/openapi.ts`
- `apps/web/src/composables/apiClient.ts`
- `internal/app/apiserver/servercore/openapi_snapshot_test.go`

### 验收标准

- `npm run generate:docs` 后可以稳定生成 Swagger。
- `npm run generate:api-types` 可以从当前 Swagger 生成前端类型。
- 至少 2 个前端模块不再手写响应 DTO。
- `npm --workspace @jftrade/web run typecheck` 通过。
- OpenAPI snapshot 测试仍能捕获破坏性 API 变更。

### 不做

- 不要求所有接口一次性迁移。
- 不把 Go DTO 自动反向生成出来。
- 不在这个 slice 引入 RPC。

## Slice 2: Server-state 迁到 TanStack Query

### 目标

把“后端数据状态”从 Pinia/页面 composable 中分离出来，让请求、缓存、刷新、loading、错误、失效重取有统一语义。

### 当前痛点

- `useConsoleData`、`useBacktestRuns`、ADK/settings 相关 composable 同时管理请求、缓存、派生状态和 UI 状态。
- 页面刷新策略分散，容易出现 stale data、重复请求、局部 loading 状态不一致。
- live stream 更新与 HTTP 重新拉取之间没有统一 cache patch 位置。

### 建议方案

1. 引入 `@tanstack/vue-query`。
2. 建立 query key 规范。
   - `["settings", section]`
   - `["strategyDefinitions"]`
   - `["backtestRuns", filters]`
   - `["adk", "catalog"]`
   - `["marketData", market, symbol, period]`
3. Pinia 保留客户端状态。
   - 当前标的、布局、主题、dock、交易环境、连接状态。
4. 第一批迁移：
   - Settings 页面读取与保存。
   - Strategy definitions 列表/详情。
   - Backtest runs 列表和单个 run 状态。

### 主要文件

- `apps/web/src/main.ts`
- `apps/web/src/composables/apiClient.ts`
- `apps/web/src/composables/useBacktestRuns.ts`
- `apps/web/src/pages/SettingsPage.vue`
- `apps/web/src/pages/BacktestPage.vue`
- `apps/web/src/pages/StrategyDesignPage.vue`

### 验收标准

- 相同页面重复进入不会重复发起不必要请求。
- 保存 settings 后能定向 invalidate 对应 query。
- 启动 backtest 后 backtest runs 列表能通过 query invalidation 或 cache patch 更新。
- Pinia 中不再保存可由服务端查询恢复的大型列表。
- `npm --workspace @jftrade/web run test` 和 typecheck 通过。

### 不做

- 不把 WebSocket/SSE 连接状态放进 TanStack Query。
- 不强行迁移所有页面；先迁移高重复、高漂移、高 loading 复杂度模块。

## Slice 3: 统一实时事件模型

### 目标

把行情、通知、订单、backtest sync task、ADK run/timeline 等实时更新统一为一套前端事件入口和 reducer/patch 边界。

### 当前痛点

- 前端已有 shared live socket，但不同业务仍容易各自处理推送、刷新和错误。
- 行情快照、盘口、K 线、通知、任务进度和 ADK run 的更新语义不同，缺少统一 envelope。
- 实时推送与 HTTP cache 的合并点不明确，后续引入 TanStack Query 后更需要统一 patch 入口。

### 建议事件 envelope

```ts
type LiveEventEnvelope<TPayload = unknown> = {
  eventId: string;
  type: string;
  source: "market-data" | "execution" | "notification" | "backtest" | "adk" | "system";
  entityId: string;
  version?: number;
  serverTime: string;
  payload: TPayload;
};
```

### 建议方案

1. 前端建立 `liveEventBus`。
   - 只负责接收、校验、分发事件。
   - 不直接写页面状态。
2. 每个业务域建立 reducer。
   - `marketDataLiveReducer`
   - `executionLiveReducer`
   - `backtestLiveReducer`
   - `adkLiveReducer`
3. reducer 只做两类事。
   - patch TanStack Query cache。
   - patch 必须保留在 Pinia 的客户端状态。
4. 后端统一输出 event envelope。
   - 不再输出旧 top-level live payload。
   - 所有下行事件统一带 `eventId`、`type`、`source`、`entityId`、`serverTime`、`payload`。

### 主要文件

- `apps/web/src/composables/sharedLiveSocket.ts`
- `apps/web/src/composables/useSharedLiveStream.ts`
- `apps/web/src/composables/useConsoleData.ts`
- `apps/web/src/components/workspace/*`
- `internal/api/live`
- `internal/live`
- `internal/marketdata`
- `internal/app/apiserver/servercore/notifications.go`

### 验收标准

- Watchlist、K 线、通知至少三类事件通过统一 bus 进入。
- Live socket reconnect 后业务 reducer 不重复注册、不重复 patch。
- backtest sync task 或 ADK run 至少一类任务进度能通过统一事件模型更新。
- 前端拒绝旧 top-level live payload，避免协议兼容层继续掩盖后端未收敛的问题。
- 有针对 reconnect、重复事件、乱序事件的测试。

### 不做

- 不改变 `payload` 内的业务语义；协议收敛只改变传输外层。
- 不把所有 HTTP 轮询立即删除；先让实时事件成为主路径，保留必要 fallback。

## Slice 4: 高吞吐 UI 性能约束

### 目标

给高频、高数据量 UI 建立明确性能边界，避免 ADK timeline、回测结果、订单列表、行情列表在数据变大后拖慢整个控制台。

### 当前痛点

- ADK timeline 和 run trace 可能快速膨胀。
- Backtest results、orders/fills、watchlist、notifications 都可能持续增长。
- Vue 深层响应式对大型不可变对象不划算。
- Mermaid、Monaco、chart 等重组件容易被父组件频繁触发重渲染。

### 建议方案

1. 大型数组使用 virtualization。
   - ADK timeline。
   - Backtest run table。
   - Orders/fills/history。
2. 大型不可变 payload 用 `shallowRef` 或 mark raw。
   - ADK trace payload。
   - Mermaid/rendered graph 输入。
   - backtest result detail。
3. 重组件明确 lazy mount。
   - Monaco editor。
   - Mermaid visualization。
   - large chart result detail。
4. 建立性能测试或截图验证。
   - 至少覆盖 ADK 大 session。
   - 至少覆盖 backtest 多结果列表。

### 主要文件

- `apps/web/src/pages/ADKPage.vue`
- `apps/web/src/components/shared/ADKRunTrace.vue`
- `apps/web/src/composables/useADKWorkflowQueueState.ts`
- `apps/web/src/pages/BacktestPage.vue`
- `apps/web/src/components/BacktestChart.vue`
- `apps/web/src/components/workspace/WatchlistPanel.vue`

### 验收标准

- 大 session 下 ADK 页面不会因 timeline 更新触发整页级重复重算。
- Backtest 结果列表分页/筛选不随总记录线性卡顿。
- Mermaid 和 Monaco 只在需要展示时初始化。
- 关键组件有测试覆盖至少一种大数据输入。

### 不做

- 不为了性能牺牲交互语义。
- 不引入过早复杂的全局 worker 渲染；先做 virtualization、shallow data 和 lazy mount。

## Slice 5: JFTrade 领域组件层

### 目标

建立业务领域组件，减少页面直接堆 Vuetify 控件，让交易控制台的交互和视觉语义稳定下来。

### 当前痛点

- 页面和组件混合了业务语义、布局、Vuetify 细节和请求状态。
- Workspace、Strategy runtime、ADK、Settings 中有很多相似状态表达：健康、连接、权限、风险、任务进度、运行状态。
- 后续 UI 优化如果只改单页，容易造成风格和行为不一致。

### 建议组件分层

```text
apps/web/src/components/
  domain/
    instrument/
    market-data/
    runtime/
    strategy/
    backtest/
    adk/
  shared/
```

### 第一批领域组件

- `InstrumentPicker`
- `MarketStatusBadge`
- `RuntimeHealthBadge`
- `BrokerAccountSelector`
- `DenseMetricStrip`
- `OrderTicket`
- `StrategyInstanceCard`
- `BacktestRunTable`
- `ADKApprovalPanel`
- `TaskProgressLine`

### 主要文件

- `apps/web/src/components/workspace/*`
- `apps/web/src/components/strategy-runtime/*`
- `apps/web/src/pages/BacktestPage.vue`
- `apps/web/src/components/adk-page/*`
- `apps/web/src/components/SectionHeader.vue`

### 验收标准

- Workspace 至少拆出 3 个可复用领域组件。
- Strategy runtime 至少拆出 2 个可复用领域组件。
- 新组件不直接发请求；数据通过 props 或 domain composable 输入。
- 组件测试覆盖关键状态：loading、empty、error、normal、disabled。

### 不做

- 不做纯视觉大改版。
- 不新增一个和 Vuetify 平行的大型 UI 框架。
- 不把领域组件变成隐式全局 store 访问点。

## Slice 6: 结构化日志与链路观测

### 目标

让 OpenD、行情订阅、回测、Pine worker、ADK run、HTTP 请求能够用统一字段串起来，减少线上/本地排障时间。

### 当前痛点

- 项目中已有测试和运行状态端点，但跨链路排查仍需要翻日志、查 SQLite、看前端状态。
- ADK、backtest、OpenD、Pine worker 都有长任务或外部依赖，一旦失败需要能快速定位在哪一段。
- 日志字段如果不统一，后续即使接 OpenTelemetry 也很难关联。

### 建议字段规范

| 字段 | 用途 |
| --- | --- |
| `request_id` | HTTP 请求链路 |
| `session_id` | ADK session 或用户工作会话 |
| `run_id` | ADK run、backtest run、strategy run |
| `task_id` | sync task、optimization task、worker task |
| `broker_id` | broker 维度 |
| `account_id` | account 维度 |
| `instrument_id` | 标的维度，例如 `HK.00700` |
| `provider_id` | ADK provider 维度 |
| `source` | Futu/OpenD/PineTS/ADK/system |
| `importance` | 日志重要性，分为 `low`、`normal`、`high`、`critical` |

### 建议方案

1. 后端先统一结构化 logger 接口。
   - 新增小的 logging package 或 app-level logger provider。
   - 不要求一次性替换所有 logrus 调用。
   - 写日志时显式标注 `importance`，并支持最低重要性阈值配置。
2. HTTP middleware 注入 `request_id`。
3. 关键链路补 span/log fields。
   - OpenD connect/query/subscribe。
   - market-data collector。
   - backtest run and sync task。
   - PineTS worker request。
   - ADK run/tool/approval。
4. 系统状态页增加轻量观测摘要。
   - 最近错误。
   - 最近慢请求。
   - OpenD query/subscribe 健康摘要。

### 主要文件

- `internal/app/apiserver/servercore/router.go`
- `internal/api/*`
- `internal/marketdata`
- `internal/integration/futu`
- `pkg/futu`
- `pkg/backtest`
- `pkg/strategy/pineworker`
- `pkg/adk`
- `apps/web/src/pages/SystemPage.vue`

### 验收标准

- 每个 HTTP 请求有稳定 `request_id`。
- OpenD 查询失败日志带 `request_id` 或业务关联字段。
- ADK run 相关日志带 `session_id`、`run_id`、`provider_id`。
- backtest 和 sync task 日志带 `run_id` 或 `task_id`。
- 至少有一条端到端排障路径文档化：从 SystemPage 状态进入日志/任务/运行记录。

### 不做

- 不强制第一阶段接入完整 collector/backend。
- 不把日志重构扩大成全仓库机械替换。
- 不在业务代码里散落 ad-hoc 字段名。

## 交付节奏建议

### Milestone A: 契约和状态基础

包含：

- Slice 1
- Slice 2 的 Settings + Strategy definitions

完成后收益：

- 前后端字段漂移明显减少。
- 页面请求/缓存语义开始统一。
- 后续 live event patch 有明确落点。

### Milestone B: 实时数据收口

包含：

- Slice 2 的 Backtest runs
- Slice 3 的 live event bus
- Watchlist/Kline/Notifications 三类事件接入

完成后收益：

- 实时行情和任务状态更新不再散落在页面里。
- WebSocket/SSE reconnect 和 cache patch 可测试。

### Milestone C: 大页面性能

包含：

- Slice 4
- ADK timeline
- Backtest results

完成后收益：

- 大 ADK session 和大量回测记录不再拖慢控制台。
- 后续增加 run/audit/task 数据量时风险降低。

### Milestone D: 产品化组件与诊断

包含：

- Slice 5
- Slice 6 的 request_id、OpenD、ADK、backtest 关键链路字段

完成后收益：

- UI 一致性提高。
- 排障路径缩短。
- 新业务模块更容易沿用统一组件和日志字段。

## 总体验收门禁

每个 milestone 合并前至少执行：

```bash
npm run generate:docs
npm --workspace @jftrade/web run typecheck
npm --workspace @jftrade/web run test
go test ./... -count=1 -timeout 300s
```

如果改到 PineTS worker 或 release asset，还需要执行：

```bash
npm run typecheck:pineworker
npm run test:pineworker
npm run check:pinets-release
```

如果改到架构依赖边界，还需要执行：

```bash
bash scripts/check-arch-deps.sh
```

## 风险控制

- 每次只迁移一个页面或一个业务域，不做跨全站大替换。
- 所有旧 API wrapper 保留兼容层，迁移完成后再删除。
- live event 不保留旧协议兼容解析；如需变更协议，必须先更新后端 envelope 与前端严格解析测试。
- 涉及交易、订单、账户、实盘控制的改动必须有业务语义测试。
- 任何性能重构必须先准备一组大数据输入，避免只凭主观流畅度判断。

## 不推荐的重构

- 不从 Vue/Vite 换到 React/Next。
- 不从 Vuetify 换到另一套 UI 库。
- 不把 REST API 全量迁移到 gRPC/ConnectRPC。
- 不为了桌面化立即引入 Electron/Tauri。
- 不把 Pinia 完全移除；它仍适合客户端状态。
- 不把 bbgo、Futu、PineTS、ADK 的边界混成一个大 service。
