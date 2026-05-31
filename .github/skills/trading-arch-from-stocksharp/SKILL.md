---
name: trading-arch-from-stocksharp
description: 'JFTrade 架构与模块拆分参考，从成熟量化框架 StockSharp 蒸馏而来。Use when: 新增交易所/券商适配器、拆分臃肿模块、设计回测或撮合、扩展策略引擎、做 Logic Flow 可视化图块到 DSL/IR 的映射、判断某段逻辑该放在 futu 适配层还是 sidecar/runtime、评审分层与解耦是否跑偏。用于在动手前对齐分层边界，避免把协议、业务、UI 职责混在一起。'
argument-hint: '说明要做的架构决策或要拆分的模块（如“新增 IB 适配器”“回测撮合粒度”“图块映射”）'
---

# 从 StockSharp 蒸馏的交易系统架构准则

把成熟开源量化框架 [StockSharp](https://github.com/StockSharp/StockSharp) 的分层与解耦经验，映射到 JFTrade 当前结构（`pkg/futu` 适配层 + `pkg/jftradeapi` sidecar + bbgo runtime + `apps/web` 前端 DSL/Logic Flow + `pkg/futu/backtest` 回测）。

动手前先读 [docs/architecture.md](../../../docs/architecture.md) 确认当前边界，再用本文校准方向。

## 何时使用

- 新增交易所/券商适配器，或扩展 `pkg/futu` 能力。
- 某个文件/模块臃肿、职责混杂，需要按职责拆分。
- 设计或改动回测、撮合、历史数据回放。
- 扩展策略引擎、指标、或 Logic Flow 图块 → DSL → Go IR 的链路。
- 评审一段逻辑该落在哪一层（协议翻译 / 控制平面 / 运行时 / UI）。

## 五条核心准则（先记这些）

1. **核心层零 UI、零资源、零本地化依赖。** StockSharp 的 `Messages`→`BusinessEntities`→`Algo` 严格不依赖 UI/Media/Localization；这些横切关注点各自独立成包。对应 JFTrade：`pkg/futu`、回测、策略 runtime 不得 import 任何前端/展示逻辑；中文标签转换只允许待在 `apps/web/src/composables/consoleDataFormatting.ts`。
2. **适配器只做协议翻译，业务逻辑由统一外壳层托管。** StockSharp 的 `XxxMessageAdapter` 只把消息翻译成交易所 API；风控/滑点/订阅生命周期/重连恢复/多券商聚合由上层 `Connector` + `BasketMessageAdapter` 统一托管——**外壳层只有一份，适配器有 N 份**。对应 JFTrade：`pkg/futu` 只负责 OpenD 协议映射；订阅调度、实时/HTTP 混合采样、通知收束等控制逻辑属于 `pkg/jftradeapi` sidecar。**JFTrade 将走向多适配器，必须从现在起把"外壳层"与"适配器"显式分开**，详见 [统一外壳层模式](./references/adapter-shell-pattern.md)。
3. **能力显式声明，不支持就显式暴露，绝不伪实现。** StockSharp 用 `AddMarketDataSupport()` / `RemoveSupportedMessage()` / `NotSupportedResultMessages` 声明能力。对应 JFTrade 已有原则：不支持的交易所能力返回 `ErrNotSupported`，而非假装成功。新增能力时同步更新声明，别让调用方靠试错发现。
4. **回测与实盘共享同一套接口，回测专属件清晰隔离。** StockSharp 让 `HistoryMessageAdapter`/`EmulationMessageAdapter` 实现与实盘相同的 `MessageAdapter` 契约，策略代码零改动即可在两种模式跑。对应 JFTrade：回测走 bbgo `backtest.Exchange`，与实盘 `futu.Exchange` 复用同一策略 runtime；回测专属（SQLite 存储、同步、撮合假设）集中在 `pkg/futu/backtest`，不要渗进实盘路径。
5. **可视化即策略，不是平行实现。** StockSharp 的 `DiagramStrategy : Strategy` —— 图块组合直接 *是* 一个策略，复用回测/优化/运行时。对应 JFTrade：Logic Flow 可视模型必须收敛成 DSL，再统一编译成 Go IR 给同一个 runtime；不要为可视化另起一套执行引擎。

详细映射见下。

## StockSharp 分层模型（自底向上）

| StockSharp 层 | 职责 | JFTrade 对应 |
| --- | --- | --- |
| `Messages/` | 唯一通信契约，消息即"不可变数据 + 意图" | bbgo message/类型 + `pkg/jftradeapi` 的 DTO/路由契约 |
| `BusinessEntities/` | 领域对象 + 接口（`IConnector` 等），消息的便利封装 | bbgo types + `pkg/futu` 暴露的交易所抽象 |
| `Algo/` | 策略框架、连接管理、风控、回测编排 | bbgo runtime + `pkg/futu/backtest` 编排 |
| `Connectors/Xxx` | 每个交易所一个目录，只做协议翻译 | `pkg/futu`（当前单适配器） |
| `Algo.Testing` + `MatchingEngine` | 历史回放 + 撮合模拟，与实盘同接口 | `pkg/futu/backtest`（store/sync/runner）+ bbgo 撮合 |
| `Diagram.Core` / `Designer.Templates` | 可视化图块、组合、模板，编译成策略 | `apps/web` Logic Flow + 策略模板，生成 DSL |
| `Charting.Interfaces` / `Configuration` / `Localization` / `Media` | 横切关注点，独立成包 | 前端图表/格式化、`config/`、i18n |

关键：**依赖只能自底向上，横切关注点旁挂、不下沉到核心。**

## 决策流程

### 新增/扩展适配器能力时（多适配器是既定方向，优先看这里）

1. **先确认"外壳层"是否独立存在。** 任何在多个适配器间通用的逻辑（订阅注册表、重连恢复、心跳、能力路由、错误归一化）都属于外壳层，只能写一份，绝不在每个适配器里复制。对照 StockSharp `BasketMessageAdapter`/`Connector`，详见 [统一外壳层模式](./references/adapter-shell-pattern.md)。
2. 判断这段逻辑是"协议翻译"还是"业务调度"。翻译留在适配器（`pkg/futu`），调度上移到外壳/sidecar（准则 2）。
3. 新能力先在适配层显式声明可用/不可用；不支持返回 `ErrNotSupported`（准则 3）。外壳层据此做能力路由，调用方不必关心背后是哪个适配器。
4. 新增第二个适配器前，先把 `pkg/futu` 中**可复用的调度逻辑抽到外壳层**，让 Futu 与新适配器并列实现同一套契约——避免把第二个适配器写成 Futu 的拷贝。
5. 参考 StockSharp 适配器目录的拆分粒度：`_MarketData` / `_Transaction` / `_Settings` 分文件，避免单文件膨胀。JFTrade `pkg/futu` 已有 `exchange_kline.go` / `exchange_trade_*.go` 等同构拆分，新增时延续。

### 拆分臃肿模块时

1. 先识别混在一起的职责类别：协议翻译 / 控制平面 / 运行时 / 展示 / 横切。
2. 按 StockSharp 的"接口层 vs 实现层"分离：稳定契约抽成接口，易变实现分文件。
3. 横切能力（格式化、配置、i18n、图表）抽成独立单元，禁止反向依赖核心。
4. 保持语义不变，只移动边界——这是工程优化而非重写。

### 设计/改动回测时

1. 回测入口必须复用实盘策略接口，验证"同一策略零改动可切换模式"（准则 4）。
2. 撮合粒度要写明取舍：StockSharp 的 `MatchingEngine` 支持 order-book 级（限价/市价/post-only/滑点）；JFTrade 当前是 bar-close 撮合。新增更细撮合前，先确认数据粒度是否支撑，并警惕未来函数 / bar 内成交假设。
3. 历史数据供给与撮合解耦：StockSharp 用 `StorageRegistry` + 回放时钟统一推进；JFTrade 用 SQLite `local_klines` + bbgo 回放。改一侧别耦死另一侧。

### 做可视化图块 → DSL → IR 时

1. 图块（节点）+ 端口（socket，带类型）+ 连线 = 数据流；参数独立持久化，支持可视模型与代码双向同步（对照 `DiagramElement`/`DiagramSocket`/`DiagramElementParam`）。
2. 可视模型不直接执行，必须生成 DSL，由后端统一解析/规划/编译成 Go IR（准则 5），与 [docs/architecture.md](../../../docs/architecture.md) 中"图块不变、显式同步才重新生成 DSL"的约束一致。
3. 指标作为独立可复用单元（StockSharp `IIndicator` 统一 `Process(input)->output` 管线），与图块、策略解耦，三者不互相内嵌实现。

## 反模式（避免踩坑清单）

- 把订阅调度、重连、风控写进 `pkg/futu` 适配层 → 违反准则 2，应上移到统一外壳层。
- 新增第二个适配器时直接拷贝 Futu 适配器 → 通用逻辑应先抽到外壳层，两个适配器并列实现同一契约。
- 不支持的能力返回伪造的成功响应 → 违反准则 3，应 `ErrNotSupported`。
- 为 Logic Flow 另写一套独立执行引擎 → 违反准则 5，应收敛到 DSL/IR。
- 回测专属假设泄漏进实盘路径，或实盘细节硬编码进回测 → 违反准则 4。
- 在核心/runtime 层做中文标签或展示格式化 → 违反准则 1，应留在前端 `consoleDataFormatting.ts`。
- 单文件承载多类职责持续膨胀 → 按 StockSharp 的分文件/接口分离粒度拆分。

## 收尾

完成架构决策或拆分后：

1. 复核是否仍满足"依赖自底向上、横切旁挂"。
2. 如改动了边界，更新 [docs/architecture.md](../../../docs/architecture.md) 对应章节，保持文档与实现一致。
3. 需要时在 `/memories/repo/` 记录新确立的约定。
