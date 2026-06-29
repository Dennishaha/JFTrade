# 故障排查入口

本文只做分诊，不再把所有细节堆在一页里。

先记住两个前提：

- 前端默认依赖的是 JFTrade sidecar 的 `/api/v1/*`，不是 bbgo 原生 `/api/*`
- JFTrade 当前只支持 API sidecar 运行模式，前端不连接 bbgo 原生 `/api/*`

## 先看症状，再进专题

| 症状 | 去看哪里 | 快速验证 |
| --- | --- | --- |
| 后端起不来、端口不对、启动后马上退出 | [troubleshooting/startup-ports.md](troubleshooting/startup-ports.md) | 开发态查 `http://127.0.0.1:3000/api/v1/system/status`，发布态查 `http://127.0.0.1:6688/api/v1/system/status` |
| 前端显示实时通道断开或没有心跳 | [troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md) | `go test ./internal/app/apiserver/servercore -run TestLiveSSESendsHeartbeat` |
| OpenD 连不上、设置保存了但运行时没生效 | [troubleshooting/opend-configuration.md](troubleshooting/opend-configuration.md) | 开发态查 `http://127.0.0.1:3000/api/v1/system/futu-opend`，发布态查 `http://127.0.0.1:6688/api/v1/system/futu-opend` |
| 美股盘前盘后或夜盘显示异常 | [troubleshooting/us-extended-hours.md](troubleshooting/us-extended-hours.md) | 检查 snapshot 是否带 `lastClosePrice` 和扩展行情块 |
| 回测明显变慢，或不确定慢在 sync 还是 replay | [troubleshooting/backtest-performance.md](troubleshooting/backtest-performance.md) | `JFTRADE_REAL_CHAIN_PROFILE=1 go test ./pkg/backtest -run '^TestRealUSMarch2026DoubleMATemplateProfile$' -v` |
| Pine 策略执行失败、worker 没启动、发布包缺 worker | [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md) | 检查 `JFTRADE_PINEWORKER_BUNDLE`、Bun runtime、`release_assets` 构建和非 mock smoke |
| 不确定到底该先查哪一层 | [troubleshooting/diagnostic-principles.md](troubleshooting/diagnostic-principles.md) | 先按职责边界和决策树定位 |

## 常用诊断命令

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
curl -fsS http://127.0.0.1:6688/api/v1/system/status
curl -fsS http://127.0.0.1:6699/api/v1/system/status
curl -fsS http://127.0.0.1:3000/api/v1/settings/brokers
curl -fsS http://127.0.0.1:6688/api/v1/settings/brokers
curl -fsS http://127.0.0.1:6699/api/v1/settings/brokers
curl -fsS http://127.0.0.1:3000/api/v1/system/futu-opend
curl -fsS http://127.0.0.1:6688/api/v1/system/futu-opend
curl -fsS http://127.0.0.1:6699/api/v1/system/futu-opend

lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:6688 -sTCP:LISTEN
lsof -nP -iTCP:6699 -sTCP:LISTEN
lsof -nP -iTCP:6688 -sTCP:LISTEN
lsof -nP -iTCP:11110 -sTCP:LISTEN
lsof -nP -iTCP:11111 -sTCP:LISTEN

go test ./...
```

## 术语统一

- API sidecar：`go run ./cmd/jftrade-api`，启动 JFTrade 控制台后端
- 发布态 GUI：默认 `127.0.0.1:6688`，内嵌前端、`/api/v1/*` 与 Swagger 的同源入口
- 发布态 gateway：默认 `127.0.0.1:6699`，API 直连与排障入口
- OpenD API port：默认 `127.0.0.1:11110`，Go 原生 TCP API 使用
- OpenD WebSocket port：默认 `127.0.0.1:11111`，FTWebSocket / JavaScript API 使用

更完整的术语、职责边界和排查顺序见 [troubleshooting/diagnostic-principles.md](troubleshooting/diagnostic-principles.md)。
