# JFTrade 活动路线图

更新时间：2026-08-31。

本文只记录当前仍未闭合、且需要继续投入的工作。已完成事项只保留在 [Go → Rust 迁移账本](architecture/go-to-rust-migration.md)、[Stage 9 closeout manifest](../tests/fixtures/rust-migration/stage9/closeout-evidence.json) 和 Git 历史中，不再进入活动清单或后续智能体任务。

## 当前放行边界

Stage 9 closeout manifest 仍为 `in_progress`。后续放行只处理 manifest 中状态为 `open` 或 `blocked` 的 gate；状态为 `passed` 的 gate 不再派工，也不得用历史本地检查结果替代发布证据。

发布治理分为两个明确阶段：构建前由 `node scripts/rust-migration/check-stage9-closeout.mjs --candidate-static` 只校验动态路由账本与唯一 owner；四平台产物汇总后再由独立 `check-release-candidate.mjs` 将真实 manifest、artifact、SHA256SUMS 与同一 workflow run/ref 的前置证据绑定。发布完成后，独立 post-release workflow 从更新后的 evidence ref 运行默认 `--check` 完整 closeout。任一 candidate 通过都不代表 release/closeout 已完成，也不证明 post-release smoke、hard-cut 或独立安全签字。

## 运行时未闭合项

当前没有仍需派工的 Rust production route、adapter、MCP baseline 或 execution reconciliation 实现门禁。后续智能体不得把已通过的 `allRouteGroups`、`uniqueWriteOwner`、MCP baseline、execution reconciliation、storage lease、Tauri readiness 或 `system/status` 投影重新列入待办；只有当前 gate 重新失败、或出现新的运行时回归证据时，才在本节重新登记。

## Release 与终局放行项

以下只对应 `tests/fixtures/rust-migration/stage9/closeout-evidence.json` 中仍为 `open` 的 gate；状态为 `passed` 的 gate 不再列入活动路线图。

- [ ] **platformRelease**：完成 macOS ARM64、Linux x64、Windows x64/ARM64 的 Tauri package、签名、安装、升级、卸载、回滚和 runtime smoke 矩阵；本机 smoke 不能替代原生 runner 证据。
- [ ] **signedUpdaterArtifact**：在 release signing workflow 生成并验证真实签名 updater artifact/feed，验证升级前停止 Rust API、Pine、Python 子进程并可回退。
- [ ] **rollbackArtifact**：归档并验证上一版本签名安装包、updater metadata 和回退说明。
- [ ] **securityReview**：完成独立 security review，覆盖 owner 边界、监听器、凭据、更新权限、恢复路径和桌面能力授权。
- [ ] **sbom**：为实际发布产物生成 SBOM 与 provenance，并完成依赖/许可证/来源审计归档。
- [ ] **backupRestoreDrill**：用上一版本真实数据副本完成 backup/restore、schema upgrade、损坏恢复、回滚和 retained worker crash recovery 演练。
- [ ] **postReleaseSmoke**：发布后在四个平台执行固定 post-release smoke，并将结果写入 closeout manifest；所有高风险 quirk 必须先有处置结论。
- [ ] **hardCutReadiness**：上述开放证据全部可复现、回退有效且无双写后，才允许关闭 closeout，并同步最终发布证据、迁移文档、module map 和 Go/Wails 删除清单。

## 后续交接顺序

1. 先运行静态 admission，再按 `platformRelease` 补齐四平台 Tauri release 矩阵，记录每个平台 package/sign/install/upgrade/uninstall/rollback/runtime smoke 证据。
2. 在四平台 artifact 汇总后生成并校验独立 candidate evidence；并行准备 `signedUpdaterArtifact`、`rollbackArtifact`、`sbom` 和 `securityReview` 的同 ref/commit 外部输入材料。qualification workflow 只接受显式成功 evidence run/artifact 与 `release-evidence-inputs.v1` manifest，绝不把本地 checker 输出、命令文本或 external-required 占位值当作通过；真实签名环境或原生平台缺失时必须 fail-closed。
3. release candidate evidence 与 updater/rollback 证据齐全后，再执行 `backupRestoreDrill` 并保留其外部证据引用。
4. 正式发布后由独立 workflow 从 post-release evidence ref 执行 `postReleaseSmoke` 和默认 `--check`，最后复核 `hardCutReadiness` 并关闭 closeout。

## 约束

不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite wire contract 或公开 `pkg/*` API；不接入 Go 路由、不做 Go fallback、不连接真实外部服务完成普通测试。任何内部 adapter 缺失都阻止 production 启动，外部依赖不可用则返回基线一致的 502/503。
