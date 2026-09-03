# 测试与质量门禁

JFTrade 不再以全仓每一类代码都达到 98% 为目标。覆盖率是发现未验证行为的信号，不是业务正确性的替代品：风险越高的边界要求越严格，新增代码必须有足够的增量覆盖，路由、订单和迁移等有限契约面则要求完整枚举。

## 覆盖率政策

| 范围 | 全局/普通模块 | 关键业务域 | 改动代码 |
| --- | ---: | ---: | ---: |
| Rust | 不以单一行覆盖率替代行为证明 | route/owner/schema/订单/协议契约完整枚举 | 新行为必须覆盖成功、拒绝和恢复路径 |
| Web | statements/lines ≥ 90%，branches/functions ≥ 85% | statements/lines/functions ≥ 95%，branches ≥ 90% | 与所属风险级别相同 |
| PineTS worker | statements/lines ≥ 90%，functions ≥ 95%，branches ≥ 80% | 协议和运行边界由契约测试完整枚举 | 不降低全局门槛规避未覆盖改动 |

关键 Rust 域包括交易和订单、实盘行情、Futu/OpenD、回测和策略执行、安全认证、SQLite schema/migration。关键 Web 的静态 95/90 门槛当前覆盖下单确认、风控、订单状态和实时行情；`BacktestPage` 与 `useBacktestRuns` 仍按关键改动代码门槛检查，但在补足业务场景前不追溯施加静态 95/90 分支门槛。目录归类和实际阈值以 Rust gate、覆盖检查器及 Vitest 配置为准。

`JFTRADE_DIFF_BASE` 指向 PR base SHA（main push 使用前一提交）时，Web 额外检查新增/修改的可执行语句。Rust 变更由 affected crate、反向依赖、Clippy、workspace tests 和 compatibility replay 覆盖。没有可执行 Web 改动时报告为 `n/a`，不会以空报告伪造覆盖率。

下列有限契约面要求“完整”，这里的完整是枚举和行为完整，而不是给复杂实现堆到 100% 行覆盖：

- 已注册 HTTP 路由、OpenAPI 路径和写请求 DTO；
- broker capability catalog 与 API/UI/tool surface；
- 订单状态迁移、fail-closed 风控和权限拒绝；
- SQLite migration 与旧数据归一；
- Pine worker 协议、生成输入和 embedded asset 选择。
- market-data helper 的 CLI、`/healthz`、双 Provider 路由、输入约束、结构化错误、线程池边界和非有限数值清洗；普通测试用 fixture 并禁止真实网络。

## 分层执行

| 层 | 内容 | 触发方式 |
| --- | --- | --- |
| L0 静态与契约 | Rust fmt/Clippy、typecheck、架构依赖、OpenAPI/API types/Tauri runtime 生成一致性、许可证和测试命名 | 每个 PR、main |
| L1 单元与组件 | Rust、Web、Pine worker 和 Python sidecar 的确定性测试；Web/worker 执行覆盖率与 Web 增量覆盖率 | 每个 PR、main |
| L2 隔离集成 | 临时 SQLite、mock HTTP/OpenD/broker/Pine worker、mock yfinance + socket 阻断；禁止调用真实外部服务 | 每个 PR、main |
| L3 系统回归 | release assets、嵌入 market-data helper 的启动/双 Provider 健康/清理、并发重复；PR 构建 Linux desktop，main 额外执行完整 Rust replay、真实 PineTS backtest smoke 和桌面矩阵 | PR / main |
| L4 手动重型验证 | race、性能基线与真实 OpenD | manual |

本地开发再按提交范围分为三个入口：`check:quick` 只读取相对 `HEAD` 的当前工作树，运行受影响 crate/package、Rust 反向依赖和产品策略检查，并列出 deferred integration checks；`check:affected` 按 merge-base 运行完整 affected 集合；`check:rust` 依次执行 Rust static、唯一一次 workspace tests 和七类 compatibility replay。`check:rust:static` 与 `check:rust:workspace` 供 CI 使用独立 runner 并行执行；workspace tests 结束后，storage、backtest、provider-runtime、trading-strategy、assistant-runtime、api-transport 和 desktop-runtime replay 并行复用已编译 target。target health 对每个 profile 最多扫描 50,000 个中间 `.rcgu.o` 后 fail-fast；该阈值高于一次完整冷门禁的正常产物，并在历史异常目录增长到数十万对象前阻断。任何 Rust affected 输出中的 `check:rust` deferred 项仍须由集成分支完成。

`.github/workflows/ci.yml` 是 PR 与 main 的主门禁。`gate-plan` 对 PR 使用 merge-base affected 计划，遇到未知路径、共享工具链或 planner 错误时 fail closed 为全量；main 始终执行完整核心门禁。Policy 固定运行，Contracts、Rust Static、Rust Tests + Compatibility、Web、Pine、Python 和 Desktop 按依赖图并行。Python lane 运行禁止外部行情网络的 pytest，并由发布矩阵构建、验证 PyInstaller helper。PR 的 desktop lane 只做 Linux 原生 smoke build；main 同时启动 Linux x64、macOS ARM64、Windows x64 构建，Windows ARM64 单独原生编译。桌面 job 只等待 contracts、Web、Pine 和 sidecar 构建输入，不等待 Rust tests，最终仍由稳定的 `Build & Test` required check 汇总。

覆盖 lane 会把命令输出及 Web/worker coverage 报告保存为 CI artifact，并在对应 job summary 摘出总量和增量结果；Rust tests 与 compatibility 在同一 job 中复用 target，便于定位失败且不重复编译。

真实 Futu/OpenD 不属于普通 PR 或 main CI：只能通过 `futu-live.yml` 手动触发，并调度到带 `self-hosted`、`futu`、`opend` 标签的 runner。该 workflow 显式设置 `JFTRADE_FUTU_LIVE_TEST=1`，执行默认 ignored 的 Rust live contract，覆盖 health probe、ProviderRouter activation、managed demand、generation-fenced HK quote cache 与 shutdown release；未连通 OpenD、行情权限不足或没有显式确认时必须失败，绝不把 ignored/跳过当作通过。性能基准保留手动触发；比较 ref 必须显式或由 workflow 固定，不能把单次样本写成发布结论。

## 本地入口

```bash
# 当前未提交切片的快速检查；输出尚未执行的集成门禁
pnpm run check:quick

# 相对 merge-base 的完整 affected 检查
pnpm run check:affected

# 快速的本地 PR 前检查；test:pr 是兼容别名
pnpm run test:preflight

# 单机可执行的 Linux CI 核心门禁，不包含 GitHub 三操作系统矩阵
pnpm run test:ci-local

# 完整本地门禁：ci-local、actionlint、完整 Rust、当前平台 desktop 和真实 PineTS smoke
pnpm run check:all

# test:main 是 check:all 的兼容入口
pnpm run test:main

# Rust static、workspace tests 与产品兼容 replay；check:rust 组合三者
pnpm run check:rust:target-health
pnpm run check:rust:static
pnpm run check:rust:workspace
pnpm run check:compatibility
pnpm run check:rust

# target health 报告大量中断编译遗留对象时，确认没有 Cargo 进程后显式清理
pnpm run clean:rust:artifacts

# 单独运行 Web 与 Pine worker 覆盖率门禁
pnpm run test:coverage

# 开发依赖安装后运行离线 helper 契约测试
python -m pytest workers/marketdata-sidecar/tests

# 构建当前平台的 PyInstaller helper（发布矩阵预先构建四个平台资产）
pnpm run build:marketdata-sidecar
```

已有自动化或个人习惯可以继续调用 `pnpm run test:pr`；它与 `test:preflight` 等价。跨平台 proto 与桌面构建仍以 GitHub Actions 为准，本地入口不伪装成完整的多平台 CI。

本地分支若要启用增量覆盖，设置 base ref；CI 会自动提供它：

```bash
JFTRADE_DIFF_BASE=origin/main pnpm run test:coverage
```

## Python 行情 sidecar 与发布资产测试

- Rust/Tauri 资产测试按目标平台选择 `runtime-assets` 中的 helper，拒绝缺失或空文件，校验 SHA-256，并验证释放目录和可执行文件权限受限；关闭 Provider 或应用时必须删除临时目录。
- sidecar manager 测试动态分配 loopback 端口、传入内部 endpoint、探测 `/healthz` 与 Provider health、停止进程和清理资产。正式运行只允许 JFTrade 自动托管的嵌入 helper；`JFTRADE_MARKETDATA_SIDECAR` 仅用于开发/测试绝对路径覆盖。
- 设置与运行时测试覆盖新安装默认 `akshare`、明确的 Futu/yfinance 选择保留、历史 yfinance 连接配置不再参与运行时、启动恢复 helper 缺失或失败时保留已配置 Provider 并报告不可用，以及显式切换失败时保持旧 Provider。
- Web/API 测试确认行情提供者菜单和契约只暴露 Provider 选择、状态与能力，不请求或渲染历史连接配置；设置页不再包含行情 Provider 分类；能力断言明确 yfinance 为延迟快照/历史 K 线、无实时推流和 Level 2。
- 发布 smoke 在 macOS ARM64、Linux AMD64、Windows AMD64、Windows ARM64 分别启动嵌入 helper 并验证 `--version`、`/healthz`、Yahoo/AKShare 导入、退出清理和 SHA-256。macOS 额外执行 helper 与应用深度签名校验；Windows 校验签名安装器；Linux 将 helper 摘要纳入 SBOM/校验清单。

真实 Yahoo/AKShare 上游只通过手动 `.github/workflows/marketdata-live.yml` 或显式
`JFTRADE_MARKETDATA_LIVE_SMOKE=1` 运行 `workers/marketdata-sidecar/scripts/marketdata_live_smoke.py`。
该 smoke 覆盖核心行情、研究端点、yfinance 分页、AKShare 五市场筛选、行业/概念板块、
错误拒绝和 31 天经济日历；普通 pytest、PR 和 main CI 不访问真实网络。live 报告只保留
版本、端点、状态码、耗时、条目数和失败分类，失败必须非零退出。

2026-08-04 的本机 macOS ARM64 样本按“递归累加 PyInstaller `onedir` 下所有普通文件的 `stat().st_size`，不计目录和文件系统分配块”测量：旧 yfinance bundle 为 89.20 MiB，新 market-data bundle 为 104.65 MiB，增加 15.45 MiB（17.3%）。这是单机迁移基线，不作为其他平台的固定体积上限；`smoke:marketdata-sidecar` 会在每个发布 runner 上报告该平台 bundle 的精确字节数和 MiB，并同时验证 frozen helper 的版本、loopback 健康与两个 Python Provider 的导入状态。

`test:preflight`、`test:pr` 和 `test:ci-local` 使用 `pnpm run check:generated` 在临时目录生成 Swagger、前端 OpenAPI 类型、契约基线和参考文档，并逐字节比较需要提交的 OpenAPI 类型、契约基线和 Pine 支持矩阵。Swagger runtime 快照只作为临时测试输入，不写入仓库。因此这些检查不会修改工作树；契约有意变化时才运行 `pnpm run generate:docs` 更新跟踪产物。

测试文件名必须描述被验证的业务行为，不得包含任意大小写或分隔形式的 `coverage`，也不得使用 `c95`、`c_98`、`push95`、`_98_` 等覆盖率数字缩写，或 `more`、`additional`、`extra`、`complete` 等无业务语义后缀。`pnpm run check:test-names` 扫描当前全仓文件；历史违规文件已经全部改为行为名称，`scripts/test-name-allowlist.txt` 当前没有豁免项。检查器仍从 merge-base Git 文件树按当前规则推导历史上限，不信任基准提交里的旧清单，因此规则扩展后可纳管此前遗漏的文件，也不能通过“新增违规文件并写入清单”绕过门禁。

## 编写测试

- Transport 断言参数绑定、状态码、错误码和 response envelope；domain port 通过 fake 覆盖业务规则与失败语义。
- Store 使用临时数据库，覆盖 migration、旧数据归一、并发与重载；集成测试使用 mock server 或协议 fixture。跨出 Futu integration 边界的测试只能依赖 `jftrade-integration-futu` 的公开测试支撑或录制 fixture，不得让领域 crate 直接依赖 OpenD codec 或生成 protobuf 类型。
- market-data sidecar 测试必须通过 ASGI transport 以及 yfinance/AKShare fixture 覆盖双 Provider 的成功、失败和隔离契约，并全局阻止 socket；普通 CI 不访问真实 Yahoo Finance 或 AKShare 网络。
- 用例必须断言业务结果、状态迁移或可观察副作用，而不只执行代码行。复杂 UI 和策略运行优先覆盖分支、拒绝路径和恢复路径。
- 真实网络、账户、交易和行情权限只出现在显式 live workflow；没有该环境时，不得以 `skip` 充当生产验证结论。
