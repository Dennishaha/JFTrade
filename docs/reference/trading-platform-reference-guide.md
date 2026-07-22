# 交易开源项目参考指引

本文只做参考映射，不表示 JFTrade 复制这些项目的实现。实际开发必须以本仓库接口、测试和许可证复核为准。

## 推荐参考方向

| 项目 | 适合参考的点 | 在 JFTrade 中的落点 |
| --- | --- | --- |
| QuantConnect LEAN | 回测/实盘统一订单生命周期、明确撮合模型版本 | `pkg/backtest` execution model、result metadata |
| NautilusTrader | 事件驱动交易内核、订单/成交状态机 | `pkg/backtest/conservative_bar_executor.go`、`internal/trading` |
| vn.py | gateway 接入、券商适配分层、事件总线思路 | `pkg/broker`、`internal/trading`、Futu adapter |
| StockSharp | broker capability 与 connector conformance | `pkg/broker` capability catalog/router、后续 conformance tests |
| OpenBB | provider registry、数据源可发现、typed data contracts | `internal/marketdata` provider descriptor；出现第二个真实 provider 后再评估 registry |
| Qlib | 研究/ML 数据集和实验管理 | 后续 ADK research workflow，不进入首批交易执行内核 |

## 当前取舍

- 回测已使用显式 execution model；新增成交语义必须保持模型版本和结果元数据可追溯。
- broker capability catalog/router 已落地，下一步是补齐可复用的 connector conformance fake 和边界测试，而不是急着接第二个真实券商。
- 行情 provider descriptor 已落地；没有真实的第二 provider 前不增加 registry 抽象，也不引入外部 Python runtime。
- 顶层许可证已经确定为 AGPL-3.0。剩余开源治理工作以 `docs/roadmap.md` 中的贡献、安全和变更记录规范为准。

## 禁区

- 不复制上游项目代码片段。
- 不用参考项目的术语替代 JFTrade 已存在的 API 契约。
- 不为了“像某项目”而引入当前产品不需要的抽象。
- 不在未复核许可证前，把上游 license 结论写进发布材料。
