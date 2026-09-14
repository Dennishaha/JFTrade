# API / SSE / WebSocket / Transport 第一批范围

本批次只处理 Go 分支 `internal/api/**`、`cmd/jftrade-api/**` 中属于 HTTP、SSE、WebSocket、认证和传输生命周期的测试；Assistant/Trading/Watchlist 业务语义留给各自批次。

## 逐项规则

- 每项使用 `Go 文件路径:行号:测试名` 作为唯一键。
- 只有 Rust 可执行测试函数与 Go 断言逐条一致时才标记 `function_exact`/`[x]`。
- Rust 仅覆盖部分断言时使用 `partial`/`[~]`，记录真实入口和缺失断言；没有入口则保持 `missing`。
- `cargo-nextest` 命令必须指向实际 crate 与测试过滤器；边界保留项不得伪造 Rust 入口。

## 当前批次基线

由 `scripts/compatibility/audit_test_parity.py` 生成：API Server & Transport Wire 共 951 条 Go 测试；本批已建立 9 条 `partial` 入口证据，仍有明确缺口，未提升为 `function_exact`。现有 50 条全局 `function_exact` 证据继续接受唯一入口校验。

## 完成门槛

逐条阅读 Go 断言并核对 Rust 测试源码；每个新增映射附带入口、差异结论和验证命令。批次结束运行审计脚本、`pnpm run check:ai-context`、相关 crate nextest 与 `git diff --check`。
