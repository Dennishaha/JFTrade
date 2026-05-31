---
name: Futu 适配层边界守卫
description: "当你新增或修改 pkg/futu 下代码、实现交易所适配能力、拆分 futu 模块、引入新券商适配器，或评审协议翻译与业务编排边界时使用。"
applyTo: "pkg/futu/**"
---
# Futu 适配层边界守卫

`pkg/futu` 仅承担协议翻译职责，不要把 sidecar 编排、运行时策略或 UI 相关逻辑塞进适配层。

## 分层边界

- `pkg/futu`：OpenD 协议映射、传输生命周期钩子、消息转换、能力暴露。
- `pkg/jftradeapi`：订阅编排、实时/HTTP 回退采样、通知汇流、控制平面行为。
- bbgo runtime / strategy 层：策略执行、风控策略、交易运行时编排。
- `apps/web`：仅做展示格式化（例如中文标签），绝不改写 API 原始值。

## 强约束

- `pkg/futu` 只放适配器逻辑：请求/响应映射、codec/proto 转换、与协议传输强耦合的重试。
- 可在多个适配器复用的编排逻辑，不得在适配器内重复实现，应抽到 `pkg/futu` 之外的统一外壳/协调层。
- 不支持能力必须显式失败（例如 `ErrNotSupported`），禁止伪造成功。
- 避免从 `pkg/futu` 反向依赖 sidecar 或 web 关注点。
- 文件职责保持单一（沿用既有拆分粒度，如 `exchange_kline.go`、`exchange_trade_read.go`、`exchange_trade_write.go`）。

## 评审清单

1. 这次改动是协议翻译，还是业务编排？
2. 这段逻辑未来是否会被第二个券商适配器复用？
3. 若会复用，是否应该迁到共享外壳/协调层，而非留在 `pkg/futu`？
4. 不支持能力是否已明确暴露？
5. 若边界变更，文档是否同步更新？

## 反模式

- 在适配器协议处理代码中直接塞订阅注册表或重连恢复策略。
- 对不支持能力返回“部分模拟成功”。
- 在适配层放用户展示格式化或本地化逻辑。
- 把单文件不断膨胀为多职责模块，而不按关注点拆分。

## 参考

- [系统架构总览](../../docs/architecture.md)
- [StockSharp 蒸馏技能](../skills/trading-arch-from-stocksharp/SKILL.md)
- [适配器外壳层深度参考](../skills/trading-arch-from-stocksharp/references/adapter-shell-pattern.md)
