# 诊断原则与术语统一

本文回答一个问题：遇到问题时，应该先定位在哪一层，再决定往哪份实现和文档钻。

## 核心心智模型

当前系统至少有四层：

- `cmd/jftrade`：决定是 API-only 还是 bbgo run
- `pkg/jftradeapi`：提供前端默认使用的 `/api/v1/*`
- `pkg/futu`：Futu 适配层，被 sidecar 和 bbgo 共享
- bbgo runtime：策略与交易运行时，不等于前端默认后端

排查时先分层，不要一上来假设所有问题都在 bbgo 或前端。

## 常用术语

| 术语 | 当前含义 |
| --- | --- |
| API-only | `go run ./cmd/jftrade api`，只跑 sidecar，用于前端开发和控制平面调试 |
| bbgo run | `go run ./cmd/jftrade run --config ./config/jftrade.yaml`，跑 bbgo 运行时，并尝试同时带起 sidecar |
| sidecar | `pkg/jftradeapi` 提供的前端适配与控制平面 |
| `/api/v1/*` | JFTrade 自有 API 契约 |
| `/api/*` | bbgo 原生路由，不是当前控制台默认接口 |
| OpenD API port | 默认 `11110`，Go 原生 TCP/protobuf 使用 |
| OpenD WebSocket port | 默认 `11111`，FTWebSocket / JavaScript API 使用 |
| observedAt | 前端实时分桶统一使用的时间参考 |

## 推荐排查顺序

1. 进程是不是还活着
2. 目标端口是不是在监听
3. 前端连的是哪条路由
4. 运行时配置是否真的写回到 Go 后端
5. OpenD 是没启动、端口填错，还是 accept 路径卡住

## 快速决策树

```text
页面异常
  -> 先看当前模式的 sidecar/gateway 端口是否还在
  -> 开发态默认 127.0.0.1:3000，发布态同源入口默认 127.0.0.1:6688，API 直连入口默认 127.0.0.1:6699
  -> 如果 settings.json 的 interfaces 或环境变量改过绑定地址，先按实际配置检查
  -> 如果 3000/6688/6699 不在：查启动模式和启动退出日志
  -> 如果 3000 在但实时 SSE 断开：查 /api/v1/stream/live 和 sidecar
  -> 如果设置保存后无效：查 settings.json 与运行时 env 注入
  -> 如果行情异常：查 OpenD 端口、sidecar 查询链路和前端实时合成
```

## 统一修复原则

- 不用浏览器本地状态掩盖后端失败
- 不把 `/api/v1/*` 和 `/api/*` 混为一谈
- 不把 `11110` 和 `11111` 混用
- 修复后至少用一个 `curl` 或测试用例验证，而不是只看 UI 颜色变化

## 相关文档

- [startup-ports.md](startup-ports.md)
- [live-stream-connection.md](live-stream-connection.md)
- [opend-configuration.md](opend-configuration.md)
- [us-extended-hours.md](us-extended-hours.md)
