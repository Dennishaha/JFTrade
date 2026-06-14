# HTTP API

> 自动生成，请勿手改。来源：`docs/swagger/swagger.json`。

## adk

### `GET /api/v1/adk`

**Summary:** 读取 ADK 快照

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `500` | httpserver.Envelope | Internal Server Error |

### `GET /api/v1/adk/agent-templates`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/agents`

**Summary:** 读取 ADK Agent 列表

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `status` | query | no | string | Agent 状态过滤 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `500` | httpserver.Envelope | Internal Server Error |

### `POST /api/v1/adk/agents`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/agents/{agentId}`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `PUT /api/v1/adk/agents/{agentId}`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/approvals`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/approvals/{approvalId}/approve`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/approvals/{approvalId}/deny`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/audit`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/chat`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/chat/stream`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/memory`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/memory`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/memory/{memoryId}`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/metrics`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/optimization-tasks`

**Summary:** ADK optimization task routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/optimization-tasks/{taskId}`

**Summary:** ADK optimization task routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/optimization-tasks/{taskId}/cancel`

**Summary:** ADK optimization task routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/providers`

**Summary:** 读取 ADK Provider 列表

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `500` | httpserver.Envelope | Internal Server Error |

### `POST /api/v1/adk/providers`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/providers/{providerId}`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `PUT /api/v1/adk/providers/{providerId}`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/providers/{providerId}/test`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/runs`

**Summary:** 读取 ADK Run 列表

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `limit` | query | no | integer | 分页大小 |
| `offset` | query | no | integer | 分页偏移 |
| `status` | query | no | string | Run 状态 |
| `agentId` | query | no | string | Agent ID |
| `sessionId` | query | no | string | Session ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `500` | httpserver.Envelope | Internal Server Error |

### `GET /api/v1/adk/runs/{runId}`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/runs/{runId}/cancel`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/sessions`

**Summary:** 读取 ADK Session 列表

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `limit` | query | no | integer | 分页大小 |
| `offset` | query | no | integer | 分页偏移 |
| `agentId` | query | no | string | Agent ID |
| `query` | query | no | string | 搜索关键字 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `500` | httpserver.Envelope | Internal Server Error |

### `POST /api/v1/adk/sessions`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/sessions/{sessionId}`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/sessions/{sessionId}`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `PUT /api/v1/adk/sessions/{sessionId}`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/sessions/{sessionId}/context`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/sessions/{sessionId}/context/compact`

**Summary:** ADK session and run routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/skills`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/skills`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/skills/{skillId}`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `PUT /api/v1/adk/skills/{skillId}`

**Summary:** ADK chat, approval, and skill routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/tasks`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/adk/tasks`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `DELETE /api/v1/adk/tasks/{taskId}`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/tasks/{taskId}`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `PUT /api/v1/adk/tasks/{taskId}`

**Summary:** ADK task and memory routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/adk/tools`

**Summary:** ADK catalog and provider management routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

## auth

### `POST /api/v1/auth/login`

**Summary:** 管理员登录

使用单管理员密钥签发 HttpOnly、SameSite=Strict 会话。

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | servercore.adminLoginRequest | 管理员密钥 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |
| `401` | servercore.envelope | Unauthorized |
| `429` | servercore.envelope | Too Many Requests |

### `POST /api/v1/auth/logout`

**Summary:** 管理员注销

注销当前管理员会话。

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/auth/session`

**Summary:** 读取管理员会话

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/auth/token`

**Summary:** 旧令牌入口（已退役）

始终返回 410 Gone；CLI 请直接使用管理员密钥作为 Bearer token。

| Response | Schema | Description |
| --- | --- | --- |
| `410` | servercore.envelope | Gone |

## backtest

### `GET /api/v1/backtests`

**Summary:** 读取回测列表

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `POST /api/v1/backtests`

**Summary:** 启动回测

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `DELETE /api/v1/backtests/{runId}`

**Summary:** 删除回测记录

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `runId` | path | yes | string | 回测运行 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/backtests/{runId}`

**Summary:** 读取回测结果

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `runId` | path | yes | string | 回测运行 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/backtests/{runId}/status`

**Summary:** 读取回测状态

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `runId` | path | yes | string | 回测运行 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/backtests/sync`

**Summary:** 启动历史数据同步

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `DELETE /api/v1/backtests/sync/{taskId}`

**Summary:** Backtest historical data sync task routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/backtests/sync/{taskId}`

**Summary:** Backtest historical data sync task routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

## broker

### `GET /api/v1/brokers/{brokerId}/cash-flows`

**Summary:** 读取券商资金流水

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `clearingDate` | query | yes | string | 清算日期 |
| `direction` | query | no | string | 方向 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/fills`

**Summary:** 读取券商成交

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `scope` | query | no | string | CURRENT 或 HISTORY |
| `symbol` | query | no | string | 证券代码 |
| `startTime` | query | no | string | 历史查询起始时间 |
| `endTime` | query | no | string | 历史查询结束时间 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/funds`

**Summary:** 读取券商资金

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/klines`

**Summary:** 读取券商 K 线

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `symbol` | query | yes | string | 证券代码 |
| `period` | query | no | string | K 线周期，默认 1d |
| `fromTime` | query | no | string | 起始时间 |
| `toTime` | query | no | string | 结束时间 |
| `limit` | query | no | integer | 返回条数 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/margin-ratios`

**Summary:** 读取券商融资融券比例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `symbol` | query | yes | string[] | 证券代码 |
| `symbols` | query | no | string[] | 证券代码列表 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/max-trade-qtys`

**Summary:** 读取券商最大可交易数量

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `symbol` | query | yes | string | 证券代码 |
| `orderType` | query | yes | string | 订单类型 |
| `price` | query | yes | number | 价格 |
| `orderIdEx` | query | no | string | 订单扩展 ID |
| `adjustSideAndLimit` | query | no | number | 调整系数 |
| `session` | query | no | string | 交易时段 |
| `positionId` | query | no | integer | 持仓 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/order-fees`

**Summary:** 读取券商订单费用

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `orderIdEx` | query | yes | string[] | 外部订单号 |
| `orderIdExList` | query | no | string[] | 外部订单号列表 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/orders`

**Summary:** 读取券商订单

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `scope` | query | no | string | CURRENT 或 HISTORY |
| `symbol` | query | no | string | 证券代码 |
| `startTime` | query | no | string | 历史查询起始时间 |
| `endTime` | query | no | string | 历史查询结束时间 |
| `status` | query | no | string[] | 订单状态 |
| `statuses` | query | no | string[] | 订单状态，逗号分隔或重复参数 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/positions`

**Summary:** 读取券商持仓

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/quote`

**Summary:** 读取券商行情

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `symbol` | query | yes | string[] | 证券代码 |
| `symbols` | query | no | string[] | 证券代码列表 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

### `GET /api/v1/brokers/{brokerId}/securities`

**Summary:** 读取券商证券快照

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | 券商 ID |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场代码 |
| `symbol` | query | yes | string[] | 证券代码 |
| `symbols` | query | no | string[] | 证券代码列表 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
| `400` | servercore.envelope | Bad Request |

## execution

### `GET /api/v1/execution/orders`

**Summary:** 读取执行订单

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `scope` | query | no | string | ACTIVE 表示仅活动订单 |
| `brokerId` | query | no | string | Broker 标识 |
| `tradingEnvironment` | query | no | string | 交易环境 |
| `accountId` | query | no | string | 账户 ID |
| `market` | query | no | string | 市场 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` |  | OK |
| `400` | servercore.envelope | Bad Request |

### `POST /api/v1/execution/orders`

**Summary:** 提交执行订单

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | servercore.executionPlaceOrderRequest | 执行订单 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` |  | OK |
| `400` | servercore.envelope | Bad Request |
| `409` | servercore.envelope | Conflict |
| `500` | servercore.envelope | Internal Server Error |

### `POST /api/v1/execution/orders/{internalOrderId}/cancel`

**Summary:** 取消执行订单

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `internalOrderId` | path | yes | string | 内部订单 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` |  | OK |
| `400` | servercore.envelope | Bad Request |
| `404` | servercore.envelope | Not Found |

### `GET /api/v1/execution/orders/{internalOrderId}/events`

**Summary:** 读取执行订单事件

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `internalOrderId` | path | yes | string | 内部订单 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` |  | OK |
| `400` | servercore.envelope | Bad Request |

### `POST /api/v1/execution/orders/preview`

**Summary:** 预览执行订单但不提交

规范化并校验订单请求，返回预览结果，不会向券商提交订单。

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

## market-data

### `GET /api/v1/market-data/candles/{market}/{symbol}`

**Summary:** 查询 K 线

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `market` | path | yes | string | 市场 |
| `symbol` | path | yes | string | 标的 |
| `period` | query | no | string | 周期 |
| `limit` | query | no | integer | 数量 |
| `fromTime` | query | no | string | 起始时间 |
| `toTime` | query | no | string | 结束时间 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `GET /api/v1/market-data/depth/{market}/{symbol}`

**Summary:** 读取盘口深度

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `market` | path | yes | string | 市场代码 |
| `symbol` | path | yes | string | 证券代码 |
| `num` | query | no | integer | 档数，默认 10，最大 50 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `502` | httpserver.Envelope | Bad Gateway |

### `GET /api/v1/market-data/instruments`

**Summary:** 搜索行情标的

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `query` | query | no | string | 搜索关键字 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `POST /api/v1/market-data/instruments/normalize`

**Summary:** Market data utility routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/market-data/markets`

**Summary:** Market data utility routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/market-data/securities/{market}/{symbol}`

**Summary:** 查询证券详情

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `market` | path | yes | string | 市场 |
| `symbol` | path | yes | string | 标的 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `GET /api/v1/market-data/snapshots/{market}/{symbol}`

**Summary:** 读取行情快照

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `market` | path | yes | string | 市场代码 |
| `symbol` | path | yes | string | 证券代码 |
| `refresh` | query | no | boolean | 是否绕过缓存强制刷新 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `502` | httpserver.Envelope | Bad Gateway |

### `DELETE /api/v1/market-data/subscriptions`

**Summary:** 清空行情订阅

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `consumerId` | query | no | string | 消费者 ID；为空时清空全部 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/market-data/subscriptions`

**Summary:** Market data utility routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/market-data/subscriptions`

**Summary:** 申请行情订阅

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | marketdata.SubscriptionRequest | 订阅请求 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `POST /api/v1/market-data/subscriptions/heartbeat`

**Summary:** 刷新订阅心跳

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | marketdata.SubscriptionHeartbeatRequest | 心跳请求 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `POST /api/v1/market-data/subscriptions/release`

**Summary:** 释放行情订阅

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | marketdata.SubscriptionRequest | 释放请求 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/ws/live`

**Summary:** 实时行情 WebSocket

| Response | Schema | Description |
| --- | --- | --- |
| `101` | string | Switching Protocols |

## plugins

### `GET /api/v1/plugins`

**Summary:** Plugin catalog and operation routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/plugins/{pluginId}/install`

**Summary:** Plugin catalog and operation routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `POST /api/v1/plugins/{pluginId}/uninstall`

**Summary:** Plugin catalog and operation routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/plugins/{pluginId}/uninstall-guidance`

**Summary:** Plugin catalog and operation routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/plugins/operations/{operationId}`

**Summary:** Plugin catalog and operation routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

## portfolio

### `GET /api/v1/portfolio/{brokerId}/cash-balances`

**Summary:** Portfolio routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/portfolio/{brokerId}/cash-reconciliation`

**Summary:** Portfolio routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/portfolio/{brokerId}/positions`

**Summary:** Portfolio routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/portfolio/{brokerId}/reconciliation`

**Summary:** Portfolio routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

## settings

### `GET /api/v1/settings/adk`

**Summary:** 读取 ADK 运行时设置

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/adk`

**Summary:** 保存 ADK 运行时设置

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | jftsettings.ADKRuntimeSettings | ADK 运行时设置 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `POST /api/v1/settings/broker-accounts`

**Summary:** 创建托管账户

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | settings.ManagedBrokerAccountWriteRequest | 托管账户 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `DELETE /api/v1/settings/broker-accounts/{accountRecordId}`

**Summary:** 删除托管账户

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `accountRecordId` | path | yes | string | 托管账户记录 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `PUT /api/v1/settings/broker-accounts/{accountRecordId}`

**Summary:** 更新托管账户

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `accountRecordId` | path | yes | string | 托管账户记录 ID |
| `request` | body | yes | settings.ManagedBrokerAccountWriteRequest | 托管账户 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/settings/brokers`

**Summary:** 读取 broker 设置

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/brokers/{brokerId}/integration`

**Summary:** 保存 broker 集成

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `brokerId` | path | yes | string | Broker 标识 |
| `request` | body | yes | settings.BrokerIntegrationSaveRequest | Broker 集成配置 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/settings/execution`

**Summary:** 读取执行设置

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/execution`

**Summary:** 保存执行设置

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | jftsettings.ExecutionSettings | 执行设置 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/settings/onboarding`

**Summary:** 读取新手引导状态

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/onboarding`

**Summary:** 保存新手引导状态

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | settings.OnboardingWriteRequest | 引导状态 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/settings/security`

**Summary:** 读取安全设置

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/security`

**Summary:** 保存安全设置

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | jftsettings.SecuritySettings | 安全设置 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `GET /api/v1/settings/ui`

**Summary:** 读取 UI 颜色配置

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `PUT /api/v1/settings/ui`

**Summary:** 保存 UI 颜色配置

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | settings.UIAppearanceSettingsWriteRequest | UI 配置 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

## strategy

### `GET /api/v1/strategies`

**Summary:** 读取策略实例列表

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `DELETE /api/v1/strategies/{instanceId}`

**Summary:** 删除策略实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `PUT /api/v1/strategies/{instanceId}`

**Summary:** 更新策略实例绑定参数

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |
| `request` | body | yes | strategy.StrategyBindingRequest | 实例绑定参数 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/strategies/{instanceId}/audit`

**Summary:** 读取策略审计记录

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |
| `limit` | query | no | integer | 分页大小 |
| `offset` | query | no | integer | 分页偏移 |
| `kind` | query | no | string | 审计类型 |
| `fromTime` | query | no | string | 起始时间 |
| `toTime` | query | no | string | 结束时间 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/strategies/{instanceId}/logs`

**Summary:** 读取策略运行日志

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |
| `limit` | query | no | integer | 分页大小 |
| `offset` | query | no | integer | 分页偏移 |
| `level` | query | no | string | 日志级别 |
| `fromTime` | query | no | string | 起始时间 |
| `toTime` | query | no | string | 结束时间 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategies/{instanceId}/pause`

**Summary:** 暂停策略实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategies/{instanceId}/refresh-definition`

**Summary:** 刷新实例关联的策略定义

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategies/{instanceId}/start`

**Summary:** 启动策略实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |
| `502` | httpserver.Envelope | Bad Gateway |

### `POST /api/v1/strategies/{instanceId}/stop`

**Summary:** 停止策略实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `instanceId` | path | yes | string | 策略实例 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/strategy-definitions`

**Summary:** 读取策略定义列表

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `POST /api/v1/strategy-definitions`

**Summary:** 创建策略定义

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | strategy.StrategyDesignDefinition | 策略定义 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

### `DELETE /api/v1/strategy-definitions/{definitionId}`

**Summary:** 删除策略定义

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `definitionId` | path | yes | string | 策略定义 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `GET /api/v1/strategy-definitions/{definitionId}`

**Summary:** 读取策略定义

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `definitionId` | path | yes | string | 策略定义 ID |
| `interval` | query | no | string | 预览周期 |
| `symbol` | query | no | string | 预览标的 |
| `useExtendedHours` | query | no | boolean | 是否包含盘前盘后 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `PUT /api/v1/strategy-definitions/{definitionId}`

**Summary:** 更新策略定义

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `definitionId` | path | yes | string | 策略定义 ID |
| `request` | body | yes | strategy.StrategyDesignDefinition | 策略定义 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategy-definitions/{definitionId}/apply-linked-instances`

**Summary:** 应用策略定义到关联实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `definitionId` | path | yes | string | 策略定义 ID |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategy-definitions/{definitionId}/instantiate`

**Summary:** 从策略定义创建实例

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `definitionId` | path | yes | string | 策略定义 ID |
| `request` | body | no | strategy.StrategyBindingRequest | 实例绑定参数 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |
| `404` | httpserver.Envelope | Not Found |

### `POST /api/v1/strategy-pine/analyze`

**Summary:** 分析 Pine 脚本

| Name | In | Required | Type | Description |
| --- | --- | --- | --- | --- |
| `request` | body | yes | strategy.AnalyzePineRequest | Pine 脚本 |

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |
| `400` | httpserver.Envelope | Bad Request |

## system

### `GET /api/v1/system/futu-opend`

**Summary:** OpenD 健康检查

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `GET /api/v1/system/futu-opend/install-guide`

**Summary:** 读取 OpenD 安装指南

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `POST /api/v1/system/futu-opend/manual-retry`

**Summary:** 手动重置 OpenD 运行时

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `GET /api/v1/system/real-trade-approvals`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-hard-stop-events`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-hard-stops`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-kill-switch`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-kill-switch-events`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-risk-events`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/real-trade-risk-limits`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/status`

**Summary:** 读取系统状态

返回 API、broker 与实时流状态摘要。

| Response | Schema | Description |
| --- | --- | --- |
| `200` | httpserver.Envelope | OK |

### `GET /api/v1/system/storage/overview`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |

### `GET /api/v1/system/worker/broker-order-updates`

**Summary:** System operational routes

| Response | Schema | Description |
| --- | --- | --- |
| `200` | servercore.envelope | OK |
