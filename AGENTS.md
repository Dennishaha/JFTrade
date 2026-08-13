# JFTrade AI 开发指令

本文件是仓库级事实源。局部目录的 `AGENTS.md` 只补充该领域的入口、依赖边界和最小验证命令；冲突时以更深层文件为准。`CLAUDE.md`、`.github/agents` 和 `.github/instructions` 不得维护另一套架构事实。

## 项目边界

JFTrade 是 Go 后端、Vue 3 控制台、Wails v3 桌面壳、Node PineTS worker 和 Python market-data helper 组成的本地量化工作台。入口和影响范围见 [`scripts/module-map.json`](scripts/module-map.json)。

配置要求：Node `>=22.13`、pnpm `11.12.0`、Go `1.26.5`、protoc `34.1`。安装依赖统一使用 `pnpm install --frozen-lockfile`。

## 日常入口

```bash
pnpm run prepare:desktop-dev  # 显式构建桌面开发使用的外部 PineTS worker
go tool wails3 dev -config ./build/config.yml -port 3003  # Wails 原生桌面联调
pnpm run prepare:desktop-release  # 显式准备发布版前端、bindings、Pine 与 market-data 资产
JFTRADE_DESKTOP_PREPARED=1 go tool wails3 build  # 使用已准备的发布资产构建 bin/JFTrade
go run ./cmd/jftrade-api   # 独立 API，默认 127.0.0.1:3000
pnpm run dev:web           # 浏览器前端，默认 127.0.0.1:3003
pnpm run check:quick       # 变更范围快速检查，不能修改工作树
pnpm run test:affected     # 只跑受影响测试
pnpm run check:generated   # 临时目录生成并比较契约，不能修改工作树
pnpm run check:all         # 完整本地门禁
```

`test:preflight`、`test:pr` 是兼容别名。改公开 HTTP 契约时运行 `pnpm run generate:docs`；开发检查使用 `check:generated`，不要把生成步骤隐含在只读检查里。

## 架构事实

- `cmd/jftrade-api` 和 `cmd/jftrade-desktop` 装配同一套 `internal/app/apiserver`，不存在第二套业务 API。
- `internal/api/*` 只做绑定、校验、service 调用、错误映射和 DTO；禁止直接碰 store、integration、SQLite、Futu protobuf。
- `internal/{system,settings,marketdata,trading,strategy,backtest,assistant,watchlist}` 承载业务规则；禁止依赖 HTTP transport、`internal/api`、具体 DB 驱动和 Futu protobuf。
- `internal/app/apiserver` 是 composition root；新业务逻辑下沉到 service 或现有 `*app` 子包，不继续扩张 `servercore`。
- `internal/store/*` 只负责 schema、migration、codec、query 和业务模型转换；`internal/integration/*` 只封装外部协议/SDK。
- `pkg/futu` 只做 OpenD/bbgo 协议适配；订阅 demand、freshness、运行时生命周期和控制平面编排属于 `internal/marketdata`、`internal/app/apiserver` 或对应 service。
- PineTS 只产出信号、图形和 order intents；Go `pkg/backtest` 负责撮合、成交、资金曲线、风控和下单。
- 前端只承诺 `/api/v1/*`；bbgo 原生 `/api/*` 不是控制台运行模式。

## 硬性约束

- 不改变公开 HTTP/OpenAPI、SSE、WebSocket、Wails bindings、SQLite schema 或公开 `pkg/*` API，除非需求明确要求。
- 生成代码（OpenAPI、reference、Wails bindings、protobuf、embedded assets）不得手工改。
- Go lint 使用 golangci-lint v2.12.0；生产函数通常不超过 80 行/60 语句，生产文件目标不超过 800 行。
- 新测试文件名描述业务行为，不使用覆盖率数字或 `more/additional/extra/complete` 等空泛命名。
- 真实 Futu/OpenD 只在显式 live workflow 使用；普通测试使用 fixture、mock server 或 testkit。
- 新增 `pkg/*` 必须有仓库外消费者或已发布公开签名依据；否则放 `internal/*`。
- 使用 `rg` 优先搜索，编辑使用 `apply_patch`，不回退用户已有改动。

## AI 工作流

1. 先读本文件和最近的局部 `AGENTS.md`，再按模块表进入专题文档和入口文件。
2. 先定位调用方、所有权和测试，再编辑；不要因文件名相似跨域复制实现。
3. 变更后先跑最窄的 affected test，再跑 `check:quick`；契约变化额外跑 `check:generated`。
4. 若边界发生变化，同步 `docs/architecture*`、`docs/README.md` 和模块表。
5. 不把一次性迁移记录、覆盖率目标或旧包路径写回架构事实文档。
