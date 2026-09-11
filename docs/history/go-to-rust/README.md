# Go 到 Rust 迁移历史

本目录保存迁移记录、行为审计、执行手册和逐路由 ledger；其中的历史结论不等同于当前完成声明，也不参与当前架构、路由所有权、门禁计划或发布资格计算。

当前产品事实以 [`../../architecture.md`](../../architecture.md)、[`../../architecture/quality-gates.md`](../../architecture/quality-gates.md) 和 [`../../architecture/release-qualification.md`](../../architecture/release-qualification.md) 为准。

相关深度审计与验证矩阵：

- [`2026-09-09-project-parity-audit.md`](2026-09-09-project-parity-audit.md)：固定 Go/main 基线的全项目能力盘点、F01–F05 修复后证据，以及 G01–G07 的限定验证；G05 真实旧发布包/四平台安装升级和 G08 live 验收仍未验证，审计中的结论不等同于迁移已完成或发布资格声明。
- [`2026-09-06-behavior-audit.md`](2026-09-06-behavior-audit.md)：本轮远端 Go / 本地 main 对比、已复现修复、实际验收范围与未闭环差异。
- [`go_to_rust_comprehensive_verification_matrix.md`](go_to_rust_comprehensive_verification_matrix.md)：Go 到 Rust 迁移全景深度验证矩阵与发布准入总览（主导航索引）
- [`verification-matrix/`](verification-matrix/)：十大核心领域代码级对比、边界失效推演与测试用例分卷目录
