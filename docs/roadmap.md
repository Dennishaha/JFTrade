# JFTrade 活动路线图

更新时间：2026-09-03。

迁移实现已经完成，活动路线图只记录当前产品和发布资格工作。历史迁移记录位于 `docs/history/go-to-rust`，不参与当前状态计算。

## 质量门禁

- [ ] 通过普通 PR 验证 affected fail-closed 计划和唯一 required context `Build & Test`。
- [ ] 合入后由 `main` CI 验证 Policy、Contracts、Rust Static、Rust Tests + Compatibility、Web、Pine、Python 和 Desktop 完整计划。
- [ ] 收集至少三次可比 CI 墙钟，目标 PR 核心中位数约 20–30 分钟；性能目标不得降低正确性门槛。

## 0.29.0 发布资格

- [ ] 四平台签名 package、安装、首次启动、升级、卸载、回滚和 runtime smoke。
- [ ] 真实签名 updater artifact/feed 与升级前子进程停止、失败回退。
- [ ] 使用线上 `v0.27.0` 原始安装包和 checksum 完成 SQLite 升级、备份恢复与损坏恢复。
- [ ] 为最终产物生成 SBOM/provenance，并完成依赖、许可证和来源审计。
- [ ] 完成独立 security review/sign-off。
- [ ] 正式 candidate receipt 达到 `candidate_ready` 后，计划 tag 才能指向同一 commit；publish 只消费 sealed candidate artifact。
- [ ] 发布后归档四平台 `post-release-validation` receipt。

Unsigned rehearsal 始终 `releaseQualified=false`，不能授权 tag、Release 或 updater feed。当前门禁重构本身不启动任何 release workflow。
