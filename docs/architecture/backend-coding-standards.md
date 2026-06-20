# 后端代码规范

本文定义 JFTrade Go 后端的分层写法。目标不是追求形式化 DDD，而是让目录边界、测试和 CI 规则能持续阻止职责漂移。

## 分层职责

### `internal/api/*`

API transport 层只负责：

- 绑定和校验 URI、query、JSON body。
- 调用对应 service。
- 把 service 错误映射为 HTTP status、错误码和响应 envelope。
- 把业务 DTO 转成对外 JSON。

API 层不得：

- 直接访问 `internal/store/*`、SQLite、`database/sql`、Gorm、sqlx。
- 直接调用 `internal/integration/*`、Futu SDK、protobuf。
- 持有后台任务生命周期或业务状态机。
- 在 handler 内实现跨步骤业务编排；这类逻辑应下沉到 service。

### `internal/{system,settings,marketdata,trading,strategy,backtest,assistant}`

业务 service 层负责：

- 承载业务规则、状态转移、输入归一化、错误语义。
- 通过接口依赖外部能力，接口优先放在使用方包内。
- 使用 context 传递请求生命周期，但不依赖 HTTP transport 类型。

业务 service 层不得：

- import Gin、`net/http` handler/response 类型或 `internal/api/*`。
- 直接 import 具体数据库驱动、Gorm、sqlx、SQLite 实现。
- 直接 import Futu protobuf 或把协议模型暴露给调用方。

### `internal/store/*`

持久化层只负责：

- 文件、SQLite 或其他存储引擎的 schema、codec、query 和 migration。
- 把存储模型转换为 service 接口要求的业务模型。

持久化层不得：

- 调用 HTTP handler、Gin context 或 API response helper。
- 直接调用 broker SDK、OpenD、LLM provider 或后台 runtime。
- 包含业务状态机；复杂规则应在 service 中表达。

### `internal/integration/*`

集成层负责：

- 封装外部 SDK、协议、protobuf、网络客户端和 provider adapter。
- 把外部协议类型转换为 broker-neutral 或 service-defined DTO。

集成层不得：

- import `internal/api/*`。
- 复用 Gin handler、HTTP response envelope 或前端 JSON 细节。
- 拥有使用方业务状态，例如行情 demand、订阅 freshness、策略 runtime lifecycle。

### `internal/app/*`

应用装配层负责：

- 进程生命周期、配置落地、依赖组装、路由装配。
- 将 store、integration、service、transport 连接起来。
- 处理启动/关闭顺序和兼容运行模式。

应用装配层不得：

- 新增可下沉到 service 的业务分支。
- 把跨模块共享状态重新集中到一个全局对象中。
- 绕过 service 直接让 handler 访问 store 或 integration。

## 强制规则

- `scripts/check-arch-deps.sh` 是项目级结构保护线，负责检查 repo-specific import 方向。
- `.golangci.yml` 使用 golangci-lint v2.12.0 的 `standard` 基线，启用默认的 `errcheck`、`govet`、`ineffassign`、`staticcheck` 和 `unused`；生产代码与测试代码统一受这些规则约束，生成代码按严格生成标记排除。
- CI 另外运行 `go vet ./...` 和 `scripts/check-arch-deps.sh`，继续检查 Go 基础问题和项目特定的依赖方向。
- 新增模块时先选择最窄目录；只有需要被其他 Go module 复用的稳定能力才放入 `pkg/*`。
- 新增 shared helper 前先确认是否属于某个业务包；不要为了少量函数建立 `utils`、`common` 或大而全 helper 包。

## 测试要求

- Handler 测试验证参数绑定、状态码、错误码和响应 envelope，不启动真实 OpenD。
- Service 测试使用 fake store/provider/broker，覆盖业务分支和错误语义。
- Store 测试使用临时目录或临时数据库，覆盖 migration、旧数据归一和并发/重载场景。
- Integration 测试使用 mock server 或协议 fixture；真实外部依赖只能放在显式集成测试路径。
- 修改公开 API 时必须同步 OpenAPI 生成结果和快照。
