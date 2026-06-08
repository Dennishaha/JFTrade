# 配置

JFTrade 的运行时配置默认位于 `var/jftrade-api/settings.json`。首次启动会生成管理员密钥和默认配置。

## 常用环境变量

- `JFTRADE_API_BIND`：API 监听地址。
- `JFTRADE_GUI_BIND`：发布态 GUI 监听地址。
- `JFTRADE_GUI_API_BASE_URL`：发布态 GUI 注入的 API 基地址。
- `JFTRADE_SETTINGS_PATH`：运行时配置文件路径。
- `JFTRADE_BACKTEST_DB`：回测数据库路径。

## OpenD 集成

OpenD 连接配置位于 `settings.json` 的 `integration.config` 中，常用字段包括 `host`、`apiPort`、`websocketPort`、`tradeMarket` 和 `securityFirm`。
