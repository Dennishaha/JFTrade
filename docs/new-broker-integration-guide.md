# 新券商接入指南

> 状态：单实现扩展草案。当前生产实现只有 Futu/OpenD，未来 12 个月没有已承诺的第二 broker。Rust broker-neutral DTO 和 production ports 尚未通过第二个真实 broker 验证；接入时必须允许根据 conformance 结果调整内部接口，但不能暗改 `/api/v1/*`。

## 当前边界

- `crates/jftrade-broker`：broker-neutral 类型、能力和错误。
- `crates/jftrade-marketdata`：ProviderRouter、行情 demand/cache 与 runtime capability。
- `crates/jftrade-trading`：订单、风险和 execution 规则。
- `crates/jftrade-integration-futu`：当前 Futu/OpenD 协议和 I/O 实现参考。
- `crates/jftrade-engine`：adapter 生命周期、production ports、路由和唯一 owner 装配。
- `crates/jftrade-api`：公开 HTTP/SSE/WebSocket transport，不接触券商 SDK 类型。

## 接入步骤

1. 先列出目标券商真实支持的市场、行情、账户、下单、撤单、推送、重连和速率限制；未验证能力必须是 unavailable，不能返回伪数据。
2. 在独立 `jftrade-integration-*` crate 封装 SDK/协议、鉴权、连接和映射；SDK 类型不得进入领域或 API crate。
3. 映射到 `jftrade-broker`/`jftrade-marketdata`/`jftrade-trading` 的中立 DTO 和 port，保留 Decimal、时区、状态机和错误分类。
4. 在 `jftrade-engine` composition root 注册配置、secret、lifecycle、capability 和唯一 owner；切换失败保持旧 owner并 fail closed。
5. 只有 production port 已有真实实现时才暴露对应 route/tool/UI capability。OpenAPI 和前端类型变化必须从中立规范源生成。
6. 增加 fixture、mock transport、timeout/cancel、rate-limit、disconnect/reconnect、crash recovery 和重复请求测试；真实凭据只进入显式 live workflow。

## 必测 conformance

- capability 声明与实际 adapter 行为一致；
- symbol/market、价格数量精度、时区和交易状态规范化；
- 下单/撤单的幂等、事务、失败恢复和唯一 owner；
- push 顺序、generation fencing、断线重订阅和 stale callback 丢弃；
- Provider/账户不可用时返回明确 409/502/503，不静默换券商；
- secret 不进入日志、fixture、artifact 或前端；
- 关闭时 listener、socket、task 和子进程全部 cancel/join。

## 验证

```bash
cargo test -p jftrade-broker -p jftrade-marketdata -p jftrade-trading --all-targets
cargo test -p jftrade-engine -p jftrade-api --all-targets
pnpm --filter @jftrade/web run test
pnpm --filter @jftrade/web run typecheck
pnpm run check:generated
pnpm run check:rust
pnpm run check:zero-go
```

不要要求新券商复制 Futu 的全部协议面，不要绕过 canonical order status、risk gateway、approval 或 capability evaluation，也不要为新 adapter 恢复 Go/bbgo 运行时。
