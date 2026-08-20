# JFTrade Go → Rust 完整迁移方案与守则

状态：执行中。更新时间：2026-08-19。当前阶段：**阶段 8 Tauri desktop facade 本地 shadow 工作包已完成；Wails 仍是唯一生产桌面 owner，Tauri 不启动原生 WebView、不接管产品子进程或发布资产；四平台打包/安装、同 bundle 黑屏对照、签名、升级与孤儿进程观察窗口仍待闭环，阶段 9 尚未启动**。

本文是 JFTrade 将 Go 后端与 Wails 桌面壳完整迁移到 Rust 的计划、边界和放行事实源。活动状态在 [roadmap.md](../roadmap.md) 汇总；当前生产架构仍以 [architecture.md](../architecture.md) 为准。任何阶段都不得用“已经写出 Rust 版本”代替兼容性、可靠性和资源验收。

## 1. 已锁定目标

最终产品形态：

- Rust 接管当前 Go 后端、应用装配、领域服务、SQLite store、Futu/OpenD 适配、HTTP/SSE/WebSocket、进程生命周期和桌面壳。
- Vue 3 控制台保留，现有 `/api/v1/*` 调用方式和用户行为保持兼容。
- Node PineTS worker 保留，仍只负责 Pine 执行、信号、图形和 order intents。
- Python market-data helper 保留，仍封装 yfinance 与 AKShare；Rust 只替换它的宿主、鉴权、生命周期和 Provider adapter。
- Assistant 的 Rust 实现使用 [Rig](https://github.com/0xPlaygrounds/rig)；阶段 6 已在窄 adapter 中引入其 provider-neutral core 类型，但未引入具体 Provider client、未切换生产模型流量。
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

本节是迁移期间新增目录、crate 和跨目录依赖的强制放置规则。目录出现在蓝图中只表示名称、owner 和最早启用阶段已经预留，**不表示应立即创建**。阶段未启动、没有实际生产代码或没有行为测试时，不得创建空 crate、占位模块或未来依赖。当前 Rust 侧已经启用 `jftrade-engine`、`jftrade-kernel`、`jftrade-broker`、`jftrade-store-sqlite`、`jftrade-backtest`、`jftrade-marketdata`、`jftrade-strategy`、`jftrade-trading`、`jftrade-assistant`、`jftrade-research`、`jftrade-watchlist`、`jftrade-settings`、`jftrade-calendar`、`jftrade-datamanagement`、`jftrade-api`、`jftrade-integration-pine`、`jftrade-integration-marketdata-helper`、`jftrade-integration-futu` 和 `apps/desktop/src-tauri`；其余目标目录仍是计划目录。

```text
crates/
  jftrade-kernel/                         # 已存在，阶段 2：Decimal、时间、ID 等纯基础类型
  jftrade-contracts/                      # 计划，阶段 2：版本化私有 RPC/wire DTO
  jftrade-broker/                         # 已存在，阶段 2：broker-neutral 类型与 ports
  jftrade-backtest/                       # 已存在，阶段 3：回测领域和计算核心
  jftrade-marketdata/                     # 已存在，阶段 4：行情领域
  jftrade-strategy/                       # 已启用，阶段 5：策略运行控制和消费方交易 port
  jftrade-trading/                        # 已启用，阶段 5：交易、风控和订单状态
  jftrade-assistant/                      # 已启用，阶段 6：Assistant 领域与 Rig adapter 边界
  jftrade-research/                       # 已启用，阶段 7：研究能力
  jftrade-watchlist/                      # 已启用，阶段 7：自选领域
  jftrade-settings/                       # 已启用，阶段 7：设置领域
  jftrade-calendar/                       # 已启用，阶段 7：交易日历领域
  jftrade-datamanagement/                 # 已启用，阶段 7：数据维护能力
  jftrade-store-sqlite/                   # 已存在，阶段 2：SQLite 只读 adapter
  jftrade-integration-pine/               # 已存在，阶段 4：Node worker adapter/lifecycle
  jftrade-integration-marketdata-helper/  # 已存在，阶段 4：Python helper adapter/lifecycle
  jftrade-integration-futu/               # 已存在，阶段 4/5：OpenD 协议 adapter
  jftrade-api/                            # 已启用，阶段 7：Axum HTTP/SSE/WebSocket
  jftrade-engine/                         # 已存在：进程入口和唯一 composition root

apps/
  web/                                    # 已存在并保留：Vue 控制台
  desktop/src-tauri/                      # 已启用，阶段 8：Tauri facade、桌面契约与受管生命周期

workers/
  pineworker/                             # 已存在并保留：Node PineTS worker
  marketdata-sidecar/                     # 已存在并保留：Python market-data helper

proto/jftrade/migration/v1/               # 计划，首个自定义私有 RPC 出现时创建
tests/fixtures/rust-migration/<capability>/ # 已启用，按能力保存 golden/differential corpus
scripts/rust-migration/                   # 已启用，differential、benchmark 和目录门禁工具
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

目录门禁已经在阶段 2 随第二个 Rust crate 落地，至少校验 workspace crate 名称/路径允许清单、阶段登记、禁止名称、上述依赖层次，以及生产代码和行为测试非空，并已接入 `test:affected` 与 `check:all`。后续 crate 必须先更新 `scripts/rust-migration/layout-policy.json` 和本节账本，再创建实现。

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

依赖版本以 2026-08-19 的 crates.io/上游发布为筛选快照。状态列标明实际引入阶段；其余是后续阶段首选候选，引入时必须重新核验。

| 能力 | 选择 | 状态 | 理由与约束 |
| --- | --- | --- | --- |
| async runtime | [Tokio 1.53.1](https://github.com/tokio-rs/tokio) | 阶段 1 已引入、阶段 4 扩展 | Tonic/Reqwest 运行时基础；阶段 4 为受管 worker 增加 io-util/process/time，仍不启用无关组件 |
| private RPC/health | [Tonic / tonic-health 0.14.6](https://github.com/hyperium/tonic) | 阶段 1 已引入 | Tokio 生态官方 gRPC 实现；标准 health 协议避免自造 contract |
| serialization | [Serde 1.0.229](https://github.com/serde-rs/serde) / `serde_json` 1.0.151 | 阶段 1 已引入 | Rust 事实标准；当前仅用于 supervisor readiness JSON |
| error | [thiserror 2.0.20](https://github.com/dtolnay/thiserror) | 阶段 1 已引入 | 库层强类型错误；应用汇总是否引入 `anyhow` 后续按需决定 |
| observability | [tracing 0.1.44](https://github.com/tokio-rs/tracing) / subscriber 0.3.23 | 阶段 1 已引入 | Tokio 官方结构化诊断生态；stdout 保留给握手，日志走 stderr |
| dependency policy | [cargo-deny 0.20.2](https://github.com/EmbarkStudios/cargo-deny) | 阶段 1 CI 工具 | 审计 advisory、license、source 和 ban；不进入产品依赖 |
| HTTP/SSE/WS | [Axum 0.8.9](https://github.com/tokio-rs/axum) / [Tower 0.5.3](https://github.com/tower-rs/tower) / [tower-http 0.7.0](https://github.com/tower-rs/tower-http) | 阶段 7 已引入 | Tokio/Tower 官方生态且广泛采用；Axum 只启用 HTTP/1、JSON、route/query、Tokio 与 WebSocket，tower-http 只启用 trace，关闭默认 feature；transport 通过 JFTrade `ApiPort` 接入，不让框架类型进入领域 crate |
| SQLite | [rusqlite 0.40.2](https://github.com/rusqlite/rusqlite) | 阶段 2 已引入 | 阶段 2 只读验证需要精确控制 open flags、PRAGMA 和 schema introspection；关闭默认 feature，仅启用 `bundled`，避免目标机系统 SQLite 漂移。异步事务 owner 阶段再重新比较 SQLx |
| loopback HTTP client | [Reqwest 0.13.4](https://github.com/seanmonstar/reqwest) | 阶段 4 已引入 | 项目官方且广泛采用；仅为本机 Python helper 启用 `json`，关闭默认 feature、TLS、proxy、cookie 和 HTTP/2；显式端口、禁重定向、限时限长并可选 per-process Bearer |
| protocol/asset digest | [RustCrypto SHA-1/SHA-2 0.11.0](https://github.com/RustCrypto/hashes) | 阶段 4 已引入 | OpenD wire 固有 SHA-1 与发布资产 SHA-256；官方 RustCrypto 实现、关闭默认 feature，不用于密码存储或自造认证协议 |
| Decimal | 自有兼容 codec；[rust_decimal 1.42.1](https://github.com/paupino/rust-decimal) 仅保留为有界算术候选 | 阶段 2 已决策 | shopspring Decimal 是任意精度字符串语义，不能无损收窄到 `rust_decimal`；bbgo fixedpoint 另按 `i64 × 10^-8` 实现。只有领域边界已证明在 96-bit 范围内时才允许引入 `rust_decimal` |
| time/identity | [time 0.3.55](https://github.com/time-rs/time) / [uuid 1.24.1](https://github.com/uuid-rs/uuid) | `time` 阶段 2 已引入；UUID 后续候选 | `time` 仅启用 std/formatting/parsing/serde，保留 RFC3339Nano 与 Unix 毫秒语义；UUID 未被阶段 2 代码使用，不提前引入 |
| CPU parallelism | [Rayon 1.12.0](https://github.com/rayon-rs/rayon) | 后续候选 | 只用于有基准证据的批量纯计算；不得与 Tokio task 无界叠加 |
| Assistant | [Rig Core 0.42.0](https://github.com/0xPlaygrounds/rig) | 阶段 6 已引入 | 官方仓库、MIT；精确锁定且关闭默认 feature，只在 `rig_adapter` 内使用 provider-neutral request/message/tool 类型；不启用 Rig 的 `reqwest` 增强、derive、rustls 或具体 Provider feature（core 自身仍含最小 HTTP/stream 基础依赖），不让 Rig 类型进入 JFTrade 持久化模型或 ports |
| desktop | [Tauri 2.11.5](https://github.com/tauri-apps/tauri) / [`@tauri-apps/api` 2.11.1](https://github.com/tauri-apps/tauri) | 阶段 8 已引入 | 官方 Rust/JS 包；Rust crate 关闭全部默认 feature，仅使用 Builder/command/state 边界，Vue 通过官方 `invoke`/`listen` 接入；本地 shadow 不启用 Wry/WebView、tray、updater 或 native TLS，待原生切片实际使用时再逐项开启 |

阶段 5 未增加第三方依赖；`jftrade-trading` 和 `jftrade-strategy` 只复用已审计的 `jftrade-kernel`、`jftrade-broker`、Serde 与 thiserror，OpenD 交易 shadow 复用阶段 4 的 adapter crate，避免在无真实 wire owner 前引入第二套 broker SDK 或持久化框架。

阶段 6 新增的唯一直接第三方依赖是 `rig-core = 0.42.0`。选择官方 Rig core crate 而非完整 facade/具体 Provider SDK，保留 JFTrade 自有 `CompletionPort`、tool schema、session/run/approval/input/workflow 契约；通过 `cargo-deny` 检查 advisory、license、source 和重复版本。真实 Provider 适配需要在显式 live 工作包中重新审查 TLS、credential、timeout、rate limit、stream cancellation 与 telemetry content 策略。

阶段 7 新增 Axum 0.8.9、Tower 0.5.3 与 tower-http 0.7.0，均来自 Tokio/Tower 官方维护生态，并继续精确锁定版本、关闭默认 feature、只开启实际生产代码使用的最小功能；`http-body-util` 仅用于 transport 行为测试。研究、自选、设置、日历和数据维护 crate 没有引入第二套 async runtime、数据库驱动、Provider SDK 或通用 service 框架。

阶段 8 新增的直接依赖只有 Tauri 官方 `tauri = 2.11.5` 与 `@tauri-apps/api = 2.11.1`。Rust 端默认 feature 全关，不把尚未运行的 Wry、tray、updater、system shell 或 TLS 提前带入产品；JS 端只在检测到 Tauri runtime 时动态加载 `core.invoke` 和 `event.listen`，当前 Wails binding 仍是生产 adapter。`cargo-deny` 审计确认 Tauri 的四目标解析仍带入 Linux GTK3 与 `urlpattern` 的 13 个“unmaintained”信息公告（没有 vulnerability/unsound 公告）；`deny.toml` 只按 advisory ID 登记官方传递图例外，并只对 7 个精确 crate/version 放行 MPL-2.0、Zlib 或 Apache-2.0 WITH LLVM-exception，禁止把许可证全局放宽。Tauri/传递版本变化时未命中的例外会告警并必须重新审查；原生 feature 仍须跟随对应平台实现和测试一起引入。

明确暂不选择：

- 不提前引入 SQLx、Rayon 或 Decimal，只为未来“占位”；Rig、Axum/Tower 与 Tauri 已分别在阶段 6/7/8 按实际 adapter 和行为测试引入。
- 不选择 Rustls `0.24.0-dev.*` 进入产品基线。
- 不引入第二个 async runtime、第二个 HTTP server 或通用 service locator。
- 不用 `libloading`/原生 ABI 直接嵌 Go；跨语言共存统一走私有进程 RPC，降低崩溃域和构建耦合。

## 6. 分阶段执行方案

### 阶段 1：Rust 工程与共存基础

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
- [x] 阶段 2/3/4/5 代表性数据集均已建立不可变 SHA-256 manifest；行为演进必须新增 corpus 版本。

阶段 1 放行条件：现有公开契约与生成资产零 diff；Rust fmt/clippy/test/policy 全绿；四目标 `cargo check` 全绿；上游原生 runner 矩阵全绿；Go/Wails 仍是唯一生产入口。在首次上游矩阵完成前，阶段 1 不标记关闭。

### 阶段 2：共享领域模型、codec 与 SQLite 只读验证（本地完成）

- [x] 从现有 DTO、Decimal、time、enum、error 和 SQLite codec 生成兼容账本及 golden corpus。
- [x] 建立无 transport/driver 依赖的 `jftrade-kernel` 与 `jftrade-broker`；只 port 纯转换和校验。
- [x] 建立 `jftrade-store-sqlite` read-only adapter，复放匿名化 backtest K 线快照；比较 schema、PRAGMA、查询结果和排序。
- [x] 建立 Go/Rust differential runner，输入同一 fixture，输出 canonical JSON，并与固定 expected JSON 三方比较。
- [x] 只读前后逐字节比较 SQLite 文件；未接入生产数据库、API 或 composition root。

放行：golden/differential 零未解释差异；只读打开不会修改数据库；错误和 Decimal/时间语义完整；资源基线完成。

#### 阶段 2 执行账本

| 工作包 | 当前 Go owner | 阶段 2 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| Decimal、fixedpoint、time codec | `shopspring/decimal` 使用方、`pkg/bbgo/fixedpoint`、`pkg/bbgo/types` | `jftrade-kernel` | 以后由 `jftrade-engine` 的版本化 DTO mapper 选择；阶段 2 不切流，回退仍走 Go | 全部消费能力通过同一 golden 且不再 import 被替代 codec | Rust codec 已实现并重放 v1 corpus；未切生产 owner |
| broker-neutral taxonomy/error | `pkg/broker` | `jftrade-broker` | 消费方窄 port 在 `jftrade-engine` 装配；Go 仍是生产 owner | 行情/交易消费方迁完，Provider adapter 不再暴露 Go broker contract | Rust taxonomy/error 已实现并重放 v1 corpus；未切生产 owner |
| backtest SQLite K 线只读 | `internal/store/backtest`、`internal/store/sqliteconn` | `jftrade-store-sqlite` | 阶段 2 仅 differential CLI，禁止接生产路径；删 Rust adapter 即回退 | 各能力已有唯一 Rust 写 owner、migration/rollback 证据且无 Go query/migration consumer | strict read-only、拒绝路径和三方 differential 已通过；未切生产 owner |

阶段 2 corpus 位于 `tests/fixtures/rust-migration/stage2`，manifest 中的 SHA-256 在该阶段关闭后不可静默修改；行为需要演进时新增 corpus 版本并记录差异。目录门禁位于 `scripts/rust-migration/check-layout.mjs`，从本阶段第二个 Rust crate 起强制校验 active/planned 状态、owner/切换/删除条件、允许路径、禁止名称、workspace 依赖和非空生产代码/行为测试。

阶段 2 本地资源基线使用同一匿名化 SQLite 文件、3 次预热、20 次 release 进程级读取；证据固定在 `resource-baseline.darwin-arm64.json`。Apple A18 Pro/macOS ARM64 上 Go p95 为 8.167 ms、峰值 RSS 13,762,560 bytes，Rust p95 为 5.885 ms、峰值 RSS 3,178,496 bytes，Rust/Go 比值分别为 0.721 和 0.231；两端 CPU 时间都低于 Darwin `/usr/bin/time` 的 10 ms 分辨率，因此不据此宣称 CPU 优势。该数据只证明本机基线，不替代 Linux/Windows/macOS 原生 CI 资格。

### 阶段 3：回测与批量计算核心（本地完成）

- [x] 建立无 transport、SQLite、Provider 或 worker 生命周期依赖的 `jftrade-backtest`，实现 `conservative-bar-v1` 的撮合、部分成交、资金/持仓、费用、已实现盈亏、资金曲线、回撤、SMA/EMA、止损/限价/OCO、reduce-only 与取消语义。
- [x] 保留 PineTS worker；Rust 只消费规范化 candle 与 order intents，不编译 Pine、不管理 Node 进程，也不取得第二个交易或数据库写 owner。
- [x] Go 参考由测试内直接调用现有私有 `conservativeBarExecutor`、`backtestFeeEngine` 和 `resultCollector`，不是另写一份简化 Go 算法。
- [x] 固定 5 个代表性 case、8 笔 fill，覆盖部分成交与费用/回撤、原子 bracket/OCO stop-first、stop-limit/滑点/做空反转、显式取消/reduce-only、运行取消与恢复；每个 case 固定 FNV-1a 结果 hash。
- [x] Rust 行为测试覆盖 corpus 三方一致、字节级确定性、损坏/截断输入拒绝、取消恢复、固定价格不同流动性分片下的资金守恒和指标边界。
- [x] differential 与 owner 演练进程均设置关闭时限；超时进程被终止，后续运行可恢复。`go / shadow / rust` 三态演练默认选择 Go，shadow 不一致时 fail closed，回退到 Go 不涉及数据迁移。
- [x] release 进程级基准使用同一已解析 corpus、3 次预热、20 次采样、每进程 5,000 case，并固定输入 SHA-256 与结果 hash。
- [x] 阶段 3 没有新增第三方依赖；复用已审计的 `serde`、`serde_json`、`thiserror` 和 `jftrade-kernel`，未提前引入 Rayon、RPC、SQLite 或 async runtime。

#### 阶段 3 执行账本

| 工作包 | 当前 Go owner | 阶段 3 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| `conservative-bar-v1` 撮合、费用、账户与结果计算 | `pkg/backtest` 的 `conservativeBarExecutor`、`backtestFeeEngine`、`resultCollector` | `jftrade-backtest` | `scripts/rust-migration/run-backtest-owner.mjs` 只用于离线 `go/shadow/rust` 演练，默认 Go；选择 `go` 即无状态回退 | 实际 Pine replay adapter 能把生产 candle/order/update 流无损映射到 Rust，公开回测行为与故障恢复通过观察期，且产品 composition root 已成为唯一 Rust owner | Rust 纯计算核心与本地证据完成；Go 仍是唯一生产 owner |
| PineTS 策略执行与 worker 生命周期 | `workers/pineworker`、Go `pkg/strategy/pineworker` 宿主 | `jftrade-integration-pine`（计划） | 阶段 3 不切换、不创建 crate；阶段 4 由 composition root 装配 | worker 鉴权、ready、超时、停止和发布资产完成跨平台验收 | 保留现状，不属于阶段 3 Rust 计算核心 |

阶段 3 corpus 位于 `tests/fixtures/rust-migration/stage3`。`manifest.json` 固定输入、expected 和 Darwin ARM64 资源基线的 SHA-256，修改既有文件会由 Go 门禁拒绝；语义演进必须新增版本。三方 differential 命令为 `pnpm run test:rust:backtest:differential`，owner 演练为 `pnpm run run:rust:stage3:owner -- --owner=shadow`，性能复测为 `pnpm run benchmark:rust:stage3`。

本机基线为 Apple A18 Pro/macOS ARM64：Go p95 161.608 ms、Rust p95 65.240 ms，Rust/Go 为 0.404；Go 峰值 RSS 39,895,040 bytes、Rust 2,768,896 bytes，Rust/Go 为 0.069；两端结果 hash 均为 `fnv1a64:050a2c89f71d3a2b`，5% p95 回退门禁、10% RSS 回退门禁和“1.5 倍吞吐或 30% RSS 降低”目标均通过。Go 数值包含测试 harness，因为生产 matcher 保持 package-private；因此吞吐与语义 hash 是主要比较依据，RSS/二进制大小只作偏保守上界，本机结果不替代后续原生平台资格。

本阶段“完成”只表示可复现的纯计算实现、兼容证据与切换演练已经闭环，不表示公开产品已切流。现有生产 Pine replay 是逐事件、长生命周期链路，阶段 3 不以测试 CLI 冒充产品 adapter；在阶段 4/7 建成真实生命周期和 composition 接缝前，Go 继续拥有撮合、费用和结果写入权，Rust 不连接公开 HTTP、生产 SQLite 或 worker。

### 阶段 4：行情 Provider、Pine/Python worker 生命周期（本地完成）

- [x] 建立 `jftrade-marketdata`，实现 broker-neutral Provider capability/constraint/health、instrument normalization、consumer demand/heartbeat/expiry、managed lease、generation cache/freshness 和 fail-closed provider router。
- [x] provider 显式切换要求 connected + ready；启动恢复可保留 warming，但 unknown 健康、poll-only 承担 managed streaming demand、managed demand 下切换均拒绝；成功切换递增 generation 并清空旧 cache。
- [x] 建立 `jftrade-integration-marketdata-helper`：SHA-256 资产校验、loopback/显式端口、可选强 Bearer、受限 Reqwest、Retry-After/有界退避、进程启动到 ready、提前退出检测和限时停止；Python 仍拥有 yfinance/AKShare 语义。
- [x] 建立 `jftrade-integration-pine`：SHA-256 bundle 校验、loopback/端口/token 边界、受管 Node 进程、ready probe/兼容退避策略、健康池、round-robin、live session pin、失败回滚和 restart 后 session 失效；Node worker 仍拥有 PineTS 执行。
- [x] 建立 `jftrade-integration-futu`：OpenD FT frame/SHA-1/长度防护、protocol/serial matching、market-data protocol ID、logical→physical 订阅计划、60 秒最小退订、5/10/20/30 秒失败退避、connection generation 重订阅和 quote-login fail-closed probe。
- [x] retained Python/Node worker 增加向后兼容的可选 Bearer；当前 Go 启动不设置令牌时行为不变，未来 Rust composition 启动时必须设置每进程强令牌；二者继续拒绝公共监听。
- [x] 固定 14 个行情操作、9 个 Pine 生命周期操作、3 个 OpenD 订阅和 3 个健康探针；Rust canonical output、三个直接调用现有 Go owner 的行为 harness 与 pinned expected 三方一致。
- [x] differential、manifest、超时恢复、worker 单测/typecheck、Python 全量测试、release replay 资源基线和 workspace 依赖/目录门禁已接入本地 gate。
- [ ] 显式 Yahoo/AKShare/OpenD live workflow、真实 Pine/Python 发布资产启动到 ready/关闭和 Linux/Windows/macOS 原生 runner 尚未执行；这些外部门禁继续阻断产品切换，但不让 Rust 取得第二 owner。

#### 阶段 4 执行账本

| 工作包 | 当前 Go/worker owner | 阶段 4 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| Provider capability、demand、freshness、cache、router | `internal/marketdata` | `jftrade-marketdata` | 以后只由 `jftrade-engine` 选择 provider router；阶段 4 CLI 仅离线 replay，回退仍走 Go | 真实 stream/poll trace、切换观察期和全部原生资源门禁通过，Rust 成为唯一产品 owner | 本地领域与拒绝/恢复证据完成；未切流 |
| Python helper 资产、HTTP 和进程生命周期 | Go `internal/marketdataassets`、`internal/app/apiserver/marketdataapp`、`internal/integration/yfinance`；Python `workers/marketdata-sidecar` 保留 Provider 语义 | `jftrade-integration-marketdata-helper` | `jftrade-engine` 以后只启动一个 authenticated loopback helper；移除 Rust composition 即回退 | packaged helper 在全部原生目标通过启动、ready、请求、故障、取消、关闭且 Go 不再启动 helper | 本地 adapter/lifecycle 与 mock HTTP 证据完成；发布资产 smoke 待外部平台 |
| Pine worker 资产、pool 和进程生命周期 | Go `pkg/strategy/pineworker`；Node `workers/pineworker` 保留 PineTS | `jftrade-integration-pine` | `jftrade-engine` 以后拥有唯一 worker pool；回退整体选择 Go manager，不迁移 live session | gRPC 全契约、发布资产、session/restart/关闭和原生资源门禁通过，Go manager 无消费者 | 本地 lifecycle/pool 与 Go 行为对照完成；真实 Rust gRPC 产品装配留在切流阶段 |
| OpenD market-data frame、健康和订阅恢复 | `internal/integration/futu`、`pkg/futu` | `jftrade-integration-futu` | `jftrade-engine` 注入 market-data port；阶段 5 的交易命令另行切换，回退保持 Go Futu runtime | 固定生成的 protobuf mapper、真实 socket/push/reconnect/live quote 与 OpenD 版本矩阵通过，Go market-data adapter 删除 | frame/serial/subscription/probe shadow 完成；raw protobuf body 尚不代表 live OpenD 资格 |

阶段 4 corpus 位于 `tests/fixtures/rust-migration/stage4`，`manifest.json` 固定 input、expected 和 Darwin ARM64 资源基线 SHA-256；关闭后的既有文件不得静默改写。三方 differential 为 `pnpm run test:rust:stage4:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage4`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：三个 Go 生产 owner 测试 harness 的 p95 为 21.155 ms、峰值 RSS 22,888,448 bytes；Rust composition replay 的 p95 为 3.268 ms、峰值 RSS 2,752,512 bytes，Rust/Go 比值为 0.154/0.120，5% p95 与 10% RSS 回退门禁通过。Go 是三个独立测试进程、Rust 是一个 replay 进程，绝对启动和 binary size 不作产品结论；该基线不启动真实外部 Provider/OpenD，也不替代发布包或原生平台资格。

本阶段“本地完成”只表示 Rust 领域/adapter/lifecycle 边界、兼容 fixture、资源基线和可回退 composition shadow 已闭环。公开 API、SQLite、真实订阅、Node/Python/Futu 产品生命周期仍由 Go 唯一拥有；真实 live、固定生成 protobuf 与发布平台 gate 完成前不得切流或删除 Go。

### 阶段 5：交易、策略运行与通知（本地完成）

目标：以只读、无副作用 shadow 方式建立交易与策略运行控制的 Rust 领域边界；Go 继续独占真实 broker 命令、SQLite 写入和用户可见通知。

- [x] 建立 `jftrade-trading`：Fixed8 命令校验、REAL/SIMULATE 风控、hard stop、幂等计划、单调审计、订单/成交去重与防倒退、原子持仓刷新、checkpoint 恢复、broker session/account generation 和无副作用通知 envelope。
- [x] 建立 `jftrade-strategy`：notify-only/paper/live 执行模式、signal 去重、暂停/恢复/断线 generation、关闭状态机，以及由消费方定义的窄 trading port；领域 crate 不直接依赖 `jftrade-trading`。
- [x] 在 `jftrade-engine` 装配 strategy→trading port；所有 `ShadowCommandPlan`、`TradePlanReceipt` 与通知均固定 `dispatch=false`，递归 differential 会拒绝任何 `dispatch=true`。
- [x] 扩展 `jftrade-integration-futu` 的 OpenD 交易 protocol ID 与 order-update mapper；read/push 只生成只读计划，unlock/place/modify/combo 等写协议在 shadow 中 fail closed。
- [x] 覆盖断线/重连、重复命令、拒绝结果重放、重复/乱序事件、cancel-fill 竞争、partial fill、失效账户 refresh、原子持仓刷新失败、checkpoint 恢复、port 故障和幂等关闭。
- [x] 固定 Go/Rust differential、零 dispatch 检查、资源基线和 SHA-256 manifest；Go 仍是唯一 broker、SQLite 与通知写 owner。
- [ ] 只读 OpenD、固定生成的交易 protobuf/socket/push/reconnect、显式小范围 live、持久化崩溃恢复及人工无双单 checklist 尚未执行；这些继续阻断产品切流和 Go 删除。

#### 阶段 5 执行账本

| 工作包 | 当前 Go owner | 阶段 5 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 订单命令、幂等、风控、订单/成交/持仓状态和审计 | `internal/trading`、`internal/store/trading` | `jftrade-trading` | `jftrade-engine` 只比较 Rust `ShadowCommandPlan`；正式切换时在 composition root 选择唯一 command/ledger owner，回退整体选回 Go | paper/fixture、只读 OpenD、小范围 live、持久化崩溃恢复和审计一致性全部通过，Rust 成为唯一写 owner且观察窗口关闭 | 本地 shadow/differential 完成；只产生命令计划和内存投影，禁止 broker/SQLite 写入 |
| 策略运行控制、通知计划和交易消费方 port | `internal/strategy` | `jftrade-strategy` | `jftrade-engine` 将策略 signal 映射到 trading 窄 port；回退保持 Go `liveruntime` | notify-only/paper/live 行为、暂停恢复、重复 signal、关闭竞争和 Pine session 恢复通过，Go manager 无消费者 | 本地 fixture/paper/live-plan 完成；通知只形成 shadow envelope，不发布用户可见事件 |
| OpenD 交易协议和 session 映射 | `pkg/futu`、`pkg/futu/opend` | `jftrade-integration-futu` | 由 `jftrade-engine` 注入 `jftrade-trading` 定义的 broker port；回退保持 Go Futu adapter | 固定生成 protobuf、真实 socket/push/reconnect、只读账户与显式小额 live 清单通过，无 Go trade adapter 消费者 | protocol ID、mapper 和写协议拒绝完成；未建立 socket 或发送交易命令 |

阶段 5 corpus 位于 `tests/fixtures/rust-migration/stage5`，覆盖 10 个 broker 状态、7 个状态迁移、6 个幂等/风控命令计划、7 个订单/成交事件、5 个原子持仓 refresh、11 个 session 操作、8 个 OpenD 交易协议和 3 个策略场景；`manifest.json` 固定 input、expected 与 Darwin ARM64 资源基线 SHA-256。三方 differential 为 `pnpm run test:rust:stage5:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage5`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：三个 Go 生产 owner 测试 harness 的 p95 为 21.641 ms、峰值 RSS 22,626,304 bytes；Rust composition replay 的 p95 为 3.822 ms、峰值 RSS 2,457,600 bytes，Rust/Go 比值为 0.177/0.109，5% p95 与 10% RSS 回退门禁通过。Go 是三个独立测试进程、Rust 是一个 replay 进程，绝对启动和 binary size 不作产品结论。

本阶段“本地完成”只证明领域状态、adapter gate、故障恢复、零 dispatch 和 fixture 资源门禁闭环。它没有打开 OpenD socket、发送 broker 命令、写 SQLite 或发布用户可见通知；只读 OpenD、显式小范围 live、持久化 crash recovery、人工无双单签字与原生平台资格完成前，阶段 5 不构成产品放行，Go 生产 owner 不变。

放行：订单/成交/持仓状态机 differential 通过；无双单；崩溃恢复和审计一致；人工 live checklist 签字。

### 阶段 6：Assistant 使用 Rig 迁移

- [x] 建立 `jftrade-assistant`，port session/run、9 个 run 状态、审批、输入、usage、stream delta、单调 audit 和可序列化 checkpoint。
- [x] port run lease 与 tool claim fencing，覆盖 held/lost/in-flight、replay-safe takeover/replay 和 fail-closed outcome unknown。
- [x] port版本化 artifact 投影和确定性任务 DAG，覆盖单一 ready task、claim/complete、self/missing dependency 与 cycle 拒绝。
- [x] 以 JFTrade 自有 `CompletionPort`/message/tool 契约隔离 Rig；Rig 类型只出现在 `rig_adapter`，tool JSON Schema 逐字段对齐 Go。
- [x] 使用 fake provider 和固定 transcript 覆盖流式 reply/reasoning/tool progress、usage、tool request，以及 transient network error 后一次重试恢复。
- [x] 固定 Go/Rust differential、checkpoint/fixture SHA-256 manifest 和 Darwin ARM64 release 资源基线。
- [ ] 具体 Provider adapter、真实 SSE/stream cancellation、credential/rate-limit/timeout、Google ADK session/event/artifact durable SQLite 全量兼容、进程崩溃重启和显式 provider live smoke 尚未执行；这些继续阻断产品切流和 Go 删除。

#### 阶段 6 执行账本

| 工作包 | 当前 Go owner | 阶段 6 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| session/run、审批、输入、stream、usage 与 audit 状态 | `internal/assistant/model`、`internal/assistant/engine`、`internal/assistant/engine/persistence` | `jftrade-assistant` | `jftrade-engine` 注入唯一 Assistant runtime；回退整体选择 Go runtime，禁止审批、输入和 audit 双写 | 全量 terminal/event projection、durable SQLite、并发 continuation、崩溃重启和观察窗口通过，Go runtime 无消费者 | 本地纯状态/checkpoint shadow 完成；Go 仍是产品和 SQLite 唯一 owner |
| run lease、tool claim、artifact 与任务 DAG | `internal/assistant/engine/persistence`、Google ADK artifact/session adapter、`engine/workflowexec` | `jftrade-assistant::{claims,artifact,workflow}` | composition root 注入 persistence/artifact ports；正式切换必须选择唯一 lease/claim/artifact 写 owner | SQLite fencing/事务/损坏恢复、ADK artifact 版本与任务并发 differential 全部通过 | 内存 checkpoint 与 Go 临时生产 SQLite harness differential 完成；Rust durable adapter 未实现 |
| Provider/tool 边界与 Rig 映射 | `internal/assistant/assembly`、`engine/providers`、Google ADK runtime | JFTrade `CompletionPort` + `jftrade-assistant::rig_adapter` | `jftrade-engine` 选择一个 Provider adapter；Rig/Provider 类型不得穿透能力边界 | 真实 Provider tool/stream/error/cancel/usage 行为和 live smoke 通过，Google ADK Provider runtime 无消费者 | `rig-core` 最小 feature、fake transcript 和 schema 映射完成；无网络、无 credential、无生产流量 |

阶段 6 corpus 位于 `tests/fixtures/rust-migration/stage6`，覆盖 9 个 run 状态、12 个状态迁移、1 个完整 request-user schema、审批与输入各一组持久化/幂等恢复、3 个非法输入、2 个 tool claim、3 个任务节点、3 个非法 DAG、2 个 artifact 版本和 3 个 stream delta；`manifest.json` 固定 input、expected 与 Darwin ARM64 资源基线 SHA-256。differential 为 `pnpm run test:rust:stage6:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage6`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：Go 生产 Assistant store/领域测试 harness 的 p95 为 67.461 ms、峰值 RSS 49,037,312 bytes；Rust composition replay 的 p95 为 5.731 ms、峰值 RSS 3,391,488 bytes，Rust/Go 比值为 0.085/0.069，5% p95 与 10% RSS 回退门禁通过。Go harness 每次创建临时生产 SQLite/ADK store，Rust 使用内存 checkpoint；绝对启动时间、binary size 和数据库性能不作产品结论。

本阶段“本地完成”只证明纯状态、checkpoint round-trip、SQLite-backed Go reference、Rig 请求投影、fake Provider 恢复和资源门禁闭环。它没有连接真实模型 Provider、写 Rust SQLite/ADK artifact store、替换 Google ADK、改变公开 API 或获取第二个审批/输入 owner；完成 provider live、durable crash recovery、全量 session/event parity 和原生平台资格前，阶段 6 不构成产品放行，Go 生产 owner 不变。

放行：终态和持久化完全一致；审批/输入不能丢失或重复；模型网络异常可恢复；Rig 可替换性由 adapter 测试保证。

### 阶段 7：Rust API/control plane 成为产品 owner（本地 shadow 完成）

- [x] 建立 `jftrade-api`：Axum/Tower router、版本化 route catalog、成功/错误 envelope、request ID、CORS、桌面 Bearer/WebSocket subprotocol、浏览器 cookie/CSRF/origin、SSE frame、WebSocket 限流、static asset/SPA fallback 和 transport metrics。
- [x] 从现有 OpenAPI baseline 机械生成并固定全部 278 个 operation、18 个 route group 和 19 个具体路径探针；Go 生产 Gin 注册表测试继续证明 baseline 没有漏注册或多注册。
- [x] 建立 `jftrade-research`、`jftrade-watchlist`、`jftrade-settings`、`jftrade-calendar` 和 `jftrade-datamanagement` 的首批纯规则及拒绝测试；领域 crate 不依赖 Axum、SQLite、Provider SDK 或其他领域 service。
- [x] 由 `jftrade-engine::stage7` 唯一装配 route catalog、领域投影和 `ApiPort`；未登记 operation fail closed，shadow 只返回确定性投影，不写数据库、不启动监听器。
- [x] 固定 Go/Rust differential、损坏/未知/缺字段输入拒绝、SHA-256 manifest 和 Darwin ARM64 release 资源基线；未修改 OpenAPI、Wails bindings、SQLite schema 或 Vue API 调用。
- [ ] 278 个 operation 的真实 handler/DTO/store port、完整 status/header/null/omitted 逐响应 replay、长时间 SSE/WebSocket 断线恢复、生产 static bundle、真实 sidecar lifecycle、route-group 灰度/回退和 Web/桌面 E2E 尚未执行；这些继续阻断产品切流和 Go 删除。

#### 阶段 7 执行账本

| 工作包 | 当前 Go owner | 阶段 7 Rust owner | 唯一切换点与回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| HTTP/OpenAPI、安全、SSE、WebSocket、static assets 与 observability | `internal/api/*`、`internal/app/apiserver/servercore`、`cmd/jftrade-api` | `jftrade-api` + `jftrade-engine::stage7` | `jftrade-engine` 按 route group 选择唯一 transport/handler owner；回退整体把该组交还 Go，禁止同一写请求双 dispatch | 278 个真实 handler、全量 response differential、长连接恢复、Web/生产 bundle、启动关闭端口和安全观察窗口通过，Go router 无消费者 | 本地 Axum transport 与全量 route inventory shadow 完成；不监听产品端口、不承接真实请求 |
| 研究与自选控制面规则 | `internal/research`、`internal/watchlist` 及对应 store | `jftrade-research`、`jftrade-watchlist` | `jftrade-engine` 注入各自窄 persistence/market-data port；route 切换与写 owner 同步原子完成 | 全量 preset/screening/group/membership/import/pagination/revision 和 DB recovery 通过，Rust 成为唯一写 owner | 首批 revision、schema、identity、去重和分页规则完成；无 SQLite/API handler |
| 设置与日历控制面规则 | `internal/settings`、`internal/jftsettings`、交易日历 owner | `jftrade-settings`、`jftrade-calendar` | composition root 先持久化再应用 listener/provider；日历 provider 由窄 port 注入 | credential/setting round-trip、listener rollback、provider fallback、timezone/session/import 通过且 Go 无消费者 | 首批密码/端口/provider 顺序与 session/source 规则完成；无 listener、credential store 或 provider 调用 |
| 数据维护与破坏性操作 fencing | `internal/datamanagement`、`databaseguard` 和各 Go store | `jftrade-datamanagement` | preview/execute 使用同一候选集指纹并由 `jftrade-engine` 注入唯一 store owner；回退不复用过期 preview | 所有数据库类别、busy/active owner、备份恢复、审计和故障注入通过，Go cleanup owner 删除 | 确定性 preview/execute 指纹与 busy fail-closed 完成；不删除、不写任何业务数据 |

阶段 7 corpus 位于 `tests/fixtures/rust-migration/stage7`，`api-control-plane-corpus.json` 由 OpenAPI baseline 机械生成 278 个 operation，并加入 19 个具体 route probe 和五个控制面领域投影；`manifest.json` 固定 input、expected 与 Darwin ARM64 资源基线 SHA-256。differential 为 `pnpm run test:rust:stage7:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage7`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：Go 生产 Gin 路由/OpenAPI 注册 harness 的 p95 为 128.371 ms、峰值 RSS 75,481,088 bytes；Rust composition replay 的 p95 为 5.740 ms、峰值 RSS 2,605,056 bytes，Rust/Go 比值为 0.045/0.035，5% p95 与 10% RSS 回退门禁通过。Go 每次装配生产 router，Rust 只做无 listener 的本地投影；绝对启动、binary size 和真实 HTTP 吞吐不作产品结论。

本阶段“本地完成”只证明 transport 边界、全量 operation inventory、首批控制面纯规则、确定性 replay 和资源门禁闭环。它没有实现或切换 278 个生产 handler，没有写 SQLite、启动公开 listener、承接长连接或接管 Node/Python/OpenD lifecycle；全量 route-group replay、Web/生产 bundle、真实 sidecar、原生平台和观察窗口完成前，阶段 7 不构成产品放行，Go/Gin 生产 owner 不变。

放行：全量契约/differential/Web 测试通过；生产 bundle 和真实 sidecar smoke 通过；启动、关闭、端口和安全边界一致。

### 阶段 8：Wails → Tauri 桌面迁移（本地 facade shadow 完成）

- [x] 创建 `apps/desktop/src-tauri`，以 `jftrade-desktop` 作为只依赖 `jftrade-engine` 的外层 shell crate；登记 Tauri config、10 个 command、4 个 event，以及启动、链接、日志、更新、主窗口/日志窗口边界。
- [x] 在 Vue 建立单一 `desktopFacade`，运行时只选 Tauri 或 Wails 一个 adapter；Tauri 使用官方 `invoke`/`listen`，Wails 继续动态加载现有生成 bindings，浏览器调用桌面命令 fail closed。
- [x] 复制 Wails 的 dev/release identity、单实例 ID、`127.0.0.1:3008`/`127.0.0.1:6699`、系统用户数据目录、settings/backtest/window-state 路径和 update policy；三平台 profile corpus 固定成功与拒绝行为。
- [x] 建立 engine → Pine worker → market-data sidecar 的资产校验和启动顺序，以及反向关闭、5 秒关闭预算、readiness 失败回收；本地实现只通过注入的 supervisor 形成可执行计划，不启动第二套产品子进程。
- [x] 固定 Go/Wails 对 Rust/Tauri 的 desktop differential、Vue 双 adapter 行为测试、SHA-256 manifest 与 Darwin ARM64 release replay 资源基线。
- [ ] 仍需启用实际 Wry/native runner，接管 Rust API/PineTS/Python 发布资产，复刻 tray/menu/notification/window state/update installer，并在同一生产 Vue bundle 上完成 Wails/Tauri 黑屏、退出孤儿、数据升级与回退对照。
- [ ] macOS ARM64、Linux x64、Windows x64/ARM64 的签名、安装、升级和卸载 smoke 尚未执行；完成前不得切换桌面入口或删除 Wails bindings/生成链。

#### 阶段 8 执行账本

| 工作包 | 当前 Wails owner | 阶段 8 Rust/Tauri owner | 唯一切换点与回退 | Wails 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| Vue desktop facade、command 与 event 契约 | `cmd/jftrade-desktop` 生成 bindings、`@wailsio/runtime` | `apps/web/src/composables/shared/desktopFacade.ts`、`jftrade-desktop::tauri_adapter` | Vue 每次只解析一个 runtime adapter；release launcher 选择 native shell，回退整体选择 Wails | 同一生产 bundle 在两个壳的启动、链接、日志、更新、窗口、菜单、单实例行为完全一致，Tauri 观察窗口关闭 | 双 adapter 与拒绝/卸载 listener 测试完成；产品仍解析 Wails |
| identity、端口与用户数据目录 | `desktop_profile.go`、`internal/desktop/runtime_path.go` | `jftrade-desktop::profile` | native shell profile 是唯一配置源；不迁移、不扫描开发数据，回退沿用同一 release 路径 | 四平台安装/升级/备份恢复验证 settings、数据库和 window state 原地兼容 | 3 平台确定性投影与 Go reference 完成；真实升级未执行 |
| Rust API、PineTS、Python helper 资产和生命周期 | `cmd/jftrade-desktop`、Go apiserver/worker managers、Wails release assets | `jftrade-desktop::lifecycle` 通过 `jftrade-engine` facade | Tauri composition 一次性取得全部 child owner；失败按 sidecar → Pine → engine 反向回收，回退不能并行启动 Wails child owner | 签名资产 hash、ready、故障、取消、5 秒关闭、crash recovery 和无孤儿进程在四平台通过 | 资产/顺序/失败回收纯逻辑完成；未启动真实 child、未接管发布资产 |
| 原生 WebView、tray/menu/notification、窗口状态、installer/updater | Wails v3 与现有四平台 release scripts | 后续启用的 Tauri native runtime/plugins | 只在四平台 RC 全绿后切换 release entrypoint；整体安装包回退 | 无黑屏、菜单/通知/链接/窗口一致，签名安装/升级/卸载通过，Wails 入口和 bindings 无消费者 | 未启动；本地 shadow 明确不启用 Wry/tray/updater |

阶段 8 corpus 位于 `tests/fixtures/rust-migration/stage8`，覆盖 3 个平台 profile、6 个链接、3 个受管资产、成功启动/反向关闭、readiness 失败回收、10 个 facade command 和 4 个 event；`manifest.json` 固定 input、expected 与 Darwin ARM64 资源基线 SHA-256。differential 为 `pnpm run test:rust:stage8:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage8`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：Go 生产 Wails 桌面行为 harness 的 p95 为 18.247 ms、峰值 RSS 42,631,168 bytes；Rust/Tauri facade replay 的 p95 为 6.658 ms、峰值 RSS 8,765,440 bytes，Rust/Go 比值为 0.365/0.206，5% p95 与 10% RSS 回退门禁通过。Rust 样本没有创建 native WebView 或真实子进程，绝对启动、RSS 与二进制大小不能用作 Tauri 产品资源结论。

本阶段“本地完成”只证明 facade owner、配置/路径语义、生命周期计划、Vue 双 adapter、确定性 replay 和资源门禁闭环。它没有运行 Tauri native shell、接管生产 API/worker、构建或签名安装包，也没有验证无黑屏、无孤儿和升级数据；四平台 release candidate 与观察窗口完成前，阶段 8 不构成产品放行，Wails 生产 owner 不变。

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

### 7.1 按能力细粒度提交与收口（强制）

迁移计划中的提交单位是一个可独立构建、验证、审查和回退的业务能力或安全不变式，不是整个迁移阶段。契约、fixture/golden、实现、differential、owner 账本和必要文档应与该能力在同一提交中闭环；只有拆分后的两部分都能独立产生价值并保持 green 时才继续拆分。

不强制把 contract/fixture、shadow/differential、production cutover 和 Go owner 退役拆成固定四个提交，也不使用多层工作包编号、文件数/行数配额或 integration wave 人为制造边界。如果分开 producer、consumer、cutover 或退役会造成死代码、错误 owner 事实、双写或无 owner 窗口，它们必须作为一个原子提交交付。

每个提交先跑最窄 affected test，再按风险扩展到 `check:quick`、迁移专项门禁和 `check:all`。必须由上游、显式 live 或其他原生发布平台执行的资格按未闭环证据登记，不得表述为产品切流或正式关闭。recovery checkpoint 只能保留在明确的备份分支，不得进入正式交付历史。除非用户明确要求，迁移提交只在本地创建，不推送、不重写已共享历史。

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
| 2026-08-19 | 每个迁移阶段完成全部实现与门禁后只形成一个本地阶段提交；阶段内工作包不单独提交 | 第 7.1 节；未获明确授权不推送 |
| 2026-08-21 | 废止阶段单提交和多层工作包编号，改为可独立构建、验证、审查与回退的能力细粒度提交 | 第 7.1 节；2026-08-19 的阶段单提交决策自本行起废止 |
| 2026-08-19 | 阶段 2 启动；shopspring Decimal 与 fixedpoint 拆分兼容，SQLite 首个只读样本选择 backtest K 线 | `tests/fixtures/rust-migration/stage2` 与阶段 2 执行账本 |
| 2026-08-19 | 阶段 2 本地实现完成；Go 保持唯一生产 owner，Rust SQLite 仍为离线只读验证工具 | golden/differential 零差异、DB bytes 不变、Darwin ARM64 release 资源基线通过 |
| 2026-08-19 | 阶段 3 本地计算核心完成；PineTS 保留，Go 保持唯一生产 owner，离线三态 selector 只做 shadow/切换/回退演练 | 5 case/8 fill 三方 differential 零差异；取消/超时恢复通过；Darwin ARM64 p95/RSS 门禁通过；阶段 3 manifest 已固定 |
| 2026-08-19 | 阶段 4 本地行情/worker 生命周期工作包完成；retained Node/Python 只增加可选私有 Bearer，Go 继续拥有公开 API、真实 Provider/OpenD 与进程 lifecycle | 14 market-data/9 Pine/3 OpenD subscription/3 probe 三方 evidence；未知健康和切换 fail closed；Darwin ARM64 p95/RSS 门禁通过；真实 live/发布平台资格保持未闭环 |
| 2026-08-19 | 阶段 5 本地交易/策略/OpenD shadow 工作包完成；所有交易计划与通知强制零 dispatch，Go 继续拥有 broker、SQLite 和用户可见通知 | 10 status/7 transition/6 command/7 update/5 position refresh/3 strategy differential；Darwin ARM64 p95/RSS 门禁通过；只读 OpenD、小额 live 与 durable recovery 保持未闭环 |
| 2026-08-19 | 阶段 6 本地 Assistant/Rig shadow 工作包完成；Rig 隔离在窄 adapter，Go/Google ADK 继续拥有生产 Provider、SQLite、artifact 与 continuation | 9 status/12 transition、审批/输入幂等、lease/claim fencing、DAG、artifact 与 fake transcript differential；Darwin ARM64 p95/RSS 门禁通过；真实 Provider live、Rust durable store 与 crash recovery 保持未闭环 |
| 2026-08-19 | 阶段 7 本地 API/control-plane shadow 工作包完成；Axum/Tower 与五个控制面领域 crate 已受目录门禁约束，Go/Gin 继续拥有公开 API、SQLite 与产品 lifecycle | 278 operation/18 route group/19 concrete probe differential；安全、envelope、SSE/WS/static 行为测试与 Darwin ARM64 p95/RSS 门禁通过；真实 handler、route cutover、长连接、bundle/sidecar 和原生平台保持未闭环 |
| 2026-08-19 | 阶段 8 本地 Tauri desktop facade shadow 工作包完成；Vue 已有 Wails/Tauri 单选 adapter，Wails 继续拥有生产 native shell、子进程与发布资产 | 3 platform/6 link/3 asset/10 command/4 event differential；readiness 失败反向回收、Vue adapter 测试与 Darwin ARM64 p95/RSS 门禁通过；native WebView、真实 child、四平台签名安装/升级/孤儿观察保持未闭环 |

完成每个阶段时在本表追加最终决策；大量一次性测试日志留在 CI artifact/提交，不复制进长期文档。
