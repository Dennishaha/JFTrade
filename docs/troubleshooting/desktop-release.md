# Wails v3 桌面构建与发布

桌面壳固定使用 Wails `v3.0.0-beta.8` 和 `@wailsio/runtime@3.0.0-beta.8`。CLI 由 Go toolchain 固定，调用方式是 `go tool wails3`，不依赖全局安装的 `wails3`。

## 构建事实源

构建入口完全采用 Wails v3 的 Taskfile 模型：

- 根 `Taskfile.yml` 负责 `build`、`package`、`run` 和 `dev` 分发；
- `build/Taskfile.yml` 负责 Vite、bindings、icons、Wails build assets 和发布输入校验；
- `build/config.yml` 的 `dev_mode.executes` 负责 blocking Go build、background Vite 和 primary `run`；
- `build/darwin/Taskfile.yml`、`build/windows/Taskfile.yml`、`build/linux/Taskfile.yml` 负责平台编译和官方打包工具。

常用命令：

```bash
go tool wails3 doctor
go tool wails3 dev -config ./build/config.yml -port 3003
go tool wails3 build
go tool wails3 package
go tool wails3 task --list-all
```

开发启动由 Wails watcher 管理，不再使用 Node supervisor、Vite 等待脚本、原生 app 缓存指纹或开发签名缓存。桌面开发前先运行 `pnpm run prepare:desktop-dev` 显式生成外部 Pine worker；Wails 启动本身不会自动构建、发现或选择 Pine worker、Python helper 或 frozen sidecar。

## 开发版与正式版

`go tool wails3 dev` 运行 `JFTrade Dev`，默认 API 为 `127.0.0.1:3008`，数据目录为仓库内 `var/jftrade-api/`。Wails 通过 `WAILS_VITE_PORT` 把 Vite 端口传给 `apps/web/vite.config.ts`，默认端口为 `3003`。

正式构建使用 `production,release_assets`，默认 API 为 `127.0.0.1:6699`，数据写入系统用户数据目录。发布版的 Pine、前端压缩包、Swagger 和 platform market-data helper 必须在构建前显式准备：

```bash
pnpm run prepare:desktop-release
export JFTRADE_DESKTOP_PREPARED=1
export VERSION=1.2.3
export COMMIT="$(git rev-parse HEAD)"
export BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
go tool wails3 build
go tool wails3 package
```

CI 使用同一个 `JFTRADE_DESKTOP_PREPARED=1` 校验开关，并从共享 artifact 下载前端、Swagger 和 Pine 输入；各平台 runner 独立构建和 smoke-test 自己的 Python helper。Wails build/package 任务不会隐式执行资产准备。

迁移阶段的 Tauri release rehearsal 使用独立入口，并且必须提供同一格式的正式 tag：

```bash
export JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3
export JFTRADE_DESKTOP_COMMIT="$(git rev-parse HEAD)"
export JFTRADE_DESKTOP_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
pnpm run build:desktop:tauri
```

launcher 会先拒绝 `dev`、`v0.0.0` 或非 `vX.Y.Z` tag，再准备 release runtime；通过后把同一版本注入 Rust build identity 和最终 Tauri bundle config。调用方附加的 `--config` 不能覆盖版本。该入口仍是迁移 rehearsal，签名、安装、升级、卸载、回退和跨平台 runtime smoke 全部通过 closeout gate 前，Wails 仍是唯一生产桌面 owner。

## 产物布局

所有 Wails 二进制和平台包写入 `bin/`：

- macOS：`bin/JFTrade`、`bin/JFTrade.app`、`bin/JFTrade-<version>-macos-arm64-<qualifier>.dmg`；
- Windows：`bin/JFTrade.exe`、`bin/JFTrade-<version>-windows-x64-<qualifier>-setup.exe`；
- Windows ARM64：`bin/JFTrade-<version>-windows-arm64-<qualifier>-setup.exe`；
- Linux：`bin/JFTrade`、`bin/JFTrade-<version>-linux-x64.AppImage`、`.deb` 和 `.rpm`。

`qualifier` 在没有签名凭据时为 `unsigned`，凭据完整配置时为 `signed`。构建目录 `var/wails-build/` 只保存本次任务生成的 Wails metadata、icons、manifest 和临时打包 staging。

## 单独打包

默认平台包：

```bash
go tool wails3 package
```

可选格式使用平台 Taskfile：

```bash
go tool wails3 task windows:package:msix
go tool wails3 task linux:package:appimage
go tool wails3 task linux:package:linux FORMAT=deb
go tool wails3 task linux:package:linux FORMAT=rpm
```

注意：Wails `v3.0.0-beta.8` 的官方 Taskfile 已包含 MSIX 任务，但该版本的
`wails3` CLI 仍尚未注册 `tool msix` 子命令。因此 beta.8 上的
`windows:package:msix` 暂不可执行；默认 Windows 发布路径仍使用 NSIS。待 Wails
修复或升级后，直接复用该 Task 即可，不要恢复旧的 Node 安装器包装层。

Windows NSIS 使用 Wails 生成的 `wails_tools.nsh` 和官方 `makensis` 调用；Linux AppImage、deb、rpm 使用 Wails generator/packager，AppImage 使用 Wails/linuxdeploy 的兼容默认压缩；macOS DMG 使用 `go tool wails3 tool package --format dmg`。仓库不再维护自定义 hdiutil DMG wrapper、Node NSIS 编译 wrapper 或 release orchestrator。

## 签名与验证

签名凭据必须全部配置或全部留空：

- macOS：`JFTRADE_MACOS_SIGN_IDENTITY`、`JFTRADE_MACOS_NOTARY_PROFILE`；
- Windows：`JFTRADE_WINDOWS_CERTIFICATE`、`JFTRADE_WINDOWS_CERTIFICATE_PASSWORD`。

没有凭据时仍执行 ad-hoc macOS bundle sealing，并生成带 `unsigned` 标记的包。配置完整时由 Wails `tool sign` 负责 macOS notarization 或 Windows Authenticode。

本地验证：

```bash
go tool wails3 task --list
pnpm run generate:wails-bindings
pnpm run check:wails-bindings
pnpm run test:scripts -- desktop
pnpm run check:quick
git diff --check
```

公开业务 API、SQLite schema、Wails bindings 签名和 `pkg/*` API 不随构建系统迁移改变。
