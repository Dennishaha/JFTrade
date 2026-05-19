# 架构与设计

## 非侵入式接入 bbgo

bbgo 暴露的关键扩展点：

1. `pkg/exchange.Register(name types.ExchangeName, factory Factory)` —
   动态把任意 `types.Exchange` 实现注册到全局工厂表，同时把名字加入
   `types.SupportedExchanges`，让 `pkg/bbgo/environment.go` 在 `Init`
   阶段通过 `exchange.NewWithEnvVarPrefix` 实例化。
2. `pkg/types.Exchange` 接口（必须实现）：
   * `Name() ExchangeName`
   * `PlatformFeeCurrency() string`
   * `ExchangeMarketDataService`: `NewStream`, `QueryMarkets`,
     `QueryTicker`, `QueryTickers`, `QueryKLines`
   * `ExchangeAccountService`: `QueryAccount`, `QueryAccountBalances`
   * `ExchangeTradeService`: `SubmitOrder`, `QueryOpenOrders`, `CancelOrders`
3. bbgo CLI 入口 `cmd.Execute()` 是 cobra 根命令。jftrade 直接复用，仅追加
   `init()` 注册自身扩展。

我们的 `pkg/futu` 包：

* `init()` 中调用 `exchange.Register("futu", Factory{EnvLoader, Constructor})`。
* Constructor 读 `FUTU_OPEND_ADDR` / `FUTU_OPEND_RSA_KEY` 等 env，构造
  `*Exchange{client: opend.New(...)}` 返回。
* Stream 实现 bbgo `types.Stream` 接口（采用 bbgo 已提供的
  `types.StandardStream` 内嵌 + 自己 hook push）。

由于 Futu 是港美股 / 期权 / 期货市场，与 bbgo 的"现货"模型存在概念差异：

* `Symbol` 映射：`HK.00700`、`US.AAPL`，按市场前缀编码。
* `Currency` 映射：HKD/USD/CNY 单独建账户余额。
* `KLine.Interval` 映射 OpenD `KLType_*`，仅暴露 bbgo 已定义的常量。
* 不支持的概念（withdraw、deposit、margin call）返回 `ErrNotSupported`。

## OpenD 协议

参考 Futu OpenAPI 文档：每个请求 / 推送都是

```
| 'FT' (2) | protoID (4 LE) | protoFmt (1) | protoVer (1) | serialNo (4 LE) |
| bodyLen (4 LE) | bodySHA1 (20) | reserved (8) | body (bodyLen) |
```

总计 44 字节头 + 变长 body。`pkg/futu/codec` 提供 `EncodeFrame` /
`DecodeFrame`，纯函数 + 单元测试。`pkg/futu/opend` 在其上封装：

* WebSocket 连接（gorilla/websocket）。
* `InitConnect`（protoID 1001） / `KeepAlive`（1004，定时心跳）。
* 请求 / 响应通过 `serialNo` 关联，map[serialNo]chan reply。
* push 类（如 `Trd_UpdateOrder`）通过 `subscribe(protoID)` 派发到订阅者。

## 测试策略

* `pkg/futu/codec`：纯帧编解码单测。
* `pkg/futu/opend`：用 in-memory `net.Pipe` + 假 server 验证 RPC。
* `pkg/futu`：mapping 函数（KLine、Order、Side、Status）单测。
* 集成：`go test -tags=integration ./pkg/futu/...` 期望本地 OpenD 11111 端口。
* 前端：vue-tsc + vite build 烟囱测试。
