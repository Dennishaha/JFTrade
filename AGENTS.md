# JFTrade AI 开发指令

本文件是仓库级开发规则的唯一入口。目标路径下的局部 `AGENTS.md` 只补充入口、依赖边界和最小验证；同一事项冲突时以更深层文件为准，其余根规则继续生效。`CLAUDE.md`、`.github/agents` 和 `.github/instructions` 只引用这些规则，不维护另一套架构事实。

## 开始任务

1. 查看 `git status --short`，保留用户已有改动；读取从根到目标目录沿途适用的 `AGENTS.md`。
2. 用 [模块表](scripts/module-map.json) 定位入口和 affected 范围，再从 [文档导航](docs/README.md) 选择相关专题，不默认通读全部文档或扫描全仓。
3. 先定位调用方、状态写入所有者和测试，再编辑。只做当前需求必要的变更，不因文件名相似跨域复制实现。
4. 验证后复查 diff，交付时说明改动、实际执行的检查及未完成项；未运行、跳过或缺少环境的检查不能记为通过。

## 项目与职责边界

JFTrade 是 Rust 引擎/API、Vue 3 控制台、Tauri 2 桌面壳、Node PineTS worker 和 Python market-data helper 组成的本地量化工作台。Rust/Tauri 持有全部生产 API 和桌面运行时；仓库不包含 Go/Wails 源码、模块、生成器、构建入口或运行产物。

- `crates/jftrade-engine` 是唯一生产 composition root；`apps/desktop/src-tauri` 管理桌面专属能力，不另建业务 API。
- `crates/jftrade-api` 只做 transport、绑定、校验、port 调用、错误映射和 wire DTO，不直接依赖 SQLite driver、Futu protobuf 或模型 Provider。
- 业务规则归属对应领域 crate；具体持久化和外部协议放在 `jftrade-store-*`、`jftrade-integration-*`，由 engine 注入。依赖方向见 [后端规范](docs/architecture/backend-coding-standards.md)。
- Rust engine 持有 SQLite 权威写入与 `WriterLease`。SQLite、交易、订阅、通知、Assistant 审批/任务和 artifact 必须保持唯一写入所有者，禁止双写。
- PineTS 只产出信号、图形和 order intents；Rust 负责撮合、成交、资金曲线、风控和下单。
- 前端只承诺 `/api/v1/*`；bbgo 原生 `/api/*` 不是控制台运行模式。

| 修改范围 | 局部指令 / 先读文档 |
| --- | --- |
| Rust 领域、API、存储与集成 | [crates/AGENTS.md](crates/AGENTS.md) |
| Vue 控制台 | [apps/web/AGENTS.md](apps/web/AGENTS.md) |
| Tauri 桌面 | [apps/desktop/src-tauri/AGENTS.md](apps/desktop/src-tauri/AGENTS.md) |
| PineTS / Python helper | [workers/AGENTS.md](workers/AGENTS.md) |
| 脚本、CI、质量门禁 | [质量门禁](docs/architecture/quality-gates.md) |
| 候选、签名、升级回滚、发布 | [发布资格](docs/architecture/release-qualification.md) |

## 环境与命令

所有命令默认从仓库根目录运行。Node 要求和 pnpm 精确版本以 [package.json](package.json) 为准；Rust 使用 [rust-toolchain.toml](rust-toolchain.toml) 和已提交的 `Cargo.lock`。protoc 版本以 [setup-rust](.github/actions/setup-rust/action.yml) 为准；Python/uv 环境见 [helper README](workers/marketdata-sidecar/README.md)。不要绕过锁文件升级工具链或依赖。

```bash
pnpm install --frozen-lockfile
pnpm run dev:desktop       # Tauri 原生桌面联调
pnpm run dev:web           # 纯浏览器前端；须另启 API 并开启 Web 访问
cargo run -p jftrade-engine --bin jftrade-api-rust
pnpm run check:quick       # 当前工作树快速反馈，只读检查
pnpm run check:affected    # merge-base affected 集成检查
pnpm run check:rust        # Rust static、workspace 测试和兼容 replay
pnpm run check:generated   # 临时目录生成并比较，不改工作树
pnpm run check:all         # 完整本地门禁，含构建与 smoke
```

`pnpm run test:affected -- --print` 可预览 merge-base 测试计划，`pnpm run check:quick -- --print` 可预览工作树计划。根指令、模块表、共享工具链或门禁变更会触发全量兜底；`quick` 不保证只跑少量测试。`test:preflight` / `test:pr` 是兼容入口，完整命令与 CI 边界见 [质量门禁](docs/architecture/quality-gates.md)。

## 修改约束

- 未经需求明确要求，不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite schema 或 worker wire contract。
- [contracts/openapi/openapi.json](contracts/openapi/openapi.json) 和 `proto/` 是契约源；生成的 OpenAPI 类型、reference、protobuf 和 embedded assets 不得手工改。有意修改 HTTP 契约时运行 `pnpm run generate:docs`；不要将生成写入步骤混进只读检查。
- Rust 默认 `#![forbid(unsafe_code)]`；直接依赖集中精确锁定，新增依赖遵守“官方优先、其次高采用项目”和 `deny.toml`，不得提前引入未使用的迁移候选。
- 生产函数通常不超过 80 行/60 语句，生产文件目标不超过 800 行；按职责拆分，不为规避门禁搬运代码或调高预算。
- 测试名描述业务行为，不使用覆盖率数字或 `more/additional/extra/complete` 等空泛命名。
- 普通测试使用 fixture、mock server、临时目录或 testkit，不连接真实 Futu/OpenD、行情源或模型 Provider；真实外部依赖只在显式 live workflow 验证。
- 使用 `rg` 优先搜索，编辑使用 `apply_patch`，不回退用户已有改动。发布、实盘、数据清理和迁移不由普通开发请求隐含授权。

## 验证与文档收尾

1. 先跑局部指令中最窄的受影响测试，再跑 `pnpm run check:quick`；先查看计划，不能通过缩小 diff 范围绕过门禁。
2. Rust 变更至少跑 `pnpm run check:rust`；公开契约变更额外跑 `pnpm run check:generated`。检查失败应保留错误证据，不静默重写 fixture 或放宽阈值。
3. 纯文档/指令修改先跑 `pnpm run check:ai-context`、核对链接与命令，再跑 `check:quick`。门禁选择和执行语义以脚本为准。
4. 边界变化时同步对应 `docs/architecture*` 专题、[文档导航](docs/README.md) 和模块表；入口文档只保留摘要及链接。
5. 架构事实不收录一次性迁移记录、覆盖率冲刺目标或旧包路径；活动计划归 [roadmap](docs/roadmap.md)，历史资料与当前规则分开。
6. 本地检查通过不代表发布资格。候选证据和发布后验证按 [发布资格](docs/architecture/release-qualification.md) 独立完成。
