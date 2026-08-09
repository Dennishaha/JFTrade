---
name: API sidecar 编排边界守卫
description: "当你修改 internal/app/apiserver、internal/api 或实时行情控制平面时使用。"
applyTo: "internal/app/apiserver/**,internal/api/**"
---

# API sidecar 编排边界守卫

`internal/app/apiserver` 是 composition root；`internal/api/*` 是 transport。两者都不能重新承载领域业务状态机。

## 强约束

- handler 只做绑定、校验、service 调用、错误映射和 DTO 转换。
- servercore 只装配 service/store/integration/runtime，优先使用 `application`、`stores`、`runtimes`、`marketdataapp` 的窄句柄。
- 订阅 demand、freshness、回退轮询属于 `internal/marketdata`；OpenD 协议映射属于 `internal/integration/futu` 或 `pkg/futu`。
- 对适配层的 `ErrNotSupported` 返回稳定、可诊断的 API 错误，不伪造成功。
- 资源先登记关闭函数再发布；失败按逆序回滚，关闭必须可重复且并发安全。

## 最小验证

```bash
go test ./internal/app/apiserver/... ./internal/api/... -count=1
pnpm run check:arch-deps
```
