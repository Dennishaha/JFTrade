# JFTrade 活动路线图

更新时间：2026-10-08。

当前活动包括迁移行为核验、产品质量和发布资格工作。迁移历史与证据位于 `docs/history/go-to-rust`；历史阶段完成声明不能代替全部映射的逐项审查。

## 当前批次：ADK heartbeat 调用方快照与写入前拒绝

durable 工具批次已完成，当前 1661 exact、2152 partial、638 boundary。失败及通过证据见 [证据积压清单](history/go-to-rust/parity-evidence-backlog.md)。开发分支为 `codex/parity-assistant-builtin-config-20261008`，整体行为目标继续 active。

- [ ] 复核lease_boundaries14/108/133/144/161五条，直接验证过期caller snapshot在真实owner写入前拒绝；不能仅由durable row过期推断。
- [ ] heartbeat成功后更新worker snapshot，避免初始快照过期后错误取消仍有效owner；定时断言使用确定信号和显式过期。
- [ ] 定向 receipt、ordinary/strict、anchor/context、quick 计划与现场 quick、完整 Rust 门禁、diff/源码指纹复核后独立提交。

## 已验证批次：ADK durable 工具与 owner fence

- [x] 五条冻结execution_claims54/108/167/265/343复核，真实tool loop执行读取一次、恢复checkpoint复用同一durable输出；FAILED output原error.message及call投影保持，旧owner零工具执行，fresh foreign lease保持完整run及lease，stale/cross-run CAS拒绝且两row不变。
- [x] 265换直接runtime reconciliation证据且保持exact；167旧generic exact降partial。context key、Go durable COMPLETED/Rust FAILED和ErrRunLeaseLost/Rust None或false的返回差异保留；当前1661 exact、2152 partial、638 boundary。
- [x] 首轮3 passed/2 failed证据保留，最终定向10 passed；首次quick单个旧catalog LEAK保留，未改源码的20次stress及quick重跑均无LEAK。quick2379 Rust/98 Pine通过，现场完整Rust3971 passed、0 failed、2 skipped，static/七类replay通过并退出0。
- [x] 1017个Rust文件指纹不变；ordinary/strict/anchor/context、五条Go blob/receipt/reuse/diff复核后提交，继续heartbeat快照边界。

## 已验证批次：ADK shutdown 与租约释放

- [x] 五条冻结lease_boundaries176/234与continuation_boundaries314/347/406复核。实际SQLite写队列阻塞时，同步chat shutdown提前返回建立行为红；closing lease仍认领、关闭后provider注册漏取消各自建立红，live stream及后台join正向控制通过。
- [x] 同步chat加入现有supervisor，关闭先停止准入，lease重试入口拒绝closing/closed，取消registry在锁内传递永久停止状态。直接owner验证一秒内取消、释放阻塞前不返回、释放后owner清空与零新lease/run写入；既有五秒deadline保留任务和ports回归通过。
- [x] 234升exact，176改用真实durable owner证据，347完整原断言闭合；314旧closing-only exact降partial，406 nullable context继续boundary，净数量不变。首次test fixture E0382编译失败与2 passed/3 failed行为receipt均保留，修复后最终定向13 passed，无LEAK。
- [x] quick2374 Rust/98 Pine、desktop11+48通过；现场完整Rust3966 passed、0 failed、2 skipped，static/七类replay通过且退出0。1016个Rust文件运行期间指纹不变，ordinary/strict/anchor/context、receipt/reuse/diff复核后独立提交，继续durable工具批次。

## 已验证批次：ADK 审批并发与续跑租约

- [x] 五条冻结runner_approval_concurrency原测试与blob SHA复核。真实ProductionAdkPort、continuation supervisor与SQLite lease覆盖重复批准在工具阻塞时返回、并发sibling完成且每工具一次、local lease释放前零执行及预取消。
- [x] async sibling和local lease等待升exact；同步duplicate/sibling的旧domain-only exact纠正为partial，保留原同步返回时机及私有map差异。预取消由真实lease等待owner证明，净数量不变：1662 exact、2151 partial、638 boundary。
- [x] 错误取消分类与macOS accepted socket继承nonblocking的两份fixture失败receipt保留；后者包含SIGABRT/单个LEAK，修正后最终定向10 passed，无LEAK。未放宽原2秒完成或50ms禁止提前执行断言，50ms不驱动30秒租约过期。
- [x] quick2369 Rust/98 Pine passed，现场完整Rust3961 passed、0 failed、2 skipped，static/七类replay通过且退出0；1015个Rust文件从最终定向到完整门禁指纹不变。ordinary/strict/anchor/context和diff复核后提交，继续shutdown批次。

## 已验证批次：ADK heartbeat 与 provider 取消

- [x] 复核 runtime_execution_lease_boundaries 的14/108/133/144/161五条及 blob SHA。实际 heartbeat UPDATE 存储错误建立独立行为红；生产 provider 取消检查接入租约丢失，覆盖 sync、idle SSE、响应头及 JSON body 等待，旧 owner 不写迟到终态。
- [x] 默认TTL精确30秒、heartbeat10秒及近过期snapshot刷新TTL精确1秒闭合，两条升exact；context/reuse、写入前/无store拒绝和原短heartbeat配置/故障注入后1秒仍partial，当前1662 exact、2151 partial、638 boundary。
- [x] 预取消保留499和 `<200ms` 原断言，新增零provider连接断言；旧mock无界accept/join导致首次quick挂起，停止单个测试进程并保留失败receipt。两次编译故障、三份行为红及首次quick单个LEAK标记均保留，重跑未见LEAK。
- [x] 新机器最终定向22 passed，quick2365 Rust/98 Pine passed，完整Rust3957 passed、0 failed、2 skipped，static和七类replay通过，明确退出0；1014个Rust文件运行期间指纹不变，ordinary/strict/anchor/context通过，继续审批并发批次。

## 已验证批次：ADK 重连失败与取消

- [x] 复核 chat_transport_disconnect55/110、chat_helpers167/216、service_test49 和 runner_chat1124 六条冻结 Go 原测试及 blob SHA。
- [x] 真实 prepared router/ProductionAdkPort/Hyper socket 覆盖两路 retry/event 写失败精确一次退出、reader 回收和终态历史保留；listener 预取消两路空 body 保留 RUNNING。Go request-context 预取消差异继续 partial。
- [x] 直接生产模板 owner 在 chat runtime 缺失时无 error、templates 非 null，service49 升 exact；disconnect55 的旧证据未覆盖 malformed retry 和 absent-runtime 后台 error，纠正为 partial。五种终态重复迟到取消/失败保持完整 row、timestamps、stream history、session events 和 audit。
- [x] 新机器定向 15 passed，quick 2357 Rust/98 Pine passed，完整 `CARGO_INCREMENTAL=0 check:rust` 3949 passed、0 failed、2 skipped，明确退出 0；1013 个 Rust 文件指纹未变。ordinary/strict/anchor 通过，当前 1660 exact、2153 partial、638 boundary。三次 fixture 失败和 reuse 元数据失败日志均保留，继续下一批。

## 已验证批次：Pine tuple重复别名与公共helper诊断

Web Origin批次已提交`1150e2c6`，现场完整Rust为3934 passed、0 failed、2 skipped。Codex整体目标继续active。

- [x] 原MACD重复别名和有效宽度重复tuple建立独立红，允许underscore/唯一名称正向控制；semantic owner修复后11条定向通过。
- [x] 完整11个冻结公共helper输入与原错误断言闭合，parse54升级exact；其他四条按summary/private helper返回差异保留partial，净exact+1/partial−1。
- [x] ordinary/strict/anchor/context通过；现场quick2491 passed/0 failed/0 skipped与完整Rust3941 passed/0 failed/2 skipped均明确退出0。源码指纹、红绿SHA及五条mapping/十二处reuse范围已复核，提交后继续Assistant内置agent配置的真实HTTP红绿。

## 已验证批次：Web 重绑定与 Origin 生命周期

Workflow 调度时间与行情错误批次已提交 `8efe40d5`，现场完整 Rust 为3930 passed、0 failed、2 skipped。Codex整体目标继续active。

- [x] 真实HTTP登录红测为200而非403，WebSocket红测为101而非403；独立失败receipt为1 passed/2 failed，源码九份已保存。
- [x] 修复生产composition，使监听器端口授权由当前动态bind持有；新端口成功、旧Origin拒绝、冲突后原监听器/设置/会话保持的13条定向验证通过。
- [x] 逐项复核相关五条partial及旧exact lifecycle331；331原文不含Origin断言，补非nil生命周期表后仍缺nil/invalid helper，纠正为partial，净exact−1/partial+1。
- [x] 定向nextest13、现场quick2349与完整check:rust3934 passed/0 failed/2 skipped均明确退出0；源码指纹、红绿receipt及六条mapping/七处reuse范围已复核。缺少原行为的条目如实保留，提交后继续Pine重复别名与公共helper诊断。

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

## 后续行为批次：Pine wire 原始输入与 RPC owner（2026-10-07）

WebSocket批次已提交`8846e40d`。候选五条partial：pineworker `proto_mapping_test.go:154/197`及`runtime_boundaries_test.go:66/79/107`，冻结Go及validClientRequest已逐项读取。测试沿用已有编码、执行与readiness owner，新增显式取消、join及有界关闭的loopback gRPC fixture，保留生产endpoint/请求校验。

- [x] 对照原始负时间/float编码向量、health caps源数组变更、K线零/倒序时间、30s默认timeout/空jobId回填、Run与Health RPC同一错误传播，五项focused直接owner测试通过。
- [x] 原始编码向量与health caps复制两条升exact，净+2/-2；CPU worker defaults、indexed错误文案、Go nil/错误对象身份及unlimited默认值差异保持明确残余，旧Go79的mismatch误记纠正。
- [x] focused 5/5及ordinary/strict/anchor通过，quick 2265 passed/1 skipped与现场完整Rust 3765 passed/2 skipped明确退出0；源码指纹及diff复核后独立提交，继续Assistant生命周期五条候选。

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
