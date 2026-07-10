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
| 开发态 Web GUI                            | `127.0.0.1:5173`  | Vite dev server                                    |
| 开发态 JFTrade sidecar                    | `127.0.0.1:3000`  | 前端 `/api/v1/*`、SSE、WS                          |
| `JFTrade Dev` sidecar                     | `127.0.0.1:6698`  | Wails 开发窗口直接访问 `/api/v1/*`、SSE、WS        |
| 发布态 Web GUI                            | `127.0.0.1:6688`  | 内嵌前端、`/api/v1/*`、SSE、WS、Swagger 的同源入口 |
| 发布态 JFTrade gateway / 正式桌面 sidecar | `127.0.0.1:6699`  | API 直连与正式 Wails 产品前端访问入口              |
| Futu OpenD API                            | `127.0.0.1:11110` | Go 原生 TCP/protobuf 查询与探针                    |
| Futu OpenD WebSocket                      | `127.0.0.1:11111` | FTWebSocket / JavaScript API                       |

`start.sh` 浏览器式本地验收仍使用其原有 release-style 运行目录。正式 Wails 产品不会在二进制或安装目录旁生成 `var/jftrade-api/`：macOS 使用 `~/Library/Application Support/JFTrade`，Windows 使用 `%LOCALAPPDATA%/JFTrade`，Linux 使用 `${XDG_DATA_HOME:-~/.local/share}/jftrade`。`JFTrade Dev` 则继续读取仓库 `var/jftrade-api/`，两者之间不做数据迁移。

开发目录或正式产品数据目录中的 `settings.json` 都可以通过顶层 `interfaces` 字段覆盖默认监听地址；启动优先级是环境变量最高，其次 `settings.json`，最后才是 profile 默认端口。例如：

```json
{
  "interfaces": {
    "apiBind": "127.0.0.1:6699",
    "guiBind": "127.0.0.1:6688"
  }
}
```

## 快速检查

```bash
curl -fsS http://127.0.0.1:3000/api/v1/system/status
curl -fsS http://127.0.0.1:6698/api/v1/system/status
curl -fsS http://127.0.0.1:6688/api/v1/system/status
curl -fsS http://127.0.0.1:6699/api/v1/system/status
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:6698 -sTCP:LISTEN
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

- 不要写“bbgo server 起不来，所以前端断开”，应写清到底是开发态 sidecar 3000 消失，还是发布态 gateway 6699 消失
- 桌面问题还要区分 `JFTrade Dev` 的 6698 和正式 `JFTrade` 的 6699；不要把同通道单实例误判成两个通道互斥
- 不要把 `/api/v1/*` 说成 bbgo 自带接口
- 不要把 `start.sh` 的默认行为等同于所有运行方式；独立 API 入口在 [../../cmd/jftrade-api/main.go](../../cmd/jftrade-api/main.go)，桌面入口在 [../../cmd/jftrade-desktop](../../cmd/jftrade-desktop)
