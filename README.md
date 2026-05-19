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
  从用户本地 `~/Downloads/FTAPIProtoFiles_10.5.6508` 生成。
* 配置：`config/jftrade.yaml`（bbgo 标准格式，新增一个 `sessions: futu`）。

## 开发

```bash
# 生成 Futu protobuf（首次或 SDK 升级时）
./scripts/gen-futu-proto.sh

# 一键测试、构建并启动（macOS/Linux）
./start.sh

# 一键测试、构建并启动（Windows CMD）
start.cmd

# 启动后端（bbgo + futu 适配器）
go run ./cmd/jftrade run --config ./config/jftrade.yaml

# 启动前端
cd apps/web && npm install && npm run dev
```

## 测试

```bash
go test ./...
cd apps/web && npm run typecheck && npm run build
```

也可以直接运行根目录的 `start.sh` 或 `start.cmd`，按顺序执行测试、前端类型检查、前端构建，然后启动后端和前端预览服务。

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
packages/ui-contracts/  前端类型契约
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
