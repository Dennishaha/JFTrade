# 启动与端口诊断

本文回答三个问题：

- 当前应该用哪种启动模式
- 各端口分别是谁在提供
- 为什么后端启动后会马上退出

## 启动模式

| 模式           | 命令                       | 适用场景                                   | 关键差异                                                      |
| -------------- | -------------------------- | ------------------------------------------ | ------------------------------------------------------------- |
| API sidecar    | `go run ./cmd/jftrade-api` | 前端开发、设置调试、行情调试、策略运行控制 | 启动 JFTrade `/api/v1/*` 控制台后端                           |
| Wails 桌面开发 | `npm run desktop:dev`      | 桌面壳、菜单、bindings、窗口与产品联调     | 启动 `JFTrade Dev`、Vite 和内置 sidecar，保留仓库数据路径     |
| Wails 正式产品 | `release_assets` 构建产物  | 日常桌面使用                               | `JFTrade` 独立单实例，使用系统用户数据目录和临时桌面 API 凭证 |

[cmd/jftrade-api/main.go](../../cmd/jftrade-api/main.go) 在进程入口会默认写入 `DISABLE_MARKETS_CACHE=1`，避免旧 market cache 影响 Futu market metadata。

## 默认端口

| 组件                                      | 默认地址          | 用途                                               |
| ----------------------------------------- | ----------------- | -------------------------------------------------- |
| 开发态 Web GUI                            | `127.0.0.1:3003`  | Vite dev server                                    |
| 开发态 JFTrade sidecar                    | `127.0.0.1:3000`  | 前端 `/api/v1/*`、SSE、WS                          |
| `JFTrade Dev` sidecar                     | `127.0.0.1:3008`  | Wails 开发窗口直接访问 `/api/v1/*`、SSE、WS        |
| 可选 Web 访问监听器                        | `127.0.0.1:6688`  | 端口可在设置中修改；桌面 Web 关闭时不创建，开启后提供前端、API、SSE、WS 和 Swagger |
| 正式 Wails 桌面 sidecar                    | `127.0.0.1:6699`  | 仅供正式 Wails WebView 无感访问，始终保持 loopback               |
| Futu OpenD API                            | `127.0.0.1:11110` | Go 原生 TCP/protobuf 查询与探针                    |
| Futu OpenD WebSocket                      | `127.0.0.1:11111` | FTWebSocket / JavaScript API                       |

`start.sh` 浏览器式本地验收仍使用其原有 release-style 运行目录。正式 Wails 产品不会在二进制或安装目录旁生成 `var/jftrade-api/`：macOS 使用 `~/Library/Application Support/JFTrade`，Windows 使用 `%LOCALAPPDATA%/JFTrade`，Linux 使用 `${XDG_DATA_HOME:-~/.local/share}/jftrade`。`JFTrade Dev` 则继续读取仓库 `var/jftrade-api/`，两者之间不做数据迁移。

开发目录或正式产品数据目录中的 `settings.json` 都可以通过顶层 `interfaces` 字段覆盖默认监听地址；启动优先级是环境变量最高，其次 `settings.json`，最后才是 profile 默认端口。例如：

```json
{
  "interfaces": {
    "guiBind": "127.0.0.1:6688"
  }
}
```

Wails 桌面的可选 Web 端口不使用 `interfaces.apiBind` 或 sidecar 端口，而由“设置 → Web 访问”的 `security.webPort` 控制。它允许 `1024`–`65535`，默认 `6688`，保存后立即切换。若提示 `WEB_ACCESS_LISTENER_UPDATE_FAILED` 或日志出现 `Web access port conflict`，原端口仍会继续服务；用 `lsof` 查占用进程或换一个空闲端口。

在 `JFTrade Dev` 中访问该端口时，UI 由 Gin 安全代理本机 Vite `3003`。如果返回 `502` 和“development UI is not available”，确认是用 `npm run desktop:dev` 启动，并检查 Vite 是否仍在监听；正式产品使用内嵌资源，不依赖 `3003`。

## 快速检查

端口是否在监听与 Web 是否已登录是两件事。桌面 Web 关闭时，`6688`（或用户端口）应直接连接失败；开启后，`401` 表示监听器已立即创建且需要 Web 登录。直接请求 `3008/6699` 不会降级为浏览器密码入口，没有桌面临时凭证时会返回 `403`。

```bash
curl -sS -o /dev/null -w '3000: %{http_code}\n' http://127.0.0.1:3000/api/v1/system/status
curl -sS -o /dev/null -w '3008: %{http_code}\n' http://127.0.0.1:3008/api/v1/system/status
curl -sS -o /dev/null -w '6688: %{http_code}\n' http://127.0.0.1:6688/api/v1/system/status
# 仅在排查正式 Wails 桌面产品时检查 6699
curl -sS -o /dev/null -w '6699: %{http_code}\n' http://127.0.0.1:6699/api/v1/system/status
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:3008 -sTCP:LISTEN
lsof -nP -iTCP:6699 -sTCP:LISTEN
lsof -nP -iTCP:6688 -sTCP:LISTEN
lsof -nP -iTCP:11110 -sTCP:LISTEN
lsof -nP -iTCP:11111 -sTCP:LISTEN
```

## 看到旧 full 模式日志

如果日志里还有：

```text
market info should not be empty, 0 markets loaded
```

这不是当前 API 入口会主动触发的路径。通常说明你运行的是旧二进制、旧分支、旧 VSCode 配置或旧脚本。先确认当前命令是 `go run ./cmd/jftrade-api`，并清理旧进程和旧构建产物，再回到上面的端口检查。

## FUTU_OPEND_ADDR 缺失或端口写错

当前默认值是 `127.0.0.1:11110`。如果 `FUTU_OPEND_ADDR` 缺失，`pkg/futu` 会回退到这个默认地址；如果你显式覆盖了错误端口，启动仍然会失败。

建议检查：

```bash
echo "$FUTU_OPEND_ADDR"
echo "$JFTRADE_FUTU_API_PORT"
```

## 需要避免的旧表述

- 不要写“bbgo server 起不来，所以前端断开”，应写清到底是开发态 sidecar 3000 消失，还是发布态同源服务 6688 消失
- 桌面问题还要区分 `JFTrade Dev` 的 3008 和正式 `JFTrade` 的 6699；不要把同通道单实例误判成两个通道互斥
- 不要把 `/api/v1/*` 说成 bbgo 自带接口
- 不要把 `start.sh` 的默认行为等同于所有运行方式；独立 API 入口在 [../../cmd/jftrade-api/main.go](../../cmd/jftrade-api/main.go)，桌面入口在 [../../cmd/jftrade-desktop](../../cmd/jftrade-desktop)
