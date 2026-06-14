# OpenD 配置与诊断

本文回答两个问题：

- OpenD 的端口、设置和运行时注入到底怎么对应
- 为什么会出现“保存成功但仍连不上”或 `request timed out`

## 先分清两个端口

| 端口 | 默认值 | 用途 |
| --- | --- | --- |
| OpenD API port | `11110` | Go 原生 TCP/protobuf 客户端使用 |
| OpenD WebSocket port | `11111` | FTWebSocket / JavaScript API 使用 |

[../../pkg/futu/opend/client.go](../../pkg/futu/opend/client.go) 当前明确使用的是原生 TCP API。不要把 Go 侧连接写成 11111。

## 设置写入链路

```text
Settings UI
  -> /api/v1/settings/*
  -> var/jftrade-api/settings.json
  -> applyRuntimeEnv()
  -> FUTU_OPEND_ADDR / JFTRADE_FUTU_API_PORT / JFTRADE_FUTU_WEBSOCKET_PORT
```

[../../internal/app/apiserver/runtime/integration_env.go](../../internal/app/apiserver/runtime/integration_env.go) 会把当前保存的配置注入运行时环境，设置保存后的运行时重置由 [../../internal/app/apiserver/servercore/server.go](../../internal/app/apiserver/servercore/server.go) 装配。

## 快速验证

```bash
curl -fsS http://127.0.0.1:3000/api/v1/settings/brokers
curl -fsS http://127.0.0.1:3000/api/v1/system/futu-opend
cat var/jftrade-api/settings.json
```

如果 UI 显示保存成功，但 `curl` 读不回新配置，就不要再查前端，直接查 sidecar 保存链路。

## 常见错误

### `opend: request timed out`

优先检查是不是把 Go 客户端指到了 `11111`。对 FTWebSocket 端口发送原生 RPC 会建立连接但不回包，最终表现为超时。

### `opend: dial 127.0.0.1:11110: i/o timeout`

如果 `lsof` 仍能看到 `11110` 在监听，但 `nc` 或实际请求超时，通常是 OpenD native API accept 路径卡住，需要重启 OpenD。

### 设置保存了但运行时没生效

先检查：

- `var/jftrade-api/settings.json` 是否已更新
- `GET /api/v1/settings/brokers` 是否能读回新值
- `GET /api/v1/system/futu-opend` 的 `lastError` 是否变化

## 关于 WebSocket Key

当前项目仍会把 `FUTU_OPEND_WEBSOCKET_KEY` 和 `JFTRADE_FUTU_WEBSOCKET_KEY` 注入环境，方便与 FTWebSocket 相关的设置和诊断共用同一份配置。

但当前原生 Go OpenD TCP 客户端不依赖这个 key 完成 API 探针和原生 RPC。不要把它写成“Go 连接 API port 时必须使用的鉴权项”。

## 需要避免的旧表述

- 不要把 `websocket_key_md5` 说成设置页应该填写的值，设置页应填明文 key
- 不要写“Go 客户端走 WebSocket 连接 OpenD”
- 不要把 `11110` 和 `11111` 混用成“都可以”
