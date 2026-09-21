# Rust Swagger 调试文档面批次（第 112 批）

本文件记录 Go 基线 `internal/app/apiserver` 的 4 条 swagger/openapi 测试迁移到 Rust 的逐条
核对结论。与前十批“补测试为主”不同，本批是**功能性补齐**：Rust 此前没有任何 `/swagger` 路由
（请求落到 JSON 404），而仓库文档与前端开发代理早已把该入口当作既有能力，清单也因此长期挂着
1 条 `missing`。涉及 `crates/jftrade-api/src/swagger_docs.rs`（新增文档面响应）、
`crates/jftrade-api/src/router.rs`（`ApiState::with_swagger_docs` 与 dispatch 分流）、
`crates/jftrade-api/tests/swagger_docs_contracts.rs`（本批新增 5 条测试）、
`crates/jftrade-engine/src/product_swagger_docs.rs`（冻结契约注入与 1 条测试）与
`crates/jftrade-engine/src/product_server.rs`（组装注入）。

## 第一百一十二批：swagger 调试文档面收口（4 条）

### 范围与分片

Go 侧参考 `internal/app/apiserver/servercore/openapi.go` 与 `server_auth.go`，测试 4 条：

- P1 `servercoretest/swagger_openapi_test.go:14`：`TestSwaggerUIAvailable`（原 `missing`）。
- P1 `servercoretest/swagger_openapi_test.go:72`：`TestOpenAPISpecExposesCorePaths`（原 `partial`）。
- P2 `servercore/openapi_boundary_test.go:11`：`TestSwaggerRoutesRedirectToBrowsableDocumentation`
  （原 `boundary`）。
- P2 `servercoretest/openapi_schema_compatibility_test.go:14`：`TestOpenAPIPreservesLegacySchemaNames`
  （原 `partial`）。

分类：4 条全部 `[x]`/`function_exact`；`internal/app/apiserver` 域内 `missing` 5 → 4（只剩
`webaccess/security_integration_test.go:188`）。

### 关键事实（本批 recon 与实测）

- Go 行为（`servercore/openapi.go`）：`/swagger` 与 `/swagger/` 返回 307 + `Location:
  /swagger/index.html`；`/swagger/swagger-initializer.js` 返回含 `/swagger/doc.json` 的 JS；
  `/swagger/doc.json` 返回 `docs/swagger`（或 `JFTRADE_OPENAPI_SOURCE` 指定的文件）；其余
  `/swagger/*` 交给 http-swagger 的内嵌资产（`swagger-ui-bundle.js`、`swagger-ui.css`），页面
  不引用 CDN。`server_auth.go:69` 把 `/swagger` 与 `/api/` 同等纳入浏览器访问控制。
- Rust 迁移前状态：`router.rs` 只把 `/swagger` 前缀当作“非静态 + 需认证”路径
  （`should_authenticate:298`、`static_response` 分流:340、SPA 兜底排除:475），而
  `product_production_route_manifest.json` 不含任何 `/swagger` 路由，因此请求落到 JSON 404
  `NOT_FOUND`；但 `docs/quick-start.md` 已列出 `http://127.0.0.1:3000/swagger/`，
  `apps/web/vite.config.ts` 也把 `/swagger` 代理到 API 端口——文档承诺与实现不一致。
- 本批实现：新增 `crates/jftrade-api/src/swagger_docs.rs`（`SwaggerDocs` 值对象 + `/swagger`
  文档面响应），`ApiState` 增加 `with_swagger_docs` 注入，`dispatch` 在静态分流之前处理
  GET/HEAD 的 `/swagger*`；engine 侧 `product_swagger_docs.rs` 用 `include_str!` 嵌入
  `contracts/openapi/openapi.json`（即 `check:contracts` 校验路由清单所用的同一份文档），
  `product_server.rs` 组装时注入，桌面监听器与 Web 访问监听器共用（`web_state` 由同一 state
  克隆）。文档面继续走既有 `authorize`：`/swagger` 仍需桌面令牌或 Web 会话。
- 契约面未被改动：`contracts/openapi/openapi.json`、`proto/`、路由清单与 OpenAPI 生成物逐字
  未变（`pnpm run check:generated` 通过且未改写工作树）。

### 逐条结论

- `swagger_openapi_test.go:14` →
  `crates/jftrade-api/tests/swagger_docs_contracts.rs::swagger_ui_is_available_offline_without_external_assets`：
  `/swagger/index.html` 200 `text/html; charset=utf-8` 且正文只引用同源资产（断言不含
  `cdn.jsdelivr.net`、`http://`、`https://`），`/swagger/docs.css` 200 `text/css; charset=utf-8`，
  `/swagger/docs.js` 200 `application/javascript; charset=utf-8` 且包含 `/swagger/doc.json`，
  `/swagger/doc.json` 200 `application/json; charset=utf-8`，HEAD 与 GET 同构且无响应体。
- `swagger_openapi_test.go:72` →
  `::swagger_document_exposes_the_frozen_core_paths`（transport 侧解析服务端响应）与
  `crates/jftrade-engine/src/product_swagger_docs.rs::tests::swagger_docs_embed_the_frozen_debug_api_contract`
  （组装侧断言注入来源）：`swagger=2.0`、`info.title=JFTrade Debug API`、paths 含
  `/api/v1/system/status`、`/api/v1/market-data/candles/{market}/{symbol}`、`/api/v1/ws/live`。
- `openapi_boundary_test.go:11` →
  `::swagger_roots_redirect_to_the_browsable_documentation_entry`：`/swagger` 与 `/swagger/`
  都返回 307 + `Location: /swagger/index.html` 且响应体为空（与 Go 的 httptest 逐条一致）。
- `openapi_schema_compatibility_test.go:14` →
  `::swagger_document_preserves_legacy_schema_names`：服务端 `definitions` 含 Go 列举的全部 40 个
  稳定名（38 个 `adk.*` 与 `servercore.WebSessionData`、`servercore.webLoginRequest`），且不出现
  `model.*`、`workflowruntime.*`、`webaccess.WebSessionData`、`webaccess.webLoginRequest`。
- 附带边界：`::swagger_surface_requires_access_and_keeps_the_json_envelope` 断言未认证
  `/swagger/` 为 401 `WEB_AUTH_REQUIRED`、未知资产 `/swagger/missing.js` 为既有 JSON
  `NOT_FOUND` 信封、`POST /swagger/doc.json` 同样 404（文档面只读）。

### 探针记录（破 → 红 → 按字节回滚）

- 把重定向目标从 `/swagger/index.html` 改为 `/swagger/doc.json`（`swagger_docs.rs`）：
  `swagger_roots_redirect_to_the_browsable_documentation_entry` 转红
  （`left: "/swagger/doc.json"` vs `right: "/swagger/index.html"`）。
- 把 `/swagger` 移出 `should_authenticate`（`router.rs`）：`swagger_surface_requires_access_and_keeps_the_json_envelope`
  转红（`left: 307` vs `right: 401`），证明面板没有被放开成匿名端点。
- 把 index 的脚本改回 `https://cdn.jsdelivr.net/...`（`swagger_docs.rs`）：
  `swagger_ui_is_available_offline_without_external_assets` 转红（正文出现外部 CDN 引用）。
- 把 engine 的 `SWAGGER_DOCUMENT` 换成 `"{}"`（`product_swagger_docs.rs`）：
  `swagger_docs_embed_the_frozen_debug_api_contract` 转红（`left: Null` vs `right: "2.0"`）。

四次探针均按字节回滚，回滚后 shasum 与探测前一致：

- `crates/jftrade-api/src/swagger_docs.rs`：`1f12c919a09aeb2b53e7d3fa6321624b29356ede447fc36f9a69a99a5e894277`
- `crates/jftrade-api/src/router.rs`：`8a26898d6bc3e37edad5b588c28f76c2a3603e574ea856f24f3eeef603a08b14`
- `crates/jftrade-api/tests/swagger_docs_contracts.rs`：`3507d45e43893b0249563d895300a736b0bd1a2d9d414ba39e480db4723ebc86`
- `crates/jftrade-engine/src/product_swagger_docs.rs`：`8890b5ebdc52a1b1ea2dac0d99207b6ff8ef300ba4f4326af07ae55201f6a2c2`
- `crates/jftrade-engine/src/product.rs`：`9d0742ceb671189d90a4ca4d05bfdc9025ad658f1be10b66d3570e6d8059f7d3`
- `crates/jftrade-engine/src/product_server.rs`：`905868ee91749c74c95ce95f1979f99c57c878ceca06824d47579d51265900e4`

### 保留差异

- **资产名与查看器实现不同**：Go 内嵌 swagger-ui（index 引用 `swagger-ui-bundle.js` 与
  `swagger-ui.css`，初始化器为 `swagger-initializer.js`）；Rust 用第一方离线查看器
  `docs.css`/`docs.js` 渲染同一份 `doc.json`，URL 入口、307 重定向、无 CDN 与 `doc.json`
  语义一致，但不 vendor swagger-ui 资产，也不提供 try-it-out / models 面板。
- Go 的 `doc.json` 可被 `JFTRADE_OPENAPI_SOURCE` 覆盖成任意文件；Rust 只服务编译期嵌入的冻结
  契约（与 `check:contracts` 同源），不存在运行时可替换的文档来源。
- 访问路径：Go 测试服务器在未启用 Web 访问时中间件直接放行（测试拿到 200）；Rust 测试用桌面
  令牌通过 `authorize`，未认证请求是 401 `WEB_AUTH_REQUIRED`（Rust 既有信封），面板并非匿名面。
- 未知 `/swagger/*` 资产：Go 交给 http-swagger 的 404；Rust 保留既有 JSON `NOT_FOUND` 信封。

### 后续待办

- 下一批：`internal/app/apiserver/webaccess/security_integration_test.go:188`
  （`TestBrowserNavigationGetsFriendlyDisabledWebPage`，`missing` 4 → 3），实现远程 Web 访问
  被禁用时的友好 HTML 禁用页。
- 再下一批：`cmd/jftrade-desktop/desktop_updates_test.go:9`、`pkg/chart/chart_type_test.go:5`、
  `scripts/archive_frontend_assets_test.go:11`。
- 本批未处理的既有独立项：审计脚本生成的 markdown 含尖括号占位符，`build:docs:generated`
  在 markdown-it 判为未闭合 HTML 标签处失败（与本批无关）。

验证：
- `cargo fmt --all`（含 `--check`）EXIT=0；`cargo clippy -p jftrade-api -p jftrade-engine
  --all-targets --locked` EXIT=0。
- `node scripts/quality/cargo-nextest.mjs run -p jftrade-api -p jftrade-engine --all-targets
  --locked --no-fail-fast`：1830 passed / 0 skipped / 0 failed（jftrade-api 71 含本批 5 条新增；
  jftrade-engine 1759 含本批 1 条新增）。
- 四次探针各自转红并已按字节回滚（重定向目标、`/swagger` 认证、外部 CDN、engine 冻结契约），
  回滚后 shasum 与探测前一致（见上文）。
- `python3 scripts/compatibility/audit_test_parity.py`：4451 Go / 3060 Rust、`[x]`
  1322 → 1326、`missing` 5 → 4、`partial` 2547 → 2545、`boundary` 573 → 572、
  0 破坏引用、0 条 `[x]` 缺少 function_exact；未锚定 function_exact 保持 202（既有基线）、
  partial 无解析引用 7（既有基线）、无断言 2（既有基线）。
- `python3.12 scripts/compatibility/parity_anchor_reconcile.py`：1327 唯一引用（已记账 1268 →
  1272；unrecorded 0、unknown 55、stale 0）。
- `pnpm run check:rust:architecture` EXIT=0；`pnpm run check:compatibility` EXIT=0（278
  OpenAPI operations / 19 路由探针，契约未变）；`pnpm run check:generated` EXIT=0 且未改写
  工作树；`node scripts/check-zero-go.mjs` EXIT=0（2920 tracked files）；`pnpm run
  check:ai-context` EXIT=0；`git diff --check` 干净。
- `pnpm run check:quick` EXIT=0。
- `pnpm run check:rust` EXIT=1：唯一失败阶段 `check:rust:policy`（`cargo deny`）报
  RUSTSEC-2026-0285 与陈旧 advisory 告警，与本仓既有基线同源。**不记为通过**；该 run 在静态
  阶段停止，等价测试面由上面的 nextest 结果覆盖。
