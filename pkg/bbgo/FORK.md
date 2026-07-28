# `pkg/bbgo` fork 说明

本目录是 JFTrade 内嵌并持续维护的 BBGO 子集，不是 Go 的 `vendor/`
目录，也不是 BBGO 仓库的完整镜像。JFTrade 改写了包路径，裁掉了未使用的
交易所、应用运行时和存储实现，并在保留的类型上继续演进。因此，不能用
`go get`、整目录覆盖或普通 dependency bot 直接升级这里的代码。

任何修改本目录上游基线、导入的上游补丁或本地差异的变更，都必须在同一个
提交中更新本文档。

## 上游基线

| 项目 | 值 |
| --- | --- |
| 上游仓库 | <https://github.com/c9s/bbgo> |
| 上游版本 | `v1.64.2` |
| 上游 commit | [`816670adaa14e95d61697d2c2a81975fd90fdff3`](https://github.com/c9s/bbgo/commit/816670adaa14e95d61697d2c2a81975fd90fdff3) |
| 上游 commit 时间 | `2026-04-14T04:40:57Z` |
| Go module 摘要 | `h1:OR2i7RUi80BZViADbX8dRyPB5t1JJQ3DKoiP2QaGupM=` |
| 本地引入提交 | `ed27afd325361191f591f9f3fef5395e4762f4c7` (`Vendor used bbgo components`) |
| 许可证 | `AGPL-3.0-only`；归属与派生代码声明见 `docs/legal/third-party-notices.md` |

这个 commit 不是根据相似代码猜出的，证据链如下：

1. 本地引入提交 `ed27afd3...` 的父提交在 `go.mod` 中直接依赖
   `github.com/c9s/bbgo v1.64.2`，并在 `go.sum` 中记录了上表的模块摘要。
2. GitHub 的官方 tag ref `refs/tags/v1.64.2` 指向
   `816670adaa14e95d61697d2c2a81975fd90fdff3`；该提交信息中的 GitHub
   verification 状态为 `verified`。
3. 将本地引入提交中的导入路径
   `github.com/jftrade/jftrade-main/pkg/bbgo` 还原为
   `github.com/c9s/bbgo/pkg` 后，最初导入的 178 个文件中有 177 个能映射到
   `v1.64.2/pkg/`：122 个逐字节一致，55 个是有本地修改的上游文件。剩余的
   `service/backtest.go` 是从上游 `pkg/service/backtest_db.go` 抽出的
   `BackTestable` 窄接口。

可用以下命令重新核验版本和 tag 指向（`go mod download` 可能填充本机 module
cache，但不会修改工作树）：

```bash
git show ed27afd325361191f591f9f3fef5395e4762f4c7^:go.mod | rg 'github.com/c9s/bbgo'
git show ed27afd325361191f591f9f3fef5395e4762f4c7^:go.sum | rg 'github.com/c9s/bbgo v1.64.2'
go mod download -json github.com/c9s/bbgo@v1.64.2
curl -fsSL https://api.github.com/repos/c9s/bbgo/git/ref/tags/v1.64.2
```

## 路径映射与初始本地修改

通常，本地 `pkg/bbgo/<relative-path>` 对应上游
`pkg/<relative-path>`。以下差异从引入时就存在，必须在同步时保留并重新审查：

| 本地路径或范围 | 相对 `v1.64.2` 的修改 | 原因与所有权边界 |
| --- | --- | --- |
| 所有保留文件 | 导入路径改为 JFTrade module；只复制 JFTrade 实际使用的文件 | 使该子集随 JFTrade 单 module 构建，并限制第三方代码面 |
| `backtest/exchange.go` | 将上游完整回测交易所缩减为行情查询、市场信息和 K 线数据源适配层 | 撮合、成交、资金曲线和风控由 JFTrade 的 `pkg/backtest` 负责，不能从上游整文件覆盖 |
| `bbgo/config.go` | 仅保留回测配置、账户余额和费率模型 | JFTrade 自己拥有配置契约和运行时装配 |
| `bbgo/environment.go` | 仅保留 session 注册及账户初始化 | JFTrade 自己拥有应用生命周期、资源关闭和后台任务 |
| `bbgo/notification.go` | 仅保留最小 notifier fan-out | 通知来源和传输由 JFTrade 服务层负责，不引入 BBGO 的 Slack/live-note 运行时 |
| `bbgo/session.go` | 将上游大型 session 缩减为市场缓存、last price、下单格式化及提交/撤单代理 | 实盘 session 生命周期、行情 demand、持久化和风控均由 JFTrade 业务 service 负责 |
| `exchange/factory.go`, `exchange/util.go` | 移除 BBGO 自带交易所构造器；保留空 registry 和通用属性读取；`IsMaxExchange` 固定为 false | 本项目只从自己的集成层注册 Futu，不携带上游中心化交易所实现 |
| `service/backtest.go` | 从上游 `service/backtest_db.go` 只抽取 `BackTestable` 接口，不复制 SQL/service 实现 | SQLite K 线存储属于 JFTrade 的存储边界 |
| `types/exchange.go` | 增加 `ExchangeFutu` 和支持列表项 | Futu 是 JFTrade 的主要交易所适配器 |
| `types/account.go` 及部分 `types/*` | 移除 `viper` 等全局配置钩子及未使用依赖；保留 JFTrade 消费的数据模型 | 防止 vendored 类型反向拥有应用配置和基础设施 |
| `types/indicator.go` | 对本项目使用的 chart API 做兼容，并采用当前 Go 语法 | 保持现有图表/指标调用可编译；后续已继续拆分文件 |
| 其余初始差异文件 | 以 `interface{}`→`any`、整数 `range`、`maps.Copy` 和 lint 修正为主，也包含删除未使用 helper、隐藏临时 JSON 字段及小型兼容修复 | 对齐仓库 Go 版本并缩小运行时依赖；同步时必须逐项复核，不能把这些差异整体假定为无语义变化 |

引入时已经是“带补丁的精选子集”，而非干净的上游 tree。若需要逐行审计初始
差异，应从 `v1.64.2` 的 module source 与本地提交 `ed27afd3...` 比较，且先
还原上述 import 前缀；不要把 `ed27afd3...` 与上游 commit 当作可直接
`git rebase` 的两条同源历史。

## 引入后的本地改动台账

以下提交构成 `ed27afd3...` 之后、截至本文档创建时的本地 patch stack。
精确文件级差异以 Git 为准：

```bash
git log --reverse --stat ed27afd325361191f591f9f3fef5395e4762f4c7..HEAD -- pkg/bbgo
git diff --name-status ed27afd325361191f591f9f3fef5395e4762f4c7 HEAD -- pkg/bbgo
```

| 本地提交 | 修改范围 | 原因 |
| --- | --- | --- |
| `3e6f3ace3dc42f1f663f13b6c0775432568cd336` | `datatype/floats/slice_test.go` | 增大随机分布样本，降低发布构建中的概率性测试波动 |
| `f01bdb661a587ab26872562141b3bec20447a6b1` | 拆分 `fixedpoint/dec.go`、`types/indicator.go`；规范 market-store 和 mock 文件名 | 满足文件长度和文件名门禁；主要是代码搬移 |
| `0899fcf0b29450537850d5ae54b9ea0481e12f1f` | 若干既有长函数/测试的 lint 标注 | 让 vendored 上游形状通过本仓库 lint |
| `b01cdfdbc9f2b64ee25567bc95b29f850a27a728`, `da0c7fa52986cb9752dec75ec82b6a945b187a72`, `8f2bd62cc8e76454f5102f3b3073211b97745be0`, `9bb203c9f79840345496cd07cf39b3b34a82fac8`, `1cd2c87ee516b378e227dc3049a288c567546676` | price-volume JSON、position、RB tree、fixedpoint 解析、stream/deposit helper、indicator dot product | 将长函数拆成可测 helper 并通过 `funlen`；price-volume JSON 同时允许数组前有空白 |
| `ba16442c056916e5fe3303240d5a45bc0d9c41f6` | 删除未使用的 bool/string helpers、Slack 子包、component、mocks、skiplist、strint | 在后端边界收口后继续缩小 vendored 面，避免维护未使用实现 |
| `66e46490330549ba78ae1eea8ba8e68e5561f57f` | `types/interval.go` | 增加 Futu/JFTrade 所需的 `10m` 周期及截断规则 |
| `86eb81b80da170c9c5c6976c2583751fb7a3d528` | `types/stream.go`, `types/trade.go` 及测试 | 让 stream `Close` 幂等；区分单事件成交量与 provider 累计成交量，避免实时 K 线量柱污染 |
| `a8778020d884674eff9bd6700b670a1cc622d173` | `types/exchange.go`, `types/profit.go` | 删除 `ExchangeBasic` 兼容别名和废弃的 `ProfitStats.Init` |

当前 Git 历史没有记录任何“基线之后已导入的上游补丁 hash”。这不等于能证明
从未人工复制过相似修复；它表示目前不能把任何后续改动归因为某个上游
commit。今后移植上游修复时，提交信息和本表都必须记录上游 commit。

## 上游安全更新策略

`pkg/bbgo` 不再以 `github.com/c9s/bbgo` module 身份出现在当前 `go.mod` 中，
所以 Dependabot、`go list -m` 和基于 module version 的漏洞匹配不能替代人工
跟踪。执行以下规则：

1. 每月至少一次、每次 JFTrade release 前，以及收到 BBGO/GitHub 安全公告时，
   检查上游 [Security Advisories](https://github.com/c9s/bbgo/security/advisories)、
   [releases](https://github.com/c9s/bbgo/releases) 和基线之后的提交。
2. 只要公告或修复触及当前保留的包、这些包的行为契约，或其直接依赖，就创建
   跟踪 issue/PR。不能因为相关上游文件已被本地裁剪就直接判定“不受影响”；
   必须按调用路径确认。
3. 安全修复优先逐 commit 移植。提交信息使用
   `bbgo upstream <full-sha>: <reason>`，本文档台账记录上游 hash、受影响本地
   文件、是否有适配。选择“不受影响”时也应在跟踪 issue 中留下路径证据。
4. 普通功能更新不做整目录覆盖。只有准备完整重新基线时，才更新上表的版本和
   commit；单独移植补丁时基线保持不变，并把补丁记入台账。

检查基线之后的相关上游变更可使用隔离目录，不要给当前仓库添加永久 remote：

```bash
bbgo_sync_dir=$(mktemp -d)
git clone --filter=blob:none https://github.com/c9s/bbgo.git "$bbgo_sync_dir"
git -C "$bbgo_sync_dir" log --oneline \
  816670adaa14e95d61697d2c2a81975fd90fdff3..origin/main -- \
  pkg/backtest pkg/bbgo pkg/datatype/floats pkg/exchange pkg/fixedpoint \
  pkg/service pkg/style pkg/types pkg/util/templateutil
```

## 同步与重新基线流程

### 选择性移植安全修复

1. 在上面的隔离 clone 中阅读完整上游 commit 及测试，确认它是否依赖未 vendored
   的 BBGO 代码。
2. 将改动人工映射到本地路径，改写 import，并保留本页记录的 JFTrade 所有权
   边界。不要直接 `git cherry-pick` 不同仓库的提交，也不要恢复已删除的交易所、
   Slack、SQL service 或全局配置运行时。
3. 为受影响的本地行为补回归测试；执行下节验证。
4. 在本台账追加上游完整 hash、本地提交、涉及路径和适配说明。基线版本不变。

### 完整重新基线

1. 先选定不可变的上游 tag 和完整 commit hash，核对 tag ref、commit 签名状态与
   module checksum。
2. 在临时目录从新 commit 的 `pkg/` 重新构造“当前仍保留文件”的干净子集；
   import 重写后，逐项重放“初始本地修改”和“引入后的本地改动台账”。
3. 对上游新增、删除和改名逐项做保留/拒绝决策。任何扩大 vendored 范围或改变
   `types.Exchange`、订单、成交、K 线、stream、fixedpoint 语义的变更，都按
   实盘交易高风险变更审查。
4. 只有干净子集、新 patch stack、测试和许可证声明全部确认后，才替换本目录并
   更新本文档的基线、摘要、差异统计和台账。重新基线应独立提交，便于回滚和审计。

## 验证门禁

修改本目录至少执行：

```bash
go test ./pkg/bbgo/... -count=1
go test ./pkg/backtest/... ./pkg/futu/... ./internal/marketdata/... ./internal/strategy/... -count=1
pnpm run lint:go
pnpm run check:arch-deps
```

涉及公开接口、交易、成交、K 线、stream、fixedpoint 或重新基线时，合并前还要
执行仓库完整门禁：

```bash
pnpm run test:preflight
pnpm run test:ci-local
```

真实 Futu/OpenD 行为仍只在仓库规定的手动 live workflow 中验证，普通 CI 中
不得用跳过测试代替结论。
