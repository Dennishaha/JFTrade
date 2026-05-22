# WebSocket 连接问题

本文回答一个问题：为什么前端显示 `WS disconnected`，以及应该先查 sidecar 还是 bbgo。

## 当前真实连接目标

前端默认连接的是：

- `ws://127.0.0.1:3000/api/v1/ws/live`

这个路由在 [../../pkg/jftradeapi/market_routes.go](../../pkg/jftradeapi/market_routes.go) 中注册，由 [../../pkg/jftradeapi/market_live.go](../../pkg/jftradeapi/market_live.go) 驱动实时心跳、tick 和通知分发。

它不是 bbgo 原生 WebSocket。

## 常见根因优先级

1. sidecar 根本没启动，或者已经跟随 `cmd/jftrade run` 一起退出
2. 前端配置错误地指向了 bbgo 或 `localhost`
3. sidecar 活着，但实时流握手失败，只有心跳没有行情
4. OpenD 不可用，sidecar 回退轮询也失败

## 快速验证

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
go test ./pkg/jftradeapi -run TestLiveWebSocketSendsHeartbeat
```

如果第一个命令失败，优先处理进程和端口问题，不要先改前端样式或提示文案。

## 修复原则

- 前端默认 API/WS/SSE 地址统一用 `127.0.0.1:3000`
- 先确认 `/api/v1/ws/live` 这条 sidecar 路由存在，再考虑前端逻辑
- 不要把“把 error 改成灰色”当成修复，那只是隐藏症状

## 相关链路

实时行情不是单一推送链路。sidecar 会：

- 尝试维护 OpenD push 订阅
- 在样本不新鲜时回退到 `QueryTickers()` 补采样
- 统一把结果写回 `/api/v1/ws/live`

所以“WS 连上了但没行情”通常要继续查 OpenD 或订阅状态，而不是只停留在前端层。

## 需要避免的旧表述

- 不要写“前端连 bbgo WS”
- 不要建议把主机名改成 `localhost`
- 不要把 WS 问题简化成 UI 显示问题