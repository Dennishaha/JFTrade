# Futu 适配局部指令

- 这里只做 OpenD/bbgo 协议映射、codec、能力声明和传输生命周期钩子。
- demand、freshness、回退轮询、通知汇流、策略运行时和 UI 展示属于上层，不要下沉进 `pkg/futu`。
- 不支持能力必须显式返回 `ErrNotSupported`，不得伪造成功或静默吞错。
- 生成 protobuf 在 `pkg/futu/pb`，禁止手工修改；协议变化通过 generator 和 fixture 验证。
- 最小验证：`go test ./pkg/futu/... -count=1`、`pnpm run check:arch-deps`。
