# 内嵌 Provider 数据源资格评估

本文是研究中心后续扩展的准入清单，不把尚未完成真实上游验证的推测写成能力承诺。当前阶段先稳定 yfinance/AKShare 已有能力；没有通过资格门槛的项目继续返回既定 409 或 `null`。

## 统一准入门槛

新增字段或能力必须同时具备：

- 字段含义、单位、币种、日期和时区可解释；
- 数据结构能满足页面需要，而不是只有一个当前点值；
- 在 sidecar 的 12 秒请求边界内完成，或能用现有缓存策略稳定完成；
- 至少用 US、HK、SH、SZ 的代表标的验证市场覆盖；
- 不需要账户秘密，不增加绕过授权或反爬的私有协议；
- 能用脱敏 fixture 固定 schema，并纳入显式 live smoke。

真实上游结果应通过手动 `marketdata-live.yml` 产生 JSON 报告后再更新本表。普通 pytest/CI 不访问真实网络。

## 候选项目

| 优先级 | 项目 | 当前状态 | 通过后实现顺序 | 未通过时的行为 |
| --- | --- | --- | --- | --- |
| P1 | 筛选结果行业 | 东财现货帧当前没有行业列，尚无稳定的批量补全证据 | sidecar 缓存/映射 → Go wire → 投影 → Web 行为测试 → 矩阵 | `industry: null` |
| P1 | AKShare 分析师目标价 | 当前研报聚合没有目标价列 | 先验证字段来源和更新时间，再按现有 analyst wire 增加可选值 | `target_price: null` |
| P2 | 估值时间序列与分位 | 当前 Provider 只有点值，不能支撑估值分位 | 需要带日期的历史序列、单位和最小样本数，再复用现有 `research.valuation` 409 边界 | 保持 409 |
| P2 | 卖空历史序列 | 当前没有覆盖 US/HK/SH/SZ 的稳定历史序列 | 需要市场覆盖、日期序列、单位和限流行为全部通过 live smoke | 保持 409 |
| P2 | 财经日历单位补全 | 百度日历当前把前值/预期/公布映射为字符串，百分号语义不稳定 | 只有上游明确提供单位时才扩展 wire；不得从数值大小推断百分号 | 保留当前字符串语义 |

## 实现约束

通过资格评估后，每个候选能力单独提交，顺序固定为：

1. sidecar 上游适配、错误分类和脱敏 fixture；
2. `internal/integration` wire/client/provider 适配；
3. `internal/productfeatures` facade 与纯函数投影；
4. 前端能力集合和行为测试；
5. 能力矩阵与 live smoke 更新。

优先复用现有通用 `broker.FeatureResult`，不为嵌入 Provider 建立第二套公开 HTTP 契约。若必须新增公开字段，先单独评审 OpenAPI 影响并运行 `pnpm run generate:docs`。
