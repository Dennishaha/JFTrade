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

## 下一行为批次：策略 HTTP 失败与实例删除副作用（2026-10-07）

插件 HTTP/file owner批次已收口，修复畸形字段和根marker失败后工件被创建/删除的问题，净exact +4/partial -4；现场quick与完整Rust门禁通过。结果、残余和失败关系见 [迁移成果摘要](history/go-to-rust/parity-progress-summary.md) 与 [证据积压清单](history/go-to-rust/parity-evidence-backlog.md)。下一批选择五条partial：strategy API routes_failure_boundaries_test 的71/138/184，以及routes_lifecycle_test的252/401。

- [x] Go原始definition list/read/create/update/delete错误路径重放HTTP500/STRATEGY_FAILED，畸形JSON400、零owner调用，失败create/update仍在owner前归一ID/name；rehearsal seam及production分类残余已说明。
- [x] 原始SMA20、US.AAPL/5m预热在真实production HTTP与SQLite owner上直接断言derivedWarmupBars20。
- [x] 原始history/snapshot故障在HTTP500中保留分类及消息；固定savedAt、直接handler缺URI400与production分类残余保留。
- [x] 四条instance mutation失败HTTP矩阵逐项核对入参；显式rehearsal seam不能证明生产CAS/store分类或回滚，残余保留。
- [x] 实例missing404、busy400、成功删除200后同一inst-1被durable软删除，busy实例保持状态；在唯一writer启动前建临时fixture，shutdown后重开检查。
- [x] 闭合实例删除一条 partial；canonical receipt、普通/strict 与 anchor 审计、quick、完整 Rust 门禁已通过，diff review后独立提交并继续；其余production故障分类、fixed fixture和handler缺参残余保留。

## 下一行为批次：交易读参数与订单回执（2026-10-07）

选择五条partial：trading execution_test.go的110/249、routes_read_handlers_test.go的71、routes_broker_contracts_test.go的143、routes_helper_boundaries_test.go的17。逐项核对冻结Go原测试，现有mapping不能替代原始断言。

- [x] 在真实production HTTP和SQLite ledger上重放PARTIALLY_FILLED/FILLED_PART与missing404 ORDER_NOT_FOUND，shutdown/restart后保持。
- [x] 重放原始空白scope/brokerId/accountId及小写market；同一ledger包含REAL/SIMULATE、其它账户/券商/市场及terminal对照，修复省略环境未读取live default getter，设置变化和显式override已验证。原Go110没有current/history调用计数断言，不沿用旧误记。
- [x] 费用ID合并去重、margin symbol归一、cash-flow方向/日期与quote/securities两工具合并走concrete adapter，记录原生reader实际入参；Go acc-1与Futu数字账户残余单独保留。
- [x] portfolio cash/position原字段与missing broker404直接断言；修复源US被请求默认HK覆盖及缺required wire字段。typed账户形状和generic helper Page/error/Retry-After差异继续保持partial。
- [x] 保留四份红测，修复省略环境、源交易市场、portfolio required字段与缺成本number回退；canonical六项通过，普通/strict及anchor、quick2201/0/0、现场完整Rust3749/0/2通过，diff review后独立提交再继续。仅两条升exact，其余三条残余保留。

## 后续行为批次：设置失败与 durable 状态（2026-10-07）

候选五条partial：settings `routes_accounts_validation_test.go:18`、`routes_failure_boundaries_test.go:48/84/125`、`routes_test.go:935`。冻结Go原始请求及断言已逐项读取；交易读批次已提交9812de90，本批五项production HTTP测试已落入源码并focused通过。

- [x] 真实production HTTP执行十一条原始请求；十条500 SETTINGS_SAVE_FAILED，pine-worker {}因现有required绑定400，另补合法完整字段请求500。每次请求后的所有设置回读和原文件字节不变；该条Pine反例仍partial。
- [x] managed account原始record-1/client-id冲突请求验证路径ID优先；实际文件写失败后保持原账户，shutdown/restart验证成功状态仍在，该条升exact。
- [x] onboarding原始completed/dismissed=false和空白lastBrokerId重置请求清除两个时间戳、保留futu，落盘与重启后回读一致，该条升exact。
- [x] 通知设置原始important/trading与off/system请求直接核对归一结果和落盘；Rust important模式扩展默认类别及off归一的差异保持partial。
- [x] 通知测试第三次事件system-notification-3、delivered及host入参直接核对；缺host当前503 SYSTEM_NOTIFICATION_UNAVAILABLE与Go500残余明确保留partial。
- [x] 本批canonical receipt、普通/strict与anchor通过；quick 2206 passed及现场完整Rust 3754 passed/2 skipped明确退出0，首轮target-health失败与清理证据保留，源码指纹与diff已复核，独立提交后继续下一批。

## 后续行为批次：production WebSocket 与通知重放（2026-10-07）

设置批次已提交`bb80a578`。候选五条partial：live `handler_test.go:207/244/316/421`及lifecycle `lifecycle_test.go:243`，冻结Go逐项读取。缺provider原测试只要求连接结束，不要求1006；同源测试只要求server.URL Origin成功握手和读帧。当前生产Web listener动态同源策略须用实际Cookie/Origin验证。

- [x] 五项production transport测试先跑红测4/1；保留depth初始snapshot/num/entityId和原nonloopback bind/Web root差异。缺provider关闭、真实Web listener同源和通知seq1重放三条闭合，净exact +3/partial -3。
- [x] 沿用现有LiveHub有界保留通知wire帧，连接快照与发送线性化；重试去重保持无订阅者false，具体SQLite projector无client cursor=0/重试推进/只收到一次验证通过。canonical八项测试通过，原失败receipt保留。
- [x] focused receipt 8/8、逐项mapping、ordinary/strict/anchor通过；quick 2312 passed及现场完整Rust 3760 passed/2 skipped均明确退出0，target-health首次失败及清理证据保留，源码指纹与diff复核后独立提交，继续Pine wire五条候选。

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
