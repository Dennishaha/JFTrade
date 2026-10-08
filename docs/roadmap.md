# JFTrade 活动路线图

更新时间：2026-10-09。

当前活动包括迁移行为核验、产品质量和发布资格工作。迁移历史与证据位于 `docs/history/go-to-rust`；历史阶段完成声明不能代替全部映射的逐项审查。

## 当前批次：ADK 确定性接管与 durable 工具完成

handler stable key、消费、UNKNOWN恢复及确定性takeover批次已验证；保留终态错误流在runtime配置恢复后继续复用，并优先检查durable身份冲突与读取失败。八条原测试复核纠正两条未闭合exact，当前1660 exact、2153 partial、638 boundary。下一批继续配置可用runtime的prepare失败、成功并发执行及预取消边界。失败及通过证据见 [证据积压清单](history/go-to-rust/parity-evidence-backlog.md)。开发分支为 `codex/parity-assistant-builtin-config-20261008`，整体行为目标继续 active。

- [x] 冻结execution_claims54/108/167/265/343五条及blob SHA复核；稳定context key经生产timeout worker交付handler，真实两次tool loop证明key非空、包含run-wrapper、first/second output.key及完整output相同、只执行一次。54升级exact，其余状态及错误分类残余不变。
- [x] 定向18 passed、quick2385 Rust/98 Pine与desktop Node11+48通过；现场完整Rust3980 passed、0 failed、2 skipped，无LEAK，static/七类replay退出0。外置卷断开及launcher原30秒超时失败留证，未改源码或放宽deadline；launcher原样20次与prediction路由20次重复通过。receipt/reuse/anchor/context/diff及1020个Rust文件冻结指纹复核后独立提交。
- [x] 214的keyed descriptor与handler实际读取观测已接入真实worker，框架非空检查不计消费；定向21通过。首轮UNKNOWN ledger、FAILED call/SUBMISSION_UNKNOWN、Run COMPLETED/degraded，第二轮UNKNOWN row保持、handler计数1。原第二次tool.Run返回与既有Unknown-claim Run终态差异继续partial，当前1661/2152/638。
- [x] 消费批次定向21、quick2388 Rust/98 Pine与desktop Node11+48通过；现场完整Rust3983 passed、0 failed、2 skipped，无LEAK。1021个Rust文件冻结指纹与receipt/reuse/anchor/context/diff复核后独立提交，夹具错误与编译失败保留。
- [x] 已映射UNKNOWN checkpoint沿同一SQLite事务及当前owner恢复FAILED callback并继续模型；两次loop、takeover、旧owner/晚到结果拒绝和journal回滚定向26通过，原ledger/输出/事件及计数1保持。214第二次tool.Run typed返回、Go空输出表示及普通expiry UNKNOWN恢复仍partial。
- [x] 本批quick2597 Rust/98 Pine及desktop Node11+48通过；现场完整Rust3985 passed、0 failed、2 skipped，static/七类replay退出0，无LEAK。1021个Rust源码冻结指纹及receipt/anchor/reuse/diff已复核，数量保持1661/2152/638。
- [x] stale及SQL identity fence的生产claim owner返回ADK_RUN_LEASE_LOST，保留loop协调停止、零执行及新owner状态；current执行及precancellation控制通过。定向63 passed，167完整Go tool.Run返回边界保持partial。
- [x] 本批quick2392 Rust/98 Pine及desktop Node11+48通过；现场完整Rust3987 passed、0 failed、2 skipped，static/七类replay退出0，无LEAK。1021个Rust冻结源码与receipt/anchor/reuse/diff复核，数量保持1661/2152/638。
- [x] 294真实worker按3秒TTL/100ms续租，原2秒窗口及initial expiry+1ms接管拒绝闭合；无心跳控制证明同一时点可接管，停止/join后token>1。定向44通过，294恢复exact，当前1662/2151/638。
- [x] 首次完整Rust失败暴露Pine退出在STOPPED/通知后才释放订阅；生产owner修复为先释放自身消费者。通知时点探针红测两处失败，修复后心跳及清理定向68通过。十条原测试复核将未覆盖broker panic/活动汇总和停止态全量零写入的176/52纠正为partial，净exact−1，当前1660/2153/638。
- [x] 修复后定向68及store3通过；重新查看quick计划并执行2600 Rust/98 Pine及desktop Node11+48通过。现场完整Rust3988 passed、0 failed、2 skipped，static/七类replay退出0，无LEAK；1021个Rust源码冻结指纹、十条receipt/reuse/anchor/diff复核，原1445 passed/1 failed的中断门禁及稳定红测保留。
- [x] takeover测试文件的短租约与过期sleep已换同一fixture事务的精确过期；原23个断言保持，新增3个fixture控制。定向36、quick2393 Rust/98 Pine及现场完整Rust3988 passed、0 failed、2 skipped通过，static/七类replay退出0，无LEAK。1021个Rust文件冻结，六条原测试/blob/receipt/reuse/anchor复核，1660/2153/638保持。
- [x] 普通工具执行记录COMPLETED与SUCCEEDED/FAILED call投影分离，108原invalid输入、false输出、COMPLETED ledger与FAILED call/error闭合，升级exact。旧SUCCEEDED/FAILED整行、output与event保持，异输出不替换winner；journal回滚、stale fence及UNKNOWN回归通过。七条原测试复核，当前1661/2152/638。
- [x] 原状态断言红测0 passed/1 failed保留；最终定向47、quick2601 Rust/98 Pine与desktop Node11+48通过。现场完整Rust3989 passed/0 failed/2 skipped，static/七类replay退出0，无LEAK；1022个Rust源码冻结，receipt/reuse/anchor/diff复核后独立提交。
- [x] 真实ProductionAdkPort POST malformed输入首次retry写失败为200 SSE、一次BrokenPipe、零event、reader释放且无run写入；五条原测试复核，55缺runtime有效请求后台终态仍partial。fixture未注册POST的24 passed/1 failed保留，注册既有生产port后定向25通过。
- [x] 本批quick2395 Rust/98 Pine与现场完整Rust3990 passed/0 failed/2 skipped通过，static/七类replay退出0，无LEAK；1022个Rust文件冻结，receipt/reuse/anchor/diff复核，1661/2152/638保持。独立desktop Node检查未被本批planner选择，不计通过。
- [x] fail-closed写工具执行错误经原事务UNKNOWN/SUBMISSION_UNKNOWN，二次loop保持原output/ledger/events与计数1，迟到成功拒绝；结构化拒绝仍COMPLETED/FAILED。quick暴露workflow.wait名称分类误判，生产owner修复为优先descriptor显式模式/缺省read权限，保留原workflow文本/errorCode断言；最终定向77通过。
- [x] 七条原测试及Go生产错误分支复核，分类不变。最终quick2398 Rust/98 Pine与desktop Node11+48通过；现场完整Rust重跑3993 passed/0 failed/2 skipped，无LEAK，static/七类replay退出0；1023个Rust文件冻结。初次行为红、quick68 passed/1 failed、首次完整3993 passed含1 LEAK均留证；LEAK原样20次未复现，未声称修复。
- [ ] 缺runtime后台终态、request-context预取消、UNKNOWN返回与已明确的策略panic/停止态边界继续推进；完整运行的未复现LEAK风险仍保留。GitHub登录已可用，提交复核后同步当前分支。
- [ ] 每批继续核对5–10条原测试，定向nextest、quick及现场完整Rust门禁通过后复核并提交；整体对齐未完成不标complete。
- [x] keyed 模式去除空白和忽略大小写后仍要求 handler 消费 key；真实 loop 红测确认原 COMPLETED 错判，修复 catalog 后定向78通过。两种规范化变体的消费/未消费控制及二次恢复保持原 output/ledger/events、执行次数1。
- [x] keyed 规范化批次 quick2399 Rust/98 Pine/desktop Node11+48与现场完整Rust3994 passed/0 failed/2 skipped通过，无LEAK，static/七类replay退出0；1023个Rust文件冻结。七条原测试分类保持，完整 typed tool.Run 返回和空 UNKNOWN 输出缺口不由规范化控制推断闭合。
- [x] 完整stream worker在已非RUNNING时复用保留事件、零模型连接。六种停止状态的open/disconnected body控制保持完整run/session events/audit，预取消/body断连只记录一次取消终态并释放登记与lease；RUNNING正向控制实际请求模型并持久化RUN_TIMED_OUT。稳定生产红0/1、最终定向27通过，无LEAK；七条原测试分类保持。
- [x] stream终态批次最终quick2402 Rust/98 Pine/desktop Node11+48→现场完整Rust3997 passed/0 failed/2 skipped通过，无LEAK，static/七类replay退出0；1024个Rust文件冻结。三次定向运行的既有重连LEAK、quick既有MCP LEAK、夹具nested runtime/错误码层级和Clippy失败全部留证，未声称LEAK修复；初次完整Rust通过结果也保留。缺runtime后台终态和idle request-context预取消仍partial。
- [x] 缺runtime首次retry BrokenPipe的有效请求现在由生产port保留非空error终态，分配streamId、同序列重连和零run/audit写入闭合。健康HTTP初始/重复POST/GET replay、after过滤、冲突409、同步chat不可用及确定性过期控制通过；原disconnect55两子分支升级exact，其余八条分类保持。生产红0/1和遗漏trait的编译失败分别留证，最终定向31通过，无LEAK。
- [x] 两个装配用例的旧stream503预期已按冻结Go改为200终态error并增强identity/sequence/零写入断言；其quick失败、enum导入编译失败与第二次quick Clippy失败分别留证。合并reader条件后最终定向33通过，无LEAK；最终quick2405 Rust/98 Pine/desktop Node11+48→现场完整Rust4000 passed/0 failed/2 skipped通过，无LEAK，static/七类replay退出0。
- [x] 本批Rust1025文件冻结与四份receipt、六份gate、九条Go blob/anchor和20处reuse/diff复核通过，现场证据及源码已独立归档。继续idle request-context零body、canonical请求指纹和private delta/context组合缺口，历史LEAK不由本批clean通过推断修复。

## 已验证批次：ADK checkpoint replay 与唯一投影 owner

- [x] execution_claims54/108/167/265/294/343六条原测试及blob SHA复核。最初single-use mock provider在第二次调用不可用，失败/诊断证据按夹具分类保留；有效第二个provider与原生产实现独立重建0 passed/2 failed红，精确缺失toolResults output。
- [x] 已映射terminal invocation output不再二次映射或替换失败文本；同一SQLite事务按当前Run lease/revision恢复checkpoint投影及journal，原invocation完全不变。stale owner拒绝且run/events不变，current takeover恢复；journal真实故障回滚全部状态，正向控制恢复成功且零重复执行/事件。
- [x] 294旧domain-only heartbeat exact纠正partial；默认真实worker不足以证明3秒TTL/100ms heartbeat及2秒/合成时点原断言。handler context key、COMPLETED/FAILED和tool.Run/SaveRun错误分类继续partial，当前1660/2153/638。
- [x] 参数E0061编译失败留证，最终定向15 passed；quick2591 Rust/98 Pine、desktop11+48通过，现场完整Rust3979 passed、0 failed、2 skipped，static/七类replay通过且exit0，无LEAK。1020个Rust文件冻结指纹一致，ordinary/strict/anchor/context、receipt/reuse/diff复核后独立提交并继续handler key。

## 已验证批次：ADK heartbeat 调用方快照与写入前拒绝

- [x] 五条冻结lease_boundaries14/108/133/144/161及blob SHA复核。真实SQLite写故障证明过期caller snapshot先前访问了writer；新增拒绝后旧worker初始snapshot在约30秒到期形成第二个独立红，两份失败receipt保留。
- [x] Store在连接访问前按唯一now拒绝snapshot过期；worker成功后采用返回的新snapshot。有效durable row、原run不变，sparse expired snapshot对不可访问connection返回LeaseLost，expiry相等边界拒绝；生产默认worker超越初始到期仍同owner/fence、TTL30秒，Drop释放并join。
- [x] 近过期原5秒durable/100ms caller输入和精确1秒TTL移至同一生产实现的确定now，消除短窗口竞态。144/161保持exact，14/108/133的nullable/context/短heartbeat时间分支保留partial，数量1661/2152/638不变。
- [x] 缺run夹具失败9 passed/3 failed保留；补齐run后最终定向12 passed。quick2587 Rust/98 Pine、desktop11+48通过；现场完整Rust3975 passed、0 failed、2 skipped，static/七类replay通过且exit0，无LEAK。1019个Rust文件指纹不变，ordinary/strict/anchor/context、receipt/reuse/diff复核后提交并继续replay批次。

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
