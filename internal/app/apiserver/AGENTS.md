# API 装配局部指令

- `servercore` 只做 composition root、HTTP/frontend shell、生命周期和窄 adapter；业务规则放进 `internal/*` service。
- `internal/api/*` 只做 transport；不要把 store、SQLite、Futu、protobuf 或后台任务带入 handler。
- 启动资源必须先登记关闭函数再发布；失败按逆序回滚，Close 可重复/并发调用。
- 优先使用现有 `application`、`stores`、`runtimes`、`marketdataapp` 句柄；浏览器资产/访问归 `webaccess`，实时 transport 装配归 `liveapp`，交易后台 worker 装配归 `tradingapp`，不在 `servercore` 平铺新字段。
- 最小验证：`go test ./internal/app/apiserver/... -count=1`、`pnpm run check:arch-deps`。
