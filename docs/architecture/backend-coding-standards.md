# Rust 后端编码与依赖边界

更新时间：2026-09-02。本文只描述当前 Rust 产品树。

## 分层

### `crates/jftrade-api`

只负责 HTTP/SSE/WebSocket transport：绑定、校验、认证、wire DTO、错误映射和连接生命周期。允许依赖领域公开 port/type，不得：

- 直接打开 SQLite 或 settings 文件；
- 依赖具体 store driver、Futu protobuf 或模型 Provider；
- 启动 OpenD、Pine 或 Python 进程；
- 持有业务状态的第二写 owner。

`ApiStream` 的 body 由单消费者取得：channel body 的消费者关闭会通知 producer，
iterator body 只随 HTTP 消费推进逐帧编码，释放 body 即释放剩余 iterator；iterator
不代表上游历史查询已经分页。HTTP listener owner 在 graceful join 前发出该 listener
的连接取消信号，结束未完成 handler、SSE 和升级后的 WebSocket；Web listener
重配置不关闭共享 desktop live hub。浏览器 HTTP 与 WebSocket 使用同一 Origin
策略，包括当前 Web listener 的动态端口校验。监听器端口授权只来自当前
runtime bind，不能把启动端口复制到静态 Origin 清单；重绑定成功后旧端口授权
撤销，重绑定失败时保持当前监听器授权。

ADK GET 重连 body 由 HTTP 消费者持有，无后台 producer task。SQLite adapter 固定
重连时的 sequence watermark，每页最多向 Rust 解码 64 条历史/新事件；watermark
之前标记 replay，之后为 live。终态排空所有页后结束，连接释放即释放 reader 与 timer。
事件仍保存在 run 的 JSON 数组中，SQLite 每页仍扫描该数组；这不承诺索引读取或固定查询成本。

### 领域 crates

`jftrade-{settings,marketdata,trading,strategy,backtest,assistant,research,watchlist}` 承载业务规则与协议中立 port。不得依赖 `jftrade-api`、Axum handler、具体 SQLite driver、Futu protobuf 或桌面类型。

跨域交互使用窄 DTO/port；第三处重复的 projection、validation 或 lifecycle 逻辑应提升到最窄共享 owner，而不是创建含糊的 `common/shared/utils` crate。

内置 Assistant agent 的保护字段比较、规范化及可编辑配置选择由 `jftrade-assistant`
持有。engine adapter 读取当前状态并验证 provider 可用性，领域规则通过后才交由
现有 store writer 保存；创建与更新入口使用同一保护规则。

Assistant chat请求身份的字段规范化与有序JSON表示同样归领域crate；engine计算摘要并在现有durable和无run保留owner比较身份，旧摘要兼容不引入第二写入者。

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

Calendar 管理器持有源健康状态和告警去重，在释放状态锁后调用协议中立 alert sink。
engine 在每次投递时读取权威日历通知设置；读取失败或通知关闭时不投递，健康状态仍由
Calendar 管理器更新。浏览器日历通知进入现有 LiveHub，沿用通知信封与有界重连历史。

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
- 冻结 compatibility fixture 是只读输入；不得从历史实现重新生成，或在 consumer 侧归一化掉真实差异。

## 最小验证

Rust 变更按风险由窄到宽运行：

```bash
node scripts/quality/cargo-nextest.mjs run -p <changed-crate> --all-targets --locked
pnpm run check:quick
pnpm run check:rust
```

测试统一使用仓库 nextest wrapper，固定版本、校验和及非打包编译环境；生产 crate 改动还需覆盖反向依赖。局部入口见 [crates/AGENTS.md](../../crates/AGENTS.md)，门禁语义见 [quality-gates.md](quality-gates.md)。契约变化额外运行 `pnpm run check:generated`。所有变更都必须保持：

```bash
pnpm run check:zero-go
pnpm run check:compatibility
```

本地门禁不能替代真实平台安装、签名、升级/回滚、SBOM、安全审查或 post-release smoke。
