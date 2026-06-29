# JFTrade 用户文档

这里是 JFTrade 控制台的用户文档入口。

- 开发态：可以直接访问 `http://127.0.0.1:3001/`，也可以通过前端开发服务器的 `http://127.0.0.1:5173/docs/` 进入
- 发布态：通过 GUI 同源路径 `/docs/` 访问

## 从这里开始

- [快速开始](./quick-start.md)：启动开发态、文档站或发布构建。
- [配置](./configuration.md)：了解运行时目录、管理员密钥、端口绑定和 OpenD 集成。
- [使用指南](./usage.md)：按控制台功能区进入账户、行情、策略、回测和系统功能。
- [排障](./troubleshooting.md)：定位启动、端口、实时连接和 OpenD 问题。

## 自动生成参考

- [HTTP API](./reference/generated/api.md)：从 Swagger 自动生成的接口参考。
- [数据类型](./reference/generated/types.md)：从前端契约自动生成的类型参考。
- [PineTS shadow engine](./pinets-shadow-engine.md)：外部 PineTS 影子引擎的实验模式和边界。
- [PineTS 契约审计](./pinets-contract-audit.md)：PineTS 切换后的执行契约、迁移兼容和 visual output 边界。
- [第三方许可证](./legal/third-party-notices.md)：AGPL PineTS / pinets 依赖、源码提供和许可证告知。

如果你是在维护仓库本身，而不是使用控制台，请回到 [README.md](./README.md) 和 [architecture.md](./architecture.md)。
