# Web 访问禁用提示页批次（第 113 批）

本文件记录 Go 基线 `internal/app/apiserver/webaccess/security_integration_test.go:188` 迁移到 Rust 的
逐条核对结论。与第 112 批同类，这是**功能性补齐**：Rust 的授权层一直只返回 JSON 错误，浏览器直接
导航到常驻桌面 API 端口时得到空 404，缺少 Go 的友好禁用页。涉及
`crates/jftrade-api/src/browser_access.rs`（新增状态类型与禁用页）、
`crates/jftrade-api/src/router.rs`（导航分流）、
`crates/jftrade-api/tests/browser_access_contracts.rs`（本批新增 3 条测试）、
`crates/jftrade-engine/src/product_server_runtime.rs`（按设置上报状态）、
`crates/jftrade-engine/src/product_server.rs`（组装注入）与
`crates/jftrade-engine/src/product_server_tests.rs`（本批新增 1 条测试）。

## 第一百一十三批：webaccess 禁用页收口（1 条）

### 范围与分片

Go 侧 1 条 `missing`：

- P1 `security_integration_test.go:188`：`TestBrowserNavigationGetsFriendlyDisabledWebPage`
  （sidecar 监听器，Web 访问未开启，`GET /` + `Accept: text/html` 期望 403 友好页）。

分类：1 条 `[x]`/`function_exact`；`internal/app/apiserver/webaccess` 域内 `missing` 归零，
全仓 `missing` 4 → 3（余 `cmd/jftrade-desktop`、`pkg/chart`、`scripts/archive_frontend_assets_test.go`）。

### 关键事实（本批 recon 与实测）

- Go 行为（`server_auth.go` 的 `webAccessMiddleware` + `acceptsWebAccessStatusPage` +
  `writeWebAccessStatusPage`）：一次中间件覆盖所有请求。当 `WebAccessEnabled()` 为假时，
  `GET`/`HEAD`、`Accept` 含 `text/html`、路径不以 `/api/` 或 `/swagger` 开头的请求得到
  403 + `text/html; charset=utf-8` + `Cache-Control: no-store` +
  `X-Content-Type-Options: nosniff`，正文含「Web 访问尚未开启」与「设置 → Web 访问」；
  其余请求继续走 JSON 信封（`WEB_ACCESS_DISABLED` / `REMOTE_WEB_ACCESS_DISABLED`）。
- Rust 迁移前状态：`crates/jftrade-api/src/router.rs` 只有两类分流——`/api/**`、`/swagger**`
  走授权与路由表，其他路径交给静态响应（生产未注入前端资产，因此空 404）。授权层只有
  `WEB_AUTH_REQUIRED`、`ORIGIN_FORBIDDEN`、`CSRF_FAILED` 等 JSON 失败，`WEB_ACCESS_DISABLED`
  仅出现在 auth-session 写入端口（登录/登出路径）。因此浏览器导航始终是裸 404。
- 本批实现：新增 `browser_access.rs`（`WebAccessState { Disabled, Available }`、
  `WebAccessStatePort`、与 Go 同文案同响应头的禁用页）；`transport_middleware` 在授权分流前
  处理导航请求；状态来自 engine：`ProductWebServerRuntime` 在 `SecurityRuntimePort::apply`
  里记录 `web_access_enabled`（设置可在运行期切换，所以用 port 而不是启动常量），
  `product_server.rs` 用 `ApiState::with_browser_access` 注入，桌面监听器与 Web 监听器共用。
- 排除面与 Go 一致：`/api/**` 与 `/swagger/**` 永不渲染该页；`Accept` 不含 `text/html`、
  非 `GET`/`HEAD`、桌面令牌受信调用者、已登录 Web 会话都不渲染该页（只有“未认证的浏览器导航”
  才看到提示）。
- 契约面未变：`contracts/openapi/openapi.json`、路由清单与生成物逐字未改
  （`pnpm run check:generated` 通过且未改写工作树）。

### 逐条结论

- `security_integration_test.go:188` →
  `crates/jftrade-api/tests/browser_access_contracts.rs::browser_navigation_gets_the_friendly_disabled_page_while_web_access_is_off`：
  `GET /` + `Accept: text/html,application/xhtml+xml` 返回 403、`text/html; charset=utf-8`、
  `no-store`、`nosniff`，正文同时含「Web 访问尚未开启」与「设置 → Web 访问」（与 Go 断言逐条对应）；
  组装侧状态由 `crates/jftrade-engine/src/product_server_tests.rs::web_runtime_reports_browser_access_state_for_the_desktop_listener`
  证明（默认 Disabled，应用开启设置后 Available，再关闭回到 Disabled）。
- 同文件附带边界：`::disabled_page_covers_head_navigations_and_leaves_other_surfaces_alone`
  断言 HEAD 导航 403 且无正文、`/api/v1/system/status` 仍为 401 `WEB_AUTH_REQUIRED`、
  `/swagger/**` 未认证仍为 401、`Accept: application/json`、带桌面令牌、`POST` 均不渲染该页；
  `::enabled_web_access_never_renders_the_disabled_page` 断言 Web 已开启时不渲染该页。

### 探针记录（破 → 红 → 按字节回滚）

- 删除 `transport_middleware` 的 `web_access_status_page` 分支：主测试转红（403 → 404），
  即证明此前确为裸 404，提示页是本批新增行为。
- 去掉 `accepts_web_access_status_page` 的 `/api/` 排除：边界测试转红（API 面 403 HTML ≠ 期望 401 JSON）。
- 去掉 `desktop_trusted` 豁免：受信桌面导航被渲染成提示页（404 → 403），边界测试转红。
- 删除 `ProductWebServerRuntime::apply` 中的 `web_access_enabled` 记录：
  `web_runtime_reports_browser_access_state_for_the_desktop_listener` 转红
  （`left: Disabled` vs `right: Available`）。

四次探针均按字节回滚，回滚后 shasum 与探测前一致：

- `crates/jftrade-api/src/browser_access.rs`：`97e80c74548e3a884afa5637fb808b6e60d12a30964b5a5fe5faffb5af6b4a69`
- `crates/jftrade-api/src/router.rs`：`abcf5ab2cfe8c404a621503f239506f5dd24fa548395be0d36dd3b203352fc8a`
- `crates/jftrade-api/src/lib.rs`：`6351b3a6fcc10fc1ca313ab834f899dd43eb3cf9161bbaf3ed080a6d80bbce7f`
- `crates/jftrade-api/tests/browser_access_contracts.rs`：`4d3334cf39bc019615fae6ffb53fe851171ad360927153e5858682b6ddf5a880`
- `crates/jftrade-engine/src/product_server_runtime.rs`：`249003efc498428d2a6aafe65d665afe6decf3ba41720f9f1fb5de31fcfaf73c`
- `crates/jftrade-engine/src/product_server.rs`：`dee0f01efc50c5c5e7f104e0d23c5862b85075653f344906dc212ffbb1e1c44d`
- `crates/jftrade-engine/src/product_server_tests.rs`：`334d230e201143dc6e81404141ff355af65cdc3263d928ca321046c3013fbe3c`
- `crates/jftrade-engine/src/product.rs`：`9a5c21bfe092125717a56fd007e0f6bd72741c54948d64266a7c84625953f826`

### 保留差异

- **「当前仅允许本机访问」页在 Rust 没有对应物**：Go 在公开访问关闭时对远端浏览器返回该页
  （403 + HTML）；Rust 的 Web 监听器此时只绑 `127.0.0.1`，远端浏览器连不上，比“403 提示页”
  更严格。Rust 的禁用页只覆盖“Web 未开启”这一态。
- Web 关闭时 Rust 不创建 Web 监听器（与 `docs/configuration.md` 既有说明一致），禁用页由常驻
  桌面监听器承担；Go 只有单监听器，两种态在同一个端口上区分。
- 非生产（test cutover）配置不创建 `ProductWebServerRuntime`，因此不注入该状态 port，
  导航请求保持既有静态/404 行为；该形态只用于本地测试与靶场。
- Go 的 JSON 分支文案是 `WEB_ACCESS_DISABLED`；Rust 未认证 API 请求沿用既有
  `401 WEB_AUTH_REQUIRED`（该项在 `security_integration_test.go:175` 已登记为 partial 基线），
  本批未改变 API 错误码，只补齐 HTML 导航面。

### 后续待办

- 下一批：`cmd/jftrade-desktop/desktop_updates_test.go:9`
  （`TestDesktopUpdateServiceSelectsLatestStableDesktopRelease`，`missing` 3 → 2），核对
  GitHub release 列表的 stable/prerelease/draft 过滤与版本选择。
- 再下一批：`pkg/chart/chart_type_test.go:5`、`scripts/archive_frontend_assets_test.go:11`。
- 本批未处理的既有独立项：审计脚本生成的 markdown 含尖括号占位符，`build:docs:generated`
  在 markdown-it/Vue 判为未闭合 HTML 标签处失败（与本批无关）。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-api -p jftrade-engine
  --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -p jftrade-engine --all-targets
  --locked --no-fail-fast`：1834 passed / 0 skipped / 0 failed（jftrade-api 74 含本批 3 条新增；
  jftrade-engine 1760 含本批 1 条新增）。
- 四次探针各自转红并已按字节回滚（删除提示分支、去掉 `/api/` 排除、去掉桌面令牌豁免、
  不记录开启状态），回滚后 shasum 与探测前一致（见上文）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3064 Rust、`[x]`
  1326 → 1327、`missing` 4 → 3、0 破坏引用、0 条 `[x]` 缺少 function_exact；未锚定 function_exact
  保持 202（既有基线）、partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1328 唯一引用（已记账 1272 →
  1273；unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0；
  `pnpm run check:generated` EXIT=0 且未改写工作树；`node scripts/check-zero-go.mjs` EXIT=0
  （2924 tracked files）；`pnpm run check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` 首跑 EXIT=1，失败阶段 `check:rust:target-health`
  （`target/debug/deps` 的 `.rcgu.o` ≥ 50000）。确认无 cargo/rustc 进程后执行
  `pnpm run clean:rust:artifacts`（移除 127059 文件 / 32.4 GiB）再跑得到 **EXIT=0**。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`）报
  RUSTSEC-2026-0285 与陈旧 advisory 告警，与本仓既有基线同源。**不记为通过**；该 run 在静态
  阶段停止，等价测试面由上面的 nextest 结果覆盖。
