# Wails v3 桌面发布与通道隔离

桌面壳固定使用 Wails `v3.0.0-alpha2.117` 和 `@wailsio/runtime@3.0.0-alpha.97`。仓库脚本只调用 `go tool wails3`，不读取全局安装的 `wails3`。

## 开发版与产品版

`npm run desktop:dev` 是 `JFTrade Dev`，默认使用 `127.0.0.1:6698` 和仓库内 `var/jftrade-api`。正式 `release_assets` 产品是 `JFTrade`，默认使用 `127.0.0.1:6699` 和系统用户数据目录。两者的 bundle/product ID 与 SingleInstance ID 不同，可以同时运行；同一通道重复启动只恢复已有窗口。

| 属性                        | `JFTrade Dev`             | 正式 `JFTrade`                   |
| --------------------------- | ------------------------- | -------------------------------- |
| 编译条件                    | 默认构建                  | `release_assets` build tag       |
| Product / SingleInstance ID | `com.jftrade.desktop.dev` | `com.jftrade.desktop`            |
| 默认 API                    | `127.0.0.1:6698`          | `127.0.0.1:6699`                 |
| 数据目录                    | 仓库 `var/jftrade-api`    | 系统用户数据目录                 |
| 更新检查                    | 禁用                      | 每日后台一次，并支持菜单手动检查 |

桌面化没有迁移业务 API：Vue 前端仍直接访问 REST/OpenAPI、SSE 和业务 WebSocket。Wails bindings 只暴露 `DesktopLinkService`、`DesktopLogService` 和 `DesktopUpdateService`；bindings 由仓库脚本生成并提交，不维护手写方法 ID。

正式产品不会扫描、复制或移动开发数据。`desktop-state.json` 只写入正式产品数据目录。显式 `JFTRADE_API_BIND` 仍可覆盖端口，但端口已被占用时启动会返回 `API port conflict`，不会关闭或接管现有进程。

正式产品数据目录：

- macOS：`~/Library/Application Support/JFTrade`
- Windows：`%LOCALAPPDATA%/JFTrade`
- Linux：`${XDG_DATA_HOME:-~/.local/share}/jftrade`

正式 sidecar 只允许监听 loopback。开发通道保留现有显式覆盖能力；如果手工把两个通道配置成同一端口，后启动的一方会明确失败，另一方继续运行。

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

也可以从 Actions 的 `Desktop Release` 工作流手动输入已有的 `vX.Y.Z` tag；手动路径默认与 tag 推送一样发布 Release。勾选 `dry_run` 时仍会完成四个平台构建并保留 workflow artifacts，但不会写入 provenance 或修改 GitHub Release。相同 tag 的发布会串行执行，避免重复上传同一组 assets。直接在 Releases 页面创建或发布 Release 不会触发构建。

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

## CI 无签名发布

`.github/workflows/desktop-release.yml` 从准确的 `vX.Y.Z` tag checkout 并构建：

- macOS：固定使用 `macos-15` ARM64 runner，仅构建 Apple Silicon ARM64，生成文件名带 `macos-arm64-unsigned` 的 DMG，不再生成 x86_64 或 Universal 产物，也不执行 Developer ID 签名、公证或 staple。
- Windows：生成文件名带 `unsigned` 的 x64 per-user NSIS，不执行 Authenticode 签名。
- Linux x64：应用二进制仍使用 GTK3；共享输入与 Linux 构建任务同时安装 GTK4/WebKitGTK 6.0，供 Wails `doctor` 和 task runner 使用，并安装 GTK3/WebKitGTK 4.1 供实际应用编译与链接。

平台 job 通过内部环境变量 `JFTRADE_DESKTOP_PREPARED=1` 使用共享输入，并会在编译前拒绝缺失或空的 Swagger、前端压缩包和 Pineworker bundle。该变量只供 CI 使用；本地 `desktop:build` / `desktop:release:*` 仍会完整准备所需资产。

普通 CI 的 `Desktop Build` 矩阵会复用 Web 与 Pine job 生成的资产，在原生 runner 上构建 Linux x64、macOS ARM64 和 Windows x64 应用。各平台验证二进制格式、目标架构和 Go 构建元数据，Linux 额外检查动态库解析，最终仍由 required check `Build & Test` 汇总门禁。

发布流程不需要任何 Apple 或 Windows 证书 secrets。未签名的 macOS 包会触发 Gatekeeper 的“无法验证开发者”提示，Windows 包可能触发 SmartScreen 提示。发布任务仍上传各平台 SPDX JSON SBOM 和 `SHA256SUMS`；GitHub artifact attestation 仅在公开仓库写入 provenance，因为 GitHub 不支持用户所有的私有仓库持久化该类 attestation。

Windows ARM64 会在原生 `windows-11-arm` runner 上生成带 `preview` 标记的无签名 per-user NSIS 安装器，作为独立 asset 进入 GitHub Release。该 runner 当前处于 GitHub public preview。

当前主要产物名：

- macOS：`JFTrade-X.Y.Z-macos-arm64-unsigned.dmg`
- Windows：`JFTrade-X.Y.Z-windows-x64-unsigned-setup.exe`
- Windows ARM64 预览：`JFTrade-X.Y.Z-windows-arm64-preview-unsigned-setup.exe`

macOS DMG 只包含 ARM64 `JFTrade.app`，不包含 Rosetta/x86_64 slice。CI 固定运行在 `macos-15` ARM64 runner，并在构建前检查 runner 架构。

## 验收要点

- 同时运行 `npm run desktop:dev` 和正式产品：6698、6699、窗口、托盘、日志和退出生命周期互不影响。
- 分别二次启动两个通道：只聚焦同通道已有窗口，不启动第二个 sidecar。
- 开发版继续读取仓库数据；正式产品只读取系统用户数据目录。
- 退出任意一方，另一方继续运行。
- macOS 用 `file`/`lipo` 确认仅 ARM64；Windows 确认 x64 与 ARM64 per-user NSIS 都可安装覆盖。
- 未签名包出现 Gatekeeper 或 SmartScreen 提示属于当前发布策略的预期行为。
