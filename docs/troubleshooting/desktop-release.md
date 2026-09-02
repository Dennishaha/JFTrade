# Tauri 2 桌面构建与发布

JFTrade 的生产桌面壳是 Tauri 2，Rust engine 是唯一 API runtime。仓库没有 Go/Wails 源码、bindings、构建入口或运行产物；历史 fixture 只由 Rust replay 消费。

## 开发

```bash
pnpm install --frozen-lockfile
pnpm run dev:desktop
```

开发壳使用 `apps/desktop/src-tauri/tauri.dev.conf.json`，默认 API 为
`127.0.0.1:3008`，Vite 为 `127.0.0.1:3003`，数据目录为仓库内
`var/jftrade-api/`。`run-tauri.mjs` 会先准备 PineTS worker，再由 Tauri 管理
Vue dev server、Rust API 和受管 helper。修改 Vue/TypeScript 文件时由 Vite HMR
处理；修改 Rust 文件时由 Tauri 重新编译并重启。

## 发布

发布构建必须提供准确的 `vX.Y.Z` tag，并在目标平台执行：

```bash
JFTRADE_DESKTOP_RELEASE_TAG=v1.2.3 pnpm run build:desktop
```

`run-tauri.mjs build` 会先运行 `prepare:tauri-release`，准备前端、PineTS、
market-data 和受管 Node runtime 资产，再构建当前平台的 Tauri bundle。生产 API
默认监听 `127.0.0.1:6699`，正式产品数据写入系统用户数据目录；可选 Web 入口默认
`127.0.0.1:6688`，由桌面设置显式开启。

跨平台包由 `.github/workflows/desktop-release.yml` 的 Tauri CI matrix 生成。每个
平台 runner 必须独立准备对应的 market-data helper，执行 runtime manifest、安装、
启动、升级、卸载和回退 smoke；不要在本机脚本中交叉编译或启动另一个平台的 API。

## 资产与完整性

Tauri bundle 的受管 runtime 位于 `runtime/`，包括 Node、Node license、PineTS
worker、protobuf 和当前平台的 market-data helper。`prepare-tauri-release-runtime.mjs`
生成 `manifest.json` 及 SHA-256 列表，`pnpm run check:tauri-release-runtime` 在构建
和 smoke 前验证其完整性。缺少、损坏或摘要不匹配时，桌面启动 fail closed。

## 验证

```bash
pnpm run check:tauri-release-runtime
pnpm run test:tauri-release-runtime
pnpm run smoke:tauri-release
pnpm run check:rust
```

真实签名 updater、四平台安装/升级/回退、SBOM 和 post-release smoke 仍须在发布
workflow 中取得证据后才能关闭 release gate。不要把本地 `cargo` 构建或单平台 smoke
写成跨平台发布资格。

## 零 Go 发布边界

OpenAPI 从 `contracts/openapi/openapi.json` 生成，历史 Stage 2–9 fixture 只由 Rust replay 消费。`start.sh`、`build-release.*`、Tauri 开发和生产入口都执行零 Go 约束；bundle、candidate inputs 和 SBOM/provenance 还会扫描 Go build-info 与 Wails 组件。
