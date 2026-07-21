# 链路观测与排障路径

JFTrade 使用一组固定关联字段串联 HTTP、OpenD、行情、回测、PineTS 与 ADK 日志。设置页的“开发者工具”只保留有限数量的进程内摘要，完整事实仍以结构化日志和运行记录为准。

## 关联字段

| 字段 | 作用 |
| --- | --- |
| `request_id` | 单次 HTTP 请求，由服务端生成或接收 `X-Request-ID` |
| `session_id` | ADK 会话 |
| `run_id` | ADK、回测或策略运行 |
| `task_id` | K 线同步、优化或 worker 任务 |
| `broker_id` / `account_id` | 券商与账户 |
| `instrument_id` | 规范化标的，例如 `HK.00700` |
| `provider_id` | ADK provider |
| `source` | `api`、`opend`、`market-data`、`backtest`、`pinets`、`adk` 等来源 |
| `importance` | 日志重要性，取值 `low`、`normal`、`high`、`critical` |

## 从开发者工具到运行记录

1. 打开 `/settings/developer-tools`，在“链路观测”中确认记录阈值、最近错误、慢请求和 OpenD 调用健康。
2. 记录事件上的 `request_id`，以及存在时的 `run_id`、`task_id`、`session_id`。
3. 使用关联字段检索 API 进程日志。标准 `slog` 文本输出可直接搜索：

   ```bash
   rg 'request_id=REQUEST_ID|run_id=RUN_ID|task_id=TASK_ID' var logs
   ```

   如果日志由 launchd、systemd 或容器平台接管，在对应日志查询器中使用同名字段过滤。
4. 对 `bt-*` 或 `sync-*` 事件，从开发者工具进入 `/backtest`，核对运行状态、同步进度和结果错误。
5. 对 `run-*` / `session-*` 事件，从开发者工具进入 `/adk/agents`，核对 timeline、tool call、approval 和 provider；需要持久化事实时查询 `var/jftrade-api/adk.db` 与 `var/jftrade-api/adk-session.db`。
6. OpenD 事件先看 `lastOperation`、失败计数和 `request_id`，再到 `/settings/futu-integration` 核对连接健康；订阅和退避细节使用相同关联字段检索结构化日志。

## 摘要边界

- 最近错误与慢请求各保留 20 条，进程重启后清空。
- 默认慢请求阈值为 750ms。
- 默认最低记录重要性为 `low`。API server 启动时可通过 `JFTRADE_OBSERVABILITY_MIN_IMPORTANCE=normal|high|critical` 抑制低重要性日志写入。
- 摘要不记录请求体、查询参数、认证头、API key、模型消息或交易密码。
- `X-Request-ID` 只接受长度不超过 128 的字母、数字及 `- _ . :`；非法值会由服务端重新生成。
- 开发者工具摘要用于定位关联 ID，不替代数据库、任务状态或完整日志。
