# Rust 后端编码与依赖边界

更新时间：2026-09-02。本文只描述当前 Rust 产品树。

## 分层

### `crates/jftrade-api`

只负责 HTTP/SSE/WebSocket transport：绑定、校验、认证、wire DTO、错误映射和连接生命周期。允许依赖领域公开 port/type，不得：

- 直接打开 SQLite 或 settings 文件；
- 依赖具体 store driver、Futu protobuf 或模型 Provider；
- 启动 OpenD、Pine 或 Python 进程；
- 持有业务状态的第二写 owner。

### 领域 crates

`jftrade-{settings,marketdata,trading,strategy,backtest,assistant,research,watchlist}` 承载业务规则与协议中立 port。不得依赖 `jftrade-api`、Axum handler、具体 SQLite driver、Futu protobuf 或桌面类型。

跨域交互使用窄 DTO/port；第三处重复的 projection、validation 或 lifecycle 逻辑应提升到最窄共享 owner，而不是创建含糊的 `common/shared/utils` crate。

### Store crates

`jftrade-store-sqlite` 和 `jftrade-store-settings-file` 负责持久化、migration、事务、编码和 `WriterLease`。业务决策留在领域层；store 不依赖 HTTP transport 或具体外部协议。

每个生产数据库只能由一个 product runtime 持有 writer lease。mutation 必须在事务和 owner fence 内完成；取消、冲突、busy、schema drift 和崩溃恢复路径要有测试。

### Integration crates

`jftrade-integration-futu`、`jftrade-integration-pine` 和 `jftrade-integration-marketdata-helper` 封装具体协议、I/O 和进程边界。生成 protobuf 类型不得离开对应 integration；领域层只接收 broker/provider-neutral DTO。

Integration 不拥有全局 Provider 选择、业务缓存、策略状态或用户可见通知；这些 owner 由领域和 `jftrade-engine` composition 持有。

### `crates/jftrade-engine`

唯一生产 composition root，负责：

- 配置解析和 fail-closed admission；
- schema/migration/WriterLease 顺序；
- store、integration、worker 与领域 port 装配；
- 278 production routes 注册；
- runtime cancellation、join 和逆序 shutdown；
- 外部依赖 unavailable adapter 与公开 502/503 语义。

业务规则不要堆入 composition root；重复装配逻辑拆成有清晰 owner 的 builder/adapter。

## 文件与函数约束

- Rust 默认 `#![forbid(unsafe_code)]`。
- 生产函数通常不超过 80 行/60 语句；生产文件目标不超过 800 行。
- 错误类型保留业务分类，transport 统一映射，禁止以字符串匹配承担控制流。
- 取消和 timeout 要穿过 port 边界；启动的 task/process 必须有明确 owner、cancel、join 和 bounded shutdown。
- 测试名描述业务行为，不使用 `more/additional/extra/complete` 等空泛词。
- 普通测试只用 fixture、mock server、临时目录和 testkit，不连接真实 OpenD、数据源或模型 Provider。

## 契约与生成物

- `contracts/openapi/openapi.json` 是公开 HTTP 规范源。
- `proto/` 是 Futu/Pine 中立 protobuf 源。
- Web API 类型、reference 和 Rust protobuf 输出是生成物，不手工修改。
- 公开契约变化运行 `pnpm run generate:docs` 和 `pnpm run check:generated`。
- 历史 Stage 2–9 fixture 是只读兼容输入；不得生成新的 Go oracle 或在 consumer 侧归一化掉真实差异。

## 最小验证

Rust 变更按风险由窄到宽运行：

```bash
cargo test -p <changed-crate> --all-targets
pnpm run check:quick
pnpm run check:rust
```

契约变化额外运行 `pnpm run check:generated`。所有变更都必须保持：

```bash
pnpm run check:go-retirement
pnpm run check:zero-go
```

本地门禁不能替代真实平台安装、签名、升级/回滚、SBOM、安全审查或 post-release smoke。
