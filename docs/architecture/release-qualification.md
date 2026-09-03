# 发布资格

更新时间：2026-09-03。

源码通过质量门禁不等于具备发布资格。JFTrade 将发布治理分为 source admission、candidate evidence、publish 和 post-release validation 四个独立边界。

1. `release-source-admission` 绑定精确 branch ref 和 commit SHA，要求同一 SHA 的 `Build & Test` 成功，并重新验证 zero-Go、278 路由契约和版本配置。该 receipt 固定 `releaseQualified=false`，不能授权 tag 或发布。
2. 正式 candidate evidence 绑定四平台签名/公证/updater、安装升级回滚、SBOM/provenance、备份恢复和独立安全签字。只有 `candidate_ready` 可授权计划 tag 指向同一 commit。
3. `publish` 只消费同 SHA 的 sealed candidate artifact，不重新构建；unsigned rehearsal artifact 永远不能用于正式发布。
4. 发布后生成独立 `post-release-validation` receipt，不修改源码树或 candidate receipt。

滚动升级基线记录在 `tests/fixtures/release/upgrade-baselines.json`。`0.29.0` 使用线上原样发布的 `v0.27.0` 安装包和 checksum；禁止从历史源码重建基线。

本轮门禁重构不创建候选分支、版本 tag、GitHub Release 或 rehearsal。
