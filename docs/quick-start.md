# 快速开始

本文只回答一个问题：你现在想跑哪一种入口。

## 开发态：控制台 + sidecar

这是最常见的开发方式。前端开发服务器在 `5173`，默认把 `/api` 和 `/swagger` 代理到 `3000`。

终端 1：

```bash
go run ./cmd/jftrade-api
```

终端 2：

```bash
npm install
npm run dev:web
```

访问入口：

- 控制台：`http://127.0.0.1:5173/`
- Swagger UI：`http://127.0.0.1:3000/swagger/`

## 开发态：只看文档站

```bash
npm install
npm run generate:docs
npm run dev:docs
```

VitePress 文档站默认在 `http://127.0.0.1:3001/`。如果前端开发服务器也在运行，则 `http://127.0.0.1:5173/docs/` 会代理到这个文档站。

## 桌面开发：JFTrade Dev

```bash
npm install
npm run desktop:dev
```

该命令同时启动 Vite 和 Wails `JFTrade Dev`。桌面 sidecar 默认监听 `127.0.0.1:6698`，数据仍写入仓库内 `var/jftrade-api/`。开发版与正式 `JFTrade` 的应用 ID、单实例 ID、窗口标题和端口相互隔离，可以同时运行。

## 本地一键验收

```bash
./start.sh
```

Windows CMD:

```cmd
start.cmd
```

这条路径会安装依赖、生成 Swagger、执行前端类型检查和构建，然后以 release-style 端口启动带内嵌前端的 sidecar：

- GUI：`http://127.0.0.1:6688/`
- API gateway：`http://127.0.0.1:6699/`

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

正式桌面构建必须提供准确的 `desktop-vX.Y.Z` tag。macOS 只生成 Apple Silicon ARM64 无签名 DMG：

```bash
JFTRADE_DESKTOP_RELEASE_TAG=desktop-v1.2.3 npm run desktop:release:darwin
```

Windows x64 无签名 per-user NSIS 安装器：

```powershell
$env:JFTRADE_DESKTOP_RELEASE_TAG = "desktop-v1.2.3"
npm run desktop:release:windows
```

Windows ARM64 使用同一命令的 `windows-arm64` 目标，生成标记为 preview 的无签名 per-user NSIS 安装器：

```powershell
$env:JFTRADE_DESKTOP_RELEASE_TAG = "desktop-v1.2.3"
npm run desktop:release:windows-arm64
```

正式产品默认监听 `127.0.0.1:6699`，数据写入系统用户数据目录，不复制仓库 `var/jftrade-api/`。平台产物、版本门禁和安全提示见 [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md)。
