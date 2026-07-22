# JFTrade 用户文档

这里是 JFTrade 控制台的用户文档入口。

- 浏览器开发态：可以直接访问 `http://127.0.0.1:3001/`，也可以通过前端开发服务器的 `http://127.0.0.1:3003/docs/` 进入
- 浏览器式发布态：通过 GUI 同源路径 `/docs/` 访问
- Wails 桌面版：前端仍直接访问 HTTP/SSE/WebSocket API；桌面构建与数据目录说明见 [桌面发布与通道隔离](./troubleshooting/desktop-release.md)

## 从这里开始

- [快速开始](./quick-start.md)：启动开发态、文档站或发布构建。
- [配置](./configuration.md)：了解运行时目录、可选 Web 访问、端口绑定和 OpenD 集成。
- [使用指南](./usage.md)：按控制台功能区进入账户、行情、策略、回测和系统功能。
- [回测执行模型](./backtest-execution-model.md)：了解 `conservative-bar-v1` 的成交规则与实盘差异。
- [自选系统](./watchlist.md)：管理多分组收藏、使用工作台自选栏，并从 Futu 预览导入。
- [排障](./troubleshooting.md)：定位启动、端口、实时连接和 OpenD 问题。
- [桌面发布与通道隔离](./troubleshooting/desktop-release.md)：运行 `JFTrade Dev`、构建正式产品并理解数据、端口和单实例隔离。

## 专题参考

- [PineTS shadow engine](./pinets-shadow-engine.md)：外部 PineTS 影子引擎的实验模式和边界。
- [PineTS 契约审计](./pinets-contract-audit.md)：PineTS 切换后的执行契约、迁移兼容和 visual output 边界。

## 自动生成参考

- [HTTP API](./reference/generated/api.md)：从 Swagger 自动生成的接口参考。
- [数据类型](./reference/generated/types.md)：从前端契约自动生成的类型参考。
- [Pine v6 支持快照](./reference/generated/pine-v6-support.md)：从 `strategy.pine_spec` 自动生成的能力与限制矩阵。
- [JFTrade 开源许可](./legal/license.md)：AGPL-3.0-only 全文、无担保说明和对应源码义务。
- [第三方许可证](./legal/third-party-notices.md)：PineTS、BBGO 及其他上游组件的版权、许可与源码说明。

如果你是在维护仓库本身，而不是使用控制台，请回到 [README.md](./README.md) 和 [architecture.md](./architecture.md)；当前版本快照也维护在 [README.md](./README.md)。
