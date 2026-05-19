# 故障排查手册

本文记录 JFTrade + bbgo + Futu OpenD 集成中容易踩到的启动、WebSocket 与配置保存问题。排查时先确认职责边界：bbgo 负责交易运行时与 exchange 抽象，JFTrade 前端依赖的是本项目自有的 `/api/v1/*` API 适配层。

## 快速检查

```bash
# 后端 JFTrade API 是否可达
curl -fsS http://127.0.0.1:3000/api/v1/system/status

# Futu 设置是否由 Go 后端返回，而不是浏览器本地兜底
curl -fsS http://127.0.0.1:3000/api/v1/settings/brokers

# 端口监听
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:11111 -sTCP:LISTEN

# 基础验证
go test ./...
NODE_OPTIONS=--no-deprecation npm run typecheck
NODE_OPTIONS=--no-deprecation npm run build:web
```

默认端口约定：

| 服务 | 默认地址 | 说明 |
|------|----------|------|
| JFTrade API | `127.0.0.1:3000` | 前端 `/api/v1/*`、SSE、WS 连接目标 |
| Web preview | `127.0.0.1:6688` | `vite preview` |
| Futu OpenD API | `127.0.0.1:11110` | Go OpenD client 实际连接端口 |
| Futu OpenD WebSocket | `127.0.0.1:11111` | FTWebSocket / JavaScript 接入端口 |

## 前端 WS 连不上

症状：状态栏显示 `WS disconnected` 或之前显示 `WS error`，前端没有实时心跳。

根因优先级：

1. 前端连接的是 `ws://127.0.0.1:3000/api/v1/ws/live`，不是 bbgo 自带 server。
2. bbgo 自带 webserver 默认 `:8080`，且路由是 `/api/*`，不提供 JFTrade 前端需要的 `/api/v1/ws/live`。
3. 如果 `cmd/jftrade run` 进程启动后又退出，刚打开的 JFTrade API 也会一起消失，前端会表现为连不上。

正确修法：

* 保持 `pkg/jftradeapi` sidecar 随 `cmd/jftrade` 启动，提供 `/api/v1/ws/live` heartbeat。
* 前端默认 API/WS/SSE 地址使用 `127.0.0.1:3000`，避免 `localhost` 在 IPv4/IPv6 上解析不一致。
* 用 `go test ./pkg/jftradeapi -run TestLiveWebSocketSendsHeartbeat` 验证 WS handler。

不要只把前端 `error` 显示改成灰色。那只能隐藏症状，不能建立 WebSocket 连接。

## OpenD 设置保存不生效

症状：Settings 页面填入 Host、API Port、WebSocket Port、WebSocket Password / Key 后，刷新或重新检测仍显示旧值，OpenD 仍连不上。

根因：

* `/api/v1/settings/brokers` 和 `/api/v1/settings/brokers/{id}/integration` 是 JFTrade 自有 API 契约，不是 bbgo 原生接口。
* 如果后端没有实现这些接口，前端用 `localStorage` 兜底会造成“看起来保存成功，但 Go 后端没有生效”。

正确修法：

* 设置必须保存到 Go 后端，默认存储文件是 `var/jftrade-api/settings.json`。
* 保存成功后，Go 进程要同步运行时配置：
  * `FUTU_OPEND_ADDR=host:apiPort`
  * `JFTRADE_FUTU_WEBSOCKET_KEY=<plain key>`
  * `FUTU_OPEND_WEBSOCKET_KEY=<plain key>`
* `GET /api/v1/settings/brokers` 应该返回刚保存的 `apiPort=11110`、`websocketPort=11111`、`websocketKey`。

注意：OpenD 配置文件中的 `websocket_key_md5` / UI 中的 `websocket_key` 只用于 FTWebSocket（JavaScript API）。JFTrade 的 Go 后端连接的是原生 OpenD API 端口 `apiPort`，走 TCP protobuf，不需要也不会发送 WebSocket 鉴权 key。

## 后端启动后马上退出

症状：日志中先看到 JFTrade API listening，然后 bbgo 报错退出；前端随后无法连接 `127.0.0.1:3000`。

常见日志：

```text
market info should not be empty, 0 markets loaded
```

根因：bbgo bootstrap 要求 exchange market map 非空；旧 market cache 或 `QueryMarkets()` 返回空会让整个进程退出。

正确修法：

* `pkg/futu.Exchange.QueryMarkets()` 至少返回一个 bootstrap market，例如 `HK.00700`。
* 启动时默认设置 `DISABLE_MARKETS_CACHE=1`，避免旧空缓存继续污染启动。
* `start.sh`、`start.cmd` 和 `cmd/jftrade/main.go` 都应覆盖这个默认值。

## FUTU_OPEND_ADDR 缺失

症状：

```text
exchange session configure error: futu exchange: missing FUTU_OPEND_ADDR
```

根因：bbgo session bootstrap 会通过 exchange factory 的 `EnvLoader` 构造 exchange；如果没有 OpenD 地址，session 初始化失败。

正确修法：

* `pkg/futu` factory 在 session-prefixed env 与 `FUTU_OPEND_ADDR` 都为空时，回退到 `127.0.0.1:11110`。
* 启动脚本显式导出：

```bash
export FUTU_OPEND_ADDR=${FUTU_OPEND_ADDR:-127.0.0.1:11110}
```

## OpenD 仍显示未连接

按顺序确认：

1. OpenD GUI 已登录。
2. OpenD API 已启用，监听 `127.0.0.1:11110`。
3. OpenD WebSocket 已启用，监听 `127.0.0.1:11111`，仅供 FTWebSocket / JavaScript API 使用。
4. 如果 OpenD 配置了 `websocket_key_md5`，Settings 中填写对应明文 key，不要填写 MD5 密文。
5. `apiPort` 是 `11110`，`websocketPort` 是 `11111`，不要混用；Go 探针和交易连接走 `apiPort`。
6. 保存后调用：

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/futu-opend
```

`runtime.connectivity` 应从 `disconnected` 变为 `connected` 或至少给出明确 `lastError`。

如果 `lastError` 是：

```text
opend: request timed out
```

优先检查是否把 Go 客户端错误地指到了 `websocketPort`。已验证 `127.0.0.1:11110` 才是原生 OpenD protobuf/TCP 端口，`127.0.0.1:11111` 是 FTWebSocket 端口；对 11111 发送原生 RPC 会建立握手但不回包，最终表现为 `request timed out`。

## 排查原则

* 先判断接口属于 bbgo 还是 JFTrade 自有 API，不要假设 bbgo 会提供 `/api/v1/*`。
* 不用浏览器本地状态掩盖后端失败；保存类问题必须能通过 `curl` 从 Go 后端读回。
* 如果前端连不上，先看进程是否还活着，再看端口是否监听，最后看 WS 路由。
* 任何修复都要覆盖启动链路：直接 `go run`、`start.sh`、`start.cmd` 的默认行为应保持一致。