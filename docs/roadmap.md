# JFTrade 活动路线图

更新时间：2026-10-07。

当前活动包括迁移行为核验、产品质量和发布资格工作。迁移历史与证据位于 `docs/history/go-to-rust`；历史阶段完成声明不能代替全部映射的逐项审查。

## 下一轮：Go → Rust 全量行为审查

整体目标是对冻结基线的全部 4451 条映射完成断言、生产 owner、可执行测试、锚点、reuse 和真实 receipt 审查。现有 exact 严格审计通过不代表全量完成；partial 和 boundary 均需逐项证据，历史结论不能直接改标为 reviewed。

- [x] 撤回 49 条未经逐项审查的批量 reviewed 标签，保留实际测试 receipt；其中两条 cache 行经原始断言核对后记录准确 partial。
- [x] 修复 8 处退役资产路径注释造成的零 Go 门禁失败；清理超限 Rust 中间构建产物后重跑 quick 通过。
- [ ] 缓存：补齐快照重复观察守卫、容量顺序与 freshness 回归；逐项处理保留期、Source promotion 和 trade 字段继承差距。
- [ ] MarketData：逐项复核剩余历史映射及 resolver 的 Go 分支，确认订阅清理 deadline、四态 health 与 stale generation 证据。
- [ ] Broker/Trading、API transport、Assistant：按真实未覆盖断言推进生产 owner 和失败回归，纠正失效引用与过宽 reuse。
- [ ] 逐批执行最窄测试、实际 receipt、审计及适用 quick/Rust 门禁，复查 diff 后提交；失败和未执行项保留证据。
- [ ] 全部映射均有逐项审查依据、可实现行为差距完成修复、架构边界说明完整且所需门禁通过后，才确认整体完成。

## 下一行为批次：research screen HTTP 与执行定义（2026-10-07）

审批 durable boundaries 与 runtime sibling 拒绝批次已收口；结果、残余和红绿证据见 [迁移成果摘要](history/go-to-rust/parity-progress-summary.md) 与 [证据积压清单](history/go-to-rust/parity-evidence-backlog.md)。下一批选择五条 research screen partial，逐项重放原始 HTTP/definition 输入，先定位 parser、catalog 和 provider port。

- [ ] 真实 production HTTP catalog 核对 display semantics、providerId 不泄露及 market=SG 的 400。
- [ ] 使用原始 brokerId=" FUTU "/market="us" 输入，断言 provider port 收到 futu/US/默认 limit50；version1 拒绝且零调用。
- [ ] HTTP POST 经 recording provider port 核对同一归一执行定义、offset50/limit25；Go FeatureQuery operation/cursor 的结构差异明确保留。
- [ ] 原始 V2 interval 的 min/max/inclusion、column identity/label 与响应 metadata 逐字段核对。
- [ ] querySchemaVersion2 + legacy filters/columns 的原始 V1 形状返回 400，provider 调用数为零。
- [ ] 至少闭合一条 partial；canonical receipt、普通/strict 与 anchor 审计、quick、完整 Rust 门禁、diff review、独立提交后继续。

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
