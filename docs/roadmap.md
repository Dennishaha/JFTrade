# JFTrade 活动路线图

更新时间：2026-08-31。

本文只记录当前仍未闭合、且需要继续投入的工作。已完成事项只保留在 [Go → Rust 迁移账本](architecture/go-to-rust-migration.md)、[Stage 9 closeout manifest](../tests/fixtures/rust-migration/stage9/closeout-evidence.json) 和 Git 历史中，不再进入活动清单或后续智能体任务。

## 当前放行边界

Stage 9 closeout manifest 仍为 `in_progress`。后续放行只处理 manifest 中状态为 `open` 或 `blocked` 的 gate；状态为 `passed` 的 gate 不再派工，也不得用历史本地检查结果替代发布证据。

## 运行时未闭合项

按依赖和共享文件冲突依次收口，完成前不再扩展新的 route group。

- [ ] **Execution reconciliation 端到端证据**：补 yfinance/AKShare 行情 + Futu 交易的 account/order/history/fill/fee、UNKNOWN 状态、CAS 和重启幂等证据。
- [ ] **Rust MCP baseline 资格**：Go SDK v1.7 的 initialize、tools/list、tools/call corpus、Origin/标准头校验和 persisted-listener apply 失败降级已闭合；剩余工作是完成 69 个 reviewed tool 的真实 production executor 覆盖，或形成经批准的缩窄 catalog，并为每个保留工具提供真实端口调用与 502/503 fail-closed 证据。
- [ ] **Runtime 状态投影真实化**：桌面 runtime readiness 为 `degraded`/`unavailable` 时必须 fail-closed，不能标记 ready 或展示主窗口；`system/status` 与 `system/storage/overview` 必须来自真实 lease/store 状态，禁止固定成功或 synthetic 空数组。

## Release 与终局放行项

- [ ] 在真实 Rust product/Tauri 入口上补齐 listener/process 证据，确认没有 Go API listener、代理或 sidecar；清理 closeout 中残留的历史 “Go remains owner/test-cutover” 叙述。
- [ ] 完成 macOS ARM64、Linux x64、Windows x64/ARM64 的 Tauri package、签名、安装、升级、卸载、回滚和 runtime smoke 矩阵；本机 smoke 不能替代原生 runner 证据。
- [ ] 在 release signing workflow 生成并验证真实签名 updater artifact/feed，验证升级前停止 Rust API、Pine、Python 子进程并可回退；同时归档上一版本签名安装包、updater metadata 和回退说明作为 rollback artifact。
- [ ] 完成独立 security review，覆盖 owner 边界、监听器、凭据、更新权限、恢复路径和桌面能力授权。
- [ ] 为实际发布产物生成 SBOM 与 provenance，并完成依赖/许可证/来源审计归档。
- [ ] 用上一版本真实数据副本完成 backup/restore、schema upgrade、损坏恢复、回滚和 retained worker crash recovery 演练。
- [ ] 发布后在四个平台执行固定 post-release smoke，并将结果写入 closeout manifest；所有高风险 quirk 必须先有处置结论。
- [ ] 只有上述开放证据全部可复现、回退有效且无双写后，才允许关闭 closeout，并同步最终发布证据、迁移文档、module map 和 Go/Wails 删除清单。

## 后续交接顺序

1. 先完成 MCP baseline 资格；在没有 Go SDK corpus 前不得扩展 method surface 或宣称 catalog 兼容。
2. 随后补齐 execution reconciliation 的行情/交易端到端证据。
3. 实现稳定后做一次只读全局审计，再按受影响范围运行验证；最终总门禁以仓库脚本的实际编排为准。
4. 最后审阅完整 diff，在 `main` 上做一次本地提交；不 push。

## 约束

不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite wire contract 或公开 `pkg/*` API；不接入 Go 路由、不做 Go fallback、不连接真实外部服务完成普通测试。任何内部 adapter 缺失都阻止 production 启动，外部依赖不可用则返回基线一致的 502/503。
