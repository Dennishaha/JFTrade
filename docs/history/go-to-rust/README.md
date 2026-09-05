# Go 到 Rust 迁移历史

本目录仅保存已经完成的迁移记录、执行手册和逐路由 ledger。它们不参与当前架构、路由所有权、门禁计划或发布资格计算。

当前产品事实以 [`../../architecture.md`](../../architecture.md)、[`../../architecture/quality-gates.md`](../../architecture/quality-gates.md) 和 [`../../architecture/release-qualification.md`](../../architecture/release-qualification.md) 为准。

相关深度审计与验证矩阵：

- [`2026-09-06-behavior-audit.md`](2026-09-06-behavior-audit.md)：本轮远端 Go / 本地 main 对比、已复现修复、实际验收范围与未闭环差异。
- [`go_to_rust_comprehensive_verification_matrix.md`](go_to_rust_comprehensive_verification_matrix.md)：Go 到 Rust 迁移全景深度验证矩阵与发布准入总览（主导航索引）
- [`verification-matrix/`](verification-matrix/)：十大核心领域代码级对比、边界失效推演与测试用例分卷目录
