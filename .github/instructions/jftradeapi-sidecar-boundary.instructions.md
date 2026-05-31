---
name: Sidecar 编排边界守卫
description: "当你新增或修改 pkg/jftradeapi 下代码、实现控制平面接口、调整实时行情编排、处理通知汇流，或评审 sidecar 与适配层/runtime 边界时使用。"
applyTo: "pkg/jftradeapi/**"
---
# Sidecar 编排边界守卫

`pkg/jftradeapi` 是前端控制平面与实时编排层，不承担交易所协议细节翻译，也不承载 UI 展示格式化。

## 分层边界

- `pkg/jftradeapi`：REST/WS 路由、订阅编排、缓存聚合、实时回退策略、通知收束。
- `pkg/futu` 及未来 broker 适配层：协议转换、连接交互、底层能力暴露。
- bbgo runtime：策略执行、交易生命周期、运行时状态机。
- `apps/web`：展示和交互层，中文标签等格式化只留在前端。

## 强约束

- sidecar 可以做能力路由和结果归一化，但不能下沉到协议字段级翻译（那是适配层职责）。
- sidecar 中可复用的多适配器编排逻辑应收敛到统一协调组件，避免在多个 route 文件重复。
- 对下游适配层返回的 `ErrNotSupported`，sidecar 需转换为明确的 API 语义（状态码+错误码+可读提示），但不得伪造可用结果。
- sidecar 只维护控制平面契约，不修改交易运行时核心语义。
- 若引入新 broker，优先扩展协调层路由，不要在业务路由里写大量 if/else 按 broker 分支。

## 评审清单

1. 这段逻辑属于控制平面编排，还是协议翻译？
2. 是否复用了统一协调层，而不是在路由中复制粘贴编排逻辑？
3. 对不支持能力是否返回了明确且一致的 API 错误语义？
4. 是否保持了前端契约稳定，不引入破坏性字段漂移？
5. 边界变化是否同步到架构文档？

## 反模式

- 在 `pkg/jftradeapi` 直接解析/拼装底层协议字段。
- 把 broker 特定分支逻辑散落在多个 route handler。
- 对适配层失败做“静默吞错”或伪成功回包。
- 在 sidecar 放置 UI 文案、标签映射等展示逻辑。

## 参考

- [系统架构总览](../../docs/architecture.md)
- [StockSharp 蒸馏技能](../skills/trading-arch-from-stocksharp/SKILL.md)
- [适配器外壳层深度参考](../skills/trading-arch-from-stocksharp/references/adapter-shell-pattern.md)
