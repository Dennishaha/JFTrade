# 2026-06 发布收口说明

## 当前判断

当前代码基线可以作为发布候选收口：

- `go test ./...` 通过
- Swagger 已迁移到 `/swagger/` 与 `/swagger/doc.json`
- `gin` 已升级到 `v1.12.0`
- broker HTTP 绑定与参数校验收敛已完成

从现在开始冻结内容，不再继续做结构性优化。仅允许修复发布阻断项。

## 对外变化

本次对外需要明确说明的变化只有两类：

1. 文档入口调整
   - 旧入口：`/openapi.json`
   - 新入口：`/swagger/` 与 `/swagger/doc.json`
2. broker 读接口错误处理收紧
   - 缺必填 query、非法数值、非法 `scope` 不再返回 degraded 结果
   - 改为直接返回 `400/BAD_REQUEST`

以下内容不作为对外功能宣传：

- 新增的 HTTP DTO / 绑定类型
- 路由和 handler 的内部整理
- `gin` 升级本身带来的实现层变化

## 固定验收

发布前保留以下固定验收，执行顺序不要改：

1. `go test ./...`
2. Swagger 路径验证
   - `GET /swagger/`
   - `GET /swagger/doc.json`
3. broker 行为验证
   - 合法 query + broker 不可用时仍返回 disconnected/degraded
   - 缺必填 query / 非法数值 / 非法 `scope` 时返回 `400/BAD_REQUEST`
4. README 核对
   - `go generate ./cmd/jftrade-api`
   - `/swagger/`
   - `/swagger/doc.json`
   - 开发 / 发布端口说明

## 测试基线

按 2026-06-07 的当前工作树基线，以下验收已通过：

```bash
go test ./...
```

Swagger 和 broker 的关键行为由现有测试覆盖：

- `pkg/jftradeapi/swagger_openapi_test.go`
- `pkg/jftradeapi/broker_routes_new_test.go`
- `pkg/jftradeapi/broker_routes_read_exchange_test.go`

## 发布说明草稿

本次收口主要完成了三项工作：

- 将 API 文档链路切换到 Swagger UI `/swagger/` 与生成的 `/swagger/doc.json`
- 升级 `gin` 到 `v1.12.0`，并用统一的 gin 绑定类型收敛 HTTP 接入层样板
- 收紧 broker 读接口的参数校验，对缺必填参数和非法 query 返回明确的 `400/BAD_REQUEST`

## 冻结规则

- 不再继续 broker/router 拆文件
- 不再继续扩展 DTO 或做下一轮样板清理
- 只修：
  - 测试失败
  - 文档错误
  - 生成产物不一致
  - 明显行为回归
