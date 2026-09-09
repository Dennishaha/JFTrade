# 发布资格

更新时间：2026-09-03。

源码通过质量门禁不等于具备发布资格。JFTrade 将发布治理分为 source admission、candidate evidence、publish 和 post-release validation 四个独立边界。

1. `release-source-admission` 绑定精确 branch ref 和 commit SHA，要求同一 SHA 的 `Build & Test` 成功，并重新验证 zero-Go、278 路由契约和版本配置。该 receipt 固定 `releaseQualified=false`，不能授权 tag 或发布。
2. 正式 candidate evidence 绑定四平台签名/公证/updater、安装升级回滚、SBOM/provenance、备份恢复和独立安全签字。只有 `candidate_ready` 可授权计划 tag 指向同一 commit。
3. `publish` 只消费同 SHA 的 sealed candidate artifact，不重新构建；unsigned rehearsal artifact 永远不能用于正式发布。
4. 发布后生成独立 `post-release-validation` receipt，不修改源码树或 candidate receipt。

滚动升级基线记录在 `tests/fixtures/release/upgrade-baselines.json`。`0.29.0` 使用线上原样发布的 `v0.27.0` 安装包和 checksum；禁止从历史源码重建基线。

## 操作入口与授权

[Desktop Release workflow](../../.github/workflows/desktop-release.yml) 通过 `workflow_dispatch` 显式选择 `rehearsal`、`candidate` 或 `publish`，不由推送 tag 自动触发。`publish` 要求已有 tag 指向经验证的同一 SHA，并提供正式 qualification run/artifact；不能以重新构建或 unsigned artifact 替代 sealed candidate。

unsigned rehearsal 只证明构建演练，通过时使用独立 `rehearsal_passed` receipt，始终 `releaseQualified=false`；不读取签名 secret，不创建 tag、Release 或 updater feed。

普通开发、文档整理或本地门禁不隐含创建候选分支、tag、Release 或触发 rehearsal 的授权。执行这些操作前必须有明确任务要求；本地构建方法见 [桌面发布文档](../troubleshooting/desktop-release.md)，未完成事项见 [roadmap](../roadmap.md)。
