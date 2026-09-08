# Rust 局部指令

继承根 [AGENTS.md](../AGENTS.md)。先读 [后端编码与依赖边界](../docs/architecture/backend-coding-standards.md)；涉及门禁再读 [质量门禁](../docs/architecture/quality-gates.md)。以下路径相对 `crates`；命令从仓库根目录运行。

## 定位顺序

- 独立启动从 `jftrade-engine/src/bin/jftrade-api-rust.rs` 进入；生产装配、route/port 连接和运行时回收在 `jftrade-engine`。
- HTTP/SSE/WS 从 `jftrade-api/src/router.rs`、`ports.rs` 进入；先区分 transport 问题与领域规则，不在 handler 内补第二份业务实现。
- 按所属 crate 的 `src/lib.rs`、`Cargo.toml` 和调用方定位领域公开 port；存储和外部协议只进入对应 `jftrade-store-*`、`jftrade-integration-*`。
- 测试先找同域 `tests/` 和源码内测试；兼容回放输入在根 `tests/fixtures/compatibility/`，冻结 golden 不作为修复目标。

## 实现检查点

- protobuf 类型留在对应 integration，跨域只传协议中立 DTO/port；不要为测试便利反向依赖 engine/API 或具体协议。
- mutation 保持事务、WriterLease 和 owner fence；覆盖取消、冲突、busy、schema drift 与恢复路径。
- 新 task/process 必须有明确 owner、cancel、join 和有界 shutdown；错误保留业务分类，不通过字符串匹配控制流程。
- 依赖从根 `Cargo.toml` 继承；不要为未使用的抽象新增 crate 或将业务规则堆入 composition。

## 最小验证

```bash
node scripts/quality/cargo-nextest.mjs run -p <changed-crate> --all-targets --locked
pnpm run check:quick
pnpm run check:rust
```

测试使用仓库 nextest wrapper，统一固定版本、校验和与非打包编译环境；不要用裸 `cargo test` 替代标准门禁。生产 crate 变化还会影响反向依赖，局部测试通过不能替代 `check:rust`。公开契约变更追加 `pnpm run check:generated`。
