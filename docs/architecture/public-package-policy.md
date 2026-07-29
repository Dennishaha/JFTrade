# Go 公开包治理

`pkg/*` 表示可被其他 Go module 导入的稳定能力，不是“仓库内多处使用”的同义词。新代码默认放在 `internal/*`；只有稳定的外部复用意图，或者已被其他公开包的 API 暴露，才足以保留在 `pkg/*`。

## 当前决策

| 分类 | 包 | 决策依据 |
| --- | --- | --- |
| 稳定执行能力 | `pkg/futu`、`pkg/backtest`、`pkg/strategy` | 被 sidecar、策略 runtime 和回测交叉使用；`pkg/futu` 还实现 bbgo exchange 契约 |
| 上游 fork | `pkg/bbgo` | 通过 `pkg/bbgo/FORK.md` 管理基线、patch stack 和安全更新，不按普通业务包内移 |
| 共享契约 | `pkg/broker`、`pkg/market`、`pkg/researchscreen`、`pkg/observability` | 导出类型被 `pkg/futu`、`pkg/backtest` 或 `pkg/strategy` 直接使用；单独内移会让公开 API 暴露不可导入的 `internal` 类型 |
| 窄而高复用 helper | `pkg/chart`、`pkg/besteffort` | 生产调用面分别覆盖 15 和 68 个文件，合并到单一使用方会制造反向依赖或重复实现 |
| 仓库私有实现 | `internal/assistant/engine`、`internal/jftsettings` | 无外部 module 契约；已从 `pkg/adk` 和 `pkg/jftsettings` 硬切，不保留兼容转发包 |
| 旧门面 | `pkg/jftradeapi` | 已删除，HTTP 装配和业务边界分别归 `internal/app/apiserver`、`internal/api/*` 和 service |

`pkg/broker` 的保留不表示已验证多券商中立性。当前只有 Futu/OpenD 实现，保留它是为了稳定 adapter/capability DTO 和已有公开依赖；新增第二 broker 时仍必须用真实 adapter 重新验证抽象。

## 变更规则

1. 新增 `pkg/*` 必须记录预期的仓库外消费者或现有公开 API 约束。
2. 内移前必须先检查生产 importer 和导出签名；如果保留包的公开签名暴露该类型，应先设计 API 迁移，不能直接移入 `internal`。
3. 硬切不留空包或 type alias 兼容壳；仓内调用者同一变更中完成迁移。
4. 删除或内移已发布的 `pkg/*` 是 Go API 破坏性变更，必须在 release note 中明确说明。
5. `scripts/check-arch-deps.sh` 持续禁止已内移的旧 import 和旧目录回流，并精确校验当前顶层 `pkg/*` 集合；新增公开包必须先更新本页的复用依据和架构 allowlist。
