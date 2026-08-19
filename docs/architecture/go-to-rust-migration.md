# JFTrade Go → Rust 完整迁移方案与守则

状态：执行中。更新时间：2026-08-19。当前阶段：**阶段 1 实现已落地，等待首次上游 CI 闭环**。

本文是 JFTrade 将 Go 后端与 Wails 桌面壳完整迁移到 Rust 的计划、边界和放行事实源。活动状态在 [roadmap.md](../roadmap.md) 汇总；当前生产架构仍以 [architecture.md](../architecture.md) 为准。任何阶段都不得用“已经写出 Rust 版本”代替兼容性、可靠性和资源验收。

## 1. 已锁定目标

最终产品形态：

- Rust 接管当前 Go 后端、应用装配、领域服务、SQLite store、Futu/OpenD 适配、HTTP/SSE/WebSocket、进程生命周期和桌面壳。
- Vue 3 控制台保留，现有 `/api/v1/*` 调用方式和用户行为保持兼容。
- Node PineTS worker 保留，仍只负责 Pine 执行、信号、图形和 order intents。
- Python market-data helper 保留，仍封装 yfinance 与 AKShare；Rust 只替换它的宿主、鉴权、生命周期和 Provider adapter。
- Assistant 的 Rust 实现使用 [Rig](https://github.com/0xPlaygrounds/rig)，但只有在 Assistant 阶段通过完整行为矩阵后才引入依赖和切换流量。
- 桌面最终从 Wails v3 迁至 [Tauri 2](https://github.com/tauri-apps/tauri)；阶段 1 不改变 Wails 生产入口、bindings 或发布资产。

迁移追求的是长期统一的 Rust 运行时、可维护性、可靠性和可控资源，不以减少代码行数为目标。迁移期间允许 Go 与 Rust 共存，但任一业务状态在任一时刻只能有一个权威 owner。

## 2. 不可退让的兼容边界

除非后续需求单独批准契约变更，迁移必须保持：

| 边界 | 要求 | 验证方式 |
| --- | --- | --- |
| HTTP/OpenAPI | path、method、status、header、JSON 字段、null/omitted、数字精度和错误 envelope 一致 | 现有 OpenAPI baseline、golden response、Go/Rust differential replay |
| SSE | event 名、字段、顺序、心跳、断线语义和终态一致 | 录制事件流逐事件比较，允许清单外差异为零 |
| WebSocket | 握手、消息 schema、顺序、重连和关闭码一致 | 双实现脚本与故障注入 |
| SQLite | 文件名、schema、migration、索引、事务、busy、WAL、时间和 Decimal 编码一致 | 数据库快照、`sqlite_master`/PRAGMA diff、查询与恢复测试 |
| 前端行为 | Vue 页面、设置、运行状态、错误和恢复路径不因后端语言改变 | 现有 Web 测试、生产 bundle smoke、桌面 E2E |
| PineTS/Python | wire contract、启动握手、超时、重试、停止和发布资产一致 | fixture、真实打包 smoke、故障恢复测试 |
| 桌面 | 用户数据目录、端口、单实例、窗口/菜单/链接、更新和日志语义一致 | Wails/Tauri 双壳验收账本和四平台安装包 smoke |
| 公开 Go `pkg/*` | 共存阶段保持；只允许在 Rust 正式大版本发布时一次性硬切并写迁移说明 | public-package 清单、编译消费者检查、major release gate |

阶段 1 新增的 Rust engine 仅是私有迁移基础，**不属于公开产品 API**，不得被前端或仓库外消费者直接调用。

## 3. 目标架构与共存方式

迁移期间的方向是：

```text
Vue / external clients
        |
        | existing HTTP / SSE / WebSocket
        v
Go production sidecar (authoritative until each cutover)
        |
        | private authenticated loopback RPC
        v
Rust migration engine
        |
        +-- future Rust domain/store/integration implementations

Node PineTS worker        Python market-data helper
        ^                            ^
        +--------- current owner ----+
                   then Rust owner
```

共存规则：

1. Go 在能力切换前继续拥有公开 transport、产品生命周期和写入权。
2. Rust bridge 只监听 loopback，使用每进程随机令牌；端口 `0` 由系统分配，不占用固定产品端口。
3. shadow 阶段允许把同一只读输入送给 Go/Rust 比较结果，不允许 Rust 写业务数据库、发单或发布用户可见事件。
4. 一个能力完成硬切后，Rust 成为该能力唯一 owner；Go 只能保留薄适配或回退开关，不能继续复制业务规则。
5. 回退必须切换 owner，而不是合并两边写入；SQLite、订单、审批、任务和订阅一律禁止双写。
6. 私有 RPC 只传稳定、显式版本化 DTO，不传 Go interface、Rust 内存布局或数据库连接。

阶段 1 bridge 采用 Tonic 自带的标准 `grpc.health.v1` 协议，服务名为 `jftrade.migration.v1.Engine`，协议版本为 `migration.v1`。新增业务 RPC 时再引入版本化 protobuf；健康桥不维护一份无业务价值的自定义生成协议。

### 3.1 Rust 目标目录指引（强制）

本节是迁移期间新增目录、crate 和跨目录依赖的强制放置规则。目录出现在蓝图中只表示名称、owner 和最早启用阶段已经预留，**不表示应立即创建**。阶段未启动、没有实际生产代码或没有行为测试时，不得创建空 crate、占位模块或未来依赖。当前 Rust 侧只有 `crates/jftrade-engine` 已存在；其余 Rust/Tauri/迁移支撑目录均是计划目录。

```text
crates/
  jftrade-kernel/                         # 计划，阶段 2：Decimal、时间、ID 等纯基础类型
  jftrade-contracts/                      # 计划，阶段 2：版本化私有 RPC/wire DTO
  jftrade-broker/                         # 计划，阶段 2/4：broker-neutral 类型与 ports
  jftrade-backtest/                       # 计划，阶段 2/3：回测领域和计算核心
  jftrade-marketdata/                     # 计划，阶段 4：行情领域
  jftrade-strategy/                       # 计划，阶段 3/5：策略领域
  jftrade-trading/                        # 计划，阶段 5：交易、风控和订单状态
  jftrade-assistant/                      # 计划，阶段 6：Assistant 领域与 Rig adapter 边界
  jftrade-research/                       # 计划，阶段 7：研究能力
  jftrade-watchlist/                      # 计划，阶段 7：自选领域
  jftrade-settings/                       # 计划，阶段 7：设置领域
  jftrade-calendar/                       # 计划，阶段 7：交易日历领域
  jftrade-datamanagement/                 # 计划，阶段 7：数据维护能力
  jftrade-store-sqlite/                   # 计划，阶段 2 起：SQLite adapter
  jftrade-integration-pine/               # 计划，阶段 3/4：Node worker adapter/lifecycle
  jftrade-integration-marketdata-helper/  # 计划，阶段 4：Python helper adapter/lifecycle
  jftrade-integration-futu/               # 计划，阶段 4/5：OpenD 协议 adapter
  jftrade-api/                            # 计划，阶段 7：Axum HTTP/SSE/WebSocket
  jftrade-engine/                         # 已存在：进程入口和唯一 composition root

apps/
  web/                                    # 已存在并保留：Vue 控制台
  desktop/src-tauri/                      # 计划，仅阶段 8 创建

workers/
  pineworker/                             # 已存在并保留：Node PineTS worker
  marketdata-sidecar/                     # 已存在并保留：Python market-data helper

proto/jftrade/migration/v1/               # 计划，首个自定义私有 RPC 出现时创建
tests/fixtures/rust-migration/<capability>/ # 计划，按能力保存 golden/differential corpus
scripts/rust-migration/                   # 计划，differential、benchmark 和目录门禁工具
```

#### 目录所有权映射

迁移按能力 owner 映射，不按同名文件机械翻译。一个 Go 目录可以拆入多个 Rust 模块，但一个 Rust 能力不得把原有 transport、store 和 integration 依赖一起复制进领域层。

| 当前 owner/目录 | 目标目录 | 最早阶段 | 放置与切换约束 |
| --- | --- | --- | --- |
| `pkg/broker`、`pkg/market` 中经证明确属跨域的基础契约 | `jftrade-kernel`、`jftrade-broker` | 2 | `kernel` 只放无业务 owner 的稳定值类型；broker-neutral 语义归 `broker`，Futu SDK 类型不得进入 |
| `pkg/backtest`、`internal/backtest` | `jftrade-backtest` | 2/3 | 先迁移模型、codec 和纯计算，再由 Go composition root 切换回测 owner |
| `internal/marketdata` | `jftrade-marketdata` | 4 | 拥有 demand、freshness、cache 和 Provider ports；不得包含 OpenD、HTTP helper 或进程管理实现 |
| `internal/strategy` | `jftrade-strategy` | 3/5 | 拥有定义、实例和运行控制规则；Pine 执行协议归 integration crate |
| `internal/trading` | `jftrade-trading` | 5 | 拥有交易命令、风控和订单状态机；实际 broker 协议只能经 port 注入 |
| `internal/assistant` | `jftrade-assistant` | 6 | 拥有 session/run/approval/tool 业务语义；Rig 类型只允许出现在该能力的窄 adapter 边界 |
| `internal/research`、`internal/watchlist`、`internal/settings`、`internal/exchangecalendar`、`internal/datamanagement` | 对应同名能力 crate | 7 | 各自保留独立 owner；不得合并成 `platform`、`common` 或 `services` 大杂烩 |
| `internal/store/*` | `jftrade-store-sqlite` | 2 起 | 只实现各能力定义的 persistence ports；SQLite driver、SQL、migration 和 row codec 不得反向进入领域 crate |
| `internal/integration/futu`、`pkg/futu` | `jftrade-integration-futu` | 4/5 | 只放 OpenD 连接、protobuf、协议映射和生命周期钩子；订阅策略与交易业务规则留在能力 crate |
| `pkg/strategy/pineworker` 的宿主侧职责 | `jftrade-integration-pine` | 3/4 | 只放 Node worker wire adapter、鉴权和生命周期；`workers/pineworker` 继续拥有 PineTS 执行 |
| `internal/marketdataassets` 与 Python helper 宿主侧职责 | `jftrade-integration-marketdata-helper` | 4 | 只放 Python helper 的资产、启动、健康、HTTP adapter 和关闭；Provider 语义继续由 Python 实现 |
| `internal/api/*` | `jftrade-api` | 7 | 只做 Axum/Tower transport、校验、DTO 映射和错误映射，不直接依赖 store 或具体 integration |
| `internal/app/apiserver`、`cmd/jftrade-api` | `jftrade-engine` | 7 | `jftrade-engine` 是唯一 Rust composition root、进程入口和 owner 切换点，不承载领域规则 |
| `cmd/jftrade-desktop`、`internal/desktop` | `apps/desktop/src-tauri` | 8 | 只承载 Tauri 壳、桌面 facade、资源与子进程生命周期；不得重新实现后端领域逻辑 |
| `apps/web`、`workers/pineworker`、`workers/marketdata-sidecar` | 保留原目录 | 全程 | 通过稳定公开 API 或版本化私有协议接入 Rust，不复制进 `crates/` |

#### 依赖方向与 crate 内部结构

规范依赖层次如下；箭头表示从稳定内层向外层扩展，**右侧可以依赖左侧，左侧不得依赖右侧**：

```text
jftrade-kernel / jftrade-broker
              -> 业务能力 crates
              -> store / integration / jftrade-api
              -> jftrade-engine
              -> apps/desktop/src-tauri
```

- `jftrade-kernel` 不依赖任何 workspace crate；只接纳确实跨两个以上能力、没有独立业务 owner 且拥有兼容 fixture 的基础值类型。
- `jftrade-broker` 只定义 broker-neutral 模型和 ports，不依赖具体券商、网络 client、SQLite、transport 或应用装配。
- 每个业务能力 crate 只依赖 `kernel`、必要的 broker-neutral 契约及纯算法库；不得直接依赖其他能力的 service。跨领域调用由消费方定义窄 port，并在 `jftrade-engine` 注入实现，禁止通过直接 import 形成环。
- `jftrade-contracts` 只持有显式版本化的私有 RPC/wire DTO、canonical codec 和边界兼容逻辑；它不是领域模型、共享状态或通用工具箱。领域 crate 不得为了复用 wire 类型而依赖它。
- store、integration 与 API 位于同一外层 adapter 级别，可以依赖对应领域 crate，但彼此不得直接依赖；协议 DTO、SQL row 和领域模型必须通过显式 mapper 转换。
- `jftrade-engine` 是唯一允许装配多个能力、store、integration 和 transport 的 Rust crate；feature flag、shadow、切换和回退选择只能出现在这里。
- Tauri 壳只依赖 `jftrade-engine` 暴露的应用生命周期 facade，不直接构造 store、Provider、Assistant runtime 或交易实现。

业务能力 crate 内按实际内容使用 `model`、`ports`、`service` 模块；不得为了对称预建空目录。适配 crate 按具体协议或 Provider 分模块，并保留显式 mapper。小范围单元测试与实现就近放置，跨实现行为测试放在 crate 的 `tests/`，只有可重复、具有代表性数据 manifest 的性能测试才进入 `benches/`。版本放在协议路径或 DTO 名称中，不创建 `v2` crate。

禁止创建无明确 owner 的 `common`、`shared`、`utils`、`helpers`、`misc`、`legacy`、`new`、`v2` 等 crate 或顶层模块。可复用代码必须先确定业务 owner；确属通用基础类型时仍需满足 `jftrade-kernel` 的准入条件。

#### 新目录与 crate 创建门禁

新增上述计划目录或任何 Rust crate 前，必须同时满足：

1. 对应阶段已经启动，并在阶段账本登记现有 Go owner、目标 Rust owner、唯一切换点、回退方式和 Go 删除条件。
2. 同一变更包含实际生产代码与描述业务行为的测试；禁止空 crate、占位模块、只含类型别名的转发层和未使用依赖。
3. 同步根 `Cargo.toml`、`scripts/module-map.json`、affected tests、依赖审计配置和本文件阶段账本；新增生成契约时同时登记固定生成器和只读 drift 检查。
4. 新直接依赖继续遵守“官方优先、其次高采用项目”、精确锁定、最小 feature、许可证/MSRV/平台/维护记录审查，并通过 `cargo-deny`。
5. 一个能力默认先使用一个 crate；只有依赖隔离、独立进程生命周期、平台/协议隔离，或由实际依赖图和编译数据证明需要边界时才继续拆分，并先在本节登记新 owner 和允许依赖。
6. 未列入蓝图的目录必须先更新本指引并说明用途、owner、依赖方向、最早阶段和删除/合并条件；不得先实现再补文档。
7. 本指引服从根目录及更深层 `AGENTS.md`。能力完成正式切换前，当前 Go 架构仍是生产事实源，Rust shadow 仍只读且不得取得第二写 owner。

本阶段只建立文档约束，不增加占位目录或自动检查。首次新增第二个 Rust crate 时，必须在同一变更加入目录门禁脚本，至少校验 workspace crate 名称/路径允许清单、阶段登记、禁止名称和上述依赖层次，并接入 `test:affected` 与 `check:all`。

## 4. 迁移守则

### 4.1 所有权和切换

1. **先契约、后实现、再切流量、最后删 Go。** 每个能力都必须有输入、输出、错误、持久化和生命周期账本。
2. **禁止按目录机械翻译。** 按领域能力和 owner 迁移，Rust 模块不得复刻现有跨层依赖。
3. **禁止双写。** shadow 只读；涉及数据库、交易、订阅、通知、审批或 artifact 的命令只能交给一个 owner。
4. **切换单位必须可回退。** feature flag 或 composition 选择只能位于装配层，不散落在领域代码。
5. **回退不降级数据。** 在 Rust 写入新格式前，Go 必须仍能读；需要不可逆 schema 时，先做双读验证并单独发布 migration。
6. **没有删除门禁就不复制实现。** 每个 Rust port 必须同时登记对应 Go 删除条件，避免永久双栈。

### 4.2 契约和数据

7. 金额、价格、数量、费率不得用二进制浮点替代现有 Decimal 语义；序列化、舍入、比较和 SQLite codec 必须有 golden fixture。
8. 时间必须明确 timezone、精度、单调/墙钟用途和空值语义；不得依赖本机 locale。
9. enum 的 unknown 值、JSON 的 omitted/null/empty、错误码与 HTTP 状态必须逐项映射，不凭“语义相近”放行。
10. SQL migration 仍是单一有序账本；Rust 不得另建同名但分叉的 migration 系统。
11. 并发、事务、busy timeout、WAL checkpoint、崩溃恢复和幂等键属于兼容契约，不只是实现细节。
12. 生成代码只由固定版本生成器产生并由只读检查验证，禁止手改。

### 4.3 Rust 工程质量

13. 默认 `#![forbid(unsafe_code)]`。确需 `unsafe` 时只能放入最小隔离 crate，必须有 ADR、安全不变量、Miri/平台测试和独立评审；不得为性能猜测放开全局策略。
14. 所有后台任务必须有 owner、取消信号、join、错误上报和关闭时限；禁止 detached task 吞错。
15. 生产代码不得依赖 `panic!`、`unwrap` 或 `expect` 处理外部输入、I/O、协议和持久化错误。
16. 使用强类型错误并在 transport 边界统一映射；日志不得成为调用方判断成功的协议。
17. 跨任务共享优先消息传递或窄锁；锁不得跨 `.await`，并发上限和背压必须显式。
18. Rust workspace 保持领域叶子依赖 composition root 的单向图；transport、数据库驱动和 OpenD protobuf 不得反向进入纯领域 crate。

### 4.4 依赖治理

19. 首选语言/项目官方维护库；没有官方方案时，才选维护活跃、采用广泛、源码清晰的高星项目。Star 只用于同等候选排序，不能替代安全、许可证、维护和契约适配评估。
20. 新直接依赖必须记录 owner、用途、替代方案、许可证、MSRV、平台支持、最近维护、安全记录和 feature 集；默认关闭不需要的 feature。
21. workspace 集中声明直接依赖并精确锁定版本；提交 `Cargo.lock`，禁止 wildcard 和未审查的 Git revision。
22. `cargo-deny` 阻止 yanked、未知 registry/git source、未允许许可证和 wildcard；重复版本先告警，热点/大型依赖再逐步收紧。
23. 每阶段只引入正在使用的依赖。未来候选不会提前进入 lockfile，避免供应链和编译成本先于价值发生。

### 4.5 测试与证据

24. 每个 port 至少有 Rust 单测、现有 Go fixture 重放、Go/Rust differential、拒绝/超时/取消/恢复测试和切换测试。
25. 对纯领域算法增加 property test；对 parser、codec 和 wire 输入增加 fuzz；涉及 `unsafe` 再加 Miri。
26. 外部 Futu、Yahoo、AKShare 和模型 Provider 只在显式 live workflow 验证；普通 CI 使用 fixture、mock server 或 testkit。
27. “本地门禁通过”不能表述成“上游已验证”。真实网络、发布包和四平台结果分别报告。
28. 一个阶段只有在验收账本的证据可复现、回退演练成功、资源门禁通过后才能关闭。

## 5. 依赖甄选基线

依赖版本以 2026-08-19 的 crates.io/上游发布为筛选快照。只有“阶段 1 已引入”进入当前 `Cargo.lock`；其余是后续阶段首选候选，引入时必须重新核验。

| 能力 | 选择 | 状态 | 理由与约束 |
| --- | --- | --- | --- |
| async runtime | [Tokio 1.53.1](https://github.com/tokio-rs/tokio) | 阶段 1 已引入 | Tonic 官方运行时基础；只启用 macros/net/rt/signal/sync |
| private RPC/health | [Tonic / tonic-health 0.14.6](https://github.com/hyperium/tonic) | 阶段 1 已引入 | Tokio 生态官方 gRPC 实现；标准 health 协议避免自造 contract |
| serialization | [Serde 1.0.229](https://github.com/serde-rs/serde) / `serde_json` 1.0.151 | 阶段 1 已引入 | Rust 事实标准；当前仅用于 supervisor readiness JSON |
| error | [thiserror 2.0.20](https://github.com/dtolnay/thiserror) | 阶段 1 已引入 | 库层强类型错误；应用汇总是否引入 `anyhow` 后续按需决定 |
| observability | [tracing 0.1.44](https://github.com/tokio-rs/tracing) / subscriber 0.3.23 | 阶段 1 已引入 | Tokio 官方结构化诊断生态；stdout 保留给握手，日志走 stderr |
| dependency policy | [cargo-deny 0.20.2](https://github.com/EmbarkStudios/cargo-deny) | 阶段 1 CI 工具 | 审计 advisory、license、source 和 ban；不进入产品依赖 |
| HTTP/SSE/WS | [Axum 0.8.9](https://github.com/tokio-rs/axum) | 后续候选 | Tokio 官方生态、与 Tower/Tonic 组合自然；API 阶段才引入 |
| SQLite | [SQLx](https://github.com/launchbadge/sqlx) | 后续候选 | 优先验证 0.9 的 SQLite 行为和编译成本；若无法精确保留 SQLite 语义，比较 `rusqlite` 后再定 |
| HTTP client/TLS | [Reqwest](https://github.com/seanmonstar/reqwest) + [Rustls 0.23 stable](https://github.com/rustls/rustls) | 后续候选 | 默认纯 Rust TLS；不采用当前 0.24 dev release，避免把预发布版本带入产品 |
| Decimal | [rust_decimal 1.42.1](https://github.com/paupino/rust-decimal) | 后续候选 | 金融定点数候选；必须先通过 Go Decimal/SQLite/JSON golden corpus |
| time/identity | [time 0.3.55](https://github.com/time-rs/time) / [uuid 1.24.1](https://github.com/uuid-rs/uuid) | 后续候选 | 仅按现有精度和 UUID 编码启用 feature，不提前引入 chrono 双栈 |
| CPU parallelism | [Rayon 1.12.0](https://github.com/rayon-rs/rayon) | 后续候选 | 只用于有基准证据的批量纯计算；不得与 Tokio task 无界叠加 |
| Assistant | [Rig 0.42.0](https://github.com/0xPlaygrounds/rig) | 已锁定、后续引入 | 用户指定；先建 provider/tool/session/approval 行为矩阵，再接生产模型 |
| desktop | [Tauri 2.11.5](https://github.com/tauri-apps/tauri) | 已锁定、后续引入 | Rust 桌面主流方案；先复制 Wails facade 和四平台发布语义，再删除 Wails |

明确暂不选择：

- 不在阶段 1 引入 Axum、SQLx、Rig、Tauri、Rayon 或 Decimal，只为未来“占位”。
- 不选择 Rustls `0.24.0-dev.*` 进入产品基线。
- 不引入第二个 async runtime、第二个 HTTP server 或通用 service locator。
- 不用 `libloading`/原生 ABI 直接嵌 Go；跨语言共存统一走私有进程 RPC，降低崩溃域和构建耦合。

## 6. 分阶段执行方案

### 阶段 1：Rust 工程与共存基础（当前）

目标：建立可持续演进但不改变产品行为的 Rust 基础。

- [x] 固定 Rust `1.97.1`、edition 2024、workspace、Clippy/rustfmt 和 release profile。
- [x] 建立 `crates/jftrade-engine`，默认禁止 unsafe。
- [x] 建立 loopback-only、per-process Bearer 认证的 Tonic health bridge。
- [x] readiness 仅输出一行 machine-readable JSON，诊断日志只写 stderr，避免 supervisor 协议被污染。
- [x] 覆盖公开监听拒绝、弱令牌拒绝、未认证拒绝、认证 health 和 graceful shutdown。
- [x] 建立精确直接版本、`Cargo.lock`、`deny.toml` 和依赖许可/source 策略。
- [x] 接入 `test:affected`、`check:quick`、`check:all` 与模块所有权。
- [x] CI 增加 Rust quality 和 Linux x64/macOS ARM64/Windows x64/Windows ARM64 编译矩阵；四个 target 已在当前 macOS 主机完成 cross-check。
- [ ] 合并前由上游原生 runner 完成首次四平台矩阵；本地 cross-check 不能替代原生平台资格。
- [ ] 下一阶段前补齐代表性基准数据集的不可变 manifest；该工作不阻塞无业务流量的阶段 1 bridge。

阶段 1 放行条件：现有公开契约与生成资产零 diff；Rust fmt/clippy/test/policy 全绿；四目标 `cargo check` 全绿；上游原生 runner 矩阵全绿；Go/Wails 仍是唯一生产入口。在首次上游矩阵完成前，阶段 1 不标记关闭。

### 阶段 2：共享领域模型、codec 与 SQLite 只读验证

1. 从现有 DTO、Decimal、time、enum、error 和 SQLite codec 生成兼容账本及 golden corpus。
2. 建立无 transport/driver 依赖的 Rust domain crates；先 port 纯转换和校验。
3. 建立 SQLite read-only adapter，复放真实匿名化快照；比较 schema、PRAGMA、查询结果和排序。
4. 建立 differential runner，输入同一 fixture，输出 canonical JSON/事件 trace。
5. 不写生产数据库，不切换 API。

放行：golden/differential 零未解释差异；只读打开不会修改数据库；错误和 Decimal/时间语义完整；资源基线完成。

### 阶段 3：回测与批量计算核心

1. 先迁移撮合、成交、资金曲线、费用、风控和指标等纯计算热点。
2. 保留 PineTS worker；Go 和 Rust 消费同一 order intents/candle corpus。
3. 对确定性、随机种子、排序、浮点/Decimal、并发和取消做 property/differential 测试。
4. shadow 运行代表性策略，记录吞吐、p50/p95、峰值 RSS、分配和结果 hash。
5. 由 Go 装配层按能力切换 owner，Rust 仍不直接暴露公开 HTTP。

放行：结果与现有 execution model 一致；错误/取消/超时恢复一致；性能资源门禁通过；可一键切回 Go 且无数据迁移。

### 阶段 4：行情 Provider、Pine/Python worker 生命周期

1. 迁移 broker-neutral market-data domain、demand/freshness/cache 和 provider router。
2. 迁移 Node PineTS 与 Python helper 的启动、令牌、端口、ready、重试、退避、停止和资产校验。
3. 迁移 yfinance/AKShare loopback HTTP adapter，但不重写 Python provider 语义。
4. 最后迁移 Futu/OpenD 协议 adapter、订阅恢复和 quote-login 状态。

放行：provider capability 矩阵一致；未知健康 fail closed；切换无陈旧订阅/缓存；真实 live workflow 单独通过。

### 阶段 5：交易、策略运行与通知

1. 迁移 broker/session、订单命令、更新流、风控、账户刷新和通知。
2. 先 paper/fixture，再只读 OpenD，再显式小范围 live；所有命令带幂等键和审计 trace。
3. 故障注入断网、重连、重复事件、乱序、部分成功和关闭竞争。
4. 交易写路径只允许单 owner，shadow 只比较计划，不发送第二份订单。

放行：订单/成交/持仓状态机 differential 通过；无双单；崩溃恢复和审计一致；人工 live checklist 签字。

### 阶段 6：Assistant 使用 Rig 迁移

1. 先 port model、session/run、approval/input、lease/claim、artifact 和 audit 的纯状态与持久化。
2. 为 Rig 建立窄 adapter，保留 JFTrade 自己的 provider/tool/workflow 契约，避免业务模型泄露 Rig 类型。
3. 逐项覆盖 tool schema、审批暂停/恢复、pending input、任务图、流式 delta、usage、错误与重启恢复。
4. 使用 fake provider 和固定 transcript 做 differential，再进行显式 provider live smoke。

放行：终态和持久化完全一致；审批/输入不能丢失或重复；模型网络异常可恢复；Rig 可替换性由 adapter 测试保证。

### 阶段 7：Rust API/control plane 成为产品 owner

1. 引入 Axum/Tower，复制 `/api/v1/*`、SSE、WebSocket、安全中间件、static assets 和 observability。
2. 使用现有 OpenAPI baseline 反向约束实现，不因 Rust 类型便利改变 wire schema。
3. 双进程 replay 后按 route group 切流量，最终 Rust 接管 API sidecar 和应用 lifecycle。
4. Go 暂时只保留桌面壳和必要回退，不再运行已切换领域逻辑。

放行：全量契约/differential/Web 测试通过；生产 bundle 和真实 sidecar smoke 通过；启动、关闭、端口和安全边界一致。

### 阶段 8：Wails → Tauri 桌面迁移

1. 在 Vue 侧建立稳定 desktop facade，逐项映射启动状态、链接、日志、更新、窗口、菜单和单实例。
2. Tauri 管理 Rust API、PineTS 和 Python helper 的发布资产与生命周期。
3. 复制开发/正式数据隔离、端口、版本注入、签名/安装器和四平台资源。
4. 同一前端 bundle 分别在 Wails/Tauri 做行为对照，切换后删除 Wails bindings 生成链。

放行：macOS ARM64、Linux x64、Windows x64/ARM64 打包与安装 smoke；无黑屏；退出无孤儿进程；升级不丢数据。

### 阶段 9：Go 删除与 Rust 大版本发布

1. 确认每个领域的 Rust owner、回退窗口、数据兼容和线上观察期已经关闭。
2. 删除 Go/Wails 入口、Go module、生成器和只服务旧实现的 adapter；同步文档、CI、license 和发布脚本。
3. 对公开 `pkg/*` 发布 hard-cut 迁移说明，不承诺 Rust major 版本继续提供 Go import compatibility。
4. 完成独立安全审查、SBOM、许可证、漏洞、恢复演练和四平台 release candidate 验收。

放行：仓库不再构建或运行 Go/Wails；所有产品能力由 Rust/Tauri + 保留的 Node/Python worker 提供；旧数据原地升级与备份恢复均通过。

## 7. 每个能力的标准工作包

每次迁移 PR/提交必须按同一顺序完成：

1. 建立契约账本：调用方、输入、输出、错误、事件、SQL、生命周期和 owner。
2. 冻结 fixture/golden：包含成功、拒绝、边界、损坏数据、取消和恢复。
3. 在纯 Rust 叶子 crate 实现领域规则，禁止先从 transport 开始。
4. 实现 adapter，并确保依赖方向不反转。
5. 跑单实现测试、property/fuzz 和 Go/Rust differential。
6. 以只读 shadow 收集真实代表性 trace 和资源数据。
7. 在 composition root 加单一切换点，演练前滚和回退。
8. 切换唯一 owner，观察稳定窗口。
9. 删除 Go 重复实现、flag 和临时 bridge；收紧预算和依赖。
10. 更新架构事实、模块表、运行手册和阶段账本。

### 7.1 分步提交与阶段收口（强制）

每个阶段必须在阶段内按可独立审查、可独立回退的步骤提交；阶段完成时不得只留下一个混合所有工作的巨型提交，也不得残留未提交的阶段文件。默认提交序列为：

1. `docs/test`：先提交契约账本、fixture/golden、基准 manifest 和拒绝/恢复测试，固定要保留的行为。
2. `feat`：提交纯领域模型与实现，不混入 transport、store、integration 或下一阶段代码。
3. `feat`：按边界分别提交 store/integration/transport adapter、differential/shadow 和 composition 切换点；不同写 owner 不得放进同一提交。
4. `chore/test`：提交 affected gate、依赖治理、CI、打包和平台验证；生成文件必须跟随其契约源提交，不能单独补交。
5. `docs`：只有全部阶段门禁通过后，才提交阶段账本、证据摘要和关闭状态；仍等待上游、live 或平台资格时必须明确标记未关闭。

每个提交都必须只包含当前步骤和不可分割的测试，提交前至少通过该步骤最窄门禁；最后一个阶段提交前必须通过 `check:quick`、该阶段专项门禁和 `check:all`。提交信息使用 `docs|test|feat|fix|chore(rust-stageN): <具体行为>`，不得使用 `wip`、`misc`、`update` 等无法表达 owner 或行为的标题。迁移提交只在本地创建，除非用户明确要求，不推送、不重写已共享历史；回退优先逐提交 revert，不靠双写或长期兼容分叉。

## 8. 性能与资源门禁

每个阶段记录同一机器、同一数据、同一 build profile 的 Go/Rust数据。至少包括：

- 吞吐、p50/p95/p99、CPU time、峰值 RSS、稳态 RSS、启动到 ready、关闭耗时。
- 回测按 candle/strategy 规模分层；行情按消息速率/订阅数；Assistant 按 session/tool 轨迹；SQLite 按真实查询和迁移快照。
- Debug 数据不能用于产品结论；使用 release binary，预热和重复次数写入 manifest。

硬门禁：

- 任何代表性关键路径 p95 不得无解释回退超过 5%。
- 峰值或稳态 RSS 不得无解释增长超过 10%。
- 启动、关闭、取消、恢复不得超出现有产品 timeout 或产生孤儿进程。
- 纯计算热点目标为至少 1.5 倍吞吐或 30% RSS 降低；未达到目标时可继续修正，但不得用微基准掩盖端到端回退。
- 如兼容性、稳定性或资源任一门禁失败，该能力维持 Go owner，直到修复并重新验收。

## 9. 阶段 1 使用与安全边界

本地质量门禁：

```bash
pnpm run check:rust
pnpm run check:rust:policy  # 需要 cargo-deny 0.20.2
```

仅用于开发验证的 engine 启动方式：

```bash
JFTRADE_RUST_ENGINE_TOKEN="$(openssl rand -hex 32)" \
  cargo run -p jftrade-engine
```

进程 ready 后 stdout 只输出一行类似：

```json
{"event":"ready","address":"127.0.0.1:54321","protocolVersion":"migration.v1","healthService":"jftrade.migration.v1.Engine"}
```

阶段 1 禁止：让前端连接该端口、使用固定弱令牌、监听非 loopback、写业务数据库、控制 Pine/Python/Futu、替换 Go API 或打入正式桌面发布资产。

## 10. 决策与阶段账本

| 日期 | 决策 | 证据/状态 |
| --- | --- | --- |
| 2026-08-19 | 完整迁移 Go/Wails；保留 Vue、Node PineTS、Python helper；Assistant 选 Rig | 用户确认 |
| 2026-08-19 | 阶段 1 仅建 Rust/coexistence 基础，Go/Wails 保持生产 owner | `crates/jftrade-engine` 与本文 |
| 2026-08-19 | health bridge 使用 Tonic 官方标准协议，不新增自定义 proto | 最小依赖、无生成器、跨平台可编译 |
| 2026-08-19 | Rust 私有 listener 强制 loopback + 每进程 Bearer，未认证 fail closed | 单元与集成测试 |

完成每个阶段时在本表追加最终决策；大量一次性测试日志留在 CI artifact/提交，不复制进长期文档。
