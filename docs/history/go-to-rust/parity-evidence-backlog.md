# Go → Rust 证据积压清单

## 2026-10-08 Settings owner 与 MCP token HTTP

- 逐项复核五条mapping及必要reuse，净 **+4 exact / -4 partial / boundary 0**，当前 **1637 / 2174 / 640**。原rebuild拒绝73、overview失败147、legacy形状247、一次性MCP token304由五个独立真实认证production HTTP测试支持；具体临时数据库、settings file与运行时owner实际装配，没有新增公开契约或持久化owner。
- canonical `verification-receipts/settings-owner-http-canonical-2026-10-08.json`：**5 passed / 0 failed / 0 ignored / 1820 filtered_out**（该数字为receipt中的engine lib suite过滤数，不是workspace总数），SHA-256 `a10731d19643828222a50007ba314c42a96fe38b664c4ccb8fb2934c874e7185`。tracked diff与两份Rust源码指纹已核对，备份于`/tmp/jftrade-settings-owner-canonical-source`。首次`settings-owner-http-initial-2026-10-08.json`五条通过，属于补充MCP拒绝启用字节守卫及轮换后读取/落盘断言之前状态，由canonical替代，原字节保留。
- MCP保持原始6697/token输入；初始读取隐藏hash，无token启用400且不写settings，reset只返回一次明文、后续PUT/GET隐藏token与hash。额外通过真实MCP监听器确认轮换前有效token进入405方法校验、轮换后旧token401/新token405，落盘不含新旧明文。原文未要求第二次reset/旧token失效，纠正旧mapping将其作为原始必要残余的描述，新增安全回归仍保留。
- overview故障在具体owner中由损坏的临时rebuild marker触发，原文仅断言500与DATABASE_STATUS_FAILED，两者均通过；marker原字节保持，移除故障后同一运行时恢复200。rebuild畸形及wrong确认逐请求检查原400/code、无marker及settings字节不变。
- 原callbacks754保留partial：overview200/id=adk及两条removed data-migration路由404已直接闭合；原合法single/adk/REBUILD adk在运行中的ADK WriterLease下实际409 DATABASE_MAINTENANCE_CONFLICT/零marker，Go callback要求200/restartRequired=true与转发成功。该production反例不能用synthetic成功替代，也不改变owner fence。
- 本批只新增五条测试、注册、五条mapping及必要reuse和本说明。现场quick、完整Rust及最终审计仍待执行，定向receipt不作为全量门禁。
- 现场quick明确退出0，日志`/tmp/jftrade-settings-owner-quick.log`：**2252 passed / 0 failed / 0 skipped**，fmt、Clippy、七类兼容回放及Pine worker **98 passed**。ordinary/strict、AI context与diff检查通过，anchor **2118 unique / 2070 recorded / 0 unrecorded / 0 stale / 48 unknown**；Rust inventory **3661**。两份Rust字节仍匹配canonical。完整Rust已启动，结束前不记通过。
- 本批完整现场`pnpm run check:rust`明确退出0，日志`/tmp/jftrade-settings-owner-rust.log`：workspace **3817 passed / 0 failed / 2 skipped**，无LEAK；static和七类兼容回放通过。跳过项未计为已执行通过，未使用历史workspace结果代替本次门禁。

## 2026-10-08 Order-book wire 与最佳价发布

- 复核五条partial及既有exact363，共六条mapping。净 **+3 exact / -3 partial / boundary 0**，当前 **1633 / 2178 / 640**。升级原orderbook13的NVDA当前wire布局、proto_v10813的Get/Update字段编号、trading_reads343的同session unsolicited派发；既有exact363改由完整原始HK.00700数值与无重复事件断言直接支持。没有改冻结fixture、generated protobuf、公开schema或依赖。
- 真实生产修复在composition-held OpenD listener：原实现只检查过滤后的bids/asks数组是否为空，会发布两侧最佳价均零的事件，甚至把原始缺失首档后的深层价作为有效输入。现在检查原始两侧首档，均零/缺失则丢弃；任一侧有效则保留原投影。顺序control marker验证零事件与恰一次完整双侧发布，不用sleep推断队列状态。
- 最终focused canonical `verification-receipts/order-book-wire-best-price-canonical-2026-10-08.json`：**7 passed / 0 failed / 0 ignored / 2759 filtered/skipped**，SHA-256 `17b053189811a405a98b47d64611d90424f984fc0e37da0360623b4520efe584`。Rust源码、tracked diff及逐文件指纹保存于`/tmp/jftrade-order-book-canonical-source`；之后仅更新mapping/reuse、批次说明与门禁结果。loopback fixture单reader、有界socket读写、显式close/join，握手失败也join；实际GetGlobalState解码与3013推送共用同一已初始化session。
- 原始生产红receipt `verification-receipts/order-book-best-price-original-red-2026-10-08.json`保留：**2 passed / 1 failed**，SHA-256 `83c24339b0036cf49951e0e1f5189ce476d10d607e5c2e69dfaf773fc7c36e2c`；由canonical的相同零首档回归替代。原生产源码/测试/diff备份于`/tmp/jftrade-order-book-red-source`。
- 两份测试实现失败也保留原字节：`order-book-wire-build-red-2026-10-08.json`退出101、零测试执行，SHA-256 `9b9e5456bdefc46f2d4dcbc5f84aafc02c0d15da9d2fbb9ca11474e3bcee96a0`，原因是误用不存在的WireTimestamp构造函数；`order-book-wire-number-assertion-red-2026-10-08.json` **6 passed / 1 failed**，SHA-256 `016b05583589854b277c6171681d84aa0052ff1255c55815b4af302ba7873eec`，原因是JSON Number(204.0)与整数比较方式不同。修复测试为实际时间戳parse，并同时断言原proto整数volume与现有wire的精确数值；未改fixture或生产数值投影。两份由最终canonical替代，不记为通过。
- 两条仍partial：原error94要求的完整Display与Rust typed Rejected格式不同，实际retType1/errCode321/message保持但不升级；原bridge111的既有测试仅证明US内存tick-cache与fixture depth num50转发，仍缺原HK.00700 securityInfo/3203 snapshot/managed subscription/orderBook detail90001、默认num10、RPC次数及零3004/3001副作用的同session闭环。新NVDA样本不能替代该原输入；桥接行保留既有receipt，不声称本批重新执行旧owner测试。
- 已先查看quick计划；最终focused与独立fmt通过。现场quick、完整Rust与最终审计结果在本批收口时追加。
- 本批现场quick退出0，日志`/tmp/jftrade-order-book-quick.log`：**2795 passed / 0 failed / 1 skipped**，fmt、Clippy、七类兼容回放、Pine worker **98 passed**与desktop脚本 **48 passed**。strict与AI context退出0，anchor **2113 unique / 2065 recorded / 0 unrecorded / 0 stale / 48 unknown**。六条mapping净变化与4451总数已核对，canonical四份Rust源码指纹仍匹配；完整Rust已启动，未结束前不记通过。
- 完整Rust首轮`/tmp/jftrade-order-book-rust.log`在target-health退出1，未进入测试。确认无Cargo/rustc进程后，只清理`target/debug/deps/*.rcgu.o`，移除 **54998 files / 7223807344 bytes**，保留库和测试binary；日志`/tmp/jftrade-order-book-cache-clean.log`。一次过早重试`/tmp/jftrade-order-book-rust-clean.log`在清理完成前仍被同一门禁拒绝，亦未执行测试。等待清理明确退出0且target-health通过后，在`/tmp/jftrade-order-book-rust-after-cache-clean.log`重跑，没有放宽阈值。
- 最终现场完整`pnpm run check:rust`明确退出0：workspace **3812 passed / 0 failed / 2 skipped**，无LEAK；static与七类兼容回放通过。ordinary/strict、AI context、anchor与diff检查通过，提交前四份Rust指纹仍匹配canonical。两个skipped未计入已执行通过。

## 2026-10-08 Embedded provider research 原始 HTTP 输入

- 五条mapping逐项复核，净 **+1 exact / -1 partial / boundary 0**，当前 **1630 / 2181 / 640**。仅原179升级：同一production product接收原始新闻与US公司行动GET，逐项核对title/brokerId/selectionReason/statement/exDate，并核对helper两次目标及limit5。生产装配实际注入helper，不使用会被composition清除的caller route port。
- 最终canonical `verification-receipts/embedded-research-http-formatted-2026-10-08.json`：**5 passed / 0 failed / 0 ignored / 2209 filtered/skipped**，SHA-256 `03e5e476ec653a25b4a214daa386dae5f61b2970f7adf7b8c6df29153148222a`。包含当时mapping/doc的完整tracked diff SHA-256 `adf0c2f44de7ea99545a7653f4eafa5475b0705b66e37ab15246b7e868a1451e` 已匹配；原始字节备份于`/tmp/jftrade-embedded-research-formatted-source`，diff于`/tmp/jftrade-embedded-research-formatted-source-diff.bin`，Rust单文件指纹于`/tmp/jftrade-embedded-research-formatted-fingerprints.json`。之后只更新receipt引用与门禁说明。
- 原始红receipt `verification-receipts/embedded-research-http-original-red-2026-10-08.json`保留，实际 **1 passed / 4 failed**，SHA-256 `785f35ce98ebfbf27fe691eef9f984407905d9f39548248d513b63d976b4c76d`。失败均为原输入的能力边界，与canonical中明确的counterexample断言配对：输入不变、实现不变、不改冻结fixture、不扩大provider授权；没有把后续绿色解释为四条原Go行为已对齐。
- 四条仍partial：errors229的SH公司行动409/无Retry-After/零helper不同于原503/2；额外合法US请求真实到helper并验证503 MARKET_DATA_PROVIDER_BUSY/2。rankings264的原US gainers/limit10与wire已闭合，但yfinance+CN boards/members为409且零新增helper，Go要求两条200。company370的US profile/analyst200和原fieldType/rating4已闭合，SH financials/cashflow与ownership为409、无helper。calendar498的六条原始yfinance请求均409/零helper，Rust该组只接受active AKShare，原Go要求六条200及日期/indicatorId/limit/字段转发。
- 当前只新增测试及其注册；无生产逻辑、HTTP契约、provider fence、锁文件或schema改动。fixture通过有界GET headers读取、独立临时数据库、固定loopback helper响应和显式cancel/join保持资源收口。定向Clippy退出0；focused只证明五条owner测试，现场quick/完整Rust及最终审计在收口时追加。
- quick首轮`/tmp/jftrade-embedded-research-quick.log`在target-health退出1（中间rcgu.o达到50000），没有执行本批quick测试；确认无Cargo/compiler进程后，指定clean移除117756 files/33.1GiB，日志`/tmp/jftrade-embedded-research-clean.log`。重跑`/tmp/jftrade-embedded-research-quick-clean.log`虽2244 passed/0 failed/0 skipped，随后fmt拒绝新增module注册顺序，整体退出1。只按formatter调整注册顺序，独立fmt退出0并重新focused生成formatted receipt；原始名字为canonical的五条绿色保留，SHA-256 `c5bba70b8f947e8ea406633369af94a8057ba5db8343a04e0c7c4a94c374cc8e`，由formatted替代。旧源码与diff备份仍在`/tmp/jftrade-embedded-research-source`及`/tmp/jftrade-embedded-research-source-diff.bin`，原失败不记通过。
- formatted现场quick退出0，日志`/tmp/jftrade-embedded-research-quick-formatted.log`：**2244 passed / 0 failed / 0 skipped**，fmt与Clippy、七类兼容回放、Pine worker **98 passed**。最终strict退出0；anchor对账 **2109 unique / 2061 recorded / 0 unrecorded / 0 stale / 48 unknown**。提交前再次核对两份Rust文件指纹匹配formatted receipt，只修改五条mapping及必要reuse。
- 本批现场完整`pnpm run check:rust`退出0，日志`/tmp/jftrade-embedded-research-rust.log`：workspace **3805 passed / 0 failed / 2 skipped**，无LEAK；Rust static与七类兼容回放均通过。两条skipped不能计入已执行通过。最终AI context、diff检查均退出0。

## 2026-10-08 Pine UDF 调用与循环作用域

- 本批只复核五条 mapping 与必要 reuse，净 **+3 exact / -3 partial / boundary 0**，当前 **1629 / 2182 / 640**。原838的六个脚本均以原错误片段拒绝；原unknown206的entry/exit unknown=1在60/61行经公开analysis拒绝；原window141完整脚本经公开analysis成功并逐项包含九个原需求key。撤销此前对window141“完整语义对象投影”和unknown206“私有parseState对象”的额外要求，它们未出现在冻结原文断言中。
- 实际修复：expression_from_text根据表达式suffix定位并按字符数转换column，避免f(x) => x及f(x) => f(x)错误选择header。独立semantic_call_scope在lower/plan前验证UDF参数数量、调用链递归及活动循环变量只读；嵌套循环退出后保留外层只读绑定，全部退出后允许同名赋值。调用链按调用行缓存已检查body，避免分支图重复展开；合法UDF、nested loop、tuple写入及原source column有回归。
- canonical `verification-receipts/pine-call-scope-canonical-2026-10-08.json`：**8 passed / 0 failed / 0 ignored / 117 filtered/skipped**，SHA-256 `d08308bcb325b33ea1a0673e4360d630dd41088d6b11be4421e4bbd8cdf76973`。tracked Rust diff SHA-256 `7f7e93e0efde1131108d4c36acb59b8feb66a7c9dc7968302dd9a235f081bee4` 已匹配；源码与历史untracked字节保存在`/tmp/jftrade-pine-call-scope-source`，diff保存在`/tmp/jftrade-pine-call-scope-source-diff.bin`，不把focused receipt作为全量门禁。
- 历史receipt保留：original-red **1 passed / 1 failed**直接发现header解析错误和loop改写被接受；initial-fix **5 passed / 0 failed**为scope cache及额外IR断言之前源码；reviewed **6 passed / 2 failed**是新增Rust IR显示断言误用了逗号空格和符号operator（实际Display使用Equal/Add等枚举名），仅更正新增断言的显示预期，原脚本与冻结fixture不变。上述当前关系由canonical解释，失败字节不删除、不改写。
- 两条仍partial：pending390虽逐项保留六个Action及全部原始参数，没有Go OrderStmt/CancelStmt的intent、SELL、symbol_position_percent、LIMIT及All投影；UDF790明确保留两函数与五条statement、typed For，未形成原七条展开Let、第八条归一化If及唯一ma:EMA:3规划。不把raw call保留当静态展开等价。
- 最窄strategy crate **124 passed / 0 failed / 0 skipped**；新增pending回归后当前125条全部进入quick和workspace门禁，canonical显式执行其中8条。Rust inventory **3644**。定向Clippy退出0。普通/strict审计退出0，anchor **2105 unique / 2057 recorded / 0 unrecorded / 0 stale / 48 unknown**。
- 现场quick `/tmp/jftrade-pine-call-scope-quick.log` 明确退出0：**2364 passed / 0 failed / 0 skipped**，fmt、Clippy、trading/strategy replay、Pine98与desktop48通过；计划保存于`/tmp/jftrade-pine-call-scope-quick-plan-final.log`。完整现场Rust `/tmp/jftrade-pine-call-scope-rust.log` 明确退出0：**3800 passed / 0 failed / 2 skipped**，static与七类replay通过，无LEAK，skipped不计通过。canonical源码指纹与最终Rust字节保持一致，公开契约、锁文件与冻结fixture无变化。

## 2026-10-08 Execution HTTP scope 与撤单前置校验

- 本批只审查五条 mapping，净 **+2 exact / -2 partial / boundary 0**，当前 **1626 / 2185 / 640**，Rust inventory **3638**。原execution_routes53与110分别由独立真实HTTP测试闭合：SIM-001/REAL-001、默认SIMULATE、显式REAL、broker/account/market组合与空结果；真实SettingsFileStore保存REAL并经production_ports实际getter进入HTTP。两条HK默认设置fixture的数量均100，seed保存原COMMAND_PLACE_ACCEPTED事件。
- 生产修复：持久化account/header与combo响应投影在取消fence和外部modify之前校验。非法账户不再先写CANCEL_SUBMITTED；缺legs组合不再先向券商发取消后才返回500。红测直接复现非法账户已被写成CANCEL_SUBMITTED；最终回归同时要求两种畸形订单的order/events全值不变、零modify。原WriterLease、revision/CAS、cancel-inflight fence保持。
- canonical `verification-receipts/execution-http-owner-clippy-reviewed-2026-10-08.json`：**9 passed / 0 failed / 0 ignored / 2230 filtered/skipped**，SHA-256 `650cb59086ee63058a5442805918634f80d808133495fe28eef80e60edc60ccd`。五个Rust源码保存在`/tmp/jftrade-execution-http-clippy-source`，对应tracked diff保存`/tmp/jftrade-execution-http-clippy-source-diff.bin`，SHA-256 `ec607fe0c2d6f19d9bc59e2026cb850427d52d2bbcee522a03378c3d227c2d30`，已逐字节匹配；升级仅引用canonical。此前finalized为独立注释导致文件802行的9条绿色，final-source为压缩重复解释注释后、Clippy多余借用修正前的9条绿色，均由clippy-reviewed替代，原receipt字节保留。没有搬移职责或提高预算。
- 历史receipt保留原字节：original-scope为夹具补原事件与REAL数量之前2条绿色；scope-lifecycle-first为header字段名误写account_id而编译失败、0执行；scope-lifecycle-compiled为2 passed/3 failed（save_order返回更新时间未用于比较，以及Rust无prepared事件）；scope-lifecycle-verified名字虽含verified，实际7 passed/1 failed（存储combo夹具缺legs）；cancel-validation-red为0 passed/1 failed，直接复现真实取消fence先于账户校验。上述由canonical解释当前状态，历史失败未清除或改写。缺legs时500已发生外部modify的日志也保留，之后夹具填有效legs与畸形回归分开验证。
- 三条仍partial：exec18原无accountId输入得到400/零place，而Go200；Rust列表SUBMITTED/raw null、submitted→cancel_submitted两事件、CANCEL_SUBMITTED，与Go BROKER_ACCEPTED/rawSUBMITTED、三COMMAND事件及accepted/orderStatus不同。cancel202缺symbol时Rust成功、非法ID错误分类和nil store类型边界不同，原errors.Is与空internalID结果未闭合。combo298的plainBroker能力、C-1原字段、独立combo service/errors.Is/store.cancelled未同输入闭合。有效Rust组合extended ID与零gateway副作用只证明相应子集。
- 普通/strict、anchor、AI context、diff通过；anchor **2103 unique / 2055 recorded / 0 unrecorded / 0 stale / 48 unknown**。quick首轮因production文件802行违反800行预算退出1（`/tmp/jftrade-execution-http-quick.log`）；两行独立解释合为操作旁注释后文件800行，architecture明确退出0。quick-final被target-health拒绝，确认无Cargo/rustc后clean移除135725文件/34.3GiB；quick-clean受影响**2239 passed / 0 failed / 0 skipped**，随后Clippy两处测试多余借用退出101。修正后engine+desktop定向Clippy退出0，canonical重新执行9条通过。对应失败日志保留在`/tmp/jftrade-execution-http-quick-final.log`、`/tmp/jftrade-execution-http-clean.log`、`/tmp/jftrade-execution-http-quick-clean.log`及定向通过日志`/tmp/jftrade-execution-http-clippy-reviewed.log`。最终quick `/tmp/jftrade-execution-http-quick-clippy.log`明确退出0：**2239 passed / 0 failed / 0 skipped**，格式、Clippy、七类replay、Pine98、desktop48通过。现场完整Rust `/tmp/jftrade-execution-http-rust.log`明确退出0：**3794 passed / 0 failed / 2 skipped**，static与七类replay通过，无LEAK；2 skipped不计通过。五个Rust源码及canonical retained receipt指纹最终核对一致，契约源、schema、锁文件、冻结fixture不变。
- 下一批五条已读取冻结原文：`parse_request_test.go:141:TestCompileSupportsV14WindowMomentumAndStatefulIndicators`、`parse_test.go:390:TestCompileSupportsPendingStopAndCancelOrders`、同790 `TestCompileSupportsExpressionUDFAndStaticForUnroll`、同838 `TestValidateScriptReportsUnsupportedUDFAndStaticForCases`、`parser_and_lowering_recovery_test.go:206:TestOrderCallsRejectUnknownNamedArgumentsBeforePlanning`。优先建立UDF参数个数、递归与loop变量readonly失败回归，补真实compiler/analysis断言；typed AST与Go展开OrderStmt/LetStmt形态差异继续保留。整体Codex目标保持active，本批收口后继续下一批。

## 2026-10-07 Responses usage 与 GET reconnect live

- 本批只复核五条 mapping，净 **+1 exact / -1 partial / boundary 0**，当前 **1624 / 2187 / 640**，Rust inventory **3632**。原 `responses_model_test.go:48` 的真实 SSE created/delta/completed/DONE 输入、final 非 partial 语义与 input9/output3/total12 全部由最终模型结果直接证明；nested cached0/reasoning1 同时保留。相邻工具名消毒、child usage 归属、原 event-ID 去重与跨审批恢复不由此条推定等价。
- Provider usage 现在留在 ModelResponse，并进入真实 Product POST SSE final 与 durable run。额外红测发现工具轮丢失用量、最终轮覆盖历史用量；工具 staging 和 success 统一在原 revision/lease fence 下累加，已有4/1 + 工具轮9/3 + 最终轮5/2 = **18/6**，terminal replay 不重复计数。
- Production GET reconnect 使用 HTTP 消费者持有的 cursor body，同连接历史 sequence1 为 replay=true，随后追加 sequence2/3 为 replay=false；终态排空后结束。双路由返回实际 stream ID，after=7 跨64/64/2三页无重复遗漏，缺 sequence 的旧事件使用数组 ordinal。idle drop 释放 store reader，listener shutdown 关闭 idle HTTP，读取方不改 run 状态，存储故障只返回一次 body error 后结束。每页最多向 Rust 解码64事件；SQLite 仍扫描 run JSON 数组，本批不声称索引读取或固定查询成本。
- canonical `verification-receipts/sse-usage-reconnect-owner-finalized-2026-10-07.json`：**15 passed / 0 failed / 0 ignored / 2290 filtered/skipped**，SHA-256 `913b352e9eb0856099d2559058d5cb7b3ea467cee2acf2d3750d4ff16cc99516`。23个Rust源码备份至 `/tmp/jftrade-sse-usage-reconnect-finalized-source`；canonical tracked diff `/tmp/jftrade-sse-usage-reconnect-finalized-diff.bin` SHA-256 `889ec54efecd83186cf32819aa3b2581790fbaea273fe7d2c91b109fd024378c` 完全匹配。后续仅更新本批五条 mapping、11个必要reuse关系及生成摘要。
- 失败证据保留原字节：`sse-stream-usage-initial` 是 fixture crypto provider 未初始化；`sse-stream-usage-provider-ready-red` 复现真实 final tokensIn=null；`sse-reconnect-live-red` 同连接只收到1条历史而缺2条live；`sse-usage-round-accumulation-red` 工具轮后仍是已有input4而不是13。上述均由finalized canonical替代为当前状态。`sse-usage-reconnect-owner-verified` 的14条绿色是累计修复前状态，由finalized替代。额外非阻塞mock socket修正前2条失败日志、两次缺字段编译诊断保留在 `/tmp`，不记通过。
- 四条partial残余：socket-like retry/event写失败精确一次与预取消idle零body；原hub按run timeout+retention清理；空/timeline-only/mixed/run-only及context/session/reasoning/reply分类组合；原空ChatRequest的terminal error与Rust clientRequestId admission JSON400差异及一次session preview内部状态。现有final裁剪、有效preview与未知agent零preview均复跑，未用usage/reconnect代替这些断言。
- 普通/strict、anchor与AI context通过；anchor **2098 unique / 2050 recorded / 0 unrecorded / 0 stale / 48 unknown**。quick初次被target-health拒绝，确认无Cargo/rustc后clean移除118263文件/29.9GiB；clean后2536测试及Clippy/七类replay/Pine98/desktop48通过。累计修复后最终quick `/tmp/jftrade-sse-usage-reconnect-final-quick.log` 明确退出0：**2537 passed / 0 failed / 0 skipped**，Clippy、七类replay、Pine98与desktop48通过。现场完整 `/tmp/jftrade-sse-usage-reconnect-rust.log` 明确退出0：**3788 passed / 0 failed / 2 skipped**，static与七类replay通过，无LEAK；2 skipped不计通过。最终23个Rust指纹及canonical diff仍匹配，契约源/schema/锁文件/冻结fixture没有改动。
- 下一批五条候选：`execution_routes_test.go:53:TestExecutionOrdersEndpointFiltersByTradingEnvironmentAndScope`、同110 `TestExecutionOrdersEndpointDefaultTradingEnvironmentFromSettings`、`exec_routes_test.go:18:TestExecutionOrderRoutesPlaceListEventsAndCancel`、`execution_gateway_lifecycle_test.go:202:TestExecutionGatewayCancelOrderBoundaries`、同298 `TestExecutionGatewayCancelComboBoundaries`。已读取冻结原测试；默认settings与原始SIM-001/REAL-001过滤须由真实HTTP证明。原exec18缺accountId输入与Rust明确要求accountId、nil store与Rust类型边界不由数字成功样例消除。整体Codex目标保持active，本批提交后继续下一批。

## 2026-10-07 Assistant shutdown deadline 与后台终态错误

- 本批逐项复核五条原Go，仅重复Service.Close条目升exact，净 **+1 exact / -1 partial / boundary 0**，当前 **1623 / 2188 / 640**，Rust inventory **3624**。原Go312只要求两次Close无错误；ProductionPortBundle经真实ADK proxy/runtime的重复shutdown直接闭合，不推断关闭后store不可写。
- 生产修复一：continuation barrier超时不再被忽略。runtime→ADK adapter→ProductionAdkPort→bundle返回未完成错误，超时保留live task owner及tool ports供重试；同步termination也传播错误。runtime和scheduler同时失败时保留两者诊断。每个owner各自有5秒等待，本批不宣称全链路共用5秒deadline。
- 生产修复二：真实后台Canvas节点失败的最终日志遗漏顶层error。checkpoint和最终projection现在保留原FAILED节点错误，既有error契约字段、WriterLease、CAS和schema不变。后台成功测试直接验证QUEUED→SUCCEEDED、同ID、run/session/result/finishedAt与US.AAPL渲染；SQLite BEFORE UPDATE RUNNING的RAISE(ABORT)验证同ID FAILED恢复、原错误、finishedAt及零model dispatch。
- canonical `verification-receipts/assistant-shutdown-queue-owner-verified-2026-10-07.json`：**8 passed / 0 failed / 0 ignored / 2187 filtered/skipped**，SHA-256 `407cdf25b2289123c1bfcb73298e2fe21adc62ca7aa4dd80511e894b582e6b3b`。12个Rust源码纳入tracked diff并备份；`/tmp/jftrade-assistant-shutdown-queue-verified-diff.bin` SHA-256 `ca601b5113f8506dd37a4a6d1f6e563b84f3534e228cf03668c48bf07a4ffc6c`与receipt一致，最终源码逐文件匹配。
- 失败证据保持原字节：shutdown-deadline-initial **1 passed / 1 failed**复现旧barrier超时误报成功；queue-recovery-initial **0 passed / 3 failed**是fixture缺start节点，修fixture后真正到达目标路径；queue-terminal-error-red **0 passed / 1 failed**发现顶层error缺失，queue-final-log-error-red **0 passed / 1 failed**在join后重新读取最终日志仍复现，排除中间checkpoint读时序。上述四个receipt均由canonical替代为当前状态，原始失败仍可审计；另曾发生Display编译诊断，0执行，未收录独立receipt，不记通过。
- 四条partial明确残余：StartWorkflow立即accepted响应；无canvasGraph的原Go失败与Rust合法legacy成功差异（有直接反例）；detached trigger/secretHash/US.MSFT响应、模板前置与nil Service panic；driver spy的saveCalls3/savedLogs2。最终SQLite一行不能替代写调用次数。共享后台成功测试分别证明两条partial中的不同子集，独立审核reuse，未扩大既有共享批准。
- ordinary/strict/anchor/context/diff通过；anchor **2097 unique / 2049 recorded / 0 unrecorded / 0 stale / 48 unknown**。reuse初次审计因新共享项缺allowed字段失败，修正元数据后通过，未放宽审计。quick首轮target-health失败；确认无Cargo/rustc后清理 **113445 files / 28.2GiB**。`/tmp/jftrade-assistant-shutdown-queue-quick-clean.log`明确exit0：受影响nextest **2225 passed / 0 failed / 0 skipped**、Clippy、七类回放、Pine98及desktop脚本48通过。`/tmp/jftrade-assistant-shutdown-queue-rust.log`明确exit0：workspace **3780 passed / 0 failed / 2 skipped**、静态及七类回放通过，无LEAK。filtered/skipped均不计通过。
- 下一批：API SSE重连的同连接replay watermark后live帧、断连释放与失败写入停止；同步复核Provider streaming usage metadata的原始9/3/12断言。持续goal保持active，以上局部通过不等于整体行为对齐完成。

## 2026-10-07 Assistant 取消清理与关闭 owner

- 本批五条原Go逐项复核，净 **+1 exact / -1 partial / boundary 0**，当前 **1622 / 2189 / 640**。实际ProductionAdkChatRuntime的8个并发shutdown均等待已受理后台callback取消后的release，全部完成且关闭后受理拒绝；原Go13的全部断言闭合，独立主测试有直接anchor和canonical receipt。
- 生产修复：scheduler.stop保留被abort的tick JoinHandle，结束检查包含future取消清理；async production shutdown等待tick和invocation，current-thread runtime可调度取消，deadline或waiter取消保留未完成owner。同步Drop检查到活动tick如实记录未完成。补充测试锁定8并发waiter、超时后重试、waiter取消后重试和真实Product关闭后的WriterLease重开，无公开契约或schema变化。
- canonical `verification-receipts/assistant-lifecycle-owner-verified-2026-10-07.json`：**12 passed / 0 failed / 0 ignored / 2177 filtered/skipped**，SHA-256 `3798ee14b0e037dec90c445ed3cb74088c65725b2d760ff59c7ce5d7764c5577`。tracked diff备份`/tmp/jftrade-assistant-lifecycle-verified-diff.bin`的SHA-256 `00188fec8615fc5e50f84bfc7156ee2f96c8f805d468977451d32bd562f32bf7`与receipt一致，全部新Rust文件已纳入tracked diff，源码备份保留。
- 失败证据保留：scheduler初次build-error（缺fixture Debug）0执行；scheduler red **0 passed / 1 failed / 2180 filtered/skipped**证明旧join提前报告成功。red时新增support目录未被旧recorder的目录级untracked发现逻辑展开，不能声称完整源码指纹；canonical已用intent-to-add纳入实际源码，升级仅引用canonical。另有owner build-error（重复Arc包装）0执行；diagnostic red **11 passed / 1 failed / 2177 filtered/skipped**是节点错误原文多RUN_CANCELLED前缀，改为逐字断言真实带code输出并保留partial，未修改fixture或抹去差异。
- 四条残余：显式Arc store在shutdown后仍可写、startWorkflowAsync关闭sentinel；同一scheduler对象restart；workflow_runs.wait的logId/30ms deadline/5000ms poll与queued→succeeded；原生runner context/errors.Is及maxAttempts3/1h retry配置。Canvas两种port取消/100ms deadline已实际断言attempts1/successor0与durable FAILED/SKIPPED，不能替代真实runner context取消。
- 普通/strict、anchor、AI context及diff检查通过；anchor **2092 unique / 2044 recorded / 0 unrecorded / 0 stale / 48 unknown**。quick首轮因上一批Pine注释中的“Go test”被零Go扫描当作命令而失败，日志`/tmp/jftrade-assistant-lifecycle-quick.log`保留；仅改该注释措辞后，`/tmp/jftrade-assistant-lifecycle-quick-policy-fix.log`明确退出0：**2274 passed / 0 failed / 1 skipped**，Clippy、七类回放、Pine98及desktop48通过，无LEAK。完整Rust首轮target-health失败留在`/tmp/jftrade-assistant-lifecycle-rust.log`；确认无Cargo进程后清理120329 files/32.9GiB，`/tmp/jftrade-assistant-lifecycle-rust-clean.log`明确退出0：**3774 passed / 0 failed / 2 skipped**，静态和七类回放通过，无LEAK。canonical engine源码与备份一致；后续Pine注释改动已纳入实际完整门禁。filtered/skipped未计通过。
- 后续具体目标：Assistant continuation barrier目前忽略超时返回值，需补超时未完成、owner保留和release后重试回归，再修复生产关闭错误传播。SSE GET历史快照后的同连接live事件、watermark及断连释放仍待闭合；这些剩余行为不因本批门禁通过而视为完成。

## 2026-10-07 Pine wire 与 gRPC 直接证据

- 仅本批五条mapping及必要reuse，净 **+2 exact / -2 partial / boundary 0**，当前 **1621 / 2190 / 640**。原始56字节编码向量和capabilities源变更两条有直接owner断言；主入口唯一，旧共享allowed批准保留。
- canonical `verification-receipts/pine-protocol-owner-verified-2026-10-07.json`：passed，5 passed/0 failed/0 ignored/51 filtered/skipped，无LEAK，SHA-256 `a1ee86b841b4d5f314a0db9d9ab6ce0d65a1bb108c5b043dd250a5e63980ac2c`。tracked diff备份`/tmp/jftrade-pine-protocol-owner-verified-diff.bin`的SHA-256 `b9f8b09bdcf29cf6e10bf295ef637ab965b699d5027cdb4fc0b18478dfb81500`与receipt一致，untracked Rust源码备份保留。
- 三条明确残余：CPU-zero worker 2/1/1及indexed时间错误；Go nil Client/WithNow/mapTransportError对象边界；Go errors.Is身份、nil transport/Close和unlimited消息默认值。原Go79只要求空jobId回填，旧mapping的mismatch拒绝要求纠正，Rust额外guard独立注明。
- 普通/strict及anchor通过：2087 unique/2039 recorded/0 unrecorded/0 stale/48 unknown。quick `/tmp/jftrade-pine-protocol-owner-quick.log`明确退出0：2265 passed/0 failed/1 skipped，Clippy通过，无LEAK。完整Rust `/tmp/jftrade-pine-protocol-owner-rust.log`明确退出0：3765 passed/0 failed/2 skipped，静态及七类回放通过，无LEAK。filtered/skipped不计通过，最终Rust源码与canonical指纹一致。

## 2026-10-07 WebSocket 重放的红绿关系

- 仅本批五条mapping及必要reuse，净 **+3 exact / -3 partial / boundary 0**，当前 **1619 / 2192 / 640**。新exact均有实际认证production握手、直接wire断言和独立主测试；旧共享reuse allowed保留，没有扩大共享批准。
- `verification-receipts/websocket-production-http-initial-2026-10-07.json`：failed，4 passed/1 failed/2175 filtered/skipped，SHA-256 `9ce62828da8d448cecbd6e85c6f31a90d611ee4772625bcd2acf1b918b97015d`。唯一红测是连接前seq1通知未重放；原receipt与修订前源码备份保留，未改冻结fixture。
- `verification-receipts/websocket-production-http-verified-2026-10-07.json`：passed，8 passed/0 failed/0 ignored/2274 filtered/skipped，无LEAK，SHA-256 `cec56d13b2c5bc4ea190315c0091a4c177f1e5a227e62b6cefe2164e6e40b791`。现有hub有界保留、顺序/重试去重、connect/publish线性化和SQLite cursor确认条件都有实际测试；没有新增持久化owner或schema，wire字段不变。
- 原Go缺provider没有关闭码要求，fixture1006与Rust1008差异不阻止该Go条目闭合。当前Web listener动态同源接受真实Cookie/Origin，旧Tauri-only判断纠正。具体残余：depth订阅初始snapshot、num50/entityId|50；原0.0.0.0:0 admission和未装assets Web根200。两条保持partial。
- 普通/strict及anchor通过，2082 unique/2034 recorded/0 unrecorded/0 stale/48 unknown。审计复核曾拒绝无Go anchor的附加引用及不一致reuse，诊断留在`/tmp/jftrade-websocket-production-http-strict-citation-failures.log`；mapping收窄到主WebSocket测试直接证明的原Go行为，三项补充owner回归保留于receipt与报告，原共享批准保留，未补虚假anchor或放宽审计。
- quick明确退出0：2312 passed/0 failed/0 skipped，Clippy、七类回放、Pine98及desktop48通过，无LEAK。完整Rust首轮target-health失败保留`/tmp/jftrade-websocket-production-http-rust.log`；确认无构建进程后清理121241 files/32.5GiB，`/tmp/jftrade-websocket-production-http-rust-clean.log`明确退出0：3760 passed/0 failed/2 skipped、静态及七类回放通过，无LEAK。skipped未计为通过，canonical Rust指纹与最终源码一致，源码备份保留。

## 2026-10-07 设置失败矩阵与 durable 状态

- 仅本批五条mapping及必要reuse，净 **+2 exact / -2 partial / boundary 0**，当前 **1616 / 2195 / 640**。两个exact分别为account路径ID/500失败及onboarding重置；都有生产HTTP、具体file owner、重启及唯一主入口证据。
- `verification-receipts/settings-durable-http-initial-2026-10-07.json`：failed，4 passed/1 failed/2171 filtered/skipped。Pine原始{}实际400 BAD_REQUEST，Go要求500。后续verified保留原始400反例，再补符合现有OpenAPI required字段的请求实际500；没有调整生产fence、契约源、fixture或隐藏失败。
- `verification-receipts/settings-durable-http-verified-2026-10-07.json`：passed，5 passed/0 failed/0 ignored/2171 filtered/skipped，SHA-256 `a7124a23f7bb72a92afc6e34dbd06439badefcf046e43a2e93bee479d4a885bf`。十一条原始请求和额外合法Pine请求，每次都检查九类runtime设置与备份字节不变；恢复原文件后字节不变。
- 三条具体残余保留：Pine {}原始500/当前400；通知important/off原始raw类别及mode与领域归一不同；缺notification tester原始500/当前typed host503。不用改输入、synthetic generic错误或删除fence消除差异。
- 普通/strict及anchor通过：2080 unique/2032 recorded/0 unrecorded/0 stale/48 unknown。quick明确退出0：2206 passed/0 failed/0 skipped、Clippy/七类回放/Pine98通过，无LEAK。现场完整Rust首轮target-health失败保留`/tmp/jftrade-settings-durable-http-rust.log`；清理116817 files/32.6GiB后`/tmp/jftrade-settings-durable-http-rust-clean.log`明确退出0：3754 passed/0 failed/2 skipped、静态及七类回放通过，无LEAK。skipped不计通过，canonical源码指纹已核对。

## 2026-10-07 交易读参数与 portfolio 的红绿关系

- 仅更新五条mapping及必要reuse；净 **+2 exact / -2 partial / boundary 0**，当前 **1614 / 2197 / 640**。原Go110的不存在sync计数要求已删除，原始filter和default REAL由production六行ledger/HTTP设置变化直接证明；Go249完整partial-fill tuple、missing404及重启直接证明。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `trading-read-http-initial-2026-10-07.json` | failed，3 passed/2 failed，2166 filtered/skipped；省略环境混入SIMULATE、测试误期望acc-1 portfolio400 | owner-verified修环境、如实核对native503；原字节保留 |
  | `trading-read-http-owner-verified-2026-10-07.json` | failed，4 passed/1 failed，2166 filtered/skipped；US trd_market却投影HK.AAPL | native-market-verified修共享position市场投影 |
  | `trading-read-http-native-market-verified-2026-10-07.json` | failed，5 passed/1 failed，2165 filtered/skipped；缺required averagePrice | wire-verified补OpenAPI已要求的portfolio字段 |
  | `trading-read-http-wire-verified-2026-10-07.json` | passed，6/6，2165 filtered/skipped，无LEAK；含旧cost/PnL fallback | 修订前源码证据；missing-cost-verified为当前canonical |
  | `trading-read-http-missing-cost-initial-2026-10-07.json` | failed，0 passed/1 failed，2170 filtered/skipped；缺成本时averagePrice为null | 保留HTTP红测，按Go firstFloat零值回退修复 |
  | `trading-read-http-missing-cost-verified-2026-10-07.json` | passed，6/6，2165 filtered/skipped，无LEAK；average/legacy/缺成本三行直接核对 | 当前canonical |

- canonical SHA-256 `290dbd8d1cdcb72e183a078b2757a97d01388a82452be45c0ad9d3163e060ca4`；各红测及绿色源码指纹已核对，保留完整diff与untracked备份。native-market补字段不是修改契约源，cash/positions lastError和position十个required键直接断言；broker与portfolio都核对US source market及价格。缺成本HTTP红测发现averagePrice=null违反既有number契约；补0回退后绿色，没有修改契约源或fixture。
- residual：read71原始acc-1 cash-flow成功无法由native42替代；portfolio143原始acc-1要求200而实际503；helper17的通用Page绑定、generic500、snapshot429/Retry-After与risk409矩阵未闭合。没有把numeric成功请求当原始输入或引入多broker registry。
- 普通/strict及anchor通过：2075 unique、2027 recorded、0 unrecorded、0 stale、48 unknown；AI context与diff检查通过。修订后quick明确退出0：2201 Rust passed/0 failed/0 skipped（含desktop Rust30）、Clippy、七类回放、Pine98和desktop检查通过，无LEAK。此前target-health失败日志保留，确认无构建进程后清理113901 files/28.5GiB；修订前quick也通过，但不用于替代修订后结果。现场完整Rust明确退出0：3749 passed/0 failed/2 skipped，静态与七类回放通过，无LEAK；两个skipped没有计为通过。最终Rust diff与canonical源码指纹一致，契约源、schema及冻结fixture无变化。

## 2026-10-07 策略失败矩阵与 durable 删除

- 仅更新本批五条及必要 reuse；净 **+1 exact / -1 partial / boundary 0**，当前 **1612 / 2199 / 640**。Go lifecycle401 的 success200/inst-1 转发、missing404、busy400 由 production HTTP 与 shutdown/reopen 后 durable 副作用直接证明。
- `strategy-failure-http-initial-2026-10-07.json` 实际六项 passed；`strategy-failure-http-reviewed-2026-10-07.json` 七项 passed，增加旧版本重启测试；修正三处 Clippy needless borrow 后，`strategy-failure-http-verified-2026-10-07.json` 为 canonical，**7 passed / 0 failed / 0 ignored / 2159 filtered/skipped**。SHA-256 `0c1f5876533282b22c19f72248b5578a747207da20f54a81d65eabb67667f49b`，sourceState 指纹已核对并备份，原始 receipt 不覆盖。
- 首轮 quick `/tmp/jftrade-strategy-failure-quick.log` 在2196项Rust通过后被Clippy拒绝三处needless borrow，整体退出1；失败日志保留，最终门禁须重跑，不能把测试集合通过当整个quick通过。
- failure71 的 rehearsal 错误传播与 production SMA20 已补；production read/write 故障分类仍需直接证明。failure138 补 production missing instantiate/apply404 且实例仍空，apply catalog500/instantiate read400 未闭合。failure184 四路 rehearsal500 与原入参已补，production CAS/store409/502 和回滚差异仍在。lifecycle252 补 history/snapshot500 和原错误文本，既有 version/restart 重跑；fixed savedAt/fixture、缺 URI400、production store 故障分类保留。
- 普通/strict、anchor和AI context通过；anchor **2072/2024/0/0/48**。quick第二轮在target-health退出1，确认无构建进程后指定clean移除118776文件/33.8 GiB；最终quick-clean明确退出0，2196 Rust passed（其中desktop 30）、Pine98、静态/七类replay通过，无LEAK。现场完整 `/tmp/jftrade-strategy-failure-rust.log` 明确退出0：**3744 passed / 0 failed / 2 skipped**，静态及七类replay通过，无LEAK；源码指纹保持，focused digest不代表全量摘要。

## 2026-10-07 插件 HTTP / concrete file owner

- 六条映射净 **+4 exact / -4 partial / boundary 0**，当前 **1611 / 2200 / 640**。仅改本批六条及必要reuse，旧allowed权限保留。HTTP lifecycle复用于API642完整五路与catalog13持久化窄子集，后者仍partial；catalog89使用直接owner独立主入口。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `plugin-http-file-owner-red-2026-10-07.json` | failed，4 passed/1 failed，2154 filtered/skipped；installation错误500却生成artifact | verified修复文件变更前类型校验；原字节保留 |
  | `plugin-http-file-owner-verified-2026-10-07.json` | passed，5/5，2154 filtered/skipped，无LEAK | reviewed增加catalog missing的直接owner断言；之前两个exact主入口重复被审计拒绝 |
  | `plugin-http-file-owner-reviewed-2026-10-07.json` | passed，6/6，2154 filtered/skipped，无LEAK；尚未补marker根值 | shape-verified替代 |
  | `plugin-http-root-shape-red-2026-10-07.json` | failed，0 passed/1 failed，2159 filtered/skipped；null marker卸载500却已删除工件 | 根object校验移到文件变更前；原字节保留 |
  | `plugin-http-file-owner-shape-verified-2026-10-07.json` | passed，6/6，2154 filtered/skipped，无LEAK；八种畸形marker场景 | 当前canonical |

- shape-verified SHA-256 `efbad0612e192d6e9f827b983d8c9657517d6b11240fc9b3ef23e7f830b9e845`；tracked diff/untracked指纹已核对并备份，Rust源码保持。普通/strict与anchor通过，未削弱unique exact主入口规则。初始普通/strict失败日志为`/tmp/jftrade-plugin-audit.log`、`/tmp/jftrade-plugin-strict.log`，复跑为reviewed/shape后缀。五项HTTP版quick明确退出0；六项quick首轮target-health退出1，确认无构建进程后指定clean（121662文件/32.3 GiB），重跑明确退出0（2190 Rust passed、Pine98、desktop48、Clippy/回放通过、无LEAK），日志见成果摘要。
- 根形状复查前完整Rust `/tmp/jftrade-plugin-rust.log` 在1667/3738通过时主动中断（退出1），不能算完整通过。shape-verified源码下最终quick `/tmp/jftrade-plugin-quick-shape.log` 明确退出0（2190 Rust passed、Pine98、desktop48、Clippy/回放通过、无LEAK）；完整Rust `/tmp/jftrade-plugin-rust-shape.log` 明确退出0（3738 passed、0 failed、2 skipped，静态/七类replay通过、无LEAK）。focused digest不代表全量门禁，最终源码指纹、AI context与diff review已核对。
- residual：catalog13缺原始RegisterPlugin默认descriptor、重复注册覆盖、saveCount5；servercore16原始legacy build tuple实测requiresRebuildfalse/安装503，与Go true/200不同。合法source生命周期不替代原始demo-plugin差异。

## 2026-10-07 Strategy/Pine durable HTTP 的红绿关系

- 本批五条映射净 **+2 exact / -2 partial / boundary 0**，当前 **1607 / 2204 / 640**。只改五条 mapping 与必要 reuse，保留此前共享审核权限。missing-start 测试复用于 Go75 的missing分支和 Go319 的instantiate/list窄子集，两条仍partial，不宣称其它生命周期等价。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `strategy-durable-http-initial-2026-10-07.json` | failed，3 passed/2 failed、2149 filtered/skipped；linked删除错误200、version缺isCurrent | 两项真实生产差异由reviewed复跑通过解释，原字节保留 |
  | `strategy-durable-http-owner-verified-2026-10-07.json` | failed，0 tests；store fixture误用seed_instance参数，编译失败 | corrected修正测试装配；名字不代表通过 |
  | `strategy-durable-http-owner-corrected-2026-10-07.json` | failed，0 tests；移动read adapter后原测试缺Preview/Snapshot trait import | read-imports修正测试作用域 |
  | `strategy-durable-http-read-imports-2026-10-07.json` | passed，7/7、2349 filtered/skipped；编译时仍有未使用import警告 | reviewed移除import并整理新测试排版后重新构建，无警告 |
  | `strategy-durable-http-reviewed-2026-10-07.json` | passed，7/7、2349 filtered/skipped，无LEAK | 当前canonical |

- reviewed SHA-256 `ec5b49114e12b375cab15947c19c7ba6729d6b154365575c3fc66cca4e0f70a4`；完整tracked diff与三个untracked Rust文件指纹已核对并备份，随后Rust源码不变。普通/strict、anchor、AI context通过。quick明确退出0（2386 Rust passed、Pine98、desktop48、无LEAK）；完整Rust首轮target-health退出1，确认无构建进程后指定clean移除127344文件/33.3 GiB。重跑明确退出0（3732 passed、0 failed、2 skipped，静态/七类replay通过、无LEAK）。四份日志见成果摘要，focused digest不充当全量门禁摘要，失败原日志保留。
- 残余：版本固定savedAt及500/直接handler缺URI400；worker capacity/preflight400、失败转换502与runtime.Stop副作用；Go无body instantiate、apply/update/runtime-risk/refresh与pause/stop/start完整状态矩阵仍未闭合。

## 2026-10-07 embedded screen production 行为回归

- 五条 API/facade mapping 净 **exact +4 / partial -4**，当前 **1605 exact / 2206 partial / 640 boundary**；旧 reuse allowed 权限逐项保留，没有扩大共享批准。
- `research-screen-embedded-http-initial-2026-10-07.json` 原字节保留：1 passed/4 failed，SHA-256 `56c8586145b546205707e3dd3b5f730c0e5b7172fa0e16f3161ad0b89024d3dd`。catalog 缺省 market、string cell 空 unit、helper envelope 三项属于测试装配/多余要求；gt 409→400 是真实 normalizer 回归，红测还记录失败期间 LEAK，不能称首轮通过。
- `research-screen-embedded-http-catalog-validated-2026-10-07.json` 为当前 canonical：5 passed/0 failed/0 ignored/2144 filtered/skipped，无 LEAK，SHA-256 `19f2f31bf87010687ff7d455f173b45548714cb214912670cd6536304254b100`。匹配 Rust diff 与全部 untracked 指纹；concrete production HTTP→helper 与纯 wire result projector 的证据范围分别标注。
- unknown total 用原始三组 FeatureResult 形状经 rehearsal wire projector 验证；不放宽 native helper 必填 total。native helper 的 next_offset1 与 Go facade 的逐行 offset 形成方式仍不同，本行仅证明原始 POST 行为，不宣称全分页等价。
- provider failure 的文本已在 production HTTP 保留，但 Go errors.Is 对象身份尚无 Rust 服务层证明，facade78 保留 partial。普通/strict/anchor 与 engine Clippy 通过；quick 退出 0（2179 Rust passed、Pine 98 passed、desktop 48 passed，无 LEAK）。完整 Rust 首轮 target-health 因至少 50000 个 `.rcgu.o` 退出 1（`/tmp/jftrade-embedded-screen-rust.log`）；确认无 Cargo/rustc/nextest 后按门禁提示清理 119470 文件/34.6 GiB，clean 日志保留。完整重跑明确退出 0（3725 passed、0 failed、2 skipped，静态/七类 replay 通过、无 LEAK），日志 `/tmp/jftrade-embedded-screen-rust-clean.log`；失败仅被后续通过结果解释，未删除。

## 2026-10-07 research screen HTTP 的 production/rehearsal 边界

- 仅 catalog、normalize、原始 V1 形状拒绝三条升级 exact；**1601 exact / 2210 partial / 640 boundary**，净变化 **+3/-3/0**。只改五条 mapping 与必要 reuse 子集，不扩大此前共享批准。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `research-screen-http-initial-2026-10-07.json` | failed，0 passed/5 failed；请求未携带 Authorization，均 401，未进入 screen 行为 | authenticated 修正测试凭证；不是生产缺陷红测 |
  | `research-screen-http-authenticated-2026-10-07.json` | failed，2 passed/3 failed；catalog/V1 400 通过，正向查询 409；production fence 清除了 recording route port | scoped 明确 rehearsal seam；没有削弱 fence，不将未接入的 zero query 视为证明 |
  | `research-screen-http-scoped-2026-10-07.json` | passed，5/5，2139 filtered/skipped | 当前 canonical |

- scoped SHA-256 `40969aaf7677396ada5d27a5f9e1297c965c2b244849632ccb51c5275d5975e6`；编译时 tracked diff 与 untracked source 已核对。catalog 与 V1 400 是 production HTTP；录制 definition 的三个请求是认证后的 rehearsal HTTP，与真实 parser/normalizer 相连，没有伪造 production broker readiness。legacy 形状还在同一 rehearsal listener 以合法 V2 的 200/一次 query 作正向控制。
- 两条 residual 明确保留 generic FeatureQuery operation/pageFrom/字符串 Cursor、Go ScreenDefinitionV2 强类型值与具体生产 adapter 的证据差异。普通/strict/anchor 与 engine Clippy 通过。现场 quick 退出 0（2174 Rust passed、Pine 98 passed）；完整 Rust 门禁退出 0（3720 passed、0 failed、2 skipped，静态与七类 replay 通过、无 LEAK）。focused digest 不充当全量摘要。

## 2026-10-07 审批 durable boundaries 与运行时拒绝

- 五条相关 partial 仅损坏负载三分支升 exact，**1598 exact / 2213 partial / 640 boundary**。只改变本批五条 mapping 和必要 reuse 子集；旧关系保留此前审核权限，不新增共享等价范围。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `approval-durable-boundaries-red-2026-10-07.json` | failed，3 passed/3 failed；两项损坏 payload 被接受，第三项在 open 的 schema trigger 校验失败，未到写入 | owner-fault-red 修正故障注入；原字节保留 |
  | `approval-durable-owner-fault-red-2026-10-07.json` | failed，4 passed/2 failed；TEMP trigger 注入三种 rollback/retry 已通过，target/sibling 类型损坏仍失败 | 修复 production decoder 后 final 替代 |
  | `approval-durable-behavior-reviewed-2026-10-07.json` | failed，0 tests；新增测试误用不存在的 get_approval，编译退出 101 | 改用已有 list 查询，closed 替代；文件名不表示通过 |
  | `approval-durable-behavior-closed-2026-10-07.json` | passed，8/8；错误类型断言仍位于 helper | final 将错误类型直接断言于三项测试正文后替代 |
  | `approval-durable-behavior-final-2026-10-07.json` | passed，8/8，2331 filtered/skipped；quick 后 Clippy 要求去除 fixture 冗余闭包 | verified 替代 |
  | `approval-durable-behavior-verified-2026-10-07.json` | passed，8/8，2331 filtered/skipped；fixture closure 修正后复跑 | 当前 canonical |

- verified SHA-256 `3b554192f59797d99e388a77da27ef8983a82c16151b8a68d16c65322594e0ce`；编译时 tracked diff 与 untracked 文件摘要已核对，后续只改 metadata/docs。普通/strict 和 anchor 通过。quick 首轮 `/tmp/jftrade-approval-quick.log` 在 **2369 tests passed** 后因新 fixture 的 Clippy redundant_closure 退出 **1**；修正后三个受影响 package Clippy 已通过，后续完整结果见下文。未将 focused receipt 用作全量门禁证据。
- 残余：Rust 没有 nil Store；重开 store 的 restage 不证明 parent reconciler；store fault 不证明所有 async/sync facade、background supervisor 或 missing-continuation envelope；直接 ModelResponse staging/两阶段 resume 不证明 Go 初始 Chat XML/统一 ResolveApproval 入口。
- quick 第二轮 `/tmp/jftrade-approval-quick-retry.log` 在 target-health 退出 **1**，未进入测试；确认无 Cargo/rustc/nextest 后运行指定 `clean:rust:artifacts`，移除 **122891 files / 29.7 GiB**。第三轮 `/tmp/jftrade-approval-quick-clean.log` 明确退出 **0**：**2369 passed / 0 failed / 0 skipped**，本次无 LEAK；Clippy、七类 replay、Pine **98 passed**、桌面 **48 passed**。完整 `/tmp/jftrade-approval-rust.log` 明确退出 **0**：workspace **3715 passed / 0 failed / 2 skipped**，静态/七类 replay 通过，本次无 LEAK；源码保持 verified receipt 对应状态。未另生成 workspace JSON receipt，focused digest 不代表全量门禁摘要。

## 2026-10-07 Assistant canvas 保存与执行

- 五条 partial 中仅图保存/重读升级 exact，当前 **1597 exact / 2214 partial / 640 boundary**，净变化 **+1/-1/0**。其余四条新增 production 断言后保留具体差异，不以 durable 输出存在证明 Go parent/Markdown/plan 字段等价。

  | 文件（均位于 verification-receipts） | 实际结果 | 后续关系 |
  | --- | --- | --- |
  | `workflow-canvas-behavior-red-2026-10-07.json` | failed，4 passed/2 failed；Go 点前缀模板未渲染、输入中的占位符被二次解释 | 修复 canvas owner 后 reviewed 替代；保持失败原字节 |
  | `workflow-canvas-behavior-green-2026-10-07.json` | passed，6/6；尚未加入 JSON/Unicode/未闭合与下游 node-output literal 断言 | reviewed 替代 |
  | `workflow-canvas-behavior-reviewed-2026-10-07.json` | passed，7/7，2131 filtered/skipped | 当前 canonical |

- reviewed SHA-256 `f5a70d548a085876241cc45e2430d1898c1785d94adcb4edaa9f07e24fe5e9b2`；已验证 sourceState 与编译时完整 diff/untracked 文件相符，更新 mapping/docs 后 Rust diff 相同。只改变五条 mapping 和原共享关系的必要子集，没有新增共享审批。
- 普通/strict audit 与 anchor 通过，unrecorded/stale=0。quick 首轮 `/tmp/jftrade-workflow-canvas-quick.log` 为 target-health 失败；确认无 Cargo 进程后按指定入口移除 **118677 files / 31.3 GiB**，原日志保留。重跑 quick `/tmp/jftrade-workflow-canvas-quick-retry.log` 明确退出 **0**：nextest **2168 passed（1 leaky）/ 0 failed / 0 skipped**、Pine **98 passed**、桌面 **48 passed**，Clippy 和七类 replay 通过。leaky 为既有 `test_non_futu_helper_candles_failure_uses_generic_market_code`，并非新增 canvas 测试。完整 `check:rust` `/tmp/jftrade-workflow-canvas-rust.log` 明确退出 **0**：workspace **3707 passed / 0 failed / 2 skipped**、静态和七类 replay 通过；同 helper 测试 PASS，本次无 LEAK 标记。保留 quick 的观测，不因全量未复现而称该历史退出风险已修复。未另生成 workspace JSON receipt。
- 残余：Go Result.Markdown、parent workflowEngine/childRunIDs/workflowPlan/workflowStatus 尚无 Rust 同形投影；Go 无图拒绝与 Rust legacy fallback 不同。当前修复只处理真实输入/输出模板行为，不改公开 DTO 或现有 fallback 策略。

## 2026-10-07 Web proxy/session 与 Origin 原始请求复核

- 五条 partial 中两条升级 exact；旧开发 Origin exact 撤回，另一条 Origin exact 改用真实 POST login owner，净变化 **+1/-1/0**，当前 **1596 exact / 2215 partial / 640 boundary**。
- 本批 `verification-receipts/` 文件保持原字节，名字不能代替结果。

  | 文件 | 实际结果与范围 | 当前关系 |
  | --- | --- | --- |
  | `web-security-development-origin-red-2026-10-07.json` | failed，0 passed/1 failed；真实生产 POST login 开发 Origin 为 200，Go 期待 403 | 保留行为反例，mapping 已恢复 partial |
  | `web-proxy-session-boundaries-2026-10-07.json` | passed，6/6；原始代理、密码触发与架构反例，第七条 Origin POST 尚未加入 | reviewed 替代 |
  | `web-proxy-session-boundaries-reviewed-2026-10-07.json` | passed，7/7；包含真实 POST login 的 evil/current-listener Origin 对照 | 当前 canonical |

- canonical SHA-256 `709229962c0a61d8fdde7a1511f691b1508c9e946d6c5911458d541c78071cdf`；**2124 filtered/skipped** 不计通过。已复核 trackedDiff/untracked 摘要；sourceState 指向编译时的测试代码，后续只更新 metadata/docs。
- 首轮 strict `/tmp/jftrade-web-proxy-session-strict.log` 因拆分 owner 后误拒绝原已审核的两个 reuse 子集失败；保留原审核子集后 `/tmp/jftrade-web-proxy-session-strict-reviewed.log` 通过，没有修改审计规则或批准新共享复用。普通 audit、anchor 通过。
- 六条 owner 的中间 quick `/tmp/jftrade-web-proxy-session-quick.log` 退出 **0**（2160 passed）；第七条加入后的最终 quick `/tmp/jftrade-web-proxy-session-quick-reviewed.log` 明确退出 **0**，nextest **2161 passed / 0 failed / 0 skipped**，Pine **98 passed**，静态和七类 replay 通过。完整 `check:rust` `/tmp/jftrade-web-proxy-session-rust.log` 明确退出 **0**：workspace **3700 passed / 0 failed / 2 skipped**、静态与七类 replay 通过；未另生成 workspace JSON receipt。
- 具体残余：private loopback proxy/XFF remote 的逐请求拒绝不存在；TCP bind 与 Go REMOTE_WEB_ACCESS_DISABLED 不同；Web disabled browser 401/403；生产开发 Origin grant 的 200/403。已用真实反例断言，不把它们视为等价完成。后续转向 Strategy/Pine 或 Assistant 的可闭合 partial，目标继续 active。

## 2026-10-07 GET replay 与 Web listener 重配置批次

- 五条相关 partial 逐项核对冻结 Go；热重绑/冲突恢复升级 exact，其余四条保留明确残余。当前 **1595 exact / 2216 partial / 640 boundary**，净变化 **+1/-1/0**。
- 本批文件均位于 `verification-receipts/`；失败文件保持原字节，新通过证据只适用于其记录的源码状态。

  | 本批文件 | 实际结果与原因 | 后续证据 |
  | --- | --- | --- |
  | `reconnect-listener-red-2026-10-07.json` | failed，0 passed/2 failed；GET materialized body，活动 SSE 导致 disable/rebind graceful join 等待 | reviewed |
  | `reconnect-listener-green-2026-10-07.json` | passed，7/7；尚无 iterator probe、重绑 WS 和 SSE EOF/旧 cookie 断言 | reviewed |
  | `reconnect-listener-verified-2026-10-07.json` | failed，12 passed/1 failed；新 Web 端口 WS 被启动时固定 Origin 列表拒绝（403） | reviewed |
  | `reconnect-listener-final-2026-10-07.json` | passed，15/15；动态 Origin 与 foreign-origin 拒绝已验证，构造函数重命名前 | reviewed |
  | `reconnect-listener-reviewed-2026-10-07.json` | passed，15/15；Clippy 要求 from_iter 改为 from_chunks 后的最终 Rust 源码 | 当前 canonical |

- canonical SHA-256：`9665a84257262cb6a7534358d4e75aa8dc2a0a96ccc30969da62721a13821b03`；15 passed、0 failed、0 ignored，另有 **2209 filtered/skipped**。已逐项复核 trackedDiff/untracked 摘要，并确认更新 metadata 后 Rust diff 不变。
- quick 首轮 `/tmp/jftrade-reconnect-listener-quick.log` 在 target-health 阈值失败，未执行测试。确认无 Cargo 进程后按仓库入口清理；日志 `/tmp/jftrade-reconnect-listener-clean.log`。重跑 `/tmp/jftrade-reconnect-listener-quick-retry.log` **2254 tests passed**，但随后 Clippy 拒绝 from_iter 命名，整体退出 1；未记为通过。
- 重命名后的 quick `/tmp/jftrade-reconnect-listener-quick-reviewed.log` 明确退出 **0**：受影响 nextest **2254 passed / 0 failed / 0 skipped**、Pine worker **98 passed**，Clippy、七类 replay 与桌面检查通过。完整 Rust `/tmp/jftrade-reconnect-listener-rust.log` 明确退出 **0**：workspace **3693 passed / 0 failed / 2 skipped**，静态与七类 replay 通过。未另生成 workspace JSON receipt，focused digest 不代替全量门禁摘要。普通/strict audit、anchor、AI context 与 diff 检查通过。
- 保留残余：GET snapshot 仍全量读取 events，同路由 socket-like 首次写失败、预取消 idle 零 body、持续 live reconnect 未闭合；hub watermark 后 live 与 timeout+retention 清理未实现；desktop bind coercion/rejection、browser disable 403/401 仍不同。下一批转向相关生命周期与 Assistant 发布序列，目标保持 active。

## 2026-10-07 活动流撤销批次与失败替代关系

- 五条相关 partial 逐项核对；安全变更取消活动 SSE 有新的 production owner 直接行为证明，升级 exact；其余四条不升级。当前 **1594 exact / 2217 partial / 640 boundary**，净变化 **+1/-1/0**。
- 以下文件均位于 `verification-receipts/`，原失败证据保持字节不变；canonical 为当前 mapping 唯一绑定的通过 receipt。

  | 本批文件 | 实际结果与适用状态 | 后续证据 |
  | --- | --- | --- |
  | `web-stream-revocation-red-2026-10-07.json` | failed，0/1；原实现换密码未结束已建立 SSE | reviewed |
  | `web-stream-revocation-green-2026-10-07.json` | failed，0/1；测试 Client 未显式初始化 rustls provider，名字不代表通过 | reviewed |
  | `web-stream-revocation-verified-2026-10-07.json` | failed，4/1；桌面连接测试误认为非法 JSON 应收到 close，实际协议忽略该消息 | reviewed |
  | `web-stream-revocation-final-2026-10-07.json` | passed，5/5；行为修复与合法订阅断言通过，连接 cancellation 参数整理前 | reviewed |
  | `web-stream-revocation-canonical-2026-10-07.json` | passed，5/5；整理 cancellation 参数后、恢复 WS select 公平性前 | reviewed |
  | `web-stream-revocation-reviewed-2026-10-07.json` | passed，5/5；最终 Rust 源码，与 sourceState 全部摘要匹配 | 当前 canonical |

- quick 首轮失败日志 `/tmp/jftrade-web-stream-quick.log` 为 target health 编译缓存阈值，非测试通过；清理日志 `/tmp/jftrade-web-stream-clean.log`。quick 重跑日志 `/tmp/jftrade-web-stream-quick-retry.log` 已确认退出 **0**（2248 nextest、98 Pine、七类 replay 与 desktop 检查通过）；完整 Rust 日志 `/tmp/jftrade-web-stream-rust.log` 已确认退出 **0**（workspace **3687 passed / 0 failed / 2 skipped**，静态与七类 replay 通过）。未生成全量 JSON receipt，不使用 focused digest 代替 workspace 门禁摘要。
- 残余 owner：`product_adk_read_api::adk_read_output` 返回拼接完整 Raw，缺 GET stream/run reconnect 首次写失败一次退出、事件失败与预取消 idle 零 body；`product_production_ports_adk_read::stream_snapshot` 只返回 retained history，缺同连接 watermark 后 live 与 timeout+retention 清理；delta 分类和 context/session/narrative 序列未闭合；空 ChatRequest terminal error 与当前 Rust admission framing 不同。
- 进一步检查 listener 重配置：安全 `save` 先调用 runtime apply，再调用会话 invalidator；disable/rebind 的 graceful join 是否会等待活动 SSE，需要独立失败回归，不能从本批同 bind 换密码测试推导通过。下一批继续围绕这些高风险流生命周期条目选 5–10 条，目标保持 active。

## 2026-10-07 认证行为批次与保留缺口

- 七条相关 partial 已逐项核对冻结 Go。logout middleware、旧 admin bearer 拒绝、带口令配置的 desktop 启动保持三条升级 exact；净变化 **exact +3 / partial -3 / boundary 0**，当前 **1593 / 2218 / 640**。不将测试数量、mapping 数量或 receipt 数量作为整体行为完成度。
- 保留四条具体残余：desktop token 的 Go harness 403/204 与 Rust production 401/200；默认 Web 关闭的 Go `403 WEB_ACCESS_DISABLED` 与 Rust `401 WEB_AUTH_REQUIRED`；认证状态测试只闭合过期会话/CSRF/snapshot/重启清除，未闭合未强制、不可用、public/trusted 全矩阵；换密码保持同 Web bind 时，尚无活动 SSE/WS 取消信号或同路由断言。
- 当前有效 receipt 为 `auth-capability-boundaries-verified-2026-10-07.json`（6/6 passed，SHA-256 `b177620849815de016133d265bc257df159004bac5ac5a7f651dd6672c384978`）；六条新 owner mapping 绑定此 receipt，第七条活动流取消继续保留独立旧 owner 的有限证据。

  | 本批 verification-receipts 文件 | 实际状态及适用源码 | 当前替代 |
  | --- | --- | --- |
  | `auth-capability-boundaries-2026-10-07.json` | failed，4 passed/1 failed；WS 清理状态轮询没有 desktop token，属于测试装配失败 | verified receipt |
  | `auth-capability-boundaries-passed-2026-10-07.json` | passed，5/5；已修正认证轮询，尚未加入过期会话 owner | verified receipt |
  | `auth-capability-boundaries-current-2026-10-07.json` | passed，6/6；过期会话已闭合，安全配置尚未增加完整 record 等值断言 | verified receipt |
  | `auth-capability-boundaries-verified-2026-10-07.json` | passed，6/6；当前六条 owner 的最终源码 | canonical |

- 原始失败 receipt 保持字节不变。focused 的 2207 filtered/skipped 不计为已执行测试；现场 quick 与完整 Rust 门禁均退出 **0**，受影响 nextest **2243 passed、0 failed、0 skipped**，Pine worker **98 passed**，workspace **3682 passed、0 failed、2 skipped**，七类 compatibility replay 全通过。日志为 `/tmp/jftrade-auth-capability-quick-2026-10-07.log` 和 `/tmp/jftrade-auth-capability-rust-2026-10-07.log`；未生成 workspace JSON receipt。普通/strict audit、anchor reconcile 通过，strict 首轮 filter 元数据错误及修复保留于摘要。
- 下一批五条相关候选：`TestSecurityChangeCancelsExistingWebStream`、`TestChatStreamReconnectAndReplayRespectClientDisconnect`、`TestChatStreamHubReplayAndCleanupBoundaries`、`TestChatStreamExecutionPublishesDeltaAndFinalVariants`、`TestExecuteADKChatStreamPublishesTerminalErrorForInvalidRequest`。已核对冻结 Go；live chat 的 `RawStream` 与 GET replay 的 materialized owner 要分开验证，整体目标保持 active。

## 2026-10-07 Broker 写边界严格复核与行为闭环

- `TestBrokerUnlockDisconnectedOpenD`、`TestBrokerPlaceOrderNoBroker` 撤回过强 exact：前者 Go `UNLOCK_FAILED/connect` 与 Rust `BROKER_NOT_CONNECTED/closed` 不同；后者原始无 query 请求在 Rust 返回 400，补账户 query 后的 502 不等同于原始 Go 行为。保留 Rust owner 与 receipt，恢复 partial。
- `TestBrokerCancelOrdersNoBroker` 新增 production HTTP、writer 调用计数和数据库重开断言，但原始未知订单请求 404/502 的分支顺序差异仍保留 partial。测试读端返回断连错误而非成功空订单，并使用夹具实际发现的账户环境，避免后台对账抢先改写种子订单。
- unlock/place/cancel 三条 `InvalidPayload` 由独立 HTTP owner 验证原始请求的 400、错误信封和 writer 零调用，从 partial 升为 exact。当前 **1590 exact / 2221 partial / 640 boundary**，净增 exact 1；strict audit 与 anchor reconcile 已通过。
- 本批现场 quick/full Rust 已完成，两个命令退出码均为 **0**：受影响 nextest **2141 passed、0 failed、0 skipped**，Pine worker **98 passed**；workspace **3677 passed、0 failed、2 skipped**，七类 compatibility replay 全部通过。日志为 `/tmp/jftrade-broker-write-boundaries-quick-2026-10-07.log` 与 `/tmp/jftrade-broker-write-boundaries-rust-2026-10-07.log`；未生成新的 workspace JSON receipt，canonical focused receipt 仅证明本批六条 owner。
- 当前 canonical receipt 为 `verification-receipts/api-broker-write-boundaries-closed-2026-10-07.json`（6/6 passed，SHA-256 `b7711ad6a5979dbe14bf0956ac0ce144f9384cc1333bcb547e248efc96b9d205`），本批六条 mapping 均绑定此 receipt。

  | 本批 receipt（均在 verification-receipts） | 实际状态与解释 | 当前替代 |
  | --- | --- | --- |
  | `api-broker-write-boundaries-2026-10-07.json` | failed，退出码 101；种子 helper Path/String 编译错误，无行为测试执行 | closed receipt |
  | `api-broker-write-boundaries-passed-2026-10-07.json` | failed，3 passed/1 failed；文件名不代表状态，OpenD readiness 装配不完整 | closed receipt |
  | `api-broker-write-boundaries-final-2026-10-07.json` | failed，3 passed/1 failed；成功空订单快照与账户环境不匹配使对账提前写 UNKNOWN | closed receipt |
  | `api-broker-write-boundaries-verified-2026-10-07.json` | failed，3 passed/1 failed；无 reader 的 runtime 清除登录位 | closed receipt |
  | `api-broker-write-boundaries-green-2026-10-07.json` | failed，3 passed/1 failed；断连 reader 已修正，种子账户环境仍不匹配 | closed receipt |
  | `api-broker-write-boundaries-current-2026-10-07.json` | passed，4/4；畸形 payload 尚为聚合 owner 的中间证据 | closed receipt（拆为三条独立 owner 后 6/6） |
  | `api-broker-write-boundaries-closed-2026-10-07.json` | passed，6/6；当前有效证据 | canonical |

  原失败文件保持原始字节，均不计入当前成功状态；本轮失败是测试装配诊断，不宣称为生产行为红测。

## 2026-10-07 API/Transport place-order 断连 HTTP 闭环收口

- `TestBrokerPlaceOrderNoBroker` 已由真实 production HTTP owner `production_http_broker_place_order_maps_disconnected_opend_to_stable_error` 收口为 `function_exact`：合法下单请求在断连 writer 下返回 502、`BROKER_NOT_CONNECTED` 和 `closed` 诊断。首轮 400 仅是 Rust 缺少 `accountId` query 的绑定边界，补齐合法 query 后复跑绿色。
- receipt：`verification-receipts/api-broker-place-order-no-broker-2026-10-07.json`，**1/1 passed**，SHA-256 `d6e3495016751a246a078dd03e4abdccd99a0e17f36571c44a58cab8481b8380`。
- 当前扫描 **4451 Go / 3518 Rust / 1589 function_exact / 2222 partial / 640 boundary**；现场 `check:quick` 与完整 `check:rust` 已通过（受影响 nextest **2138 passed、0 failed、0 skipped**；workspace **3674 passed、0 failed、2 skipped**；7 类 compatibility replay 全部通过），strict audit 与 anchor reconcile 也已通过（2033 unique、1985 recorded、0 unrecorded、0 stale、48 unknown）。

## 2026-10-07 API/Transport broker unlock 断连 HTTP 闭环收口

- `TestBrokerUnlockDisconnectedOpenD` 已由真实 production HTTP owner `production_http_broker_unlock_maps_disconnected_opend_to_stable_error` 收口为 `function_exact`：共享 trade runtime 注入断连 writer，真实 `/api/v1/brokers/futu/unlock` 返回 502、`BROKER_NOT_CONNECTED` 和 `closed` 诊断；叶级 owner 继续覆盖错误映射与成功参数透传。
- receipt：`verification-receipts/api-broker-unlock-disconnected-2026-10-07.json`，**3/3 passed**，SHA-256 `64f2be57a9c7ef63bf9783e0d5884e759a7769d867737301f61610e7c4cee726`，同时覆盖 HTTP 组合 owner、断连错误映射 owner 和成功 unlock 参数透传 owner。
- 本批只修改一条 mapping 和一个 focused production owner；当前扫描 **4451 Go / 3517 Rust / 1588 function_exact / 2223 partial / 640 boundary**。`check:quick` 与完整 `check:rust` 已通过（workspace **3673 passed、0 failed、2 skipped**，7 类 compatibility replay 全部通过），strict parity audit 与 anchor reconcile 也已通过（2032 unique、1984 recorded、0 unrecorded、0 stale、48 unknown）。

## 2026-10-07 API/Transport strategy lifecycle 因果链收口

- `TestStrategiesEndpointReturnsList` 已由真实 production Product HTTP owner `production_http_strategy_lifecycle_projects_started_activity` 收口为 `function_exact`：HTTP 创建 definition、instantiate 实例并 start，随后从同一 durable SQLite projection 读取列表、logs、audit，分别断言实例可见、`started` 日志和 `STARTED` 审计。
- SQLite RUNNING 转换在事务内同时落 `STARTED` 审计和 `started` info 日志，避免测试只验证合成快照；旧 `strategy_instance_read_routes_match_group_fixture_in_cutover_only` 仅保留 malformed pagination 引用。
- receipt：`verification-receipts/strategy-lifecycle-http-2026-10-07.json`，**1/1 passed**，SHA-256 `2afb5eb15f1837b14e7c4ef6697f20193c0855a7090310f70f9f6ef18e9c6200`；验证提交 `5098c4fe`。
- 本批不刷新无行为变化的 report/mapping；下一批优先寻找 API/Transport、Strategy/Pine 或 Assistant/Workflow 中仍缺 production causal owner 的高风险 partial。

## 2026-10-06 API/Transport persisted strategy log tail 收口

- `TestStrategiesEndpointIncludesPersistedRuntimeLogTail` 已由真实 Product HTTP owner `production_http_strategies_list_includes_persisted_runtime_log_tail` 收口为 `function_exact`：production SQLite 写入 runtime error 后，经 `/api/v1/strategies` 列表读取并验证日志尾部完整文本。
- receipt：`verification-receipts/strategy-log-tail-http-2026-10-06.json`，**1/1 passed**，SHA-256 `c38bee2f3ec1b56d04d1ddd49e897904a749967243cc778cfce81f85c2292f21`；mapping 已绑定提交 `2e648eb1`，旧 fixture owner 仅保留其他引用。
- 当前扫描 **4451 Go / 3515 Rust / 1586 function_exact / 2225 partial / 640 boundary**；`check:quick` 与完整 `check:rust` 均已通过，workspace **3671 passed、0 failed、2 skipped**，7 类 compatibility replay 全部通过；focused receipt 只证明本条 owner，strict audit 与 anchor reconcile 另行通过。

## 2026-10-06 API/Transport strategy activity filter 收口

- `TestStrategyLogsAndAuditEndpointsSupportPaginationAndFilters` 已由真实 Product HTTP owner `production_http_strategy_activity_filters_level_and_time_window` 收口为 `function_exact`：同一 production SQLite store 写入 info/warning 日志与 kind 审计后，经 HTTP 路由验证 level/time、kind/time 过滤，logs 的 limit/offset 结果与 `total/returned/hasMore`，以及 audit 分页元数据。
- receipt：`verification-receipts/strategy-activity-filter-http-2026-10-06.json`，**1/1 passed**，SHA-256 `ff1619e330f6536315770138c1be41d81022800859aa6cbcc14e30fed7f018d6`；mapping 已绑定提交 `036c2205`，旧 fixture owner 仅保留其余引用。
- focused receipt 只证明本条 owner；完整门禁与 strict 审计另行现场确认，不将 focused receipt 单独视为 workspace 通过。
- 现场收口已完成：workspace **3670 passed、0 failed、2 skipped**，7 类 compatibility replay 通过；当前严格扫描为 **4451 Go / 3514 Rust / 1585 function_exact / 2226 partial / 640 boundary**，数量覆盖约 **78.9%**。

## 2026-10-06 Strategy/Pine runtime lifecycle 收口

- `TestCatalogRuntimeTransitionsPersistStateAndActivity` 已由 durable SQLite Product HTTP owner `strategy_runtime_sqlite_test_cutover_replays_transport_and_restart` 收口为 `function_exact`：真实路由顺序执行 `STOPPED → RUNNING → PAUSED → STOPPED`，每次转换复读持久状态，重启后继续 stop，并断言 `STARTED`、`PAUSED`、`STOPPED` 审计各一次且顺序稳定。
- 绿色 receipt：`verification-receipts/strategy-runtime-lifecycle-2026-10-06-passed.json`，**1/1 passed**，SHA-256 `af4fadeb2cd3f14ca813c52b620931682375fe71fa6cbabf00474e82309092c6`；首次断言顺序失败的红 receipt `strategy-runtime-lifecycle-2026-10-06.json` 保留，不作为当前成功证据。
- strict parity audit 与 anchor reconcile 已通过（2028 unique、1980 recorded、0 unrecorded、0 stale、48 unknown）。`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` 仍保持 partial，因为 Rust 对 RUNNING 采用 resume 语义而 Go 是 blanket reset。
- 现场完整 `pnpm run check:rust` 首次在 workspace nextest 因 Node probe “ok” case 并发超时失败（2069 passed、1 failed、2 skipped，fail-fast）；隔离重跑该测试通过。随后完整重跑已通过：workspace **3669 passed、0 failed、2 skipped**，7 类 compatibility replay 全部通过；首轮失败保留为诊断证据，不计入当前门禁状态。

## 2026-10-06 Strategy/Pine 活动组合根批次

- `TestCatalogActivitySupportsPagingFilteringAndRuntimeObservationEnrichment` 已由真实生产 HTTP owner 收口为 `function_exact`：`production_http_strategy_activity_pages_filters_and_merges_runtime_observation` 使用 durable strategy SQLite store，验证日志分页（`limit=1&offset=1`）、页面 `total/returned/hasMore`、audit `kind` 过滤、runtime observation 富化和最近日志顺序。
- receipt：`verification-receipts/strategy-activity-http-2026-10-06.json`，**1/1 passed**，SHA-256 `de1921e652d5be0080532c62712e5feddd37f2da5d85f211d768b5c3f7b52d7c`；strict audit、anchor reconcile 均通过。
- 当前 Strategy/Pine 高风险残余：`TestCatalogRuntimeTransitionsPersistStateAndActivity` 仍缺真实 start→pause→stop 每次恰好一次保存/审计的 production route 闭环；`TestCatalogStartupReconcileResetsStaleRunningAndPausedState` 保留 Rust resume 与 Go blanket-reset 的架构差异。下一目标继续优先真实 runtime lifecycle，不刷新无行为变化的报告或 mapping。

## 2026-10-06 API HTTP / WebSocket 行为批次

- `TestSyncRouteClassifiesRequestErrorsAsBadRequest` 从 partial 升为 `function_exact`：真实 Product HTTP 注入生产 `ProductionBacktestPort`，逐项发送 Go 的非法 symbol、非法 since 和反向时间范围，均断言 HTTP 400、`ok=false`、`BAD_REQUEST`、JSON content-type、无 success data，并确认未创建活动 sync task。
- 新增真实 WebSocket depth 订阅测试：由生产 OpenD listener 投影两次 book push，连线客户端收到更新后的价格、`meta.resolvedAt` 和 depth 字段，断线后统计与订阅释放。该条保持 partial：缺少订阅触发的初始 snapshot，push 的 `request.num` 使用实际档数而非请求档数，`entityId` 没有 Go 的 `|50` 后缀。Go 原始 `TestHandlerDepthUpdatePublishesFreshPayload` 没有“相同 resolvedAt 不重发”的断言，不再把该额外要求当作此函数的唯一残余。
- 限定 receipt `verification-receipts/api-sync-http-ws-depth-2026-10-06.json`：**5/5 passed**，SHA-256 `b31ef3a7a788aed0d740e0641ada3e1450243c2d719530c42be6c3506f2c679b`；在 helper 日期修复后的源码上复跑，覆盖两条 mapping 的全部新旧 Rust owner。首轮测试编译缺少 import，修复后的复跑通过；编译失败记录保留在本地 `/tmp/jftrade-api-sync-http-ws-depth-2026-10-06-compile-failed.json`，不作为行为红测。
- 本批只修改两条 mapping 及两条 single-owner reuse 关系；Rust 测试 **3510→3512**，`function_exact` **1581→1582**，partial **2230→2229**。数量覆盖仍为 **78.9%**，严格行为等价仍约 **35.5%**，均不代表整体目标完成。
- `check:quick` 首轮失败：五个 helper sync 用例固定使用 9 月 29 日的 1m 数据，现场日期已超出生产 yfinance 七天历史窗口。请求、mock candle 与分页 cursor 改为同一次捕获的近期 UTC 窗口，保留原行为断言与生产拒绝阈值；另修正正向 capability 用例的相同日期依赖。红 receipt `verification-receipts/backtest-helper-sync-window-red-2026-10-06.json`（0 passed、5 failed）由绿色 `verification-receipts/backtest-helper-sync-window-passed-2026-10-06.json`（6/6 passed）替代为当前定向状态，不删除原失败证据。
- strict parity audit、AI context、anchor reconcile（0 unrecorded / 0 stale）均通过；`check:quick` 重跑已通过，受影响 nextest **2132/2132 passed**。现场完整 `check:rust` 首轮在 target-health 停止（至少 50,000 个 `.rcgu.o`），确认无 Cargo/rustc 进程后按仓库提示运行 `clean:rust:artifacts`，清理可重建编译产物；首次日志保留在本地 `/tmp/jftrade-api-transport-check-rust-2026-10-06.log`。完整重跑明确退出码 **0**：workspace **3668 passed、0 failed、2 skipped**，七类 compatibility replay 全部通过；实际日志为本地 `/tmp/jftrade-api-transport-check-rust-2026-10-06-rerun.log`，未伪造新的 workspace JSON receipt。
- 两个历史失败 receipt 的红绿/current rerun 替代关系已在下一节明确保留，不删除或改写原失败证据。

## 2026-10-04 严格复核补证与下一轮门槛

- ### 历史失败 receipt 的替代关系

  下表中的红 receipt 保留为先红证据，不能删除或改写；当前审计以对应的绿色复跑 receipt 为有效状态。绿色 receipt 的 mapping digest 已分别记录在 `manual-test-mappings.json`，因此历史失败不会被误计为当前门禁失败。

  | 历史红 receipt | 替代绿色 receipt | 当前解释 |
  | --- | --- | --- |
  | `verification-receipts/p1-broker-cancel-rejection-red-2026-09-28T184300Z.json` | `verification-receipts/p1-broker-cancel-rejection-2026-09-28T184500Z.json`；当前工作树复跑为 `verification-receipts/broker-cancel-rejection-2026-10-03.json` | 先红阶段复现旧的 `UNKNOWN` 写入；绿色阶段已验证明确券商拒单保留 `CANCEL_SUBMITTED` 并记录拒单事件。 |
  | `verification-receipts/backtest-aggregation-2026-09-30.json` | `verification-receipts/backtest-aggregation-passed-2026-09-30.json`；当前工作树复跑为 `verification-receipts/backtest-aggregation-2026-10-03.json` | 首轮聚合批次失败后已隔离复跑通过；后续分页、cursor 和聚合顺序 receipt 继续覆盖同一 owner，未把残余语义升级为 exact。 |

- 本轮现场全量 workspace receipt `verification-receipts/workspace-nextest-2026-10-04-final.json` 为 **3664 passed、0 failed、2 skipped suites**（文件 SHA-256 `47f5a5cbba3d5cfd8214c8ca515f45ec04e77855475fa8be6436841db891b210`）；manual evidence verifier 已对 4451 条 mapping 全部通过。
- 2026-10-05 `pnpm run check:rust` 现场复跑为 **3665 passed、0 failed、2 skipped**，7 组 compatibility replay 全部通过；未另生成 workspace receipt，归档证据仍以 2026-10-04 receipt digest 为准。
- 2026-10-06 在当前工作树再次执行 `pnpm run check:rust`，明确退出码 **0**：workspace **3665 passed、0 failed、2 skipped**，7 组 compatibility replay 全部通过；本次仍未生成新的 workspace receipt，因此不改写既有 receipt digest。
- 2026-10-05 新增 `verification-receipts/backtest-aggregation-stream-cursor-2026-10-05.json`，**13/13 passed**：`stream_candles` 对直接区间使用固定页大小和递进 cursor，分钟聚合按完整目标 bucket 分段读取；新增 600 根 1m 跨页回放与 10,000 根 1m→5m 跨交易时段顺序校验。日/周/月聚合仍保留完整读取路径，QueryKLinesCh 多区间 channel 与 EnsureCoverage 仍是 residual，相关 mapping 不升级。
- 新增 `verification-receipts/backtest-aggregation-pages-stream-2026-10-04.json`，**12/12 passed**，覆盖 10,000 根 1m 数据的分页边界、顺序和 callback stream；旧的 2026-10-03 小批 receipt 保留作历史证据。
- 本 receipt 已写回两条 Backtest mapping；当前聚合证据仍只证明结果一致性，不证明有界内存或 Go `QueryKLinesCh` 多区间 channel，因此仍保持 `partial`。
- API/Transport 新增 `verification-receipts/api-sse-write-failure-2026-10-04.json`（2/2 passed）：SSE 通知与 heartbeat 的真实 writer 写失败均在单次回调后终止循环，不重试。WS 会话 send 失败仍只证明 close/断开，不具备 Go dispatcher 的错误返回对象，因此保留 partial。
- API/Transport 新增 `verification-receipts/api-ws-close-lifecycle-2026-10-04.json`（1/1 passed）：LiveHub shutdown 幂等、拒绝新连接、最后连接释放后清空 connected/active instruments 统计，并在 stopped 状态继续拒绝连接；nil handler 的 HTTP 404 仍未有 Rust 同形 owner 断言。
- API/Transport 新增 `verification-receipts/api-execution-order-detail-envelope-2026-10-04.json`（3/3 passed）：execution order detail 的缺失订单与 store failure 在 engine owner 和真实 API router 上分别保持 `404 ORDER_NOT_FOUND`、`500 GET_ORDER_FAILED`，并验证 JSON envelope 字段；缺失 ID 的 handler-level 400 与 typed route 404 差异继续保持 partial。
- Assistant/Workflow 新增 `verification-receipts/assistant-stream-replay-marker-2026-10-04.json`（3/3 passed）：live POST stream frame 不带 `replay:true`，retained/recovered stream frame 继续带 replay marker；Go hub TTL 与内存事件上限属于 durable 模型差异，继续保持 partial。
- Assistant/Workflow 的 chat reconnect 条目已复用 `api-sse-write-failure-2026-10-04.json` 作为真实 transport 证据；当前 residual 是 ADK route 物化 body 与 Go socket-like writer 的 owner 形状差异。
- 下一轮门槛：API/Transport、Strategy/Pine、Assistant/Workflow 三个 P1 域各完成至少一个真实行为闭环，并分别具备 reviewed assertion、Parity anchor、绿色 receipt；随后复跑 `check:quick`、`check:rust` 与 strict parity audit。

## 最新状态：2026-10-03 高风险 partial 行为测试批次

- 本批修改后的现场 `pnpm run check:rust` 已退出码 0：workspace nextest **3659 passed、0 failed，原始输出含 2 个 ignored suite**，7 类 compatibility replay 全部通过；最终工作树 receipt 为 `verification-receipts/workspace-nextest-2026-10-03-final-round.json`（`sha256:bad1550dc8be2d9fbd8ade5b97b61cf9234053ba35aa70fba8c3927ff589a3c0`）。历史 mapping 继续绑定原 canonical receipt，避免批量改写既有证据。
- broker cancel rejection 旧失败记录已由当前工作树 **3/3 passed** receipt `verification-receipts/broker-cancel-rejection-2026-10-03.json`（`sha256:e9fc3b9310f9cefbe81e13e61e7c3d37f0fa9aad09667e3e02406b10cb730001`）取代为当前证据；backtest aggregation 旧失败记录已由 **9/9 passed** receipt `verification-receipts/backtest-aggregation-2026-10-03.json`（`sha256:36db334705ee7d95b9ec5cce5b1b14d85046536bfaa4fe443e493d69589c0023`）取代为当前证据。旧失败 receipt 保留在历史目录供引用，当前 mapping 与新 receipt 均指向绿色复跑证据。
- 新增两个真实 owner receipt：`verification-receipts/backtest-aggregation-pages-stream-2026-10-03.json`（1/1，`sha256:32c3bd7d119e0ec7dbbff0efbd3da47185dba099da9348797328c5fb27b3c123`）和 `verification-receipts/broker-capability-rejection-2026-10-03.json`（1/1，`sha256:70a342b816ad2b08ffe9d7c74dd9731482234d4d8ee01353cfb9a26e83fda429`）。Backtest callback stream、forward/backward limit 和 capability unsupported 的一次性失败/UNKNOWN 账本语义已可执行复现；多区间 channel、非零 UTC wire 投影、请求级 capability 4xx 与 REJECTED/rawBrokerStatus 仍是明确 residual。
- strict parity audit 与 manual evidence verifier 均通过；本批不升级 partial 为 exact。
- `pnpm run check:quick` 已通过：zero-go、policy/contracts、受影响 Rust nextest/clippy、7 类 compatibility replay、Pine worker 与 desktop checks 全部通过。

## 最新状态：2026-09-30 provider 写路由错误矩阵批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:128:TestMarketDataSettingsRoutesMapValidationPersistenceAndRuntimeErrors` 已由真实 Product HTTP 测试 `live_provider_http_route_maps_validation_persistence_and_runtime_failures` 覆盖四类行为：malformed JSON→400 `BAD_REQUEST`、非法 provider→400 `MARKET_DATA_PROVIDER_INVALID`、runtime activation failure→409 `MARKET_DATA_PROVIDER_UPDATE_FAILED` 且旧值保持、settings persistence failure→500 `SETTINGS_SAVE_FAILED`。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-marketdata-provider-errors-reviewed-2026-09-30.json`，文件 SHA-256：`b5cfb9b3a9b816624477a39a9913528b2801a26c31b1df60eaa4276f67cd2c53`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2312→2311**，function_exact **1504→1505**。本轮以真实错误行为、reviewed assertion 与 strict gap 下降收口，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 Web 密码保护 API 行为批次

- 冻结 Go `internal/app/apiserver/webaccess/security_integration_test.go:175:TestWebPasswordIsRequiredForProtectedAPI` 已由真实 Product HTTP 测试 `protected_system_status_requires_web_password_over_product_http` 覆盖：启用 Web 密码保护后，无 session 访问 `/api/v1/system/status` 返回 `401`，错误码为 `WEB_AUTH_REQUIRED`。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-web-auth-required-product-http-reviewed-2026-09-30.json`，文件 SHA-256：`26640e64417f4739d20bb1d629adbb242bc60d4ffa8743f05f398ba3676d69ce`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2313→2312**，function_exact **1503→1504**。本轮以真实行为测试、reviewed assertion 与 strict gap 下降收口，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 live provider HTTP callback 批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:18:TestMarketDataSettingsRoutesReadSaveAndApplyProvider` 已由真实 Product HTTP GET/PUT fixture 覆盖：seed `yfinance`，切换 `futu` 与 `yfinance` 均返回规范化 `activeProvider`，注入 `ActiveProviderState` 的 activation callback 对两次实际 selection 恰好调用两次。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/api-live-provider-http-reviewed-2026-09-30.json`，文件 SHA-256：`052d983ca4fc111bb019ea843dd3f3644eb84c9dc8aa5ffac63a9f94ce9bd004`。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2314→2313**，function_exact **1502→1503**。本轮以真实行为测试、reviewed assertion 与 strict gap 下降作为收口条件，不以测试数量或 receipt 数量作为完成率。


## 最新状态：2026-09-30 backtest provider HTTP route 批次

- 冻结 Go `internal/api/settings/routes_market_data_test.go:63` 已由真实 Product HTTP fixture 覆盖：provider catalog 与 yfinance capabilities、prepare failure 的 409 `MARKET_DATA_PROVIDER_UPDATE_FAILED` envelope、旧值保持、成功切换及 GET 回读。
- 为测试组合增加可控 prepare failure seam；生产默认行为不变。定向 nextest **1/1 passed**，receipt `verification-receipts/api-backtest-provider-http-reviewed-2026-09-30.json`，文件 SHA-256：`bfa7c9803beaa15ab798a1ea7a0dd3f5f14766b6bd44e9997590b0817fbd9fbc`。
- mapping 从 partial 升 reviewed `function_exact`；partial **2315→2314**，function_exact **1501→1502**。不以测试数量或 receipt 数量作为完成率。

## 历史批次记录（以下阶段数值不代表最新状态）

## 最新状态：2026-09-30 current-KL 缺失 S2C 行为批次

- 冻结 Go `pkg/futu/opend/market_read_boundaries_test.go:212` 的断言已逐项核对。已有独立 Rust framed-socket reader 测试直接发送 `GET_KL`，模拟 `retType=0` 且缺失 S2C，断言成功空 klines、空 name 和正确协议号；它与另一条纯 decoder Go 测试使用不同 Rust owner，不再合并计数。
- mapping 从 partial 升为 reviewed `function_exact`；partial **2316→2315**，function_exact **1500→1501**。这是行为缺口下降，不以测试数量或 receipt 数量作为完成率。
- 定向 nextest **1/1 passed**；receipt `verification-receipts/futu-current-kl-empty-s2c-reviewed-2026-09-30.json`，文件 SHA-256：`eb09e7f11ce37faa435c2f6072739f742e49cb024efcdf1bbe0f9e35b52badbf`。

## 历史批次记录（以下阶段数值不代表最新状态）

## 最新状态：2026-09-30 HTTPS 代理登录行为批次

- 冻结 Go `security_integration_test.go:210` 已复核。新增真实 Product HTTP + `ProductionAuthSessionManager` 回归，使用临时 settings/session 文件；断言 loopback + `X-Forwarded-Proto=https` 登录为 200，cookie 带 Secure/HttpOnly/SameSite=Strict、session 有效、响应 no-store。无转发头的对照请求不带 Secure。新增行为首次即绿，没有生产修复；编译阶段错误不作为功能红测。
- 该条从 legacy partial 升为 reviewed function_exact。实际映射：function_exact **1499→1500**、partial **2317→2316**、boundary 635；均不是完成率。strict evidence gap **0→0**，不将本批计为 strict gap 净下降；实际减少的是一条 HTTP 行为缺口。
- strict audit 通过；旧 function_exact anchor 缺口和 assertionless 引用复核均为 **0**。anchor reconcile：1899 unique / 1852 recorded / 0 unrecorded / 0 stale / 47 unknown Go line。2 条无可解析 Rust 测试的 acknowledged partial 仍保留。
- 定向 nextest 实际通过；receipt：`verification-receipts/api-webaccess-secure-cookie-reviewed-2026-09-30.json`，文件 SHA-256：`4c614ca808796890f14ef8e95b941b44c630dcf03733c8947769852279de644a`。映射中的 argv、toolchain、commit、timestamp 和 digest 取自该 receipt。
- 本轮全量门禁实际失败：`check:rust` 在 target-health（至少 50000 rcgu.o）停止；`check:quick` 在既有 zero-go provenance 规则停止。未清理 target、删除 provenance 或放宽门禁；这两项不记为通过。全局目标尚未完成。

## 历史批次记录（以下阶段数值不代表最新状态）

本轮模块批量复核：Futu 234 条 legacy exact 绑定 265/265 passed；Assistant/Workflow 164 条绑定 186/186 passed；均写入 reviewed assertion 与同批 receipt，高 fan-out reuse 仍保持未审。strict gap 实际 **1966→1498→1170**，当前仍未通过。

Futu/OpenD 追加批次：watchlist 7/7、trade-account/helper 8/8、subscription/session 10/10 nextest 均通过；26 条已有 exact 写入 reviewed assertion 与 receipt。strict gap 实际 **1998→1982→1966**，高 fan-out reuse 仍保持未审，strict 仍失败。

本清单只统计缺少函数级 Rust 证据的 `[~]` 项；不代表功能缺失，也不代表已覆盖。每项需要人工对照 Go 断言并补充真实 Rust 测试函数、命令或边界结论。

当前积压：**2960 项**（按当前 `manual-test-mappings.json` 的 `[~]` 条目重算）。

最新审计快照（2026-09-29 13:40 UTC）：Go `4451`、Rust `3398`；`function_exact=1491`、`partial=2325`、`boundary=635`；Parity anchor reconcile 为 `1895/1848/0/0/47`（unique/recorded/unrecorded/stale/unknown）。最新批次：API partial/boundary provenance 复核后，8 个 reuse relation 具备 reviewed 引用；broker/market-rule 12/12、Futu watchlist 7/7 nextest 通过，6 条 watchlist exact 与 13 条 broker exact 写入有效 receipt。strict gap 实际 **2041→2036→2010→1998**，剩余 1998 条仍需按行为证据收口。

本轮严格收口记录：Pine client validation、API runtime/market snapshot/WebSocket、desktop readiness 与 Pine indicator owner 均经过真实 nextest；strict gap 由 2052 降至 2041，仍有 2041 条 function_exact evidence gaps。

API observability 与 runtime 两批已收口；随后 frontend asset 行为测试 3/3 通过并将两条 partial 升为 exact（strict 净值不变），再为 WebSocket client registry 建立独立 owner。联合 API tail 复跑 4/4 通过，receipt `sha256:8c885bf0c5b09e5243b1ef111134b9805bf761ded361160755046f4bb1417eb2`；marketdata forwarding 29/29、servercoretest 23/23、servercore behavior 33/33、marketdataapp 22/22、remaining API 25/25、Strategy/Pine parse 21/21、live execution 19/19、risk/order 10/10 通过，先后收口 15、19、17、19、19、21、7、8 条 reviewed exact，strict gap 实际 **2554→2152**；API transport owner 的 legacy exact 已清零。严格审计仍未通过。

随后批量收口 API runtime 的 3 条 legacy exact（路径环境覆盖、相对 settings 资源派生、strategy preview warmup）。真实 Rust owner **4/4 passed**，receipt `sha256:48d34e9714e6c67134a3a60cd34c18276898c03791c202e5e4a156ddd37d4b07`；strict gap 实际 **2563→2557**，仍保留其余历史 reuse/receipt 缺口。

第二批 API transport route 复核再收口六条已有 exact（optional query、缺失 URI、markets provider failure、非法 refresh、订阅租约冲突）；8/8 行为测试通过并绑定 receipt `sha256:84dbc60dbeb07184ad3c0cbc750321351de2bd6b052e61ea613ea329e0ba553c`。strict gap 实际 **3262→3250**；其余 `[~]` 与结构不等价项继续保留，未用测试数量或 receipt 数量表示完成率。

第三批 API auth/origin 复核六条已有 exact；7/7 行为测试通过并绑定 receipt `sha256:a90d53c5245a42b16cb1397efcf98d666f3bc7aa3c02e56a9dda6e60b19d59e2`。strict gap 实际 **3250→3238**；桌面 scheme 的 wails/tauri 差异继续作为已登记 boundary，不计作未记录功能。

第四批 API subscription 复核七条已有 exact；7/7 行为测试通过并绑定 receipt `sha256:66229dc2cec638a02dc06384b47949a7bca05c593b67daa57dbbce6cecb41710`。strict gap 实际 **3238→3224**；malformed wire fixture 与 subscription owner 的组合仍只按逐条 assertion 计入。

WebSocket live 批次 5/5 行为测试通过并绑定 receipt `sha256:d5630dbcfc1a2e7e10bdaf5bf09f96e992fbff7c53addf84023fa4f0880510dd`；3 条 exact 升为 reviewed，heartbeat `liveClients` 缺失与 Host/allowlist same-origin 差异改记 reviewed partial。strict gap 实际 **3224→3210**。

## 按领域

| 领域 | 条目数 |
|---|---:|
| assistant_workflow | 571 |
| api_transport | 517 |
| other | 503 |
| strategy_pine | 454 |
| backtest_calendar | 313 |
| marketdata_quotes | 162 |
| storage_sqlite | 198 |
| futu_opend | 142 |
| trading_broker | 58 |
| settings_watchlist | 39 |

## 按 Go 文件（Top 50）

| Go 文件 | 条目数 |
|---|---:|
| `cmd/jftrade-desktop/main_test.go` | 24 |
| `pkg/backtest/pineworker_command_executor_test.go` | 23 |
| `pkg/bbgo/types/indicator_test.go` | 23 |
| `pkg/strategy/indicatorbinding/parse_test.go` | 22 |
| `internal/strategy/pine_live_executor_test.go` | 20 |
| `pkg/backtest/conservative_bar_executor_test.go` | 19 |
| `cmd/check-go-coverage/changed_lines_analysis_test.go` | 19 |
| `internal/integration/yfinance/conversion_test.go` | 18 |
| `internal/app/apiserver/marketdataapp/runtime_test.go` | 17 |
| `internal/store/sqliteschema/catalog_test.go` | 17 |
| `pkg/strategy/pine/parse_collection_test.go` | 17 |
| `internal/marketdata/instrument_resolver_test.go` | 15 |
| `cmd/check-go-coverage/profile_analysis_test.go` | 15 |
| `internal/app/apiserver/server_test.go` | 14 |
| `internal/backtest/service_test.go` | 14 |
| `pkg/backtest/internal/storage/store_runtime_invariants_test.go` | 14 |
| `pkg/backtest/store_test.go` | 14 |
| `internal/integration/akshare/boundaries_test.go` | 14 |
| `pkg/strategy/pine/parse_test.go` | 14 |
| `internal/app/apiserver/lifecycle/lifecycle_test.go` | 13 |
| `internal/app/apiserver/servercoretest/broker_new_test.go` | 13 |
| `internal/marketdataassets/asset_selection_boundaries_test.go` | 13 |
| `internal/marketdataassets/cache_test.go` | 13 |
| `internal/store/strategy/store_test.go` | 13 |
| `internal/strategy/pineruntime/runtime_test.go` | 13 |
| `pkg/backtest/pineworker_adapter_test.go` | 12 |
| `internal/marketdata/collector_test.go` | 12 |
| `internal/marketdata/subscription_lifecycle_test.go` | 12 |
| `internal/integration/akshare/provider_company_research_test.go` | 12 |
| `internal/integration/yfinance/client_test.go` | 12 |
| `internal/trading/order_updates_test.go` | 12 |
| `internal/app/apiserver/backtestapp/historical_source_test.go` | 11 |
| `internal/app/apiserver/runtimes/handle_lifecycle_test.go` | 11 |
| `pkg/backtest/filter_store_session_queries_test.go` | 11 |
| `internal/integration/futu/marketdata_runtime_test.go` | 11 |
| `pkg/market/market_normalization_test.go` | 11 |
| `pkg/market/market_test.go` | 11 |
| `internal/integration/yfinance/provider_test.go` | 11 |
| `internal/watchlist/futu/source_test.go` | 11 |
| `internal/strategy/liveruntime/manager_boundaries_test.go` | 11 |
| `internal/strategy/pine_live_command_test.go` | 11 |
| `pkg/strategy/pine/parse_object_test.go` | 11 |
| `pkg/strategy/pine/parse_semantic_test.go` | 11 |
| `internal/assistant/workflow_crud_test.go` | 10 |
| `internal/backtest/input_and_readiness_validation_test.go` | 10 |
| `internal/backtest/sync_test.go` | 10 |
| `internal/marketdata/service_facade_test.go` | 10 |
| `pkg/bbgo/types/rbtree_test.go` | 10 |
| `scripts/go-test-quality/main_test.go` | 10 |
| `internal/store/sqliteconn/conn_test.go` | 10 |

## 最近验证

- 2026-09-28 Strategy/Pine manager close P1：`TestManagerCloseAggregatesNamedSessionErrorsOnce` 先由单 session 错误 fixture 暴露聚合上下文缺口，随后新增两个真实活跃 Pine session 的并发 shutdown 回归；`shutdown_with_error` 现在串行化并发关停、稳定复用聚合错误、按 instance/market.symbol/session close 命名错误，并保证每个 session 只 close 一次。定向 engine nextest 4/4、完整 `check:rust` 3522/3522（2 skipped）、quick 2571/2571（1 skipped）均通过；receipt `sha256:e319b1dd01b56b3ae8bb821aa23bd831c2eb4ca5216765035961fd92f0bdd185`。对应 Go 条目已升级为 `function_exact`；启动竞态聚合的相邻条目仍保留 partial。

- 2026-09-26：Backtest/Calendar P1 取消边界先红后修；`production_helper_sync_cancel_aborts_in_flight_request` 在修复前 2 秒超时，补 worker cancel signal 后由 `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(production_helper_sync_cancel_aborts_in_flight_request)'` 通过。映射仍为 `[~]`，因为 Rust 尚未直接断言 Go 的 `context.Canceled` 与内存 progress 快照。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-strategy --all-targets --locked`：20/20 通过（nextest run `ba80ce7b-485a-4c2d-9ea1-08011ea97730`）。
- 该结果仅证明 `jftrade-strategy` 当前测试集合可执行，不会自动提升未建立函数级映射的 `[~]` 条目。

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-integration-pine --all-targets --locked`：36 项执行，36 通过，1 项跳过（nextest run `5817aea5-d89d-40ee-bf4b-1eb706127be6`）。

- `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked`：1,066/1,066 通过，0 跳过（日志 `/tmp/jftrade-engine-nextest.log`，汇总耗时 120.745s）。

- `pnpm run check:rust`：workspace nextest 1,862 项中 1,862 通过、2 跳过（摘要耗时 125.259s）；SQLite/backtest/provider/trading-strategy/assistant/API/desktop compatibility replay 全部通过。完整日志：`/tmp/check-rust.log`。
- 2026-09-27 strategy_pine P1：`TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs` 先红后修（未修复实现 hour 预期 180、实际 1170），新增 planner `estimate_security_source_bars_handles_period_and_timeframe_fallbacks` 后 nextest 1/1 通过；修复 `resolve_timeframe_minutes` 的 hour canonical alias，mapping/reuse/anchor 已同步。当前全局审计为 Go 4451、Rust 3370、`function_exact=1493`、`partial=2332`、`boundary=628`、`missing=0`；`parity_anchor_reconcile.py` 为 1783/1737/0/0/46。`audit_test_parity.py --strict` 仍因 evidence/receipt gaps 失败，未伪造 receiptDigest。
- 2026-09-27 API transport P1：`TestExplicitBrokerRoutesUseBrokerReaderAndNeverLegacyFallback` 先红后修，新增 quote-read 四路显式 brokerId guard 与 active alias 正向回归。active=yfinance + `brokerId=futu` 现在在 provider/helper/OpenD 读取前返回 409 `MARKET_DATA_CAPABILITY_UNSUPPORTED`，且 helper 请求数为 0；该条因 Rust 无 Go 同形 broker reader registry 仍是 partial。mapping/reuse/anchor 已同步，receiptDigest 保持空。
- 2026-09-27 API transport P1：`TestMarketQueryAndExecutionPayloadFallbacksRemainDeterministic` 补齐可迁移行情断言。新增 pathTail 正常/短路径回归与 `limit=0`/负数→1 的真实路由回归；旧实现先红为 `request.limit=200`，修复后 6/6 精准 nextest 通过。blank limit→默认 200、非法输入拒绝和反向时间窗 fallback 已有证据；execution payload/helper 语义因 Rust 无同形 seam 继续列为 partial，未伪造 receiptDigest。
- 2026-09-27 Backtest/Calendar P1：`TestHistoricalKLineSyncerRejectsBrokenPagination` 新增生产同步回归 `production_helper_sync_rejects_broken_pagination_cursors`，覆盖 missing `nextBefore`、向前 cursor 失败及到达 `since` 边界成功落库，定向 nextest 1/1 通过。映射保持 partial（Rust yfinance helper/durable task 与 Go 可注入 HistoricalKLineSyncer/source、futu provider、内存 progress seam 不同形），mapping/reuse/anchor 已同步，未伪造 receiptDigest。
- 2026-09-27 Broker disconnected/degraded P1：Go `broker-read.json` 与 servercore 断连测试冻结 13 路 200 degraded；Rust 生产 broker-read owner 由 `broker_read_routes_fail_closed_when_snapshot_port_is_unavailable`、`broker_read_fails_closed_without_trade_client`、`broker_klines_valid_request_fails_closed_without_historical_source` 与 securities missing-router 测试固定为 503 `BROKER_READ_UNAVAILABLE` 并保留上游错误。显式 test-cutover fixture 仍回放旧 200 envelope；不改生产 wire。相关 servercore/read-failure mapping 已改为 `boundary`，混合 service fallback 行保留 `partial` 并注明 source-gated 三路边界，receiptDigest 继续为空。
- 2026-09-27 Futu/OpenD runtime P1：`TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts` 已由真实 OpenD mock 覆盖 global-state probe → `SharedTradeReadRuntime` → production broker runtime route，断言 connected startup、`serverVersion=10.9.7000`、markets/health、2 个 discovered accounts 及 REAL 优先排序。`product_runtime_composes_opend_provider_and_fences_shutdown_ownership` 定向 nextest 1/1 通过，映射已由旧 account/order reconciliation 证据行升级为 `function_exact`；真实 receipt 为 `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`（[workspace receipt](verification-receipts/workspace-nextest-2026-09-27T061100Z.json)）。strict audit 当前仍有 4085 个历史 evidence/receipt gaps，未伪造其他 digest。

- 2026-09-27 provider health backoff P1：`TestProviderHealthRetryDelayBacksOffAndCaps` 先红后修。新回归 `helper_restart_policy_defaults_match_go_provider_health_retry_delays` 发现默认 500ms 与 Go 首次 100ms 不一致；修复 `HelperRestartPolicy` 与 managed helper restart policy 为 100ms 初始、1s 上限后，定向 engine nextest 2/2 与 sidecar 回归 3/3 通过，mapping 升为 `function_exact`。同文件 cancellation/error 透传条目仍为 partial；receiptDigest 保持为空。当前 `[~]` 积压按映射重算为 2955 条，严格审计 evidence/receipt gaps 仍未收口。

- 2026-09-27 strict evidence batch：人工逐条复核 17 条单引用 `function_exact`（Assistant/ADK 11 条，Strategy/Pine warmup 与 error normalization 6 条）的 Go 与 Rust 测试体；17 条均已有 `// Parity:` anchor、workspace receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8` 中的 passed test，且 `assertionCoverage.source=reviewed`。未覆盖子断言的 session-negative、空数组与 stream transport 条目未纳入本批。严格审计缺口由 4085 降至 4021，仍不能宣称整体 strict 通过。
- 2026-09-27 全量验证收尾：当前工作树 workspace nextest 3504/3504 passed、0 failed、2 suite skipped；七类 compatibility replay 全部通过。结构化 receipt 为 `sha256:2f7422488ce7addc2b81ad38563b3b7662345283896be959843301f2b609d66f`（[receipt](verification-receipts/workspace-nextest-2026-09-27T071942Z.json)）。当前 `[~]` 积压 2955 条；strict evidence/receipt gaps 为 4019，仍按缺口推进，不宣称严格审计完成。

- 2026-09-27 最新积压快照：API launcher 两条 `[x]` 已有源码锚点与 receipt `sha256:bb5be96baa9877eba7e6d5f9f0f3455fd7d3608f5912999bd6e53cfc8f3bd757`；MarketData/Calendar 三条单引用 `[x]` 使用 receipt `sha256:9d3a85b571117001d7de7cb3a4badbf5f9825c308f243b51763b87426dc22115`。`audit_test_parity.py --strict` 当前真实失败为 3957 个 gap，不能把 workspace nextest 全绿当作 strict parity 完成。
- 2026-09-27 Assistant session-context 最新收口：三条单引用 `[x]` 均已关联 receipt `sha256:917fa4ba6a8484f68cb156733271d11cc5da94a702b60e52d1f91229a1ea5078`（3/3 passed），strict gap 进一步降至 3948；其余历史 function_exact 仍逐条等待 reviewed/reuse/receipt 证据。
- 2026-09-27 Watchlist quote P1 收口：三条单引用 `[x]`（extended session projection、overlapping batch single-flight、provider cache policy）均已关联 receipt `sha256:ae6267da865221f941864dca401b67519a51c0a38b9c0069be36bd4010bad7e3`（3/3 passed）。当前 Go 4451、Rust 3380、`function_exact=1499`、`partial=2318`、`boundary=634`、`missing=0`；anchor `1791/1745/0/0/46`；strict gap 3927。SG.D05 timezone、metadata side effect 与 watchlist port-level fallback/permission isolation 仍是明确 partial，不纳入 exact。
- 2026-09-27 Calendar health strict evidence：三条单引用 `[x]` 已补 `assertionCoverage.source=reviewed` 并关联 receipt `sha256:27d4c070d9bbff94444923e57d51a73b4f62069b70fc6dd5fe357382c0e87bcc`（3/3 passed）；单引用 reuse/Parity anchor 已存在。strict gap 3921，剩余历史 exact rows 继续按 reviewed/reuse/receipt 缺口推进。
- 2026-09-27 Backtest retry P1 收口：`TestHistoricalProviderRetryExhaustionAndTimerCancellation` 的两条 helper owner 测试已通过定向 nextest 2/2；固定退避取消先红后修为取消感知轮询，映射由 partial 升为 `function_exact`。两个 Rust evidence 共用该 Go reference 的 `Parity:` anchor，reuse 为两条单引用，`assertionCoverage.source=reviewed`，receipt `sha256:26dc42121e439058156a76cfe41acd1d0c041eb7104b2f320d99861e119019dc`。当前 exact 1500、partial 2317、boundary 634，anchor `1792/1746/0/0/46`；strict gap 仍为 3921，下一批优先处理已有 anchor/reuse 的 strict evidence rows。
- 2026-09-27 Trading/Broker strict evidence C 批收口：三条已有 Trading exact（derivative single-leg preview、unreadable real-trade control plane、env fallback + US overnight normalization）完成 Go/Rust 断言复核，补 reviewed assertion 与 receipt `sha256:77f918d03712f238ef42d3340f4cc5c73f951a0406e534d74b4c3ad251cfe258`（3/3 passed）；三条 reuse 均为单引用且已由 `parity_anchor_reconcile.py` 记账。无生产变更，strict gap 由 3921 降至 3915，继续处理剩余历史 evidence/receipt 缺口。
- 2026-09-27 Query/MarketData strict evidence：逐条复核 `TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues`、`TestDecodeMarketCandlesQueryParsesRepeatedSessions` 与 `TestCandlesRouteNormalizesRepeatedSessions`。前两条保留 query owner 的空白/非法时间与 repeated-session 去重断言，补 `Parity:` anchor；路由条目先让 provider session 断言转红，修正为分页请求序列 `RTH/RTH/ETH/ETH/ALL/ALL` 并锁定去重后的三条唯一路由。三条定向 nextest 3/3 通过，receipt `sha256:6d383be156db7803a762e52849c2d20b09d245fe42aac931543f27d95ee59496`；共享 query/candle owner 的多引用 reuse 已 reviewed，相关 Go session 聚合差异继续保持 partial。当前 exact 1500、partial 2317、boundary 634；anchor `1793/1747/0/0/46`；strict gap 3901，整体仍未通过。
- 2026-09-27 API transport auth strict evidence：复核 Origin/CSRF 会话写、PATCH 会话写、浏览器禁用页（含 engine 状态）与密码会话读写五条 P1 exact；联合 `jftrade-api`/`jftrade-engine` nextest 5/5 通过，receipt `sha256:c5457db228673e47ae252a104992aa8c261fce3df081f60dfd28cf7c6682cc17`。五条 assertion 均标记 reviewed，保留现有 Parity anchor/reuse；strict gap 降至 3885，整体仍未通过。
- 2026-09-27 Assistant/API strict evidence：复核 `TestADKSessionsCRUDAndFilteringRoutes`、`TestChatStreamTransportHandlesDisconnectedClients`、`TestStreamReconnectAndSkillContracts`、`TestCatalogSessionRunAndObservabilityContracts` 与 `TestSessionTimelineFailureKeepsLegacyErrorCode`；复合映射的 composer、live-stream、disconnect、replay、catalog 与 timeline owner 全部纳入联合 engine nextest，8/8 通过。receipt `sha256:fb75c71c988292130d6e3f017e66f1a2a3de3f3d19b3ec106a6af3ca84da3c7e`；五条 assertion 标记 reviewed，strict gap 降至 3875，整体仍未通过。
- 2026-09-27 Backtest/API strict evidence：复核 `TestBacktestSyncRouteRejectsObsoleteSessionScope`、`TestSyncProgressAndCancelRoutesHandleSuccessAndNotFound`、`TestStatusResultAndDeleteRoutesCoverTerminalAndStoreFailures`、`TestHistoricalPageParsingRejectsMalformedProviderValues` 与 `TestStartScriptRejectsBlankResearchScript`；联合 engine nextest 覆盖 sync scope、progress/cancel、status/result/delete、provider page conversion 与 blank script，10/10 通过。receipt `sha256:fda37329aa034a644056012e122883e27d99a6586ad7bf82003d3af39f110f6a`；strict gap 降至 3870，整体仍未通过。
- 2026-09-27 MarketData boundary strict evidence：复核 candle period 空白/别名、provider generation fence、invalid sessions、subscription canceled mapping 与 logical cleanup 后 release/clear 五条 P1 exact；联合 engine nextest 6/6 通过，receipt `sha256:baa66579a6a1866149f08707d9e80790638ec1fb29f48ed34962a50ae5f17478`。五条 assertion 均完成 reviewed/passed receipt，strict gap 降至 3865，整体仍未通过。
- 2026-09-27 MarketData/API forwarding strict evidence：复核 news/corporate-actions 参数与转发、tick fresh-cache/strict-before、managed-account not-found 与 strategy activity pagination 五条 P1 exact；联合 engine nextest 11/11 通过（activity owner 在 lib 与 bounded integration binary 均通过），receipt `sha256:a583ff41724cfc3bcd259fcf9627a7c8594e6a9aa5c4a79e0082cf332a71c622`。strict gap 降至 3860，整体仍未通过。
- 2026-09-27 Execution/Watchlist strict evidence：复核 activity time/pagination、execution cancel envelope、broker empty arrays 与两条 watchlist business exact；engine/store-sqlite 联合 nextest 10/10 通过，receipt `sha256:53f26ac779a8505c741acb4ea4e9f6316da471fb6e04f0eeeea7b823f63e301b`。strict gap 降至 3855，整体仍未通过。
- 2026-09-27 Runtime/Maintenance strict evidence：复核 installers rollback、SQLite backup/cleanup、Futu quote health/disconnect、OpenD health、provider activation 五组 P1 exact；engine/store-sqlite/integration-futu/marketdata/desktop 联合 nextest 10/10（含 1 个 live OpenD ignored）通过，receipt `sha256:b58fc1c29238f5349c80b07263e7375b665440d25cbe46ae45201a4903967df6`。strict gap 降至 3850，整体仍未通过。
- 2026-09-27 Market HTTP strict evidence：复核 US intraday session labels/unknown-session error 与 snapshot fresh-cache、cache-miss、force-refresh 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:4c4bec4be84d55d241713c361ec13c8feb7011c4863b51821d1b51df8c679a80`。strict gap 降至 3845，整体仍未通过。
- 2026-09-27 Helper/runtime strict evidence：sidecar manager reuse、bounded kill escalation、Node macOS fallback/required aggregation、desktop password session 与 strategy runtime panic recovery 六条 P1 exact 已完成 reviewed assertion 与统一 receipt `sha256:52ef8cc1e2ccf7d57130269388da3d90544630132116f3cc4cf9c431d7de2c51`；marketdata-helper/engine 联合 nextest 7/7 通过。strict gap 降至 3833，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 API portfolio/settings strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:cf733b63c6b03f627d273983d92eff928ecb09de97b1f79be6c6c95d04e3ee76`（api/engine/settings-file 联合 nextest 5/5）；覆盖 browser-only security write、portfolio cash/degraded/removed route 与 broker settings defaults。strict gap 降至 3823，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu marketdata runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:57f5e6283bc3618f2b76411d6374d775dc85c6a8e939d6512d8c0615c06b4899`（integration-futu/marketdata 联合 nextest 5/5，live OpenD ignored）；覆盖 cache inheritance、fallback filtering/projection 与 generation fencing。strict gap 降至 3813，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Assistant boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:7f97c2a3cc83a59db7c1bfa55be2233314f69e89bf29de08c35c2ba56592506`（engine nextest 5/5）；覆盖 MCP auth/catalog、session context、input cancellation、approval resume 与 missing-run continuation。strict gap 降至 3803，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu K-line pagination strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:489957b0b5e2b5d8f04047abee16c357751def89d390ec871a2016be66b24c70`（engine/integration-futu 联合 nextest 5/5，live OpenD ignored）；覆盖 cursor/page normalization、market-time conversion 与 inclusive boundaries。strict gap 降至 3793，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 API/marketdata tail strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:7612368c8b4ebc983e552ba11b03a5ede9cad29575c83c5728c8578411901a69`（engine/marketdata 联合 nextest 5/5）；覆盖 provider retirement、status normalization、cookie-only WS、tick volume delta 与 pagination metadata。strict gap 降至 3783，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Backtest/Calendar/Settings strict evidence：五条 P1 exact 已完成 reviewed assertion 与 receipt `sha256:e2ffadfff2d2d62a4d697f5bf9a65f47f446f2b60316848ed77cac4d6517ae34`（engine/calendar/settings 联合 nextest 6/6）；覆盖 DST range、calendar validation/cache indexing、empty broker defaults 与 watchlist provider failure preservation。strict gap 降至 3774，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Trading/Broker control-plane strict evidence：五条 P1 exact 已完成 reviewed assertion 与 receipt `sha256:6246b9ab3c1d3050579515161c6005f3103a8eb2ac583573a6369db130eeb2bc`（engine nextest 9/9）；覆盖 control-plane availability/audit persistence、option combo lifecycle、forced reconciliation wake 与 capability route table。strict gap 降至 3764，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu subscription reconciler strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c4734a37b0151e32c4e87c9b949f9ae10fa286364e4e337484fd9eb1b2bb1577`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 deferred release、ack-based retention/retry、connection ownership cleanup 与 failed-record pruning。strict gap 降至 3754，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu OpenD boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:b10f6c20a8a268cddbaed2a56114e7e63c4a24caad8e1cd6a856d6c70d17c420`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 timeout/keep-alive、history decode、depth closed-session 与 trading disconnected boundaries。strict gap 降至 3749，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-27 Futu session/trade strict evidence：五条 Go 映射、七个 Rust owner tests 已完成 reviewed assertion 与 receipt `sha256:9b9427f38df767a2d63f217953fd27bae053fda603622c86d9a7ef33d51b3814`（engine/integration-futu 联合 nextest 7/7，live OpenD ignored）；覆盖 session normalization/defaults、previous-close rules、trade authority 与 CN location fallback。strict gap 降至 3739，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-gate strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c6e22cccbab1c95b191598b13684cc7d6632461fc80d06f8fbed43fb736981b2`（engine nextest 5/5）；覆盖 compaction gate、workflow parent、pending approval tail 与 active-run detection。strict gap 降至 3729，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu subscription/trade strict evidence：六条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:66b32e68a91fcc48f6557dcb436c9ea2d5d16af485d6418fcddc83bb83a25dc1`（integration-futu + engine nextest 6/6）；覆盖 subscription ack/retry、connection generation fencing、terminal close、failed-connect fail-closed 与交易写入断连前置条件。strict gap 降至 3723，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu protocol strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:6478da8aebc98d804488cf76f032ff5ceaf9cc33c3a3fd38aaff652d8355429a`（integration-futu nextest 5/5）；覆盖 prediction catalog identity/pagination、research request injection、history pagination/filter forwarding 与 user-security error preservation。strict gap 降至 3718，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-context strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:0ca3d7f2e3b1e564c6fc25e2a91823516d7de78fb8e56d03ee042f9a272c92da`（engine nextest 5/5）；覆盖 read-pressure projection、model auto-compaction 与 pending-approval protected-tail selection。strict gap 降至 3713，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu K-line boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:f38ccd784ac1bf1afd47e5a14600fc1f016af24883a0787fb35b97b78f55801a`（integration-futu + engine nextest 5/5）；覆盖 period mapping、cursor/time validation、pagination bounds 与 Session_ALL fallback。strict gap 降至 3708，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu snapshot/listener strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:5843770babe2087467db4d181f4c556dcea44dcf03a89385610e9ded8179dad8`（integration-futu + engine nextest 5/5）；覆盖 snapshot batching/cache、sliding budget/rate-limit/cancellation、empty result handling 与 basic quote malformed-row drop。strict gap 降至 3703，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant store-ops strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:66c0e83d636b8f8045e81d60c6cd382808d168af88f8d211cc8739dcdc9df635`（engine nextest 5/5）；覆盖 agent ownership guard、cancel/deny transaction、missing-target classifications 与 listing filters/sort。strict gap 降至 3698，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant session-context2 strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:83808bfd4b00fe6f1aacfe581f8ee1453badfe7659bff03d5d281583ba62163a`（engine nextest 5/5）；覆盖 provider override/revision compaction、handoff revision filtering、append visibility 与 pending approval preservation。strict gap 降至 3696，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:cbeb1497d9d8ee4db2dbafb58ab65670dd8abbd74d3c020007e4ab127b04b211`（engine nextest 5/5）；覆盖 tool ordering、cancellation join、probe timeout cap、approval lease cancellation 与 session title reuse。strict gap 降至 3695，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu history-window strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:603f4b41c4847b7f46c69791b46609ecb78503502e1118929ce1f5d5d9f09338`（integration-futu nextest 5/5）；覆盖 session planning、multi-page pagination/page limits、upstream page sizing 与 payload-less success normalization。strict gap 降至 3692，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant store-lifecycle strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:df78dcad3dcd67f0e6dc10d4c57da16dee947f4114ebfdeb509578e2df9bc07e`（store-sqlite + engine nextest 5/5）；覆盖 cascade cleanup、session/composer/approval store semantics 与 provider timeout normalization。strict gap 降至 3687，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant run-time strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:172dd3f940aca79bcb55c464e065550bf98f4273f0ff1559bba3f44ad9e5560c`（engine nextest 5/5）；覆盖 terminal cancellation audit、configured/per-run timeout windows、resume reset 与 expiry cleanup。strict gap 降至 3682，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu notification/probe strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:ae8a2c04479d9119e93e169d4f3e3a048488f478e60b591242891cc9c4040d6a`（integration-futu nextest 6/6，live OpenD ignored）；覆盖 notification payload/status routing、closed-port disconnected probe、program-status formatting 与 candle-session mapping。strict gap 降至 3672，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu subscription-reconciler strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:c3d81229c5939b1e25203f0f1c269d609600a0ed226da538c5fc7b1e8bd5e3e0`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 subscription sharing/deferred release、concurrent idempotence、retry ladder/reacquire、delayed fallback 与 connection quota reset。strict gap 降至 3662，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu client-recovery strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:8691574cb891a5f89c932ccd431f408bf4c3893654277016ba7e57c0c62ace54`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 replay-safe recoverable errors、session replacement、callback lock release、minimum version 与 typed transport failures。strict gap 降至 3652，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu quote/empty-boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:65f7118836c30241fe900c00e4cb632ce9c8ead298e4c7d90e93f87bab8c69c1`（integration-futu nextest 10/10，live OpenD ignored）；覆盖 empty normalization、order-book/basic-quote empty/rejection paths、duplicate quote projection 与 invalid/payload-less snapshot rows。strict gap 降至 3642，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Assistant engine-gates strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:484a697d804103853875d6d7e2d34429dc85981bc916d7568cb3e1fecca74a49`（engine nextest 5/5）；覆盖 strategy optimization persistence、MCP runtime lifecycle、agent/provider resolution、tool catalog availability 与 retryability envelope。strict gap 降至 3632，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu mixed-boundaries strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:77b7aeff1ac829b88bffc92e012affa9c99f2f1b01599c1d957e5d7a1cac473f`（integration-futu nextest 5/5，live OpenD ignored）；覆盖 tick fallback、subscription normalization/release、HK-only research state 与 quote-right cache refresh。strict gap 降至 3622，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 API market-runtime strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:ea1a72725d8f5cd97f2e15017b44283975b76e1aef4d2af17bab7a72ef24009b`（engine nextest 7/7）；覆盖 candle cache/provider fallback、research provider forwarding 与 strategy cancel dispatch。strict gap 降至 3612，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Futu research/boundary strict evidence：五条单引用 P1 exact 已完成 reviewed assertion 与 receipt `sha256:b32a88c5f551e7f3b00501146b2f5b3e32ec0a931078327803c0bcd36c2d493c`（integration-futu nextest 9/9，live OpenD ignored）；覆盖 K-line/price helper、research/calendar pagination、disconnected reads 与 fallback wire coercion。strict gap 降至 3602，仍有历史 evidence/receipt 缺口，整体未通过。
- 2026-09-28 Strategy/Pine targeted cancel P1：`TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders` 先补真实 execution store owner 断言，覆盖成功撤单移除 tracking、失败保留 tracking 与 foreign/untracked gateway 隔离；定向 engine nextest 3/3 passed，receipt `sha256:3423dac32787c4000997a77e747bdf13c64a024334cf4c27e9e341eb47d25fa3`。对应映射升为 `function_exact`；cancel-all 的 execution-store 成功终态仍保持 partial，不把 strategy owner 证据重复计入。
- 2026-09-28 Strategy/Pine cancel-all success P1：`TestLiveCommandExecutorCancelAll` 先补真实 store-backed gateway 终态断言，覆盖逐笔派发和成功清空 active ledger；定向 engine nextest 9/9 passed，receipt `sha256:fc615c228ba5af017d074b6793421fc3e624cc447a0abd801c9a8ad8c8eb6901`。对应映射升为 `function_exact`，失败撤单保留 tracking 仍由相邻回归覆盖。

- 2026-09-28 P1 Strategy/Pine lifecycle：已补 `Pause/Stop` 状态先写后停 runtime 与转换失败保留 owner 的真实回归，定向 nextest 4/4；映射仍为 `partial`。剩余证据缺口是 Go `WithLiveMarketStreamRefresher` 每次操作刷新两次的同形计数 seam，不将 router demand reconcile 过度宣称为 exact。

- 2026-09-29 P1 Strategy/Pine targeted cancel alias：先红复现陈旧意图 ownership error，随后在 `dispatch_cancel_intent` 增加确定性 clientOrderId alias 解析、internal id 去重和 stale no-op；定向 nextest 3/3 passed，receipt `sha256:a3e1e7ac8664d078feccbdd1fd234bb2abf8e0ec6e2e51a2d3ad5c43de7f9fc7`。映射保持 `partial`，剩余缺口是 Go 显式 `activeOrderAliases` 多腿持久化 owner 与复杂 OCO alias 回归。
## 2026-09-30 API SSE reviewed batch

- 已完成 5 条 API transport SSE 行为条目的 Go/Rust 断言复核与联合 nextest 5/5：`write_event_propagates_serialization_and_write_failures`、`stream_loop_ignores_trigger_without_callback`、`write_failures_are_reported_with_their_source_message`、`concurrent_writers_serialize_frames`、`write_event_returns_flush_panics_as_errors`。
- 共享 receipt：`verification-receipts/api-sse-reviewed-2026-09-30.json`，digest `sha256:23cb9085910da407cffce794533dee1ff1eea1154a3a50a766fa6afd67ed9ec9`；单引用 reuse 与 Parity anchor 均保留。
- strict gap 由 3395 降至 3391；剩余 API transport exact 继续按 reviewed assertion、anchor、reuse 与 passed receipt 收口，不能用 receipt 数量替代行为完成率。
## 2026-09-30 Futu transport reviewed batch

- 5 条 Futu transport exact 已补真实 Parity anchor、reviewed assertions 和联合 nextest receipt：`TestTradeReadMethodsPropagateTargetProtocolDisconnects`、`TestQuoteKLineAndOrderBookPropagateTargetDisconnects`、`TestTradeWriteMethodsPropagateAccountAndWriteDisconnects`、`TestTradeWritesAreNotReplayedWhenResponseIsLost`、`TestDirectSubscriptionCallsPropagateClosedClientErrors`。
- receipt：`verification-receipts/futu-transport-reviewed-2026-09-30.json`，digest `sha256:a0906c826c136f6c21afaaf156d85bc399947b9047e47431aa33fcc242ad4290`；strict gap 3385→3378。

## 2026-09-29 evidence closure checkpoint

本轮只计入有真实行为断言、reviewed assertion coverage、有效 Parity anchor 和 passed receipt 的 5 条 API SSE exact。联合测试覆盖 7 个 Rust 测试（含 router headers 与 retry=0 断言），receipt `sha256:8f5cf46fdb2e9bcd5af4d3de9b52781eb5cbbcc4af2ebdde8c267252e0c05ec4`。严格审计缺口由 3378 降至 **3368**；其余历史 exact 缺口继续列为 backlog，不以 Rust 测试总数、receipt 数量或单次 verification passed 代替完成率。

## 2026-09-29 Futu P2 evidence closure

本轮只把 3 条同形度足够的 Futu 行计入 reviewed：默认端口、订阅 frame、零价 previous-close。7 个 Rust 行为断言在联合 nextest 中通过（本轮单独执行 3/3），receipt `sha256:c121cb50db0c539b0df79c1bea408c2aaa59111be0295c6b9169b2bc875b30c2`；strict gap 实际下降到 **3353**。Pine asset 选择行保留其 `(Asset,false,nil)` 与 Rust typed error 的 seam 差异，未机械升 exact。

## 2026-09-29 partial correction

本轮明确记录两项剩余功能差异：日线 QueryKLines 的真实 OpenD client 返回断言、RequestHistoryKL 的 KLine/next cursor response 投影尚未由 Rust 同形测试证明。条目保留 `partial`，即使窄 helper/frame 测试通过也不计为 exact；strict gap 实际下降到 **3341**。

## 2026-09-29 remaining behavior gaps after review

- Futu funds：补 availableFunds 优先级、locked/maxWithdrawal、CN/MY 兜底和负 locked 夹零的同路径行为断言。
- Futu order mapping：补 Go 目标订单类型折叠、FOK/nil/DAY TimeInForce 与 margin account type 断言。
- Pine asset：Go 的 embedded FS 选择、missing/empty 返回空 Asset + false + nil 与 Rust typed error 仍为不同 seam；保持 partial。
- Futu history：真实 QueryKLines/RequestHistoryKL client response 投影仍为 partial。

本轮 reviewed exact 中 prediction push、Assistant research backtest、candle adjustment 都有真实 Rust 行为断言与 passed receipt；strict gap 当前 **3323**，整体未通过。
## 2026-09-29 Futu snapshot fallback reviewed batch

- 六条 Futu snapshot fallback exact 已完成断言复核并升级 `reviewed`：行与 market 分组、canonical/取消/错误与 clone、strict delayed quote fields、static-id 无订阅回退、TTL 正负缓存、StockScreen 错误传播。
- 定向 nextest 6/6 passed；receipt：`verification-receipts/futu-snapshot-fallback-reviewed-2026-09-29.json`，digest `sha256:5d2e8d55218adc5252b1389a258dbf15803b757a9bb05091ccd10b78c56cb8f6`。
- strict gap 由 **3323** 降至 **3312**。剩余 exact 仍需逐项补 reviewed assertion、有效 anchor 与 passed receipt；本批不以测试总数或 receipt 数量代表完成率。

## 2026-09-29 API auth/SSE reviewed batch

- SSE loop 两条与 auth middleware 三条已完成 reviewed assertion、anchor 与 passed receipt；receipt 分别为 `api-sse-loop-reviewed-2026-09-29.json`（`sha256:803a99b17ac95ec97490cbf3f1f91ee8c3aa95d061ee9966dfdd9217f3bae18e`）和 `api-auth-boundaries-reviewed-2026-09-29.json`（`sha256:d9491a62ef3b46d0ff5ee03f652748d32fa1bd219460c95a0edd7e3748702b89`）。
- `TestAuthSkipsPublicPaths` 与 `TestAuthProtectsLogout` 因 `/health`/204 断言和 logout 200/204 投影差异收窄为 reviewed partial。
- strict gap 由 **3303** 降至 **3297**；API 低测试比例仍是后续行为补齐重点。

`TestAuthRejectsNilAuthenticator` 与 `TestAuthRejectsUntrustedOrigin` 随后完成 reviewed 收口；2/2 nextest receipt `api-auth-rejections-reviewed-2026-09-29.json`，digest `sha256:d302cddef14057dec554dd8d48e00892a71e7c5a2e4c565fff7010970fbe800e`，strict gap 3297→3293。

## 2026-09-29 API P1 Web/Execution review

- 三条 Web 行为已 reviewed：disabled navigation page、cookie+CSRF browser flow、cookie-only WebSocket；receipt `api-web-p1-reviewed-2026-09-29.json`，digest `sha256:02d835502c6392862c7678ed3087c19e51198186c265a1583cc181bbac8d5b92`。
- ETH execution session 的 Rust normalization/wire 测试 3/3 passed，receipt `api-execution-session-reviewed-2026-09-29.json`，digest `sha256:8abeb1dae760d9861b6bd1f5070052482df98eb3047d517386db66084255dce8`；由于缺少 Go 同形 HTTP route，映射保持 reviewed partial。
- 前端资源完整矩阵、密码变化触发 session invalidation、POST logout route 均已纠正为 reviewed partial；strict gap 3293→3274。

启动 rollback 条目随后完成证据复核：engine nextest 2/2 passed，receipt `api-startup-rollback-reviewed-2026-09-29.json`，digest `sha256:5ff41a57feb4610daf6dfae3a59381aad5bf567d79b1f659a0da8068872fa066`。Rust 覆盖 production migration/resource rollback，但 Go generic Handle callback 与 error-chain seam 未同形迁移，映射保持 reviewed partial；strict gap 3274→3270。

## 2026-09-29 API transport P2 behavior review checkpoint

- 已复核 7 条既有 `function_exact` 的真实行为断言：request observability、非法 request id、route method/path isolation、Swagger core paths、system request id propagation、web login rate limit。
- 联合 nextest **9/9 passed**；receipt `api-transport-p2-behavior-reviewed-2026-09-29.json`，digest `sha256:ce452bd3c5eb1ac54fa17c8f9bd952a69c8e811dc9abc3b0bcd2e9503492a19d`。
- strict gap **3088→3065**，仅计 reviewed assertion +有效 anchor + passed receipt +审核 reuse 的实际收口；Rust 测试总数、receipt 数量和 verification passed 不作为完成率。
- assertionless exact 复核结果为 **0**；anchor reconcile `1894/1847/0/0/47`。后续优先清理剩余 legacy-conclusion exact 与 API transport wire owner 行为缺口。

锚点补齐后 strict gap 由 **3065 降至 3064**；当前 strict error 分类为 receipt 1146、reviewed assertion 1044、reuse 869、test filter 5，anchor 缺口为 0。

## 2026-09-29 API transport P2 runtime behavior checkpoint

- 8 条 runtime/strategy/settings exact 完成 Go 断言复核并升为 `reviewed`：provider switch、warming health、Node dependency diagnostics、strategy quantity sizing、broker normalization、combo quantity mode。
- 联合 nextest **12/12 passed**；receipt `api-transport-p2-runtime-reviewed-2026-09-29.json`，digest `sha256:652cbc5eae4d4533af8b22ff2ea8f098cb6398bea05bf7f3d6e9871403d18d6b`。
- strict gap **3064→3048**，只计真实行为测试、reviewed assertion、有效 anchor、passed receipt；测试总数与 receipt 数量不作为完成率。

## 2026-09-29 API datamigration P2 behavior checkpoint

- 8 条 SQLite 维护 exact 完成 Go 断言复核并升为 `reviewed`：backup retention/quota、incompatible snapshot、failed backup cleanup、rebuild selection、manifest drift、schema catalog。
- `jftrade-store-sqlite` 定向 nextest **8/8 passed**；receipt `api-transport-p2-datamigration-reviewed-2026-09-29.json`，digest `sha256:4d739bd52477eb28f8bf1dbdcaba199570f595d524ea28b7b89a9793aa5397a4d`。
- strict gap **3048→3032**；本轮仅按真实行为、reviewed assertion、anchor、receipt 收口计入。

## 2026-09-29 API datamigration safety checkpoint

- 8 条 rebuild-safety/backtest/broker route exact 完成逐项断言复核并升为 `reviewed`。
- `jftrade-store-sqlite`/`jftrade-engine` nextest **8/8 passed**；receipt `api-transport-p2-datamigration-safety-reviewed-2026-09-29.json`，digest `sha256:c91a4ac3fe696e1fbebd5897d1f96b81b9e42433c0ea3761390452bc25ae0119`。
- strict gap **3032→3016**；root-only skip 分支仍保留环境边界说明。

## 2026-09-29 API read/settings P2 behavior checkpoint

- 10 条 read/settings/runtime exact 完成断言复核并升为 `reviewed`：lookback、preview failure、sidecar stop、appearance/market fixtures、market profile、research preset、settings environment isolation、onboarding/readiness。
- 联合 nextest **10/10 passed**；receipt `api-transport-p2-readsettings-reviewed-2026-09-29.json`，digest `sha256:77a4805fce48b4565c50fe4d7475287a2977f08bae3486c1007be627fdbb65fc`。
- strict gap **3016→2996**，只计真实行为证据。

## 2026-09-29 API shared-owner P2 checkpoint

- 5 条 shared-owner exact 完成断言复核：backup marker retention、K-line explicit bounds、current-bar intent、depth method rejection、legacy source-format rejection。
- 8 个 owner tests nextest **8/8 passed**；receipt `api-transport-p2-shared-owners-reviewed-2026-09-29.json`，digest `sha256:9c451f9dc06c5c9679f7299c2d089593a3526086b535c497b1210fa8dc3f9080`。
- strict gap **2996→2986**；没有把共享测试命中次数当作完成率。

## 2026-09-29 runtime dependency shared-owner checkpoint

- Node probe OK/outdated/invalid/command-error 两条 exact 完成断言复核，shared owner reuse 已审核。
- `jftrade-engine` nextest **1/1 passed**；receipt `api-transport-p2-runtime-dependencies-shared-reviewed-2026-09-29.json`，digest `sha256:e12b481f0265f0680e5e6ca822d09c2ff5770ca08cd2924ddffe996a56c395a0`。
- strict gap **2986→2980**。

本轮补充审核两个已 reviewed owner 的 reuse 关系（optional query bool alias、candle adjustment normalization）；无新增行为测试，strict gap **2980→2976**，该下降仅表示 reuse 证据闭合，不计为新增功能行为。

## 2026-09-29 broker runtime correction

- 两条 broker runtime 旧 exact 因缺少同形 production HTTP route owner，降为 reviewed `partial`。
- Rust projection 字段仍有证据；真实 HTTP 200/ok envelope/assembly wiring 留在 backlog。
- strict gap **2976→2969**；该下降不计为新增行为。

## 2026-09-29 strict batch evidence update

Execution、Backtest、Strategy/Pine、Assistant workflow、Watchlist、Provider Research 六个 API 行为批次已逐项核对 Go assertions，并以真实 nextest owner 测试和 receipt 收口；本轮没有把文档行数、receipt 数量或 verification passed 当作完成率。6 个 batch receipts 已写入 `verification-receipts/`，对应 mapping 的 `assertionCoverage.source` 和多引用 reuse 已升为 `reviewed`。

strict gap 实际 **2969→2853**。剩余缺口仍主要是历史 `legacy-conclusion` assertion、未绑定 receipt 和未审核 reuse；API transport 低比例的行为补齐继续按 route owner 推进。边界/partial 引用未因共享测试而升级为 exact。

## 2026-09-29 live volume/heartbeat evidence update

OpenD live listener 与 ws-live fixture 的 13 条 exact 已完成逐项 assertion review、anchor/reuse 审核和真实 passed receipt。覆盖 volumeDelta/cumulativeVolume、超大累计量、trade/depth 同订阅投影及 heartbeat/通知 wire。strict gap **2853→2821**；没有把 fixture case 数或 receipt 数量作为完成率。

## 2026-09-29 execution validation evidence update

US price tick/session/market-code 两条 API exact 已以 engine 与 Futu wire owner 测试重新验证并绑定 receipt；严格审计 **2821→2805**。其他非 API 引用保持原结论，不因共享 owner 自动升级。

## 2026-09-29 system status evidence update

System status/runtime resource 的 4 条 exact 已完成真实 owner 测试、assertion review、anchor/reuse 审核和 receipt 绑定；status mapper 的非同形 DTO 边界继续保留 partial。strict gap **2805→2794**。

## 2026-09-29 receipt coverage repair

补跑缺失 Rust owner 并替换相关 receipts，修复 reviewed rows 的 testFilter 覆盖缺口；当前 reviewed exact 不再存在 `rustEvidence` 未包含于 `testFilter` 的 mismatch。strict gap **2794→2775**。

## 2026-09-29 ADK catalog evidence update

ADK catalog/middleware 的 3 条 exact 已完成 production assembly owner 测试、assertion review、reuse 审核和 receipt 绑定；strict gap **2775→2764**。当前 reviewed exact 的 `rustEvidence ⊆ testFilter` mismatch count 为 0。

## 2026-09-29 runtime resources/lifecycle evidence update

runtime resource ownership 与 lifecycle 两批已完成真实 owner 测试、assertion review、anchor/reuse 审核及 receipt 绑定；集中布局与 Go callback 形态差异保持 partial/boundary。strict gap **2764→2732**。

## 2026-09-30 API/Assistant evidence update

补跑 runtime lifecycle 遗漏 owner 后，按 API/Assistant route owner 批量收口 approval、workflow、chat stream、catalog、task/memory 与 provider 边界。所有本轮 exact 都有行为测试、reviewed assertion、有效 receipt 和 reuse 审核；严格 gap **2732→2657**。随后对 8 个共享 owner 做一致性 reuse 审计，gap **2657→2641**。没有把 receipt 数量或 Rust 测试总数当作完成率，partial/boundary 结论保持不变。

## 2026-09-30 transport/data-management evidence update

新增 API/Transport route owner receipt（13/13）并收口 9 条 exact；补齐 8 条 reviewed receipt 元数据后，strict gap **2641→2612**。新增 SQLite/data-management owner receipt（10/10），覆盖 schema 缺失、损坏 marker、备份配额、overview/cleanup 与 pending rebuild rollback，strict gap **2612→2592**。所有变化均以实际行为测试和 strict gap 下降为准。

## 2026-09-30 replay/assembly evidence update

重跑 backtest P1 旧 receipt 的 10 个 owner 并绑定完整当前 commit，strict gap **2592→2589**；重跑 Assistant P1 9 个 owner，并收口 5 个无共享 owner 的 assembly/MCP exact，strict gap **2589→2579**。仍有共享 owner 的 legacy 引用时，保留其原结论并继续列入 backlog。
## 2026-09-30 API marketdata forwarding batch

- 15 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 calendar/company/news/rankings/screen/index-constituents forwarding 及 capability、helper isolation、limit/page 边界。
- 定向 engine nextest **29/29 passed**；receipt：`api-transport-marketdata-forwarding-reviewed-2026-09-30.json`，digest `sha256:51abd6e018b42e2f4f3a8ee2acd76ecb8e53cc80fbc64c4a176cddc17e9a6528`。
- 15 条 mapping 已从 `legacy-conclusion` 升为 reviewed assertion，并绑定当前 commit/testFilter；15 个小 fan-out reuse relation 已审核。高 fan-out shared owner 继续保留待审，不因共享测试自动扩大 exact。
- strict gap **2554→2503**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API servercoretest batch

- 19 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 backtest sync、broker projection、system/strategy contract、settings、onboarding 与 watchlist runtime。
- workspace nextest **23/23 passed**；receipt：`api-transport-servercoretest-reviewed-2026-09-30.json`，digest `sha256:d0f9cbb88920c3bed3fa60b9a47c6fea80a6e277a0b7ad06e844d33bff7f2463`。
- 19 条 mapping 已升为 reviewed assertion 并绑定当前 commit/testFilter；17 个小 fan-out reuse relation 已审核。高 fan-out shared owner 继续保留待审，不因共享测试自动扩大 exact。
- strict gap **2503→2441**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API servercore behavior batch

- 17 条 API Server/Transport Wire `function_exact` 完成 Go 断言到 Rust owner 的逐项复核，覆盖 data-management、live volume、notification、capability catalog、strategy runtime/trading、OpenD health、security 与 strategy delete。
- workspace nextest **33/33 passed**（多 target 的同名 owner 均纳入 receipt）；receipt：`api-transport-servercore-reviewed-2026-09-30.json`，digest `sha256:022cdab8ccecceae349bab2bb53c3a545b4aee938098f712474abbc84edbc1f1`。
- 17 条 mapping 已升为 reviewed assertion 并绑定当前 commit/testFilter；19 个 fan-out ≤6 reuse relation 已审核。高 fan-out shared owner 继续保留待审。
- strict gap **2441→2376**；全局 strict 仍失败，下一批优先清理 API transport 高 fan-out reuse 与剩余 legacy assertions。
## 2026-09-30 API marketdataapp behavior batch

- 19 条 API Server/Transport Wire `function_exact` 完成 provider switch、sidecar、health、search、depth/kline 与 subscription 行为复核。
- workspace nextest **22/22 passed**；receipt：`api-transport-marketdataapp-reviewed-2026-09-30.json`，digest `sha256:af682a98cf08d8157b78645a0ea1ce63ce87e8196669be186e1d333de14313bc`。
- 19 条 mapping 升为 reviewed assertion 并绑定当前 commit/testFilter；19 个 fan-out ≤6 reuse relation 已审核，高 fan-out shared owner 继续保留待审。
- strict gap **2376→2308**；全局 strict 仍失败。
## 2026-09-30 API remaining legacy exact closure

- 收口 API Server/Transport Wire 最后 19 条 legacy `function_exact`，覆盖 application、startup、Futu probe、lifecycle、status、combo、web auth 与 settings。
- workspace nextest **25/25 passed**；receipt：`api-transport-remaining-reviewed-2026-09-30.json`，digest `sha256:848b85e4751fa1f1c3addb93b9188624f8de95c12413857c5f367ea7ad4be9e7`。
- API transport owner 的 `legacy-conclusion` exact 已清零；21 个 fan-out ≤6 reuse relation 已审核，高 fan-out 与其他领域 legacy 继续保留。
- strict gap **2308→2244**（reuse 566、receipt 880、assertion 798）；全局 strict 仍失败。
## 2026-09-30 Strategy/Pine parse batch

- P1 `pkg/strategy/pine/parse_test.go` 的 21 条 `function_exact` 完成 Go 断言到 Rust owner 复核，覆盖 parse/analyze/validate、metadata、history、request.security、advanced indicator/order 与 risk declarations。
- workspace nextest **21/21 passed**；receipt：`strategy-pine-parse-reviewed-2026-09-30.json`，digest `sha256:fa86abe492eedff72da09451189fa1e17fd7f07e0bd0059b65ee2efbd77bd483`。
- 21 条 mapping 升为 reviewed assertion；13 个低 fan-out reuse relation 已审核，framework-language 47-way high fan-out 继续 backlog。
- strict gap **2244→2188**；全局 strict 仍失败。
## 2026-09-30 Strategy/Pine live execution batch

- 7 条 P1 Strategy/Pine live execution `function_exact` 完成 Go 断言复核，覆盖 stop/reduce-only、risk reason、instance scope、entry/close sizing 与缺失 quantity 拒绝。
- workspace nextest **19/19 passed**（多 target 实例）；receipt：`strategy-pine-live-execution-reviewed-2026-09-30.json`，digest `sha256:dc4d66ed418c004d700714a02b97e5da22571edb94f878e88eb175d7d1f265c6`。
- strict gap **2188→2172**；Strategy/Pine 仍有 42 条 legacy exact，高 fan-out reuse 继续单独审查。
## 2026-09-30 Strategy/Pine risk and order-boundary batch

- 8 条 Strategy/Pine `function_exact` 完成 risk mode、qualified position、order metadata/trailing boundary 与 truncation 断言复核。
- workspace nextest **10/10 passed**；receipt：`strategy-pine-risk-order-reviewed-2026-09-30.json`，digest `sha256:a8d9603e2f3f9cd70b93eac40093f44326cab698c15a00a7f628552b41606de6`。
- 8 条 mapping 升为 reviewed assertion；5 个低 fan-out reuse relation 已审核；strict gap **2172→2152**。

## 2026-09-30 Strategy/Pine legacy exact closure

- 29 条已有真实 owner 的 Strategy/Pine `function_exact` 已完成 reviewed assertion 与通过 receipt；对应 strict gap **2139→2067**。
- 仍未自动放行高 fan-out shared owner；需要后续把共享引用按 Go assertion 分组复核，再绑定同一批完整 receipt。

## 2026-09-30 API transport envelope/reuse review

- `TestResponseEnvelopeWriters` 已补齐 Rust envelope owner 的 404 `NOT_FOUND/resource not found` 行为断言并升级 exact。
- auth/origin/CSRF/CORS/SSE 的 9 个低 fan-out reuse relation 已完成 reviewed；高 fan-out relation 保留 backlog。
- 本轮严格 gap **2067→2053**；API transport 仍有大量 partial 与高 fan-out reuse，不能以 10.6% 数量比例视为完成。

## 2026-09-30 API logout HTTP projection

- `TestWebLogoutClearsSessionCookie` 已补真实 product HTTP response 的 Set-Cookie 断言，并结合 manager token invalidation 证据升级为 exact。
- auth-session route fixture 的 3-way shared owner reuse 已 reviewed；strict gap **2053→2052**。
- `TestAuthProtectsLogout` 保持 partial：Go 中间件 stub 的 204 与生产 logout endpoint 的 200 是明确边界，不以同名测试强行升级。

## 2026-09-29 批量证据收口与 API fan-out backlog

- 日历 45 条、存储 28 条、设置 16 条 legacy exact 已由真实 owner 测试与通过 receipt 批量升为 reviewed；对应测试分别为 47/47、30/30、19/19。
- API receipt refresh 重新验证 16 个 engine owner 测试并修正 11 条旧短 commit receipt；所有更新 mapping 均包含可执行 testFilter、40 位 verifiedCommit 与 raw NDJSON digest。
- 低 fan-out API relation 已 reviewed 27 条；剩余严格缺口集中在高 fan-out shared owner、历史 receipt 与 assertion review，不能用 relation 数量替代行为审查。
- strict gap 链：**908→818→762→730→689→678**；下一批优先逐组复核高 fan-out owner 的 Go assertion 分组，并补 API Server/Transport 的真实行为测试。

## 2026-09-29 受控 fan-out 与历史 receipt 继续收口

- API runtime 8 条旧 receipt（10 tests）、Assistant 30 条旧 receipt（30 tests）均以当前 40 位 commit 重新验证；未改变任何 partial/boundary 结论。
- system 9 条、backtest 4 条、researchscreen 7 条 legacy exact 已批量升为 reviewed，并分别保存可追溯 NDJSON receipt。
- 仅 3 个经过人工逐引用核对的高 fan-out owner relation 设为 reviewed；instrument search、settings product、watchlist、maintenance 等高 fan-out 仍明确列在 backlog，避免批量证据替代行为审查。
- strict gap 链更新为 **908→818→762→730→689→678→670→640→614→600→595**。

## 2026-09-29 小模块 reviewed 与 API catalog relation

- asset/security/retry/Futu integration 的 16 个 owner 测试全部通过；settings/watchlist/research/desktop 的 18 个 owner 测试全部通过；对应 receipt 已写入 mapping。
- 另有 8 个小模块 mapping 由真实 9-test 批次升为 reviewed；不是 receipt 数量完成率，而是每条 mapping 都绑定自己的行为 testFilter。
- API catalog instrument-search 两个 shared owner relation 经人工逐引用 reviewed；settings product、watchlist 与 maintenance 的高 fan-out relation 保持 unreviewed backlog。
- strict gap 最新为 **509**，剩余以 reuse（高 fan-out）、receipt 和 assertion review 分类处理。

## 2026-09-29 Assertion review closure batch

- 已关闭 13 条 `function_exact` assertion review 缺口：对应真实 Rust owner 均在当前工作树定向 nextest 通过，receipt `assertion-review-2026-09-29.json`。
- strict gap 实际从 **468 降至 455**；当前剩余项分类为 **455 条 shared-owner reuse relation**，需要按 `referenceKeys` 逐引用核对后才能将 `reviewStatus` 升为 `reviewed`。
- 不以测试数量、receipt 数量或 verification passed 代替行为证据；未逐引用核对的高 fan-out owner 保持 backlog。

## 2026-09-29 API/settings shared owner review

- `product_server_persists_ui_settings_and_reports_actual_port` 的 20-way relation 已完成逐 `referenceKeys` 审核并记录 reviewNote；4 条 exact gap 释放，16 条 partial 继续 backlog。
- strict gap **455→451**。下一优先级转向 watchlist read 与 maintenance/API transport owners，继续按全量引用比较后放行。

## 2026-09-29 API/datamigration maintenance owner batch

- 8 个维护 shared-owner relation 已逐 referenceKeys 审核并记录 reviewNote，释放 9 条 exact 引用；partial/boundary 仍保留各自缺口。
- strict gap **451→442**。下一批继续处理 API marketdata forwarding/cache 与 watchlist shared owners。

## 2026-09-29 API/watchlist read owner

- watchlist read owner 的 11-way relation 已逐引用审核，释放 1 条 exact；其余 10 条 partial/boundary 仍需实际行为补齐。
- strict gap **442→441**，继续处理 API marketdata forwarding/cache shared owners。

## 2026-09-29 API marketdata forwarding/cache owners

- 9 个 API marketdata shared owner relation 已逐项审核，释放其 exact references；剩余差异集中在 provider facade 组合、collector/push 分离、TTL/时间语义和 conversion 聚合边界。
- strict gap **441→429**，继续处理 calendar/company/news/index forwarding 与 transport wire owner。

## 2026-09-29 API forwarding-wire owners

- 18 个 API forwarding shared owner relation 已完成逐 referenceKeys 审核，释放 34 条 exact；保留 provider capability、conversion aggregation、collector/push separation 与 sidecar facade partial 差异。
- strict gap **429→395**。剩余重点转向 strategy/runtime shared owner、Assistant shared owner 与 transport lifecycle。

## 2026-09-29 API/runtime tail and candle validation

- API/runtime tail 的 6 个 relation 与 candle adjustment 4-way relation 均逐引用审核；对应 owner tests 8/8 与 1/1 通过，所有 partial/boundary 差异保留。
- strict gap **395→377**。当前剩余以 Assistant/MCP/strategy shared owner 为主，继续按 fan-out 逐组收口。

## 2026-09-30 Assistant MCP policy owner

- MCP loopback peer 与 Host rebinding 的 2 个 shared relation 已 reviewed，释放 4 条 exact；MCP lifecycle manager 的 disabled/port-conflict/transport 分支仍待 owner review。
- strict gap **377→373**。

## 2026-09-30 Assistant shared owners

- claims/lease, tool failure, timeout, approval, workflow threshold 与 continuation supervisor 的 9 个 relation 已 reviewed，释放 18 条 exact；partial failure/stub/long-running 结论未改变。
- strict gap **373→355**，下一批优先处理 MCP/application adapter/tool-catalog shared owners。

## 2026-09-30 Futu/marketdata shared owners

- research/basic quote/batch snapshot/embedded research 的 4 个 relation 已 reviewed，释放 12 条 exact；research catalog 的非法 market 组合仍为 partial。
- strict gap **355→343**。

## 2026-09-30 Futu/engine exact-pair owners

- 21 个 2-way/2-exact relation 已 reviewed，释放 42 条 exact；下一批继续处理高 fan-out mixed partial owner 与 Assistant application/MCP。
- strict gap **343→301**。

## 2026-09-30 Futu triple owners

- 7 个 Futu 3-way relation 已 reviewed，释放 14 条 exact；剩余主要是 Assistant application/MCP 和 mixed partial owner。
- strict gap **301→287**。
## 2026-09-30 Assistant MCP server owner batch

- MCP server owner 的 14 个 shared relation 已逐项对照全部 `referenceKeys`，并以同名 Rust owner 行为测试复核 account/portfolio、strategy/backtest/model、listener、catalog、dependency、unsafe-host 与 workflow wait 结论。
- 定向 engine nextest **14/14 passed**；receipt `assistant-mcp-server-owner-reviewed-2026-09-30.json`，digest `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`。
- strict gap **287→272**。HTTP fetch、schema、product dispatch 及其他高 fan-out owner 仍保留 backlog；未用 receipt 数量替代行为覆盖判断。

## 2026-09-30 Assistant application/tool-catalog owner batch

- 16 个 fan-out ≤4 的 application/tool-catalog relation 已逐项对照 `referenceKeys` 并运行对应 owner：workflow、execution/trade、market candle/backtest、portfolio/research、optimization、catalog、capability 与 instrument boundary。
- 定向 engine nextest **17/17 passed**；receipt `assistant-application-owner-reviewed-2026-09-30.json`，digest `sha256:9d64884dbd4dfa97317b1272f9a616f8980daf39aea3b663c90748e49c5b3c69`。
- strict gap **272→256**。10-way strategy binding、ADK runtime 高 fan-out 与 mixed partial 继续 backlog；未用 receipt 数量替代行为覆盖判断。

## 2026-09-30 ADK runtime pair owner batch

- 20 个 fan-out=2 的 ADK runtime relation 已逐项对照 `referenceKeys` 并运行 input/approval/lease/projection/terminal/session owner；高 fan-out runtime/store relation 保留 backlog。
- 定向 engine nextest **20/20 passed**；receipt `assistant-adk-runtime-pairs-reviewed-2026-09-30.json`，digest `sha256:4a81b9bc4b2324ea45c9ed00b94198fe11e0766718c7e061596c2136ce0af21a`。
- strict gap **256→236**，仍只按实际行为、reviewed relation、有效 receipt 计数。

## 2026-09-30 API route pair owner batch

- 20 个 fan-out=2 的 API/Transport relation 已逐项对照 `referenceKeys` 并运行 execution/system/marketdata/Futu route owner；partial/provider 边界保持原结论。
- 定向 workspace nextest **20/20 passed**；receipt `api-route-pairs-reviewed-2026-09-30.json`，digest `sha256:5e63078e658a06423b3c1b0453baeb53f72de24a2e26a4da3faea60c003c084c`。
- strict gap **236→216**，未用测试数量或 receipt 数量替代行为覆盖判断。

## 2026-09-30 API route tail owner batch

- Futu notification/quote-right labels 与 settings backtest-provider atomic preparation 两个 fan-out=2 relation 已逐项复核。
- 定向 workspace nextest **2/2 passed**；receipt `api-route-pairs-tail-reviewed-2026-09-30.json`，digest `sha256:a65d2d16e2e20bb0f2a9e74fdb690e8733f1cf4623ddc3da7c1f92c38f5c00d6`。
- strict gap **216→214**，其他 API 高 fan-out 与 mixed partial 保留 backlog。

## 2026-09-30 API route triple-owner batch

- 14 个 fan-out=3 的 API/Transport relation 已逐项对照 `referenceKeys` 并运行 execution/marketdata/trade/Futu/maintenance/watchlist owner。
- 定向 workspace nextest **14/14 passed**；receipt `api-route-triples-reviewed-2026-09-30.json`，digest `sha256:5548a060148522457f38e479d3d4d3ef8efa868c9137f035ccf75a79d75585ad`。
- strict gap **214→200**，higher fan-out 与 mixed partial 仍保留 backlog。

## 2026-09-30 ADK runtime triple-owner batch

- 13 个 fan-out=3 的 ADK runtime relation 已逐项对照 `referenceKeys` 并运行 input/turn/expiry/fencing/gate/terminal/handoff/session owner。
- 定向 engine nextest **13/13 passed**；receipt `assistant-adk-runtime-triples-reviewed-2026-09-30.json`，digest `sha256:40a8bbf45d8b4d060d82e0b10b6ba4240623818c154fc9145587541e96f1a333`。
- strict gap **200→187**，higher fan-out runtime/store relation继续 backlog。

## 2026-09-30 Assistant claims/runtime owner batch

- 6 个 `jftrade-assistant` fan-out=2–4 relation 已逐项对照 `referenceKeys` 并运行 workflow/claims/runtime owner。
- 定向 workspace nextest **6/6 passed**；receipt `assistant-claims-owners-reviewed-2026-09-30.json`，digest `sha256:d100bdc4c3bf6e5e5c77e48d8db495259f294334cae5137a63752f23cd182cf3`。
- strict gap **187→181**，高 fan-out ADK/store relation继续 backlog。

## 2026-09-30 ADK store owner batch

- 7 个 SQLite ADK store fan-out≤4 relation 已逐项对照 `referenceKeys` 并运行 atomic projection/provider/artifact/session/writer/approval owner。
- 定向 workspace nextest **7/7 passed**；receipt `adk-store-owners-reviewed-2026-09-30.json`，digest `sha256:98fd30e706a5cae7d009a6e00852927d73aaee0352a081f637926548e9e22885`。
- strict gap **181→174**，高 fan-out store relation继续 backlog。

## 2026-09-30 Futu/OpenD pair owner batch

- 20 个 Futu/OpenD fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 quote/health/search/kline/session/order-book owner。
- 定向 `jftrade-integration-futu` nextest **20/20 passed**；receipt `futu-opend-pairs-reviewed-2026-09-30.json`，digest `sha256:687a59f3da757e1fbf5c57147f130f111fc42f726855149bd81cad3f625482a1`。
- strict gap **174→154**，其余 Futu 高 fan-out与mixed partial保留 backlog。

## 2026-09-30 Futu/OpenD pair owner batch 2

- 第二组 20 个 Futu/OpenD fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 quote-rights/snapshot/subscription/trade/watchlist/recovery/prediction owner。
- 定向 `jftrade-integration-futu` nextest **20/20 passed**；receipt `futu-opend-pairs-2-reviewed-2026-09-30.json`，digest `sha256:2c24a6e94b730a3dad00c289924da912d36acdfec2e161f307f1f47fc0f56a70`。
- strict gap **154→134**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Futu/OpenD pair tail owner

- user security group type encoding/projection 的最后一个 Futu/OpenD fan-out=2 relation 已逐项复核。
- 定向 `jftrade-integration-futu` nextest **1/1 passed**；receipt `futu-opend-pair-tail-reviewed-2026-09-30.json`，digest `sha256:1f56d2b79405bbad59b530e99ddcb46d9ac8e5589a56c759a6b1636dd90c2290`。
- strict gap **134→133**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Cross-domain pair owner batch

- 跨领域 20 个 fan-out=2 relation 已逐项对照 `referenceKeys` 并运行 calendar/ADK/execution/marketdata/research/capability owner。
- 定向 workspace nextest **20/20 passed**；receipt `cross-domain-pairs-reviewed-2026-09-30.json`，digest `sha256:11200ac86d98093d5f2a7439a503bd0412ad759a418ff2bfaa4dbbbed90f49d8`。
- strict gap **133→113**，高 fan-out与mixed partial继续 backlog。

## 2026-09-30 Futu/OpenD triple owner batch 2

- `referenceCount=3` 的 16 个 Futu/OpenD、marketdata、SQLite relation 已逐引用复核并升为 `reviewed`；只释放对应 16 条 `function_exact` exact reuse，未改变已有 partial/boundary 结论。
- 定向 workspace nextest **16/16 passed**；receipt `futu-opend-triples-2-reviewed-2026-09-30.json`；raw output digest `sha256:7564322fb6753e5a1262f184c00d6d07694179d6a6f606f295dd7c648e7f2933`。
- strict audit **72→56**；剩余 56 条为未审核的高 fan-out/shared owner relation，继续按 referenceCount 分批处理。测试数量比例、receipt 数量和 verification passed 不计为完成率。

## 2026-09-30 Mixed high-fanout owner batches

- 两批共 40 个 shared owner relation 完成真实 owner 复核：第一批 nextest **21/21 passed**、释放 25 条 exact（strict **56→31**）；第二批 **20/20 passed**、释放 20 条 exact（strict **31→11**）。receipt 分别为 `mixed-highfanout-owners-reviewed-2026-09-30.json` 与 `mixed-highfanout-owners-2-reviewed-2026-09-30.json`。
- 最后一批 Futu/research 11 个 owner **11/11 passed**，receipt `final-futu-research-owners-reviewed-2026-09-30.json`，strict **11→0**。此处的完成条件是逐项行为证据收口，不是测试数量或 receipt 数量。

## 2026-09-30 API transport SPA boundary

- `frontend_spa_fallback_respects_path_and_accept_boundaries` 先红后修：旧 router 对 `Accept: application/json` 仍返回 SPA，修复后按路径与 Accept 矩阵返回 HTML/404；`TestShouldServeFrontendIndexRequestBoundaries` 已升级为 reviewed `function_exact`。
- receipt `api-transport-spa-boundary-reviewed-2026-09-30.json`，nextest **1/1 passed**，strict 保持全量通过；API 数量比 10.7% 仍只是风险信号。

## 2026-09-29 API provider-test wire batch

- `TestProviderAndAgentValidationContracts` 的 provider probe 路由已补真实 HTTP 行为证据：默认 quick、full、invalid slow 与 missing-provider 502 envelope 均逐项断言。
- 先红后修未知 provider 状态码，定向 nextest **4/4 passed**；receipt `api-provider-test-wire-reviewed-2026-09-29.json`，digest `sha256:b4ce4838b0f6ebe37e9474caabda06913c14ab226e3fbcc7ba332fd324ee6800`。
- 该 partial 已升为 `function_exact`；strict function_exact 证据从 **1492→1493**，strict audit 通过。剩余 API transport backlog 继续按真实 wire 差异推进。

## 2026-09-30 API assistant chat/SSE wire batch

- `TestChatAndSSEContracts` 已补齐 chat JSON envelope 与成功 SSE wire 行为：真实 production composition 断言 `200 + ok=true`、`text/event-stream`、配置化 `X-ADK-Stream-Idle-Timeout-Ms=420000`，以及 session→run→terminal 顺序与 durable session id。
- provider fixture 使用两条 loopback Responses 连接；provider failure 的 JSON 投影不与成功流头部混用。定向 engine nextest **1/1 passed**；receipt `api-assistant-chat-sse-contract-reviewed-2026-09-30.json`，digest `sha256:ef17475a3ca52c8d11254d0a921de827174661872903d9d93a6ea34a09c353f8`。
- partial→`function_exact`，strict function_exact 证据 **1493→1494**；strict audit 通过。剩余 dispatcher/WS/backtest boundary partial 继续保留并按行为缺口推进。

## 2026-09-30 API live WebSocket shutdown lifecycle batch

- `TestHandlerConnectionLimitAndCloseLifecycle` 已由两个真实 owner 测试覆盖：`ws_live_transport_rejects_origin_and_limit_without_leaking_permits` 锁定 503 limit、permit release 与再次握手，`ws_live_shutdown_closes_active_connection_and_releases_depth_subscription` 锁定活动连接 close、`1001` 原因、连接清零和 depth demand 释放。
- 红测先发现测试夹具不支持 extended-length masked frame，随后补齐标准长度编码；第二次红测发现 shutdown 前可能有 queued text frame，测试改为排空后检查 close。生产路径最终 **2/2 passed**；receipt `api-live-handler-close-lifecycle-reviewed-2026-09-30.json`，digest `sha256:14c38cbc91d70bb6b9a627c8fddd509294d9899c7a119ba0bf6c20079930b181`。
- partial→`function_exact`；strict function_exact **1494→1495**，partial **2322→2321**。同一 limit owner 与 servercore partial 的多引用 reuse 已显式标记 reviewed，未扩大 servercore 的诊断字段结论。
## 2026-09-30 API bindings required-path wire batch

| BindURI required path + escape matrix | 1 mapping（由 partial 升 function_exact） | `product_query::uri_escape_validation_accepts_literal_percent_and_rejects_malformed` + `transport_contracts::missing_path_parameter_returns_not_found_json_without_dispatch` | API nextest 3/3 passed；receipt `api-bindings-wire-exact-reviewed-2026-09-30.json`；digest `sha256:df775fcfd9a151371616e8dfe64aef9996d0f1c84088b7d4925d8cd6eba2ec0f` | 先红后修确认认证先于路由；已认证缺参请求断言 404 JSON envelope 与 port 不调用，strict function_exact **1495→1496**，数量比例不作为完成率 |
## 2026-09-30 Daily candle assertion review

- 冻结 Go `market_http_test.go:91` 的断言逐项核对：成功返回一根 daily candle，candle 与 meta 均省略 session，meta.extendedHours=false。生产 read owner 测试补齐 array length、meta.session 缺省和 extendedHours=false；首次即绿，无生产差异修复。
- 定向 nextest 1/1 通过；mapping 从 partial 升为 reviewed function_exact，partial 2321→2320，exact 1496→1497；strict audit 必须以本轮实际输出确认。receipt `api-daily-candle-reviewed-2026-09-30.json` 文件 digest `sha256:81066e73e3ad596e37e434d16a68862032bf1e46f5b7f1ac386fede7d975b832`。
- 修正上一轮 bindings mapping 的 receiptDigest：日志 rawOutputSha256 仅描述日志，receiptDigest 应为 receipt 文件 SHA-256（`b0b57dcbc9ccdaa5869ff5c59955dc4cc97f97a109dfc0bb59f83dbe6b9bfad0`）。
- 全量 check:rust 本轮仍在 target-health 失败（至少 50000 rcgu.o）；check:quick 实际执行后在 check:zero-go 失败。两项均未记为通过。
## 2026-09-30 System hard-stop handler boundary review

- 冻结 Go `routes_test.go:102` 直接调用 handler；DELETE 请求 URL 不代表注册路由。Go 与 Rust 公开 release 路由均为 POST `/{hardStopId}/release`。修正原映射把 handler seam 差异当作公开 route 差异的结论。
- Rust dispatch 增加 `%20/%20%20/%09` 空白 id 回归：400、BAD_REQUEST、hard stop id is required，零 port 调用；已有 quantity/notional=0 拒绝保留。新断言首次通过，无生产差异。
- 定向 nextest 1/1 通过，receipt `api-system-blank-hard-stop-reviewed-2026-09-30.json`（文件 digest `aabb091fc34e9a5dc1fff7211d4291cb6aca91d450bdd2fa4dcfe28d6ff21ac0`）。assertionCoverage 从 legacy-conclusion 升 reviewed；独立无 param handler seam 保留 partial，不宣称 exact 或整体完成。
- 本批 strict gap 没有下降；实质变化是空白 id 防御行为被断言、错误差异结论被纠正。后续需继续模块批量补 wire 行为，而非以 receipt 或通过数量计完成率。
## 2026-09-30 Execution ETH session route batch

- 对照 Go `exec_validate_test.go:88` 的 HTTP status、ETH session、fillOutsideRTH 三项断言，新增真实 Product POST 测试锁定 200 与 raw session；既有 parser/to_trade_request/Futu wire 测试锁定 ETH→session=2、fillOutsideRTH=true 及协议编码。
- 先红后修核对 owner：HTTP write port 接收 raw payload，不能错误要求其携带下游计算字段 `fillOutsideRTH`；移除错误断言后组合测试 **3/3 passed**。receipt `api-execution-eth-session-wire-reviewed-2026-09-30.json`，文件 digest `sha256:ef814b566fa23db1756adfc112360616db9f75b2193b496ee280ff98aad9086c`。
- mapping 从 partial 升 reviewed function_exact，strict function_exact **1497→1498**，partial **2320→2319**（随后 daily candle 批次已将 partial 更新为 **2318**）；数量比例不作为完成率。
## 2026-09-30 Assistant catalog HTTP composition batch

- `TestCatalogSessionRunAndObservabilityContracts` 由 port-level partial 补为真实 Product HTTP composition：同一 ProductionAdkPort seed 下逐一请求 12 个 catalog/session/run/observability GET 路由，断言 HTTP 200 + `ok=true`，并断言 DELETE provider 200 + `ok=true`。
- 定向 nextest 1/1 passed；receipt `api-adk-catalog-http-reviewed-2026-09-30.json`，文件 digest `sha256:8249f1afebb9345f3c0a4bb8fefb4eab02ac313e3706f75b39fdc26e5283e383`。
- mapping 从 partial 升 `function_exact`，strict function_exact **1498→1499**，partial 实际 **2318→2317**；数量比例不作为完成率。
