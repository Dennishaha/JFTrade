# 2026-09-06 Go / Rust 行为复核

## 基线与证据边界

- `git fetch origin go` 确认远端基线 `452dea115ca75c51361e8876c2aefd7c009839b8`。
- 本地 main 为 `9072fe9b36a5d0e893558c59d9f27a61021624d9`，开始时工作树干净。
- 修复在工作树，未提交、未推送；未访问真实 OpenD、账户或用户数据库。
- 此次发现旧测试未覆盖的安全回归，不能继续使用历史“16 项全部关闭”作为完整迁移结论。

## 已修复

| 编号 | 问题 | 修复 |
| --- | --- | --- |
| R1 | 活动/历史快照没有候选时直接写 FAILED，并声称券商从未接受；停止非终态对账。 | 保持 UNKNOWN 和后续对账资格；重复扫描事件幂等，迟到的有身份回执仍可恢复。查不到不再等同于拒绝。 |
| R2 | 仅凭标的、数量、价格、时间窗口猜配唯一外单；用户备注也能绕过属性校验。 | 删除属性猜配及任意备注匹配。仅接受实际通过 remark 发送的非空 clientOrderId 回显，并核对订单属性。证据不足保留 UNKNOWN，不重报。 |
| R3 | 所有本地订单的 ID 被全局排除，其他账户/环境/市场的同号订单阻止恢复。 | 按 broker/account/environment/market 隔离已认领 ID，与 Go 索引作用域一致。 |
| R4 | 查询截止到 10:00，就把 09:30–10:00 产出为完成的 60m 栏。 | cutoff 不再充当收盘边界；只有真实 session 尾部允许短桶，保留缺分钟校验。 |

依据：

- Go `internal/store/trading/submission_safety_test.go` 的 UNKNOWN 不重试约束；`internal/store/trading/ledger.go::findInternalOrderIDLocked` 的作用域索引；`pkg/backtest/internal/storage/store_aggregate.go` 的完整桶/结束时间过滤。
- Rust [恢复实现](../../../crates/jftrade-engine/src/product_production_ports_execution_reconciliation_recovery.rs)、[身份测试](../../../crates/jftrade-engine/src/product_production_ports_execution_reconciliation_identity_tests.rs)、[聚合实现](../../../crates/jftrade-store-sqlite/src/backtest_market_data_aggregation.rs)、[聚合测试](../../../crates/jftrade-store-sqlite/tests/backtest_market_data_session_dst_aggregation.rs)。

R1/R2 属于近期迁移收尾新增恢复逻辑的回归，不代表 Go 原本具有同样的自动认领功能。旧测试“无候选必须 FAILED”“时间窗内必须认领”的错误预期已纠正；恢复成功的 fixture 明确携带 clientOrderId 回显。

修复前新增的三个身份测试均失败；R4 cutoff 测试在 30 分钟处实际返回 1 根、预期 0 根，确认复现。R3 测试覆盖四种异作用域，既有测试继续覆盖同作用域重复认领保护。

## 全局检查与实际覆盖

直接解析远端 `tests/fixtures/openapi-baseline.json` 与本地 `contracts/openapi/openapi.json`，对象键排序后比较：278 个方法/路径无增删，全部 paths 对象和 468 个 definitions 对象相同。这是静态契约证据，不是副作用全覆盖证明。

七类冻结语料均通过，实际覆盖如下：

| 能力 | 回放输出 |
| --- | --- |
| storage | 2 张表、3 根 K 线 |
| backtest | 5 场景、8 成交 |
| provider-runtime | 14 行情操作、9 Pine 生命周期操作、3 OpenD 订阅、3 健康探针 |
| trading-strategy | 10 状态、7 转换、6 命令计划、7 更新事件、5 持仓刷新、3 策略场景；零下单 |
| assistant-runtime | 9 状态、12 转换、3 输入拒绝、2 持久 claim、3 工作流任务、2 artifact 版本、3 流片段 |
| api-transport | 278 静态操作、18 路由组、19 具体请求探针 |
| desktop-runtime | 3 平台配置、6 链接场景、10 facade 命令、4 事件；不是四平台安装验收 |

验收命令：

- `check:contracts` 通过，含只读 `check:generated`；276 认证路由、2 公开路由、128 浏览器写操作防护检查通过。
- 对账专项 52 个测试通过。
- 聚合/会话专项 17 个测试通过。
- 最终 `pnpm run check:rust` 退出码 0：1491 个测试通过、2 个默认跳过；fmt、Clippy、架构/生产策略、cargo deny 与七类回放通过。跳过项是显式 live OpenD 测试和依赖原生 Pine worker 的冒烟，不计为通过。
- 最终 `pnpm run check:quick` 退出码 0：受影响 Rust 测试 979 个通过、Pine worker 测试 92 个通过，policy/contracts、增量 Clippy、七类回放和桌面脚本检查通过。
- `git diff --check` 与报告本地链接存在性检查通过；公开契约、生成物和 lockfile 未改变。最终只追加本文验收结果，未再修改受测生产代码。

## 未闭环差异与验证缺口

以下没有计为本轮已修复，不应随绿灯自动关闭：

1. **日历与聚合范围**：Go `pkg/market/session.go` 通过当前 calendar resolver 取日历；Rust 聚合自行硬编码 session/部分短交易日，未消费 CalendarManager 的人工覆盖和来源优先级。假期、人工停市、跨日 extended、日/周/月下级聚合、历史预热还需生产差分测试。R4 不解决这些范围。
2. **外部券商订单发现**：Go `OrderUpdatesWorker::Sync/HandleOrderUpdate` 将新发现订单交给 `upsertBrokerOrderWithSource` 建账；Rust `reconcile_pending_orders` 从本地候选出发。需补验仅券商侧存在的手工订单是否经其他入口持久化，以及重启后 events/来源/手续费一致性。实时券商列表可见不等于执行账本落库；不能用 R2 的猜配代替外单发现。
3. **Worker 边界**：`MAX_VISUAL_OUTPUTS=1000` 限制对象数而不是编码字节数；大文本、plots/logs/intents 仍需真实 gRPC 阈值测试。drawingCount 对可变对象/定长滚动数组也需要增量更新测试。本轮未修改 worker wire 或截断订单意图。
4. **ADK / 九库 / Provider / 桌面**：冻结回放与 workspace 测试不等于逐个重跑历史故障注入、真实旧安装数据升级、全部跨库删除中断点或四平台安装签名。未重验的历史 PASS 不是本轮结论。

若真实库已被旧恢复逻辑误写 FAILED 或误绑定，本修复不会猜测并自动改写历史；需要导出事件与券商回执后另行核对。本轮没有用户数据操作或发布资格结论。
