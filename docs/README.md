# JFTrade 文档导航

本文档面向两类读者：

- 想快速理解当前系统边界的开发者
- 需要让后续 AI 协作不跑偏的维护者

如果你只看一篇，请先看 [architecture.md](architecture.md)。

## 阅读顺序

### 1. 先理解系统现在是怎么跑的

- [architecture.md](architecture.md)：当前系统架构、双运行模式、核心数据流、职责边界。

### 2. 再看你要改的专题

- [troubleshooting.md](troubleshooting.md)：排障入口，按启动、实时 SSE、OpenD、行情时段分流。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口，包含实时合成与防回归约束。
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：前端 DSL 策略设计专题，覆盖 Logic Flow、Monaco、模板和 visualModel 同步约束。

### 3. 最后看参考资料

- [reference/README.md](reference/README.md)：协议细节、上游 bbgo 资料、历史参考文档入口。

## 文档分层约定

为避免信息密度失控，docs 统一按下面的规则组织：

- 顶层入口文档只回答“这块是什么、边界在哪里、先看哪篇”。
- 实现细节、边界条件、回归案例放到专题子目录，不继续堆在总览文档里。
- 协议原文、上游资料、长篇背景说明放到 reference 层，不干扰当前架构判断。

## 当前文档地图

```text
docs/
├── README.md                 文档导航
├── architecture.md           当前系统架构总览
├── troubleshooting.md        排障入口
├── frontend-kline.md         前端行情/K 线入口
├── troubleshooting/          排障专题
├── frontend/                 前端与行情专题
	├── strategy-authoring.md   前端策略设计专题
├── reference/                协议与参考资料
	└── bbgo-doc/              上游 bbgo 参考资料（保持原样）
```

## AI 协作约定

后续 AI 在动手前应优先按下面顺序取上下文：

1. 先读 [architecture.md](architecture.md)，确认是在改 sidecar、bbgo 运行时、前端，还是 Futu 适配层。
2. 如果是启动、端口、连接问题，再进 [troubleshooting.md](troubleshooting.md)。
3. 如果是策略定义、Logic Flow、DSL 脚本同步或编辑器问题，再进 [frontend/strategy-authoring.md](frontend/strategy-authoring.md)。
4. 如果是实时行情、K 线、实时 SSE、快照合成问题，再进 [frontend-kline.md](frontend-kline.md)。
5. 只有在需要协议或上游背景时，才进入 [reference/README.md](reference/README.md) 或 [reference/bbgo-doc/README.md](reference/bbgo-doc/README.md)。

## 后端测试分层（持续更新）

为降低单文件测试复杂度，`pkg/jftradeapi` 的测试按业务域拆分维护：

- `server_test.go`：保留 sidecar server 通用行为与非专题场景。
- `server_backtest_routes_test.go`：聚焦回测创建路由参数归一与执行结果校验场景。
- `server_backtest_routes_warmup_test.go`：聚焦回测创建后根据策略历史 K 线推导 warmup 并校验执行结果场景。
- `server_backtest_run_store_test.go`：聚焦回测运行记录重载恢复与终态删除约束场景。
- `server_strategy_definitions_test.go`：聚焦策略定义创建与删除保护场景。
- `server_strategy_definitions_lifecycle_test.go`：聚焦策略定义实例化后的编译产物、绑定更新与 start/pause/stop/delete 状态流转场景。
- `server_strategy_definitions_preview_test.go`：聚焦策略定义预览与 legacy source format 归一场景。
- `server_strategy_runtime_logs_test.go`：聚焦策略实例列表、运行日志与审计分页筛选场景。
- `server_strategy_definition_sync_test.go`：聚焦策略定义版本同步与 refresh-definition 场景。
- `server_market_data_test.go`：聚焦行情订阅获取、心跳续约与释放场景。
- `server_plugin_catalog_test.go`：聚焦插件目录读取、安装/卸载与操作记录场景。
- `broker_routes_test.go`：保留 broker 路由断连与兜底响应场景。
- `broker_routes_read_exchange_test.go`：聚焦 broker 路由读侧 exchange-backed 主流程场景。
- `broker_routes_test_http_helpers.go`：集中 HTTP 解包与地址端口小工具。
- `broker_routes_test_opend_server.go`：集中 OpenD mock server 生命周期、协议收发与响应分派。
- `broker_routes_test_normalize_helpers.go`：集中 broker route 测试用数据归一化与过滤 helper。
- `strategy_runtime_manager_test.go`：聚焦策略运行时基础行为（通知模式、市场元数据补齐）。
- `strategy_runtime_manager_test_helpers.go`：集中策略运行时测试共享桩与通用辅助函数。
- `strategy_runtime_manager_trading_test.go`：聚焦策略运行时交易执行链路（live 下单、持仓刷新、断连回退、K 线补轮询）。
- `strategy_runtime_manager_polling_test.go`：聚焦策略运行时闭市 K 线轮询补单场景。
- `strategy_runtime_manager_observation_test.go`：聚焦运行时观测输出、重启后观测持久化与 panic 自动收敛场景。
- `execution_command_routes_test.go`：聚焦 execution 命令主链路（下单、撤单、事件写入与列表联动）。
- `execution_command_validation_test.go`：聚焦 execution 命令参数归一与输入校验（US 精度、交易时段透传、symbol/code 约束）。
- `execution_routes_test.go`：聚焦 execution 订单同步链路与 worker 状态追踪。
- `execution_push_writeback_test.go`：聚焦 execution 推送回写通知与已发现订单复用场景。
- `market_data_test.go`：聚焦行情 K 线 session 规则（intraday session 路由与日线 session 元数据约束）。
- `market_data_realtime_bucket_test.go`：聚焦行情 K 线实时桶拼接场景（history + current bucket 合并）。
- `market_data_snapshot_tick_test.go`：聚焦行情快照与 tick 路径（cache 命中、cache miss、强制刷新与回退）。
- `market_data_security_details_test.go`：聚焦证券详情类型块映射与 snapshot/staticInfo 联合查询校验。
- `market_data_fixture_test.go`：聚焦行情测试的 OpenD mock server 生命周期与基础报价响应。
- `market_data_fixture_runtime_helpers_test.go`：集中行情测试运行时组装、tick cache seed 与 snapshot/tick 响应断言辅助。
- `market_data_fixture_security_test.go`：聚焦行情测试的证券快照与静态信息 fixture 组装。
- `market_data_fixture_kline_test.go`：聚焦行情测试的历史/当前 K 线响应夹具与 K 线 proto 构造辅助。
- `market_data_depth_test.go`：聚焦盘口深度端点路由注册、HTTP 方法校验、num 参数钳位、市场/代码大小写归一、mock OpenD 响应校验、空盘口、OpenD 错误传播及路由防碰撞场景。
