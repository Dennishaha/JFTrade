# 故障排查入口

本文只做分诊，不再把所有细节堆在一页里。

先记住两个前提：

- 前端默认依赖的是 JFTrade sidecar 的 `/api/v1/*`，不是 bbgo 原生 `/api/*`
- 启动方式分为 API-only 和 bbgo run，两者看到的故障现象可能完全不同

## 先看症状，再进专题

| 症状 | 去看哪里 | 快速验证 |
| --- | --- | --- |
| 后端起不来、端口不对、启动后马上退出 | [troubleshooting/startup-ports.md](troubleshooting/startup-ports.md) | 开发态查 `http://127.0.0.1:3000/api/v1/system/status`，发布态查 `http://127.0.0.1:6699/api/v1/system/status` |
| 前端显示 WS disconnected 或没有心跳 | [troubleshooting/websocket-connection.md](troubleshooting/websocket-connection.md) | `go test ./pkg/jftradeapi -run TestLiveWebSocketSendsHeartbeat` |
| OpenD 连不上、设置保存了但运行时没生效 | [troubleshooting/opend-configuration.md](troubleshooting/opend-configuration.md) | 开发态查 `http://127.0.0.1:3000/api/v1/system/futu-opend`，发布态查 `http://127.0.0.1:6699/api/v1/system/futu-opend` |
| 美股盘前盘后或夜盘显示异常 | [troubleshooting/us-extended-hours.md](troubleshooting/us-extended-hours.md) | 检查 snapshot 是否带 `lastClosePrice` 和扩展行情块 |
| 不确定到底该先查哪一层 | [troubleshooting/diagnostic-principles.md](troubleshooting/diagnostic-principles.md) | 先按职责边界和决策树定位 |

## 常用诊断命令

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
curl -fsS http://127.0.0.1:6699/api/v1/system/status
curl -fsS http://127.0.0.1:3000/api/v1/settings/brokers
curl -fsS http://127.0.0.1:6699/api/v1/settings/brokers
curl -fsS http://127.0.0.1:3000/api/v1/system/futu-opend
curl -fsS http://127.0.0.1:6699/api/v1/system/futu-opend

lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:6699 -sTCP:LISTEN
lsof -nP -iTCP:6688 -sTCP:LISTEN
lsof -nP -iTCP:11110 -sTCP:LISTEN
lsof -nP -iTCP:11111 -sTCP:LISTEN

go test ./...
```

## 术语统一

- API-only：`go run ./cmd/jftrade api`，只启动 sidecar
- bbgo run：`go run ./cmd/jftrade run --config ./config/jftrade.yaml`，启动 bbgo 运行时，并尝试附带 sidecar
- 发布态 GUI：默认 `127.0.0.1:6688`，内嵌前端静态入口
- 发布态 gateway：默认 `127.0.0.1:6699`，发布二进制对外提供的 `/api/v1/*` 与 Swagger 入口
- OpenD API port：默认 `127.0.0.1:11110`，Go 原生 TCP API 使用
- OpenD WebSocket port：默认 `127.0.0.1:11111`，FTWebSocket / JavaScript API 使用

更完整的术语、职责边界和排查顺序见 [troubleshooting/diagnostic-principles.md](troubleshooting/diagnostic-principles.md)。