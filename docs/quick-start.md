# 快速开始

本文只回答一个问题：你现在想跑哪一种入口。

## 开发态：控制台 + sidecar

这是最常见的开发方式。前端开发服务器在 `5173`，默认把 `/api` 和 `/swagger` 代理到 `3000`。

终端 1：

```bash
go run ./cmd/jftrade api
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

## 完整策略/交易运行时

```bash
go run ./cmd/jftrade run --config ./config/jftrade.yaml
```

这条路径会进入 bbgo runtime，适合策略执行和交易链路验证。它不是前端控制台默认依赖的后端接口形态。

## 发布构建

```bash
./build-release.sh
```

Windows PowerShell:

```powershell
.\build-release.ps1
```

发布脚本会生成 API-only 发行版，并把前端和文档站一起打包到 `dist/`。
