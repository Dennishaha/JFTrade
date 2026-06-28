# JFTrade 维护者文档导航

这份 README 面向仓库维护者、协作者和后续 AI。它不重复介绍项目本身，只负责把你引到正确的事实来源。

如果只看一篇，请先看 [architecture.md](architecture.md)。

## 推荐阅读顺序

### 1. 先确认系统边界

- [architecture.md](architecture.md)：当前系统架构、单一 API 入口、请求链路和职责边界。
- [architecture/backend-coding-standards.md](architecture/backend-coding-standards.md)：后端分层约束、依赖方向和常见禁区。

### 2. 再按问题类型进入专题

- [troubleshooting.md](troubleshooting.md)：启动、端口、实时连接、OpenD、回测性能的排障入口。
- [adk.md](adk.md)：GO-ADK / Agent 控制面、权限模式、内置 tools 和运行时文件。
- [frontend-kline.md](frontend-kline.md)：前端行情与 K 线专题入口。
- [frontend/strategy-authoring.md](frontend/strategy-authoring.md)：策略定义、Logic Flow、Pine 编辑与 visual model 同步。
- [pinets-hardcut-migration.md](pinets-hardcut-migration.md)：PineTS 硬切替换 Go Pine runtime 的执行计划、进度、测试覆盖和性能门禁。
- [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)：PineTS worker 发布、运行配置、embedded asset 和非 mock smoke 放行清单。
- [reference/README.md](reference/README.md)：协议细节、OpenD 资料和上游参考。

### 3. 最后再看历史收口记录

以下文档保留为历史背景，不是当前默认入口：

- [review-boundaries-2026-06.md](review-boundaries-2026-06.md)
- [release-closeout-2026-06.md](release-closeout-2026-06.md)

## 快速路由

- 改启动方式、端口、运行时目录：先看 [architecture.md](architecture.md) 和 [troubleshooting/startup-ports.md](troubleshooting/startup-ports.md)
- 改前端默认接口、系统状态、设置：先看 [architecture.md](architecture.md)、[configuration.md](configuration.md)、[troubleshooting.md](troubleshooting.md)
- 改 ADK、agent、approval、provider、tools：先看 [adk.md](adk.md)
- 改实时行情、K 线、SSE、WS：先看 [frontend-kline.md](frontend-kline.md) 和 [troubleshooting/live-stream-connection.md](troubleshooting/live-stream-connection.md)
- 改 PineTS worker、worker pool、embedded asset、发布验收：先看 [pinets-hardcut-migration.md](pinets-hardcut-migration.md) 和 [troubleshooting/pinets-worker-release.md](troubleshooting/pinets-worker-release.md)
- 改 Futu / OpenD 协议和映射：先看 [reference/README.md](reference/README.md)

## 文档职责边界

- 根仓库 `README.md`：仓库级入口，回答“项目现在怎么跑”
- 本文档：维护者导航，回答“遇到这个问题先看哪篇”
- [index.md](index.md)：VitePress 用户文档首页，面向控制台使用者

不要把实现细节、长篇回归记录或协议原文继续堆回入口文档；它们应留在专题页或 reference 层。

## AI 协作入口

后续 AI 在动手前建议按下面顺序取上下文：

1. 读 [architecture.md](architecture.md)，先判断问题属于 sidecar、前端、Futu 集成还是底层 bbgo 公共能力。
2. 读对应专题页，而不是直接在根目录全仓库盲搜。
3. 只有需要协议原文或上游背景时，才进入 [reference/README.md](reference/README.md) 或 `reference/bbgo-doc/`。
