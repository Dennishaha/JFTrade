# 统一外壳层模式：StockSharp 源码对照

把 StockSharp 的"适配器外壳层"（adapter shell）拆解清楚，作为 JFTrade 走向多适配器时的源码级参考。所有路径相对 `~/Desktop/StockSharp`。

## 一句话

**适配器有 N 份，外壳层只有 1 份。** 适配器只翻译协议；外壳层做聚合、路由、重连、订阅恢复、能力裁决——这套逻辑写一次，所有适配器复用。

## 三层结构

```
策略 / runtime
      │  只面向统一契约，不知道背后是哪个交易所
      ▼
┌─────────────────────────────────────────────┐
│ 外壳层（只有一份）                            │
│  Algo/Connector.cs            订阅/状态/重连恢复 │
│  Algo/BasketMessageAdapter.cs 多适配器聚合+路由 │
└─────────────────────────────────────────────┘
      │  统一契约 IMessageAdapter（消息进/消息出）
      ▼
┌──────────────┬──────────────┬───────────────┐
│ Futu 适配器   │ IB 适配器     │ Binance 适配器 │  ← N 份，只翻译协议
│ (pkg/futu)   │              │               │
└──────────────┴──────────────┴───────────────┘
```

## 关键源码与职责

### 统一契约：`Messages/IMessageAdapter.cs` + `Messages/MessageAdapter.cs`

每个适配器实现同一接口，新适配器只需关注协议细节：

- `ConnectAsync` / `DisconnectAsync` / `ResetAsync` / `TimeAsync` —— 生命周期钩子
- `SendInMessage`（收命令消息）/ `OnNewOutMessage`（吐结果消息）—— 消息进出双通道
- `PossibleSupportedMessages` / `SupportedInMessages` / `NotSupportedResultMessages` —— 能力声明
- 基类 `MessageAdapter` 已实现大量通用样板，子类只补协议特化部分

> **JFTrade 落点**：把 `pkg/futu.Exchange` 当前直接暴露的方法，逐步收敛成一组稳定契约（订阅、下单、查询、连接生命周期），让未来的第二个适配器实现同一组契约，而不是各搞一套方法签名。

### 能力声明（构造期显式注册）

适配器在构造函数里声明能力，而非靠调用方试错：

- `this.AddMarketDataSupport()` / `this.AddTransactionalSupport()`
- `this.AddSupportedMarketDataType(...)` / `this.AddSupportedCandleTimeFrames(...)`
- `this.RemoveSupportedMessage(...)` —— 不支持就摘掉，外壳层据此路由

> **JFTrade 落点**：延续 `ErrNotSupported` 原则，但更进一步——让外壳层能查询每个适配器的能力集，做"能力路由 + 友好降级"，而不是把 `ErrNotSupported` 冒泡到前端。

### 多适配器聚合：`Algo/BasketMessageAdapter.cs`

一个进程接多个交易所时的核心。它负责：

- 持有多个 `IMessageAdapter`，按 portfolio/security/能力把消息**路由**到正确适配器
- 把多个适配器的 out 消息**汇流**成一条统一事件流
- 统一处理跨适配器的错误归一化、消息去重、顺序保证

> **JFTrade 落点**：这是 JFTrade 现在**还没有、但多适配器后必须有**的一层。今天可以先建一个最小外壳（即使只挂 Futu 一个适配器），把"路由 + 汇流"接口预留出来，避免将来推倒重来。

### 连接与订阅托管：`Algo/Connector.cs`

外壳层里更靠上的协调者，持有 `_subscriptionManager`：

- 订阅的注册、去重、生命周期管理
- 断线重连后的**订阅恢复**（适配器只管重连本身，恢复哪些订阅由外壳层记账）
- 连接状态机统一对外

> **JFTrade 落点**：当前 `pkg/jftradeapi` 的订阅调度 + 实时/HTTP 混合采样，本质就是外壳层职责。多适配器后，把"订阅记账与恢复"从适配器彻底剥离到这一层。

## 适配器内部拆分粒度（避免单文件膨胀）

StockSharp 每个适配器目录同构拆分，参考 `Connectors/BitStamp/`：

| 文件 | 职责 |
| --- | --- |
| `XxxMessageAdapter.cs` | 入口 + 连接生命周期 |
| `XxxMessageAdapter_MarketData.cs` | 行情订阅翻译 |
| `XxxMessageAdapter_Transaction.cs` | 下单/撤单/查询翻译 |
| `XxxMessageAdapter_Settings.cs` | 凭据与配置 |
| `Native/` | 底层协议/SDK 封装 |

JFTrade `pkg/futu` 已有同构习惯（`exchange_kline.go` / `exchange_trade_read.go` / `exchange_trade_write.go` / `exchange_accounts.go`），新增适配器时延续这套分文件约定。

## 落地顺序建议（给 JFTrade）

1. **抽契约**：从 `pkg/futu` 现有方法归纳出稳定的适配器接口（订阅/交易/查询/生命周期/能力声明）。
2. **建外壳**：新增一个最小外壳层（路由 + 汇流 + 订阅记账），即使当前只挂 Futu。
3. **迁调度**：把分散在 sidecar/适配器里的订阅恢复、重连、能力裁决归并到外壳层。
4. **加适配器**：第二个适配器并列实现同一契约，验证"零改动复用外壳"。

每完成一步，回写 [docs/architecture.md](../../../../docs/architecture.md) 与 `/memories/repo/`。
