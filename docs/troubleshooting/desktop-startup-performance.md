# 桌面启动验证

桌面开发启动现在由 Wails v3 原生 watcher 管理。旧版 Node supervisor 的原生 bundle 缓存和启动耗时数据不再适用于当前构建链。

## 当前开发入口

```bash
pnpm run prepare:desktop-dev
go tool wails3 dev -config ./build/config.yml -port 3003
```

Wails 会监督三个进程阶段：

1. blocking：`go tool wails3 build DEV=true`；
2. background：`go tool wails3 task common:dev:frontend`；
3. primary：`go tool wails3 task run`。

修改 Go 文件时由 Wails 重新构建并重启 primary app；修改 Vue/TypeScript 文件时由 Vite HMR 处理。`prepare:desktop-dev` 是生命周期外的显式 Pine worker 资产准备；开发启动本身不会自动构建、发现或选择 Pine/Python 运行时。

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
