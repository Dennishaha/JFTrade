# Rust Migration Execution Playbook

本文是 JFTrade Go/Wails -> Rust/Tauri 迁移的执行细则，供 harness、并行 agent 和集成 agent 使用。它不替代仓库根 AGENTS.md、局部 AGENTS.md 或 go-to-rust-migration.md；冲突时以更深层指令和当前仓库事实为准。

## Harness 启动协议

任何迁移任务在输出目标、选择 route group 或创建分支前，必须按顺序完成：

1. 读取本文，得到执行规则、风险分级、并行边界、问题收集格式和完成条件。
2. 读取根 AGENTS.md，再读取目标模块最近的局部 AGENTS.md。
3. 读取 docs/architecture/go-to-rust-migration.md、scripts/module-map.json 和相关入口文件。
4. 读取当前 tests/fixtures/rust-migration/stage9/route-ownership.json、OpenAPI baseline、group ledger、fixture、reference test 和 differential harness。
5. 执行：

   git status --short --branch
   node scripts/rust-migration/check-stage9-route-coverage.mjs

6. 根据动态结果输出本轮目标。目标必须包含当前统计、候选 group、tier、依赖、共享文件冲突、分支配额、验证命令、问题/quirk 收集位置和本轮明确不做的事项。
7. 目标输出完成前不得编辑代码；不得把计划中的调用或未执行的验证写成已完成事实。

不得只根据用户提示词中的旧统计、旧 group 名或旧提交历史派工。route-ownership.json 和门禁输出是当前状态的唯一计数来源。

## 最终目标

持续推进迁移，直到 Go 后端、Wails 桌面壳和所有 Go production owner 满足最终删除准入，由 Rust/Tauri 完整接管产品运行时。Vue 3 控制台、Node PineTS worker 和 Python market-data helper 按现有架构保留。PineTS 只产生信号、图形和 order intents；撮合、成交、资金曲线、风控和下单仍属于后端 owner。

当前允许 Go/Rust 共存，但同一业务状态任何时刻只能有一个权威 owner。Rust 新实现默认不是 production owner。开始工作前必须动态读取统计，不能假设仍为 278 operation、26 shadow、80 cutover-test-only、172 remaining、0 Rust production owner。

## 不可退让的硬约束

1. 遵守根目录及局部 AGENTS.md 全部边界：领域 crate 禁止依赖 HTTP transport、DB driver、SQLite 具体实现、Futu protobuf 和具体外部协议；internal/api/* 不得直接访问 store、integration、SQLite 或 Futu；生成代码不得手工修改；不回退用户已有改动。
2. 禁止双写。SQLite、订单、策略运行状态、审批、任务、订阅、通知、Provider/OpenD 生命周期和用户可见事件任何时候只能有一个 owner。
3. owner 切换只能在 composition root 发生。Rust 默认只读；领域 crate、handler 和测试 adapter 不得自行切换生产 owner。
4. 不新增 Rust production owner。新增 route 只能登记为 shadow、cutover-test-only 或 cutover-qualified；写实现只能在临时目录、fixture、mock port 和显式 test-cutover profile 中启用。
5. 默认 profile 不得注册 test-cutover route，不得激活 Provider、连接真实 OpenD、启动生产 helper、写生产 SQLite、发单、写通知或发布用户可见事件。
6. 不改变公开 HTTP/OpenAPI、SSE、WebSocket、Wails bindings、SQLite schema、公开 pkg/* API、worker wire contract 和桌面公开行为，除非需求明确要求。
7. wire 契约逐字节兼容：path、method、status、header、JSON 字段及顺序、null/omitted、空数组、数字精度、时间格式、错误 envelope、错误优先级、取消和超时语义均以 Go baseline 为准。
8. 发现 Go 疑似 bug 时照抄 observable behavior，不在迁移切片内修复，不通过合理化改变 Go 行为。
9. 不手工修改 OpenAPI、Wails bindings、protobuf、reference 和 embedded assets 等生成物。公开契约变化按仓库要求运行 pnpm run generate:docs 或 pnpm run check:generated。
10. Go/Rust shadow 只能只读；SQLite、交易、订阅、通知、Assistant 审批/任务和 artifact 禁止双写。
11. Rust 默认 #![forbid(unsafe_code)]。依赖集中声明、精确锁定，遵守 Cargo.lock、deny.toml、许可证、MSRV、平台支持和最小 feature 规则；不得提前引入未使用候选依赖。
12. 新增 pkg/* 必须有仓库外消费者或已发布公开签名依据；否则放入 internal/* 或对应 Rust 私有 crate。
13. 生产函数通常不超过 80 行/60 语句，生产文件目标不超过 800 行；被第三个切片重复粘贴的代码必须抽成共享工具。
14. 普通测试禁止使用真实 Futu/OpenD、Yahoo、AKShare、模型 Provider 或生产 helper；使用 fixture、mock server、ASGI transport、testkit 和临时目录。
15. 不按 Go 文件路径机械翻译，按能力 owner、领域边界、port 和 composition root 映射实现。

## 并行模型：最多三条活动分支

最多同时存在 3 条活动分支，包含集成分支：1 条集成分支负责共享文件、合并、统一门禁、架构账本、最终硬切和 Go 删除准入；最多 2 条 worker 分支处理互不重叠的 route group 或 capability slice。不得创建未登记的临时 worktree、隐式 agent 分支或额外集成分支。

同一个 route group、Rust crate、fixture、reference test、product assembly 或共享 harness 同一波只能由一个 worker 修改。已有未提交修改时先识别所属 group 和文件范围，不得覆盖、回退、清理或重新实现；已占用 group 视为 locked。

采用广度优先：每轮按 capability、tier、剩余 operation 数、依赖、风险和文件重叠生成候选矩阵；优先选最多两个共享文件最少、无需真实 Provider/OpenD 的独立 C 档 group。没有两个真正独立的 group 时只启动一个。一个 worker 一次只领取一个完整 group，完成交接后才领取下一组。C 档清空后推进 B 档，B 档证据完整后推进 A 档；A/B/C 不混入同一提交。硬切、owner 切换、Go/Wails 删除和最终 release gate 必须由集成分支串行执行。

## 风险分级

Tier A：所有写操作和状态变更，包括下单/撤单、策略启停、broker 配置写入、订阅变更、maintenance mutation、Assistant 审批/任务/会话 mutation。必须执行完整 8 步：契约账本、冻结 fixture、叶子 crate、adapter、单测/property/fuzz、shadow 或 rehearsal、test-cutover、cutover-qualified，一步不减。

Tier B：SSE/WebSocket、实时快照、跨域聚合、并发/取消/超时/恢复、依赖 Provider/OpenD 生命周期的读。必须有契约账本、Go/Rust differential、shadow 或等价显式 rehearsal；涉及并发、取消、超时、恢复或数值边界时补 property/fuzz。

Tier C：无副作用简单只读 GET 投影，如 settings、system、strategy-definitions、alerts、watchlist、plugins、catalog/status。GET 若依赖 SQLite、worker 生命周期、Provider/OpenD、实时状态、权限上下文、长连接或复杂聚合，升级为 B。依赖 consumer-owned snapshot port、只能显式 test-cutover 注册的只读 route，登记 cutover-test-only，不得强行登记 shadow。

## C 档组级批量工作流

一次处理一个完整 route group，不逐 operation 建工作包：

1. 从 tests/fixtures/rust-migration/stage7/api-control-plane-corpus.json 和现有 Stage 9 fixture 提取整组 route，写入 `tests/fixtures/rust-migration/stage9/ledgers/<group>.md`。逐 operation 记录 method、path、请求/响应要点、header、空值语义和错误分支。
2. 优先扩展现有 scripts/rust-migration/stage9_*_reference_test.go 和 generator，为整组生成 golden；禁止为单个 operation 手写独立 fixture。
3. 一个 differential 脚本覆盖整组，至少覆盖成功、空结果、404/400/401/403/409/5xx、port unavailable 及适用的 timeout/cancel。
4. 一个 Rust 测试文件或模块用表驱动覆盖整组。
5. 一次性更新 route-ownership.json、ledger 和 evidence；按实际运行模式选择 shadow 或 cutover-test-only。
6. 发现的 quirk 立即写入该组 ledger。
7. worker 先跑本组最窄测试，再跑 `pnpm run check:quick`；完整 `pnpm run check:rust` 由集成分支在合并该波次后运行。契约变化另跑 `pnpm run check:generated`。

## A/B 档规则

A 档写操作必须证明唯一 owner、重复请求语义、事务边界、失败回滚、取消/超时、重启恢复、通知/任务/审批副作用隔离和 test-cutover fencing。Rust 写实现只能在临时目录和显式 test profile 启用。

B 档先完成输入/输出 corpus、流式事件或并发行为 differential，再实现 Rust adapter。SSE/WS 比较握手、事件名、字段、顺序、心跳、断线、重连、取消和关闭码。普通测试不得连接真实外部服务。

## 共享 harness 与文件边界

以下文件默认只由集成分支串行维护：

- tests/fixtures/rust-migration/stage9/route-ownership.json
- scripts/rust-migration/stage9-route-ownership.mjs
- scripts/rust-migration/check-stage9-route-coverage.mjs
- crates/jftrade-engine/src/product*.rs、route assembly 和 composition root
- docs/architecture/go-to-rust-migration.md、docs/README.md
- package.json、workspace manifest、共享 differential runner

worker 默认只修改本 group 独占的 Rust crate、ledger、fixture、reference test 和专属 differential。必须修改共享文件时，先报告精确文件、原因、区段和冲突风险，由集成分支应用最小 patch。不得通过格式化、排序或全文件重写制造冲突。

新写 Go 参照测试前，先搜索可参数化扩展的现有 harness；优先扩展，不新建平行体系。任何被第三个切片重复粘贴的代码，必须在当前切片抽成共享工具。exact-operation rehearsal proxy、通用 fixture loader 和参数化 differential runner 等一次性基础设施优先于重复实现单域逻辑。

## Known quirks 与疑似 bug 收集

必须收集所有潜在问题，包括 status/header/envelope/字段/null/omitted/空数组/错误优先级差异，边界差一，分页和排序不稳定，时间/时区/精度损失，Decimal/fixedpoint 异常，重复请求、取消、超时、断线、重启、锁竞争、事务回滚、资源释放、Provider/OpenD/worker 不可用、权限上下文差异，以及 fixture、harness、生成器或 Rust 实现错误。

每个问题首次发现时立即记录在对应 group ledger，格式如下：

    quirk: <精确现象>
    范围: <group> / <method> <path>
    证据: <fixture、测试、differential 输出或文件位置>
    分类: go-behavior | rust-implementation | fixture | harness | generated-contract | unknown
    判定: intended | deviated | unresolved
    处置: 复刻，待硬切后修复 | 修复 Rust 使其匹配 Go | 修复 fixture/harness | 阻断切片
    风险: low | medium | high | release-blocker
    owner: <Go/Rust/集成分支或待指定>
    后续: <硬切前、硬切后或发布前处理条件>

在分类和判定完成前必须为 unresolved，先用最小复现 fixture、Go baseline 和 Rust replay 三方复核。Go 疑似 bug 必须照抄并记录：`quirk: <现象> | 判定: intended/deviated | 处置: 复刻，待硬切后修复`。不得在迁移切片内修复 Go observable bug，不得删除 quirk 记录来让 differential 变绿。确认是测试错误时保留原记录并追加复核结论。

high 或 release-blocker 必须进入 group ledger、切片报告和最终 hard-cut checklist；未解决的高风险问题不得进入 cutover-qualified。所有未修复 quirk 必须在 Go 删除前明确硬切前修复、硬切后修复或接受现状。

## 验证、提交和交接

### 门禁分层

| 命令 | 比较范围 | 责任方 | 用途与边界 |
| --- | --- | --- | --- |
| `pnpm run check:quick` | 当前工作树相对 `HEAD` | worker | 最快反馈；运行受影响模块、Rust crate 及其反向依赖和迁移静态账本。只报告 deferred integration checks，不代表 PR 或发布资格。 |
| `pnpm run check:affected` | 当前分支相对 merge-base | 集成分支 | 每波合并后的受影响门禁；共享 manifest、依赖图过大或无法安全缩窄时允许回退到 workspace/full package 测试。它不替代完整 Rust migration gate。 |
| `pnpm run check:rust:workspace` | 整个 Rust workspace | 门禁编排/故障定位 | target health、layout、route coverage、fmt、Clippy 和 workspace tests；不执行迁移 differential。 |
| `pnpm run check:rust:differential` | Stage 2–9 全部迁移差分 | 门禁编排/故障定位 | Stage 2–8 最多两路并行，Stage 9 最后串行；不重复 workspace 静态检查。 |
| `pnpm run check:rust` | 完整 Rust workspace 与迁移差分 | 集成分支 | Rust migration 的最终本地门禁。一次执行中 target health 只检查一次，并依次组合 workspace 与 differential。 |

`check:all` 在 preflight 阶段运行 `check:rust:workspace`，在后续串行阶段运行 `check:rust:differential`，避免在同一主门禁中重复完整 `check:rust`。单独运行分层命令只用于定位失败或对应门禁层；不得把两个分层命令中的任意一个单独写成完整 Rust gate 已通过。

### 执行顺序

worker 切片按以下顺序验证：

1. 本 group 最窄的 fixture、Go reference、Rust unit/integration 和 rehearsal test。
2. `pnpm run check:quick`。
3. 在交接中原样记录失败项、未运行项和 deferred integration checks。

集成分支每波合并后按以下顺序验证：

1. `pnpm run check:affected`。
2. 涉及 Rust migration 时运行一次 `pnpm run check:rust`；不得因 affected 已通过而省略，也不在同一波次为每个 worker 重复运行。
3. 公开契约变化时运行 `pnpm run check:generated`。
4. 发布或主分支准入按要求继续运行 `pnpm run check:all`。

### 长时间任务与产物健康

affected runner、Rust 门禁的并行阶段和 Stage 9 runner 每 30 秒输出心跳，并在命令完成后报告耗时；workspace 串行命令直接转发工具自身输出。心跳持续出现且子进程仍存活时，不得仅因测试暂时没有逐条输出就判定挂起或并发启动第二个 Cargo 重任务。Stage 9 product differential 固定为三组 Go package 与三组 Cargo package/target 批次；engine integration target 从 `crates/jftrade-engine/tests/*.rs` 动态发现。每个 Stage 9 批次上限为 300 秒，超时后终止整个子进程树，防止遗留 Cargo、rustc 或测试进程继续占锁。

`check:rust:target-health` 会对 debug/release profile 的 `.rcgu.o` 中间文件做提前终止扫描；任一 profile 达到 50,000 个即失败。该检查只报告问题，绝不自动删除产物。失败时：

1. 先确认没有仍在运行的 Cargo、rustc、Clippy 或 Rust 测试进程；有进程时先定位其所属任务，不得边编译边清理。
2. 确认无 Rust 构建任务后，显式运行 `pnpm run clean:rust:artifacts`。
3. 重新运行最窄失败命令；集成分支随后重新完成原门禁。

不得把 quick 通过、持续心跳或 deferred integration checks 写成完整门禁通过，也不得为缩短耗时删除测试、放宽 differential 或自动清空用户构建缓存。

一个 route group 一个提交，不混合不同 tier、能力或 owner 变更。推荐格式：

    feat(rust-migration): shadow <group> read-only operations (batch)
    feat(rust-migration): cutover-test <group> read-only operations (batch)
    feat(rust-migration): rehearse <group> mutation operations (test-only)

worker 交接必须包含 group、tier、operation 数和状态变化；修改文件和共享文件触碰情况；src/test/fixture/docs 行数；验证命令及结果；quirk、未解决差异、未跑门禁及原因；下一波可并行 group。提交前确认没有其他 worker 改动、生成物、默认 owner 变更或真实外部服务连接。

## 单切片 Definition of Done

- [ ] 该组所有 operation 在 route-ownership.json 中状态正确更新。
- [ ] node scripts/rust-migration/check-stage9-route-coverage.mjs 通过。
- [ ] 组级 differential 全绿。
- [ ] worker 的 pnpm run check:quick 通过，交接记录包含它输出的 deferred integration checks。
- [ ] 集成分支的 pnpm run check:affected 通过。
- [ ] Rust migration 变更的完整 pnpm run check:rust 在集成分支通过；未完成前该组不得登记为最终 cutover-qualified。
- [ ] 契约变化时 pnpm run check:generated 通过。
- [ ] 没有 Rust production owner 变更、默认 profile 写 route、真实 Provider/OpenD/helper 激活或任何双写。
- [ ] 架构账本追加 1 至 3 行，包含门禁实际派生的最新 operation 统计。
- [ ] 所有 quirk 已记录；high/release-blocker 已进入总账和后续 gate。
- [ ] 切片报告包含组名、operation 数、tier、修改文件、src/test/fixture/docs 行数、验证命令、问题、未完成项和下一波 group。

## 最终 Go 删除准入

所有 route group 达到 cutover-qualified 后仍不得立即删除 Go。集成分支必须串行完成：HTTP、SSE、WebSocket、桌面、worker、SQLite、通知、任务、审批、交易和 Provider/OpenD 的最终 owner 矩阵；每项能力的唯一 owner 切换、回退和无双写证据；四平台 Rust/Tauri release candidate、签名安装包、签名 updater、SBOM 和安全审查；backup/restore、崩溃恢复、锁释放、升级失败回退、资源泄漏和 post-release smoke；所有 known quirks 的处置结论；关闭 Go/Wails production entrypoint 的独立验证；以及架构文档、README、module map、发布脚本和删除清单同步。

只有全部证据可复现、回退有效、hard-cut checklist 通过且没有未批准 owner 变更时，才允许删除 Go/Wails。任何 gate 失败都保持 Go owner，不得通过删测试、放宽 differential、修改统计、隐藏 quirk 或降低安全边界来完成迁移。

## Harness 输出目标格式

读取本文和仓库事实后，harness 必须先输出一份短目标，至少包含：

    迁移波次: <编号>
    当前统计: <由 route coverage 动态派生>
    活动分支: <集成分支 + worker 分支，最多 3 条>
    本轮 group: <最多 2 个，含 tier 和 operation 数>
    不并行原因: <共享文件、owner、生命周期或依赖冲突>
    修改边界: <每个 worker 的独占文件范围>
    必须验证: <affected / check:quick / check:rust / check:generated>
    问题收集: <ledger 路径和 quirk 状态>
    本轮不做: <production owner、真实 Provider/OpenD、Go 删除等>
    完成条件: <本 group 的 DoD>

目标输出完成后，才开始编辑代码或创建 worker 任务。
