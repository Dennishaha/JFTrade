# 快速开始

本文只回答一个问题：你现在想跑哪一种入口。

开始前请安装 Node.js `>=22.13` 与 pnpm `11.21.0`。仓库只接受根目录 `pnpm-lock.yaml`，以下安装命令不会改写锁文件。

## 桌面开发（推荐）

```bash
pnpm install --frozen-lockfile
pnpm run dev:desktop
```

`pnpm run dev:desktop` 由 Tauri 管理 Vue dev server、Rust API 和受管 worker。开发壳 API 默认监听 `127.0.0.1:3008`，数据仍写入仓库内 `var/jftrade-api/`。桌面始终免登录；开发版与正式 `JFTrade` 的应用 ID、单实例 ID、窗口标题和端口相互隔离，可以同时运行。Pine worker 和 market-data helper 由发布准备流程提供，不把 Go API 作为启动前置条件。

## 可选：浏览器前端 + sidecar

这条路径仅用于纯浏览器前端开发。先在桌面端的“设置 → Web 访问”中设置密码并主动开启；前端开发服务器在 `3003`，默认把 `/api` 和 `/swagger` 代理到 `3000`。

终端 1：

```bash
cargo run -p jftrade-engine --bin jftrade-api-rust
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

这条路径会安装依赖、生成契约、执行前端类型检查和构建，然后启动带内嵌前端的单端口发布服务。Web 默认关闭；先从桌面端的“设置 → Web 访问”配置密码并开启后，可使用：

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

## Tauri 正式产品

正式桌面构建必须提供准确的 `vX.Y.Z` tag：

```bash
JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3 pnpm run build:desktop
```

正式产品的 Tauri 受管 Rust API 默认监听 `127.0.0.1:6699`，只供桌面 WebView 无感使用。用户主动开启 Web 后会立即创建默认 `127.0.0.1:6688` 的浏览器入口；端口可在“设置 → Web 访问”修改并立即切换。数据写入系统用户数据目录，不复制仓库 `var/jftrade-api/`。允许其他设备访问也会立即生效，且内置 HTTP 仅适用于可信局域网；互联网访问必须配置 HTTPS 反向代理。平台产物、版本门禁和安全提示见 [troubleshooting/desktop-release.md](troubleshooting/desktop-release.md)。
