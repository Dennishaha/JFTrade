# 配置

独立 API、浏览器开发态和 `JFTrade Dev` 的运行时配置默认位于仓库内 `var/jftrade-api/settings.json`。首次启动后，运行时目录中通常会出现：

- `settings.json`
- `backtest.db`
- `watchlists.db`

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

- `JFTRADE_API_BIND`：独立 API/Wails sidecar 监听地址。开发 API 默认 `127.0.0.1:3000`，`JFTrade Dev` 默认 `127.0.0.1:3008`，正式桌面产品默认 `127.0.0.1:6699`。
- `JFTRADE_GUI_BIND`：带内嵌前端的生产 HTTP 服务监听地址，默认 `127.0.0.1:6688`；该端口同时提供前端、API、SSE、WS 和 Swagger。
- `JFTRADE_GUI_API_BASE_URL`：历史兼容字段；单端口生产服务始终使用同源 API，不再依赖该覆盖。
- `JFTRADE_SETTINGS_PATH`：运行时配置文件路径。
- `JFTRADE_BACKTEST_DB`：回测数据库路径。
- `JFTRADE_WATCHLIST_DB`：本地自选主数据库路径。未设置时使用 `settings.json` 同目录下的 `watchlists.db`。

桌面 profile 由编译期 `release_assets` build tag 决定，不通过运行时环境变量猜测。Wails sidecar 始终只监听 loopback；显式 `JFTRADE_API_BIND` 只负责其内部端口，不再承担浏览器访问。可选 Web 入口使用独立监听器和用户设置的端口，因此公网开关不会改变 Wails sidecar 的边界。任一端口被占用时启动都会返回明确的端口冲突，不会结束或接管已有进程。

## `settings.json` 里的关键部分

### 监听地址

开发目录或正式产品数据目录中的 `settings.json` 可以通过顶层 `interfaces` 覆盖默认端口：

```json
{
  "interfaces": {
    "guiBind": "127.0.0.1:6688"
  }
}
```

带内嵌前端的独立生产后端仍使用 `guiBind`。`apiBind` 保留给独立 API 开发态和 Wails sidecar。Wails 桌面产品另从 `security.webPort` 读取可选 Web 端口：

```json
{
  "security": {
    "webAccessEnabled": false,
    "publicAccessEnabled": false,
    "webPort": 6688
  }
}
```

普通用户不需要直接编辑该文件，应在“设置 → Web 访问”中修改。端口允许 `1024`–`65535`，默认 `6688`。

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

## Web 访问与密码

JFTrade 默认仅供 Wails 桌面应用使用，普通用户不需要密码、Key 或额外配置。正式桌面产品每次启动会在内存中生成临时桌面能力凭证并自动注入当前 WebView；它不会持久化，也不需要用户查看或管理。

需要浏览器访问时，在桌面端打开“设置 → Web 访问”，确认或修改对外端口，设置不少于 15 个字符的 Web 访问密码并主动开启。后端只在 `settings.json` 中保存带随机盐的 Argon2id 单向校验值，读取设置的 API 不返回密码或校验值。启停 Web、改变端口或网络范围会立即创建、替换或释放监听器，同时使已有浏览器会话和长连接失效，桌面端不受影响。若新端口已被占用，保存会失败并保留原端口与原设置。

Web 关闭时，Wails 桌面产品不会创建浏览器监听器。开启后默认监听 `127.0.0.1:<webPort>`，仅供本机浏览器访问；打开“允许局域网/其他设备访问”后立即改为 `0.0.0.0:<webPort>`。当前内置服务只提供 HTTP，因此该选项仅适合可信局域网。通过互联网访问必须自行配置全程 HTTPS 反向代理，不能直接暴露 JFTrade 端口；同机代理应转发 `X-Forwarded-Proto: https` 和 `X-Forwarded-For`，JFTrade 只信任来自 loopback 的这些声明，并据此签发 `Secure` 会话 Cookie，以及执行访问范围和登录限速判断。

正式产品从内嵌前端资源提供 Web UI；`JFTrade Dev` 没有内嵌资源，因此可选 Web 监听器会代理同一开发命令启动的本机 Vite 服务（默认 `127.0.0.1:3003`）。`/runtime-config.js` 仍由 Gin 生成浏览器配置，不会把浏览器引向 Wails sidecar。开发代理只接受 loopback 目标。

Web 访问的启停、端口、改密和网络范围只能从可信桌面应用修改；浏览器设置页仅展示状态。旧版 `adminAuthRequired` 和 `secrets/admin.key` 不会迁移成 Web 密码，升级后 Web 保持关闭，应用会清理自己运行目录中的旧密钥文件。

如果需要定位认证或设置保存问题，继续看 [troubleshooting.md](./troubleshooting.md)。
