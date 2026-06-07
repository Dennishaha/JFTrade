# 2026-06 发布收口 Review 边界

本文档只服务于这一次发布收口，目标是把当前工作树中的大改动拆成可以独立审阅的 4 块。

建议 review 顺序：

1. 依赖升级
2. Swagger 迁移
3. 通用 HTTP 收敛
4. broker 收敛

## 1. 依赖升级

### 改了什么

- `github.com/gin-gonic/gin` 升级到 `v1.12.0`
- 引入 `swaggo` 文档链路需要的依赖
- 同步 `go.mod` / `go.sum`

### 没改什么

- 业务 API 路径
- 鉴权模型
- 主响应 envelope 结构

### 外部可见行为

- `gin` 版本升级本身不作为单独功能对外宣传
- 影响主要体现在后续 HTTP 绑定收敛和 Swagger 生成链路可用

## 2. Swagger 迁移

### 改了什么

- 文档入口统一到 `/swagger/` 与 `/swagger/doc.json`
- 引入 `cmd/jftrade-api/docs.go` 中的 `go:generate`
- `docs/swagger/` 改为提交生成产物
- README 与前端开发代理同步到新路径

### 没改什么

- 业务接口本身的 URL 结构
- 运行时鉴权边界
- sidecar 对外服务端口策略

### 外部可见行为

- 旧 `/openapi.json` 不再作为文档入口
- Swagger UI 主入口为 `/swagger/`
- Swagger JSON 主入口为 `/swagger/doc.json`

## 3. 通用 HTTP 收敛

### 改了什么

- 增加统一的 `http_bindings.go`，把 URI / query / body 绑定类型集中管理
- 使用 `ShouldBindUri` / `ShouldBindQuery` 替换一批手写参数读取
- 路由组织收敛到新的 `router.go`
- 删除旧的手写 OpenAPI builder

### 没改什么

- 业务 handler 的核心读写语义
- 市场数据、策略、ADK、execution 等接口的主响应字段
- 无法自然映射到 gin 绑定的兼容归一逻辑

### 外部可见行为

- 大多数接口仅是实现层收敛，无新增对外功能
- Swagger 文档覆盖面提升，HTTP 参数行为更一致

## 4. broker 收敛

### 改了什么

- broker read-side query 改为 typed DTO + 显式转换
- broker write-side body 改为 API 自己的 DTO，再转换为 domain query
- broker 参数校验前移到 handler 层
- 补齐 broker 的 shape 校验和断连行为测试

### 没改什么

- broker 路径结构
- broker 鉴权方式
- broker 读接口在下游不可用时的 degraded/disconnected 响应风格

### 外部可见行为

- 缺必填 query、非法数值、非法 `scope` 现在直接返回 `400/BAD_REQUEST`
- 合法 query + broker 不可用时，仍返回 `200` 且 `connectivity=disconnected/degraded`

## Review 备注

- 本轮目标已经切换到“发布收口”，不再继续追求文件再拆分或进一步样板压缩。
- 如果 review 中发现问题，只接受发布阻断类修复：测试失败、文档错误、生成产物不一致、明显回归。
