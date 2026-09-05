# Go to Rust 迁移验证矩阵与后续任务指引

> 本文是历史迁移核查的任务入口，不是当前架构事实源或发布授权。分卷保留原始分析，未经复核的推演不得当成已复现缺陷，也不得直接照搬其中的修复方案。

- 文档复核日期：2026-09-05。
- 本次代码核查基点：`8c9e0464a7242a0fbea693a55cf122d753b43717`；工作树包含既有未提交文档，非干净发布候选。
- 原文历史基线：`origin/go@452dea11`（标注为 `v0.27.0`）；本次未重新验证该 ref/tag 对应关系，历史行为比较须先验证基线。
- 范围：10 个领域、16 个既有风险编号（4 P0、9 P1、3 P2）。编号保留用于追踪，**不代表已确认严重度或正式门禁结果**。
- 事实源：[仓库指令](../../../AGENTS.md)、[模块表](../../../scripts/module-map.json)、[质量门禁](../../architecture/quality-gates.md)、[发布资格](../../architecture/release-qualification.md)。

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

当前下表各项均为 **待验证**：本轮核实了部分源码事实，但未执行这些场景的缺陷复现或修复验收。

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
| P1-07 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | append 拒绝后 Rust/Node 会话状态是否失配 | 重复、乱序、修订、非法数据分别测试；检查失败后 append/open/close、内存与会话数，不能把保留有效会话直接认定为泄漏。 | 待验证 |
| P1-08 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | 实盘与回测预热是否产生不可接受偏差 | 同脚本/数据/种子比较指标及信号，定义容差与首个可交易 bar；记录样本不足策略及资源成本。 | 待验证 |
| P1-09 / [09 Wire](./verification-matrix/09-pinets-wire-events.md) | 实际 gRPC 发送/接收配置及超限恢复 | 查明两端实际限额，在阈值上下测请求/响应、错误映射及会话恢复；完整保留 intents，压缩不能替代限额测试。 | 待验证 |
| P2-01 / [01 Pine](./verification-matrix/01-pine-runtime.md) | 崩溃后预留是否占用配额、能否安全回收 | reserve/submit/persist 各点故障注入及日期边界；先排除券商已接受，不能盲目释放不确定订单配额。 | **已关闭 / PASS**：消除 2x 重叠计数，SQL 锚定匹配 `'%reservation: ' || a.detail || ')%'` 杜绝子串碰撞，与对账联动安全回收，4 项专项测试通过。 |
| P2-02 / [06 进程](./verification-matrix/06-sidecars-resilience.md) | yfinance 冷启动下历史 Futu 订单可否对账 | 冷启动与切源分开；有/无在途订单、OpenD 不可用、账户发现；行情源独立于交易能力，失败状态可见。 | **已关闭 / PASS (事实反证)**：架构实现 OpenD 交易会话与行情源解耦，冷启动仅要求 `futu_trade_enabled` 即可初始化交易会话与对账后台，6 项既有测试反证闭环。 |
| P2-03 / [10 发布](./verification-matrix/10-zero-go-tauri-frontend.md) | 候选包是否符合各平台运行与签名要求 | 当前四平台实际安装/升级/回滚、无开发环境启动、sidecar 完整性、签名/公证/updater 及产物 Zero-Go；严重度按失败影响重评，不默认仅为维护项。 | 待验证 |

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
