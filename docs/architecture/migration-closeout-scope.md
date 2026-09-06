# Go → Rust 收尾提交范围说明

本文记录收尾阶段历史提交的实际功能边界，避免把验证标题误读为“仅测试”。不改写共享历史；后续整理通过增量提交完成。

## 已有提交的实际范围

| 提交 | 实际内容 | 范围说明 |
| --- | --- | --- |
| `c0a297c3` | 实盘控制面风险协调器、硬停审计，以及通知投影器初版 | 风险与通知属于两个可独立回滚的生产功能，后续以文件/模块边界拆分说明，不回退已验证行为。 |
| `0c8d8437` | RuntimeRisk 动态读取、策略生命周期 CAS、运行时模块拆分 | 模块拆分服务于生产文件行数约束；状态机语义以当前 CAS 实现为准。 |
| `086c7e35` | Pine order intent 归一化、实例归属撤单、执行模块拆分 | Pine 只产出 intent，Rust 负责账户快照、风控和下单。 |
| `21cfaa13` | Native/LiveHub 通知游标和设置过滤 | 游标与事件幂等标记均属于执行数据库投影，不改变 HTTP/SSE/WebSocket 契约。 |
| `9ee1b494` | 组合单方向净名义金额、金额模式优先级和报价预取 | 只涉及交易域风控与报价读取。 |
| `e3fe8e3d` | Provider managed consumer 约束、Helper/OpenD 运行时切换 | 交易 OpenD 会话与市场数据 Provider 保持独立生命周期。 |
| `15d9d6bc` | MCP 严格 schema、兼容归一化、replay-safe allowlist | 兼容字段应继续集中到单一边界，禁止在执行器内重复 fallback。 |
| `f253e867` | 端到端验收，同时包含生产装配、错误映射、ADK 目录和严格控制读取 | 标题并不代表纯测试；新增生产行为需在后续提交中单独标明。 |

## 收尾提交规则

- 生产修复、持久化/兼容层和验证辅助保持独立提交；验证提交不得再修改生产逻辑。
- 不执行 destructive reset，不重写上述共享历史，不执行 push。
- 不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite 现有表结构或 Pine worker wire contract；若持久化幂等确需新表，必须先提交 schema 评审与兼容迁移说明。
- `PRODUCTION_DATABASE_IDS` 等仅测试访问的 re-export，只有在确认无生产调用方后才删除。
