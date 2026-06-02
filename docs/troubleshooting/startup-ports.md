# 启动与端口诊断

本文回答三个问题：

- 当前应该用哪种启动模式
- 各端口分别是谁在提供
- 为什么后端启动后会马上退出

## 两种启动模式

| 模式 | 命令 | 适用场景 | 关键差异 |
| --- | --- | --- | --- |
| API-only | `go run ./cmd/jftrade api` | 前端开发、设置调试、行情调试 | 只启动 sidecar，不进入 bbgo 策略运行时 |
| bbgo run | `go run ./cmd/jftrade run --config ./config/jftrade.yaml` | 完整策略与交易运行时 | 进入 bbgo，并尝试同时启动 sidecar |

[cmd/jftrade/main.go](../../cmd/jftrade/main.go) 在进程入口会默认写入 `DISABLE_MARKETS_CACHE=1`，避免空 market cache 把 bbgo 启动过程带偏。

## 默认端口

| 组件 | 默认地址 | 用途 |
| --- | --- | --- |
| 开发态 Web GUI | `127.0.0.1:5173` | Vite dev server |
| 开发态 JFTrade sidecar | `127.0.0.1:3000` | 前端 `/api/v1/*`、SSE、WS |
| 发布态 Web GUI | `127.0.0.1:6688` | 内嵌前端、`/api/v1/*`、SSE、WS、Swagger 的同源入口 |
| 发布态 JFTrade gateway | `127.0.0.1:6699` | API 直连与排障入口 |
| Futu OpenD API | `127.0.0.1:11110` | Go 原生 TCP/protobuf 查询与探针 |
| Futu OpenD WebSocket | `127.0.0.1:11111` | FTWebSocket / JavaScript API |

发布态首次启动时，还会在二进制旁边生成 `var/jftrade-api/` 运行时目录，默认把 `settings.json`、`backtest.db` 和策略运行时文件都放到这里。

如果存在 `var/jftrade-api/settings.json`，现在也可以通过顶层 `interfaces` 字段覆盖默认监听地址；启动优先级是环境变量最高，其次 `settings.json`，最后才是默认端口。例如：

```json
{
	"interfaces": {
		"apiBind": "127.0.0.1:6699",
		"guiBind": "127.0.0.1:6688"
	}
}
```

## 快速检查

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
curl -fsS http://127.0.0.1:6688/api/v1/system/status
curl -fsS http://127.0.0.1:6699/api/v1/system/status
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:6699 -sTCP:LISTEN
lsof -nP -iTCP:6688 -sTCP:LISTEN
lsof -nP -iTCP:11110 -sTCP:LISTEN
lsof -nP -iTCP:11111 -sTCP:LISTEN
```

## 后端启动后马上退出

最常见的日志是：

```text
market info should not be empty, 0 markets loaded
```

这不是前端问题，而是 bbgo bootstrap 失败。当前要求是：

- `pkg/futu.Exchange.QueryMarkets()` 至少返回一个 bootstrap market
- `DISABLE_MARKETS_CACHE=1` 保持启用，避免旧空缓存继续污染启动

如果你在前端开发阶段不需要完整交易运行时，优先切回 API-only，先把 sidecar 相关问题单独排清。

## FUTU_OPEND_ADDR 缺失或端口写错

当前默认值是 `127.0.0.1:11110`。如果 `FUTU_OPEND_ADDR` 缺失，`pkg/futu` 会回退到这个默认地址；如果你显式覆盖了错误端口，启动仍然会失败。

建议检查：

```bash
echo "$FUTU_OPEND_ADDR"
echo "$JFTRADE_FUTU_API_PORT"
```

## 需要避免的旧表述

- 不要写“bbgo server 起不来，所以前端断开”，应写清到底是开发态 sidecar 3000 消失、发布态 gateway 6699 消失，还是 bbgo 自身失败
- 不要把 `/api/v1/*` 说成 bbgo 自带接口
- 不要把 `start.sh` 的默认行为等同于所有运行方式，真正的模式判断在 [../../cmd/jftrade/main.go](../../cmd/jftrade/main.go)
