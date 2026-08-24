# JFTrade Go → Rust 完整迁移方案与守则

状态：执行中。更新时间：2026-08-24。当前阶段：**阶段 9 生产 owner 接管与删除准入正在执行；Rust/Tauri release candidate 已能以受鉴权 loopback 只读 shadow 启动 26 个真实 GET handler。当前 122 个普通 JSON GET operation 已通过 Go fixture、Rust replay、authenticated loopback wire/error/timeout/crash rehearsal 和 restart-time Go rollback 达到 cutover-qualified；23 个 settings/system GET 仍为 read-only shadow。133 个 cutover-test-only operation 包含全部 129 个 POST/PUT/PATCH/DELETE mutation，以及仍缺少专用 transport corpus 的 4 个 GET：ADK run/stream SSE、auth session browser transport 和 live WebSocket。research-presets-write 已补 composition-root authenticated mutation rehearsal及 Rust durable test-cutover store：后者只打开既有且 schema 验证通过的 Go-compatible SQLite，强制 test-only profile 与唯一 writer lease，并通过 revision 并发 fencing、损坏拒绝和重启恢复测试；完整 `ScreenDefinitionV2` 规则和 product port 尚未接入，因此三条 route 不升级。自动 route ownership 门禁当前为 278 个 operation、23 shadow、133 个 cutover-test-only、122 个 cutover-qualified、0 个 remaining、0 个 Rust production owner。Go/Wails 仍是全部产品写入与正式发布入口的唯一 owner。在所有 Tier A 写路径的唯一 owner、幂等、事务、恢复与副作用隔离，以及 SSE/WS/browser auth、四平台 RC、签名 updater、SBOM、安全审查和恢复演练通过前不得删除 Go/Wails**。

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

本节是迁移期间新增目录、crate 和跨目录依赖的强制放置规则。目录出现在蓝图中只表示名称、owner 和最早启用阶段已经预留，**不表示应立即创建**。阶段未启动、没有实际生产代码或没有行为测试时，不得创建空 crate、占位模块或未来依赖。当前 Rust 侧已经启用 `jftrade-engine`、`jftrade-kernel`、`jftrade-broker`、`jftrade-store-sqlite`、`jftrade-store-settings-file`、`jftrade-backtest`、`jftrade-marketdata`、`jftrade-strategy`、`jftrade-trading`、`jftrade-assistant`、`jftrade-research`、`jftrade-watchlist`、`jftrade-settings`、`jftrade-calendar`、`jftrade-datamanagement`、`jftrade-api`、`jftrade-integration-pine`、`jftrade-integration-marketdata-helper`、`jftrade-integration-futu` 和 `apps/desktop/src-tauri`；其余目标目录仍是计划目录。

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
  jftrade-store-sqlite/                   # 已存在，阶段 2 只读 adapter；阶段 9 仅显式 test-cutover writer
  jftrade-store-settings-file/            # 已启用，阶段 9：兼容 settings.json 的原子文件 adapter
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
| `internal/store/*` | `jftrade-store-sqlite`、`jftrade-store-settings-file` | 2/9 起 | SQLite adapter 只实现各能力定义的数据库 ports；settings-file adapter 只保留既有 JSON 文件形状、未知字段、权限和原子替换。两者均不得把 driver、I/O 或 row/document codec 反向带入领域 crate |
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
29. 本地 worker 使用当前工作树范围的 `check:quick`，只承担切片级快速反馈；完整 merge-base affected gate 和 Stage 1–9 Rust differential 由集成分支每波执行。快速门禁必须显式报告 deferred integration checks，不能用降频替代 wire、唯一 owner、禁止双写、恢复、四平台或 hard-cut 证据。

## 5. 依赖甄选基线

依赖版本以 2026-08-20 的 crates.io/上游发布为筛选快照。状态列标明实际引入阶段；其余是后续阶段首选候选，引入时必须重新核验。

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
| atomic settings file | [tempfile 3.27.0](https://github.com/Stebalien/tempfile) | 阶段 9 已引入 | 广泛采用的跨平台临时文件实现；只在 `jftrade-store-settings-file` 中用于同目录私有临时文件、flush/fsync 与覆盖持久化，保留现有 `settings.json`、未知字段和 Unix `0700/0600` 权限，不进入领域模型 |
| local secret hashing/encoding | [RustCrypto Argon2 0.5.3](https://github.com/RustCrypto/password-hashes) / [base64 0.23.1](https://github.com/marshallpierce/rust-base64) / [getrandom 0.4.3](https://github.com/rust-random/getrandom) | 阶段 9 已引入 | Web access password 与 MCP token reset 均与 Go owner 保持 Argon2id v19、64 MiB/3 次/单 lane、16-byte salt、32-byte verifier 兼容；token 使用 32 个系统随机字节和 RFC 4648 URL-safe 无 padding 编码。Argon2 只启用 alloc/password-hash，base64 只启用 alloc 并关闭默认 SIMD unsafe；明文 password 不落盘，token 只返回一次，settings 文件只存 PHC verifier |
| URL path percent decoding | [percent-encoding 2.3.2](https://github.com/servo/rust-url/tree/main/percent_encoding) | 阶段 9 已引入 | Servo `rust-url` 官方 workspace 的小型无网络库，MIT/Apache-2.0；只在 `jftrade-engine` transport 边界解码 managed account ID 的 UTF-8 percent-encoded path segment（ID 含 `|`），避免手写不完整 decoder，不进入领域 crate。版本已由 Tauri/URL 图传递锁定，提升为直接依赖不增加第二套 URL 实现 |
| loopback HTTP client | [Reqwest 0.13.4](https://github.com/seanmonstar/reqwest) | 阶段 4 已引入 | 项目官方且广泛采用；仅为本机 Python helper 启用 `json`，关闭默认 feature、TLS、proxy、cookie 和 HTTP/2；显式端口、禁重定向、限时限长并可选 per-process Bearer |
| protocol/asset digest | [RustCrypto SHA-1/SHA-2 0.11.0](https://github.com/RustCrypto/hashes) | 阶段 4 已引入 | OpenD wire 固有 SHA-1 与发布资产 SHA-256；官方 RustCrypto 实现、关闭默认 feature，不用于密码存储或自造认证协议 |
| Decimal | 自有兼容 codec；[rust_decimal 1.42.1](https://github.com/paupino/rust-decimal) 仅保留为有界算术候选 | 阶段 2 已决策 | shopspring Decimal 是任意精度字符串语义，不能无损收窄到 `rust_decimal`；bbgo fixedpoint 另按 `i64 × 10^-8` 实现。只有领域边界已证明在 96-bit 范围内时才允许引入 `rust_decimal` |
| time/identity | [time 0.3.55](https://github.com/time-rs/time) / [uuid 1.24.1](https://github.com/uuid-rs/uuid) | `time` 阶段 2 已引入；UUID 后续候选 | `time` 仅启用 std/formatting/parsing/serde，保留 RFC3339Nano 与 Unix 毫秒语义；UUID 未被阶段 2 代码使用，不提前引入 |
| CPU parallelism | [Rayon 1.12.0](https://github.com/rayon-rs/rayon) | 后续候选 | 只用于有基准证据的批量纯计算；不得与 Tokio task 无界叠加 |
| Assistant | [Rig Core 0.42.0](https://github.com/0xPlaygrounds/rig) | 阶段 6 已引入 | 官方仓库、MIT；精确锁定且关闭默认 feature，只在 `rig_adapter` 内使用 provider-neutral request/message/tool 类型；不启用 Rig 的 `reqwest` 增强、derive、rustls 或具体 Provider feature（core 自身仍含最小 HTTP/stream 基础依赖），不让 Rig 类型进入 JFTrade 持久化模型或 ports |
| desktop | [Tauri 2.11.5](https://github.com/tauri-apps/tauri) / [`@tauri-apps/api` 2.11.1](https://github.com/tauri-apps/tauri) | 阶段 8 已引入 | 官方 Rust/JS 包；Rust crate 关闭全部默认 feature，仅使用 Builder/command/state 边界，Vue 通过官方 `invoke`/`listen` 接入；本地 shadow 不启用 Wry/WebView、tray、updater 或 native TLS，待原生切片实际使用时再逐项开启 |
| desktop notification | [tauri-plugin-notification 2.3.3](https://github.com/tauri-apps/plugins-workspace) | 阶段 9 RC 已引入 | Tauri 官方插件；只由 `jftrade-engine` 定义的窄 notification port 注入原生壳，未成为唯一写 owner前不登记通知测试 route，不向 Vue 暴露插件命令权限 |
| desktop updater | [tauri-plugin-updater 2.10.1](https://github.com/tauri-apps/plugins-workspace) | 阶段 9 RC 已引入 | Tauri 官方签名更新器；关闭默认 feature，仅启用 rustls TLS 与桌面压缩包支持。开发构建不联网，release 只有同时取得 HTTPS endpoint 与 Minisign 公钥才自动检查；下载和安装必须由用户显式触发，安装前回收受管子进程。私钥只属于 release signing 环境，不进入仓库或应用配置 |

阶段 5 未增加第三方依赖；`jftrade-trading` 和 `jftrade-strategy` 只复用已审计的 `jftrade-kernel`、`jftrade-broker`、Serde 与 thiserror，OpenD 交易 shadow 复用阶段 4 的 adapter crate，避免在无真实 wire owner 前引入第二套 broker SDK 或持久化框架。

阶段 6 新增的唯一直接第三方依赖是 `rig-core = 0.42.0`。选择官方 Rig core crate 而非完整 facade/具体 Provider SDK，保留 JFTrade 自有 `CompletionPort`、tool schema、session/run/approval/input/workflow 契约；通过 `cargo-deny` 检查 advisory、license、source 和重复版本。真实 Provider 适配需要在显式 live 工作包中重新审查 TLS、credential、timeout、rate limit、stream cancellation 与 telemetry content 策略。

阶段 7 新增 Axum 0.8.9、Tower 0.5.3 与 tower-http 0.7.0，均来自 Tokio/Tower 官方维护生态，并继续精确锁定版本、关闭默认 feature、只开启实际生产代码使用的最小功能；`http-body-util` 仅用于 transport 行为测试。研究、自选、设置、日历和数据维护 crate 没有引入第二套 async runtime、数据库驱动、Provider SDK 或通用 service 框架。

阶段 8 新增的直接依赖只有 Tauri 官方 `tauri = 2.11.5` 与 `@tauri-apps/api = 2.11.1`。Rust 端当时默认 feature 全关，不把尚未运行的 Wry、tray、updater、system shell 或 TLS 提前带入产品；JS 端只在检测到 Tauri runtime 时动态加载 `core.invoke` 和 `event.listen`，当前 Wails binding 仍是生产 adapter。阶段 9 启用 Wry/tray 后，`cargo-deny` 审计确认 Tauri 的四目标解析带入 Linux GTK3 与 `urlpattern` 的 14 个“unmaintained”信息公告（没有 vulnerability/unsound 公告）；`deny.toml` 只按 advisory ID 登记官方传递图例外，并只对 12 个精确 crate/version 放行 MPL-2.0、Zlib、ISC 或 Apache-2.0 WITH LLVM-exception，禁止把许可证全局放宽。Tauri/传递版本变化时未命中的例外会告警并必须重新审查；原生 feature 必须继续跟随对应平台实现和测试一起引入。

阶段 9 首个生产切片新增 `tempfile = 3.27.0`，用于替代 Go/Windows `MoveFileEx` 与 Unix rename 的跨平台覆盖持久化细节。候选中的手写固定 `.tmp` 文件会产生碰撞和权限风险，直接使用平台 API又违反默认 `forbid(unsafe_code)`；因此选择维护活跃、采用广泛且专注临时文件安全边界的实现，并继续精确锁定。该依赖不授权 adapter 改写未知 settings 字段或另建配置格式。

阶段 9 的 Futu broker descriptor 只增加既有 workspace foundation `jftrade-broker` 依赖：adapter 负责填充 broker-neutral 描述符，领域与 API 不复制 Futu 常量。该依赖方向已登记到目录门禁，未新增第三方 SDK，也不授权 adapter 反向调用领域 service。

阶段 9 broker settings cutover 为包含 `|` 的 managed account ID 增加精确锁定的 `percent-encoding = 2.3.2`。候选中的手写替换无法正确处理通用 UTF-8 与非法编码，完整 `url` parser 对单一 path segment 又过重；因此直接复用已经在 Tauri/URL 传递图中的 Servo 官方实现，只暴露 `percent_decode_str` 给 Axum handler。该 crate 没有默认 feature、网络、文件或 provider 能力，解码失败按既有 HTTP bad request 契约拒绝。

阶段 9 Web access password/MCP token cutover 测试新增 RustCrypto 官方 `argon2 = 0.5.3`、广泛采用的 `base64 = 0.23.1`，并复用已锁定的 `getrandom = 0.4.3`。实现固定为现有 Go verifier 的 Argon2id 参数并在校验前拒绝参数漂移，避免损坏文件请求无界内存/CPU；base64 关闭默认 `simd-unsafe`，只保留分配型 URL-safe encoder。写入先原子持久化、再调用消费方定义的 listener port，失败回滚旧记录；security PUT 还必须接收 transport 在鉴权后生成的 `desktop_trusted` 元数据，已通过 Web session/CSRF 的浏览器请求仍返回 desktop-only 403。由于 Rust Web/MCP listener 尚未接管，这三个 mutation 只在 cutover 测试 route catalog 中登记，默认产品 shadow 不可调用。

阶段 9 原生 RC 只在真实通知 adapter 到位后引入 Tauri 官方 `tauri-plugin-notification = 2.3.3`，沿用插件自身的跨平台权限与发送实现，不手写 macOS/Windows/Linux API。通知 route 仍受唯一 owner 门禁约束，默认只读 shadow 不注册它；没有额外安装 JS 包或放开 capability 权限。

阶段 9 更新边界使用 Tauri 官方 `tauri-plugin-updater = 2.10.1`，不复用 Go 的无签名 GitHub feed 下载逻辑，也不向 WebView 暴露插件原生命令权限。`JFTRADE_TAURI_UPDATER_ENDPOINT` 与 `JFTRADE_TAURI_UPDATER_PUBKEY` 必须成对提供：release 构建时提供会固化到二进制，运行时提供只用于隔离验收；endpoint 必须为无凭据 HTTPS URL。配置缺失时不自动联网且手动检查明确返回未配置，配置不完整时 release 启动失败。检查只缓存官方插件验证目标所需的签名元数据；用户点击“下载并安装”后才下载，Minisign 验签成功后先停止 Rust API/Pine/Python 进程树再替换应用并退出。插件使用精确锁定的 Rustls 0.23.43 ring provider；desktop 只通过 `jftrade-engine` feature 将 provider 安装职责传到现有 helper 的 Reqwest 构造边界，避免 workspace feature 合并后出现“启用 TLS 却没有默认 crypto provider”的 panic，非 desktop 构建不会因此启用 Rustls。真实签名私钥、`createUpdaterArtifacts` 发布产物和升级/回退 smoke 仍须在四平台 release signing workflow 闭环，未完成前不构成 updater 放行。

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
- [x] 阶段 9 macOS RC 已启用实际 Wry/native runner，接入受鉴权的只读 Rust API shadow 与受管 PineTS/Python 发布资产，并实现 tray/menu、notification adapter 和兼容 window state；正式 launcher 与全部业务写 owner 均未切换。
- [x] 已接入官方签名 updater 的检查、显式下载/安装、失败重试与安装前进程树回收边界；开发/缺配置时不联网，Stage 8 冻结的 10-command fixture 不变，安装作为 Stage 9 独立 command 扩展。
- [ ] 仍需在 release signing workflow 生成真实签名 updater artifact，并在同一生产 Vue bundle 上完成 Wails/Tauri 黑屏、退出孤儿、数据升级与回退对照。
- [ ] macOS ARM64、Linux x64、Windows x64/ARM64 的签名、安装、升级和卸载 smoke 尚未执行；完成前不得切换桌面入口或删除 Wails bindings/生成链。

#### 阶段 8 执行账本

| 工作包 | 当前 Wails owner | 阶段 8 Rust/Tauri owner | 唯一切换点与回退 | Wails 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| Vue desktop facade、command 与 event 契约 | `cmd/jftrade-desktop` 生成 bindings、`@wailsio/runtime` | `apps/web/src/composables/shared/desktopFacade.ts`、`jftrade-desktop::tauri_adapter` | Vue 每次只解析一个 runtime adapter；release launcher 选择 native shell，回退整体选择 Wails | 同一生产 bundle 在两个壳的启动、链接、日志、更新、窗口、菜单、单实例行为完全一致，Tauri 观察窗口关闭 | 双 adapter 与拒绝/卸载 listener 测试完成；产品仍解析 Wails |
| identity、端口与用户数据目录 | `desktop_profile.go`、`internal/desktop/runtime_path.go` | `jftrade-desktop::profile` | native shell profile 是唯一配置源；不迁移、不扫描开发数据，回退沿用同一 release 路径 | 四平台安装/升级/备份恢复验证 settings、数据库和 window state 原地兼容 | 3 平台确定性投影与 Go reference 完成；真实升级未执行 |
| Rust API、PineTS、Python helper 资产和生命周期 | `cmd/jftrade-desktop`、Go apiserver/worker managers、Wails release assets | `jftrade-desktop::native` 与 `jftrade-engine::product_runtime` | Tauri RC 一次性取得其隔离进程树 owner；失败按 helper → Pine → engine 反向回收，正式 launcher 不并行启动两个产品 owner | 签名资产 hash、ready、故障、取消、5 秒关闭、crash recovery 和无孤儿进程在四平台通过 | macOS RC 已启动真实 child 并验证资源 hash、401、ready、反向关闭和无孤儿；其余平台未验证，Rust API 仍只读 |
| 原生 WebView、tray/menu/notification、窗口状态、installer/updater | Wails v3 与现有四平台 release scripts | `jftrade-desktop::native`、Tauri 官方 notification/updater plugins | 只在四平台 RC 全绿后切换 release entrypoint；整体安装包回退 | 无黑屏、菜单/通知/链接/窗口一致，签名安装/升级/卸载通过，Wails 入口和 bindings 无消费者 | macOS RC 已实现 WebView/tray/menu/notification/window state 与签名 updater 代码边界；真实签名 artifact、视觉黑屏对照和四平台安装升级仍未完成 |

阶段 8 corpus 位于 `tests/fixtures/rust-migration/stage8`，覆盖 3 个平台 profile、6 个链接、3 个受管资产、成功启动/反向关闭、readiness 失败回收、10 个 facade command 和 4 个 event；`manifest.json` 固定 input、expected 与 Darwin ARM64 资源基线 SHA-256。differential 为 `pnpm run test:rust:stage8:differential`，本机 release 资源复测为 `pnpm run benchmark:rust:stage8`。

Apple A18 Pro/macOS ARM64 上，同一 corpus 3 次预热、20 次采样：Go 生产 Wails 桌面行为 harness 的 p95 为 18.247 ms、峰值 RSS 42,631,168 bytes；Rust/Tauri facade replay 的 p95 为 6.658 ms、峰值 RSS 8,765,440 bytes，Rust/Go 比值为 0.365/0.206，5% p95 与 10% RSS 回退门禁通过。Rust 样本没有创建 native WebView 或真实子进程，绝对启动、RSS 与二进制大小不能用作 Tauri 产品资源结论。

阶段 8 提交当时的“本地完成”只证明 facade owner、配置/路径语义、生命周期计划、Vue 双 adapter、确定性 replay 和资源门禁闭环。阶段 9 工作树随后补充了 macOS native RC、真实受管 child、`.app` smoke 和官方签名 updater 代码边界，但仍未取得任何生产业务写 owner，也未完成视觉黑屏、真实签名 updater artifact、四平台签名安装和升级数据验证；四平台 release candidate 与观察窗口完成前，阶段 8/9 均不构成产品放行，Wails 生产 owner 不变。

放行：macOS ARM64、Linux x64、Windows x64/ARM64 打包与安装 smoke；无黑屏；退出无孤儿进程；升级不丢数据。

### 阶段 9：Go 删除与 Rust 大版本发布

当前准入状态：执行中，尚未进入删除步骤。`jftrade-engine` 已增加受鉴权的独立 read-only product shadow：真实启动 loopback Axum，当前登记 26 个 GET handler，其中 immutable-catalog-read 的 `GET /api/v1/adk/agent-templates`、`GET /api/v1/research/screens/catalog` 与 appearance-read 的 `GET /api/v1/settings/ui` 已达到 C 档 cutover-qualified；alerts-read 的两个 GET、plugins-read 的三个 GET、strategy-definitions-read 的四个 GET、backtests-run-read 的三个 GET、research-preset-read 的两个 GET 与 watchlist-read 的六个 GET 通过 Go owner fixture、Rust replay、authenticated sidecar wire/error/timeout/crash/restart rehearsal 达到 cutover-qualified，但仍由 Go 保持 production owner，默认 shadow 不注册任何写入或通知副作用 route。auth-session 一个浏览器会话投影、market-data-news-actions 两个 provider-backed GET、market-data-quote-read 十个行情/订阅状态 GET、market-data-prediction-read 十二个 prediction-market GET、watchlists remote-list 一个 GET、portfolio 两个 broker-backed GET、research provider-read 十四个 GET、execution-read 三个订单/详情/事件 GET、market-data-provider-read 一个 provider status GET、market-data-catalog-read 两个 markets/instruments GET、market-data-derivatives-read 两个 warrants/futures GET、market-data-options-read 五个 option GET、brokers-read 十三个 GET、system-read 两个生命周期 GET、backtests-sync-read 一个 mutable progress GET 与 strategy-instance-read 三个策略实例/活动 GET 仅在显式 test-cutover 注入各自 consumer-owned snapshot port 时登记；adk-chat-stream 的两个 POST 与 market-data-provider-actions 的五个 provider-backed POST 仅在显式 test-cutover port 下登记，alerts-write 的两个 POST、plugins-write 的两个 POST、research-screens-write 的一个 POST、research-presets-write 的三个 POST/PATCH/DELETE、strategy-definitions-write 的五个 POST/PUT/DELETE、auth-session-write 的两个 POST、watchlist-write 的八个 POST/PATCH/PUT/DELETE、watchlists-remote-write 的一个 POST 与 adk-mutations 的 37 个 mutation/control、strategies-write 的 7 个 mutation/control、execution-write 的 7 个 POST、system-write 的 7 个 mutation、market-data-subscription-mutation 的 6 个 mutation、backtests-write 的两个 POST/两个 DELETE 仅在显式 mutation test port 下登记，Go SQLite、浏览器 session/cookie/CSRF/password 校验、OpenD、plugin catalog/runtime、plugin lifecycle、strategy store、runtime activity store、backtest run store、sync worker、broker runtime、research provider/runtime preset store、order-update worker、execution ledger/refresh worker、market-data provider lifecycle、Assistant runtime、provider quote persistence 和正式 lifecycle 仍由 Go 唯一拥有；各组均通过组级 fixture、参数化 Rust 测试与统一 product differential，默认 shadow 不注册。UI appearance、onboarding、execution、ADK timeout、security password、MCP 配置/token、system notification、Pine worker、exchange calendar、market-data/backtest Provider 选择与 broker integration/account 的兼容写实现只在临时目录 cutover 测试中启用；`route-ownership.json` v2 逐 operation 记录 method、path、capability、implementation status、production owner、Go removal status、依赖和证据，门禁当前派生为 278 个 operation、23 个 shadow、232 个 cutover-test-only、23 个 cutover-qualified、0 个 remaining、0 个 Rust production owner。Go/Wails 正式入口、所有生产写入、Web/MCP listener、SQLite 写入、Provider、交易、Assistant、WebSocket 和发布入口继续保持唯一生产 owner。

首个切片账本：

| 能力 | 当前 Go owner | Rust owner | 切换/回退 | Go 删除条件 | 状态 |
| --- | --- | --- | --- | --- | --- |
| Stage 9 product/API rehearsal | Go API、settings、SQLite、calendar、watchlist、trading 与各 integration owner | `jftrade-engine::product` 及分域 Rust crates | 默认只读 profile 保持 26 个 GET handler，其中 immutable-catalog-read 的 2 个操作与 appearance-read 的 1 个操作已 cutover-qualified；alerts-read 的 2 个操作、plugins-read 的 3 个操作、strategy-definitions-read 的 4 个操作、backtests-run-read 的 3 个操作、research-preset-read 的 2 个操作与 watchlist-read 的 6 个操作通过 authenticated sidecar rehearsal 达到 cutover-qualified，仍由 Go 保持 production owner且默认 profile 不注册；显式 test-cutover 登记 232 条 operation。auth-session、auth-session-write、market-data-news-actions、market-data-quote-read、market-data-prediction-read、market-data-provider-actions、market-data-subscription-mutation、brokers-write、adk-chat-stream、research-screens-write、settings/maintenance/calendar、alerts-write、plugins-write、research-presets-write、strategy-definitions-write、watchlist-write、watchlists、watchlists-remote-write、backtests-sync-read、backtests-write、execution-write、system-write、adk-mutations、strategies-write、portfolio、research-read、execution-read、market-data-provider-read、market-data-catalog-read、market-data-derivatives-read、market-data-options-read、brokers-read、strategy-instance-read rehearsal 只在临时数据和 fixture/snapshot/mutation ports 下运行，正式 launcher、公开 wire 与 Go production owner 不变 | 278 个 operation 全部 cutover-qualified、唯一写 owner、四平台 RC、安全/SBOM/签名 updater/备份恢复和 hard-cut readiness 完成 | 当前为 23 shadow/232 cutover-test-only/23 cutover-qualified/0 remaining/0 Rust production owner；真实 Provider、交易、Assistant、WebSocket 与发布入口仍由 Go/Wails 拥有 |
| 交易日历 manager control-plane test-cutover slice | Go `internal/exchangecalendar.Manager` | `jftrade-calendar::CalendarManager`，由 `jftrade-engine::product` 组合 | 仅当 `ProductConfig::test_cutover` 注入单一 manager 时同时登记 sources/status/probe/probe-by-market/refresh/refresh-by-market；无 manager 时六条 route 均不登记，默认 shadow 与正式 launcher 不变 | 真实 HTTP source adapters、生产 settings reload、平台恢复 differential 和 production owner 切换完成 | 动态 registry/health/cache/snapshot projection、Go control fixture differential、unknown/all-fail/partial/timeout/cancel/restore/persistence-failure 已通过；四条 POST 为 cutover-test-only，Go 仍为 production owner |
| Tauri native RC、PineTS/Python 受管发布资源 | Wails `cmd/jftrade-desktop` 与 Go child managers | `apps/desktop/src-tauri`、`jftrade-engine::product_runtime` | RC 只连接上行 read-only shadow；资源 hash、ready 与反向关闭失败即整体退出，正式 launcher 不切换 | 同一 Vue production bundle、tray/menu/notification/window state/updater、签名安装升级卸载与 4 平台无孤儿全部通过 | macOS ARM64 `.app` 构建和隔离 HOME smoke 已通过，受管 Node/Python 路径、401、ready、窗口状态、5 秒关闭与无孤儿可复现；官方 updater 代码边界已完成，真实签名 artifact、其余平台和视觉仍未闭环 |

1. 确认 278 个 operation、所有唯一写 owner、四平台 RC、签名 updater、SBOM、安全审查、回退 artifact 与备份恢复演练全部通过；不以生产观察窗口替代硬切前证据。
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
8. 四平台 RC、签名回退 artifact 与备份恢复演练全部通过后，原子切换唯一 owner；新版本不保留 Go runtime fallback。
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

Stage 9 的 Go-supervised rehearsal 必须显式启用，默认 profile 不启动第二个产品进程：

```bash
cargo build -p jftrade-engine --bin jftrade-api-rust
JFTRADE_RUST_REHEARSAL_PROFILE=read-only-shadow.v1 \
  JFTRADE_RUST_API_EXECUTABLE="$PWD/target/debug/jftrade-api-rust" \
  go run ./cmd/jftrade-api
```

Go composition root 为每个子进程生成随机 Bearer，让 Rust 绑定动态 loopback 端口，并在发布 Go router 前逐项验证 `jftrade-product-rehearsal.v1`、route profile、26 条精确 operation、profile digest、二进制 SHA-256 与 authenticated status probe。任一 ready/鉴权/资源检查失败都会回收子进程并使整个 rehearsal 启动失败。

Go transport 按 method + OpenAPI path template 精确选择普通 JSON operation。该中间件位于 desktop token、Web access、Auth、origin 与 CSRF 检查之后，只向 Rust 传递私有 Bearer、内部协议标记、已验证 access surface、request ID、query、body 和取消信号；公开 Cookie、Authorization 与 CSRF 不跨越进程边界。Rust sidecar 只在 loopback listener、Bearer、内部协议和 access surface 同时验证成功时接受请求。请求一旦交给 Rust，错误、超时或取消都不会重放 Go handler；SSE、WebSocket、文件及非 JSON 请求继续由 Go 处理。

显式 `read-only-shadow.v1` profile 当前只选择 `GET /api/v1/adk/agent-templates` 和 `GET /api/v1/research/screens/catalog`。两条 immutable catalog route 的 Go/Rust status、contract headers、envelope、request ID、query variant 和 body 已完成 differential；Rust 5xx、超时和 crash 均 fail closed。切回 Go 必须关闭 profile 并重启以创建新的 Go-only router，不允许在失败请求内回放。两条 operation 在账本中仍为 shadow，默认 profile 和 278 个 production owner 不变。

settings 文件和每个非内存 SQLite 数据库现在使用独立的 `<resource>.jftrade-owner.lock` 作为跨进程写入 fencing。Go 通过现有 `golang.org/x/sys` 的 Unix/Windows 文件锁实现，Rust 通过固定 1.97.1 工具链的 `std::fs::File` lock API 实现；两端写入相同的非敏感 `owner/pid/start/profile` 诊断。锁冲突立即 fail closed，read-only shadow 不取写锁；事务、原子 settings 替换和启动期 rebuild 删除在完整变更期间持锁。释放时只解锁并关闭句柄，不删除锁文件，避免 inode 竞态；进程崩溃后的锁由 OS 释放。该 fencing 只允许显式 rehearsal 的临时写入演练，不改变 Go 的 production owner。

data-management 的 cleanup execute、backup、compact 与 rebuild 现在只在 `cutover-test-only.v1` 的临时数据库中登记。execute 一次性消费 preview token，在同一个 `BEGIN IMMEDIATE` 事务内按原 cutoff/retention 重新计算完整候选指纹并删除，集合漂移或过期均拒绝且写请求不重试；backup 使用 SQLite `VACUUM INTO` 后验证 `quick_check`、foreign keys、大小与 SHA-256；compact 在可验证备份后执行 checkpoint/VACUUM；rebuild 为全部目标先持有独立 writer lease、生成或复验备份，再同目录 fsync 并原子替换 marker，不在请求中删除源库。四条 route 因此变为 cutover-test-only，当前门禁派生为 26 shadow/26 cutover-test-only/226 remaining/0 Rust production owner；默认 profile、正式 Go owner、SQLite schema 和公开 wire 不变。

`jftrade-calendar` 已增加与 Go store 相同的 `MARKET/YYYY/source.json` snapshot 持久化：字段顺序、`omitempty`、RFC3339Nano offset/fraction、末尾换行和本地年份由 Go-owned fixture 锁定；写入使用同目录临时文件、`fsync`、原子替换和 Unix 目录同步，Unix 权限为目录 `0755`、文件 `0644`，Windows 通过安全的跨平台原子 persist 路径替换。读取是纯只读递归扫描，缺失 root、权限错误、截断和损坏 JSON 均逐文件报告，不会创建目录或用坏文件覆盖已成功加载的 snapshot。该 store 已由 fixture-backed manager lifecycle 消费，但尚未接入公开 route 或正式 launcher，账本与 Go production owner 不变。

`jftrade-calendar` manager lifecycle 已在 fixture source 边界内实现：动态 registry 和稳定优先级、manual/external/builtin policy、持久化 snapshot restore 与内存 cache、source health 和 1 至 24 小时失败退避、settings reload、start/close/cancel。启动任一 source 失败会逆序关闭已经启动的 source 并进入 closed 状态；Close 幂等并等待 worker 退出；新 snapshot 必须先持久化成功才替换内存 cache，因此持久化失败仍保留上一份有效 snapshot。外部 source 只通过窄 port 注入，本批没有复制真实 HTTP Provider；manager 只接入显式 test-cutover profile，正式 launcher 与 Go production owner 不变。

calendar control-plane 的 test-cutover 现在只接受一个真实 `CalendarManager`，不再分别注入 sources/status 静态 snapshot port。同一 manager 动态投影 registry、source health、effective market、cache 和 snapshot summary，并执行 probe、probe-by-market、refresh、refresh-by-market；无 manager 时六条 route 均不登记。probe 不写 cache，refresh 先持久化后替换 cache；未知市场保持 Go-compatible accepted no-op，全失败、部分成功、协作式 timeout/cancel、restore 和持久化失败均有拒绝或恢复测试。四条 POST 由 remaining 转为 cutover-test-only 后，门禁派生为 26 shadow/30 cutover-test-only/222 remaining/0 Rust production owner。真实 HTTP source、正式 launcher 和 production owner 仍保持 Go。

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
| 2026-08-20 | 阶段 9 data-management cleanup preview 纳入 cutover-test-only 账本，并登记两条静态只读 catalog shadow；不扩大 Rust 生产 owner | 该工作包先登记 cleanup preview、catalog 与 calendar source test-cutover；cleanup preview 只生成并校验候选预览，SQLite adapter 保持只读，cleanup/execute、backup、compact、rebuild 等写入操作不注册、不执行；后续 calendar status test-cutover slice 已将当前账本统一为 26 shadow/22 cutover-test-only/230 remaining，Go 仍是唯一生产写 owner |
| 2026-08-21 | Stage 9 路由与正式收口账本升级为逐 operation、硬切前证据模型 | 278 个 operation 逐条记录实现状态、生产 owner、Go removal、依赖和证据，所有计数由门禁派生；正式收口以 hard-cut readiness、签名 rollback artifact、backup/restore drill 和立即 post-release smoke 取代生产观察窗口。当前仍为 26 shadow/22 cutover-test-only/230 remaining/0 Rust production owner，Go/Wails 正式入口不变 |
| 2026-08-21 | 收敛旧 schema-pack consumer worktree 的有效 SQLite 拒绝行为，不恢复旧 pack 架构 | 当前九库机械冻结 schema fixture 直接覆盖只读字节不变、metadata 错误优先级、静态与动态表名/结构、显式与 partial index、foreign key、trigger/view、STRICT/WITHOUT ROWID 和截断数据库拒绝；验证后退役两个未合并 schema-pack worktree，生产 SQLite 仍由 Go 唯一写入 |
| 2026-08-21 | 拆分 Stage 9 product composition 与 route contributors，并以 capability set 取代全局 write-owner 开关 | system、settings、calendar、data-management、watchlist/research/trading route 分域贡献；17 个 test-cutover capability 可独立登记 route，端口型 capability 仍同时要求窄 port。新增 800 行 product 生产文件门禁，默认 capability set 为空且 route/wire/Go production owner 不变 |
| 2026-08-21 | 拆分 Tauri native runtime adapters，保持既有 facade 与发布资源契约 | lifecycle、resource integrity/runtime、window/tray、notification/updater、logs 分域落盘，主 native module 仅保留装配；800 行 Rust 生产文件门禁同时覆盖 native 文件族，commands、events、窗口行为、资源路径和 Go/Wails production owner 不变 |
| 2026-08-21 | Go composition root 在显式 read-only rehearsal profile 下管理 Rust product sidecar | 默认不启动；显式 profile 使用动态 loopback、每进程随机 Bearer、固定 ready 协议、26-operation profile digest、capability 列表、可执行文件 SHA-256 与 authenticated probe，验证完成前不发布 Go router；失败逆序回收且不静默回退。当前尚未代理公开请求，278 个 production owner 仍全部为 Go |
| 2026-08-21 | 建立 exact-operation Go-to-Rust rehearsal proxy，暂不启用 operation | 只选择 method + OpenAPI template 精确命中的普通 JSON；鉴权与 access surface 在 Go 完成，Rust 再验证私有 Bearer 和内部协议；request ID/query/body/cancel 透传，失败或超时不得回放 Go，SSE/WS/文件保持 Go owner；当前选择集为空，账本和 production owner 不变 |
| 2026-08-21 | 在显式 rehearsal profile 中演练两条 immutable catalog shadow route | 仅选择 agent templates 与 research screen catalog；status、contract headers、envelope、request ID、query/body 完成 wire differential，5xx/timeout/crash 均不回放 Go，关闭 profile 后重启才恢复 Go-only router。账本仍为 26 shadow/22 cutover-test-only/230 remaining/0 Rust production owner |
| 2026-08-21 | 为 settings 与每个 SQLite 资源增加 Go/Rust 跨进程 writer lease | 使用持久 `*.jftrade-owner.lock`、统一非敏感诊断和 OS crash-release 语义；Go settings、统一 SQLite 写事务与启动期 rebuild 删除均 fail closed，Rust settings 写入使用同一协议，read-only shadow 不取锁。Windows/Linux Go 交叉编译、冲突/事务/崩溃释放测试与 Rust workspace 门禁通过；production owner 与 route 计数不变 |
| 2026-08-21 | 在临时数据库演练 fenced data-management maintenance | cleanup execute 事务内重验 preview token/expiry/完整候选集且不重试；backup/compact/rebuild 使用 per-database writer lease、SQLite integrity/FK 校验、SHA-256 与原子 marker，compact/rebuild 前保留可恢复备份。四条 route 仅为 cutover-test-only，账本为 26 shadow/26 cutover-test-only/226 remaining/0 Rust production owner，正式 Go owner 不变 |
| 2026-08-21 | 为 Rust calendar 增加 Go-compatible snapshot 持久化 | Go-owned fixture 锁定 `MARKET/YYYY/source.json`、RFC3339Nano JSON、末尾换行和 Unix `0755/0644`；Rust 同目录临时文件经过 file fsync、原子替换和目录同步，加载不创建路径并隔离遍历、权限、截断和损坏文件。manager/route 尚未接入，账本和 production owner 不变 |
| 2026-08-21 | 为 Rust calendar 增加 manager lifecycle | registry、manual/external/builtin policy、snapshot restore/cache、source health/backoff、settings reload 与 start/close/cancel 已由 fixture source 覆盖；启动失败逆序释放，Close 幂等，持久化失败保留有效内存 snapshot。真实 HTTP Provider、公开 route 与正式 launcher 尚未接入，账本和 Go production owner 不变 |
| 2026-08-21 | 在 test-cutover 接入单一 Rust calendar manager control-plane | sources/status 与 probe/refresh 四组操作共享 registry、health、cache 和 lifecycle；无 manager 不登记 route，unknown/all-fail/partial/timeout/cancel/restore/persistence-failure 已覆盖。新增四条 cutover-test-only route 后账本为 26 shadow/30 cutover-test-only/222 remaining/0 Rust production owner；真实 HTTP source、正式 launcher 和 Go production owner 不变 |
| 2026-08-20 | 拒绝将 `GET /api/v1/brokers/capabilities` 与 `GET /api/v1/market-data/markets` 登记为 Rust shadow route | 两条 route 依赖 Go 运行时 evaluator、OpenD/account/quote 权限及 active provider/sidecar 的动态能力语义，静态 fixture 无法安全伪造；待 broker/runtime capability port 与 market-data provider lifecycle 建成，并完成真实状态、权限、失败恢复 differential 后再重新评审；该决定不新增 route，当前总账为 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-20 | 拒绝将 `GET /api/v1/research/screens/presets` 与 `GET /api/v1/research/screens/presets/{presetId}` 登记为 Rust shadow route | Go `NormalizeDefinitionV2` 的完整规范化/校验语义与 research SQLite read-only adapter 尚未具备，不能用简单 JSON object 校验或创建数据库伪造 preset wire；待规范化规则、只读 store port、schema/恢复 differential 和拒绝路径完成后再评审；该决定不新增 route，当前总账为 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-20 | 拒绝将 `GET /api/v1/system/exchange-calendars/sources` 登记为 Rust production/shadow route | 该 route 依赖 Go ExchangeCalendar Manager 的动态 registry、status、cache 与 health 语义；Rust 当前仍不能用静态数据或空返回伪造正式 owner。允许独立的 consumer-owned snapshot port 仅在 test-cutover 注入时登记该 route，正式 launcher 与 Go 生产 owner 不变；sources 本身不新增 production/shadow owner，当前总账为 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-20 | 先落地 `jftrade-calendar` source descriptor/status provider-neutral 纯模型，并将 sources route 限制在 test-cutover port | 纯投影覆盖默认 registry、legacy source ID alias、enabled 去重、状态时间字段与恢复/告警 wire，并由 Go owner fixture differential 锁定；跨进程 manager/runtime port、动态 cache/health 与远端刷新仍未具备，不能扩大 Rust production/shadow owner；source slice 本身不新增 production/shadow owner，当前总账为 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-20 | Stage 9 remaining GET 全量静态候选复审完成，暂不新增 shadow route | ADK tools/providers/runs、research presets/data、calendar sources、broker capabilities、market-data/plug-in/prediction/portfolio/strategy/watchlist 等均依赖运行时 registry、SQLite、provider、权限或生命周期；在消费方窄 port、版本化 snapshot/RPC、失败恢复 differential 前，静态成功响应会制造错误 owner 事实；保持 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-20 | `GET /api/v1/system/exchange-calendars/sources` 仅作为 test-cutover port slice 接入，不登记为 production/shadow owner | `jftrade-engine` 增加 consumer-owned `CalendarSourceSnapshotPort`；只有注入 Go manager fixture 时才注册 route，默认 shadow 不注册；Go fixture differential、成功 envelope、snapshot port unavailable 的 503 fail-closed 与 route coverage 均通过；route ownership 当前为 26 shadow/22 cutover-test-only/230 remaining，正式 Go owner、OpenAPI、SQLite 不变 |
| 2026-08-20 | `GET /api/v1/system/exchange-calendars/status` 仅作为独立 status-port test-cutover slice 接入，不登记为 production/shadow owner | `jftrade-engine` 增加 consumer-owned `CalendarStatusSnapshotPort`；只有注入固定 clock/settings/registry/snapshot 的 Go manager fixture 时才注册 route，默认 shadow 不注册；完整 status wire、sample schedules、source health、status-only/both-port route isolation、成功 envelope 与 status port unavailable 的 503 fail-closed 均通过；route ownership 更新为 26 shadow/22 cutover-test-only/230 remaining，正式 Go owner、OpenAPI、SQLite 与远端刷新 lifecycle 不变 |
| 2026-08-20 | `GET /api/v1/watchlist/instruments/{market}/{symbol}/memberships` 仅作为 test-cutover port slice 接入，不登记为 production/shadow owner | `jftrade-engine` 增加消费方定义的 `WatchlistMembershipSnapshotPort`，`jftrade-watchlist` 严格规范化 US/HK/SH/SZ 与 CNSH/CNSZ 别名，并以 Go SQLite fixture differential 锁定 revision/groups 投影；仅注入 fixture port 时注册，未知或 port 不可用均 fail-closed，默认 shadow、正式 launcher、Go watchlist/SQLite owner 不变；route ownership 当前为 26 shadow/22 cutover-test-only/230 remaining |
| 2026-08-21 | `alerts-read` 组完成 test-cutover 批量切片 | 两个 GET 通过统一 Go fixture/Rust differential 与参数化测试；仅注入 `AlertSnapshotPort` 时登记，默认 shadow 不注册，Go/OpenD 仍为唯一 production owner；总账更新为 26 shadow/32 cutover-test-only/220 remaining |
| 2026-08-21 | `strategy-definitions-read` 组完成 test-cutover 批量切片 | 四个 GET 覆盖 list/detail/preview/versions/history/404；仅注入 `StrategyDefinitionSnapshotPort` 时登记，Rust 不打开 strategy SQLite；与 alerts 组安全边界统一后，总账为 26 shadow/36 cutover-test-only/216 remaining/0 Rust production owner |
| 2026-08-21 | `watchlists-read` 组完成 test-cutover 批量切片 | 一个 GET `/api/v1/watchlists/remote` 通过统一 Go fixture/Rust differential 与参数化测试；仅注入 `RemoteWatchlistSnapshotPort` 时登记，默认 shadow 不注册，Go broker registry/OpenD 与 watchlist state 仍为唯一 production owner；总账更新为 26 shadow/74 cutover-test-only/178 remaining/0 Rust production owner |
| 2026-08-21 | `system-read` 组完成 test-cutover 批量切片 | 两个生命周期依赖 GET（OpenD health、broker order-update worker）通过统一 Go fixture/Rust differential 与参数化测试；仅注入 `SystemReadSnapshotPort` 时登记，默认 shadow 不注册，Go OpenD、broker registry、order-update worker 与交易状态仍为唯一 production owner；总账更新为 26 shadow/76 cutover-test-only/176 remaining/0 Rust production owner |
| 2026-08-21 | `backtests-run-read` 组完成 test-cutover 批量切片 | 三个本地 run projection GET（列表、状态、完整结果）通过统一 Go fixture/Rust differential 与参数化测试；仅注入 `BacktestReadSnapshotPort` 时登记，默认 shadow 不注册，Go run store、PineTS、market-data sync 与 SQLite 写入仍为唯一 production owner；总账更新为 26 shadow/79 cutover-test-only/173 remaining/0 Rust production owner |
| 2026-08-21 | `backtests-sync-read` 组完成 test-cutover 切片 | 一个依赖进程内任务生命周期的 GET（同步进度）通过固定 Go fixture、Rust snapshot adapter、参数化测试与统一 product differential；仅注入 `BacktestSyncReadSnapshotPort` 时登记，默认 shadow 不注册，Go sync worker、task store、Provider/OpenD 与取消/写入仍为唯一 production owner；总账更新为 26 shadow/80 cutover-test-only/172 remaining/0 Rust production owner |
| 2026-08-22 | `strategy-instance-read` 组完成 B 档 test-cutover 批量切片 | 三个策略实例/活动 GET 通过 Go catalog fixture、分页/时间过滤与错误分支 differential、Rust `StrategyReadSnapshotPort` 参数化测试和 fail-closed route isolation；Go catalog、definition sync、runtime observation/activity store、PineTS、生命周期和写入仍为唯一 production owner；总账更新为 26 shadow/83 cutover-test-only/169 remaining/0 Rust production owner |
| 2026-08-22 | `research-preset-read` 组完成 C 档 test-cutover 批量切片 | 两个预设 GET 通过固定 Go service/repository fixture、完整 `ScreenPreset` wire projection、Rust `ResearchPresetReadSnapshotPort` 参数化测试和 mutation route isolation；Go `NormalizeDefinitionV2`、research SQLite、revision 与所有 preset 写入仍为唯一 production owner；总账更新为 26 shadow/85 cutover-test-only/167 remaining/0 Rust production owner |
| 2026-08-22 | `execution-read` 组完成 B 档 test-cutover 批量切片 | 三个订单只读 GET 通过 Go service fixture 覆盖 active/list filters、订单刷新/详情、recent event truncation、未知事件 ID 和 list/detail/event failures；Rust `ExecutionReadSnapshotPort` 仅在显式 test-cutover 注册，execution ledger、order-update worker、broker/OpenD、权限和所有写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/88 cutover-test-only/164 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-provider-read` 组完成 B 档 test-cutover 批量切片 | 一个 provider status GET 通过 Go provider fixture 覆盖 descriptor、健康成功/degraded、runtime/subscription 投影与 provider failure；Rust `MarketDataProviderReadSnapshotPort` 仅在显式 test-cutover 注册，provider/OpenD 生命周期、订阅、缓存和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/89 cutover-test-only/163 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-catalog-read` 组完成 B 档 test-cutover 批量切片 | 两个 markets/instruments GET 通过 Go Provider fixture 覆盖成功、输入错误、descriptor/market/search failure；Rust `MarketDataCatalogReadSnapshotPort` 仅在显式 test-cutover 注册，Provider/OpenD 生命周期、resolver/cache、订阅和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/91 cutover-test-only/161 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-derivatives-read` 组完成 B 档 test-cutover 批量切片 | 两个 warrants/futures GET 通过 Go `DerivativeCatalogReader` fixture 覆盖 warrants list/related/screen、future contracts 和 capability unavailable；Rust `MarketDataDerivativeReadSnapshotPort` 仅在显式 test-cutover 注册，Provider/OpenD 生命周期、broker resolution、缓存和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/93 cutover-test-only/159 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-options-read` 组完成 B 档 test-cutover 批量切片 | 五个 option GET 通过 Go `FeatureQuery` fixture 覆盖 chain/expiration/screen/analysis/events 和 capability unavailable；Rust `MarketDataOptionsReadSnapshotPort` 仅在显式 test-cutover 注册，Provider/OpenD 生命周期、broker resolution、订阅和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/98 cutover-test-only/154 remaining/0 Rust production owner |
| 2026-08-22 | `auth-session` 组完成 B 档 test-cutover 切片 | `GET /api/v1/auth/session` 通过 Go owner fixture 覆盖无 cookie、浏览器 session、可信 desktop、允许/拒绝 Origin、CORS/request-ID/`Cache-Control: no-store` 头与 error precedence；Rust `AuthSessionSnapshotPort` 仅在显式 test-cutover 注册，Go 浏览器 session、cookie、CSRF、password 验证和失效仍为唯一 owner；总账更新为 26 shadow/99 cutover-test-only/153 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-news-actions-read` 组完成 B 档 test-cutover 批量切片 | 两个 provider-backed GET 通过 Go Provider fixture 覆盖 limit/range validation、CN leaf normalization、null/empty arrays、capability/provider-change/fallback/warming/busy 分支和精确 `Retry-After: 1|2`；Rust `MarketDataNewsActionsReadSnapshotPort` 仅在显式 test-cutover 注册，共享 `ApiFailure` transport 保留可选 retry metadata，Provider/OpenD、sidecar、cache、subscription 和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/101 cutover-test-only/151 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-quote-read` 组完成 B 档 test-cutover 批量切片 | 十个 provider/broker-backed GET 通过 Go quote fixture 覆盖空订阅、security/candle/snapshot/depth 成功与空结果、输入校验、capability/provider failure、warming/busy 和精确 `Retry-After`；Rust `MarketDataQuoteReadSnapshotPort` 仅在显式 test-cutover 注册，重复 request key 的 fixture harness 已按顺序复核，Provider/OpenD、subscription demand、cache 和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/111 cutover-test-only/141 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-prediction-read` 组完成 B 档 test-cutover 批量切片 | 十二个 prediction-market GET 通过 Go eligibility/capability/provider fixture 覆盖 discovery、contract history/depth/snapshot、空结果、ineligible account、missing broker、provider failure/warming/busy 与 retry metadata；Rust `MarketDataPredictionReadSnapshotPort` 仅在显式 test-cutover 注册，eligibility、Provider/OpenD、subscription 和所有 prediction writes 仍由 Go 唯一拥有；总账更新为 26 shadow/123 cutover-test-only/129 remaining/0 Rust production owner |
| 2026-08-22 | `market-data-news-search-read` 组完成 B 档 test-cutover 批量切片 | 一个 provider-backed news search GET 通过 Go Provider fixture 覆盖 embedded provider precedence、query normalization/pagination、empty/null entries、capability/provider failure、fallback、warming/busy、missing instrument 与 malformed page size；Rust `MarketDataNewsSearchReadSnapshotPort` 仅在显式 test-cutover 注册，Provider/OpenD、sidecar、cache、subscription 和所有 market-data 写 route 仍由 Go 唯一拥有；总账更新为 26 shadow/124 cutover-test-only/128 remaining/0 Rust production owner |
| 2026-08-22 | `adk-read` 组完成 B 档 test-cutover 批量切片 | 24 个 ADK GET 通过 fresh Go testkit store、in-memory session service、完整 snapshot/error fixture 和 Rust replay 覆盖成功投影、分页 query、malformed percent query、动态 ID、资源 404 与 stream error paths；Rust `AdkReadSnapshotPort` 仅在显式 test-cutover 注册，ADK SQLite、Provider、session/runtime、任务/审批/会话写入与 SSE 成功 stream header 仍由 Go 唯一拥有；当前总账为 26 shadow/148 cutover-test-only/104 remaining/0 Rust production owner，成功 SSE corpus/header 兼容仍是 qualification blocker |
   | 2026-08-22 | `ws-live` 组完成 B 档 test-cutover 接入 | `GET /api/v1/ws/live` 通过 Go WebSocket corpus、Rust replay、现有 authenticated loopback transport 和 product route-isolation test；仅显式 `WsLiveSnapshotPort` 存在时注册，默认 profile 不注册，Go live registry、Provider/OpenD、subscription、notification replay、market ticks、depth bridge 与 WebSocket production owner 不变；总账更新为 26 shadow/149 cutover-test-only/103 remaining/0 Rust production owner；Origin plain-text、abnormal close、replay ordering 和缺失 `docs/swagger` harness quirks 仍阻塞 qualification |
   | 2026-08-22 | `strategy-pine` 组完成 B 档 test-cutover 接入 | `POST /api/v1/strategy-pine/analyze` 通过 Go-generated analysis/worker fixture、Rust leaf replay、product envelope/error/retry 测试和 route isolation；仅显式 `StrategyPineAnalyzeSnapshotPort` 存在时注册，默认 profile 不注册，Go Pine parser、PineTS worker lifecycle、analysis metadata 和 shadow projection 仍为唯一 production owner；总账更新为 26 shadow/150 cutover-test-only/102 remaining/0 Rust production owner；worker cancellation context、shadow-error precedence 和完整 release/recovery 证据仍阻塞 qualification |
   | 2026-08-22 | `market-data-provider-actions` 组完成 B 档 test-cutover 接入 | 五个 provider-backed POST（instrument normalize、option analysis、zero-DTE contracts、prediction combo quotes、batch snapshots）通过 Go provider fixture、Rust raw-request replay、精确状态/错误/`Retry-After` 与 product route isolation；仅显式 `MarketDataProviderActionsPort` 存在时注册，Go Provider/OpenD lifecycle、subscription demand、SQLite 与 prediction quote persistence 仍是唯一 owner；总账更新为 26 shadow/159 cutover-test-only/93 remaining/0 Rust production owner；combo quote 持久化、真实 provider lifecycle、四平台发布、安全、恢复与 hard-cut 仍阻塞 qualification |
   | 2026-08-22 | `adk-chat-stream` 组完成 B 档 test-cutover 接入 | 两个 ADK chat POST 通过 Go fixture、Rust JSON/SSE replay、原样 `X-ADK-*` headers、retry/event framing、错误 precedence 与 product transport route isolation；仅显式 `AdkChatStreamPort` 存在时注册，Go Assistant runtime、Provider、session/run lifecycle、SQLite、auth/CSRF middleware 与 background reconnect hub 仍是唯一 owner；总账更新为 26 shadow/161 cutover-test-only/91 remaining/0 Rust production owner；disconnect/reconnect replay、product auth/CSRF differential、四平台发布、安全、恢复与 hard-cut 仍阻塞 qualification |
   | 2026-08-23 | `auth-session-write` 组完成 A 档 test-cutover 接入 | `POST /api/v1/auth/login` 与 `POST /api/v1/auth/logout` 通过 Go Web access/middleware fixture、Rust leaf replay、rate-limit/header quirk 复核和 product raw-response route isolation；仅显式 `AuthSessionWritePort` 注册，Go password/session/cookie/CSRF/rate limiter 仍是唯一 production owner；总账更新为 26 shadow/171 cutover-test-only/81 remaining/0 Rust production owner；重复请求、取消/超时、session restart/recovery、安全、签名发布和 hard-cut 仍阻塞 qualification |
   | 2026-08-23 | `watchlists-remote-write` 组完成 A 档 test-cutover 接入 | `POST /api/v1/watchlists/remote` 通过 Go broker-feature fixture、Rust leaf replay、payload-state/repeated-write/recovery/rate-limit quirks 和 product raw-response route isolation；仅显式 `RemoteWatchlistWritePort` 注册，Go broker registry/OpenD/remote watchlist state 仍是唯一 production owner；总账更新为 26 shadow/172 cutover-test-only/80 remaining/0 Rust production owner；幂等、取消/超时、恢复、真实 provider、安全、签名发布和 hard-cut 仍阻塞 qualification |
   | 2026-08-23 | `backtests-write` 组完成 A 档 test-only rehearsal | `POST /api/v1/backtests`、`POST /api/v1/backtests/sync`、两个 DELETE mutation 通过 Go owner fixture（38 cases/40 requests）、Rust leaf replay、malformed-escape/blank-task/error-precedence quirk 复核和 product raw-response route isolation；仅显式 `BacktestsWritePort` 注册，Go run/task store、PineTS、market-data sync、SQLite 与异步恢复仍是唯一 production owner；总账更新为 26 shadow/176 cutover-test-only/72 remaining/0 Rust production owner，重复 sync-start、幂等、取消恢复和 durable owner 证据仍阻塞 qualification |
  | 2026-08-23 | `watchlist-write` 组完成 A 档 test-only rehearsal | 八个本地 watchlist mutation route 通过 Go owner fixture（45 cases）、Rust leaf replay、missing binding empty-string/JSON binding/cancellation envelope quirks 和 product raw-response route isolation；仅显式 `WatchlistWritePort` 注册，Go watchlist SQLite、quote/provider/OpenD、import transaction 与副作用仍是唯一 production owner；与 backtests-write 集成后总账更新为 26 shadow/184 cutover-test-only/68 remaining/0 Rust production owner，真实事务恢复、幂等、安全、签名发布和 hard-cut 仍阻塞 qualification |
   | 2026-08-23 | `strategies-write` 组完成 A 档 test-only rehearsal | 七个 strategy-instance mutation/control route 通过 Go owner fixture（35 cases/36 requests）、Rust leaf replay、null/trailing JSON、pause failure-code、start cancellation/timeout 与 repeated-pause quirks 三方复核；仅显式 `StrategyRuntimeWritePort` 注册，Go strategy catalog/runtime manager、PineTS、subscriptions、activity/notification side effects 与 SQLite 仍为唯一 production owner；总账更新为 26 shadow/228 cutover-test-only/24 remaining/0 Rust production owner，重复请求、取消恢复、真实 runtime fencing、安全、签名发布和 hard-cut 仍阻塞 qualification
   | 2026-08-23 | `adk-mutations` 组完成 A 档 test-only rehearsal | 37 个 Assistant mutation/control route 通过 Go owner fixture（40 cases）、Rust leaf replay、session/context dynamic ID normalization、empty-suffix matcher、skill/provider/workflow error projection quirks 三方复核；仅显式 `AdkMutationPort` 注册，Go Assistant runtime、provider、session/task/approval/workflow/skill stores、notifications 与 SQLite 仍为唯一 production owner；总账保持为 26 shadow/228 cutover-test-only/24 remaining/0 Rust production owner，真实副作用恢复、认证 loopback、唯一 owner、安全、签名发布和 hard-cut 仍阻塞 qualification
   | 2026-08-23 | `execution-write` 组完成 A 档 test-only 接入 | 七个 execution buying-power/combo/order preview/place/cancel route 通过 Go owner fixture（57 cases/62 requests）、Rust leaf replay、product raw-response route isolation 与统一 differential；仅显式 `ExecutionWritePort` 注册，Go broker/OpenD、risk、execution ledger、order-update worker、SQLite 与所有副作用仍为唯一 production owner；总账更新为 26 shadow/242 cutover-test-only/10 remaining/0 Rust production owner；重复 durable idempotency、取消 fencing、恢复、安全、签名发布和 hard-cut 仍阻塞 qualification
   | 2026-08-23 | `system-write` 组完成 A 档 test-only 接入 | 七个 OpenD retry/real-trade risk-limit/hard-stop/kill-switch route 通过 Go owner fixture（47 cases/68 requests）、Rust leaf replay、fail-closed port isolation 与统一 differential；仅显式 `SystemWritePort` 注册，Go OpenD、真实交易安全控制、持久化、broker fencing、通知与正式入口仍为唯一 production owner；总账保持为 26 shadow/242 cutover-test-only/10 remaining/0 Rust production owner；取消/409 语义、重复请求、安全评审、恢复、签名发布和 hard-cut 仍阻塞 qualification
   | 2026-08-23 | `market-data-subscription-mutation` 组完成 A 档 test-only 接入 | 六个行情订阅/预测市场 lease mutation route 通过 Go owner fixture（55 cases/55 requests）、Rust leaf replay、fail-closed port isolation 与统一 differential；仅显式 `MarketDataSubscriptionMutationPort` 注册，Go 订阅 demand、prediction eligibility、Provider/OpenD lifecycle、lease state、持久化与 cleanup 仍为唯一 production owner；总账更新为 26 shadow/248 cutover-test-only/4 remaining/0 Rust production owner；durable lease idempotency、recovery、取消映射和 no-double-write 仍阻塞 qualification
   | 2026-08-23 | `brokers-write` 组完成 A 档 test-only 接入 | 三个 broker place/cancel/unlock mutation route 通过 Go owner fixture（65 cases/68 requests）、Rust leaf replay、fail-closed port isolation 与统一 differential；仅显式 `BrokersWritePort` 注册，Go broker/OpenD、risk、order state、SQLite、notifications 与 trading side effects 仍为唯一 production owner；总账更新为 26 shadow/251 cutover-test-only/1 remaining/0 Rust production owner；重复 mutation、取消/超时、安全、恢复、签名发布和 hard-cut 仍阻塞 qualification

   | 2026-08-23 | `research-screens-write` 组完成 A 档 test-only 接入 | `POST /api/v1/research/screens` 通过 Go owner fixture（22 cases/27 requests）、Rust leaf replay、strict JSON/null、query normalization、cache、provider retry/error 与 concurrent fixture quirks 三方复核；仅显式 `ResearchScreenWritePort` 注册，Go research service/provider/cache/runtime 与所有持久化状态仍为唯一 production owner；总账更新为 26 shadow/252 cutover-test-only/0 remaining/0 Rust production owner；cache ownership、provider adapter failure evidence、唯一 owner、安全、恢复、签名发布和 hard-cut 仍阻塞 qualification |
   | 2026-08-23 | `immutable-catalog-read` 组完成 C 档 qualification | `GET /api/v1/adk/agent-templates` 与 `GET /api/v1/research/screens/catalog` 通过 Go reference、全 provider/market/error fixture、Rust replay、authenticated loopback wire differential、Rust error/timeout/crash fail-closed 与 restart-time Go rollback；总账更新为 24 shadow/252 cutover-test-only/2 cutover-qualified/0 remaining/0 Rust production owner；Go production owner、唯一 owner、四平台 release、签名、安全、恢复和 hard-cut gates 仍未关闭 |
   | 2026-08-23 | `appearance-read` 组完成 C 档 qualification | `GET /api/v1/settings/ui` 通过 Go sidecar wire reference、缺省/规范化/独立 fallback/`null` fixture、authenticated Rust shadow replay、Rust read-only file fencing、error/timeout/crash fail-closed 与 restart-time Go rollback；三方复核确认 Go 对 `null` appearance 返回默认值，Rust settings-file adapter 已兼容为 absent optional field；总账更新为 23 shadow/252 cutover-test-only/3 cutover-qualified/0 remaining/0 Rust production owner；Go production owner、唯一 owner、四平台 release、签名、安全、恢复和 hard-cut gates 仍未关闭 |
   | 2026-08-23 | `alerts-read` 组完成 C 档 qualification | 两个 alerts GET 通过 Go owner wire/error fixture（空结果、capability/provider failure）、Rust `AlertSnapshotPort` replay、Go authenticated sidecar wire/error/timeout/crash/restart rehearsal 与 read-only fencing；三方复核确认 Rust 原有错误映射差异已补齐为 Go-compatible 409/502/503；总账更新为 23 shadow/248 cutover-test-only/7 cutover-qualified/0 remaining/0 Rust production owner；Go broker/OpenD 与正式 owner 不变，默认 shadow 不注册该 snapshot-port route |
   | 2026-08-23 | `plugins-read` 组完成 C 档 qualification | 两个 plugins GET 通过 Go owner fixture（空 catalog、404、非法 percent escape、headers）、Rust replay、malformed escape 兼容修复与 Go authenticated sidecar wire/error/timeout/crash/restart rehearsal；三方复核确认 Go `%ZZ` 400 行为已由 Rust route adapter 复刻；总账保持为 23 shadow/248 cutover-test-only/7 cutover-qualified/0 remaining/0 Rust production owner；Go plugin catalog/runtime 与正式 owner 不变，默认 shadow 不注册该 snapshot-port route |
   | 2026-08-23 | `strategy-definitions-read` 组完成 C 档 qualification | 四个 strategy definitions GET 通过 Go owner fixture（preview、history、soft-delete、malformed path/query）、Rust replay、malformed escape/query compatibility 修复与 Go authenticated sidecar wire/error/timeout/crash/restart rehearsal；三方复核确认 Go strategy store、preview derivation 与 restart rollback 仍由 Go 唯一拥有，Rust 仅消费只读 snapshot port；总账更新为 23 shadow/244 cutover-test-only/11 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-23 | `backtests-run-read` 组完成 C 档 qualification | 三个 backtest run GET 通过 Go owner fixture（empty list、blank ID 404、malformed escape 400、store failure）、Rust replay、Go authenticated sidecar wire/error/timeout/crash/restart rehearsal；三方复核确认 Go run store、SQLite、PineTS、sync worker 与 provider lifecycle 仍由 Go 唯一拥有，Rust 仅消费只读 snapshot port；总账更新为 23 shadow/241 cutover-test-only/14 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-23 | `research-preset-read` 组完成 C 档 qualification | 两个 research preset GET 通过 Go owner fixture、Rust snapshot replay、authenticated sidecar wire/error/timeout/crash/restart rehearsal 与重启后 Go rollback；三方复核确认 Go research preset SQLite/service、`NormalizeDefinitionV2` 与所有写 route 仍由 Go 唯一拥有，Rust 仅消费只读 snapshot port；总账更新为 23 shadow/239 cutover-test-only/16 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-24 | `plugins-read` 组补齐 uninstall-guidance C 档 qualification | 第三个 plugins GET 通过 Go handler reference、完整 status/message/quoted command fixture、Rust replay、非法 `%ZZ` 兼容修复及既有 authenticated sidecar wire/error/timeout/crash/restart rehearsal；Rust 仍只消费显式只读 snapshot port，不探测文件、不执行命令，Go plugin catalog/runtime/lifecycle 与两个 mutation 仍为唯一 production owner；总账更新为 23 shadow/238 cutover-test-only/17 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-24 | `watchlist-read` 组完成 C 档 qualification | 六个本地 watchlist GET 通过真实 Go Gin handler fixture（11 cases，覆盖空结果、query、400/404）、Rust path+query/error replay、authenticated sidecar wire/error/timeout/crash/restart rehearsal与失败不回放 Go 证明；`GET /sources` 的 Go-owned source cache upsert 记录为 medium compatibility quirk，Rust 仍只消费显式 snapshot port，不打开 SQLite、不激活 source reader，Go watchlist store/refresh/全部 mutation 仍为唯一 production owner；总账更新为 23 shadow/232 cutover-test-only/23 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-24 | 普通 JSON GET qualification 收口并补 `research-presets-write` mutation fencing rehearsal | 122 个普通 JSON GET 已达到 cutover-qualified；仅 ADK SSE、auth browser session 与 live WebSocket 四个特殊 GET 保持 test-only。research presets 三条 mutation 经 authenticated loopback composition rehearsal 验证 exact-operation 路由、cookie 隔离、重复冲突、revision fence、失败/超时/crash 不回放 Go、隔离数据库重启恢复及 Go-only rollback 不双写；该 boundary 仍使用临时 Go reference owner，Rust durable repository 未实现，所以 route 保持 test-only。总账为 23 shadow/133 cutover-test-only/122 cutover-qualified/0 remaining/0 Rust production owner |
   | 2026-08-24 | `research-presets-write` 增加 Rust durable test-cutover store | `jftrade-store-sqlite` 新增仅接受 `cutover-test-only.v1`、只打开既有 Go-compatible schema、持有跨进程唯一 writer lease 的 research preset store；CRUD、duplicate conflict、原子 revision update、并发单胜者、损坏/schema drift 拒绝和重启恢复通过。该 store 不创建/迁移 schema，且尚未接入 product port；Rust 完整 definition normalization、Go/Rust durable corpus、取消与 backup/restore 仍未关闭，三条 route 保持 test-only，production owner 仍为 Go |

完成每个阶段时在本表追加最终决策；大量一次性测试日志留在 CI artifact/提交，不复制进长期文档。
