# Futu 对外接口索引

本文整理当前项目实际依赖，或者后续扩展时最容易碰到的 Futu OpenD / 行情 / 交易接口族，并给出在原始文档中的查找位置。

## 阅读前提

当前项目对 Futu 的使用方式是：

- [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go) 直接连接 OpenD 原生 TCP / protobuf API
- [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) 把其中一部分能力包装成 bbgo `Exchange`
- [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) 把 BasicQot push 翻译成 bbgo 标准流事件

这意味着：

- 当前项目不是通过 FTWebSocket / JavaScript API 接入 Futu
- 当前真正用到的接口集中在连接控制、BasicQot、K 线和系统通知

## 1. 连接与控制协议

| 协议 ID | 接口 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- | --- |
| `1001` | `InitConnect` | 建立 OpenD 原生连接，拿到连接信息与保活间隔 | [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 `1001` 协议表项、`InitConnect.proto` |
| `1002` | `GetGlobalState` | 查询全局市场状态与连接状态 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取全局市场状态” |
| `1003` | `Notify` | 接收 OpenD 运行事件、连接状态、行情权限与额度通知 | [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go)、[../../../pkg/jftradeapi/notifications.go](../../../pkg/jftradeapi/notifications.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 `1003` 协议表项、`Notify.proto` |
| `1004` | `KeepAlive` | 维持 OpenD TCP 会话 | [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 `1004` 协议表项、`KeepAlive.proto` |

## 2. 当前真正接入的行情接口

| 协议 ID | 接口 | 当前用途 | 本项目落点 | 原始文档位置 |
| --- | --- | --- | --- | --- |
| `3001` | `Qot_Sub` | 订阅 BasicQot 实时推送 | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “订阅反订阅” |
| `3004` | `Qot_GetBasicQot` | 拉取基础报价与快照字段 | [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时报价” |
| `3005` | `Qot_UpdateBasicQot` | 接收 BasicQot push，并转成 bbgo `BookTicker` / `Trade` | [../../../pkg/futu/stream.go](../../../pkg/futu/stream.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “实时报价回调” |
| `3006` | `Qot_GetKL` | 拉取当前窗口内 K 线与当前未收盘桶 | [../../../pkg/futu/exchange_kline.go](../../../pkg/futu/exchange_kline.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时 K 线” |
| `3103` | `Qot_RequestHistoryKL` | 拉取较长时间范围历史 K 线 | [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go) | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取历史 K 线” |

## 3. 当前项目会参考，但未直接接入的行情接口

这些接口在原始文档中存在，但当前项目没有直接把它们做成独立调用面：

| 接口族 | 当前状态 | 原始文档位置 |
| --- | --- | --- |
| 获取快照 | 作为背景参考；当前项目主要从 BasicQot 组合出自己的 snapshot 语义 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取快照” |
| 获取实时摆盘 | 未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时摆盘” |
| 获取实时分时 | 未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时分时” |
| 获取实时逐笔 | 未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时逐笔” |
| 获取实时经纪队列 | 未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时经纪队列” |

## 4. 交易接口族

当前项目在 [../../../pkg/futu/opend/client.go](../../../pkg/futu/opend/client.go) 中声明了多组交易协议 ID，但 [../../../pkg/futu/exchange.go](../../../pkg/futu/exchange.go) 的交易面仍以占位为主，大多数方法返回 `ErrNotSupported`。

因此这组接口目前更适合作为“扩展预备索引”，而不是“已实装能力”。

| 协议 ID | 接口 | 当前状态 | 原始文档位置 |
| --- | --- | --- | --- |
| `2001` | `Trd_GetAccList` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取交易业务账户列表 |
| `2005` | `Trd_UnlockTrade` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表、`解锁交易` |
| `2008` | `Trd_SubAccPush` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：订阅业务账户的交易推送数据 |
| `2101` | `Trd_GetFunds` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取账户资金 |
| `2102` | `Trd_GetPositionList` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取账户持仓 |
| `2201` | `Trd_GetOrderList` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取订单列表 |
| `2202` | `Trd_PlaceOrder` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表、`下单` |
| `2205` | `Trd_ModifyOrder` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：修改订单 |
| `2208` | `Trd_UpdateOrder` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：推送订单状态变动通知 |
| `2218` | `Trd_UpdateOrderFill` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：推送成交通知 |
| `2221` | `Trd_GetHistoryOrderList` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取历史订单列表 |
| `2223` | `Trd_GetHistoryOrderFillList` | 已声明，未接入 | [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表：获取历史成交列表 |

## 5. 当前最值得先看的原始文档入口

如果你是为了继续开发本项目，而不是通读整个 Futu 文档，优先看：

1. [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “行情接口总览”
2. [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “订阅反订阅”
3. [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时报价”
4. [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的 “获取实时 K 线” 与 “获取历史 K 线”
5. [../Futu-API-Doc-zh-Proto.md](../Futu-API-Doc-zh-Proto.md) 中的交易接口总览表

## 对后续 AI 的提醒

- 当前项目真正的实时行情主链路是 `3001 + 3005 + 3004`，不是整份文档里所有 quote 接口
- 当前项目真正的 K 线主链路是 `3006 + 3103`
- 当前项目的交易协议大多还处于“声明但未实装”状态，不要误判为已经具备完整下单能力
- 当前项目连接 OpenD 时用的是原生 TCP / protobuf，不是 FTWebSocket