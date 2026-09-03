# 永久产品门禁

更新时间：2026-09-03。

JFTrade 的质量门禁面向当前 Rust/Tauri 产品，不使用迁移阶段作为调度或通过条件。

## 固定入口

- `check:policy`：零 Go、workspace 架构、生产路由策略、测试命名、AI 上下文和 workflow policy。
- `check:contracts`：OpenAPI、生成物、278 条 Rust 路由与认证策略。
- `check:rust:static`：target health、fmt、Clippy、workspace architecture、production policy 和 `cargo deny`。
- `test:rust`：唯一完整 Rust 测试命令，固定为 `cargo test --workspace --all-targets --locked`。
- `check:compatibility`：按 storage、backtest、provider-runtime、trading-strategy、assistant-runtime、api-transport、desktop-runtime 七类回放冻结语料。
- `check:web`、`check:pine`、`check:python`、`check:desktop`：各运行时独立验证。
- `check:quick`、`check:affected`、`check:all`：工作树快速反馈、merge-base affected 和完整本地入口。

## PR 与 main

PR 的 `gate-plan` 从 merge-base 计算受影响 lane。未知产品路径、Cargo/lockfile/toolchain、workflow、门禁脚本或 module map 变化会 fail closed 为全量；生产 crate 变化包含 workspace 反向依赖。planner 失败同样输出全量计划。

`main` 无条件执行完整核心门禁。Policy、Contracts、Rust Static、Rust Tests + Compatibility、Web、Pine 和 Python 并行；compatibility 在 workspace tests 后复用同一 job 的 `target`，七类 replay 并行。Desktop 只依赖自己的 Web、Pine、sidecar 和 contracts 构建输入，不等待 Rust Static 或 Rust Tests。

保护分支只要求稳定 context `Build & Test`。聚合器仅在 planner 标记 lane 无需执行时接受 `skipped`。

## 冻结语料

语料位于 `tests/fixtures/compatibility/<capability>`。每个能力 manifest 只记录 schema、能力、来源 release 和文件 SHA-256；golden 不由旧实现重新生成。历史来源信息可以保留在 `docs/history/go-to-rust`，但不参与当前状态计算。
