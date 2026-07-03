# 交易开源项目参考指引

本文只做参考映射，不表示 JFTrade 复制这些项目的实现。实际开发必须以本仓库接口、测试和许可证复核为准。

## 推荐参考方向

| 项目 | 适合参考的点 | 在 JFTrade 中的落点 |
| --- | --- | --- |
| QuantConnect LEAN | 回测/实盘统一订单生命周期、明确撮合模型版本 | `pkg/backtest` execution model、result metadata |
| NautilusTrader | 事件驱动交易内核、订单/成交状态机 | `pkg/backtest/conservative_bar_executor.go`、后续 `pkg/execution` |
| vn.py | gateway 接入、券商适配分层、事件总线思路 | `pkg/broker`、`internal/trading`、Futu adapter |
| StockSharp | broker capability 与 connector conformance | broker fake/conformance tests |
| OpenBB | provider registry、数据源可发现、typed data contracts | `internal/marketdata` provider descriptor |
| Qlib | 研究/ML 数据集和实验管理 | 后续 ADK research workflow，不进入首批交易执行内核 |

## 当前取舍

- 先参考 LEAN/NautilusTrader 的执行正确性，不先做大规模 UI 或框架替换。
- 券商扩展先做 conformance fake，不急着接第二个真实券商。
- 行情扩展先做 provider descriptor，不引入外部 Python runtime。
- 开源工程化先补流程和检查，顶层许可证由项目方单独决定。

## 禁区

- 不复制上游项目代码片段。
- 不用参考项目的术语替代 JFTrade 已存在的 API 契约。
- 不为了“像某项目”而引入当前产品不需要的抽象。
- 不在未复核许可证前，把上游 license 结论写进发布材料。
