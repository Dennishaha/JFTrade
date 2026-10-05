# Go to Rust 迁移验证矩阵与后续任务指引

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

> 本文是历史迁移核查的任务入口，不是当前架构事实源或发布授权。分卷保留原始分析，未经复核的推演不得当成已复现缺陷，也不得直接照搬其中的修复方案。

> **2026-09-06 再核查**：下文历史“全部关闭”结论已被新复现推翻。订单无候选误判 FAILED、属性猜配外单、身份作用域以及查询截断聚合栏均有修复，详见[本轮行为复核](2026-09-06-behavior-audit.md)。旧表保留历史记录，不能据其 PASS 宣告完整兼容。

- 文档复核日期：2026-09-05。
- 本次代码核查基点：`8c9e0464a7242a0fbea693a55cf122d753b43717`；工作树包含既有未提交文档，非干净发布候选。
- 原文历史基线：`origin/go@452dea11`（标注为 `v0.27.0`）；本次未重新验证该 ref/tag 对应关系，历史行为比较须先验证基线。
- 范围：10 个领域、16 个既有风险编号（4 P0、9 P1、3 P2）。编号保留用于追踪，**不代表已确认严重度或正式门禁结果**。

## 2026-09-29 严格证据批次（当前工作树）

本批以行为测试、reviewed assertion、anchor、reuse relation 与 passed receipt 共同判断：Pine client validation 由 partial 升为 reviewed `function_exact`；API runtime/market snapshot/WebSocket、desktop readiness 8 条 exact 绑定 7 个真实 nextest（7/7 passed）；Pine indicator EMA/MACD 2 条 exact 绑定 2/2 passed。strict gap 实际 **2052→2047→2046→2041**，审计仍未通过。当前映射为 `function_exact=1491`、`partial=2325`、`boundary=635`；测试总数与 receipt 数量不作为完成率。
- 事实源：[仓库指令](../../../AGENTS.md)、[模块表](../../../scripts/module-map.json)、[质量门禁](../../architecture/quality-gates.md)、[发布资格](../../architecture/release-qualification.md)。

## 2026-09-29 严格证据增量

本轮不以测试数量或 receipt 数量计完成率。
本批后续核验：API provenance 批量复核与 8 个共享 owner relation 审查完成；broker/market-rule 12/12 与 Futu watchlist 7/7 真实测试通过，strict gap 实际 **2041→2036→2010→1998**。当前仍有 1998 条 strict evidence gaps，保留高 fan-out unreviewed relation 与 partial/boundary 结论。
在前一批 28 条基础上，继续复核 Assistant route、Swagger/OpenAPI contract、webaccess transport boundary 共 12 条已有真实行为映射；三组目标 nextest 分别 4/4、4/4、4/4 通过，`manual-test-mappings.json`、report、inventory 与锚点对账已同步。strict gap 实际由 3112 降至 **3088**，剩余历史 evidence/receipt 缺口继续按行为批次推进。

## 2026-09-30 API observability strict evidence

复核 `pkg/observability/observability_test.go:113:TestRecorderBoundsErrorsSlowRequestsAndOpenDHealth` 的有界错误、慢请求、OpenD correlation 与 minimum-importance 断言；真实 `jftrade-api` owner 与 WebSocket tail 一并 nextest **2/2 passed**。该 mapping 已补 `assertionCoverage.source=reviewed`、runner/toolchain/commit/receipt 元数据，receipt `sha256:9fa21653293b25a5387a6e258ea46c4d0a7fda346e4e40f018408ad33b8ae02a`。strict gap **2565→2563**；本轮只计行为测试、reviewed assertion 和有效 receipt，严格审计仍未通过。

API runtime 批次继续复核路径环境覆盖、相对 settings 资源派生和 strategy preview warmup 三条 exact；4 个 owner 测试 **4/4 passed**，receipt `sha256:48d34e9714e6c67134a3a60cd34c18276898c03791c202e5e4a156ddd37d4b07`。三条 mapping 升为 reviewed，strict gap **2563→2557**；剩余差距仍按严格审计实际收口。

API frontend asset 行为批次新增 root/index、静态 `app.js`、SPA fallback 与缺失 JSON 资源的 wire 断言，3/3 passed；两条 Go partial 升为 exact，但 strict gap 没有下降，按规则只记录为行为覆盖增加。随后 `TestClientRegistryTracksActiveInstruments` 使用独立 Rust owner，联合 API tail 4/4 passed，receipt `sha256:8c885bf0c5b09e5243b1ef111134b9805bf761ded361160755046f4bb1417eb2`，strict gap **2557→2554**。

## 一、已核实事实与重要勘误

| 原记录 | 本轮修正及证据边界 |
| --- | --- |
| 2,624 个纳管源码文件 | 本次 `git ls-files` 得到 2,624 个**纳管文件**，包含文档和配置；不是源码数量，也不覆盖未纳管文件、依赖缓存或发布包。 |
| 278 条路由 100% 兼容 | [生产 manifest](../../../crates/jftrade-engine/src/product_production_route_manifest.json) 的 `operations` 为 278。数量相同不能证明方法/路径集合、认证、DTO、错误、流式行为及副作用兼容。 |
| 前端实际调用 265 条，13 条均为功能盲区 | 原统计缺少脚本、排除规则与输出证据，暂不采信覆盖率。生成类型中的路径不是运行时调用；未直接调用可能是合法替代入口。分卷 10 的 13 条仅作逐项核查候选。 |
| WriterLease 通过 POSIX fcntl 绝对杜绝双写 | [实现](../../../crates/jftrade-owner-lock/src/lib.rs) 使用 `File::try_lock`，锁文件后缀为 `.jftrade-owner.lock`。不要硬编码未核实的系统调用；单属主保证要求所有写入路径遵守同一锁协议，不能约束绕过租约的外部写入。 |
| 美股开盘桶为 09:00–10:00 UTC，100% 崩溃/500 | 原文混淆本地时间和 UTC。[聚合实现](../../../crates/jftrade-store-sqlite/src/backtest_market_data_aggregation.rs) 确有 UTC 分桶及完整数量/连续性校验；缺覆盖返回错误不等于进程崩溃，也未证明所有 HTTP 路径返回 500。 |
| 只要 broker_order_id 为空就进入 UNKNOWN | [对账实现](../../../crates/jftrade-engine/src/product_production_ports_execution_reconciliation.rs) 同时检查数值 ID 与 `broker_order_id_ex`；两者均不可用才进入身份未知分支。不能由 UNKNOWN 直接推导重复下单或爆仓。 |
| 前端解锁缺口意味着所有实盘报单必然失败 | 本次搜索仅在生成类型中发现 unlock，未发现业务调用；需通过 UI 场景确认。影响取决于账户、交易环境与已有解锁状态；规范路径参数是 `{brokerId}`。 |
| 四平台为 macOS arm64/x64、Linux、Windows | 当前[升级基线](../../../tests/fixtures/release/upgrade-baselines.json)列出 linux-x64、macos-arm64、windows-x64、windows-arm64。正式验证以当前发布配置为准，不沿用原平台枚举。 |
| 静态 Zero-Go 审查证明发布包无残留 | 必须分别记录源码门禁与实际传入发布产物的扫描结果；本次没有扫描正式候选包。 |
| 完成四个 P0 即取得发布资格 | 错误。source admission、candidate evidence、publish、post-release validation 是独立边界，详见第五节。 |

### 修复建议的安全边界

1. **聚合**：先确定交易所日历、session scope、桶锚点及末尾短桶语义；不能简单删除完整性校验，否则真实缺失分钟会被掩盖。按开盘锚定的小时桶与整点截断桶是不同产品语义。
2. **对账**：保留不确定状态的 fail-closed 行为；仅在券商支持且证据唯一时反向绑定。禁止仅凭标的、数量、时间窗口猜配订单，禁止身份未知时自动重报。
3. **子进程恢复**：先追踪桌面/独立 API 的实际进程所有者。已有 `ProcessSupervisor` 抽象不证明生产运行中自动重启；缺少某个统计方法调用也不能单独证明全系统无法恢复。避免再建第二个 supervisor。
4. **跨库删除**：多个连接各自提交不等于跨库原子事务；先定义归属、保留策略、可重入清理及崩溃补偿，不预设 ATTACH 能解决所有 WAL/恢复约束。
5. **迁移**：不预先指定 v5 或承诺所有历史版本都支持；以受支持升级基线和真实 schema 为依据。新增 SQLite schema 必须取得明确需求授权。
6. **Worker**：重复、乱序和同时间戳修订应分别定义，不能统一静默丢弃；图形截断或压缩不能未经验证就视为超限修复，更不能丢弃 order intents。
7. **EMA**：`(1 - 2/201)^200 ≈ 13.53%` 是递推模型下初始差值的剩余权重，不是实际指标相对误差。种子、有效样本与 PineTS 行为须实测；不直接强制 500–1000 根。
8. **打包**：不采用原文未经验证的 CentOS 7/manylinux2014 统一打包建议；Python wheel 兼容策略不能代替整个 Tauri 应用的目标平台验证。

## 二、任务状态与证据规则

下表保存历史 16 项关闭声明，不是当前重新验收结果。2026-09-06 已重开订单恢复与聚合边界核查；当前证据与剩余限制以[本轮行为复核](2026-09-06-behavior-audit.md)为准。

状态流转：待验证 → 已复现 / 不成立 / 范围外；已复现 → 修复中 → 待验收 → 已关闭。
- **已复现**：精确 SHA、输入 fixture、复现命令、预期/实际结果及影响范围齐全，再确认严重度。
- **不成立**：提供反证代码及对应测试；不能只写“已有实现”。
- **范围外**：提供支持范围依据及风险接受记录，不能冒充已修复。
- **已关闭**：修复 SHA、回归测试、门禁结果和残余限制齐全；正式发布证据另行管理。
- 原分卷的复选框是拟议用例，不是测试运行记录；旧行号仅用于定位，后续用符号和实际调用链重新核实。

## 三、16 项任务台账

所有条目初始均未指派；接手时按第四节记录负责人和证据。P0/P1/P2 仅保留原编号。

| ID / 领域 | 待证明的问题 | 最小验收条件与禁止事项 | 验证状态与闭环依据 |
| --- | --- | --- | --- |
| P0-01 / [04 时间](./verification-matrix/04-backtest-time-dst.md) | 常规时段 1m→60m 请求是否因桶边界拒绝有效数据 | 同时验证本地/UTC、DST 前后交易日、短交易日、真实缺分钟及 HTTP 错误映射；缺数据仍须可识别。 | **已关闭 / PASS**：会话感知聚合与 09:30 本地锚定闭环，DST 转换日与短交易日自适应，真实缺分钟严格 fail-closed，4 项专项测试通过。 |
| P0-02 / [05 对账](./verification-matrix/05-broker-reconciliation.md) | 券商接受但本地未记录回执的崩溃窗口 | mock 券商计数 + 重启同库；覆盖两类订单 ID、唯一/多候选/无候选、已成交及撤单；不重复报单，不误认领。 | **已关闭 / PASS**：`recovery.rs` 崩溃自愈恢复逻辑闭环，支持活动/历史快照去重、本地已占 ID 外单保护、Priority 1/2 消歧，14 项专项测试通过。 |
| P0-03 / [06 进程](./verification-matrix/06-sidecars-resilience.md) | Helper/Node 退出后恢复链是否闭环 | 桌面和独立 API 分开测试；退出、卡死、连续启动失败、主动停机；单一进程属主、退避上限、健康恢复、会话重建且无重放订单。恢复时限先定义，不沿用未经批准的 5 秒要求。 | **已关闭 / PASS**：单属主生命周期 Supervisor 守护闭环，500ms~10s 指数退避重启自愈，崩溃后 clean reopen 与四重防历史订单重放屏障，21 项专项测试通过。 |
| P0-04 / [10 前端](./verification-matrix/10-zero-go-tauri-frontend.md) | 锁定券商的用户解锁路径是否缺失 | mock 下验证锁定/已解锁、错误密码、取消、超时、模拟账户；凭据不落日志/持久化，不因解锁重试重复下单。 | **已关闭 / PASS**：零依赖 RFC 1321 MD5 实现，`useBrokerUnlock` + `BrokerUnlockDialog` 补齐主动解锁（工具栏未解锁徽标）与被动拦截（400/502/超时/密码错误脱敏处理），模拟盘免解锁，凭据即时擦除，全套 15 项专项测试通过。 |
| P1-01 / [04 时间](./verification-matrix/04-backtest-time-dst.md) | regular/extended 的桶边界是否符合约定 | 同一 fixture 对照盘前/常规/盘后；允许的 extended 数据不能被误判为污染；与 P0-01 一起验证。 | **已关闭 / PASS**：盘前与常规会话物理分界截断，盘前数据不再污染 09:30 官方开盘价，与 P0-01 联动闭环。 |
| P1-02 / [05 对账](./verification-matrix/05-broker-reconciliation.md) | 交易 Push 是否进入生产对账链、轮询延迟是否满足要求 | 先查协议解码、路由及订阅；重复/乱序/丢 Push/断线重订阅测试；Push 与轮询共用唯一投影写入者并保留兜底。 | **已关闭 / PASS**：Push 即刻唤醒对账 Worker（响应延迟 < 100ms），15s 轮询兜底，乱序成交回执经 `covered_by_snapshot` 幂等防重，单写属主 `WriterLease` 杜绝写冲突，TC-D5-01/02/04 及专项测试 5 项全部通过。 |
| P1-03 / [07 ADK](./verification-matrix/07-adk-leases-approvals.md) | 租约过期接管是否可能重复执行外部工具 | 故意阻塞模型/存储并接管，统计外部调用；验证 fencing 和幂等边界，不能把本地 token 当作外部 exactly-once 保证。 | **已关闭 / PASS**：Fail-closed 屏障原子阻断（`UNKNOWN` 状态）、迟到提交 fencing 拦截（`LeaseLost`）、`ADK_TOOL_OUTCOME_UNKNOWN` 映射闭环，10 项专项测试通过。 |
| P1-04 / [07 ADK](./verification-matrix/07-adk-leases-approvals.md) | 删除会话是否遗留应删除的跨库数据 | 先确认事件/工件归属与保留策略；每阶段失败后重启重试，验证无误删、无不可恢复半清理及共享工件损失。 | **已关闭 / PASS**：`delete_session_cascade` 跨库按 `artifact` $\to$ `session` $\to$ `adk` 顺序级联物理删除，严格保护 `session_id != 'user'` 共享工件，移除先验 404 检查彻底扫荡孤儿残余，全套 25 项跨库级联、崩溃注入与对抗压力测试全部通过。 |
| P1-05 / [08 SQLite](./verification-matrix/08-sqlite-schemas-migrations.md) | 实际事件查询是否缺索引及存在性能回退 | 核实当前 schema、SQL 与 EXPLAIN QUERY PLAN；固定数据量/查询基准，确认收益后再申请迁移，不仅凭索引名判断。 | **已关闭 / PASS**：实测 `events` 主键首列为 `id` 导致 `WHERE session_id = ?` 触发全表扫描与临时 B-Tree 排序，反证 Go 历史提议索引因谓词不匹配依然扫描，证明瘦索引 `(session_id, timestamp ASC, id ASC)` 提速 14.4x~48.7x 且消除内存排序，避免大文本宽覆盖索引写放大，4 项基准与执行计划测试通过。 |
| P1-06 / [08 SQLite](./verification-matrix/08-sqlite-schemas-migrations.md) | 受支持旧库是否缺升级路径 | 从真实受支持基线建立 fixture；升级、重复打开、迁移中断、备份恢复、较新版本拒绝降级；不重建历史安装包作为发布基线。 | **已关闭 / PASS**：澄清 Go 历史基线无增量迁移而 Rust 采用三层回滚与在线备份保护，实测 `strategy (v1->v2)` 与 `adk (v2->v4)` 升级及幂等重开，语法注入触发原子事务回滚，多层严格阻断高版本降级，3 项专项测试通过。 |
| P1-07 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | append 拒绝后 Rust/Node 会话状态是否失配 | 重复、乱序、修订、非法数据分别测试；检查失败后 append/open/close、内存与会话数，不能把保留有效会话直接认定为泄漏。 | **已关闭 / PASS**：修复非递增时间戳校验前置导致的 `liveSessions` 泄漏，添加 `openLiveSession` 冲突自愈清理与 `resultMarker` drawings 增量去重切片，消除实盘策略 `HALTED` 死锁崩溃风险，单元测试通过。 |
| P1-08 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | 实盘与回测预热是否产生不可接受偏差 | 同脚本/数据/种子比较指标及信号，定义容差与首个可交易 bar；记录样本不足策略及资源成本。 | **已关闭 / PASS**：完成 EMA/RMA 指标衰减残差数学推导与阶跃响应仿真，确认 200 根预热残差为 13.4%（RMA 36.7%），证明 700 根（$3.5 \times N$）可达 $<0.1\%$ 容差并给出策略首个可交易 Bar 准入基准，专项测试通过。 |
| P1-09 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | 实际 gRPC 发送/接收配置及超限恢复 | 查明两端实际限额，在阈值上下测请求/响应、错误映射及会话恢复；完整保留 intents，压缩不能替代限额测试。 | **已关闭 / PASS**：在 `normalizeVisualOutputs` 中引入 1000 个绘图对象 FIFO 上限（TV 标准 500 lines/boxes），实测负载限制在 ~200KB（< 5% 4MB 阈值），彻底消除海量图形导致 gRPC 报文溢出与会话异常注销，专项测试通过。 |
| P2-01 / [01 Pine](./verification-matrix/01-pine-runtime.md) | 崩溃后预留是否占用配额、能否安全回收 | reserve/submit/persist 各点故障注入及日期边界；先排除券商已接受，不能盲目释放不确定订单配额。 | **已关闭 / PASS**：消除 2x 重叠计数，SQL 锚定匹配 `'%reservation: ' || a.detail || ')%'` 杜绝子串碰撞，与对账联动安全回收，4 项专项测试通过。 |
| P2-02 / [06 进程](./verification-matrix/06-sidecars-resilience.md) | yfinance 冷启动下历史 Futu 订单可否对账 | 冷启动与切源分开；有/无在途订单、OpenD 不可用、账户发现；行情源独立于交易能力，失败状态可见。 | **已关闭 / PASS (事实反证)**：架构实现 OpenD 交易会话与行情源解耦，冷启动仅要求 `futu_trade_enabled` 即可初始化交易会话与对账后台，6 项既有测试反证闭环。 |
| P2-03 / [10 发布](./verification-matrix/10-zero-go-tauri-frontend.md) | 候选包是否符合各平台运行与签名要求 | 当前四平台实际安装/升级/回滚、无开发环境启动、sidecar 完整性、签名/公证/updater 及产物 Zero-Go；严重度按失败影响重评，不默认仅为维护项。 | **已关闭 / PASS (工程门禁与资格边界闭环)**：`check:zero-go` 严格通过（2661 源码与产物文件 0 残留），4 平台 Sidecar 运行时打包与冒烟测试全过，Tauri 发布资格遵循 `release-qualification.md` 四层独立边界，签名凭据保护与 13 项盲区路由治理闭环，专项测试通过。 |

## 四、后续任务执行方式

### 建议批次与依赖

1. **证据基线**：固定实际 SHA/工作树状态，核实历史 ref、支持平台、已有测试和 278 路由集合；重算前端调用分类。
2. **优先核查资金与数据安全**：P0-02 + P2-01、P1-03、P0-01 + P1-01；先给出复现/反证，再做最小修复。
3. **运行恢复与交互**：P0-03 + P2-02、P0-04、P1-02；P1-07 应与 Node 恢复协议协调，不能各加一套恢复逻辑。
4. **存储与 Worker**：P1-04/05/06 协调 schema 所有权与迁移计划；P1-07/08/09 共测预热、报文大小和会话生命周期。
5. **候选验证**：P2-03 及第五节。上述批次不是工期承诺，也不自动创建候选分支、tag 或发布。

### 已闭环任务台账记录 (P0-02, P2-01, P1-03)

#### 1. P0-02 券商接受但在本地记录回执前崩溃窗口对账自愈

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P0-02 / worker_m1_remediation & worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 基线 commit `ccac83d1`，新增 `crates/jftrade-engine/src/product_production_ports_execution_reconciliation_recovery.rs`，接入 `product_production_ports_execution_reconciliation.rs:165` |
| 状态 / 确认严重度 | 已关闭 / P0 (阻止崩溃后订单重复提交导致的双重成交与资金损失) |
| 生产调用链 / 所有者 | `product_production_ports_execution_reconciliation.rs` -> `resolve_unidentified_submission` -> `execution.db` 单一写属主（`WriterLease`）+ Futu `TradeReadPort` |
| 复现或反证 | 在本地未落库外部订单号前注入崩溃，旧逻辑在缺少 `broker_order_id` 时直接将订单置为 `UNKNOWN` 放弃对账，触发策略重试；反证测试证实实现自愈恢复后，单候选自动绑定 ID，多候选歧义置 `UNKNOWN` 保持现场，零候选置 `FAILED` 安全触发配额释放。 |
| 修复 / 回归 | 修复代码：`crates/jftrade-engine/src/product_production_ports_execution_reconciliation_recovery.rs`；回归测试用例（14 项，位于 `product_production_ports_execution_reconciliation_order_state_tests.rs`）：<br>• `reconciliation_crash_window_recovers_submitting_order_and_binds_broker_ids`<br>• `reconciliation_crash_window_recovers_filled_order_and_reconciles_fills`<br>• `reconciliation_partial_fill_during_crash_window_recovers_to_partially_filled`<br>• `reconciliation_no_candidate_transitions_to_failed_for_safe_quota_reclaim`<br>• `reconciliation_foreign_order_protection_ignores_unmatched_remark`<br>• `reconciliation_claimed_order_exclusion_protects_against_cross_binding`<br>• `reconciliation_priority_1_matches_client_order_id_in_remark`<br>• `reconciliation_priority_1_matches_even_beyond_300s_window`<br>• `reconciliation_timestamp_window_boundary_301s_vs_299s`<br>• `reconciliation_extended_id_matching_and_binding_without_numeric_id`<br>• `challenge_edge_case_1_three_identical_broker_orders_no_client_id_remains_unknown`<br>• `challenge_edge_case_2_different_symbol_and_conflicting_remark_not_claimed`<br>• `challenge_edge_case_3_broker_network_failure_does_not_mutate_or_release_quota`<br>• `challenge_edge_case_4_order_state_transitions_filled_cancelled_rejected` |
| 门禁 | `cargo test -p jftrade-engine --lib reconciliation` (43 passed, 0 failed, 退出码 0)；`pnpm run check:rust:static` 退出码 0 |
| 剩余风险 / 依赖 | 1. 并发盲发多笔无 remark 同属性同价格订单在崩溃前受理时，识别为歧义并保持 UNKNOWN，需人工审计；<br>2. 恢复依赖券商当日活动与历史订单可查窗口，依赖 15 秒轮询健康守护。 |

#### 2. P2-01 Pine 运行时每日配额生命周期与并发安全

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P2-01 / worker_m1_remediation & worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 基线 commit `ccac83d1`，修改 `crates/jftrade-store-sqlite/src/strategy_runtime_observation.rs:72-76` |
| 状态 / 确认严重度 | 已关闭 / P2 (消除配额统计 2x 重叠与 SQL 子串碰撞，杜绝超额报单) |
| 生产调用链 / 所有者 | `strategy_runtime_execution.rs:495` -> `strategy_runtime_observation.rs:reserve_daily_order / count_daily_orders` -> `strategy_runtime.db` Immediate 事务 |
| 复现或反证 | 无锚定 `LIKE '%' || a.detail || '%'` 使得在途 `:1:1` 预留被已提交 `:1:10` 的 detail 匹配为真，导致在途预留被提前丢弃，限额 2 时第 3 笔错误放行；反证测试 `test_stress_quota_reservation_substring_collision_counter_evidence` 证实修复后第 3 笔必须返回 `Err(Conflict)`。 |
| 修复 / 回归 | 修复代码：严格锚定子串查询 `(r.detail = a.detail OR r.detail LIKE '%reservation: ' || a.detail || ')%')` 并消除 2x 双重计数；回归测试用例（4 项，位于 `strategy_runtime_observation.rs`）：<br>• `test_reserve_daily_order_no_double_counting_and_safe_reclaim`<br>• `test_stress_quota_reservation_substring_collision_counter_evidence`<br>• `test_stress_quota_lifecycle_exact_counts`<br>• `test_stress_concurrent_quota_reservations` |
| 门禁 | `cargo test -p jftrade-store-sqlite --lib strategy_runtime_observation` (4 passed, 退出码 0)；`cargo test -p jftrade-store-sqlite` (15 test suites passed, 退出码 0)；`pnpm run check:rust:static` 退出码 0 |
| 剩余风险 / 依赖 | 孤儿预留的安全释放依赖对账将零候选订单置为 `FAILED`；若对账网络持续中断，预留保留至当日零点重置，属于预期 fail-closed 保守防线。 |

#### 3. P1-03 ADK 慢推理租约失窃与外部工具防重 Fencing 屏障

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P1-03 / worker_m2 & worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 基线 commit `ccac83d1`，修改 `crates/jftrade-store-sqlite/src/adk.rs`、`crates/jftrade-engine/src/product_adk_model_runtime.rs`、`crates/jftrade-engine/src/product_adk_model_runtime_tool_loop.rs`，新增测试模块 `product_adk_model_runtime_fencing_tests.rs`、`product_adk_model_runtime_takeover_tests.rs` 及集成测试 `tests/adk_lease_fencing_takeover_edge_conditions.rs` |
| 状态 / 确认严重度 | 已关闭 / P1 (消除租约超时接管后外部工具二次执行与过期结果迟到覆写风险) |
| 生产调用链 / 所有者 | `product_adk_model_runtime_tool_loop.rs` -> `claim_tool_invocation_if_status_and_revision` / `commit_tool_result_if_status_and_revision_with_event` -> `adk.db` 单一写属主 |
| 复现或反证 | 原逻辑在租约超时后直接以新 owner 重新派发 `Execute`，导致外部副作用工具（报单）被二次执行；修复后原子置为 `UNKNOWN` 并返回 `AdkToolInvocationClaim::Unknown`，接管 Worker 物理调用计数严格为 1，原 Worker 迟到 commit 严格被 `Err(AdkStoreError::LeaseLost)` 拒绝。 |
| 修复 / 回归 | 修复代码：区分 replay_safe 工具，租约超时原子 fail-closed 屏障，Fencing Token 与 Run Lease Token 校验；回归测试（10 项）：<br>• `product::product_adk_model_runtime::fencing_tests::fail_closed_lease_takeover_blocks_duplicate_tool_execution_and_stale_commit`<br>• `product::product_adk_model_runtime::fencing_tests::multiple_workers_simultaneous_takeover_after_lease_expiry_never_executes_fail_closed_tool`<br>• `product::product_adk_model_runtime::fencing_tests::takeover_worker_never_invokes_external_tool_for_expired_running_invocation`<br>• `product::product_adk_model_runtime::takeover_tests::stale_worker_late_result_commit_never_succeeds_under_any_takeover_or_expiry_condition`<br>• `product::product_adk_model_runtime::takeover_tests::replay_safe_tools_re_execute_on_takeover_and_deduplicate_subsequent_claims`<br>• `tests/adk_lease_fencing_takeover_edge_conditions.rs` 5 个集成测试 |
| 门禁 | `cargo test -p jftrade-engine --test adk_lease_fencing_takeover_edge_conditions` (5 passed, 退出码 0)；`cargo test -p jftrade-engine --lib product::product_adk_model_runtime` (26 passed, 退出码 0)；`pnpm run check:rust:static` 退出码 0 |
| 剩余风险 / 依赖 | 本地 SQLite Fencing 阻断了本地进程和接管 Worker 的重复发起与覆写；外部券商若本身不支持 ClientOrderId 幂等性，已飞往外部的请求需结合 P0-02 对账扫描解决。 |

#### 4. P0-01 & P1-01 回测时间桶聚合与夏令时对齐 (DST & Session 语义)

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P0-01 & P1-01 / worker_time_dst & worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 基线 commit `ccac83d1` / `415eb996`，修复提交 `0cc6d60b`（修改 `crates/jftrade-store-sqlite/src/backtest_market_data_aggregation.rs`、`crates/jftrade-store-sqlite/src/backtest_market_data.rs`、`Cargo.toml`，新增测试 `tests/backtest_market_data_session_dst_aggregation.rs`） |
| 状态 / 确认严重度 | **已关闭 / PASS** (P0-01: P0 级，消除 60m/小时桶常规回测 100% 崩溃拒绝；P1-01: P1 级，消除盘前污染官方开盘价风险) |
| 生产调用链 / 所有者 | `BacktestMarketDataStore::read_candles / query_candles` -> `aggregate_range` -> `resolve_aggregation_buckets` -> `aggregate_bucket` -> `backtest.db` 单一写属主（`WriterLease`） |
| 复现或反证 | **P0-01 复现**：旧逻辑采用纯 UTC 取模，在美股 09:30 开盘时（EST 14:30 UTC / EDT 13:30 UTC）将首桶计算为 14:00/13:00 UTC，而开盘前半小时无常规数据，`rows.len() == 30 != 60` 直接抛出 `Coverage("missing 60m coverage")` 硬错误崩溃；<br>**P1-01 复现**：在 extended 模式下执行 60m 聚合，09:00~09:30 盘前数据与 09:30~10:00 常规数据混入同一桶，`open` 被 09:00 盘前价（100）污染，未反映 09:30 官方开盘价（500）；<br>**修复与反证**：实现交易所会话感知桶解析（支持 US/HK/CN 交易时区与夏冬令时动态切换，包含短交易日提前收盘），首桶精确锚定 09:30 本地对应 UTC；盘前与常规交易时段在 09:30 处物理切分边界；同时强约束红线测试证实，真实缺失分钟（如 09:45 或尾盘短桶 15:50 缺失）100% 精确抛出 `Coverage` 错误，严禁掩盖数据缺失。 |
| 修复 / 回归 | 修复代码：`crates/jftrade-store-sqlite/src/backtest_market_data_aggregation.rs`（`resolve_aggregation_buckets`、`aggregate_range`、`aggregate_bucket`），`crates/jftrade-store-sqlite/src/backtest_market_data.rs`；<br>专项回归测试（4 项，位于 `tests/backtest_market_data_session_dst_aggregation.rs`）：<br>• `test_tc_d4_01_regular_session_intraday_sub_hourly_aggregation`<br>• `test_tc_d4_02_and_03_dst_boundary_and_60m_session_anchored_aggregation`<br>• `test_tc_d4_04_extended_session_pre_market_does_not_pollute_regular_open`<br>• `test_safety_red_line_missing_minute_fails_closed_with_coverage_error` |
| 门禁 | `cargo test -p jftrade-store-sqlite` (16 suites pass, 退出码 0)；`pnpm run check:rust:static` 退出码 0；`pnpm run check:clippy` 退出码 0；`pnpm run check:generated` 退出码 0；`pnpm run check:quick` 退出码 0 |
| 剩余风险 / 依赖 | 1. 针对非股票类或无特定交易所日历的自定义标的，自动平滑回退至纯 UTC 周期分桶；<br>2. 聚合校验严格遵循 fail-closed，依赖上游数据同步（`BacktestSyncTask`）保证交易分钟完整落库。 |

#### 5. P2-02 yfinance 冷启动下历史 Futu 订单对账解耦

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P2-02 / worker_closure / 2026-09-05 |
| 核查 SHA / 工作树差异 | 生产代码基线 `ccac83d1` / `crates/jftrade-engine/src/product_runtime_composition.rs:48-78`、`product_production_ports_execution_reconciliation_provider_tests.rs:136-197` |
| 状态 / 确认严重度 | **已关闭 / PASS (事实反证)** (原 P2 级推演在当前架构下不成立，已由角色解耦机制彻底消除) |
| 生产调用链 / 所有者 | `compose_market_data_runtime` -> `futu_trade_enabled` -> `OpenDProviderRuntime` -> `ExecutionReconciliationWorker` -> `execution.db` 单一写属主 |
| 复现或反证 | 原材料推测以 yfinance 冷启动时因未选 Futu 行情而导致 OpenD 未初始化、历史订单无法对账；反证证据证实：架构将 OpenD 交易会话作为独立属主管理，冷启动仅要求 `futu_trade_enabled` 即可初始化交易会话与对账后台；已由 `helper_market_data_providers_reconcile_futu_account_order_fill_and_fee` 等测试全链路验证通过。 |
| 修复 / 回归 | 既有回归测试（`product_production_ports_execution_reconciliation_provider_tests.rs` 及 `product_production_ports_trade_tests.rs`）：<br>• `helper_market_data_providers_reconcile_futu_account_order_fill_and_fee`<br>• `helper_market_data_providers_fail_closed_without_futu_trade_session`<br>• `helper_market_data_providers_project_futu_broker_and_portfolio_routes`<br>• `helper_market_data_provider_keeps_futu_trade_reads_on_the_trade_session`<br>• `provider_switch_does_not_disconnect_an_existing_futu_trade_session`<br>• `helper_market_data_provider_without_trade_session_fails_closed` |
| 门禁 | `cargo test -p jftrade-engine --lib helper_market_data_provider` (全量通过，退出码 0)；`pnpm run check:quick` (退出码 0) |
| 剩余风险 / 依赖 | OpenD 宿主进程必须外部处于运行状态；当 OpenD 挂掉时，交易接口显式报错 Fail-Closed，符合预期。 |

#### 6. P0-03 Helper 与 Node 子进程异常退出韧性自愈与防重放屏障

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P0-03 / Antigravity / 2026-09-05 |
| 核查 SHA / 工作树差异 | 修改 `crates/jftrade-engine/src/product_runtime_supervisor.rs`、`product_runtime_helper_health.rs`、`product_runtime_workers.rs`、`strategy_runtime_session_recovery.rs`，新增测试 `tests/sidecar_subprocesses_crash_recovery_resilience.rs`、`tests/sidecar_chaos_and_backoff_stress.rs`、`tests/session_recovery_zero_order_replay_adversarial.rs` |
| 状态 / 确认严重度 | **已关闭 / PASS** (P0 级，彻底攻克 Python Helper 与 Node Worker 崩溃退出后无自动拉起及策略会话不可用的韧性倒退) |
| 生产调用链 / 所有者 | `product_runtime_start.rs` -> `ProductRuntimeSupervisor` / `HelperHealthMonitor` / `PineReadinessMonitor` -> 单属主生命周期守护 |
| 复现或反证 | **复现**：旧逻辑在 Python Helper 退出后仅记录 `healthy = false`，从未调用 `start` 重新拉起；Node Worker 在 append 失败且 close 失败时 revision 卡在非零导致死锁；<br>**修复与反证**：实现单属主生命周期守护，支持异常退出检测与 500ms~10s 指数退避重启；Worker 崩溃后 revision 强制置 0 触发 clean open；四重防重放屏障验证证明重连后零历史订单重放。 |
| 修复 / 回归 | 修复代码：`product_runtime_supervisor.rs`、`product_runtime_helper_health.rs`、`readiness.rs`、`strategy_runtime_session_recovery.rs`；<br>专项测试用例（21 项）：<br>• `test_python_helper_crash_auto_recovery_exponential_backoff_and_reaping`<br>• `test_node_pine_worker_crash_auto_recovery_and_single_ownership`<br>• `test_exponential_backoff_progression_and_upper_bound_capping`<br>• `sidecar_chaos_and_backoff_stress.rs` (7 项测试)<br>• `session_recovery_zero_order_replay_adversarial.rs` (11 项测试) |
| 门禁 | `cargo test -p jftrade-engine --test sidecar_subprocesses_crash_recovery_resilience --test sidecar_chaos_and_backoff_stress --test session_recovery_zero_order_replay_adversarial` (21 passed, 退出码 0)；`cargo clippy -p jftrade-engine` 退出码 0；`pnpm run check:quick` 退出码 0 |
| 剩余风险 / 依赖 | 外部操作系统环境需具备执行 Python/Node 相应二进制权限与端口可用性；持续致命故障按 10s 上限退避重试，不引发 CPU 旋锁。 |

#### 7. P1-07 PineTS Worker 乱序容错与会话泄漏自愈

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P1-07 / Antigravity / 2026-09-05 |
| 核查 SHA / 工作树差异 | 修改 `workers/pineworker/src/pinetsExecutor.ts`、`workers/pineworker/src/pinetsResult.ts`、`workers/pineworker/src/pinetsExecutor.test.ts` |
| 状态 / 确认严重度 | **已关闭 / PASS** (消除乱序 Tick 导致 Worker 内存会话泄漏、随后 Rust 重建被拒及策略永久 HALTED 的高危死锁) |
| 生产调用链 / 所有者 | `strategy_runtime.rs` -> `IntegrationPineExecutor` -> gRPC `append` / `open` -> `pinetsExecutor.ts` (`liveSessions: Map<string, NativeLiveSession>`) |
| 复现或反证 | **复现**：旧逻辑在 `pinetsExecutor.ts:129-136` 将开盘时间非递增校验置于 `try` 块外直接抛错，导致 `catch` 块的 `this.liveSessions.delete(sessionId)` 永远无法被调用，会话残留泄漏在 Node 堆中；Rust 端捕获 append 失败后将 revision 置 0 重试 open，但 Node 端的 `openLiveSession` 判定 `liveSessions.has(sessionId)` 存在无条件抛出 `"already exists"` 异常，致使 Rust 策略主循环直接 break 彻底进入不可逆的 `HALTED` 挂死状态；<br>**修复与反证**：将单调校验移入 `try` 块，确保抛错时 100% 触发 `this.liveSessions.delete` 物理清除；同时在 `openLiveSession` 补充防御自愈（若会话已存在则先行清理再重建）；并在 `pinetsResult.ts` 中针对 `drawings` 实现增量切片去重，消除累积膨胀。 |
| 修复 / 回归 | 修复代码：`workers/pineworker/src/pinetsExecutor.ts`、`workers/pineworker/src/pinetsResult.ts`；<br>专项测试用例：`workers/pineworker/src/pinetsExecutor.test.ts` 中的 `invalidates session and self-heals when non-monotonic candle open times are received`；92 项 pineworker 测试全量通过。 |
| 门禁 | `pnpm --filter @jftrade/pineworker test` (92 passed, 退出码 0)；`pnpm run check:pine` 退出码 0；`pnpm run check:quick` 退出码 0 |
| 剩余风险 / 依赖 | 行情源时钟回拨或跨日重复推送可在入口处被安全截断并自愈，不影响策略后续 Bar 的持续推进。 |

#### 8. P1-08 实盘与回测指标预热收敛残差基准

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P1-08 / Antigravity / 2026-09-05 |
| 核查 SHA / 工作树差异 | 新增 `crates/jftrade-engine/tests/pinets_wire_events_and_warmup_convergence.rs` |
| 状态 / 确认严重度 | **已关闭 / PASS** (建立实盘 200 根历史预热对 IIR 指标残差衰减的量化基准与准入容差规范) |
| 生产调用链 / 所有者 | `strategy_runtime.rs` -> `candle_limit` -> `BacktestMarketDataStore` -> PineTS Indicator Warmup Engine |
| 复现或反证 | **实测与数学推导**：对于技术分析中广泛使用的无限脉冲响应（IIR）指标，如 `EMA(N)`（衰减系数 $\alpha = 2/(N+1)$）与 `RMA(N)`（Wilder 平滑 $\alpha = 1/N$），第 $k$ 根 Bar 初始值误差权重为 $(1-\alpha)^k$。<br>实测验证：<br>1. 实盘默认 $k=200$ 时，`EMA(200)` 残差高达 **13.4%**，`RMA(200)` 残差高达 **36.7%**，仿真阶跃响应显示 200 根预热相对于真实值产生超过 1.0% 的明显偏差；<br>2. 预热增至 $k=460$ 时残差降至 $<1.1\%$；增至 $k=700$（$3.5 \times N$）时残差降至 **$<0.1\%$**（1000 ppm）；增至 1000 根时降至 **$<0.005\%$**（50 ppm）；<br>3. 确定了策略首个可交易 Bar 的安全准入容差与生产配置建议（$k \ge 3.5 \times N$ 或设置可交易起始延迟）。 |
| 修复 / 回归 | 专项实证测试集：`crates/jftrade-engine/tests/pinets_wire_events_and_warmup_convergence.rs`：<br>• `test_p1_08_ema_warmup_residual_error_decay_mathematics`<br>• `test_p1_08_rma_wilder_warmup_residual_error_decay_mathematics`<br>• `test_p1_08_simulated_ema_step_response_divergence_between_200_and_1000_bars` |
| 门禁 | `cargo test -p jftrade-engine --test pinets_wire_events_and_warmup_convergence` (4 passed, 退出码 0)；`cargo clippy` 退出码 0 |
| 剩余风险 / 依赖 | 对于长周期 IIR 策略，建议在实盘配置中设置 `candle_limit >= 700` 或在脚本中前置校验 `bar_index >= 3.5 * length` 避免启动初期的信号偏离。 |

#### 9. P1-09 gRPC 报文大小边界与大绘图对象 FIFO 上限保护

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P1-09 / Antigravity / 2026-09-05 |
| 核查 SHA / 工作树差异 | 修改 `workers/pineworker/src/adapter.ts`、`workers/pineworker/src/adapter.test.ts`，测试于 `crates/jftrade-engine/tests/pinets_wire_events_and_warmup_convergence.rs` |
| 状态 / 确认严重度 | **已关闭 / PASS** (消除海量绘图对象撑爆 4MB gRPC 报文限额导致的会话非正常注销) |
| 生产调用链 / 所有者 | `adapter.ts:normalizeVisualOutputs` -> `grpcServer.ts` -> Tonic gRPC Transport (`DEFAULT_MAX_MESSAGE_BYTES = 4MB`) |
| 复现或反证 | **复现**：当 Pine 脚本在密集循环中频繁调用 `line.new()`、`box.new()` 或 `label.new()` 产出数万个绘图对象时，未经限流的 JSON 报文可能超过 10MB，突破 Tonic 4MB 上限，引发解码失败与会话强行注销；<br>**修复与反证**：在 `adapter.ts:normalizeVisualOutputs` 中引入符合 TradingView 行业规范（标准上限 500 lines + 500 boxes）的 **1,000 个绘图对象 FIFO 滑动窗口截断**，实测 1000 个对象的最大载荷稳定在 ~200KB（仅占 4MB 阈值的 < 5%），既完整保留最新绘图与 orderIntents，又彻底杜绝了报文溢出风险。 |
| 修复 / 回归 | 修复代码：`workers/pineworker/src/adapter.ts` (`MAX_VISUAL_OUTPUTS = 1000` 切片截断)；<br>专项测试用例：<br>• `workers/pineworker/src/adapter.test.ts`: `enforces 1000 item FIFO cap on visual outputs to protect against 4MB gRPC boundary`<br>• `crates/jftrade-engine/tests/pinets_wire_events_and_warmup_convergence.rs`: `test_p1_09_grpc_4mb_boundary_and_drawing_cap_bounds` |
| 门禁 | `pnpm --filter @jftrade/pineworker test` (92 passed, 退出码 0)；`cargo test -p jftrade-engine --test pinets_wire_events_and_warmup_convergence` (4 passed, 退出码 0) |
| 剩余风险 / 依赖 | 超过 1000 个历史绘图对象将被保留最新的 1000 个，符合图表交互体验且不影响订单意图（`orderIntents` 始终完整保留）。 |

#### 10. P2-03 候选包多平台完整性与 Zero-Go 发布资格治理

| 字段 | 内容 |
| --- | --- |
| ID / 负责人 / 日期 | P2-03 / Antigravity / 2026-09-06 |
| 核查 SHA / 工作树差异 | 修改 `docs/history/go-to-rust/verification-matrix/10-zero-go-tauri-frontend.md`、`docs/history/go-to-rust/go_to_rust_comprehensive_verification_matrix.md` |
| 状态 / 确认严重度 | **已关闭 / PASS (工程门禁与资格边界闭环)** (确立四平台 Sidecar 独立自包含与 Zero-Go 发布治理体系，阻断非签名或 Go 残留产物) |
| 生产调用链 / 所有者 | `check-zero-go.mjs` + `prepare-tauri-release-runtime.mjs` + `check-desktop-release-policy.mjs` -> Tauri Desktop Release Pipeline |
| 复现或反证 | **实测与反证**：<br>1. **Zero-Go 全景门禁**：`pnpm run check:zero-go` 严格扫描 2,661 个纳管源码文件与发布产物，确认 0 个 `.go` 文件、0 个退役 Go 入口、ELF/Mach-O 二进制中 0 个 `\xff Go buildinf:` 魔数，Go/Wails 历史代码彻底清零；<br>2. **4 平台 Sidecar 自包含打包与冒烟**：macOS (arm64/amd64)、Linux (amd64)、Windows (amd64) 的 Python Helper 与 Node PineTS 打包运行时自包含，`test:tauri-release-runtime` (11 项)、`test:marketdata-sidecar-asset-build` (6 项)、`test:marketdata-sidecar-smoke` (5 项) 全量通过；<br>3. **发布资格四层边界隔离**：依据 `docs/architecture/release-qualification.md`，CI/CD 发布治理划分为 source admission、candidate evidence、publish、post-release 四个独立边界。`check-desktop-release-policy` 在缺少苹果/微软签名与公证凭据时严格 fail-closed 阻断正式 candidate/publish（杜绝未签名包混入发布），同时允许本地开发环境执行 unsigned rehearsal 验证流程；<br>4. **前端 13 个非标准盲区治理**：对未直接映射 UI 的 13 条后端路由完成全量性质勘误与归类（含冗余探针、调试流、全量树合并等），其中交易解锁（P0-04）已闭环，除权除息与购买力预估等纳入后续演进排期。 |
| 修复 / 回归 | 既有门禁与专项回归测试（全部通过）：<br>• `pnpm run check:zero-go` (2,661 files verified, exit 0)<br>• `pnpm run test:tauri-release-runtime` (11 passed, exit 0)<br>• `pnpm run test:marketdata-sidecar-asset-build` (6 passed, exit 0)<br>• `pnpm run test:marketdata-sidecar-smoke` (5 passed, exit 0)<br>• `node scripts/check-desktop-release-policy.mjs --operation rehearsal` (exit 0)<br>• `pnpm run check:route-contracts` (278/278 routes verified, exit 0) |
| 门禁 | `pnpm run check:zero-go` 退出码 0；`pnpm run test:tauri-release-runtime` 退出码 0；`pnpm run check:quick` 退出码 0 |
| 剩余风险 / 依赖 | 正式生产发布必须在配置完整 Apple Developer 与 Windows Authenticode 证书的保密 CI 环境中执行，不可使用开发机本地未签名构建替代正式 candidate。 |

## 五、补充验证与发布边界

原 16 项不是完整验收范围，还需按受影响领域检查：
- **领域 02 Provider**：切换失败回滚、旧 generation 在途请求/缓存隔离、策略订阅屏障、交易通道保留；覆盖真实生产调用路径而非仅状态结构单测。
- **领域 03 路由/租约**：方法+路径集合、认证/授权与错误信封、SSE 断线续传、WebSocket 重连/背压；九库逐项覆盖并发进程、持锁者死亡、路径别名及只读访问。
- **兼容性**：storage、backtest、provider-runtime、trading-strategy、assistant-runtime、api-transport、desktop-runtime 七类冻结语料，不能以路由数量替代行为 replay。
- **数据恢复与安全**：备份一致性、恢复失败回退、凭据脱敏、日志/工件敏感信息、升级中断与磁盘不足；破坏性测试仅在隔离临时目录执行。
- **正式发布**：source admission 绑定同 SHA 的 Build & Test，receipt 固定 `releaseQualified=false`；只有完整证据的 `candidate_ready` 才可授权计划 tag。
- **候选与发布后**：同 SHA 四平台签名/公证/updater、安装升级回滚、SBOM/provenance、备份恢复及独立安全签字；publish 仅消费 sealed candidate、不重新构建；发布后独立 receipt，不改候选记录。unsigned rehearsal 不可替代正式候选。

本次文档整理不运行 live workflow，不创建 candidate、tag、Release 或 rehearsal，不证明任何正式候选已通过。

### 本轮文档验证记录

- 主文及改写摘要/行动入口的相对文件链接存在性检查通过；台账 16 个风险 ID 唯一性检查通过。
- `test:affected -- --worktree` 与 `check:quick` 均因既有 `.agents/ORIGINAL_REQUEST.md` 等未知变更触发 fail-closed 扩大检查范围。两次执行中的 policy/contracts 阶段通过，含源码 Zero-Go（2,624 纳管文件、**0 个发布产物**）、278 路由契约、只读生成契约及 122 个 policy 脚本测试。
- 两个入口随后进入编译/多运行时检查，本轮主动停止，不计为整体通过；Rust/兼容性/桌面发布及表中场景均无完整验收结论。
- `git diff --check` 报告既有 `docs/history/go-to-rust/README.md:10` 文件尾空行；该文件非本轮修改，未回退或顺手修正。上述目标文档目前未纳管，不能仅靠普通 `git diff` 证明其质量。

## 六、分卷导航与维护规则

- [分卷目录](./verification-matrix/README.md)
- [00 执行摘要](./verification-matrix/00-executive-summary.md)
- 领域 [01 Pine](./verification-matrix/01-pine-runtime.md)、[02 Provider](./verification-matrix/02-provider-switching.md)、[03 路由/租约](./verification-matrix/03-routes-and-writerlease.md)
- 领域 [04 时间](./verification-matrix/04-backtest-time-dst.md)、[05 对账](./verification-matrix/05-broker-reconciliation.md)、[06 进程](./verification-matrix/06-sidecars-resilience.md)
- 领域 [07 ADK](./verification-matrix/07-adk-leases-approvals.md)、[08 SQLite](./verification-matrix/08-sqlite-schemas-migrations.md)、[09 Wire](./verification-matrix/09-pinets-wire-events.md)、[10 发布/前端](./verification-matrix/10-zero-go-tauri-frontend.md)
- [11 执行与验收入口](./verification-matrix/11-release-qualification-action-plan.md)

主文维护状态、勘误与任务编号；分卷维护对应领域的复现/反证和细节。更新结论时同步对应分卷，但不复制整张风险表。一次性迁移发现留在 history；只有真实架构边界变化才同步架构文档、docs/README.md 和模块表。

## 2026-09-27 当前 parity 审计快照

本历史入口不回写前述批次的验收结论；当前逐条 Go→Rust 映射以 `manual-test-mappings.json` 为准。最新审计结果为 Go `4451`、Rust `3389`，其中 `function_exact=1504`、`partial=2313`、`boundary=634`、`missing=0`，重复 exact 与不存在 Rust crate 均为 0。Parity anchor reconcile 为 `1803/1756/0/0/47`（unique/recorded/unrecorded/stale/unknown）。严格审计仍有历史 evidence/receipt gaps，未通过。

2026-09-26 批次补齐了 provider probe quick/full、无 reasoning mappings、timeout cap、TestProvider 能力回写、strategy activity 日志/审计查询失败的已知空页回归，以及 workflow invalid-input 三路错误文案证据；另补充 auth generation fence 的 ABA 回归。该批次记录时 `audit_test_parity.py --strict` 受 4085 个 function_exact evidence/receipt gaps 阻断；后续批次已将当前缺口推进至 3948。本快照只记录映射与锚点状态，不改变本文件的历史发布资格边界。

2026-09-27 strategy_pine P1 warmup fallback 补齐：`TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs` 以 planner 的 `security_source:day/hour/week` requirement key 逐例锁定 period=0、canonical hour 空 interval 与 unknown-symbol week/5m 三个边界；修复 `resolve_timeframe_minutes` 的 hour canonical alias 缺失。修复前专测 1/1 红（hour 得到 1170 而非 180），修复后定向 nextest 1/1 通过。当前审计快照为 Go 4451、Rust 3370、`function_exact=1493`、`partial=2332`、`boundary=628`、`missing=0`；Parity anchor reconcile 1783/1737/0/0/46。receiptDigest 仍为空，strict evidence/receipt gaps 不因本条伪造通过。

2026-09-27 API transport P1 增量：显式 `brokerId` 读路由先红后修。active=yfinance 且 `brokerId=futu` 时，Rust quote read owner 在 securities/snapshots/candles/depth 四路读取前返回 409，并以 helper 请求数为 0 锁定无 legacy fallback；active `yahoo-finance` alias 正向路径通过。Go 注入式 broker reader 与 Rust active-provider owner 不同，映射继续为 partial；新增 engine 定向 nextest 2/2 通过，未伪造 receiptDigest。

2026-09-27 auth/session P1 增量与全量验证收尾：新增 `auth_state_maps_stay_bounded_and_evict_oldest_entries`，先以超限回归复现旧实现只驱逐一项的边界，再修复 session state map 的循环驱逐；定向 engine nextest 1/1 通过。最终只读 parity 审计为 Go `4451`、Rust `3371`，`function_exact=1493`、`partial=2330`、`boundary=628`、`missing=0`；anchor reconcile 为 `1783/1737/0/0/46`（unique/recorded/unrecorded/stale/unknown）。`manual-test-mappings.json` 4451 rows、metadata 校验通过；`audit_test_parity.py --strict` 仍真实失败，保留 `4084` 个 evidence/receipt gaps，未伪造 `receiptDigest`。`pnpm run check:rust` 的 architecture、production-policy、format、clippy、deny、workspace nextest（3499 passed、2 skipped）及七类 compatibility replay 均通过。
2026-09-27 API transport P1 增量：复核 `TestMarketQueryAndExecutionPayloadFallbacksRemainDeterministic`。Rust 新增 `market_symbol_path_tail_matches_go_split_and_empty_guard` 与 `candle_route_clamps_zero_limit_to_one_like_go_query_helper`；旧 `limit=0` 实现先红（公开 request limit 为 200 而 Go `LimitOrDefault` 为 1），修复为显式非正值→1、空白值仍默认 200。连同非法 limit、显式/反向时间窗回归共 6/6 nextest 通过。Go execution payload nil/不可编码回退与字符串 helper 因没有同形 Rust seam 保持 partial；最新审计为 Go `4451`、Rust `3373`、`function_exact=1493`、`partial=2330`、`boundary=628`、`missing=0`，anchor `1783/1737/0/0/46`，strict 仍不得宣称通过。

2026-09-27 Broker disconnected/degraded P1 边界确认：Go `broker-read.json` 的资金、持仓、订单、成交、现金流、费用、保证金、最大交易量、quote、klines、securities 断连样本均为 200 degraded；Rust 生产 broker-read 端口在无 trade/session/market-data source 时由 `BROKER_READ_UNAVAILABLE` 503 fail-closed 测试固定，后端错误文本仍保留。`broker_klines_valid_request_fails_closed_without_historical_source`、`broker_securities_fails_closed_without_market_data_router`、`broker_read_fails_closed_without_trade_client` 与 `broker_read_routes_fail_closed_when_snapshot_port_is_unavailable` 构成最窄 owner 证据；显式 test-cutover fixture 的 200 回放不改变生产语义。对应映射已从待产品决策的 partial 改为 boundary（混合 service fallback 条目保持 partial），不恢复旧 wire，也未伪造 receipt。
2026-09-27 parity 增量：Backtest/Calendar 分页 cursor 生产路径补测覆盖 missing `nextBefore`、向前 cursor 失败与到达 `since` 边界成功落库；Strategy/Pine 趋势与状态 TA 映射纠偏，新增同脚本分析回归并将 `TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions` 从 partial 升级为 function_exact。两批均完成真实定向 nextest、mapping/reuse/anchor 同步；未填充 receiptDigest，strict evidence/receipt 审计仍保持未通过。

2026-09-27 Futu/OpenD runtime P1 收口：`TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts` 已更新旧的 account/order reconciliation 证据行。真实 OpenD mock 经过 global-state probe、`SharedTradeReadRuntime` 与 production broker runtime route，逐项断言 connected startup、`serverVersion=10.9.7000`、markets/health、2 个 discovered accounts 及 REAL 优先排序；`product_runtime_composes_opend_provider_and_fences_shutdown_ownership` 定向 nextest 1/1 通过。当前映射为 `function_exact`，证据 receipt 为 `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`（[workspace receipt](verification-receipts/workspace-nextest-2026-09-27T061100Z.json)）。当前只读审计为 Go `4451`、Rust `3376`、`function_exact=1495`、`partial=2322`、`boundary=634`、`missing=0`；anchor reconcile `1786/1740/0/0/46`，strict 仍有 `4021` 个历史 evidence/receipt gaps，不能宣称整体严格审计通过。

2026-09-27 provider health backoff P1：新增 `helper_restart_policy_defaults_match_go_provider_health_retry_delays`，先以默认策略断言复现 500ms 与 Go 100ms 的真实差异，再将 `HelperRestartPolicy` 及 managed market-data helper 的生产 restart policy 对齐为 100ms 初始、1s 上限，并逐档断言 100→200→400→800→1000→1000→1000ms。定向 engine nextest 2/2、sidecar 回归 3/3 通过；`runtime_health_test.go:222` 映射升为 `function_exact`，`:240` 的 cancellation/error 透传仍保留 partial。当前审计为 Go `4451`、Rust `3376`、`function_exact=1496`、`partial=2321`、`boundary=634`、`missing=0`；anchor reconcile `1786/1740/0/0/46`。未填充 receiptDigest，strict evidence/receipt gaps 仍未通过。

2026-09-27 strict evidence batch：17 条已有单引用 `function_exact` 完成 Go/Rust 测试体人工复核，并关联真实 workspace receipt `sha256:10e0f6f0303126574a56bb913fce6b78e03ef69a359136b6a92d3d6be3eb5bf8`；未覆盖子断言的 session-negative、空数组与 stream transport 条目保留原结论。strict evidence/receipt gaps 由 4085 降至 4021，仍不得宣称整体严格审计通过。
2026-09-27 全量验证收尾：workspace nextest 3504/3504 passed、0 failed、2 suite skipped；SQLite/backtest/provider-runtime/trading-strategy/assistant/API/desktop compatibility replay 全部通过。当前 receipt 为 `sha256:2f7422488ce7addc2b81ad38563b3b7662345283896be959843301f2b609d66f`（[receipt](verification-receipts/workspace-nextest-2026-09-27T071942Z.json)）；最新 parity 为 Go 4451、Rust 3376、`function_exact=1496`、`partial=2321`、`boundary=634`，strict 仍剩 4019 个历史 evidence/receipt gaps，不能宣称整体严格审计通过。

2026-09-27 strict follow-up：API launcher `main_test.go:86/:121` 已补源码 `// Parity:` 锚点并以 2/2 nextest receipt `sha256:bb5be96baa9877eba7e6d5f9f0f3455fd7d3608f5912999bd6e53cfc8f3bd757` 验证；MarketData/Calendar 三条单引用使用 receipt `sha256:9d3a85b571117001d7de7cb3a4badbf5f9825c308f243b51763b87426dc22115`（3/3）。当前只读审计为 Go 4451、Rust 3377、`function_exact=1496`、`partial=2321`、`boundary=634`、`missing=0`；anchor `1788/1742/0/0/46`；strict 仍真实失败 3957 条，未宣称整体通过。
2026-09-27 Assistant/session-context strict follow-up：三条 compaction revision/notice 映射均已补 reviewed assertion、reuse、anchor 与真实 receipt `sha256:917fa4ba6a8484f68cb156733271d11cc5da94a702b60e52d1f91229a1ea5078`（定向 3/3）。strict 仍真实失败 3948 条；该批不改变 Go helper seam 与 Rust durable owner 的边界结论。
2026-09-27 Watchlist quote P1：补齐 Futu pre/after/overnight 扩展 session 价差投影、重叠 batch quote single-flight 与 provider TTL（Futu 2.5s、helper 15s）；先红后修后定向 nextest 3/3、相关 Watchlist nextest 56/56 通过，receipt `sha256:ae6267da865221f941864dca401b67519a51c0a38b9c0069be36bd4010bad7e3`。三条映射升级为 `function_exact`。SG.D05 unknown-market timezone、metadata 回写及 watchlist 端口级 delayed fallback/permission isolation 保持 partial；当前 Go 4451、Rust 3380、`function_exact=1499`、`partial=2318`、`boundary=634`、`missing=0`，anchor `1791/1745/0/0/46`，strict 真实失败 3927 个 evidence/receipt gaps。
2026-09-27 Calendar health strict evidence：对 `TestManagerProbeMarksEmptyParsesUnhealthy`、`TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts`、`TestManagerProbeRecoveryClearsCurrentFetchError` 的 Go/Rust 断言逐条复核，保持三条 `function_exact`，并以 `manager_lifecycle` 定向 nextest 3/3 receipt `sha256:27d4c070d9bbff94444923e57d51a73b4f62069b70fc6dd5fe357382c0e87bcc` 收口 reviewed assertion。该批不改生产代码；strict gap 由 3927 降至 3921，整体严格审计仍未通过。
2026-09-27 Backtest retry P1：`TestHistoricalProviderRetryExhaustionAndTimerCancellation` 新增四次 transient 请求耗尽与退避中取消两条 Rust helper 回归；固定 timer 取消路径先红，随后将 Futu/helper 重试等待统一为取消感知轮询。定向 nextest 2/2 通过，receipt `sha256:26dc42121e439058156a76cfe41acd1d0c041eb7104b2f320d99861e119019dc`；映射由 partial 升为 function_exact，两个 evidence 均有同 Go anchor、reviewed assertion 与单引用 reuse。当前 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`，anchor `1792/1746/0/0/46`；strict 仍真实失败 3921 个历史 evidence/receipt gaps。
2026-09-27 Trading/Broker strict evidence C 批：复核 `TestDerivativeSingleLegRequiresBrokerPreviewAndStableClientID`、`TestRealTradeControlPlaneFailsClosedWhenPersistedStateCannotLoad`、`TestNormalizeExecutionOrderUsesEnvFallbackAndSupportsNonLimitUSSessions` 三条既有 exact owner；三条均保留 function_exact，补 reviewed assertion、单引用 reuse/Parity anchor 与真实联合 nextest receipt `sha256:77f918d03712f238ef42d3340f4cc5c73f951a0406e534d74b4c3ad251cfe258`（3/3 passed）。无生产变更；当前 strict 仍真实失败 3915 个历史 evidence/receipt gaps。
2026-09-27 Query/MarketData strict evidence：复核 `TestNormalizeOptionalQueryTimeAcceptsEmptyAndRejectsMalformedValues`、`TestDecodeMarketCandlesQueryParsesRepeatedSessions` 与 `TestCandlesRouteNormalizesRepeatedSessions`。query 两条补 `Parity:` anchor；route 条目先红后修，直接断言分页 provider 路由 `RTH/RTH/ETH/ETH/ALL/ALL` 与去重后的三条唯一路由。三条定向 nextest 3/3 通过，receipt `sha256:6d383be156db7803a762e52849c2d20b09d245fe42aac931543f27d95ee59496`；reuse 多引用已 reviewed，相关 Go session 聚合差异继续保持 partial。当前 exact 1500、partial 2317、boundary 634；anchor `1793/1747/0/0/46`；strict gap 3901，整体仍未通过。
2026-09-27 Futu/OpenD K-line session strict evidence：复核 `TestQueryKLinesSplitsUSHistoricalRequestsBySessionAndMergesResults`、`TestQueryKLinesForSessionsFiltersUSHistoricalRoutes`、`TestResolveHistoricalRequestSessionUsesRouteForRTHAndOvernight` 与 `TestShouldFallbackHistoricalKLineSplitRecognizesChineseSupportedSessionsMessage`。扇出测试先因 Option 断言编译失败，修正后补齐 `extendedTime=true` 与 pre/regular/overnight 标签；联合 engine/integration-futu nextest 5/5 通过，receipt `sha256:cad7100b896a7aab3714ef4187d68e428db7e84f5585bef76c92fa807e593cfb`。四条映射均保留 `function_exact`，reviewed assertion、Parity anchor 与 reuse 已同步；session helper 的 Go 过滤子断言仍待同形 owner 证据，未机械升级。最新审计为 Go 4451、Rust 3382、`function_exact=1500`、`partial=2317`、`boundary=634`、`missing=0`；anchor `1793/1747/0/0/46`；strict gap 3892，整体仍未通过。
2026-09-27 API transport auth strict evidence：复核 Origin/CSRF 会话写、PATCH 会话写、浏览器禁用页（含 web runtime 状态）与密码会话读写五条 P1 exact；真实联合 nextest 5/5 通过，receipt `sha256:c5457db228673e47ae252a104992aa8c261fce3df081f60dfd28cf7c6682cc17`。五条映射均完成 reviewed assertion 与 passed verification，strict gap 3885，整体仍未通过。
2026-09-27 Assistant/API strict evidence：复核五条 P1 exact 的全部 Rust owner，覆盖 sessions CRUD/composer、断连 499、stream replay、catalog observability 与 timeline legacy error；联合 nextest 8/8 通过，receipt `sha256:fb75c71c988292130d6e3f017e66f1a2a3de3f3d19b3ec106a6af3ca84da3c7e`。映射完成 reviewed assertion 与 passed verification，strict gap 3875，整体仍未通过。
2026-09-27 Backtest/API strict evidence：复核五条 P1 exact 的 sync scope、progress/cancel、status/result/delete、provider page conversion 与 blank script owner；真实联合 engine nextest 10/10 通过，receipt `sha256:fda37329aa034a644056012e122883e27d99a6586ad7bf82003d3af39f110f6a`。映射完成 reviewed assertion 与 passed verification，strict gap 3870，整体仍未通过。
2026-09-27 MarketData boundary strict evidence：复核五条 P1 exact 的 candle period、generation fence、invalid sessions、subscription cancellation 与 logical cleanup owner；真实联合 engine nextest 6/6 通过，receipt `sha256:baa66579a6a1866149f08707d9e80790638ec1fb29f48ed34962a50ae5f17478`。映射完成 reviewed assertion 与 passed verification，strict gap 3865，整体仍未通过。
2026-09-27 MarketData/API forwarding strict evidence：复核五条 P1 exact 的 news/corporate-actions forwarding、tick cache/pagination、managed-account not-found 与 strategy activity owner；真实联合 engine nextest 11/11 通过，receipt `sha256:a583ff41724cfc3bcd259fcf9627a7c8594e6a9aa5c4a79e0082cf332a71c622`。映射完成 reviewed assertion 与 passed verification，strict gap 3860，整体仍未通过。
2026-09-27 Execution/Watchlist strict evidence：复核五条 P1 exact 的 activity pagination/time、execution cancel envelope、broker empty arrays 与 watchlist membership/body boundaries；真实 engine/store-sqlite 联合 nextest 10/10 通过，receipt `sha256:53f26ac779a8505c741acb4ea4e9f6316da471fb6e04f0eeeea7b823f63e301b`。映射完成 reviewed assertion 与 passed verification，strict gap 3855，整体仍未通过。
2026-09-27 Runtime/Maintenance strict evidence：复核 installers rollback、SQLite backup/cleanup、Futu quote health/disconnect、OpenD health 与 provider activation 五组 P1 exact；真实跨 crate nextest 10/10（live OpenD ignored）通过，receipt `sha256:b58fc1c29238f5349c80b07263e7375b665440d25cbe46ae45201a4903967df6`。映射完成 reviewed assertion 与 passed verification，strict gap 3850，整体仍未通过。
2026-09-27 Market HTTP strict evidence：复核 US intraday session labels/unknown-session error 与 snapshot fresh-cache、cache-miss、force-refresh 五条 P1 exact；真实 engine nextest 5/5 通过，receipt `sha256:4c4bec4be84d55d241713c361ec13c8feb7011c4863b51821d1b51df8c679a80`。映射完成 reviewed assertion 与 passed verification，strict gap 3845，整体仍未通过。
2026-09-27 Helper/runtime strict evidence：复核 sidecar manager/process bounded stop、Node common-path fallback/required aggregation、desktop web password session 与 strategy runtime panic reconciliation 六条 P1 exact；marketdata-helper/engine 联合 nextest 7/7 通过（strategy runtime owner 同名测试覆盖 lib 与 bounded integration binary），receipt `sha256:52ef8cc1e2ccf7d57130269388da3d90544630132116f3cc4cf9c431d7de2c51`。映射完成 reviewed assertion 与 passed verification，strict gap 3833，整体仍未通过。
2026-09-27 API portfolio/settings strict evidence：复核 browser-only security write、portfolio cash fallback/degraded empty/removed route 与 broker settings defaults 五条 P1 exact；api/engine/settings-file 联合 nextest 5/5 通过，receipt `sha256:cf733b63c6b03f627d273983d92eff928ecb09de97b1f79be6c6c95d04e3ee76`。映射完成 reviewed assertion 与 passed verification，strict gap 3823，整体仍未通过。
2026-09-27 Futu marketdata runtime strict evidence：复核 trade→quote cache inheritance、fallback instrument filtering、fallback ticker/snapshot projection 与 stale connection-generation fencing 五条 P1 exact；integration-futu/marketdata 联合 nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:57f5e6283bc3618f2b76411d6374d775dc85c6a8e939d6512d8c0615c06b4899`。映射完成 reviewed assertion 与 passed verification，strict gap 3813，整体仍未通过。
2026-09-27 Assistant boundary strict evidence：复核 MCP token/tool catalog、session context fallback、pending input cancellation、closed-stream approval resume 与 missing-run continuation 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:7f97c2a3cc83a59db7c1bfa55be2233314f69e89bf29de08c35c2ba56592506`。映射完成 reviewed assertion 与 passed verification，strict gap 3803，整体仍未通过。
2026-09-27 Futu K-line pagination strict evidence：复核 latest-page exclusive cursor、OpenD market-time/UTC conversion、exact-second cursor boundary、page dedupe/sort 与 inclusive range 五条 P1 exact；engine/integration-futu 联合 nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:489957b0b5e2b5d8f04047abee16c357751def89d390ec871a2016be66b24c70`。映射完成 reviewed assertion 与 passed verification，strict gap 3793，整体仍未通过。
2026-09-27 API/marketdata tail strict evidence：复核 provider switch retirement、runtime status wire normalization、cookie-only WebSocket upgrade、跨交易日 tick volume delta 与 candle pagination metadata 五条 P1 exact；engine/marketdata 联合 nextest 5/5 通过，receipt `sha256:7612368c8b4ebc983e552ba11b03a5ede9cad29575c83c5728c8578411901a69`。映射完成 reviewed assertion 与 passed verification，strict gap 3783，整体仍未通过。
2026-09-27 Backtest/Calendar/Settings strict evidence：复核 DST market-date range、corrupt calendar snapshot rejection、跨年 snapshot cache index、empty broker defaults 与 rejected watchlist provider switch 五组 P1 exact；engine/calendar/settings 联合 nextest 6/6 通过，receipt `sha256:e2ffadfff2d2d62a4d697f5bf9a65f47f446f2b60316848ed77cac4d6517ae34`。映射完成 reviewed assertion 与 passed verification，strict gap 3774，整体仍未通过。
2026-09-27 Trading/Broker control-plane strict evidence：复核空/损坏控制面状态、hard-stop 审计持久化失败粘性降级、option combo 生命周期、forced reconciliation wake 与 capability route-table 五组 P1 exact；engine nextest 9/9 通过，receipt `sha256:6246b9ab3c1d3050579515161c6005f3103a8eb2ac583573a6369db130eeb2bc`。映射完成 reviewed assertion 与 passed verification，strict gap 3764，整体仍未通过。
2026-09-27 Futu subscription reconciler strict evidence：复核 provider switch 延迟物理释放、OpenD ack retention/retry timing、closed-connection ownership cleanup 与 never-established record drop 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:c4734a37b0151e32c4e87c9b949f9ae10fa286364e4e337484fd9eb1b2bb1577`。映射完成 reviewed assertion 与 passed verification，strict gap 3754，整体仍未通过。
2026-09-27 Futu OpenD boundary strict evidence：复核 request timeout stale-waiter isolation、keep-alive shutdown、history optional-field/empty-result handling、closed-session depth rejection 与 disconnected trading reads 五条 P1 exact；integration-futu nextest 5/5 通过（live OpenD suite 保持 ignored），receipt `sha256:b10f6c20a8a268cddbaed2a56114e7e63c4a24caad8e1cd6a856d6c70d17c420`。映射完成 reviewed assertion 与 passed verification，strict gap 3749，整体仍未通过。
2026-09-27 Futu session/trade strict evidence：复核 K 线 session selection/annotation/default validation、US previous-close session rules、trade-account authority fallback 与 mainland Shanghai location fallback 七条 P1 exact；engine/integration-futu 联合 nextest 7/7 通过（live OpenD suite 保持 ignored），receipt `sha256:9b9427f38df767a2d63f217953fd27bae053fda603622c86d9a7ef33d51b3814`。映射完成 reviewed assertion 与 passed verification，strict gap 3739，整体仍未通过。
2026-09-28 Assistant session-gate strict evidence：复核 auto-compaction gate exclusion/free-path、workflow active-parent compaction、pending-approval protected tail 与 approval-waiting active-run detection 五条 P1 exact；engine nextest 5/5 通过，receipt `sha256:c6e22cccbab1c95b191598b13684cc7d6632461fc80d06f8fbed43fb736981b2`。映射完成 reviewed assertion 与 passed verification，strict gap 3729，整体仍未通过。
2026-09-28 Futu subscription/trade strict evidence：六条 P1 exact（订阅确认、连接代际失效、terminal close、failed connect fail-closed、交易写入前置条件、空成功修改单）完成 Go/Rust 断言复核；integration-futu + engine 联合 nextest 6/6 通过，receipt `sha256:66b32e68a91fcc48f6557dcb436c9ea2d5d16af485d6418fcddc83bb83a25dc1`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3729 降至 3723，整体仍未通过。
2026-09-28 Futu protocol strict evidence：五条 P1 exact（prediction catalog、research request injection、history K-line pagination、history order filters、user-security error propagation）完成 Go/Rust 断言复核；integration-futu nextest 5/5 通过，receipt `sha256:6478da8aebc98d804488cf76f032ff5ceaf9cc33c3a3fd38aaff652d8355429a`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3723 降至 3718，整体仍未通过。
2026-09-28 Assistant session-context strict evidence：五条 P1 exact（read pressure、model payload auto-compaction、pending approval earliest/remaining protected tail）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:0ca3d7f2e3b1e564c6fc25e2a91823516d7de78fb8e56d03ee042f9a272c92da`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3718 降至 3713，整体仍未通过。
2026-09-28 Futu K-line boundary strict evidence：五条 P1 exact（period 双协议映射、cursor/time rejection、分页 helper bounds、period catalog rejection、Session_ALL fallback）完成 Go/Rust 断言复核；integration-futu + engine 联合 nextest 5/5 通过，receipt `sha256:f38ccd784ac1bf1afd47e5a14600fc1f016af24883a0787fb35b97b78f55801a`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3713 降至 3708，整体仍未通过。
2026-09-28 Futu snapshot/listener strict evidence：五条 P1 exact（snapshot batching/cache、budget/rate-limit/cancellation、empty/unexpected result 与 malformed quote push drop）完成 Go/Rust 断言复核；integration-futu + engine 联合 nextest 5/5 通过，receipt `sha256:5843770babe2087467db4d181f4c556dcea44dcf03a89385610e9ded8179dad8`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3708 降至 3703，整体仍未通过。
2026-09-28 Assistant store-ops strict evidence：五条 P1 exact（session ownership、cancel/approval atomicity、missing-run failure、missing-approval idempotence、run listing ordering）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:66c0e83d636b8f8045e81d60c6cd382808d168af88f8d211cc8739dcdc9df635`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3703 降至 3698，整体仍未通过。
2026-09-28 Assistant session-context2 strict evidence：五条 P1 exact（provider override、revision compaction、handoff revision、append-during-compaction、pending approval call preservation）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:83808bfd4b00fe6f1aacfe581f8ee1453badfe7659bff03d5d281583ba62163a`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3698 降至 3696，整体仍未通过。
2026-09-28 Assistant runtime strict evidence：五条 P1 exact（tool ordering、cancellation join、probe timeout cap、approval lease cancellation、session title reuse）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:cbeb1497d9d8ee4db2dbafb58ab65670dd8abbd74d3c020007e4ab127b04b211`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3696 降至 3695，整体仍未通过。
2026-09-28 Futu history-window strict evidence：五条 P1 exact（session planner、multi-page window、page budget、upstream page size、payload-less security info）完成 Go/Rust 断言复核；integration-futu nextest 5/5 通过，receipt `sha256:603f4b41c4847b7f46c69791b46609ecb78503502e1118929ce1f5d5d9f09338`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3695 降至 3692，整体仍未通过。
2026-09-28 Assistant store-lifecycle strict evidence：五条 P1 exact（跨库 cascade、session page、composer state、approval sorting、provider timeout persistence）完成 Go/Rust 断言复核；store-sqlite + engine 联合 nextest 5/5 通过，receipt `sha256:df78dcad3dcd67f0e6dc10d4c57da16dee947f4114ebfdeb509578e2df9bc07e`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3692 降至 3687，整体仍未通过。
2026-09-28 Assistant run-time strict evidence：五条 P1 exact（terminal cancel audit、run timeout freeze、timed-out resume、expiry reconciliation、per-run timeout）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:172dd3f940aca79bcb55c464e065550bf98f4273f0ff1559bba3f44ad9e5560c`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3687 降至 3682，整体仍未通过。
2026-09-28 Futu notification/probe strict evidence：五条 P1 exact（notification payload/status routing、closed-port disconnected probe、program-status formatting、candle-session mapping）完成 Go/Rust 断言复核；integration-futu nextest 实际 6/6 通过（同名 partial helper 一并命中，live OpenD ignored），receipt `sha256:ae8a2c04479d9119e93e169d4f3e3a048488f478e60b591242891cc9c4040d6a`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3682 降至 3672，整体仍未通过。
2026-09-28 Futu subscription-reconciler strict evidence：五条 P1 exact（physical sharing/deferred release、concurrent idempotence、retry/reacquire reset、BasicQot delayed fallback、connection quota reset）完成 Go/Rust 断言复核；integration-futu nextest 5/5 通过（live OpenD ignored），receipt `sha256:c3d81229c5939b1e25203f0f1c269d609600a0ed226da538c5fc7b1e8bd5e3e0`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3672 降至 3662，整体仍未通过。
2026-09-28 Futu client-recovery strict evidence：五条 P1 exact（recoverable replay policy、closed-ready replacement、notification callback reconnect、minimum-version rejection、typed init/session transport failures）完成 Go/Rust 断言复核；integration-futu nextest 5/5 通过（live OpenD ignored），receipt `sha256:8691574cb891a5f89c932ccd431f408bf4c3893654277016ba7e57c0c62ace54`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3662 降至 3652，整体仍未通过。
2026-09-28 Futu quote/empty-boundary strict evidence：五条 P1 exact（empty instrument/order-book/basic-quote paths、duplicate quote projection、invalid/payload-less snapshots）完成 Go/Rust 断言复核；integration-futu nextest 实际 10/10 通过（live OpenD ignored），receipt `sha256:65f7118836c30241fe900c00e4cb632ce9c8ead298e4c7d90e93f87bab8c69c1`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3652 降至 3642，整体仍未通过。
2026-09-28 Assistant engine-gates strict evidence：五条 P1 exact（strategy optimize persistence、MCP disabled lifecycle、agent/provider resolution、tool catalog availability、tool-error retryability）完成 Go/Rust 断言复核；engine nextest 5/5 通过，receipt `sha256:484a697d804103853875d6d7e2d34429dc85981bc916d7568cb3e1fecca74a49`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3642 降至 3632，整体仍未通过。
2026-09-28 Futu mixed-boundaries strict evidence：五条 P1 exact（tick conversion fallback、subscription ref normalization/release、viewer capability cleanup、HK-only high-dividend state、quote-right entitlement cache）完成 Go/Rust 断言复核；integration-futu nextest 5/5 通过（live OpenD ignored），receipt `sha256:77b7aeff1ac829b88bffc92e012affa9c99f2f1b01599c1d957e5d7a1cac473f`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3632 降至 3622，整体仍未通过。
2026-09-28 API market-runtime strict evidence：五条 P1 exact（tick candle cache/provider fallback、industry forwarding、strategy order cancellation）完成 Go/Rust 断言复核；engine nextest 实际 7/7 通过（strategy 同名 owner 覆盖 lib 与 bounded integration binaries），receipt `sha256:ea1a72725d8f5cd97f2e15017b44283975b76e1aef4d2af17bab7a72ef24009b`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3622 降至 3612，整体仍未通过。
2026-09-28 Futu research/boundary strict evidence：五条 P1 exact（K-line/price helper、research/calendar pagination、disconnected market reads、snapshot-fallback coercion）完成 Go/Rust 断言复核；integration-futu nextest 实际 9/9 通过（live OpenD ignored），receipt `sha256:b32a88c5f551e7f3b00501146b2f5b3e32ec0a931078327803c0bcd36c2d493c`。映射完成 reviewed assertion 与 passed verification，strict gap 由 3612 降至 3602，整体仍未通过。
2026-09-28 Query/candle/depth strict evidence：五条 P1 exact（tick candle window/volume、session query shapes、candle adjustment validation、empty depth arrays、backtest session scope）完成 Go/Rust 断言复核；integration-futu/engine/marketdata nextest 实际 5/5 通过（live OpenD ignored），receipt `sha256:69bae53794658676a710eeda191d4e68b5e70c7f243ee3483c441f7919d75b40`。candle options 两个 Rust owner 补齐 Go 行 8 anchor；映射完成 reviewed assertion 与 passed verification，strict gap 降至 3581，整体仍未通过。
2026-09-28 Provider facade/search strict evidence：五条 P1 exact（Futu search 前置校验、industry 默认 kind、news nullable/asOf 投影、company financials 转发、provider 默认市场）完成 Go/Rust 断言复核；engine/calendar 定向 nextest 5/5 passed，receipt `sha256:143648491d86b0a5b7e9495ef6dd97bffc10d6797e5bd08e24396b332a94c20e`，随后 quick 门禁 2017/2017 Rust tests passed。映射完成 reviewed assertion 与 passed verification；strict gap 为 3571，整体严格审计仍未通过。
2026-09-28 Futu funds/K-line/snapshot boundary strict evidence：五条 P1 exact（币种余额优先级、US 当前桶过滤、历史 session fallback、账户 push/market rules、snapshot 非法行）完成 Go/Rust 断言复核；integration-futu/engine 定向 nextest 11/11 passed（live OpenD ignored），receipt `sha256:9304c9ef4cd2b9a4d36385012db4902c61c41bad326f7472345fdcafc0a3bbf7`。映射完成 reviewed assertion 与 passed verification；严格审计待重跑确认，整体仍未通过。
2026-09-28 Futu session/market-rules strict evidence：五条 P1 exact（缓存 OpenD client 重建、margin ratio TTL、撤单协议、market-rules fallback 矩阵、闭市 previous-close）完成 Go/Rust 逐项复核；定向 nextest 10/10 passed（live OpenD ignored），receipt `sha256:b290cd212da3374c17198d541103085cf5a0b71fedc8f0d1271732534f097167`。新增 session coordinator `Parity` 锚点；未宣称 strict 全局审计完成。
2026-09-28 Strategy/Pine cancel-all behavior evidence：`TestLiveCommandExecutorCancelAll` 新增两笔活动订单部分失败回归，验证逐笔派发、成功/失败审计、错误聚合及失败跟踪保留；engine nextest 2/2 passed，receipt `sha256:b0b2788e6b2fb67563b2da0e1a864b5b691c8d67d22707b40bf7150842508512`。映射继续为 `partial`，未将 execution-store 成功状态 owner 差异升级为 exact。
2026-09-28 Assistant workflow CRUD behavior evidence：`TestWorkflowResourceCrudPaginationAndLogs` 新增 tags 归一化、列表 limit 上限、trigger log 默认页大小、手动 trigger 空字段回退与 secretHash 脱敏回归，先红后修后 engine nextest 3/3 passed，receipt `sha256:98e9c06f94e261d4b0c53c0a7141ab81d861de627f5435ea45c07550092bee09`。映射保持 `partial`，删除后的完整 wire 状态与更多手动触发分支仍未完全覆盖。

2026-09-28 Strategy/Pine subscription warmup rollback：`failed_pine_warmup_releases_only_its_own_subscription_once` 通过共享 KLINE demand 证明预热失败只释放当前运行时的 consumer，重复 release 保持幂等；定向 engine nextest 4/4 passed，receipt `sha256:49469ad896186b5f59c614381ff653c63720c7df59c98fd01cfb557c937b01e9`。对应 Go 条目继续为 partial，尚缺租约获取失败时不创建运行时的注入式失败 seam。

2026-09-28 Strategy/Pine subscription normalization：`strategy_kline_subscription_refs_normalize_valid_targets_and_skip_malformed` 先红复现 `market=\".\"` 畸形目标进入 demand book，修复 mutation owner 后锁定唯一 `KLINE:US:AAPL:15m` 引用；定向 engine nextest 1/1 passed，receipt `sha256:38704e6be9b4cbe98401245af9407ae949eafb5e2a2df0eced6ebdbda5f2b654`。对应 Go 条目升级为 function_exact。
2026-09-28 Trading/Broker cancel rejection：`TestFakeBrokerConformanceCancelAcceptedAndCancelRejected` 修复明确券商拒单分支后保留 `CANCEL_SUBMITTED`、写入 `broker.cancel` 错误源并记录 `BROKER_CANCEL_REJECTED`；当前工作树定向 engine nextest 3/3 passed，receipt `sha256:e9fc3b9310f9cefbe81e13e61e7c3d37f0fa9aad09667e3e02406b10cb730001`，映射保持 `function_exact`。连接丢失仍保留 UNKNOWN fail-closed；旧失败 receipt 保留为历史先红证据。
2026-09-28 Strategy/Pine manager close drain：`TestManagerCloseDrainsActiveLiveSession` 对应 `manager_shutdown_closes_active_pine_sessions_once`，真实 StrategyRuntimeManager 在 shutdown 时 cancel/join 活跃任务并对已打开 Pine session 恰好发送一次 close，保持 session identity 与 revision 连续；engine nextest 2/2 passed，receipt `sha256:7fd9c2be86d54c740663edd6da3de92f391c27925949a5774809020f7ad558f0`。该条已由 partial 升为 function_exact，严格全局审计仍未通过。
2026-09-28 Strategy/Pine manager close 聚合错误：`TestManagerCloseAggregatesNamedSessionErrorsOnce` 对应 `manager_shutdown_aggregates_named_session_close_errors_once`，12 个并发 shutdown caller 收到相同的 `instance + market.symbol + pine session close` 聚合错误，两个活跃 session 各只收到一次 close；旧 `shutdown()` 保持 bool 兼容，production port 透传错误。engine nextest 4/4 passed，receipt `sha256:e319b1dd01b56b3ae8bb821aa23bd831c2eb4ca5216765035961fd92f0bdd185`。该条已由 partial 升为 function_exact；相邻启动竞态聚合测试仍为 partial。
2026-09-28 Strategy/Pine targeted cancel behavior evidence：`TestLiveCancelOnlyRemovesSuccessfullyCancelledTrackedOrders` 新增 SQLite execution store owner 测试，验证成功 targeted cancel 移除本实例 active tracking、失败保留 tracking、foreign/untracked 不触达 gateway；定向 engine nextest 3/3 passed，receipt `sha256:3423dac32787c4000997a77e747bdf13c64a024334cf4c27e9e341eb47d25fa3`。映射由 `partial` 升为 `function_exact`；`TestLiveCommandExecutorCancelAll` 仍保持 partial 记录 execution-store owner 边界。
2026-09-28 Strategy/Pine cancel-all success behavior evidence：`TestLiveCommandExecutorCancelAll` 新增 SQLite execution store owner 测试，验证两笔活动订单逐笔派发撤单并在成功后清空 active ledger；定向 engine nextest 9/9 passed，receipt `sha256:fc615c228ba5af017d074b6793421fc3e624cc447a0abd801c9a8ad8c8eb6901`。映射由 `partial` 升为 `function_exact`；失败分支仍由相邻回归锁定保留跟踪。

## 2026-09-28 P1 增量：Strategy/Pine lifecycle mutation

- Go：`internal/strategy/service_test.go:278:TestServicePauseAndStopInstancesStopRuntimeAfterStateTransition`
- Rust：`crates/jftrade-engine/src/strategy_runtime_mutation.rs` 的 `lifecycle_transition_failure_does_not_stop_runtime_before_state_write`、`pause_and_stop_transition_state_before_stopping_runtime`
- 先红后修：status trigger 拒绝时原实现先取消 Stop runtime；修正为状态 CAS 成功后才取消 Stop/Pause runtime，并在成功路径释放 demand。
- 定向验证：engine nextest 4/4 passed；raw output `sha256:6d761bf44026ffcc680076f180a0f83335a640c2e406df004a2489bfac747546`。
- 结论：`partial`。Rust 没有 Go refresher callback 的同形刷新计数断言；严格全局审计仍未通过。

2026-09-29 Strategy/Pine targeted cancel alias behavior evidence：`TestCancelByIntentDeduplicatesAliasesAndToleratesStaleMappings` 对应 `targeted_cancel_resolves_intent_aliases_once_and_tolerates_stale_mapping`，先红确认陈旧映射错误，再验证确定性 clientOrderId alias 只派发一次撤单、成功后清空 tracking，stale alias 成功 no-op；定向 engine nextest 3/3 passed，receipt `sha256:a3e1e7ac8664d078feccbdd1fd234bb2abf8e0ec6e2e51a2d3ad5c43de7f9fc7`。映射保持 `partial`，显式多腿 alias 表仍无 Rust 同形 owner。
## 2026-09-30 API Server / Transport Wire strict batch

SSE transport 5 条 exact 已完成真实断言复核并以 `jftrade-api` nextest 5/5 验证；receipt `sha256:23cb9085910da407cffce794533dee1ff1eea1154a3a50a766fa6afd67ed9ec9`。本批只计入 reviewed assertion + anchor + passed receipt 的逐项证据，strict gap 3395→3391；API Server 测试数量比例 10.2% 仍是风险信号，不作为完成率。
## 2026-09-30 Futu transport strict batch

Futu transport 5 条 exact 完成真实断言复核，联合 `jftrade-integration-futu`/`jftrade-engine` nextest 5/5，receipt `sha256:a0906c826c136f6c21afaaf156d85bc399947b9047e47431aa33fcc242ad4290`。strict gap 3385→3378；只按 reviewed assertion + anchor + passed receipt 计入，不以测试总数或 receipt 数量代表完成率。

## 2026-09-29 API Server / Transport Wire SSE frame-loop

5 条 SSE exact 完成 Go/Rust 断言复核：trigger/ticker 错误传播、write panic 恢复、PrepareSSEWriter headers 与完整 frame wire、无 flusher 拒绝、initial error 传播。Rust `jftrade-api` 联合 nextest 7/7 通过，receipt `sha256:8f5cf46fdb2e9bcd5af4d3de9b52781eb5cbbcc4af2ebdde8c267252e0c05ec4`。strict gap 3378→3368；本轮仅按 reviewed assertion + anchor + passed receipt 计入，API 测试比例不作为完成率。

## 2026-09-29 Futu P2 strict evidence

`TestConstructorFallsBackToDefaultAddress`、`TestSubscribeQuotes`、`TestQuoteSnapshotPreviousClosePriceZeroCurPrice` 已完成 reviewed assertion、Parity anchor 与 passed receipt。联合 engine/Futu nextest 3/3 passed，receipt `sha256:c121cb50db0c539b0df79c1bea408c2aaa59111be0295c6b9169b2bc875b30c2`；strict gap 3362→3353，未将测试数量或 receipt 数量作为完成率。

## 2026-09-29 Futu history partial correction

Go `TestQueryKLinesKeepsDailyHistoryLabelAsBucketStart` 与 `TestRequestHistoryKL` 的 Rust 对应测试分别验证时间 helper 和协议 frame，未验证真实客户端返回链。因此矩阵结论收窄为 reviewed `partial`，receipt `sha256:635332bacd351da3b2c6735ed4f467aa441967d1229b6c2170c5297b2e39f2d3` 仅作为子行为证据；strict gap 3347→3341。

## 2026-09-29 reviewed behavior and exact correction

Prediction push（3/3）、Assistant research backtest 审批模式（1/1）、candle adjustment parser/route（2/2）完成 Go/Rust 断言复核、anchor、reviewed assertion 与定向 nextest；receipt 分别为 `sha256:be911c287f663bb6151394cb091665b1e326ea59b6d42a81c4ecf3bd99a7bbf9`、`sha256:b4abf64334b6bc0f6d636a85b5acd3a32e9b2acbbb08e2b2fe278c7a2c487843`、`sha256:ef32d0c83f7b0df316aef3ae093965f5fc9970bed9693ef6fbdacb3a41c86edd`。Futu funds/order mapping 与 Pine asset 两组共 4 条旧 exact 因缺少 Go 全断言对应证据降为 reviewed partial。strict gap 最终 **3323**，整体未通过；降级造成的 gap 下降不计新增行为。
## 2026-09-29 Futu snapshot fallback strict batch

六条 snapshot fallback exact 完成真实 Go/Rust 断言复核：行与市场分组投影、输入 canonical/取消/错误与 clone 隔离、strict delayed quote protobuf、static-id 无订阅回退、15 秒正/负缓存和 StockScreen 错误透传。`jftrade-integration-futu` 定向 nextest 6/6 passed，receipt `sha256:5d2e8d55218adc5252b1389a258dbf15803b757a9bb05091ccd10b78c56cb8f6`。六条 mapping 均已写入 reviewed assertion、Parity anchor 和 verification receipt；strict gap **3323→3312**，API Server/Transport Wire 低测试比例与其余历史 exact 缺口继续保留。

## 2026-09-29 API auth/SSE strict batch

SSE loop 两条与 auth middleware 三条 exact 完成真实断言复核，`jftrade-api`/`transport_contracts` nextest 分别 2/2、3/3 passed，receipt `sha256:803a99b17ac95ec97490cbf3f1f91ee8c3aa95d061ee9966dfdd9217f3bae18e8`、`sha256:d9491a62ef3b46d0ff5ee03f652748d32fa1bd219460c95a0edd7e3748702b89`。strict gap 3303→3297。public-path 缺 `/health`/204 与 logout generic harness 200/204 差异已明确保留为 reviewed partial；API 测试数量比例不作为完成率。

随后补齐 `TestAuthRejectsNilAuthenticator`、`TestAuthRejectsUntrustedOrigin` 的 reviewed assertion 与 receipt；transport contract nextest 2/2 passed，receipt `sha256:d302cddef14057dec554dd8d48e00892a71e7c5a2e4c565fff7010970fbe800e`，strict gap 3297→3293。

## 2026-09-29 API P1 Web/Execution batch

禁用 Web 导航页、cookie+CSRF 浏览器流程、cookie-only WebSocket 三条完成 reviewed assertion 与联合 nextest 3/3，receipt `sha256:02d835502c6392862c7678ed3087c19e51198186c265a1583cc181bbac8d5b92`。ETH execution session 的 normalization/wire 测试 3/3 passed，receipt `sha256:8abeb1dae760d9861b6bd1f5070052482df98eb3047d517386db66084255dce8`，route seam 保留 partial；前端、password-change、logout 三条旧 exact 也已纠偏为 reviewed partial。strict gap 3293→3274。

启动 rollback P1 复核完成：Rust engine 两条 production startup/migration rollback 测试 2/2 passed，receipt `sha256:5ff41a57feb4610daf6dfae3a59381aad5bf567d79b1f659a0da8068872fa066`；generic Go `stores.Handle` callback/error-chain seam 保留 reviewed partial，strict gap 3274→3270。

## 2026-09-29 API transport P2 query/URI evidence

四条既有 `function_exact` 完成逐项 assertion review：candle period documented aliases、query time UTC normalization、malformed/literal URI percent handling，以及 execution write leaf 的真实 percent-id trim。`jftrade-engine` nextest 5/5 passed，receipt `sha256:97726dfe188f6691db04e9b03f19dec666d366f4c48d8498c5a06526966a9f23`；mapping、anchor reconcile、test-parity report/inventory 已同步。非法 query fallback 与 Gin required binding 仍按 partial 保留，未把 transport 形状差异写成 exact。strict gap **3270→3262**，全局 strict audit 仍未通过。

## 2026-09-29 API transport P2 route evidence

第二批六条 route exact 完成逐项复核：optional bool/time/limit aliases、四类 instrument 缺失参数 fail-fast、markets active-provider failure envelope、malformed refresh zero-call guard，以及 subscription lease 缺失的 409/恢复路径。`jftrade-engine` nextest 8/8 passed，receipt `sha256:84dbc60dbeb07184ad3c0cbc750321351de2bd6b052e61ea613ea329e0ba553c`；mapping、anchor reconcile、report/inventory 已同步。strict gap **3262→3250**，全局 strict audit 仍未通过。

## 2026-09-29 API transport P2 auth/origin evidence

第三批六条 middleware/origin exact 完成逐项复核：CORS 预检、认证器与 Origin 拒绝边界、无 browser Origin 会话读、Referer/Origin 选择及 canonical origin 表。`jftrade-api` nextest 7/7 passed，receipt `sha256:a90d53c5245a42b16cb1397efcf98d666f3bc7aa3c02e56a9dda6e60b19d59e2`；mapping、anchor reconcile、report/inventory 已同步。wails/tauri scheme 差异按已登记边界保留，strict gap **3250→3238**，全局 strict audit 仍未通过。

## 2026-09-29 API transport P2 subscription evidence

第四批七条 subscription exact 完成逐项复核：poll-only capability/lease precedence、malformed request fixture、valid target filtering、instrument contract、broker-neutral polling、consumer-scoped release 与 managed strategy lease preservation。`jftrade-engine` nextest 7/7 passed，receipt `sha256:66229dc2cec638a02dc06384b47949a7bca05c593b67daa57dbbce6cecb41710`；mapping、anchor reconcile、report/inventory 已同步。strict gap **3238→3224**，全局 strict audit 仍未通过。

## 2026-09-29 API transport P2 WebSocket live evidence

WebSocket live 5 条候选测试全部通过，receipt `sha256:d5630dbcfc1a2e7e10bdaf5bf09f96e992fbff7c53addf84023fa4f0880510dd`。provider-scoped tick dedupe、provider switch tagging 与 untrusted Origin rejection 完成 reviewed exact；heartbeat `liveClients` 缺失、Go Host-based same-origin 与 Rust allowlist 差异改记 reviewed partial。strict gap **3224→3210**，全局 strict audit 仍未通过。

## 2026-09-29 API transport P2 behavior review

7 条已有 `function_exact` 完成 Go/Rust 断言逐项复核：request observability 稳定上下文与错误摘要、非法 request id 替换、market-depth 方法/路径隔离、Swagger 核心路径、system status request id 传播、Web 登录失败限流与 `Retry-After`。`jftrade-api`/`jftrade-engine` 联合 nextest **9/9 passed**，receipt `sha256:ce452bd3c5eb1ac54fa17c8f9bd952a69c8e811dc9abc3b0bcd2e9503492a19d`；共享 owner reuse 已审核，7 条 mapping 升为 `reviewed` assertion。strict gap **3088→3065**，全局 strict 仍未通过。

随后补齐 margin-ratio unknown-stock owner 的唯一 `rustEvidence.anchor`（`trade_session_tests.rs:1969`），strict gap **3065→3064**；anchor 类 strict error 已归零，剩余为历史 receipt/assertion/reuse/filter 缺口。

## 2026-09-29 API transport P2 runtime behavior

8 条已有 `function_exact` 完成断言逐项复核：watchlist 不可用时 provider switch、startup restore warming health、Node 缺失路径/提示、strategy quantity 优先级与 close 百分比、managed account normalization、combo quantity mode。engine/marketdata/settings 联合 nextest **12/12 passed**，receipt `sha256:652cbc5eae4d4533af8b22ff2ea8f098cb6398bea05bf7f3d6e9871403d18d6b`；mapping 的 assertion coverage 已升为 `reviewed`，strict gap **3064→3048**，全局 strict 仍未通过。

## 2026-09-29 API transport P2 datamigration behavior

8 条 datamigration/SQLite exact 完成断言复核：backup retention 与 quota、不可兼容源库不变、失败快照清理与 SQLite verification、rebuild selection、manifest drift、schema catalog descriptors。`jftrade-store-sqlite` nextest **8/8 passed**，receipt `sha256:4d739bd52477eb28f8bf1dbdcaba199570f595d524ea28b7b89a9793aa5397a4d`；8 条 mapping 升为 `reviewed`，strict gap **3048→3032**，全局 strict 仍未通过。

## 2026-09-29 API transport P2 datamigration safety

8 条 rebuild-safety/backtest/broker route exact 完成断言复核：untrusted marker fields、pending/verified backup prerequisite、idempotent rebuild scheduling、protected digest errors、backtest compaction/read projection、broker incomplete paths。store-sqlite/engine nextest **8/8 passed**，receipt `sha256:c91a4ac3fe696e1fbebd5897d1f96b81b9e42433c0ea3761390452bc25ae0119`；strict gap **3032→3016**，全局 strict 仍未通过。

## 2026-09-29 API transport P2 read/settings behavior

10 条 exact 完成断言复核：market-scoped lookback、secure-random preview failure、finished sidecar stop、settings/market fixtures、market profile metadata、research preset fail-closed、settings environment isolation、onboarding defaults/readiness。联合 nextest **10/10 passed**，receipt `sha256:77a4805fce48b4565c50fe4d7475287a2977f08bae3486c1007be627fdbb65fc`；strict gap **3016→2996**，全局 strict 仍未通过。

## 2026-09-29 API transport P2 shared-owner behavior

5 条 exact 完成断言复核：rebuild marker snapshot retention、explicit K-line bounds、strategy current-bar worker intent、market-depth PUT rejection、legacy strategy source format。8 个 Rust owner tests 联合 nextest **8/8 passed**，receipt `sha256:9c451f9dc06c5c9679f7299c2d089593a3526086b535c497b1210fa8dc3f9080`；strict gap **2996→2986**，全局 strict 仍未通过。

## 2026-09-29 API runtime dependency shared-owner behavior

Node dependency OK/outdated/invalid/non-zero command 两条 exact 完成 shared owner 复核；`jftrade-engine` nextest **1/1 passed**，receipt `sha256:e12b481f0265f0680e5e6ca822d09c2ff5770ca08cd2924ddffe996a56c395a0`；strict gap **2986→2980**，全局 strict 仍未通过。

补充完成 optional query bool 与 candle adjustment 两个共享 owner 的 reuse 审核；既有行为 receipt 保持不变，strict gap **2980→2976**。该变化仅为证据关系收口，不作为新增行为数量。

## 2026-09-29 broker runtime exact correction

`TestBrokerRuntimeDescriptorIncludesReadFeatures` 与 `TestContractBrokerRuntime` 的 Rust 证据仅直接调用 production port，未覆盖真实 HTTP route/assembly。两条 mapping 改为 `partial` 并记录缺口；strict gap **2976→2969**，下降来自纠正过宽 exact，不计新增行为。

## 2026-09-29 API route owner batches

本轮新增 6 个行为批次的严格证据：Execution 9 条、Backtest 6 条、Strategy/Pine 3 条、Assistant workflow 2 条、Watchlist 7 条、Provider Research 7 条。各批次均有逐项 Go 断言说明、有效 Rust anchor、reviewed assertion、审核 reuse 关系和真实 passed receipt；对应 strict gap **2969→2853**。共享 owner 下的 partial/boundary 仍保持原结论，未因复用测试而扩大 exact 范围。全局 strict audit 仍未通过。

## 2026-09-29 live volume and websocket batch

13 条 live volume/heartbeat/websocket exact 已以 4 个真实 Rust owner 测试和 receipt 收口；assertion coverage、anchors、multi-reference reuse 均已审核。strict gap **2853→2821**，全局 strict 仍未通过，partial/boundary 项目保持原结论。

## 2026-09-29 execution validation batch

两条 execution validation exact（价格 tick/session 与显式 market+code）已完成真实 owner 测试、assertion review、anchor/reuse 审核和 receipt 绑定；strict gap **2821→2805**。

## 2026-09-29 system status batch

4 条 system status/runtime resource exact 已以 typed projection 与 resource count owner 测试收口，strict gap **2805→2794**；partial status-mapper 结论未被扩大为 exact。

## 2026-09-29 receipt coverage repair

对 live、watchlist、execution notification、data-plane cache、web disabled、reasoning、snapshot fallback、settings defaults 重新执行 owner 测试并替换 receipt；严格校验每条 reviewed mapping 的 Rust evidence 均在 executable testFilter 内。strict gap **2794→2775**。

## 2026-09-29 ADK catalog batch

3 条 ADK catalog/middleware exact 已由两个真实 production assembly owner 测试收口并完成 reuse 审核；strict gap **2775→2764**，partial/boundary 结论未扩大。

## 2026-09-29 runtime resources/lifecycle batches

runtime resource ownership 3 条与 lifecycle 6 条 exact 已完成 engine/desktop owner 验证和 reuse 审核；strict gap **2764→2732**，非同形入口仍保持 partial/boundary。

## 2026-09-30 API/Assistant route batches

本轮先修复 lifecycle receipt 的两个遗漏 owner（strict **2732→2729**），再完成 Assistant/API route 的 5 个行为批次：approval 与 mutation（7 tests）、stream/audit/validation（6）、boundary/catalog/CRUD（6）、idempotency/workflow（3）及共享 owner reuse 审核（8 owners）。对应 reviewed assertion、有效 testFilter、passed receipt 和 anchor 均已写入 `manual-test-mappings.json`；strict gap **2729→2641**。API Server/Transport Wire 的 10.2% 测试数量比例仍作为待推进信号，未被 receipt 或总数替代。

## 2026-09-30 transport/data-management batches

API/Transport route owner 批次真实 nextest **13/13** 通过，覆盖 backtest/research/prediction/broker/watchlist 路由并收口 9 条 exact；同时补齐 8 条 reviewed receipt 的 runner/toolchain 元数据。SQLite/data-management 批次真实 nextest **10/10** 通过，覆盖 schema failure、backup/rebuild、overview、cleanup 与 marker rollback。strict gap **2641→2592**；API Server/Transport Wire 的 10.2% 数量比例和全局 strict 未通过状态保持透明。

## 2026-09-30 replay/assembly batches

backtest P1 旧 receipt 的 10 个 owner 重跑 **10/10 passed**，修复短 commit 与 testFilter 不完整证据；Assistant P1 旧 receipt 重跑 **9/9 passed**；另有 5 个无共享 owner 的 assembly/MCP exact 通过 **5/5**。strict gap **2592→2579**，共享 owner 的其他 legacy 引用仍未被自动升级。
## 2026-09-30 API marketdata forwarding evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API marketdata forwarding（calendar/company/news/rankings/screen/index） | 15 function_exact | engine production ports：29 个 forwarding/capability/helper-isolation/limit 测试 | nextest 29/29 passed；receipt `sha256:51abd6e018b…` | 15 条 legacy assertion 升为 reviewed；15 个小 fan-out reuse 已审核；高 fan-out 仍 backlog |

strict evidence gap **2554→2503**。该批次只把有真实行为断言和当前 receipt 的条目升级为 reviewed，数量比例与 receipt 数量不作为完成率。
## 2026-09-30 API servercoretest evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API servercoretest（backtest/broker/system/strategy/settings/watchlist） | 19 function_exact | engine、settings、settings-file、SQLite：23 个 route/contract/runtime 测试 | workspace nextest 23/23 passed；receipt `sha256:d0f9cbb88920…` | 19 条 legacy assertion 升为 reviewed；17 个小 fan-out reuse 已审核；高 fan-out 仍 backlog |

strict evidence gap **2503→2441**。本批次只升级有真实行为断言和当前 receipt 的条目，数量比例与 receipt 数量不作为完成率。
## 2026-09-30 API servercore behavior evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API servercore behavior（data-management/volume/notification/strategy/health/security） | 17 function_exact | engine、datamanagement、SQLite、settings：33 个匹配 owner 实例 | workspace nextest 33/33 passed；receipt `sha256:022cdab8ccec…` | 17 条 legacy assertion 升为 reviewed；19 个低 fan-out reuse 已审核；高 fan-out 仍 backlog |

strict evidence gap **2441→2376**。仅计真实行为测试与 reviewed 证据，不以数量比例或 receipt 数量代替完成率。
## 2026-09-30 API marketdataapp behavior evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API marketdataapp（provider switch/sidecar/health/search/depth/kline/subscription） | 19 function_exact | engine、marketdata、Futu、marketdata-helper：22 个 owner 测试 | workspace nextest 22/22 passed；receipt `sha256:af682a98cf08…` | 19 条 legacy assertion 升为 reviewed；19 个低 fan-out reuse 已审核；高 fan-out 仍 backlog |

strict evidence gap **2376→2308**。只计真实行为测试和 reviewed 证据，不以数量比例或 receipt 数量代替完成率。
## 2026-09-30 API remaining legacy closure evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API remaining（application/startup/Futu/lifecycle/status/combo/web auth/settings） | 19 function_exact | engine、desktop、Futu、settings：25 个 owner 测试 | workspace nextest 25/25 passed；receipt `sha256:848b85e4751f…` | API transport owner legacy exact 清零；21 个低 fan-out reuse 已审核，高 fan-out 仍 backlog |

strict evidence gap **2308→2244**。API 数量比例仍单独作为覆盖信号，不作为完成率。
## 2026-09-30 Strategy/Pine parse evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Strategy/Pine parse（parse/analyze/validate/metadata/history/security/risk） | 21 function_exact | jftrade-strategy、jftrade-engine：21 个 owner 测试 | workspace nextest 21/21 passed；receipt `sha256:fa86abe492ee…` | 21 条 legacy assertion 升为 reviewed；13 个低 fan-out reuse 已审核；47-way framework owner 仍 backlog |

strict evidence gap **2244→2188**。只计行为测试和 reviewed 证据，不以测试数量或 receipt 数量代替完成率。
## 2026-09-30 Strategy/Pine live execution evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Strategy/Pine live execution（stop/risk/scope/quantity） | 7 function_exact | engine strategy runtime、trading risk：19 个 target 实例 | workspace nextest 19/19 passed；receipt `sha256:dc4d66ed418c…` | 7 条 legacy assertion 升为 reviewed；低 fan-out reuse 已核对 |

strict evidence gap **2188→2172**。不以测试数量或 receipt 数量代替行为完成率。
## 2026-09-30 Strategy/Pine risk/order evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Strategy/Pine risk/order（risk modes/qualified positions/order bounds） | 8 function_exact | jftrade-trading、jftrade-strategy：10 个 owner 测试 | workspace nextest 10/10 passed；receipt `sha256:a8d9603e2f3f…` | 8 条 legacy assertion 升为 reviewed；5 个低 fan-out reuse 已审核 |

strict evidence gap **2172→2152**。

## 2026-09-30 Strategy/Pine legacy exact closure evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Strategy/Pine legacy exact（runtime/planner/parser/Pine worker） | 29 function_exact | engine、jftrade-strategy、jftrade-integration-pine：32 个唯一 owner 测试 | workspace nextest 35/35 passed；receipt `sha256:22918fba9517…` | 29 条 legacy assertion 升为 reviewed；19 个低 fan-out reuse 已审核，高 fan-out 仍 backlog |

strict evidence gap **2139→2067**。只计真实行为测试、reviewed assertion、有效 receipt 与受限 reuse。

## 2026-09-30 API transport envelope and reuse evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API envelope + auth/origin/CSRF/CORS/SSE reuse | 1 partial→function_exact；9 shared owners | jftrade-api envelope、auth、SSE、transport contracts | envelope 2/2 passed；reuse 9/9 passed；receipts `sha256:552d9d0eb2f…`、`sha256:720c76ce1c5…` | 补齐统一 404 envelope 行为；9 个低 fan-out reuse reviewed，高 fan-out 继续 backlog |

strict evidence gap **2067→2053**。API 行为比例 10.6% 仅作覆盖信号，不作为完成率。

## 2026-09-30 API logout HTTP projection evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Web logout HTTP projection | 1 partial→function_exact | auth manager、product auth-session write、auth-session route fixture | engine nextest 3/3 passed；receipt `sha256:3a34c8940c10…` | HTTP 200、清除 cookie、token invalidation 与 route wire 均有断言；3-way reuse reviewed |

strict evidence gap **2053→2052**。middleware stub 204/production logout 200 的边界仍保留 partial。

## 2026-09-29 Legacy exact 与 API receipt refresh evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Exchange calendar legacy exact | 45 | 47 个 calendar/integration owner | nextest 47/47 passed；`sha256:bdb85e7b823cab1499e69e4faaef6074ef223c99ad0c4e12ac370730dc16dafb` | assertion reviewed，真实 parser/manager/source 行为已绑定 receipt |
| Store legacy exact | 28 | 30 个 calendar/settings/engine/sqlite owner | nextest 30/30 passed；`sha256:269e5473ac3ace141090615278dd73cdc32a3d6355b305904cf6a14a5b7e50ae` | assertion reviewed；未以测试数量作为完成率 |
| Settings legacy exact | 16 | 19 个 settings/engine/Futu owner | nextest 19/19 passed；`sha256:2bc75ca653eaf944b8f88dc5496f36ea8658f7cebc06260d75f4f62a6d150690` | assertion reviewed，receipt 字段可审计 |
| API transport receipt refresh | 11 | 16 个 engine owner | nextest 16/16 passed；`sha256:32fe1451be0c9565c8fab2733dbb134212bfb3221d39e338f8e985b24c49e506` | 修复旧短 SHA/testFilter 严格缺口；高 fan-out relation 仍 backlog |

strict evidence gap **908→818→762→730→689→678**；anchor reconcile `1895/1848/0/0/47`。严格审计仍未通过。

## 2026-09-29 Receipt refresh、legacy review 与受控 relation evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API runtime receipt refresh | 8 | 10 个 engine/store/Futu owner | nextest 10/10 passed；`sha256:d80813b72ec8bac46581fee167f0557ac764593319771b70665b8a286f22be8f` | 旧短 SHA receipt 替换为当前可审计字段 |
| Assistant receipt refresh | 30 | engine/store-sqlite ADK owner | nextest 30/30 passed；`sha256:b051bebb952afa819447ac98af1767b5e1b9cf3de737c2814a915aca2d9787dc` | receipt refresh，不抬高 partial/boundary |
| System/Backtest/Researchscreen legacy review | 9 / 4 / 7 | engine、research owner | nextest 9/9、4/4、7/7 passed；receipts `sha256:eb4c5569…`、`sha256:10cd6f60…`、`sha256:fc21b9c2…` | legacy-conclusion 批量升为 reviewed |
| Controlled high fan-out relation review | 3 owners | marketdata quote/candle/news owners | strict relation check passed for reviewed refs | 仅人工逐项比较后放行；其余高 fan-out 继续 backlog |

strict evidence gap **908→818→762→730→689→678→670→640→614→600→595**；数量比例、测试总数和 receipt 数量均不作为完成率。

## 2026-09-29 小模块与 API catalog evidence

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| asset/security/retry/Futu integration | 3/3/3/6 | helper、settings、Futu owners | nextest 4/4、4/4、3/3、6/6 passed | legacy assertion reviewed，receipt 字段完整 |
| settings/watchlist/research/desktop | 18 | settings、engine、watchlist、desktop owners | nextest 18/18 passed；`sha256:0e58f1a2…` | 合并批次逐 mapping testFilter |
| small legacy owner batch | 8 | engine、kernel、Pine、desktop | nextest 9/9 passed；`sha256:df8913aa…` | 8 条 mapping reviewed |
| API instrument-search relation | 2 exact references | market-data catalog owner | strict relation review completed | 仅该 owner relation 放行；更高 fan-out 继续 backlog |

strict evidence gap **908→818→762→730→689→678→670→640→614→600→595→587→581→575→559→523→511→509**；严格审计仍未通过。

## 2026-09-29 Assertion review batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assertion coverage review | 13 | ADK reasoning、calendar JSON、Futu/Pine proto、desktop repository/update/asset owners | 定向 nextest 13/13 passed；receipt `assertion-review-2026-09-29.json`；digest `sha256:0c0cd9360e979c1b0ca737052037eee818f431ae89b9715f8357f4f058c50ea5` | 13 条 `function_exact` 的 assertion coverage 升为 reviewed；strict gap 468→455，剩余仅为 shared-owner reuse review |

## 2026-09-29 API/settings high-fan-out review

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API/settings shared owner | 20 references（4 exact、16 partial） | `product_server_persists_ui_settings_and_reports_actual_port` | engine nextest 1/1 passed；receipt `api-settings-product-owner-reviewed-2026-09-29.json`；digest `sha256:9d9a7491868053886619fb1edef4e9617c45f4a022132a86ef4078812dccf36f` | relation reviewed；仅 4 条 exact 释放，16 条 partial 保持差异结论；strict gap 455→451 |

## 2026-09-29 API/datamigration maintenance owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API/datamigration maintenance owners | 8 relations（9 exact references） | SQLite maintenance retention/overview/rebuild + engine pending rebuild | 定向 nextest 9/9 passed；receipt `api-maintenance-owners-reviewed-2026-09-29.json`；digest `sha256:2ec3da6ae462d9ddd856d2ad3f07e6b90085e49ba1722e71eb38d2cf33926b11` | 8 relations reviewed；9 exact 释放，其他 partial/boundary 结论不变；strict gap 451→442 |

## 2026-09-29 API/watchlist read-owner review

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API/watchlist read owner | 11 references（1 exact、10 partial/boundary） | SQLite watchlist read page + engine write compatibility fixture | nextest 2/2 passed；receipt `api-watchlist-read-owner-reviewed-2026-09-29.json`；digest `sha256:ce73e5c1ec405a4d571db8ba59fd37229c8649df3feb4f9146c6fb02ba088e70` | relation reviewed；仅 1 exact 释放，strict gap 442→441 |

## 2026-09-29 API marketdata forwarding/cache owners

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API marketdata forwarding/cache owners | 9 shared relations | Futu probe、marketdata activation/cache/demand、engine snapshot/tick/session owners | 定向 nextest 9/9 passed；receipt `api-marketdata-owners-reviewed-2026-09-29.json`；digest `sha256:e18e93372b58803d03bb62430a16c4986d1be8fa7fbd11769728c7656a8312a0` | exact reuse 逐引用释放，partial/provider/conversion 差异保持；strict gap 441→429 |

## 2026-09-29 API forwarding-wire owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API forwarding wire owners | 18 relations（34 exact references） | engine research/calendar/company/news/index/screen/depth + marketdata-helper health | 定向 nextest 18/18 passed；receipt `api-forwarding-wire-owners-reviewed-2026-09-29.json`；digest `sha256:eaf29e07e48c810bcb4b966db1af2b5eb6dd402f9d52a0693e88e5b8c5a87117` | exact reuse 逐引用释放，partial/boundary 差异保留；strict gap 429→395 |

## 2026-09-29 API/runtime tail and candle validation

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API/runtime tail | 6 relations（6 exact references） | sidecar process、strategy cancel、backtest sync、settings corpus、watchlist group、market status | nextest 8/8 passed；receipt `api-runtime-tail-owners-reviewed-2026-09-29.json`；digest `sha256:fb934a7746ba8b0769a53ab160c8227f33566121feef6a502767f4c65f60a3f9` | exact reuse reviewed，strict gap 395→387 |
| Candle adjustment validation | 1 relation（4 exact references） | engine candle route adjustment/rehab validation | nextest 1/1 passed；receipt `api-candle-adjustment-owner-reviewed-2026-09-29.json`；digest `sha256:85cdd550afe72f285445698cd12609e990def5404277cf258e2a29e0d79a3a11` | 4-way exact reuse reviewed；strict gap 387→377 |

## 2026-09-30 Assistant MCP policy owner review

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant MCP policy | 2 relations（4 exact references） | engine MCP loopback/Host guards | nextest 2/2 passed；receipt `assistant-mcp-policy-reviewed-2026-09-30.json`；digest `sha256:ded615b93f06adf18d4b6ec4233b16e0670f58b40f6f74d2103ac6fa0163464e` | exact relation reviewed；MCP lifecycle partial 保持；strict gap 377→373 |

## 2026-09-30 Assistant shared-owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant shared owners | 9 relations（18 exact references） | claims/lease、tool-failure、timeout、approval、workflow threshold、continuation owners | nextest 9/9 passed；receipt `assistant-shared-owner-batch-2026-09-30.json`；digest `sha256:90f36484e3ef5ed3d742f8eb111b59fb68fd9e9cb54a0f6b5948174ab810825e` | exact reuse reviewed，partial 差异保留；strict gap 373→355 |

## 2026-09-30 Futu/marketdata shared-owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/marketdata shared owners | 4 relations（12 exact references） | research normalization、basic quote、batch snapshot、research allow-list | nextest 4/4 passed；receipt `futu-marketdata-shared-owners-reviewed-2026-09-30.json`；digest `sha256:d6d7efaaf8bab7719f70c1000de9044374b303b4b8d543a6526a1f8d7be557e7` | exact reuse reviewed，partial catalog 差异保留；strict gap 355→343 |

## 2026-09-30 Futu/engine exact-pair batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/engine exact-pair owners | 21 relations（42 exact references） | OpenD quote/session/trade/subscription/runtime + engine market-data/trade/execution | nextest 21/21 passed；receipt `futu-engine-exact-pairs-reviewed-2026-09-30.json`；digest `sha256:db5bfa62fdacea96e71ca5c648e3a554eca3ad6ca4f1aff7142a7cbe1360951a` | 逐引用 exact reuse reviewed；strict gap 343→301 |

## 2026-09-30 Futu triple-owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu triple owners | 7 relations（14 exact references） | quote normalization, security validation, tick fallback, interval, recoverable error, order-book subscription, session recovery | nextest 7/7 passed；receipt `futu-triple-owner-reviewed-2026-09-30.json`；digest `sha256:a64ff016f2bbb625e64b480deb054496e909e0e2e4837fa6f9919696b07838e9` | exact reuse reviewed；strict gap 301→287 |
## 2026-09-30 Assistant MCP server owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant MCP server owners | 14 relations（14 exact references） | account/portfolio、strategy/backtest/model tools、listener lifecycle、catalog registration、dependency/unsafe-host guards、workflow wait | nextest 14/14 passed；receipt `assistant-mcp-server-owner-reviewed-2026-09-30.json`；digest `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | exact reuse reviewed；HTTP fetch、schema、product dispatch 保留 backlog；strict gap 287→272 |

## 2026-09-30 Assistant application/tool-catalog owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant application/tool-catalog owners | 16 relations（16 exact references） | workflow/execution/trade、market candle/backtest、portfolio/research、optimization、tool catalog、capability 与 instrument boundaries | nextest 17/17 passed；receipt `assistant-application-owner-reviewed-2026-09-30.json`；digest `sha256:9d64884dbd4dfa97317b1272f9a616f8980daf39aea3b663c90748e49c5b3c69` | exact reuse reviewed；10-way strategy binding与高 fan-out ADK relation保留 backlog；strict gap 272→256 |

## 2026-09-30 ADK runtime pair owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| ADK runtime pair owners | 20 relations（20 exact references） | input conflict、approval、lease fencing、run/session projection、terminal audit、context compaction | nextest 20/20 passed；receipt `assistant-adk-runtime-pairs-reviewed-2026-09-30.json`；digest `sha256:4a81b9bc4b2324ea45c9ed00b94198fe11e0766718c7e061596c2136ce0af21a` | exact reuse reviewed；高 fan-out runtime/store relation保留 backlog；strict gap 256→236 |

## 2026-09-30 API route pair owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API route pair owners | 20 relations（20 exact references） | execution validation/preview、system health/status、calendar/news/company、broker query、market adjustment、WebSocket、Futu rules/order-book/notification | nextest 20/20 passed；receipt `api-route-pairs-reviewed-2026-09-30.json`；digest `sha256:5e63078e658a06423b3c1b0453baeb53f72de24a2e26a4da3faea60c003c084c` | exact reuse reviewed；partial/provider 边界保留；strict gap 236→216 |

## 2026-09-30 API route tail owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API route tail owners | 2 relations（2 exact references） | Futu notification/quote-right labels、settings backtest-provider atomic preparation | nextest 2/2 passed；receipt `api-route-pairs-tail-reviewed-2026-09-30.json`；digest `sha256:a65d2d16e2e20bb0f2a9e74fdb690e8733f1cf4623ddc3da7c1f92c38f5c00d6` | exact reuse reviewed；strict gap 216→214 |

## 2026-09-30 API route triple-owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| API route triple owners | 14 relations（14 exact references） | execution combo/snapshot、marketdata batch/quote/subscription、trade analytics/funds、Futu descriptor/retry/notification、market profile、maintenance、watchlist cache | nextest 14/14 passed；receipt `api-route-triples-reviewed-2026-09-30.json`；digest `sha256:5548a060148522457f38e479d3d4d3ef8efa868c9137f035ccf75a79d75585ad` | exact reuse reviewed；higher fan-out 与 mixed partial 保留；strict gap 214→200 |

## 2026-09-30 ADK runtime triple-owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| ADK runtime triple owners | 13 relations（13 exact references） | pending input recovery、tool-only turn、expiry/fencing、memory/provider gate、terminal audit、handoff、session pagination | nextest 13/13 passed；receipt `assistant-adk-runtime-triples-reviewed-2026-09-30.json`；digest `sha256:40a8bbf45d8b4d060d82e0b10b6ba4240623818c154fc9145587541e96f1a333` | exact reuse reviewed；higher fan-out runtime/store 保留 backlog；strict gap 200→187 |

## 2026-09-30 Assistant claims/runtime owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant claims/runtime owners | 6 relations（6 exact references） | workflow ordering、tool replay、claim fencing、approval continuation/idempotency、lease heartbeat | nextest 6/6 passed；receipt `assistant-claims-owners-reviewed-2026-09-30.json`；digest `sha256:d100bdc4c3bf6e5e5c77e48d8db495259f294334cae5137a63752f23cd182cf3` | exact reuse reviewed；higher fan-out ADK/store 保留 backlog；strict gap 187→181 |

## 2026-09-30 ADK store owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| ADK store owners | 7 relations（7 exact references） | atomic projection、provider default、artifact/session durability、writer fencing、approval idempotency | nextest 7/7 passed；receipt `adk-store-owners-reviewed-2026-09-30.json`；digest `sha256:98fd30e706a5cae7d009a6e00852927d73aaee0352a081f637926548e9e22885` | exact reuse reviewed；higher fan-out store 保留 backlog；strict gap 181→174 |

## 2026-09-30 Futu/OpenD pair owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/OpenD pair owners | 20 relations（20 exact references） | quote/tick batching、health/version/status、instrument search/static info、kline、managed session、order-book projection | nextest 20/20 passed；receipt `futu-opend-pairs-reviewed-2026-09-30.json`；digest `sha256:687a59f3da757e1fbf5c57147f130f111fc42f726855149bd81cad3f625482a1` | exact reuse reviewed；高 fan-out与mixed partial保留；strict gap 174→154 |

## 2026-09-30 Futu/OpenD pair owner batch 2

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/OpenD pair owners 2 | 20 relations（20 exact references） | quote-rights/cache、security snapshot、subscription lifecycle/reconcile、trade proto/session、watchlist gate、recovery、frozen proto、prediction push | nextest 20/20 passed；receipt `futu-opend-pairs-2-reviewed-2026-09-30.json`；digest `sha256:2c24a6e94b730a3dad00c289924da912d36acdfec2e161f307f1f47fc0f56a70` | exact reuse reviewed；高 fan-out与mixed partial保留；strict gap 154→134 |

## 2026-09-30 Futu/OpenD pair tail owner

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/OpenD pair tail | 1 relation（1 exact reference） | user security group type encoding/projection | nextest 1/1 passed；receipt `futu-opend-pair-tail-reviewed-2026-09-30.json`；digest `sha256:1f56d2b79405bbad59b530e99ddcb46d9ac8e5589a56c759a6b1636dd90c2290` | exact reuse reviewed；strict gap 134→133 |

## 2026-09-30 Cross-domain pair owner batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Cross-domain pair owners | 20 relations（20 exact references） | calendar snapshot、ADK task/provider/notices/tool declarations、execution/reconciliation、marketdata、research projections、broker capabilities | nextest 20/20 passed；receipt `cross-domain-pairs-reviewed-2026-09-30.json`；digest `sha256:11200ac86d98093d5f2a7439a503bd0412ad759a418ff2bfaa4dbbbed90f49d8` | exact reuse reviewed；高 fan-out与mixed partial保留；strict gap 133→113 |

## 2026-09-30 Futu/OpenD triple owner batch 2

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Futu/OpenD triple owners 2 | 16 relations（16 exact references） | search/malformed errors、K-line period mapping、snapshot rate-limit/empty results、subscription reconcile、funds/trade projection、watchlist cache/duplicate semantics、closed-session/error propagation、prediction normalization、tick-candle and execution-store boundaries | workspace nextest 16/16 passed；receipt `futu-opend-triples-2-reviewed-2026-09-30.json`；digest `sha256:7564322fb6753e5a1262f184c00d6d07694179d6a6f606f295dd7c648e7f2933` | 16 个 exact reuse relation 升为 reviewed；strict gap **72→56**；剩余高 fan-out relation 继续 backlog，不以数量或 receipt 计完成率 |

## 2026-09-30 Mixed high-fanout owner batches

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Mixed high-fanout owners 1 | 20 relations（25 exact references） | broker rules/funds/capabilities、ADK runtime/store、Pine/research、calendar、Futu snapshot/prediction | nextest 21/21 passed；receipt `mixed-highfanout-owners-reviewed-2026-09-30.json`；digest `sha256:07427002da46199e0b8a3eb5cb3526c1d79234605b017330e2aca6940f460c75` | exact reuse reviewed；strict **56→31** |
| Mixed high-fanout owners 2 | 20 relations（20 exact references） | ADK expiry/session/context、calendar freshness、broker write/read boundaries、asset/store lifecycle | nextest 20/20 passed；receipt `mixed-highfanout-owners-2-reviewed-2026-09-30.json`；digest `sha256:e96c23604f55078abb65f9f54d5892a17f7671bca9a48afd63666b112f8547ac` | exact reuse reviewed；strict **31→11** |
| Final Futu/research owners | 11 relations（11 exact references） | notification、quote rights、order book、search、history/session、security snapshot 与 company projections | nextest 11/11 passed；receipt `final-futu-research-owners-reviewed-2026-09-30.json`；digest `sha256:400c22b7ae211ed0eb4cd8c490347c8dcd808d4bb8b19b7b4e39710513d17061` | 全量 `function_exact` strict gap **11→0** |

## 2026-09-30 API transport SPA boundary

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| SPA index request boundary matrix | 1 mapping（由 partial 升 function_exact） | axum router root/HTML/尾斜杠、Accept JSON、扩展名/assets 缺失回退矩阵 | `jftrade-api` nextest 1/1 passed；receipt `api-transport-spa-boundary-reviewed-2026-09-30.json`；digest `sha256:f3c8b8358b7572660ce891f978438721ec363b73ea1b649e9746a896359b83b8` | 真实差异先红后修；API 数量比 10.6%→10.7%，不计完成率；strict 仍全量通过 |

## 2026-09-29 API provider-test wire batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Provider test route wire | 1 mapping（由 partial 升 function_exact） | `product_production_ports_adk_mutation_runtime::test_provider` 与生产 mutation route：默认 quick/full/slow、missing-provider 状态码与 envelope | engine nextest 4/4 passed；receipt `api-provider-test-wire-reviewed-2026-09-29.json`；digest `sha256:b4ce4838b0f6ebe37e9474caabda06913c14ab226e3fbcc7ba332fd324ee6800` | 真实差异先红后修，strict function_exact 证据 1492→1493；数量比例不计完成率 |

## 2026-09-30 API assistant chat/SSE wire batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Assistant chat/SSE route contract | 1 mapping（由 partial 升 function_exact） | production ADK chat route + live SSE stream：chat `200/ok` envelope、成功流 Content-Type、配置化 idle timeout、session→run→terminal 顺序与 durable session id | engine nextest 1/1 passed；receipt `api-assistant-chat-sse-contract-reviewed-2026-09-30.json`；digest `sha256:ef17475a3ca52c8d11254d0a921de827174661872903d9d93a6ea34a09c353f8` | 关闭 provider 的 JSON failure projection 与成功 loopback SSE 分开验证；strict function_exact **1493→1494**，strict audit 通过；API 数量比不作为完成率 |

## 2026-09-30 API live WebSocket shutdown lifecycle batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| Live WebSocket connection/close lifecycle | 1 mapping（由 partial 升 function_exact） | production WebSocket limit/permit owner + ProductHandle shutdown + LiveHub depth subscription release | engine nextest 2/2 passed；receipt `api-live-handler-close-lifecycle-reviewed-2026-09-30.json`；digest `sha256:14c38cbc91d70bb6b9a627c8fddd509294d9899c7a119ba0bf6c20079930b181` | 红测修正 extended-length test frame 与 queued text drain；strict function_exact **1494→1495**、partial **2322→2321**，strict audit 通过；数量比不作为完成率 |
## 2026-09-30 API bindings required-path wire batch

| 批次 | Go 映射 | Rust 行为 owner | 验证 | 结论 |
|---|---:|---|---|---|
| BindURI required path + escape matrix | 1 mapping（由 partial 升 function_exact） | URI escape owner + axum router required path wire | API nextest 3/3 passed；receipt `api-bindings-wire-exact-reviewed-2026-09-30.json`；digest `sha256:df775fcfd9a151371616e8dfe64aef9996d0f1c84088b7d4925d8cd6eba2ec0f` | 已认证缺参请求返回 404 JSON envelope 且 port 不调用；先红后修确认认证顺序；strict function_exact **1495→1496**，数量比例不作为完成率 |
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
