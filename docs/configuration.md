# 配置

JFTrade 的运行时配置默认位于 `var/jftrade-api/settings.json`。首次启动后，运行时目录中通常会出现：

- `settings.json`
- `backtest.db`
- `secrets/admin.key`

## 配置优先级

对监听地址和部分运行时配置，当前优先级是：

1. 环境变量
2. `settings.json`
3. 内置默认值

如果你改了环境变量，再去编辑 `settings.json` 却发现不生效，先检查进程启动环境。

## 常用环境变量

- `JFTRADE_API_BIND`：API 监听地址。开发态默认 `127.0.0.1:3000`，发布态 gateway 默认 `127.0.0.1:6699`。
- `JFTRADE_GUI_BIND`：发布态 GUI 监听地址，默认 `127.0.0.1:6688`。
- `JFTRADE_GUI_API_BASE_URL`：覆盖发布态 GUI 注入的 API 基地址。
- `JFTRADE_SETTINGS_PATH`：运行时配置文件路径。
- `JFTRADE_BACKTEST_DB`：回测数据库路径。
- `JFTRADE_ADMIN_KEY`：直接通过环境变量提供管理员密钥。
- `JFTRADE_ADMIN_KEY_FILE`：从文件读取管理员密钥。

## `settings.json` 里的关键部分

### 监听地址

`settings.json` 顶层的 `interfaces` 可以覆盖默认绑定地址：

```json
{
  "interfaces": {
    "apiBind": "127.0.0.1:6699",
    "guiBind": "127.0.0.1:6688"
  }
}
```

### Futu / OpenD 集成

OpenD 连接配置位于 `integration.config`。常用字段包括：

- `host`
- `apiPort`
- `websocketPort`
- `websocketKey`
- `tradeMarket`
- `securityFirm`
- `useEncryption`

典型结构如下：

```json
{
  "integration": {
    "brokerId": "futu",
    "enabled": true,
    "config": {
      "type": "futu",
      "host": "127.0.0.1",
      "apiPort": 11110,
      "websocketPort": 11111,
      "websocketKey": "123456",
      "tradeMarket": "HK",
      "securityFirm": "FUTUSECURITIES"
    }
  }
}
```

## 管理员密钥

sidecar 默认要求管理员认证。首次启动时会生成 `secrets/admin.key`，浏览器登录和 CLI 调用都依赖这份密钥或其等价环境变量。

如果需要定位认证或设置保存问题，继续看 [troubleshooting.md](./troubleshooting.md)。
