# Goroutine 生命周期审计

更新时间：2026-07-29。

> 归档审计：本文记录的是 2026-07-29 的 Go/Wails 桌面基线。表中
> `cmd/jftrade-desktop` 行号对应迁移前源码，当前生产桌面入口已切换为
> `apps/desktop/src-tauri`（Tauri）；这些行保留作历史证据，不是可执行的当前路径。

本文记录 P3-1 的代码级审计账本。基线固定为审计开始时的 57 个非测试直接 `go` 语句、2 个 `WaitGroup.Go` 和 2 个 `time.AfterFunc`，合计 61 个显式异步启动面；其中 `internal/integration/futu/testkit` 的 4 个启动面只服务测试。行号用于定位审计基线，长期判断以函数和 owner 为准。

风险口径：

- **低**：退出条件、资源 owner 和关闭顺序完整，或工作天然有界且不会在 owner 关闭后访问下游资源。
- **中**：当前调用有界，但仍依赖调用者 drain、provider 遵守 context、单次连接或回调不可重入等契约；违反契约可能遗留 goroutine。
- **高**：Close 后仍可启动、取消后继续写资源、存在 `Add/Wait` 竞态，或关闭无法可靠 join。本轮确认的高风险均已修复。
- **已修复**：本轮补了 admission、cancel、解除阻塞和 join，并有确定性 lifecycle/race 测试。

## 61 个基线启动面

### Desktop、API 与应用生命周期

| # | 基线启动点 | owner、退出与 join | 结论 |
|---:|---|---|---|
| 1 | `cmd/jftrade-desktop/desktop_updates.go:133` 周期更新检查 | desktop context 退出 ticker；单次 HTTP 检查有客户端超时，但 desktop Close 不 join | 中：网络调用有界，后续可纳入 desktop task group |
| 2 | `cmd/jftrade-desktop/desktop_updates.go:163` 手动更新检查 | 一次性 HTTP 检查；无应用级 join，完成后可能回调 window | 中：有界但依赖 window 生命周期 |
| 3 | `cmd/jftrade-desktop/main.go:245` 信号退出 | 等待根 context，随后只调用幂等 `state.quit` | 低 |
| 4 | `internal/api/assistant/chat_stream_hub.go:203` detached chat | 改由 Assistant transport 的 application context、admission mutex 和 WG 管理；Close 先 cancel、再 join，且先于 runtime 关闭 | 已修复 |
| 5 | `internal/api/live/handler.go:156` WS cancel bridge | request 或 handler context 任一结束即退出；Handler Close 关闭实际连接并等待 active handler | 低 |
| 6 | `internal/api/live/handler.go:283` WS read loop | socket Close 解除 `ReadMessage`；active handler 等 dispatcher 和 read closed 后退出 | 低 |
| 7 | `internal/app/apiserver/lifecycle/lifecycle.go:140` 根 context shutdown | 根 context 触发带 5 秒上限的统一 shutdown；资源关闭函数幂等 | 低 |
| 8 | `internal/app/apiserver/lifecycle/lifecycle.go:324` API `Serve` | listener/server 由 lifecycle resource 持有，`Shutdown` 解除 Serve 并等待 handler | 低 |
| 9 | `internal/app/apiserver/lifecycle/lifecycle.go:347` integrated `Serve` | 同上 | 低 |
| 10 | `internal/app/apiserver/lifecycle/lifecycle.go:445` optional Web `Serve` | manager 串行切换 listener，Close/Shutdown 解除 Serve | 低 |
| 11 | `internal/app/apiserver/servercore/adk_workflow.go:28` workflow event | servercore 不再裸启动；Service 自身非阻塞接纳并由 workflow context/WG 收割 | 已修复 |
| 12 | `internal/app/apiserver/servercore/notifications.go:105` notification bridge | 一次性连接，15 秒超时；没有纳入 server resource join | 中：有界，但仍依赖 marketdata runtime 关闭顺序 |
| 13 | `internal/app/apiserver/servercore/server_auth.go:37` auth context bridge | access 或 request context 任一结束即退出，只调用 request cancel | 低 |
| 14 | `internal/assistant/assembly/mcp_server.go:100` MCP `Serve` | server Close 解除 Accept；新增 manager `serveWG`，Close 在释放 manager 锁后 join 所有历代 Serve | 已修复 |

### Assistant engine 与 workflow

| # | 基线启动点 | owner、退出与 join | 结论 |
|---:|---|---|---|
| 15 | `internal/assistant/engine/google_exec.go:50` ADK runner wrapper | 外层可按 deadline 返回，buffered result 不会阻塞发送；若底层 ADK runner 不响应 context，inner runner 仍可能存活 | 中：第三方 runner 协作退出契约 |
| 16 | `internal/assistant/engine/runner.go:321` runtime background | `closing` 与 `WG.Add` 共用 admission lock；Runtime Close cancel 后 Wait | 低 |
| 17 | `internal/assistant/engine/runner_approval.go:147` approval continuation | continuation claim 去重；admission、runtime context、WG 和 Close 顺序完整 | 低 |
| 18 | `internal/assistant/engine/runner_input.go:84` input continuation | 与 approval 相同，并在退出时释放 durable/in-process claim | 低 |
| 19 | `internal/assistant/engine/runtime_execution_lease.go:221` tool invocation heartbeat | 调用者持有 `stop`；stop cancel 并同步读取 `done`，心跳 I/O 另有 5 秒上限 | 低 |
| 20 | `internal/assistant/engine/tools.go:430` registered tool wrapper | 删除超时返回后可能继续副作用的 detached goroutine；ToolFunc 同步执行并要求遵守 context，panic 仍映射为 error | 已修复 |
| 21 | `internal/assistant/workflow_schedule.go:65` schedule invocation | 统一走 Service workflow admission/context/WG | 已修复 |
| 22 | `internal/assistant/workflow_schedule.go:110` market threshold invocation | 同上 | 已修复 |
| 23 | `internal/assistant/workflows.go:471` matched event invocation | 同上 | 已修复 |
| 24 | `internal/assistant/workflows.go:496` generic event invocation | 同上 | 已修复 |
| 25 | `internal/assistant/workflows.go:544` queued manual/API run | 排队前预留 WG；关闭后拒绝；goroutine defer release，Close 等待后才关 runtime/store | 已修复 |

### 业务 service、store 与测试支撑

| # | 基线启动点 | owner、退出与 join | 结论 |
|---:|---|---|---|
| 26 | `internal/backtest/run.go:102` backtest run | `beginTask` 在 lifecycle lock 下 Add；Close 拒绝新任务、cancel、Wait | 低 |
| 27 | `internal/backtest/sync.go:32` K 线同步 | 同一 Service task group；worker defer 关闭 syncer、cancel、Done | 低 |
| 28 | `internal/exchangecalendar/manager.go:88` manager loop | `startOnce`、`stopOnce`、stop channel 和 manager WG | 低 |
| 29 | `internal/exchangecalendar/manager_refresh.go:50` refresh stop bridge | refresh 同步运行在 manager loop 内；bridge 由 stop、timeout 任一解除，manager WG 间接覆盖 refresh | 低 |
| 30 | `internal/integration/futu/testkit/broker_server.go:78` accept loop | test server Close 关闭 listener，并由 testkit WG 收割 | 低，仅测试支撑 |
| 31 | `internal/integration/futu/testkit/broker_server.go:204` connection handler | 连接由 test server 记录并在 Close 关闭，WG 收割 | 低，仅测试支撑 |
| 32 | `internal/integration/futu/testkit/quote_server.go:63` accept loop | 同 testkit broker server | 低，仅测试支撑 |
| 33 | `internal/integration/futu/testkit/quote_server.go:102` connection handler | 同 testkit broker server | 低，仅测试支撑 |
| 34 | `internal/marketdata/collector.go:148` collector loop | constructor 先 Add；Close 标记 closed、增 generation、cancel/close stream、Wait | 低 |
| 35 | `internal/marketdata/collector.go:366` stream connect | `closed/generation` admission 与 WG.Add 同锁；Close 先解除阻塞 stream 再 Wait | 低 |
| 36 | `internal/marketdata/collector.go:407` fallback polling | 查询 context 有 timeout；generation 防迟到结果提交；Close Wait | 低 |
| 37 | `internal/marketdata/instrument_resolver.go:284` multi-market lookup | response channel 按 fan-out 数量缓冲，caller 取消后 send 不阻塞 | 中：provider 必须遵守 context，否则 provider 调用自身可滞留 |
| 38 | `internal/store/sqliteconn/coordinator.go:84` idle cleanup | 只等待当前 write tail；所有 ticket 都必须 finish，清理完成或重新被引用即退出 | 低 |
| 39 | `internal/store/sqliteconn/coordinator.go:173` cancelled ticket cleanup | caller 取消后等待 predecessor，再 `finish` 自身 ticket，避免阻断后续写 | 低 |
| 40 | `internal/store/trading/ledger.go:594` persistence worker | queue 是 owner；Close 在锁下拒绝并关闭 queue，然后 WG.Wait 后关闭 DB | 低 |
| 41 | `internal/strategy/liveruntime/manager.go:496` closed-K-line sync | 每个 managed runtime 统一 Add；Close cancel 后 Wait，再关 Pine session 和订阅 | 已修复 |
| 42 | `internal/strategy/pineruntime/runner.go:139` session context watcher | session 新增 done；手动/runner Close 都解除 watcher；注册与 WG.Add 在 runner lock 下，Runner Close join | 已修复 |
| 43 | `pkg/backtest/internal/storage/store_query.go:141` K 线结果 channel | worker 以关闭两个输出 channel 结束，但接口没有 context | 中：调用者必须持续 drain，放弃未消费 channel 会阻塞发送 |
| 44 | `pkg/backtest/session_filter_store.go:186` session filter channel | 同上，并依赖下游 base channel 被完整消费 | 中：调用者 drain 契约 |

### 公共包、Futu 与 Pine worker

| # | 基线启动点 | owner、退出与 join | 结论 |
|---:|---|---|---|
| 45 | `pkg/bbgo/types/connectivitygroup.go:220` state waiter | caller context 是唯一 owner，到达目标或 cancel 退出 | 低 |
| 46 | `pkg/bbgo/types/serial_market_store.go:51` ticker processor | context 可退出，但 store 没有 join；当前产品没有构造该类型 | 中：公共包调用契约，产品路径未使用 |
| 47 | `pkg/bbgo/types/serial_market_store.go:168` async AddKLine | 每根闭合 K 线可启动一次，无 cancel/join；当前产品没有启用该异步路径 | 中：若重新启用必须改为有界 queue/worker |
| 48 | `pkg/bbgo/types/stream.go:445` standard reconnector | caller context 或 `CloseC` 退出；重连过程沿用 context | 低 |
| 49 | `pkg/bbgo/types/syncgroup.go:48` function group | 每次 `Add` 对应 worker Done，调用者以 `Wait` join | 低 |
| 50 | `pkg/futu/opend/client.go:147` TCP read loop | Connect 在锁内 Add；Close 先关 conn 解除 Read，再 Wait | 已修复；订阅回调不得同步反向调用同一 Client.Close |
| 51 | `pkg/futu/opend/client.go:190` keepalive | Close 后禁止启动；Done/请求 timeout 退出并由 Client Close join | 已修复 |
| 52 | `pkg/futu/stream.go:72` reconnect loop | Connect/Close 串行，generation admission 与 WG.Add 同锁；Close cancel、关 client、Wait | 已修复 |
| 53 | `pkg/futu/stream.go:155` quote watcher | 统一走 generation worker owner；client Done 或 stream context 退出 | 已修复 |
| 54 | `pkg/futu/stream_orderbook.go:41` order-book watcher | 同 quote watcher | 已修复 |
| 55 | `pkg/strategy/pineengine/pine_ts_client.go:148` stderr reader | stderr 使用独立锁，避免与 call mutex 反压死锁；进程关闭后由 client WG join | 已修复 |
| 56 | `pkg/strategy/pineengine/pine_ts_client.go:149` process Wait | 每次启动先 Add，Close kill/pipe close 后 Wait；与 stderr reader 一并收割 | 已修复 |
| 57 | `pkg/strategy/pineworker/process_launcher.go:236` process Stop waiter | Stop 在正常退出、超时 kill 或 caller cancel 后都同步读取 `done` | 低 |

### 非直接 `go` 的四个基线启动面

| # | 基线启动点 | owner、退出与 join | 结论 |
|---:|---|---|---|
| 58 | `cmd/jftrade-desktop/desktop_window_state.go:128` debounce timer | 新 schedule 先 Stop 旧 timer；Close 标记 closed、Stop 并同步 flush，迟到 callback 只看到 closed | 低 |
| 59 | `cmd/jftrade-desktop/tray_menu_darwin.go:13` tray timer | 20ms 单次 UI workaround，无资源循环 | 低 |
| 60 | `internal/assistant/engine/runtime_execution_lease.go:86` run lease `WaitGroup.Go` | 改为 closing/Add 共用 admission lock；runtime context 联动 cancel；Close 等心跳释放 durable lease 后再关 store | 已修复 |
| 61 | `internal/assistant/workflow_schedule.go:15` scheduler `WaitGroup.Go` | 改为 Start/Stop mutex 串行 admission；Stop cancel 并 Wait 当前 tick；Service Close 再等待 workflow task group | 已修复 |

## 审计后的当前状态

- 当前源码有 52 个直接 `go`、5 个 `AfterFunc`、0 个 `WaitGroup.Go`，合计 57 个显式异步启动面；扣除 4 个 testkit 启动点后，产品路径为 53 个。
- 数量变化不是验收目标。部分 named launch 被统一包装为匿名 worker，真正的改进是 owner、admission、cancel、解除阻塞和 join 已形成闭环。
- 当前没有未处理的高风险项。本轮修复了 Assistant transport、workflow/scheduler、tool wrapper、run lease、MCP Serve、strategy live/Pine session、Futu client/stream 和旧 PineTS client。
- 仍保留 9 个中风险契约：ADK runner 的 context 协作退出、两处 desktop update、notification bridge、instrument provider fan-out、两处 backtest channel drain，以及未进入产品路径的两处 `SerialMarketDataStore`。Futu Client 另外保留“同步订阅回调不可反向 Close”的重入约束。

## 后续修改规则

新增异步启动必须在同一变更中回答：

1. 谁在关闭后阻止新的 `Add` 或启动；
2. 哪个 context、channel、socket close 或 process kill 解除所有阻塞点；
3. 哪个 owner join，且 join 发生在关闭 DB、session、listener 或下游 runtime 之前；
4. 错误由谁观察，是否会在 caller 已返回后继续提交业务状态；
5. 是否有 barrier 测试覆盖 Close 与启动并发，且通过定向 stress 和 race。

不要用全局 goroutine 数、固定 sleep 后“看起来没增长”，或只给 goroutine 加 buffered result channel来证明生命周期安全。
