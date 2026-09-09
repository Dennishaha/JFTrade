---
name: Futu 适配层边界守卫
description: "修改或评审 Rust Futu/OpenD 集成的协议映射、能力声明与生命周期边界时使用。"
applyTo: "crates/jftrade-integration-futu/**"
---

# Futu 适配层边界守卫

先读取根 [AGENTS.md](../../AGENTS.md)、[Rust 局部指令](../../crates/AGENTS.md) 和 [后端依赖边界](../../docs/architecture/backend-coding-standards.md)。本文件只提供 Futu 评审清单，不另行定义分层。

1. 改动是否属于协议翻译、I/O 或传输生命周期？Provider 选择、业务缓存和策略编排应回到后端规范定义的 owner。
2. 生成 protobuf 类型是否仍封装在 integration 内，跨边界使用协议中立 DTO/port？
3. 不支持能力、上游不可用、超时与取消是否保留明确失败语义，而非伪造成功？
4. 是否检查行情、交易、策略等实际调用方，并用 fixture/mock/testkit 覆盖拒绝与恢复路径？
5. 契约变化是否从规范源重新生成并验证？普通测试不得连接真实 OpenD；live 验证按根规则执行。
