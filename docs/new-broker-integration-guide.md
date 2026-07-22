# 新券商接入指南

JFTrade 当前是 Futu-first 产品，但 broker 边界已经支持按能力选择 adapter。新增券商不是“实现一个接口就自动出现”：必须同时完成 adapter、capability 声明、应用装配、运行时选择、API/UI surface 和 conformance 测试。

## 先理解当前边界

- `pkg/broker/broker.go`：顶层 `Broker`、完整读写服务和基础可选接口。
- `pkg/broker/advanced.go`：微观行情、衍生品、研究、预测市场、规则和组合交易等可选接口。
- `pkg/broker/features.go`：feature ID、capability 状态与 runtime evaluation 数据结构。
- `pkg/broker/catalog.go`：能力到 adapter interface、API、UI、tool、权限和测试的机器可检查映射。
- `pkg/broker/capability_support.go`：声明的 feature 如何验证实际接口实现。
- `pkg/broker/router.go`：显式 broker、默认 broker 和 fallback 的选择规则。
- `internal/app/apiserver/servercore`：当前 registry 创建、adapter 生命周期和 service 装配位置。
- `pkg/futu/adapter*.go`：当前完整实现参考。

## 接入步骤

### 1. 实现顶层 adapter

新 adapter 必须实现 `broker.Broker`：

```go
type Broker interface {
    ID() string
    Descriptor() Descriptor
    DiscoverAccounts(context.Context) ([]Account, error)
    Trading() TradingService
    MarketData() MarketDataReader
}
```

约束：

- `ID` 必须稳定、唯一，并与设置、账户和 capability 中的 `brokerId` 一致。
- 不支持完整 read 或 trade service 时返回 `nil`；不要提供会在常规调用中 panic 的空壳。
- `DiscoverAccounts` 返回 canonical `broker.Account`，不要把券商 SDK 类型泄漏到业务层。
- SDK/protobuf 到 `broker.*` 的转换留在 adapter 包内。

### 2. 只实现真实支持的能力接口

`MarketDataReader` 和 `TradingService` 是完整服务接口；一旦返回非 nil，就必须实现其全部方法。更细的产品能力使用 `advanced.go` 中的可选接口组合，例如：

- `BatchSnapshotSource`
- `MarketMicrostructureReader`
- `InstrumentProfileReader`
- `DerivativeCatalogReader`
- `OptionAnalyticsReader`
- `InstrumentResearchReader`
- `MarketResearchReader`
- `TechnicalIndicatorReader`
- `ProductRuleProvider`
- `ComboTradingService`
- `CustomizationService`

连接、推送、自选导入等能力继续使用 `BrokerConnector`、`OrderPushSubscriber`、`QuoteSubscriber`、`WatchlistGroupReader` 等窄接口。

不要仅为了让 capability 检查通过而实现伪接口；尚未支持的 feature 应明确不声明，或声明为 `unavailable/degraded` 并给出稳定原因。

### 3. 声明静态与运行时 capability

`Descriptor()` 至少要提供稳定 ID、显示名、环境、市场和 `FeatureCapability`。每个 feature 必须：

- 存在于 `broker.BuiltinCapabilityCatalog`；
- 对应 adapter 实际实现的接口；
- 使用正确的 `read`、`write` 或 `trade` access；
- 说明是否要求连接、账户和行情权限；
- 与实际支持的 market、product class、market segment 和 period 一致。

如果可用性取决于连接、账户、权限或限流，实现 `broker.CapabilityEvaluator`。静态“代码支持”与运行时“当前可用”必须分开，不能用一次连接失败永久否定 capability。

### 4. 接入应用装配与选择

当前 registry 在 `internal/app/apiserver/servercore` 创建，Futu adapter 由 OpenD integration 生命周期按需 `Replace`。新增券商需要显式完成：

1. 配置和 secret 存储；
2. client/connection 生命周期；
3. `broker.Registry` 注册或替换；
4. default broker 与 fallback 顺序；
5. system/settings descriptor 与健康状态；
6. 关闭、重连和数据库重建时的资源释放。

当前没有自动插件发现，也没有可从任意包调用的 `Server.RegisterBroker` 公共方法。不要照搬旧文档在 `init()` 中做全局注册；装配必须显式、可测试，并受进程生命周期管理。

### 5. 接入产品 surface

能力目录把 adapter 能力与 API、UI、ADK/MCP tool 和权限绑定。新增 feature 或 surface 时同步处理：

- `pkg/broker/catalog.go`
- 对应 `internal/api/*` route 与 service
- `apps/web` 的选择、展示和不可用状态
- OpenAPI/前端生成类型
- ADK/MCP 权限；交易能力必须维持 critical approval 与 pre-trade risk 边界

只接入已有 feature 时，不应复制一套券商专属业务路由；让请求通过 broker feature router 选择 adapter。

## Conformance 验收

每个新 adapter 至少覆盖：

- descriptor 与实际接口一致；
- 显式选择、默认选择、fallback、未知 broker 和 unsupported capability；
- 账户发现和 canonical 类型映射；
- query、place、cancel、push update、partial fill 与 out-of-order update；
- 断连、重连、权限不足、行情权不足和 rate limit；
- REAL 下单经过共享 pre-trade risk gateway，gateway 不可用时 fail closed；
- `Close` 后 goroutine、socket 与订阅可释放；
- 不依赖真实券商的 fake adapter conformance suite。

最低回归入口：

```bash
go test ./pkg/broker ./internal/trading ./internal/marketdata ./internal/app/apiserver/servercore -count=1
pnpm --filter @jftrade/web run test
pnpm --filter @jftrade/web run typecheck
bash scripts/check-arch-deps.sh
```

## 自选导入与快照

自选领域使用稳定 `sourceId` 表示券商登录连接，不直接复用交易 `accountId`。远端分组导入与密集快照分别实现 `WatchlistGroupReader`、`FreshWatchlistGroupReader` 和 `BatchSnapshotSource`；完整数据主权、幂等和刷新约束见 [自选系统](watchlist.md#增加新的券商-source)。

## 非目标

- 不要求所有券商支持 Futu 的全部协议面。
- 不让券商 SDK 类型进入 `internal/api`、`internal/trading` 或前端契约。
- 不因新增一个 adapter 改变已有 `/api/v1/*` envelope。
- 不绕过 canonical order status、risk gateway、approval 或 capability evaluation。
