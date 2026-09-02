# Rust Migration Closeout Playbook

更新时间：2026-09-02。

本文不再派发 Go → Rust 实现切片。278 条生产路由、唯一写 owner、Rust/Tauri composition 和源码删除已经完成；当前只处理零 Go 回归、历史 fixture replay 与 `0.29.0` 发布资格。历史 Stage 2–9 ledger 保留在 `tests/fixtures/rust-migration` 和 Git 历史中，不参与当前状态计算。

## 启动协议

任何零 Go或发布收口任务在编辑前按顺序执行：

1. 读取根 `AGENTS.md` 和目标目录最近的局部 `AGENTS.md`。
2. 读取本文、`go-to-rust-migration.md`、`docs/roadmap.md` 和 `scripts/module-map.json`。
3. 读取 `route-ownership.json`、`closeout-evidence.json` 和目标 gate 的 schema/checker。
4. 运行：

   ```bash
   git status --short --branch
   git diff --cached --name-only
   node scripts/rust-migration/check-stage9-route-coverage.mjs
   pnpm run check:go-retirement
   pnpm run check:zero-go
   ```

5. 输出本轮目标，明确修改边界、仍开放 gate、必须验证和不做事项。

不能从历史段落、旧统计或 fixture 内的旧 owner 文字推导当前状态。路由以 route ledger/checker 为准，release 以 closeout manifest 和 artifact-bound evidence 为准。

## 当前不变量

1. 不恢复 `.go`、Go module/work、Go generator、Go CI、`setup-go`、Go API 或 Wails runtime/entrypoint。
2. 不从历史源码重建线上兼容基线；只使用 `last-go-release-baseline.json` 绑定的 `v0.27.0` 已发布字节。
3. 不运行或更新 Go oracle。Stage 2–9 fixture 只由 Rust/Node replay 消费。
4. 不改变公开 HTTP/OpenAPI、SSE、WebSocket、SQLite schema 或 worker wire contract，除非需求明确批准。
5. 所有业务状态只有一个 Rust owner；SQLite、交易、订阅、通知、审批、任务和 artifact 禁止双写。
6. 普通测试不连接真实 Futu/OpenD、Yahoo、AKShare 或模型 Provider；真实依赖只进入显式 live/release workflow。
7. Rust 默认 `#![forbid(unsafe_code)]`；依赖集中精确锁定并遵守 `Cargo.lock`、`deny.toml`、许可证和平台约束。
8. 不把本地 smoke、fixture、checker 文本或占位报告伪装成原生平台、签名、安全或 post-release 证据。

## 工作包顺序

### 1. Go retirement ratchet

`check:go-retirement` 以不可变迁移起点为基线，只允许 Go/Wails 文件和 active command 范围减少。任何恢复、重命名迁移或新入口都失败。

### 2. 语言无关契约

- OpenAPI 规范源是 `contracts/openapi/openapi.json`。
- Futu/Pine protobuf 位于 `proto/`。
- Node 生成 Web 类型和参考文档；Rust build scripts 生成私有 protobuf 类型。
- `check:generated` 必须只读工作树，在临时目录比较输出。

### 3. Rust compatibility replay

- Stage 2–9 原始 fixture 不改写来源语义。
- Rust replay 覆盖 success/rejection/null/empty、timeout/cancel、failure/recovery、stream interruption/reconnect 和 SQLite upgrade。
- 发现 fixture 问题时先证明是 fixture/harness 错误；不能为了变绿把 `null` 改成 `[]` 或丢弃历史 quirk。
- Futu/OpenD 使用 Rust protocol test、录制 fixture 和显式凭据 live workflow；Pine/backtest 使用 Rust/Node 门禁。

### 4. 源码与入口删除

删除状态只依赖：

- `allRouteGroups=passed`；
- `uniqueWriteOwner=passed`；
- entry status 为 `removed`；
- `check:zero-go` 证明源码、模块、generator、active build graph 和入口不存在。

owner deletion 不依赖尚未发布的安装包或 post-release smoke。它也不代表候选或正式发布合格。

### 5. 零 Go 产物门禁

`check:zero-go` 必须接入：

- quick/affected/check:all 和 PR CI；
-本地 release scripts；
- Tauri bundle 检查；
- candidate input sealing；
- SBOM/provenance 检查。

扫描至少覆盖 `.go`/module 文件、Go 命令/`setup-go`、Go API/Wails 路径与运行时、Go build-info、SBOM purl/component。历史 docs/fixtures 允许保留来源文字。

### 6. 发布资格

发布证据必须来自实际平台并绑定 release ref、commit、workflow run、artifact path、size 和 digest。顺序是：

1. `--candidate-static`：仅检查 278 routes、Rust owner 和删除状态。
2. 四平台构建、签名、安装/升级/卸载/回滚/runtime smoke。
3. artifact-bound candidate checker：绑定安装包、SHA256SUMS、签名 updater、SBOM/provenance、rollback、backup/restore 和 security evidence。
4. 从线上 `v0.27.0` 原始安装包升级到 `0.29.0`，验证 9 个数据库、设置、备份恢复和核心功能。
5. 正式发布后，从不同 evidence ref 运行四平台 post-release smoke 和完整 `--check`。
6. 只有全部 gate 通过后关闭 `hardCutReadiness` 和 manifest。

`0.29.0` tag 必须在 Stage 9 closeout 允许后创建；不得为了触发证据流程预先创建一个被解释为正式放行的 tag。qualification workflow 的受控 tag/ref 规则以当前 release 文档和 workflow 为准。

## 门禁分层

| 命令 | 作用 | 不能证明 |
| --- | --- | --- |
| `pnpm run check:go-retirement` | Go/Wails 范围相对迁移基线没有增长 | 当前树绝对零 Go、发布包合格 |
| `pnpm run check:zero-go` | 当前跟踪树和传入 artifact 没有 Go/Wails | 行为兼容、签名、原生安装/升级 |
| `pnpm run check:quick` | 工作树 affected 快速反馈 | 完整 Rust 或 release 资格 |
| `pnpm run check:affected` | merge-base affected 集成门禁 | 完整 Rust differential |
| `pnpm run check:generated` | 中立契约生成可复现 | 运行时行为兼容 |
| `pnpm run check:rust:workspace` | Rust fmt、Clippy、workspace tests | Stage 2–9 全量 replay |
| `pnpm run check:rust:differential` | Stage 2–9 Rust fixture replay | workspace 静态质量、原生发布 |
| `pnpm run check:rust` | 完整 Rust workspace + replay | 四平台签名/升级/回滚 |
| `pnpm run check:all` | 完整本地仓库门禁 | 外部平台和 post-release 证据 |

## 提交策略

保持以下独立、非 squash 提交边界：

1. Go retirement ratchet。
2. 去除 Go 生成/oracle 依赖。
3. 删除 Go/Wails 实现和入口。
4. 零 Go 源码/产物门禁。
5. 发布资格、基线和事实源。

共享 worktree 有用户改动时使用隔离 worktree。每个提交前检查 `git status --short`、`git diff --check` 和实际 diff；只 stage 本批文件，不 push、不 tag，除非用户明确要求。

## 完成报告

报告必须区分：

- 已实现并提交的门禁；
- 本轮实际运行且通过的命令；
- 未运行或因环境缺失跳过的 native/live/release 验证；
- 仍为 open 的 closeout gates；
- worktree、分支、提交、是否 push/tag。

不能把 `ownerDeletion=passed` 写成 `hardCutReadiness=passed`，也不能把零 Go source gate 写成 `0.29.0` 已发布。
