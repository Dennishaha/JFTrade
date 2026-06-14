# bbgo 对外接口索引

本文只整理当前项目实际依赖的 bbgo 公共接口与扩展点，不尝试穷举整个 bbgo 生态。

## 阅读前提

当前项目与 bbgo 的关系是：

- [../../../cmd/jftrade/main.go](../../../cmd/jftrade/main.go) 复用 bbgo CLI 入口
- [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) 通过注册机制接入 bbgo exchange factory
- [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) 复用 bbgo 的 Stream / StandardStream 抽象
- [../../../internal/app/apiserver/servercore/notifications.go](../../../internal/app/apiserver/servercore/notifications.go) 复用 bbgo 通知系统

## 1. 运行入口与注册机制

| 接口或扩展点 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- |
| `cmd.Execute()` | 复用 bbgo CLI 根命令 | [../../../cmd/jftrade/main.go](../../../cmd/jftrade/main.go) | [../bbgo-doc/commands/bbgo.md](../bbgo-doc/commands/bbgo.md)、[../bbgo-doc/commands/bbgo_run.md](../bbgo-doc/commands/bbgo_run.md) |
| `exchange.Register(name, factory)` | 把 `futu` 注册到 bbgo exchange factory | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) 的 “Exchange Factory” 与 “Implementation” |
| `exchange.Factory` | 提供 `EnvLoader` 与 `Constructor` | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | 文档不足；最近的说明在 [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) |

## 2. Exchange 接口族

这组接口是当前项目最核心的 bbgo 集成面。

| 接口或方法族 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- |
| `types.Exchange` | 交易所总接口 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) 的 “Checklist” |
| `QueryMarkets` | 提供市场列表，避免 bbgo bootstrap 因空 market 失败 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) |
| `QueryTicker` / `QueryTickers` | 提供快照报价 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) |
| `NewStream` | 提供实时流入口 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md)、[../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) |
| `QueryAccount` / `QueryAccountBalances` | 账户接口，占位实现 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | 文档不足；`adding-new-exchange` 只覆盖检查单级别说明 |
| `SubmitOrder` / `QueryOpenOrders` / `CancelOrders` | 交易接口，当前大多返回 `ErrNotSupported` | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) |

## 3. Stream 与事件回调

| 接口或扩展点 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- |
| `types.Stream` | 统一流抽象 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) |
| `types.StandardStream` | 复用连接生命周期、重连、订阅与回调中枢 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) |
| `Subscribe(...)` 与订阅切片 | 记录订阅，再由具体交易所流翻译成 broker 订阅命令 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) 的 “Subscriptions” |
| `OnConnect` / `OnDisconnect` / `Reconnect` | 连接生命周期回调 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) 的 “Callbacks” 与 “Reconnection model” |
| `EmitBookTickerUpdate` / `EmitMarketTrade` | 把 Futu push 事件翻译成 bbgo 标准事件 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md) 的 “Callbacks (event hub)” |

## 4. 通知系统

| 接口或扩展点 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- |
| `bbgo.Notification.AddNotifier(...)` | 注册 JFTrade live 通知桥 | [../../../internal/app/apiserver/servercore/notifications.go](../../../internal/app/apiserver/servercore/notifications.go) | 文档不足；最接近的使用侧资料是 [../bbgo-doc/configuration/slack.md](../bbgo-doc/configuration/slack.md) 与 [../bbgo-doc/configuration/telegram.md](../bbgo-doc/configuration/telegram.md) |
| `bbgo.Notify(...)` | 把通知写入 bbgo 全局通知总线 | [../../../internal/app/apiserver/servercore/notifications.go](../../../internal/app/apiserver/servercore/notifications.go) | 文档不足；通知后端示例见 [../bbgo-doc/configuration/slack.md](../bbgo-doc/configuration/slack.md) 与 [../bbgo-doc/configuration/telegram.md](../bbgo-doc/configuration/telegram.md) |
| `bbgo.Notifier` | JFTrade live bridge 实现的通知接收器接口 | [../../../internal/app/apiserver/servercore/notifications.go](../../../internal/app/apiserver/servercore/notifications.go) | 文档不足 |

## 5. 环境变量与会话发现

| 接口或扩展点 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- |
| 以 session 前缀解析环境变量 | 让 bbgo 能通过环境变量构造 exchange session | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../bbgo-doc/topics/developing-strategy.md](../bbgo-doc/topics/developing-strategy.md) 中的 session / env 说明、[../bbgo-doc/configuration/envvars.md](../bbgo-doc/configuration/envvars.md) |
| `EnvLoader` | 把外部环境变量转成 exchange options | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | 文档不足；最近的说明仍是 [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md) |

## 当前最重要的文档落点

如果你只想快速定位当前项目需要的 bbgo 能力，优先看：

1. [../bbgo-doc/development/adding-new-exchange.md](../bbgo-doc/development/adding-new-exchange.md)
2. [../bbgo-doc/topics/standard-stream.md](../bbgo-doc/topics/standard-stream.md)
3. [../bbgo-doc/commands/bbgo.md](../bbgo-doc/commands/bbgo.md)
4. [../bbgo-doc/topics/developing-strategy.md](../bbgo-doc/topics/developing-strategy.md)

## 对后续 AI 的提醒

- `adding-new-exchange` 更像检查单，不是完整 API 手册
- `StandardStream` 文档比 `Exchange Factory` 文档完整得多
- bbgo 通知系统在本地参考文档里覆盖不足，改这块时不要只依赖文档，先回到 [../../../internal/app/apiserver/servercore/notifications.go](../../../internal/app/apiserver/servercore/notifications.go)
- 文档里仍能看到 `SubmitOrders` 这类旧命名，和当前项目实现的 `SubmitOrder` 不必完全一一对应，语义上按“交易下单能力”理解
- 2026-05-22 起，证券 rich details / security snapshot 不再尝试塞进 bbgo 标准 `types.Exchange` 语义层；当前实现落在项目侧 Futu adapter + `internal/api/marketdata` 的 `/api/v1/market-data/securities/{market}/{symbol}`，bbgo 仍主要承载通用 quote / ticker / stream 能力
