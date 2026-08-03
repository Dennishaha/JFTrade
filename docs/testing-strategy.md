# 测试与质量门禁

JFTrade 不再以全仓每一类代码都达到 98% 为目标。覆盖率是发现未验证行为的信号，不是业务正确性的替代品：风险越高的边界要求越严格，新增代码必须有足够的增量覆盖，路由、订单和迁移等有限契约面则要求完整枚举。

## 覆盖率政策

| 范围 | 全局/普通模块 | 关键业务域 | 改动代码 |
| --- | ---: | ---: | ---: |
| Go | 业务总量 ≥ 90%，普通 package ≥ 85% | ≥ 95% | 普通 ≥ 90%，关键域 ≥ 95% |
| Web | statements/lines ≥ 90%，branches/functions ≥ 85% | statements/lines/functions ≥ 95%，branches ≥ 90% | 与所属风险级别相同 |
| PineTS worker | statements/lines ≥ 90%，functions ≥ 95%，branches ≥ 80% | 协议和运行边界由契约测试完整枚举 | 不降低全局门槛规避未覆盖改动 |

关键 Go 域包括交易和订单、实盘行情、Futu/OpenD、回测和策略执行、安全认证、SQLite schema/migration。关键 Web 的静态 95/90 门槛当前覆盖下单确认、风控、订单状态和实时行情；`BacktestPage` 与 `useBacktestRuns` 仍按关键改动代码门槛检查，但在补足业务场景前不追溯施加静态 95/90 分支门槛。目录归类和实际阈值以覆盖检查器及 Vitest 配置为准。

`JFTRADE_DIFF_BASE` 指向 PR base SHA（main push 使用前一提交）时，Go 与 Web 额外检查新增/修改的可执行语句。没有可执行改动时报告为 `n/a`，不会以空报告伪造覆盖率。普通总量达标但新代码没有测试，增量门禁仍会失败。

下列有限契约面要求“完整”，这里的完整是枚举和行为完整，而不是给复杂实现堆到 100% 行覆盖：

- 已注册 HTTP 路由、OpenAPI 路径和写请求 DTO；
- broker capability catalog 与 API/UI/tool surface；
- 订单状态迁移、fail-closed 风控和权限拒绝；
- SQLite migration 与旧数据归一；
- Pine worker 协议、生成输入和 embedded asset 选择。
- yfinance helper 的 CLI（`--host`、`--port`、`--version`）、`/health`、路由、输入约束、结构化错误和非有限数值清洗；PyInstaller 冻结产物必须能启动并退出。

## 分层执行

| 层 | 内容 | 触发方式 |
| --- | --- | --- |
| L0 静态与契约 | lint、vet、typecheck、架构依赖、OpenAPI/API types/Wails 生成一致性、许可证和测试命名 | 每个 PR、main |
| L1 单元与组件 | Go、Web、Pine worker 和 Python sidecar 的确定性测试；前三者执行覆盖率/增量覆盖率 | 每个 PR、main |
| L2 隔离集成 | 临时 SQLite、`httptest`、mock OpenD/broker/Pine worker、mock yfinance + socket 阻断；禁止调用真实外部服务 | 每个 PR、main |
| L3 系统回归 | release assets、嵌入 yfinance helper 的启动/健康/清理、并发重复；PR 构建 Linux desktop，main 额外执行完整 Go 回归、真实 PineTS backtest smoke 和三平台 desktop build | PR / main |
| L4 手动重型验证 | race、性能基线与真实 OpenD | manual |

`.github/workflows/ci.yml` 是 PR 与 main 的主门禁。合同和参考文档由独立 job 统一生成并检查一次，再通过 workflow artifact 交给 Go、Web 资产和 desktop 消费；不依赖合同的 Web 质量、Pine、proto 和 yfinance sidecar job 可立即并行。yfinance lane 在 Python 3.11 和 3.14 上运行禁止外部行情网络的 pytest，只有 3.14 lane 构建并验证 PyInstaller helper。PR 的 desktop lane 只做 Linux 原生 smoke build；main 的 desktop matrix 使用 Python 3.14 验证 Linux AMD64、macOS ARM64、Windows AMD64 和 Windows ARM64 的 helper 资产与对应桌面产物。桌面 job 只有在对应基础门禁全部通过后才启动，最终仍由稳定的 `Build & Test` required check 汇总。

每个覆盖 lane 会把命令输出及 Go/Web/worker 的 coverage 报告保存为 CI artifact（保留 7 天），并在对应 job summary 摘出总量和增量结果，便于定位门禁失败而不依赖本地复现。

真实 Futu/OpenD 不属于普通 PR 或 main CI：只能通过 `futu-live.yml` 手动触发，并调度到带 `self-hosted`、`futu`、`opend` 标签的 runner。该 workflow 显式设置 `JFTRADE_FUTU_LIVE_TEST=1`，并在未连通 OpenD 或权限不足时失败，绝不把跳过当作通过。性能基准保留手动触发；每周在 GitHub 托管的 macOS ARM64 runner 上，将当前 main 与其上一提交放在同一 job 内连续比较。手动性能测试未填写 `compare_ref` 时同样比较上一提交，填写后才使用指定基线。

## 本地入口

```bash
# 快速的本地提交前检查；test:pr 是兼容别名
pnpm run test:preflight

# 单机可执行的 Linux CI 核心门禁，不包含 GitHub 三操作系统矩阵
pnpm run test:ci-local

# 在 ci-local 之上执行完整 Go、当前平台 desktop 和真实 PineTS smoke
pnpm run test:main

# 单独运行三套覆盖率门禁
pnpm run test:coverage

# 开发依赖安装后运行离线 helper 契约测试
python -m pytest workers/yfinance-sidecar/tests

# 构建当前平台的 PyInstaller helper（发布矩阵预先构建四个平台资产）
pnpm run build:yfinance-sidecar
```

已有自动化或个人习惯可以继续调用 `pnpm run test:pr`；它与 `test:preflight` 等价。跨平台 proto 与桌面构建仍以 GitHub Actions 为准，本地入口不伪装成完整的多平台 CI。

本地分支若要启用增量覆盖，设置 base ref；CI 会自动提供它：

```bash
JFTRADE_DIFF_BASE=origin/main pnpm run test:coverage
```

## yfinance 与发布资产测试

- Go 资产测试按 `GOOS/GOARCH` 选择 `release_assets` 中的 helper，拒绝缺失或空文件，校验 SHA-256，并验证释放目录和可执行文件权限受限；关闭 Provider 或应用时必须删除临时目录。
- sidecar manager 测试动态分配 loopback 端口、传入内部 endpoint、探测 `/health`、停止进程和清理资产。正式运行只允许 JFTrade 自动托管的嵌入 helper；`JFTRADE_YFINANCE_SIDECAR` 仅用于开发/测试的绝对路径覆盖。
- 设置与运行时测试覆盖新安装默认 `yfinance`、明确的 Futu/yfinance 选择保留、历史 yfinance 连接配置不再参与运行时、启动恢复 helper 缺失或失败时回退 Futu，以及显式切换失败时保持旧 Provider。
- Web/API 测试确认行情提供者菜单和契约只暴露 Provider 选择、状态与能力，不请求或渲染历史连接配置；设置页不再包含行情 Provider 分类；能力断言明确 yfinance 为延迟快照/历史 K 线、无实时推流和 Level 2。
- 发布 smoke 在 macOS ARM64、Linux AMD64、Windows AMD64、Windows ARM64 分别启动嵌入 helper 并验证 `--version`、`/health`、退出清理和 SHA-256。macOS 额外执行 helper 与应用深度签名校验；Windows 校验签名安装器；Linux 将 helper 摘要纳入 SBOM/校验清单。

`test:preflight` 会先运行 `pnpm run generate:docs`，自动补齐或刷新 Swagger、前端 OpenAPI 类型、契约基线和参考文档，然后执行契约与质量门禁。因此它可以直接在干净检出上运行；生成物可能更新当前工作树。`test:ci-local` 在同一生成步骤后立即运行 tracked 生成物漂移检查，未提交的契约差异仍会硬失败。

测试文件名必须描述被验证的业务行为，不得包含任意大小写或分隔形式的 `coverage`，也不得使用 `c95`、`c_98` 等覆盖率数字缩写。`pnpm run check:test-names` 扫描当前全仓文件；历史违规文件已经全部改为行为名称，`scripts/test-name-allowlist.txt` 当前没有豁免项。检查器仍从 merge-base Git 文件树按当前规则推导历史上限，不信任基准提交里的旧清单，因此规则扩展后可纳管此前遗漏的文件，也不能通过“新增违规文件并写入清单”绕过门禁。

`pnpm run check:test-quality` 使用 Go AST 识别 `testing.T` 失败调用、testify 断言和仓内断言 helper 调用。它会报告全仓所有未识别到断言的 `Test*`，但只对相对 merge-base 新增的缺口硬失败；普通函数调用、`Sleep`、`Skip` 或启动 goroutine 不再被当作断言。确实以“不 panic”、helper process 退出等效果作为契约的测试，必须在 `scripts/go-test-quality-exemptions.json` 中按文件、测试函数和具体理由登记；重复、理由过短或已经失效的条目都会失败。`report:test-quality` 保留为兼容入口，但执行同一硬门禁。

## 编写测试

- Handler 断言参数绑定、状态码、错误码和 response envelope；service 通过 fake 覆盖业务规则与失败语义。
- Store 使用临时数据库，覆盖 migration、旧数据归一、并发与重载；集成测试使用 mock server 或协议 fixture。跨出 Futu integration 边界的测试只能依赖 `internal/integration/futu/testkit` 的语义 fixture，不得直接 import OpenD codec 或生成 protobuf 包。
- yfinance sidecar 测试必须通过 ASGI transport 和 mock `yfinance` 覆盖成功/失败契约，并全局阻止 socket；普通 CI 不访问真实 Yahoo Finance。
- 用例必须断言业务结果、状态迁移或可观察副作用，而不只执行代码行。复杂 UI 和策略运行优先覆盖分支、拒绝路径和恢复路径。
- 无法用值或状态表达、只能以“不 panic”等执行效果验证的 Go 用例必须登记带理由的测试质量例外，不得依赖任意函数调用骗过检查。
- 真实网络、账户、交易和行情权限只出现在显式 live workflow；没有该环境时，不得以 `skip` 充当生产验证结论。
