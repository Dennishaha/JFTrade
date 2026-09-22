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

## 第一百二十批（分片二）：读查询投影升锚、指针辅助判界、lotSize 记账（3 条）

### 范围

- `pkg/broker/broker_test.go:97`：`TestConvertFutuReadQuery`。
- `pkg/broker/broker_test.go:114`：`TestPointerHelpersReturnStableIndependentValues`。
- `pkg/broker/broker_test.go:157`：`TestApplyMarketRuleUsesLotSizeAsQuantityConstraints`。

结论：`:97` 升 `[x]`/`function_exact`（引用 MCP 生产工具执行器的 futu 读作用域回归测试）；
`:114` 由 `partial` 改判 `boundary`；`:157` 保留 `[~]`/`partial`，结论改为“引用已占用、无功能缺口”。

### 关键事实（本批 recon 与实测）

- **Go 的 `ReadQuery.BrokerID` 在 Rust 由路由承载**：Rust 读端口没有 `brokerId` 字段，
  `crates/jftrade-engine/src/product_mcp_production_executor.rs::portfolio_summary` 把
  `accountId`、`tradingEnvironment`、`market` 逐字段编码成查询串，打到
  `/api/v1/portfolio/futu/*` 与 `/api/v1/brokers/futu/orders` 三个 futu 作用域路由，
  并在 `brokerId` 非 futu 时以 `BAD_REQUEST` 拒绝。因此“券商身份固定 + 三个读作用域字段原样透传”
  这一 Go 契约在 Rust 有可断言的等价证据。
- **指针辅助函数没有 Rust 对应物**：可选字段用 `Option<T>`，`Some(value)` 按值传递，
  不存在 Go 那种“两次 `new(v)` 返回不同地址”的可观察差异，也没有可迁移的导出 API。
- **lotSize 行为已等价，缺的只是记账**：`apply_market_rule` 在 `lot_size > 0` 时同时写
  `min_quantity` 与 `step_size`，与 Go `ApplyMarketRule` 的 lot 分支一致；该 Rust 测试已被
  `pkg/futu/exchange_test.go:62` 的 `[x]` 行占用，按 `[x]` 引用全局唯一约束不在本行重复占用。

### 新增锚点

`crates/jftrade-engine/src/product_mcp_server_tests.rs::portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers`
补写 `/// Parity: go:452dea11:pkg/broker/broker_test.go:97` 锚点与说明（仅注释，无行为变更）；
该测试此前无任何 Parity 锚点，本次为 `[x]` 引用补上代码侧证据，使清单声明与代码注释相互对应。

### 批次结果

- 8 条全部给出结论：**1 条升 `[x]` + 6 条改判 `boundary` + 1 条保留 `partial`（引用占用、无功能缺口）**。
- `pkg/broker/broker_test.go` 归零待办：10 条 = **3 `[x]` + 6 `boundary` + 1 `partial`**，0 `missing`。
- 全局：4451 = function_exact **1356** + partial **2496** + boundary **595** + module_only 4 + missing 0
  （本批前为 1355 / 2503 / 589 / 4 / 0）。Rust 测试 3098（本批未新增测试函数）。
- reconcile：anchors **1360**、已记账 **1304**、unrecorded 0、unknown 55、stale 1
  （stale 为既有 `internal/api/trading/execution_test.go:47`，与本批无关）。

### 后续待办（trading_broker 域剩余 71 条 partial）

按文件收敛顺序：`pkg/broker/catalog_test.go` 6、`internal/trading/broker_test.go` 5、
`internal/trading/risk_status_broker_boundaries_test.go` 5、`internal/trading/execution_products_test.go` 4、
`internal/trading/responses_test.go` 4、`internal/trading/order_updates_test.go` 14（并发/重连/恢复子集）、
`pkg/broker/research_screen_test.go` 3、`pkg/broker/product_capability_contracts_test.go` 2、
`internal/trading/control_plane_*_test.go` 6、`internal/trading/service_test.go` 3 及其余单条文件。

### 验证记录（分片二）

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 映射写入 | `python3.12 /tmp/b82_apply.py /tmp/b120_p2.json` | 3 行写入，`[x]` 1355 → 1356，键集 4451 不变 |
| 格式 | `cargo fmt --all --check` | 通过 |
| 定向测试 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-broker --all-targets --locked -E 'test(portfolio_summary_merges_positions_balances_and_orders_and_rejects_unknown_brokers) or test(broker_lot_size_initializes_minimum_and_step_quantity)'` | 通过 |
| 审计 | `python3 scripts/compatibility/audit_test_parity.py` | 通过：1356 function_exact 引用均可解析、无重复引用 |
| 锚点对账 | `python3.12 scripts/compatibility/parity_anchor_reconcile.py` | anchors 1360、已记账 1304、unrecorded 0、unknown 55、stale 1（既有） |

## 批次门禁记录（第一百二十批）

| 门禁 | 命令 | 结果 |
| --- | --- | --- |
| 格式 | `cargo fmt --all --check` | 通过 |
| 静态检查 | `pnpm run check:rust:clippy`（另跑 `cargo clippy -p jftrade-engine -p jftrade-trading -p jftrade-store-sqlite --all-targets --locked`） | 通过 |
| 定向测试 | 见分片二验证记录 | 2 条通过 |
| 三 crate 全量 | `node scripts/quality/cargo-nextest.mjs run -p jftrade-engine -p jftrade-trading -p jftrade-store-sqlite --all-targets --locked --no-fail-fast` | 通过：2049 条全绿 |
| workspace 测试 | `pnpm run test:rust` | 通过：3195 条通过、2 条 skipped |
| 兼容 replay | `pnpm run check:compatibility` | 通过（storage/backtest/provider-runtime/trading-strategy/assistant-runtime/api-transport/desktop-runtime） |
| 生成物 | `pnpm run check:generated` | 通过：contracts 校验未改动工作树 |
| Zero-Go | `node scripts/check-zero-go.mjs` | 通过：2938 个跟踪文件、0 个发布产物 |
| AI 上下文 | `pnpm run check:ai-context` | 通过：6 个模块、8 个指令文件 |
| 架构/策略/目标健康 | `pnpm run check:rust:architecture`、`check:rust:production-policy`、`check:rust:target-health` | 全部通过（`.rcgu.o` 35986 < 50000） |
| 工作树空白 | `git diff --check` | 通过 |
| 快速门禁 | `pnpm run check:quick` | **未全通过**：唯一失败为既有 `check:rust:static`（`advisories FAILED`），其余阶段通过 |
| Rust 全量门禁 | `pnpm run check:rust` | **未通过（既有阻断）**：`check:rust:static` 在 advisories 阶段中止，退出码 1 |
| workspace 门禁复跑 | `pnpm run check:rust:workspace` | 第二次通过（第一次因既有 launcher 抖动失败，见下） |

### 既有阻断与抖动说明（与本批无关）

- `check:rust:static` / `check:rust:policy` 的失败原因为 `cargo deny check advisories`
  报 1 条 `error[vulnerability]`（RUSTSEC-2026-0285，TLS 1.3 握手消息跨加密层级被错误接受）
  与 8 条 `warning[advisory-not-detected]`（`deny.toml` 中 RUSTSEC-2024-04xx Tauri/GTK3 忽略项已不再命中）。
  本批未改动 `deny.toml` 或依赖树，按既有事实如实标注“未通过”。
- `crates/jftrade-engine/tests/product_api_launcher_lifecycle.rs::api_launcher_serves_on_the_configured_address_and_stops_on_termination_signal`
  在并行高负载下偶发失败（`left: None, right: Some(0)`，未走 shutdown 路径）；隔离复跑与随后
  的 `check:rust:workspace` 全量复跑均通过，属既有抖动而非本批回归。
