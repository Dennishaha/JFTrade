# JFTrade Go → Rust 迁移事实源

状态：运行时迁移与源码删除已完成，`0.29.0` 发布收口进行中。更新时间：2026-09-02。

## 当前结论

- Rust/Tauri 已接管 278 条 `/api/v1/*` 生产路由、全部业务 owner、9 个 SQLite 数据库和桌面运行时。
- `route-ownership.json` 动态派生为 278 `cutover-qualified`、278 `productionOwner=rust`、278 `goRemovalStatus=removed`、0 remaining。
- Go/Wails 源码、模块文件、生成器、CI/构建入口、桌面入口和运行产物已从产品树删除。
- `ownerDeletion.go` 与 `ownerDeletion.wails` 已关闭；它们证明的是仓库/入口状态，不代替发布资格。
- `0.29.0` 直接作为首个零 Go 版本，不额外制作、重建或发布“最后一份 Go 兼容基线”。
- Stage 9 closeout 仍为 `in_progress`。平台安装、签名 updater、安全审查、SBOM、升级/回滚、备份恢复和 post-release smoke 没有全部通过前，不允许创建 `0.29.0` tag，也不得声称正式迁移发布已经完成。

当前状态只由以下机器可读事实派生：

- `tests/fixtures/rust-migration/stage9/route-ownership.json`
- `crates/jftrade-engine/src/product_production_route_manifest.json`
- `tests/fixtures/rust-migration/stage9/closeout-evidence.json`
- `scripts/rust-migration/layout-policy.json`
- `scripts/module-map.json`

历史 ledger、旧阶段说明、fixture 中的 owner 字样和 Git 提交信息只记录迁移过程，不参与当前路由、owner、删除或发布状态计算。

## 线上最后一个 Go 版本

线上最后一个正式 Go 版本是 `v0.27.0`，发布 commit 为 `452dea115ca75c51361e8876c2aefd7c009839b8`。完整 release URL、发布时间、安装包 URL、官方 SHA-256 和签名缺口记录在：

- `tests/fixtures/rust-migration/stage9/last-go-release-baseline.json`

该 tag 是 lightweight tag，没有发布 tag signature 或 detached release signature；macOS 与 Windows 文件名明确标记为 unsigned。这是历史发布事实，不能通过重建或补签改变。资格测试必须：

1. 下载并校验线上发布的原始字节。
2. 直接从这些安装包升级到目标 `0.29.0` 候选。
3. 禁止从历史 Go 源码临时重建基线。
4. 禁止生成新的“最终 Go corpus”或让 Go 更新 Stage 2–9 fixture。

如果新 schema 不能被旧版本安全读取，回滚流程必须先恢复升级前备份，再启动旧版本；不得让旧版本直接打开升级后的数据库。

## 兼容契约

除非需求明确批准，零 Go 收口不改变：

| 边界 | 保持内容 | 当前门禁 |
| --- | --- | --- |
| HTTP/OpenAPI | path、method、status、header、JSON、null/omitted、精度和错误 envelope | 中立 OpenAPI、278 route manifest、认证矩阵、Rust route tests |
| SSE/WebSocket | 握手、事件、顺序、心跳、断线/重连、取消和关闭 | Rust transport/product replay 与录制 fixture |
| SQLite | 文件名、schema、migration、索引、事务、busy/WAL、时间与 Decimal 编码 | Rust schema replay、fixture、upgrade/backup/restore drill |
| 前端 | `/api/v1/*`、页面状态、错误与恢复行为 | Web tests、production bundle smoke、桌面 smoke |
| Pine/Python | wire、启动、鉴权、超时、重试、停止和发布资产 | Rust/Node/Python tests 与真实打包 smoke |
| Futu/OpenD | protobuf、retType/error、推送、订阅、重连和唯一 owner | Rust protocol tests、录制 fixture、显式 live workflow |

`contracts/openapi/openapi.json` 是语言无关的 HTTP 规范源，由 Node 生成 Web 类型和参考文档。`proto/futu` 与 `proto/pineworker` 是中立 protobuf 目录，只保留 Rust/Node 消费链。

Stage 2–9 的历史 fixtures/corpus 保持原始字节和来源描述，required checks 只运行 Rust replay。历史 fixture 可以包含 Go 来源、observable quirks 和旧 owner 记录，但不得作为当前实现或 fallback。

## 零 Go 门禁

### `check:go-retirement`

以迁移开始 commit 为不可变基线，允许 Go/Wails 范围减少，禁止新增、移动或恢复：

- `.go` 与 Go module/work 文件；
- Go build/test/run/generate 命令和 `setup-go`；
- Go API、Wails runtime 或构建入口。

它保留单调递减的迁移审计意义，并接入 quick/affected/PR CI。

### `check:zero-go`

最终不变量同时检查源码和发布产物：

- 版本控制中没有 `.go`、`go.mod`、`go.sum`、`go.work` 或 `go.work.sum`；
- active scripts、package scripts、CI、apps、crates 和 workers 中没有 Go 命令、`setup-go`、Go API 或 Wails runtime/entrypoint；
- Tauri bundle、candidate inputs、SBOM/provenance 中没有 Go/Wails 文件、组件或 Go build-info；
- 发布脚本和 release workflow 在构建/资格检查中运行零 Go 门禁。

历史文档和 `tests/fixtures` 可以说明 Go 来源。“零 Go”指当前仓库不存在 Go 源码、工具链、生成依赖和运行产物，不要求删除历史文字。

## 迁移完成后的架构

- `crates/jftrade-engine`：唯一 product composition 和生产 API runtime。
- `crates/jftrade-api`：HTTP/SSE/WebSocket transport 和认证。
- `crates/jftrade-*`：领域、SQLite/settings stores、Futu/Pine/helper integrations。
- `apps/desktop/src-tauri`：唯一桌面壳。
- `apps/web`：Vue 3 控制台。
- `workers/pineworker`：Node PineTS worker，只输出信号、图形和 order intents。
- `workers/marketdata-sidecar`：Python yfinance/AKShare helper。
- `runtime-assets/{web,pine,marketdata}`：发布资产准备位置。

任何业务状态只允许一个 Rust owner；SQLite、交易、订阅、通知、Assistant 审批/任务、artifact 和 worker lifecycle 禁止双写。

## `0.29.0` 发布资格

以下均须绑定同一 release ref、commit、workflow run 和实际产物：

- 278 路由覆盖、认证矩阵、Rust production policy、生成契约和完整 Rust workspace 门禁通过。
- 所有历史兼容 fixture 由 Rust replay 通过，覆盖成功、拒绝、空值、超时、恢复、流中断和 SQLite 升级。
- 从已发布 `v0.27.0` 安装包到 `0.29.0` 的真实安装升级、数据迁移、备份恢复和核心功能 smoke 通过。
- macOS ARM64、Linux x64、Windows x64/ARM64 完成 package/sign/install/upgrade/uninstall/rollback/runtime smoke；release matrix 若增加 Linux ARM64，必须以同样证据纳入 manifest 后再宣称支持。
- 签名 updater、SBOM/provenance、独立安全审查、Pine/Python runtime 资产和最终安装包零 Go 检查通过。
- 正式发布后完成固定 post-release smoke，并关闭 `hardCutReadiness` 和 closeout manifest。

`check-stage9-closeout.mjs --candidate-static` 只证明本地路由和唯一 owner；artifact-bound candidate checker 证明候选产物；默认 `--check` 只在发布后证据完整时允许 formal close。三者不可互相替代。

## 未完成工作

活动项只在 [roadmap.md](../roadmap.md) 和 closeout manifest 中维护。当前仍开放：

- `platformRelease`
- `signedUpdaterArtifact`
- `securityReview`
- `sbom`
- `rollbackArtifact`
- `backupRestoreDrill`
- `postReleaseSmoke`
- `hardCutReadiness`

关闭这些 gate 前不新增 Go 补丁版、不重建基线、不重新打兼容 tag、不 push `0.29.0` tag。
