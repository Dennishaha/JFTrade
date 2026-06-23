# 新券商接入指南

本文档描述如何为 JFTrade 添加一个新的券商适配器。

## 概述

JFTrade 使用 `pkg/broker/` 包定义的统一接口来抽象所有券商操作。接入新券商只需实现这些接口并注册到 Registry。

## 三步接入

### 步骤 1：创建券商包和适配器

创建 `pkg/<broker-id>/` 包，实现 `broker.Broker` 接口：

```go
// pkg/ib/adapter.go
package ib

import (
    "context"
    "github.com/jftrade/jftrade-main/pkg/broker"
)

type ibAdapter struct {
    client *IBClient
}

func NewBrokerAdapter(client *IBClient) broker.Broker {
    return &ibAdapter{client: client}
}

func (a *ibAdapter) ID() string { return "ib" }

func (a *ibAdapter) Descriptor() broker.Descriptor {
    return broker.Descriptor{
        ID:          "ib",
        DisplayName: "Interactive Brokers",
        Environments: []string{"PAPER", "LIVE"},
        Capabilities: []broker.MarketCapability{{
            Market:        "US",
            SupportsQuote: true,
            SupportsTrade: true,
            ReadFeatures: map[string]any{
                "funds":     map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
                "positions": map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
                "orders":    map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
            },
        }},
    }
}

func (a *ibAdapter) DiscoverAccounts(ctx context.Context) ([]broker.Account, error) {
    // 调用 IB API 发现账户
    accounts, err := a.client.GetManagedAccounts(ctx)
    if err != nil { return nil, err }
    result := make([]broker.Account, len(accounts))
    for i, acc := range accounts {
        result[i] = broker.Account{
            ID:                 acc.AccountID,
            BrokerID:           "ib",
            TradingEnvironment: acc.IsPaper ? "PAPER" : "LIVE",
            Market:             "US",
        }
    }
    return result, nil
}

func (a *ibAdapter) Trading() broker.TradingService {
    return &ibTradingService{client: a.client}
}

func (a *ibAdapter) MarketData() broker.MarketDataReader {
    return &ibMarketDataReader{client: a.client}
}
```

### 步骤 2：实现 MarketDataReader 和 TradingService

```go
// pkg/ib/market_data.go
type ibMarketDataReader struct {
    client *IBClient
}

func (r *ibMarketDataReader) QueryFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
    // 调用 IB API 获取资金
    funds, err := r.client.GetAccountSummary(ctx, query.AccountID)
    if err != nil { return nil, err }
    return &broker.FundsSnapshot{
        AccountID:     query.AccountID,
        TotalAssets:   broker.Float64Ptr(funds.NetLiquidation),
        Cash:          broker.Float64Ptr(funds.TotalCashValue),
    }, nil
}

// ... 实现其他 9 个 MarketDataReader 方法
```

```go
// pkg/ib/trading.go
type ibTradingService struct {
    client *IBClient
}

func (s *ibTradingService) PlaceOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
    orderId, err := s.client.PlaceOrder(ctx, query.AccountID, query.Symbol, query.Side, query.OrderType, query.Quantity, query.Price)
    if err != nil { return nil, err }
    return &broker.PlaceOrderResult{
        AccountID:     query.AccountID,
        BrokerOrderID: orderId,
        Status:        "SUBMITTED",
    }, nil
}

func (s *ibTradingService) CancelOrders(ctx context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
    for _, order := range orders {
        if err := s.client.CancelOrder(ctx, query.AccountID, order.BrokerOrderID); err != nil {
            return err
        }
    }
    return nil
}
```

### 步骤 3：注册到 sidecar 装配层

```go
// internal/app/apiserver/servercore/server.go
import "github.com/jftrade/jftrade-main/pkg/ib"

// 在 Server 初始化后注册
ibClient := ib.NewClient(ibConfig)
server.RegisterBroker(ib.NewBrokerAdapter(ibClient))
```

## 接口清单

### broker.Broker（必须实现）

| 方法 | 返回 | 说明 |
|------|------|------|
| `ID()` | `string` | 唯一券商标识 |
| `Descriptor()` | `Descriptor` | 能力描述 |
| `DiscoverAccounts(ctx)` | `[]Account, error` | 发现可用账户 |
| `Trading()` | `TradingService` | 交易服务（可返回 nil） |
| `MarketData()` | `MarketDataReader` | 行情服务（可返回 nil） |

### broker.MarketDataReader（可选，按需实现）

| 方法 | 说明 |
|------|------|
| `QueryFunds` | 资金查询 |
| `QueryPositions` | 持仓查询 |
| `QueryOrders` | 当日订单 |
| `QueryHistoryOrders` | 历史订单 |
| `QueryOrderFills` | 当日成交 |
| `QueryHistoryOrderFills` | 历史成交 |
| `QueryOrderFees` | 手续费 |
| `QueryMarginRatios` | 保证金比率 |
| `QueryCashFlows` | 资金流水 |
| `QueryMaxTradeQuantity` | 最大可交易数量 |

### broker.TradingService（可选，按需实现）

| 方法 | 说明 |
|------|------|
| `PlaceOrder` | 下单 |
| `CancelOrders` | 撤单 |

### broker.OrderPushSubscriber（可选，推送型券商）

| 方法 | 说明 |
|------|------|
| `SubscribeOrderUpdates` | 订阅订单推送 |
| `UnsubscribeOrderUpdates` | 取消订阅 |

### broker.BrokerConnector（可选，需连接管理）

| 方法 | 说明 |
|------|------|
| `Connect(ctx)` | 建立连接 |
| `Close()` | 关闭连接 |

## 类型映射

所有 `broker.*` 类型字段名与 JSON 序列化名一致，可直接作为 API 响应返回。对于券商特有的字段，可使用 `*float64`、`*string` 等指针字段，不支持的留 nil 即可。

## 参考：Futu 适配器

完整实现参考 `pkg/futu/adapter.go` 和 `pkg/futu/adapter_convert.go`。
