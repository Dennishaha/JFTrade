# Wails v3 桌面发布与通道隔离

桌面壳固定使用 Wails `v3.0.0-alpha2.117` 和 `@wailsio/runtime@3.0.0-alpha.97`。仓库脚本只调用 `go tool wails3`，不读取全局安装的 `wails3`。

桌面构建的事实来源是根 `Taskfile.yml`、`build/config.yml` 和三个平台 Taskfile。Node 入口只校验 tag、解析 Version/Commit/BuildTime 并调用 Wails task；平台资源、production flags、应用 bundle、NSIS、AppImage 和 Linux 包均由 Wails task/tool 生成。

`go tool wails3 build`、`go tool wails3 package` 和 `go tool wails3 task desktop:*` 都通过同一套任务工作；仓库继续保留 npm 别名作为日常入口。

## 开发版与产品版

`npm run desktop:dev` 是 `JFTrade Dev`，默认使用 `127.0.0.1:3008` 和仓库内 `var/jftrade-api`。正式 `release_assets` 产品是 `JFTrade`，默认使用 `127.0.0.1:6699` 和系统用户数据目录。两者的 bundle/product ID 与 SingleInstance ID 不同，可以同时运行；同一通道重复启动只恢复已有窗口。

| 属性                        | `JFTrade Dev`             | 正式 `JFTrade`                   |
| --------------------------- | ------------------------- | -------------------------------- |
| 编译条件                    | 默认构建                  | `production,release_assets`      |
| Product / SingleInstance ID | `com.jftrade.desktop.dev` | `com.jftrade.desktop`            |
| 默认 API                    | `127.0.0.1:3008`          | `127.0.0.1:6699`                 |
| 可选 Web 入口               | 用户设置，默认 `127.0.0.1:6688` | 用户设置，默认 `127.0.0.1:6688` |
| 数据目录                    | 仓库 `var/jftrade-api`    | 系统用户数据目录                 |
| 更新检查                    | 禁用                      | 每日后台一次，并支持菜单手动检查 |

桌面化没有迁移业务 API：Vue 前端仍直接访问 REST/OpenAPI、SSE 和业务 WebSocket。Wails bindings 只暴露 `DesktopLinkService`、`DesktopLogService` 和 `DesktopUpdateService`；bindings 由仓库脚本生成并提交，不维护手写方法 ID。

正式产品通过 Wails `production` tag 关闭 DevTools、调试 runtime 和开发资源代理，并统一启用 `-trimpath`、`-s -w`；Linux 额外使用 `gtk3`，Windows 使用 GUI subsystem。正式产品不会扫描、复制或移动开发数据。`desktop-state.json` 只写入正式产品数据目录。显式 `JFTRADE_API_BIND` 仍可覆盖端口，但端口已被占用时启动会返回 `API port conflict`，不会关闭或接管现有进程。

正式产品数据目录：

- macOS：`~/Library/Application Support/JFTrade`
- Windows：`%LOCALAPPDATA%/JFTrade`
- Linux：`${XDG_DATA_HOME:-~/.local/share}/jftrade`

正式 sidecar 只允许监听 loopback，也不会接受浏览器密码登录。可选 Web 入口是第二个 Gin HTTP 监听器：仅在用户已设置密码并主动开启时创建，默认也只监听 loopback；允许其他设备访问后才监听所有接口。启停、端口和网络范围保存后立即作用于监听器；新端口冲突时会保留原监听器和原设置。如果手工把 sidecar、Web 或两个通道配置成同一端口，后启动的一方会明确失败，另一方继续运行。

## 版本与本地验证

正式发布只接受 `vX.Y.Z`：

```bash
JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3 npm run desktop:release:darwin
```

`dev`、`v0.0.0`、分支名和其他 tag 都会被 release 脚本拒绝。版本、提交号和构建时间会同时注入 Go buildinfo、macOS Info.plist 和 Windows version resource。

推送 tag 会启动 `.github/workflows/desktop-release.yml`。工作流先集中生成一次 Swagger、前端压缩包和 Pineworker bundle，再把同一组输入交给四个平台构建；平台任务不再重复安装前端依赖或生成平台无关资产。`publish` 会等待四个平台任务结束，macOS、Windows x64 和 Linux 全部通过后创建或更新同名 GitHub Release，并上传二进制、SBOM 和 `SHA256SUMS`。Windows ARM64 是预览构建，失败不会阻塞这三套正式资产：

```bash
git tag v1.2.3
git push origin v1.2.3
```

也可以从 Actions 的 `Desktop Release` 工作流手动输入已有的 `vX.Y.Z` tag；手动路径默认与 tag 推送一样发布 Release。勾选 `dry_run` 时仍会完成四个平台构建并保留 workflow artifacts，但不会写入 provenance 或修改 GitHub Release。相同 tag 的发布会串行执行，重跑时使用本次构建结果覆盖同名 assets，无论 Release 当前是 draft 还是已发布状态。直接在 Releases 页面创建或发布 Release 不会触发构建。

开发构建与 bindings：

```bash
npm run desktop:dev
npm run generate:wails-bindings
npm run check:wails-bindings
```

常用验证命令：

```bash
npm run desktop:doctor
npm run check:desktop
npm run typecheck:web
```

扩展平台包按需生成，不进入默认 GitHub Release：

```bash
npm run desktop:package:windows-msix
npm run desktop:package:linux-appimage
npm run desktop:package:linux-deb
npm run desktop:package:linux-rpm
npm run desktop:package:linux-arch
```

## CI 发布与可选签名

`.github/workflows/desktop-release.yml` 从准确的 `vX.Y.Z` tag checkout 并构建：

- macOS：固定使用 `macos-15` ARM64 runner。无证书时仍执行 ad-hoc bundle sealing 和严格 codesign 校验；完整配置 secrets 时通过 Wails sign tool 执行 Developer ID 签名与公证。
- Windows：生成带 WebView2 bootstrapper 的 x64 per-user Wails NSIS；完整配置 secrets 时通过 Wails sign tool 对应用和安装器执行 Authenticode。
- Linux x64：使用 GTK3/WebKitGTK 4.1，默认同时生成裸二进制、AppImage 和 deb。

平台 job 通过内部环境变量 `JFTRADE_DESKTOP_PREPARED=1` 使用共享输入，并会在编译前拒绝缺失或空的 Swagger、前端压缩包和 Pineworker bundle。该变量只供 CI 使用；本地 `desktop:build` / `desktop:release:*` 仍会完整准备所需资产。

普通 CI 的 `Desktop Build` 矩阵会复用 Web 与 Pine job 生成的资产，在原生 runner 上构建 Linux x64、macOS ARM64 和 Windows x64 应用。各平台验证二进制格式、目标架构和 Go 构建元数据，Linux 额外检查动态库解析，最终仍由 required check `Build & Test` 汇总门禁。

签名采用“全部配置或全部不配置”：部分配置会立即失败，禁止静默降级。macOS secrets 为 `JFTRADE_MACOS_CERTIFICATE_BASE64`、`JFTRADE_MACOS_CERTIFICATE_PASSWORD`、`JFTRADE_MACOS_SIGN_IDENTITY`、`JFTRADE_MACOS_NOTARY_APPLE_ID`、`JFTRADE_MACOS_NOTARY_PASSWORD`、`JFTRADE_MACOS_NOTARY_TEAM_ID`；Windows secrets 为 `JFTRADE_WINDOWS_CERTIFICATE_BASE64` 和 `JFTRADE_WINDOWS_CERTIFICATE_PASSWORD`。完全未配置时仍发布带 `unsigned` 的产物，可能触发 Gatekeeper 或 SmartScreen 提示。

Windows ARM64 会在原生 `windows-11-arm` runner 上生成带 `preview` 标记的无签名 per-user NSIS 安装器，作为独立 asset 进入 GitHub Release。该 runner 当前处于 GitHub public preview。

当前主要产物名：

- macOS：`JFTrade-X.Y.Z-macos-arm64-unsigned.dmg`
- Windows：`JFTrade-X.Y.Z-windows-x64-unsigned-setup.exe`
- Windows ARM64 预览：`JFTrade-X.Y.Z-windows-arm64-preview-unsigned-setup.exe`
- Linux：`jftrade-desktop-linux-amd64`、AppImage 和 deb

证书启用后 macOS/Windows 文件名中的 `unsigned` 变为 `signed`。Windows MSIX 与 Linux rpm/Arch 包只提供显式 Wails task。

macOS DMG 只包含 ARM64 `JFTrade.app`，不包含 Rosetta/x86_64 slice。CI 固定运行在 `macos-15` ARM64 runner，并在构建前检查 runner 架构。

DMG 使用标准拖拽安装布局：左侧为 `JFTrade.app`，右侧为指向 `/Applications` 的文件夹快捷方式，背景箭头和说明文字引导用户将应用拖入 Applications。背景保留可审查的 SVG 矢量源，并在打包时生成 1320×800、144 DPI 的 Retina 2× PNG。发布任务会重新挂载 DMG，验证应用、快捷方式、背景分辨率和 Finder `.DS_Store` 布局都已写入。

## 验收要点

- 同时运行 `npm run desktop:dev` 和正式产品：3008、6699、窗口、托盘、日志和退出生命周期互不影响。
- 分别二次启动两个通道：只聚焦同通道已有窗口，不启动第二个 sidecar。
- 开发版继续读取仓库数据；正式产品只读取系统用户数据目录。
- 退出任意一方，另一方继续运行。
- macOS 用 `file`/`lipo` 确认仅 ARM64；Windows 确认 x64 与 ARM64 per-user NSIS 都可安装覆盖。
- macOS 必须通过 `codesign --verify --deep --strict`；Windows PE subsystem 必须为 GUI；Linux AppImage 必须可解包，deb 必须声明 GTK3/WebKitGTK 4.1 依赖。
- 未签名包出现 Gatekeeper 或 SmartScreen 提示属于当前发布策略的预期行为。
