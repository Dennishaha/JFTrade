# 快速开始

本文只回答一个问题：你现在想跑哪一种入口。

开始前请安装 Node.js `>=22.13` 与 pnpm `11.12.0`。仓库只接受根目录 `pnpm-lock.yaml`，以下安装命令不会改写锁文件。

## 桌面开发：JFTrade Dev（推荐）

```bash
pnpm install --frozen-lockfile
pnpm run prepare:desktop-dev
go tool wails3 dev -config ./build/config.yml -port 3003
```

Wails 原生 `dev_mode.executes` 会依次监督 Go 开发构建、Vite 和 `run` 任务。桌面 sidecar 默认监听 `127.0.0.1:3008`，数据仍写入仓库内 `var/jftrade-api/`。桌面始终免登录；开发版与正式 `JFTrade` 的应用 ID、单实例 ID、窗口标题和端口相互隔离，可以同时运行。Pine worker 必须先通过 `prepare:desktop-dev` 显式生成；开发启动不会自动构建、发现或选择 Pine/Python 运行时。

## 可选：浏览器前端 + sidecar

这条路径仅用于纯浏览器前端开发。先在 `JFTrade Dev` 的“设置 → Web 访问”中设置密码并主动开启；前端开发服务器在 `3003`，默认把 `/api` 和 `/swagger` 代理到 `3000`。

终端 1：

```bash
go run ./cmd/jftrade-api
```

终端 2：

```bash
pnpm install --frozen-lockfile
pnpm run dev:web
```

Web 已开启后的访问入口：

- 控制台：`http://127.0.0.1:3003/`
- Swagger UI：`http://127.0.0.1:3000/swagger/`

## 开发态：只看文档站

```bash
pnpm install --frozen-lockfile
pnpm run generate:docs
pnpm run dev:docs
```

VitePress 文档站默认在 `http://127.0.0.1:3001/`。如果前端开发服务器也在运行，则 `http://127.0.0.1:3003/docs/` 会代理到这个文档站。

## 本地一键验收

```bash
./start.sh
```

Windows CMD:

```cmd
start.cmd
```

这条路径会安装依赖、生成 Swagger、执行前端类型检查和构建，然后启动带内嵌前端的单端口发布服务。Web 默认关闭；先从 `JFTrade Dev` 的“设置 → Web 访问”配置密码并开启后，可使用：

- 前端 + API：`http://127.0.0.1:6688/`

## 发布构建

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

发布脚本会生成 API-only 发行版，并把前端和文档站一起打包到 `dist/`。

## Wails 正式产品

正式桌面构建必须提供准确的 `vX.Y.Z` tag。macOS 只生成 Apple Silicon ARM64 无签名 DMG：

```bash
pnpm run prepare:desktop-release
JFTRADE_DESKTOP_PREPARED=1 VERSION=1.2.3 COMMIT="$(git rev-parse HEAD)" \
  go tool wails3 package GOOS=darwin GOARCH=arm64 QUALIFIER=unsigned
```

Windows x64 无签名 per-user NSIS 安装器：

```powershell
$env:JFTRADE_DESKTOP_PREPARED = "1"
$env:VERSION = "1.2.3"
go tool wails3 package
```

Windows ARM64 使用同一命令的 `windows-arm64` 目标，生成标记为 preview 的无签名 per-user NSIS 安装器：

```powershell
$env:JFTRADE_DESKTOP_PREPARED = "1"
$env:VERSION = "1.2.3"
go tool wails3 package GOARCH=arm64
```

正式产品的 Wails sidecar 固定监听 `127.0.0.1:6699`，只供桌面 WebView 无感使用。用户主动开启 Web 后会立即创建默认 `127.0.0.1:6688` 的浏览器入口；端口可在“设置 → Web 访问”修改并立即切换。数据写入系统用户数据目录，不复制仓库 `var/jftrade-api/`。允许其他设备访问也会立即生效，且内置 HTTP 仅适用于可信局域网；互联网访问必须配置 HTTPS 反向代理。平台产物、版本门禁和安全提示见 [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md)。
