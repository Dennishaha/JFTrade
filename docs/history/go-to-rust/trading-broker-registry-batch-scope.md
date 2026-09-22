# Trading Broker 域：`pkg/broker/broker_test.go` 剩余 8 条 partial 收口（第一百二十批）

本文件记录 `pkg/broker/broker_test.go` 中第 93 批遗留的 8 条 `partial` 的逐条收口。
该文件是 Go 券商中立抽象层（broker registry、Futu 读查询转换、指针辅助、行情规则
lotSize 归一）的测试集合；第 93 批已收口 `TestApplyMarketRuleIgnoresMissingAndInvalidLotSize`
与 `TestBrokerError` 两条（`[x]`），本批处理其余 8 条。Rust 侧对应实现分布在
`crates/jftrade-engine`（组合根 `ProductionPortBundle`、能力目录投影、MCP 生产工具执行器）、
`crates/jftrade-broker`（中立 taxonomy、`apply_market_rule`）与
`crates/jftrade-marketdata`（`ProviderRouter` 注册/激活/停用）。

分片计划（每片一次提交）：

- 分片一 `:13`、`:27`、`:46`、`:67`、`:85`：五个 broker registry 语义（空表、注册查询、
  替换、删除、重复注册 panic）逐条改判 `boundary`；
- 分片二 `:97` Futu 读查询转换升 `[x]`、`:114` 指针辅助改判 `boundary`、
  `:157` lotSize 断言保留 `partial` 并记录引用占用事实。

## 第一百二十批（分片一）：多券商注册表语义改判边界（5 条）

### 范围

- `pkg/broker/broker_test.go:13`：`TestRegistryBasic`。
- `pkg/broker/broker_test.go:27`：`TestRegistryRegisterAndLookup`。
- `pkg/broker/broker_test.go:46`：`TestRegistryReplaceUpdatesActiveBroker`。
- `pkg/broker/broker_test.go:67`：`TestRegistryRemoveDeletesOnlySelectedBroker`。
- `pkg/broker/broker_test.go:85`：`TestRegistryDuplicatePanics`。

结论：5 条全部由 `partial` 改判 `boundary`（状态仍为 `[~]`）。全仓 `function_exact` 保持
**1355**、`partial` 2503 → **2498**、`boundary` 589 → **594**、`module_only` 4、
`missing` 0（合计 4451 不变）。

### 关键事实（本批 recon 与实测）

- **Rust 没有运行时 broker 注册表**：`crates/jftrade-engine/src/product_production_ports_types.rs`
  的 `ProductionPortBundle` 以显式端口字段（`Arc<dyn ..Port>`）注入适配器，
  `installed_adapters` 是 `ProductionRouteAdapter` 闭集枚举的 `BTreeSet`，由
  `derive_installed_adapters()` 从具体端口推导。构造完成即安装完毕，不存在
  “先建空表、后注册”的状态，也没有按 id 增删或枚举 broker 的查询面。
- **“缺失”只按路由可见**：`adapter_binding()` 对未安装适配器返回 `None`，
  readiness 走 `MissingInternalAdapter`/`ExternalUnavailable`；Go 的
  `IDs()/All()/ActiveBroker()` 这类全局列表在 Rust 无对应入口。
- **最近似的运行时 owner 在行情域**：`crates/jftrade-marketdata` 的 `ProviderRouter`
  以 `selection_id` 为键支持 `register`（覆盖注册项）、`activate`、`deactivate`
  （停用并栅栏缓存、推进代际）与 `update_health`，对象是行情 Provider
  （futu/yfinance/akshare）而不是 broker 实例。这些语义只在结论中作为“非等价邻居证据”记录。
- **重复注册在类型层不可达**：`ProductionRouteAdapter` 是闭集枚举，
  `BTreeSet` 插入同值幂等，结构上无法出现两条同 id 条目；静态目录另有
  feature/协议 id 唯一性守卫。Go 的 panic 分支在 Rust 无对应代码路径。

### 改判明细

| Go 测试 | 边界原因（不适用） | Rust 侧保留差异 | 非等价邻居证据 |
| --- | --- | --- | --- |
| `:13 TestRegistryBasic` | 无“空注册表”可查询对象，安装状态由组合根闭集表达 | 未知 brokerId 只表现为过滤后的空列表，不是空注册表 | `capabilities_filters_by_broker_market_and_feature_id` |
| `:27 TestRegistryRegisterAndLookup` | 无按 id 注册后查询入口，端口由组合根注入 | 安装是否生效按路由 readiness 暴露，无 `ActiveBroker` | `prediction_readiness_reports_missing_uninstalled_adapter` |
| `:46 TestRegistryReplaceUpdatesActiveBroker` | 无同 id 换 broker 实例入口，运行期切换对象是行情 Provider | Provider 切换 fail-closed + 代际栅栏，Go 替换为立即覆盖 | `provider_activation_fails_closed_preserves_previous_generation_and_recovers_after_health_update` |
| `:67 TestRegistryRemoveDeletesOnlySelectedBroker` | 无按 id 删除 broker 入口；停用走唯一 owner 的进程级 shutdown | `deactivate` 未注册 id 返回 `ProviderNotFound`，Go 静默无副作用 | `deactivation_fences_cache_and_marks_router_inactive` |
| `:85 TestRegistryDuplicatePanics` | 无运行时注册 API，重复注册状态不可构造 | 类型系统 + 目录守卫替代 panic，错误分支不可达 | `every_catalog_protocol_maps_to_a_feature_with_one_stable_id` |

### 将来升级的修复位置与回归要求

若项目引入多 broker 运行时注册表，应在 `crates/jftrade-engine` 组合根新增
`BrokerRegistry`（id 键；`register/lookup/ids/all/active/replace/remove`），并让
capabilities、readiness 与列表路由读同一 owner（唯一写入所有权）。回归至少覆盖：

- 空表：`ids` 为空、`active` 为空、`lookup` 未命中；
- 注册：注册后即可 lookup，首注册成为 active，`ids/all` 与插入顺序无关；
- 重复：同 id 二次注册被拒（显式错误而非 panic），首个实例仍可查询，计数不变；
- 替换：同 id 替换后查询到新实例、active 跟随；新 id 独立新增；替换失败保持旧实例；
- 删除：删除单 id 后其余 id 不受影响，删除不存在 id 的幂等语义需显式记录。

### 验证记录（分片一）

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b120_p1.json` | 5 行写入，`[x]` 1355 → 1355，键集 4451 不变 |
| 邻居证据复跑 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine --all-targets --locked -E 'test(portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers)'` | 通过（构建 1m49s，断言 0.198s） |
