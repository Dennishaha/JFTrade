# JFTrade 包结构重构方案

> 目标：支持多券商接入，通过统一抽象层管理；解决单包职责过重问题。

## 当前进度

| Phase | 状态 | 描述 |
|-------|------|------|
| Phase 1 | ✅ 完成 | `pkg/broker/` 统一券商抽象层 |
| Phase 4 | ✅ 完成 | Futu 实现 Broker 接口 + Server 集成 Registry |
| Phase 5 | ✅ 完成 | 路由层迁移到 broker 接口 |
| Phase 6 | ✅ 完成 | `execution_routes.go` + `strategy_runtime_manager.go` + `PlaceOrder` 迁移 |
| Phase 7 | ✅ 完成 | `pkg/futu/exchange_trade_read.go` 拆分为 5 个文件 |
| Phase 8 | ✅ 完成 | `pkg/jftradeapi/` 核心大文件拆分（strategy_catalog、execution） |

---

## 一、已完成工作详情

### Phase 1: `pkg/broker/` 统一券商抽象层

新增文件：

| 文件 | 行数 | 职责 |
|------|------|------|
| `broker.go` | ~75 | 核心接口：Broker、MarketDataReader、TradingService、OrderPushSubscriber、BrokerConnector |
| `types.go` | ~310 | 统一类型：ReadQuery、Account、20+ Snapshot/Query 类型（含 Futu 扩展字段） |
| `descriptor.go` | ~20 | 描述符：Descriptor、MarketCapability |
| `errors.go` | ~30 | 统一错误：BrokerError + 常见错误码 |
| `registry.go` | ~45 | 注册表：Registry、Register、Lookup、ActiveBroker、IDs、All |
| `helpers.go` | ~25 | 辅助函数：Float64Ptr、StringPtr、BoolPtr、Uint64Ptr |
| `broker_test.go` | ~110 | 7 个测试用例 |

### Phase 4: Futu Adapter + Server 集成

新增文件：

| 文件 | 行数 | 职责 |
|------|------|------|
| `pkg/futu/adapter.go` | ~240 | `futuAdapter` → `broker.Broker`、`futuMarketDataReader` → `broker.MarketDataReader`、`futuTradingService` → `broker.TradingService` |
| `pkg/futu/adapter_convert.go` | ~280 | Futu ↔ broker 双向类型转换器，编译时接口合规检查 |
| `pkg/futu/adapter_test.go` | ~100 | 3 个测试用例 |

Server 改造：

- `Server` 新增 `brokers *broker.Registry` 字段
- `futuExchange()` 创建 Exchange 后自动注册到 Registry
- 新增 `Server.activeBroker()` 方法
- 新增 `Server.brokerMarketDataReader()` 方法

### Phase 5: 路由层迁移（全部完成）

| 文件 | 变更 |
|------|------|
| `broker_routes.go` | `futu.BrokerReadQuery` → `broker.ReadQuery`；`s.futuExchange().QueryBrokerXxx` → `s.brokerMarketDataReader().QueryXxx` |
| `execution_routes.go` | 下单链路全面迁移：`normalizedExecutionPlaceOrder` 使用 `broker.PlaceOrderQuery` 替代 `futu.BrokerPlaceOrderQuery` + `types.SubmitOrder`；`placeExecutionOrder` 调用 `PlaceBrokerOrder(ctx, query)` 替代旧三参数签名；`BrokerOrderID`/`Status` 从 `broker.PlaceOrderResult` 获取；取消下单使用 `s.activeBroker().Trading().CancelOrders()` |
| `broker_order_updates_worker.go` | `futu.BrokerReadQuery` → `broker.ReadQuery`；`futu.RuntimeAccount` → `broker.Account`；账户发现改用 `s.activeBroker().DiscoverAccounts()`；订单同步改用 `reader.QueryOrders()`；保留必要的 Futu←→broker 转换桥接函数 |
| `execution_store.go` | `futu.BrokerOrderSnapshot` → `broker.OrderSnapshot`；`futu.BrokerOrderFillSnapshot` → `broker.OrderFillSnapshot` |
| `strategy_runtime_manager.go` | `strategyRuntimeExchange` 接口使用 `broker.ReadQuery`/`broker.PlaceOrderQuery`/`broker.PlaceOrderResult`；`strategyRuntimeBrokerBridge` 组合 `broker.Broker` 实现新接口；`strategyLiveOrderExecutor.SubmitOrders` 从 `bbgotypes.SubmitOrder` 构建 `broker.PlaceOrderQuery` |
| `strategy_runtime_manager_test_helpers_test.go` | Stub 的 `PlaceBrokerOrder`/`QueryBrokerFunds`/`QueryBrokerPositions` 签名迁移到 `broker.*` 类型 |
| `strategy_runtime_manager_polling_test.go` | `futu.BrokerPositionSnapshot` → `broker.PositionSnapshot` |
| `strategy_runtime_manager_trading_test.go` | `futu.BrokerPositionSnapshot` → `broker.PositionSnapshot` |

### Phase 6: PlaceOrder 链路最终迁移

| 文件 | 变更 |
|------|------|
| `futu/adapter.go` | `PlaceOrderResult.Status` 从 `string(result.Order.Status)`（=`"NEW"`）→ 优先使用 `result.Order.OriginalStatus`（=`"SUBMITTED"`），回退到 `string(result.Order.Status)` |
| 所有测试 | `strategyRuntimeStubExchange` 的 `PlaceBrokerOrder` 改为返回 `broker.PlaceOrderResult`；`SubmitOrders` 填充完整的 `PlaceOrderQuery`（含默认 `TimeInForce: DAY`）

### 迁移完成度统计

| 指标 | 完成情况 |
|------|---------|
| `futu.BrokerReadQuery` 引用 | ✅ 全部迁移（仅保留桥接转换函数） |
| `futu.BrokerFundsSnapshot` 引用 | ✅ 零残留 |
| `futu.BrokerPositionSnapshot` 引用 | ✅ 零残留 |
| `futu.BrokerPlaceOrderQuery` 引用 | ✅ 零残留 |
| `futu.BrokerPlaceOrderResult` 引用 | ✅ 零残留 |
| 其他 `futu.Broker*` 类型 | ✅ 全部迁移完毕 |
| `pkg/jftradeapi/` 中的 `futu` 导入 | 9 个文件（均为行情/序列化/Exchange 实例化等非 Broker 类型场景） |

---

## 二、新券商接入指南

接入一个新券商（例如 Interactive Brokers）只需 3 步：

### 步骤 1：实现 `broker.Broker` 接口

```go
// pkg/ib/adapter.go
package ib

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
                "funds":    map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
                "positions": map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
                "orders":   map[string]any{"supportedEnvironments": []string{"PAPER", "LIVE"}},
            },
        }},
    }
}

func (a *ibAdapter) DiscoverAccounts(ctx context.Context) ([]broker.Account, error) {
    // 调用 IB API 发现账户
}

func (a *ibAdapter) MarketData() broker.MarketDataReader {
    return &ibMarketDataReader{client: a.client}
}

func (a *ibAdapter) Trading() broker.TradingService {
    return &ibTradingService{client: a.client}
}
```

### 步骤 2：在 `cmd/jftrade/main.go` 注册

```go
import (
    "github.com/jftrade/jftrade-main/pkg/ib"
)

func init() {
    // 在 Server 初始化后注册
    server.RegisterBroker(ib.NewBrokerAdapter(ibClient))
}
```

### 步骤 3：前端路由自动生效

`serveBrokerRoutes` 已通过 `activeBroker().ID()` 动态查找券商，前端访问 `/api/v1/brokers/ib/funds` 即可自动路由到 IB 适配器。

---

## 三、待执行工作

### Phase 7: `exchange_trade_read.go` 文件拆分（已完成）

为避免子包循环依赖，采用**同包内文件拆分**策略，将 1334 行的 `exchange_trade_read.go` 拆分为 5 个职责清晰的文件：

| 新文件 | 行数 | 职责 |
|--------|------|------|
| `exchange_trade_read.go` (保留) | 476 | 12个 `QueryBroker*` 方法 + 3个 bbgo 适配方法 |
| `exchange_trade_read_account.go` | 168 | 账户解析：`resolveTradeAccountWithClient`、`candidateTradeAccountFromProto`、`resolveTradeMarket`、`trdMarketFromNormalized`、优先级/过滤 |
| `exchange_trade_read_proto.go` | 238 | Proto→Snapshot 转换器：`*FromProto` 系列函数 |
| `exchange_trade_read_convert.go` | 240 | bbgo 类型桥接：`bbgoOrderFromBrokerOrder`、Side/Type/TIF/Status 映射、`balanceMap*`、时间解析、market/currency 推断 |
| `exchange_trade_read_helpers.go` | 190 | 工具函数：filter 构建、枚举映射、`optional*` 系列、`fixedpointFrom*`、`parseUint64` |

### Phase 8: `pkg/jftradeapi/` 核心大文件拆分（已完成）

为避免子包循环依赖，采用同包内文件拆分策略：

| 操作 | 变更 |
|------|------|
| `strategy_catalog_store.go` (1077→705行) | 提取类型定义到 `strategy_catalog_types.go`（251行），提取活动/日志到 `strategy_catalog_activity.go`（131行） |
| `execution_store.go` (719→485行) | 提取类型定义到 `execution_types.go`（103行），提取工具函数到 `execution_helpers.go`（142行） |

---

## 四、风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 大规模 import 变更 | 渐进式推进，每步独立编译验证 |
| futu 内部类型与 broker 类型双重维护 | Futu 内部类型保留，adapter 层做映射 |
| 测试覆盖不足 | 每个 Phase 都编译+测试验证 |
| broker 接口过度抽象 | 先只抽象 Futu 已有能力 |
