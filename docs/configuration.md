# 配置

独立 API、浏览器开发态和 `JFTrade Dev` 的运行时配置默认位于仓库内 `var/jftrade-api/settings.json`。首次启动后，运行时目录中通常会出现：

- `settings.json`
- `backtest.db`
- `watchlists.db`
- `secrets/admin.key`

正式 Wails 产品使用独立的系统用户数据目录，不扫描、复制或移动仓库数据：

| 平台    | 默认目录                                   |
| ------- | ------------------------------------------ |
| macOS   | `~/Library/Application Support/JFTrade`    |
| Windows | `%LOCALAPPDATA%/JFTrade`                   |
| Linux   | `${XDG_DATA_HOME:-~/.local/share}/jftrade` |

正式产品的 `settings.json`、`backtest.db`、`watchlists.db`、策略、插件、日志和 `desktop-state.json` 都从该目录派生；安装目录保持无业务数据。

## 配置优先级

对监听地址和部分运行时配置，当前优先级是：

1. 环境变量
2. `settings.json`
3. 内置默认值

如果你改了环境变量，再去编辑 `settings.json` 却发现不生效，先检查进程启动环境。

## 常用环境变量

- `JFTRADE_API_BIND`：独立 API/Wails sidecar 监听地址。开发 API 默认 `127.0.0.1:3000`，`JFTrade Dev` 默认 `127.0.0.1:6698`，正式桌面产品默认 `127.0.0.1:6699`。
- `JFTRADE_GUI_BIND`：带内嵌前端的生产 HTTP 服务监听地址，默认 `127.0.0.1:6688`；该端口同时提供前端、API、SSE、WS 和 Swagger。
- `JFTRADE_GUI_API_BASE_URL`：历史兼容字段；单端口生产服务始终使用同源 API，不再依赖该覆盖。
- `JFTRADE_SETTINGS_PATH`：运行时配置文件路径。
- `JFTRADE_BACKTEST_DB`：回测数据库路径。
- `JFTRADE_WATCHLIST_DB`：本地自选主数据库路径。未设置时使用 `settings.json` 同目录下的 `watchlists.db`。
- `JFTRADE_ADMIN_KEY`：直接通过环境变量提供管理员密钥。
- `JFTRADE_ADMIN_KEY_FILE`：从文件读取管理员密钥。

桌面 profile 由编译期 `release_assets` build tag 决定，不通过运行时环境变量猜测。显式 `JFTRADE_API_BIND` 仍可覆盖默认端口；如果端口已被占用，桌面启动会返回明确的 `API port conflict`，不会结束或接管已有进程。正式产品的 sidecar 强制只监听 loopback。

## `settings.json` 里的关键部分

### 监听地址

开发目录或正式产品数据目录中的 `settings.json` 都可以通过顶层 `interfaces` 覆盖默认绑定地址：

```json
{
  "interfaces": {
    "guiBind": "127.0.0.1:6688"
  }
}
```

带内嵌前端的生产后端仅使用 `guiBind`。`apiBind` 仍保留给独立 API 开发态和 Wails sidecar。

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

启用 Futu 后，自选系统会注册稳定 source `futu:default`，用于只读发现和导入 Futu 分组。自选主数据始终保存在 `watchlists.db`，不会因禁用 Futu 或 OpenD 暂时离线而改用券商数据。详细边界见 [自选系统](./watchlist.md)。

## 管理员密钥

sidecar 默认要求管理员认证。首次启动时会生成 `secrets/admin.key`，浏览器登录和 CLI 调用都依赖这份密钥或其等价环境变量。

正式桌面产品还会在启动时生成临时桌面 API 凭证并注入当前 WebView；该凭证不改变纯 Web、REST/OpenAPI 或业务 WebSocket 契约，也不会写成需要用户管理的长期密钥。

如果需要定位认证或设置保存问题，继续看 [troubleshooting.md](./troubleshooting.md)。
