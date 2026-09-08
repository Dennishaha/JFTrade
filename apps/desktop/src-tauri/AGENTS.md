# Tauri 桌面局部指令

继承根 [AGENTS.md](../../../AGENTS.md)。先读 [桌面构建与发布](../../../docs/troubleshooting/desktop-release.md)；候选或发布任务再读 [发布资格](../../../docs/architecture/release-qualification.md)。以下路径相对此目录；命令从仓库根目录运行。

## 入口与边界

- 入口为 `src/main.rs` / `src/lib.rs`；profile、运行时生命周期和 IPC 契约分别从 `src/profile.rs`、`src/lifecycle.rs`、`src/contract.rs` 定位，原生能力从 `src/native.rs` 进入。
- Tauri 只管理窗口、单实例、更新、桌面专属服务和受管 runtime；业务能力经 Rust engine 的 HTTP/SSE/WS API 访问，IPC 不形成第二套业务 API。
- 保持开发/正式通道的数据目录、端口和身份隔离；正式产品不扫描或迁移开发数据。桌面临时 token 不能泄露到日志或 Web 入口。
- runtime 资产由根 `scripts/run-tauri.mjs`、`scripts/prepare-tauri-release-runtime.mjs` 准备并校验；不手改 bundle、manifest、摘要或 embedded assets，不对缺失资产静默降级。

## 最小验证

```bash
pnpm run test:tauri-release-runtime
node scripts/quality/cargo-nextest.mjs run -p jftrade-desktop --all-targets --locked
pnpm run check:quick
```

Rust 变更追加 `pnpm run check:rust`。涉及资产时，对已准备的当前平台资产运行 `pnpm run check:tauri-release-runtime`；实际打包与 smoke 按桌面专题执行，不能把开发配置的测试通过当作正式包验证。

`build:desktop` 会准备发布资产并构建，要求显式版本标识，不属于只读检查。除非用户明确要求，不创建候选、tag 或 Release，不触发签名/发布 workflow。
