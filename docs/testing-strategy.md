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

## 分层执行

| 层 | 内容 | 触发方式 |
| --- | --- | --- |
| L0 静态与契约 | lint、vet、typecheck、架构依赖、OpenAPI/API types/Wails 生成一致性、许可证和测试命名 | 每个 PR、main |
| L1 单元与组件 | Go、Web、worker 的确定性测试及覆盖率/增量覆盖率 | 每个 PR、main |
| L2 隔离集成 | 临时 SQLite、`httptest`、mock OpenD/broker/Pine worker；禁止调用真实外部服务 | 每个 PR、main |
| L3 系统回归 | 完整确定性回归、release assets、真实 PineTS 进程 smoke；PR 只构建 Linux desktop，main 额外构建 Linux/macOS/Windows | PR / main |
| L4 重型验证 | race、并发重复、真实 PineTS backtest smoke；性能基线与真实 OpenD | nightly / manual |

`.github/workflows/ci.yml` 是 PR 与 main 的主门禁。PR 的 desktop lane 只做 Linux 原生 smoke build；main 的 desktop matrix 验证 Linux x64、macOS ARM64 和 Windows x64。nightly 在每天 02:00 Asia/Shanghai（GitHub cron 的前一日 18:00 UTC）运行 race、并发重复和真实 PineTS smoke。

每个覆盖 lane 会把命令输出及 Go/Web/worker 的 coverage 报告保存为 CI artifact（保留 7 天），并在对应 job summary 摘出总量和增量结果，便于定位门禁失败而不依赖本地复现。

真实 Futu/OpenD 不属于普通 PR 或 nightly：只能通过 `futu-live.yml` 手动触发，并调度到带 `self-hosted`、`futu`、`opend` 标签的 runner。该 workflow 显式设置 `JFTRADE_FUTU_LIVE_TEST=1`，并在未连通 OpenD 或权限不足时失败，绝不把跳过当作通过。性能基准保留手动触发；每周在同一 self-hosted runner 上把当前 main 与其上一提交比较。

## 本地入口

```bash
# PR 的确定性 L0-L2 门禁
pnpm run test:pr

# 本机可执行的 main 回归（当前平台 desktop + 真实 PineTS smoke）
pnpm run test:main

# race、并发重复与真实 PineTS backtest smoke
pnpm run test:nightly

# 单独运行三套覆盖率门禁
pnpm run test:coverage
```

本地分支若要启用增量覆盖，设置 base ref；CI 会自动提供它：

```bash
JFTRADE_DIFF_BASE=origin/main pnpm run test:coverage
```

测试文件名必须描述被验证的业务行为。新建或重命名的测试文件不得使用 `coverage_98`、`c95` 等覆盖率数字名称；`pnpm run check:test-names` 只检查相对 base 新增的测试文件，不要求为历史文件做无意义改名。

## 编写测试

- Handler 断言参数绑定、状态码、错误码和 response envelope；service 通过 fake 覆盖业务规则与失败语义。
- Store 使用临时数据库，覆盖 migration、旧数据归一、并发与重载；集成测试使用 mock server 或协议 fixture。
- 用例必须断言业务结果、状态迁移或可观察副作用，而不只执行代码行。复杂 UI 和策略运行优先覆盖分支、拒绝路径和恢复路径。
- 真实网络、账户、交易和行情权限只出现在显式 live workflow；没有该环境时，不得以 `skip` 充当生产验证结论。
