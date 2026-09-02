# JFTrade 活动路线图

更新时间：2026-09-02。

本文只记录当前仍未闭合、且需要继续投入的工作。已完成事项只保留在 [Go → Rust 迁移账本](architecture/go-to-rust-migration.md)、[Stage 9 closeout manifest](../tests/fixtures/rust-migration/stage9/closeout-evidence.json) 和 Git 历史中，不再进入活动清单或后续智能体任务。

## 当前放行边界

Stage 9 closeout manifest 仍为 `in_progress`。后续放行只处理 manifest 中状态为 `open` 或 `blocked` 的 gate；状态为 `passed` 的 gate 不再派工，也不得用历史本地检查结果替代发布证据。

发布治理分为两个明确阶段：构建前由 `node scripts/rust-migration/check-stage9-closeout.mjs --candidate-static` 只校验动态路由账本与唯一 owner；四平台签名候选产物汇总后再由独立 `check-release-candidate.mjs` 将真实 manifest、artifact、SHA256SUMS 与同一候选分支 commit 的前置证据绑定。只有 `candidate_ready` 才允许让计划 tag 指向该同一 commit；publish 只消费该 qualification run 的 sealed artifact，不重新构建。发布完成后，独立 post-release workflow 从更新后的 evidence ref 运行默认 `--check` 完整 closeout。unsigned `rehearsal_passed` 永远不授权 tag 或 publish，也不关闭任何正式 gate。

## 运行时未闭合项

HTTP production route、唯一写 owner、execution reconciliation、storage lease、Tauri readiness、`system/status` 投影和零 Go 源码/入口删除没有仍需派工的实现门禁。MCP 已完成 transport、69 个工具名称的 catalog baseline 和 native production executor：当前 69 个工具均具备 native executor，0 个仍结构性 `fail-closed`；其中 `strategy.pine_spec` 和 `strategy.validate_pine` 在进程内执行已审阅的 native Pine 子集，不等同于完整 Pine v6 runtime，provider/research 依赖在外部 runtime 或 typed reader 不可用时仍按契约返回 `unavailable`。`tools/list` 的 69 个逐工具 input schema 已通过历史 fixture 的 required/enum/bounds/`additionalProperties` 全量 deep-equality Rust replay。后续实现必须继续保持真实 adapter 或 external-unavailable 语义，不得用 generic schema、fixture 成功或文档声明掩盖真实能力边界。已通过的 `allRouteGroups`、`uniqueWriteOwner`、`ownerDeletion`、execution reconciliation、storage lease、Tauri readiness 和 `system/status` 门禁只有在出现新的运行时回归证据时才重新登记。

`0.29.0` 是计划中的首个零 Go 版本。升级资格使用线上原样发布的 `v0.27.0` 安装包和官方 checksum；不新增 Go 补丁版、不重建基线、不重新生成最终 corpus。

## Release 与终局放行项

以下只对应 `tests/fixtures/rust-migration/stage9/closeout-evidence.json` 中仍为 `open` 的 gate；状态为 `passed` 的 gate 不再列入活动路线图。

- [ ] **platformRelease**：完成 macOS ARM64、Linux x64、Windows x64/ARM64 的 Tauri package、签名、安装、升级、卸载、回滚和 runtime smoke 矩阵；本机 smoke 不能替代原生 runner 证据。
- [ ] **signedUpdaterArtifact**：在 release signing workflow 生成并验证真实签名 updater artifact/feed，验证升级前停止 Rust API、Pine、Python 子进程并可回退。
- [ ] **rollbackArtifact**：归档并验证线上 `v0.27.0` 原始安装包/checksum、`0.29.0` updater metadata、升级前备份和回退说明；`v0.27.0` 没有发布签名，不能伪造签名证据。
- [ ] **securityReview**：完成独立 security review，覆盖 owner 边界、监听器、凭据、更新权限、恢复路径和桌面能力授权。
- [ ] **sbom**：为实际发布产物生成 SBOM 与 provenance，并完成依赖/许可证/来源审计归档。
- [ ] **backupRestoreDrill**：用上一版本真实数据副本完成 backup/restore、schema upgrade、损坏恢复、回滚和 retained worker crash recovery 演练。
- [ ] **postReleaseSmoke**：发布后在四个平台执行固定 post-release smoke，并将结果写入 closeout manifest；所有高风险 quirk 必须先有处置结论。
- [ ] **hardCutReadiness**：正式发布后的 smoke、回退和全部开放证据可复现且无双写后关闭 closeout，并同步最终发布证据、迁移文档和 module map；它不再作为创建 tag 的前置条件，tag 的唯一前置资格是绑定同一 commit 的正式 `candidate_ready`。

## 后续交接顺序

1. 可先在固定 `release/0.29.0-candidate` commit 上运行 unsigned rehearsal。`desktop-release-evidence-source.yml` 使用受保护的 `release-evidence` environment，在四个原生 runner 下载 immutable candidate artifact 与线上 `v0.27.0` 原始安装包，执行 install/first-start/upgrade/9-DB/runtime/uninstall/backup-restore/rollback/zero-Go；intake → payload → evidence → qualification 只归档 `rehearsal_passed`，签名、notarization、updater signature 与独立 security sign-off 固定为 `not_run/open`。
2. 正式候选运行静态 admission 和完整签名四平台矩阵，并准备 `signedUpdaterArtifact`、`rollbackArtifact`、`sbom`、`backupRestoreDrill` 与独立 `securityReview` 的同 ref/commit 输入。四条 evidence workflow 统一使用 `qualification_mode`、`candidate_ref`、`planned_release_tag`，逐层校验 run/ref/SHA、immutable artifact ID/digest 与报告字节；formal 与 rehearsal artifact 名称和 checker 互不兼容。
3. 只有 formal qualification 产生 `candidate_ready` 后，才创建指向同一 commit 的计划 tag；publish 必须显式消费该 qualification run 的 `desktop-release-candidate-evidence` sealed artifact，不接受 rehearsal receipt，也不重新构建。
4. 正式发布后由独立 workflow 从 post-release evidence ref 执行 `postReleaseSmoke` 和默认 `--check`，最后复核 `hardCutReadiness` 并关闭 closeout。

## 约束

不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite wire contract 或 worker contract；不恢复 Go 路由/工具链、不做 Go fallback、不连接真实外部服务完成普通测试。任何内部 adapter 缺失都阻止 production 启动，外部依赖不可用则返回基线一致的 502/503。
