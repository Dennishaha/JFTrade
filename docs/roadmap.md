# JFTrade 活动路线图

更新时间：2026-08-31。

本文只记录当前仍未闭合、且需要继续投入的工作。已完成事项只保留在 [Go → Rust 迁移账本](architecture/go-to-rust-migration.md)、[Stage 9 closeout manifest](../tests/fixtures/rust-migration/stage9/closeout-evidence.json) 和 Git 历史中，不再进入活动清单或后续智能体任务。

## 当前放行边界

Stage 9 closeout manifest 仍为 `in_progress`。后续放行只处理 manifest 中状态为 `open` 或 `blocked` 的 gate；状态为 `passed` 的 gate 不再派工，也不得用历史本地检查结果替代发布证据。

## 运行时未闭合项

按依赖和共享文件冲突依次收口，完成前不再扩展新的 route group。

- [ ] **Rust MCP Streamable HTTP 资格收口**：以 Go SDK v1.7 client compatibility 和 HTTP corpus 确认 stateless method surface；若 baseline 仍为 POST-only，则 `GET`/`DELETE` 保持 `405 Allow: POST`，不得为满足旧清单扩展 SSE session。继续收口 `Accept`/`Content-Type`/`MCP-Protocol-Version`、Origin、initialize、敏感字段白名单、reviewed tool catalog、无 runtime fail-closed、失败回滚和逆序 teardown。
- [ ] **ADK durable runtime 收口**：workflow 在外部调用前写入 durable invocation 并以 CAS 收敛终态/恢复孤儿任务；Skill 下载消除 DNS rebinding TOCTOU 并补安全 ZIP 安装；compact 使用真实 context/handoff 语义；默认 Provider 切换必须保持事务性唯一。
- [ ] **Execution reconciliation 解耦收口**：保留 broker ID discovery、订单/成交/费用/历史恢复、未知 broker 状态显式错误和重启幂等；将交易 OpenD/login/trade-reader readiness 与行情 active provider 解耦，确保 yfinance/AKShare 行情模式下交易对账仍可运行或返回准确的 broker unavailable。

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

1. 先完成 MCP Streamable HTTP 资格和 ADK durable runtime；两者共享 `product_server`/composition root 时由集成方串行审阅和落地，并分别记录未解决 quirk。
2. 随后修复 Execution reconciliation 的行情/交易 provider 解耦；不得扩大到无关 route group。
3. 实现稳定后，由集成方做一次只读全局审计，再按受影响范围运行验证；最终总门禁以仓库脚本的实际编排为准。
4. 最后审阅完整 diff，在 `main` 上做一次本地提交；不 push。

## 约束

不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite wire contract 或公开 `pkg/*` API；不接入 Go 路由、不做 Go fallback、不连接真实外部服务完成普通测试。任何内部 adapter 缺失都阻止 production 启动，外部依赖不可用则返回基线一致的 502/503。
