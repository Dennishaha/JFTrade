---
name: Futu 适配层边界守卫
description: "当你新增或修改 pkg/futu 下代码、实现交易所适配能力、拆分 Futu 模块，或评审协议翻译与业务编排边界时使用。"
applyTo: "pkg/futu/**"
---

# Futu 适配层边界守卫

`pkg/futu` 只承担 OpenD/bbgo 协议翻译、能力声明和传输生命周期钩子。

## 分层边界

- `pkg/futu`：请求/响应映射、codec、协议传输和明确的 `ErrNotSupported`。
- `internal/marketdata`：demand、订阅 freshness、tick cache、回退轮询和 backoff。
- `internal/app/apiserver/marketdataapp`：行情 runtime 装配和 provider 投影。
- `internal/api/*`：HTTP/SSE/WS 参数绑定、错误映射和 wire DTO。
- `apps/web`：展示格式化和交互状态，不改写 API 原始值。

## 评审清单

1. 这是协议翻译，还是业务编排？编排不得进入 `pkg/futu`。
2. 不支持能力是否显式返回 `ErrNotSupported`？
3. 是否会影响 sidecar、策略 runtime 和回测三个调用方？
4. 修改协议后是否更新 fixture、生成代码和 reference 文档？
