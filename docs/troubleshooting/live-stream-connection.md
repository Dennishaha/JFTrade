# 实时 SSE 连接问题

本文回答一个问题：为什么前端显示实时通道断开，以及应该先查 sidecar 还是 bbgo。

盘口深度现在也走同样的 SSE 思路：`/api/v1/market-data/depth/*` 支持 `Accept: text/event-stream`，并且在券商支持 order book push 时会优先消费事件流。

## 当前真实连接目标

前端默认连接的是：

- `http://127.0.0.1:3000/api/v1/stream/live`

这个路由在 [../../pkg/jftradeapi/market_routes.go](../../pkg/jftradeapi/market_routes.go) 中注册，由 [../../pkg/jftradeapi/market_live.go](../../pkg/jftradeapi/market_live.go) 驱动实时心跳、tick 和通知分发。

它不是 bbgo 原生 WebSocket。

盘口深度 SSE 则由 [../../pkg/jftradeapi/market_data.go](../../pkg/jftradeapi/market_data.go) 驱动，底层优先使用 Futu/OpenD 的 `Qot_UpdateOrderBook` 推送；如果推送不可用，会回退为 HTTP 查询 + 低频刷新，避免前端完全失去数据。

## 常见根因优先级

1. sidecar 根本没启动，或者已经跟随 `cmd/jftrade run` 一起退出
2. 前端配置错误地指向了 bbgo 或 `localhost`
3. sidecar 活着，但实时流握手失败，只有心跳没有行情
4. OpenD 不可用，sidecar 回退轮询也失败
5. depth SSE 已连上，但券商没有订阅成功或没有 order book push 权限

## 快速验证

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
go test ./pkg/jftradeapi -run TestLiveWebSocketSendsHeartbeat
go test ./pkg/jftradeapi -run TestMarketDepthSSEStreamSendsInitialPayload
```

如果第一个命令失败，优先处理进程和端口问题，不要先改前端样式或提示文案。

## 修复原则

- 前端默认 API/WS/SSE 地址统一用 `127.0.0.1:3000`
- 先确认 `/api/v1/stream/live` 这条 sidecar 路由存在，再考虑前端逻辑
- 如果是盘口深度页面，确认请求头里带了 `Accept: text/event-stream`；不带时会走一次性 JSON
- 不要把“把 error 改成灰色”当成修复，那只是隐藏症状

## 相关链路

实时行情不是单一推送链路。sidecar 会：

- 尝试维护 OpenD push 订阅
- 在样本不新鲜时回退到 `QueryTickers()` 补采样
- 统一把结果写回 `/api/v1/stream/live`

所以“WS 连上了但没行情”通常要继续查 OpenD 或订阅状态，而不是只停留在前端层。

盘口深度如果只看到首屏快照、不再更新，优先看这几件事：

- Futu/OpenD 是否支持并返回 `Qot_UpdateOrderBook`
- 当前 symbol 是否已经成功订阅 order book
- 前端是否真的通过 SSE 请求了 `/api/v1/market-data/depth/*`

## 需要避免的旧表述

- 不要写“前端连 bbgo WS”
- 不要建议把主机名改成 `localhost`
- 不要把 WS 问题简化成 UI 显示问题
