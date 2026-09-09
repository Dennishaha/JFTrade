---
name: trading-arch-from-stocksharp
description: "借鉴 StockSharp 评审 JFTrade 交易系统的职责与依赖边界。用于适配器扩展、协议与业务编排拆分、回测/实盘或策略创作架构决策；不用于普通页面修改，不要求引入新框架或预建多券商层。"
---

# 交易系统架构参考

[StockSharp](https://github.com/StockSharp/StockSharp) 是设计参考，不是 JFTrade 的实现规范。先读取根 [AGENTS.md](../../../AGENTS.md)、目标目录局部指令和 [当前架构](../../../docs/architecture.md)；依赖约束以 [后端规范](../../../docs/architecture/backend-coding-standards.md) 为准，不在本 skill 复制模块映射。

## 评审方法

- 从当前调用方和状态写入 owner 出发，判断问题是协议翻译、领域决策、生产装配还是展示；不要按上游类名机械创建本地模块。
- 只有存在具体消费者和稳定职责时才抽 port 或共享实现；新增券商必须遵循 [集成指南](../../../docs/new-broker-integration-guide.md)，不为未来假想需求预建调度层。
- 明确区分上游设计观察、本地源码事实与建议。需要源码级对照时使用用户提供的 checkout/ref 或上游仓库，并记录所查 commit；不要假设本机存在固定的 StockSharp 目录。
- 检查能力声明和失败路径：不可用、不支持、超时与取消应有明确语义，不以空结果、伪成功或静默跨 Provider 回退掩盖。

## 按问题补充上下文

- 协议适配、订阅恢复、跨适配器能力路由：读 [适配器与编排边界参考](references/adapter-shell-pattern.md)。
- 回测、撮合与实盘差异：读 [回测执行模型](../../../docs/backtest-execution-model.md)，按现有成交、资金曲线和风控 owner 评审，不把“共用策略”解释为两种模式完全等价。
- 可视化图块、Pine 编辑和执行：读 [策略创作](../../../docs/frontend/strategy-authoring.md) 与 [PineTS 契约](../../../docs/pinets-contract-audit.md)，不另建执行引擎或恢复旧编译链路。

## 输出与收尾

给出当前问题、归属依据、最小调整及对应行为测试。若只要求评审，交付建议而不实施；若要求实现，按根/局部指令验证，只在边界变化时同步专题文档。不要把任务结论写成独立于仓库事实源的新规则。
