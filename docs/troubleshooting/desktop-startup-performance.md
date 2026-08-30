# 桌面启动验证

桌面开发启动现在由 Tauri 2 原生 watcher 管理。旧版 Wails/Node supervisor 的原生 bundle 缓存和启动耗时数据不再适用于当前构建链。

## 当前开发入口

```bash
pnpm run dev:desktop
```

Tauri 会监督 Rust API、Vite 和受管 runtime 三个进程阶段：

1. Rust product API：`jftrade-api-rust`，默认 sidecar `127.0.0.1:3008`；
2. background：Vite dev server `127.0.0.1:3003`；
3. primary：Tauri `jftrade-desktop` 窗口。

修改 Rust 文件时由 Tauri 重新构建并重启 primary app；修改 Vue/TypeScript 文件时由 Vite HMR 处理。开发启动会通过 `build:pineworker:dev` 准备外部 PineTS worker，不会把 Go API 作为运行时前置。

## 发布启动验证

发布版必须先显式准备输入，再构建和打包：

```bash
pnpm run prepare:desktop-release
JFTRADE_DESKTOP_PREPARED=1 VERSION=1.2.3 \
  COMMIT="$(git rev-parse HEAD)" \
  BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  go tool wails3 package
```

验证重点是 `bin/` 中的官方产物、应用 bundle 的签名/版本信息、API ready 日志、端口释放，以及缺少显式运行时配置时没有隐式构建行为。
