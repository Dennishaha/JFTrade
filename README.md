# JFTrade (bbgo-plugin edition)

JFTrade 现以 [bbgo](https://github.com/c9s/bbgo) 为底，通过其对外抽象层注入 **Futu**
交易所适配器，不侵入 bbgo 源码。前端保留原 TradingView 风味的 Vue3 控制台。

## 架构

```
+---------------------------+         +-------------------------+
| apps/web (Vue3 + Vite)    |  HTTP   | bbgo cmd run (HTTP+WS)  |
| TradingView-flavored UI   | <-----> | pkg/server routes /api  |
+---------------------------+         +-----------+-------------+
                                                  |
                                                  v  registered at init()
                                         +--------+--------+
                                         | pkg/futu        |
                                         |  Exchange impl  |
                                         |  (bbgo Plugin)  |
                                         +--------+--------+
                                                  |
                                                  v protobuf over WS
                                          +-------+--------+
                                          | Futu OpenD     |
                                          +----------------+
```

* `cmd/jftrade/main.go` 直接调用 `bbgo cmd.Execute()`；通过空导入
  `_ "github.com/jftrade/jftrade-main/pkg/futu"` 在进程启动时把 `futu` 注册到
  bbgo 的 `exchange.Factory`。
* Futu OpenD 协议层（44 字节帧头 + protobuf）在 `pkg/futu/opend`。
* Futu protobuf 生成产物在 `pkg/futu/pb`，由 `scripts/gen-futu-proto.sh`
  从用户本地 `~/Downloads/FTAPIProtoFiles_10.5.6508` 结合仓库内
  `scripts/futu-proto-overlays` 补充协议生成。
* 配置：`config/jftrade.yaml`（bbgo 标准格式，新增一个 `sessions: futu`）。

## 开发

```bash
# 生成 Futu protobuf（首次或 SDK 升级时）
./scripts/gen-futu-proto.sh

# 一键测试、构建并启动（macOS/Linux）
./start.sh

# 一键测试、构建并启动（Windows CMD）
start.cmd

# 启动后端 API sidecar（开发控制台默认路径，不启动 bbgo 策略/交易流）
go run ./cmd/jftrade api

# 直接运行二进制（无参数默认启动 API 与内嵌前端，适合发布产物）
./jftrade

# Swagger 调试文档
# Swagger UI: http://127.0.0.1:3000/swagger/
# Swagger JSON: http://127.0.0.1:3000/swagger/doc.json
# 重新生成 Swagger 与参考文档（生成文件不提交入库）
npm run generate:docs

# 如需完整 bbgo 引擎，再显式启动 bbgo run
go run ./cmd/jftrade run --config ./config/jftrade.yaml

# 启动前端
npm install
npm run dev:web
```

## 生产部署

发布构建会把前端静态资源嵌入 Go 二进制。发布态默认从 `127.0.0.1:6688` 提供同源 GUI、`/api/v1/*`、SSE、WS、Swagger；`127.0.0.1:6699` 仍保留为 API 直连接口，方便脚本和排障使用。

```bash
# macOS / Linux
./build-release.sh

# Windows PowerShell
.\build-release.ps1

# Windows CMD 包装入口
build-release.cmd
```

发布脚本会依次完成：

1. 安装前端依赖。
2. 生成 Swagger 与参考文档。
3. 构建 `apps/web/dist`。
4. 暂存 `internal/frontendassets/dist/`，并生成压缩归档 `internal/frontendassets/dist.zip` 供发布标签嵌入，减少最终二进制体积。
5. 一次性输出以下平台产物到 `dist/`：macOS arm64、Linux amd64、Windows amd64、Windows arm64。

当前发布脚本固定构建 `cmd/jftrade-api`，也就是只保留 sidecar 与内嵌 GUI 的 API-only 发行版；发布产物名仍然保持 `jftrade`，但不再包含 bbgo `run` 模式和原有 CLI 命令面。以当前 darwin arm64 产物为例，发布二进制约 `55.6MB`。

当前发布构建已经使用 `-trimpath`、`-buildvcs=false`、`-tags 'release_assets,netgo,osusergo'`、`-ldflags '-s -w'`，并把嵌入前端改成 zip 归档再嵌入；同时发布入口固定收敛到 `cmd/jftrade-api`。以当前 darwin arm64 产物为例，发布二进制已从早期约 `79M` 降到约 `55.6MB`。如果仍需要完整 bbgo `run` 和原有 CLI 能力，继续通过源码入口 `go run ./cmd/jftrade run --config ./config/jftrade.yaml` 使用，不再作为当前发布包分发。

发布态二进制默认无参数即启动 API 网关与内嵌前端；首次启动会在二进制同级目录下自动生成 `var/jftrade-api/` 运行时目录，并写入默认 `settings.json` 与回测库文件。开发态仍保持 Vite `5173` + sidecar `3000` 的结构。

`settings.json` 现在支持配置 GUI / gateway 绑定地址。启动时优先级为：环境变量 > `settings.json` > 内置默认值。示例：

```json
{
  "interfaces": {
    "apiBind": "127.0.0.1:6699",
    "guiBind": "127.0.0.1:6688"
  },
  "integration": {
    "brokerId": "futu",
    "enabled": true,
    "config": {
      "type": "futu",
      "host": "127.0.0.1",
      "apiPort": 11110,
      "websocketPort": 11111,
      "maxWebSocketConnections": 20,
      "useEncryption": false,
      "websocketKey": "123456",
      "tradeMarket": "HK",
      "securityFirm": "FUTUSECURITIES"
    }
  }
}
```

可选环境变量：

- `JFTRADE_API_BIND`：API 监听地址，开发态默认 `127.0.0.1:3000`，发布态默认 `127.0.0.1:6699`
- `JFTRADE_GUI_BIND`：内嵌前端监听地址，仅发布态启用，默认 `127.0.0.1:6688`
- `JFTRADE_GUI_API_BASE_URL`：覆盖发布态 GUI 注入的 API 基地址；默认跟随 `JFTRADE_API_BIND`
- `JFTRADE_SETTINGS_PATH`：设置文件路径，开发态默认 `var/jftrade-api/settings.json`，发布态默认 `<binary-dir>/var/jftrade-api/settings.json`
- `JFTRADE_BACKTEST_DB`：回测数据库路径，开发态默认 `var/jftrade-api/backtest.db`，发布态默认 `<binary-dir>/var/jftrade-api/backtest.db`
- `JFTRADE_VERSION` / `JFTRADE_COMMIT` / `JFTRADE_BUILD_TIME`：覆盖发布脚本注入的构建元数据

## 测试

```bash
go test ./...
cd apps/web && npm run typecheck && npm run build
```

也可以直接运行根目录的 `start.sh` 或 `start.cmd`，按顺序执行测试、前端类型检查、前端构建，然后启动带内嵌前端的 Go 服务。

开发态后端 API 启动后，可直接打开 Swagger UI 进行调试：`http://127.0.0.1:3000/swagger/`。
发布态默认从 `http://127.0.0.1:6688/` 打开控制台，Swagger UI 与 Swagger JSON 分别位于 `http://127.0.0.1:6688/swagger/` 和 `http://127.0.0.1:6688/swagger/doc.json`；`6699` 仍可作为 API 直连地址使用。

前端 K 线实时合成与防回归说明见 `docs/frontend-kline.md`。

## 文档

开发时建议按下面顺序取上下文：

1. [docs/README.md](docs/README.md)：文档导航与 AI 协作约定
2. [docs/architecture.md](docs/architecture.md)：当前系统架构、双运行模式、职责边界
3. [docs/frontend-kline.md](docs/frontend-kline.md)：前端行情与 K 线入口
4. [docs/troubleshooting.md](docs/troubleshooting.md)：排障入口
5. [docs/reference/README.md](docs/reference/README.md)：协议与上游参考资料

## 目录结构

```
cmd/jftrade/        bbgo 包装入口
pkg/futu/           Futu 交易所适配器
  opend/            OpenD WebSocket + 44-byte frame
  codec/            帧编解码（与生成 proto 无关，可独立测试）
  pb/               生成的 Futu protobuf go 文件
config/             bbgo 运行配置
docs/               架构与设计文档
apps/web/           Vue3 前端（保留 TradingView 风味）
apps/web/src/contracts/  前端类型契约
scripts/            proto 生成等工具脚本
```

## 与旧版本的区别

| 维度 | 旧 JFTrade | jftrade-main |
|------|----------|--------------|
| 后端 | 自研 Go API + Worker | bbgo + Futu 插件 |
| Broker 抽象 | 自定义 Broker 接口 | bbgo `types.Exchange` |
| 持久化 | 自管 SQLite + 18 migrations | bbgo 内置 service 层 |
| 前端 | Vue3 + 自定义 API client | Vue3 + bbgo `/api/*` 适配 |
| Futu proto | 网络拉取 py-futu-api | 本地 FTAPIProtoFiles_10.5.6508 |
