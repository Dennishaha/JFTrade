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

有合法 startedAt（解析失败时回退 createdAt）的非终态 ADK run，GET 重连按其冻结
maxDurationMs 加 30 分钟保留期判断可见性；非正时限使用 30 分钟默认值，超过截止点
返回 stream not found。时限规则归 Assistant 领域，engine 读取去除历史数组的元数据并
装配时间；读取或类型验证失败时不由该保留期判断拒绝，仍沿用 cursor/body 原错误路径。
这项读策略不删除 durable history、不修改 run 终态；记录级终态 TTL 与无合法时间的
last-event 回退不在这一判断中。

每个 HTTP 请求在 transport 中创建独立 `RequestCancellation`；上游 transport owner
也可通过 Axum request extension 传入同一信号。SSE body owner 在首次和后续读取前
检查该请求信号，预取消可在 retry 前结束为空 body，idle 取消唤醒并立即释放 reader。
取消只释放该请求 body，同 listener 的其他请求继续服务；信号不进入序列化 `ApiRequest`，
不创建后台 task，不获得领域写入权。

### 领域 crates

`jftrade-{settings,marketdata,trading,strategy,backtest,assistant,research,watchlist}` 承载业务规则与协议中立 port。不得依赖 `jftrade-api`、Axum handler、具体 SQLite driver、Futu protobuf 或桌面类型。

跨域交互使用窄 DTO/port；第三处重复的 projection、validation 或 lifecycle 逻辑应提升到最窄共享 owner，而不是创建含糊的 `common/shared/utils` crate。

内置 Assistant agent 的保护字段比较、规范化及可编辑配置选择由 `jftrade-assistant`
持有。engine adapter 读取当前状态并验证 provider 可用性，领域规则通过后才交由
现有 store writer 保存；创建与更新入口使用同一保护规则。

Skill 文档的 YAML frontmatter 解码归 `jftrade-assistant`。engine 文件系统 adapter
在列表读取时发现 skills 根目录的直接子目录，将有效 SKILL.md 的元数据与 durable
安装记录、内置目录投影合并；读取不写 SQLite 或审计。根目录来自
`JFTRADE_ADK_SKILLS`，未配置时为 settings 文件同级的 skills 目录，安装与发现共用。
卸载仍由原 mutation owner 执行；文件系统投影不另建写入者。
Skill 目录排序按 source 再按 displayName，外部文档的未知工具保留并标记 WARNING；
内置文档按产品目录约定免除此警告。排序和工具引用规则由 Assistant 领域持有，
engine 仅提供文件元数据及实际工具目录。
下载后的 Skill 文档在原安装 mutation owner 内由领域规则写入 source 元数据；
单文件与 ZIP 的 SKILL.md 使用同一规则，资源文件保留原内容。安装记录的工具、
version、校验状态与内容哈希从实际安装文件投影后交给原 SQLite writer 保存，
不创建第二条持久化路径。
Skill 卸载的 SQLite DELETE 由原 writer 在事务中执行，文件清理由 engine mutation
owner 提供。DELETE 执行失败不调用文件清理，文件清理失败回滚记录删除；callback
持有 store 连接期间不能重入同一 store。跨 SQLite commit 与文件系统的整体原子性
以及文件系统部分删除的恢复需另行处理。
Skill registry 卸载缺失项保留类型化的文件系统 NotFound 源错误；原 mutation port
负责将它投影为 500/ADK_SKILL_UNINSTALL_FAILED，错误分类不依赖消息文本。
原 Skill 安装 owner 在创建临时目录和写入记录前检查已注册内置 ID；catalog 形式的
内置 Skill 具有与已存在安装目录相同的重复安装保护，外部单文档及 ZIP 不能替换它。
Skill URL 下载由原安装 mutation owner 执行；生产入口固定注入安全地址解析器。
重定向每跳重新校验 URL 与地址，并把 HTTP client 固定到该已校验地址，达到第五次
重定向时报错。最终文档保持原始安装 URL。安装错误统一映射为
400/ADK_SKILL_INSTALL_FAILED；单文档与 ZIP 的文件大小及安全路径规则继续生效。
同步 mutation 的线程以有界下载截止时间驱动同一异步下载/安装 owner，20 秒总截止时间
跨重定向保持；reqwest 的类型化 timeout 保留为下载超时，完整下载成功后才调用安装 writer。

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
