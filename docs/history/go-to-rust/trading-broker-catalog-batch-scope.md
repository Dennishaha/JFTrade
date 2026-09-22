# Trading Broker 域：`pkg/broker/catalog_test.go` 剩余 6 条 partial 收口（第一百二十一批）

本文件记录 `pkg/broker/catalog_test.go` 中第 93 批遗留的 6 条 `partial` 的逐条收口。
该文件是 Go 能力目录（capability catalog）与券商特性路由（`BrokerFeatureRouter`）的测试集合：
adapter 接口支持探测、只读 MCP 安全校验、显式/默认/回退选择、目录校验与排序分支、
路由失败与产品维度分支、候选排序去重与原因回落。Rust 侧对应实现分布在
`crates/jftrade-engine`（能力目录投影 `product_broker_capabilities_projection.rs`、
生产适配绑定 `product_production_adapter_bindings_tests.rs`）与
`crates/jftrade-marketdata`（`ProviderRouter` 激活/健康与行情域选择）。

分片计划（每片一次提交）：

- 分片一 `:44`、`:75`、`:98`：接口支持探测、只读 MCP 安全校验、显式选择与回退；
- 分片二 `:141`、`:195`、`:233`：目录校验/排序分支、路由失败与产品维度、候选排序与原因回落。

## 第一百二十一批（分片一）：接口支持、只读 MCP 与显式选择（3 条）

### 范围与结论

- `pkg/broker/catalog_test.go:44`：`TestAdapterInterfaceSupportRejectsNilMissingAndUnknownImplementations`
  → 保留 `[~]`/`partial`（反射式接口探测在 Rust 无对应入口，正半区由 adapter 安装独立性断言覆盖）。
- `pkg/broker/catalog_test.go:75`：`TestCapabilityCatalogRejectsUnsafeWriteMCP`
  → 升 `[x]`/`function_exact`（非法目录在 Rust 不可构造，不变量由全量 feature 断言覆盖）。
- `pkg/broker/catalog_test.go:98`：`TestBrokerFeatureRouterHonorsExplicitSelectionAndFallback`
  → 保留 `[~]`/`partial`（显式选择不回退已覆盖，configured fallback 候选列表在 Rust 不存在）。

分片一后全局计数：4451 = function_exact **1357** + partial **2495** + boundary **595** +
module_only 4 + missing 0（本批前为 1356 / 2496 / 595 / 4 / 0）。

### 关键事实（本批 recon 与实测）

- **adapterInterface 是发布元数据，不是查询入口**：`product_broker_capabilities_projection.rs`
  中 `spec.adapter` 仅用于 `catalog_feature` 的 `"adapterInterface"` 字段（第 155 行），
  目录没有任何按接口名查询实现的 API；运行时可用性由 `feature_runtime_available` 按
  feature id + 组合根实际装入的 reader 判定，因此 Go 的 nil、未实现名称、未知名称三分支
  在 Rust 不可构造。
- **只读 MCP 不变量由构造层排除非法目录**：`surface.readOnlyMcp` 只在 `access == "read"`
  时写入，write/trade 组合不可构造；`capability_access_classes_bound_the_reviewed_read_only_surface`
  对全部 feature 正向断言 read → `read_only`/`none`/`readOnlyMcp=true` + 工具在
  `REVIEWED_READ_ONLY_TOOLS` 内，write → `write_external`/`high` 且不在只读面，
  trade → `live_trading`/`critical` 且不在只读面，并校验访问类恰为三类、trading 计数 6。
- **显式 broker 选择的不回退语义已对齐**：`capabilities_filters_by_broker_market_and_feature_id`
  断言 `brokerId=other` 时 `brokers=[]`、`runtime=[]`（不回落到 futu）、catalog 仍可发现；
  这就是 Go「显式 brokerId 锁定后不得回退」的可观察等价物。缺失的是多 broker 候选列表与
  `configured_fallback` 原因值——Rust 只有单一 futu adapter，provider 侧 reason 仅
  `explicit_broker`/`active_provider`。

### 升锚明细

| Go 测试 | Rust 入口 | 结论 |
| --- | --- | --- |
| `catalog_test.go:75 TestCapabilityCatalogRejectsUnsafeWriteMCP` | `product_broker_capabilities_projection_tests.rs::capability_access_classes_bound_the_reviewed_read_only_surface` | `[x]`：非法目录不可构造，逐 feature 断言三类访问与只读 MCP 边界（覆盖强于 Go 单例拒绝）。 |

本批为该 Rust 测试补写 `/// Parity: go:452dea11:pkg/broker/catalog_test.go:75` 锚点
（该测试原有 `adk_product_catalog_test.go:29` 锚点保留，仅新增注释，无行为变更）。

### 保留差异与后续待办

- `:44` 缺口：Rust 无 name→bool 的接口能力矩阵；若将来引入，应在组合根新增查询并对
  nil/未知名称返回 false，再补回归。当前可达证据为 per-operation adapter 独立性
  （`prediction_readiness_is_independent_per_operation_adapter`）与
  `declared_features_publish_the_reader_interface_that_gates_them`（已被
  `product_capability_contracts_test.go:159` 的 `[x]` 行占用）。
- `:98` 缺口：`configured_fallback` 候选列表与注册顺序语义需先引入 broker 候选注册表
  （显式选择 / 注册顺序 / 回退列表 / reason 取值）才能补回归。

### 验证记录（分片一）

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b121_p1.json` | 3 行写入，`[x]` 1356 → 1357，键集 4451 不变 |
| 定向测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(prediction_readiness_is_independent_per_operation_adapter) or test(capability_access_classes_bound_the_reviewed_read_only_surface) or test(capabilities_filters_by_broker_market_and_feature_id)'` | 3 条通过 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过：1357 function_exact 引用均可解析、无重复引用 |
| 锚点对账 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1361、已记账 1305、unrecorded 0、unknown 55、stale 1（既有） |
