# OpenD 协议入口

本文不是 Futu 文档翻译，而是说明本项目当前实际依赖 OpenD 的哪些能力、对应哪层实现、排障时先看哪里。

## 当前使用范围

本项目当前主要用到：

- `1001` InitConnect
- `1003` Notify
- `1004` KeepAlive
- `3001` Qot_Sub
- `3004` GetBasicQot
- `3005` Qot_UpdateBasicQot
- `3006` GetKL
- `3103` RequestHistoryKL

交易相关协议则按 bbgo/sidecar 需求逐步扩展。

## 当前端口语义

- `127.0.0.1:11110`：原生 OpenD TCP/protobuf API，当前 Go 客户端使用
- `127.0.0.1:11111`：FTWebSocket / JavaScript API

当前 [../../pkg/futu/opend/client.go](../../pkg/futu/opend/client.go) 通过 TCP 连接 `11110`。不要把 11111 写成 Go 原生 RPC 端口。

## 实现分层

| 位置 | 职责 |
| --- | --- |
| `pkg/futu/codec` | 44 字节帧编解码 |
| `pkg/futu/opend` | OpenD TCP 客户端、keepalive、请求响应关联 |
| `pkg/futu/exchange.go` | Futu 适配层，对上提供 Exchange 能力 |
| `pkg/futu/stream.go` | 基于 BasicQot push 的实时流桥接 |

## 当前项目约定

- 原生 TCP 客户端不依赖 WebSocket key 完成 API 探针和原生 RPC
- OpenD settings 中的 key 仍会保留，用于与 FTWebSocket 相关的设置和诊断保持一致
- sidecar 与 bbgo 共用 `pkg/futu`，但不是同一条调用链

## 继续阅读

- 需要完整字段表：看 [Futu-API-Doc-zh-Proto.md](Futu-API-Doc-zh-Proto.md)
- 需要知道当前系统边界：看 [../architecture.md](../architecture.md)
- 需要查连接问题：看 [../troubleshooting/opend-configuration.md](../troubleshooting/opend-configuration.md)