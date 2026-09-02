# SQLite 查询计划与索引审计

> 历史审计：本文保留迁移前 Go store 的查询计划、路径和候选索引证据。
> 当前 SQLite owner 与 schema gate 以 Rust store、迁移事实源和 closeout manifest 为准；本文不参与当前状态计算。

更新时间：2026-07-29。

本文记录 P3-2 对 9 个受管 SQLite 数据库的生产查询、现有索引和 `EXPLAIN QUERY PLAN` 结论。结论以实际 SQL 谓词、排序、调用频率、数据增长和写放大为依据，不以表名或单列 `WHERE` 猜测索引。

## Schema 边界

当前 catalog 管理以下数据库：

| database id | 当前版本 | 开发态文件 |
|---|---:|---|
| `backtest` | 2 | `backtest.db` |
| `backtest-runs` | 1 | `backtest-runs.db` |
| `strategy` | 2 | `strategy-runtime.db` |
| `execution-orders` | 5 | `execution-orders.db` |
| `adk` | 2 | `adk.db` |
| `adk-session` | 4 | `adk-session.db` |
| `adk-artifact` | 1 | `adk-artifact.db` |
| `watchlist` | 1 | `watchlists.db` |
| `research` | 1 | `research.db` |

catalog 当前声明 45 个显式索引，并严格比较 metadata、表、列、主键、外键、索引、view 和 trigger。`ValidateCurrentFile` 是只读 preflight；仓库没有增量 schema migration。任何 index 增删都会让已有文件与 current manifest 不一致，不能只改 `catalog_statements.go` 或放宽校验。

## 已落地的无 schema 优化

### ADK confirmation lookup

`ApprovalByConfirmationCallID` 原查询只有 expression equality，没有写出 partial index 的 predicate，SQLite 无法证明查询满足 partial index 条件。查询现在显式包含：

```sql
WHERE COALESCE(json_extract(payload_json, '$.confirmationCallId'), '') <> ''
  AND json_extract(payload_json, '$.confirmationCallId') = ?
```

计划从 scan 变为：

```text
SEARCH adk_approvals USING INDEX idx_adk_approvals_confirmation_call (<expr>=?)
```

### ADK audit 过滤与分页

API 过去先读取并反序列化全部 `adk_audit_events`，再在 Go 过滤和分页。现在 store 在 SQL 中执行 `kind`/`subject_id` 过滤、`COUNT(*)`、`ORDER BY created_at DESC, id ASC`、`LIMIT/OFFSET`；非分页调用不额外执行 count。响应排序、total 和超范围 offset 语义保持不变。

### Watchlist 分组列表

分组查询过去从 `watchlist_instruments` 扫描，再用 correlated `EXISTS` 检查 membership。现在从隐式主键 `(group_id, instrument_id)` 驱动：

```sql
FROM watchlist_memberships member
JOIN watchlist_instruments i ON i.instrument_id = member.instrument_id
WHERE member.group_id = ?
  AND member.instrument_id > ?
ORDER BY member.instrument_id
```

分页 cursor、market/query 过滤、instrument 排序与 hydration 语义不变；计划不再扫描全部 instrument 或建立临时排序。

### Execution event 恢复

启动恢复过去按全局 `created_at,id` 排序，与现有 `(internal_order_id, created_at, id)` 索引不匹配。内存只按 `internal_order_id` 分桶消费事件，因此改为：

```sql
ORDER BY internal_order_id ASC, created_at ASC, id ASC
```

每个订单内的事件时序和 sequence 恢复保持不变，计划命中 `idx_execution_order_events_order` 且不再使用临时 B-tree。

## 逐库结论

| 数据库 | 主要查询计划 | 决策 |
|---|---|---|
| `backtest` | 动态 K 线表使用 `WITHOUT ROWID` 和 `PRIMARY KEY(end_time)`；范围、边界、倒序 latest 都走主键 | `keep/no-op` |
| `backtest-runs` | status/updated list 与清理由现有索引覆盖；清理预览的 `SUM(LENGTH(request_json)+LENGTH(result_json))` 必须读取大 JSON | `query/data-model follow-up`：持久化 payload bytes，索引不是瓶颈 |
| `strategy` | definition/version/instance 查询已覆盖；runtime log/audit 实际总带 `instance_id`，单列 `level`/`kind` 索引不能覆盖真实过滤与排序 | `replace-index after migration` |
| `execution-orders` | event 恢复已改写；seen-fill retention 按 `created_at` scan，但只在启动/设置变更清理且当前极小 | `query-rewrite` 已完成；seen-fill 先 benchmark |
| `adk` | workflow due、lease expiry、tool invocation expiry 已有匹配索引；confirmation 与 audit 下推已修复；approval/task 的 run 过滤及 audit subject-only 仍有增长风险 | `query-rewrite` 已完成；复合索引进入迁移候选 |
| `adk-session` | GO-ADK event Get/After 和 session cascade child probe 都 scan `events`，并为时间倒序建立临时 B-tree | **高风险已确认，受迁移政策阻断** |
| `adk-artifact` | version、Load/Delete、scope list 均由 `(app_name,user_id,session_id,file_name,version)` 主键前缀覆盖；scope list 只为 `DISTINCT file_name` 做小范围临时去重 | `keep/no-op` |
| `watchlist` | membership 双向查询由 PK/反向索引覆盖；分组 list 已改写；origin source/group 删除和 import-run 实际排序仍不匹配 | `query-rewrite` 已完成；两个索引进入迁移候选 |
| `research` | 主键、name unique、created list 均正确命中 | `keep/no-op` |

## 已确认但暂不直接增加的索引

### P0：`adk-session.events`

GO-ADK v2 的真实读取是：

```sql
WHERE app_name = ? AND user_id = ? AND session_id = ?
  [AND timestamp >= ?]
ORDER BY timestamp DESC
[LIMIT ?]
```

当前主键为 `(id, app_name, user_id, session_id)`，首列 `id` 使上述查询和 `ON DELETE CASCADE` 的 child probe 都无法按 session 搜索。最小正确索引是：

```sql
CREATE INDEX idx_adk_session_events_session_time
ON events(app_name, user_id, session_id, timestamp DESC);
```

同构数据库验证后，普通 Get、After+limit 和 cascade 都从 `SCAN events` 变为 session prefix `SEARCH`，读取不再使用临时排序。开发库审计时只有 4 个 session、54 个 event，规模尚小，但 Runner 每次执行、approval/input resume、projection 和 compaction 都会读取整段 session history，因此这是结构性高风险，不以当前行数降级。

当前不直接落地的原因是 schema 政策，而不是索引收益不确定：把 session v4 直接升到 v5 会让所有现有 v4 文件在只读 preflight 被判 incompatible，现有恢复路径是备份后重建并丢弃原始 session context。仓库在 2026-07 已明确移除 legacy/incremental migration，本专项不能隐式恢复自动升级政策。

如获准单独落地，必须：

1. 保留精确 v4 manifest，并增加只识别 `adk-session 4 → 5` 的迁移；
2. 只读验证完整 v4 后，以 `IMMEDIATE` 事务再次核对版本，原子执行 `CREATE INDEX` 和 conditional metadata update；
3. commit 后重新按 v5 strict manifest 验证；
4. 对未知版本、schema drift 和损坏文件保持 byte-for-byte 不变；
5. 覆盖磁盘满、DDL/update/commit fault、取消、双进程并发、重复迁移和完整数据保留。

### 其余迁移候选

| 优先级 | 候选 | 依据与约束 |
|---|---|---|
| P1 | `adk_approvals(run_id,status)` | pending approval persistence/resume 按 run+status；当前只有 status index |
| P1 | `adk_tasks(run_id,updated_at DESC,id)` | run 过滤与 session 删除会随全任务表增长；status/agent 索引不覆盖 run-only |
| P1 | strategy log `(instance_id,level,at_ms DESC,id DESC)` | 30 万行 synthetic：分页约 `1.393ms → 0.028ms`，count `2.160ms → 0.032ms` |
| P1 | strategy audit `(instance_id,kind,at_ms DESC,id DESC)` | 与真实 instance+kind 查询和排序一致；应替换而不是叠加无效单列索引 |
| P1 | watchlist origin `(source_id,remote_group_id)` | 30 万行 synthetic 的来源清理约 `12.108ms → 0.030ms` |
| P1 | watchlist import global/source created cursor indexes | 现有 `(source_id,run_id)` 不覆盖 `created_at DESC,run_id DESC`；30 万行来源分页约 `5.902ms → 0.024ms` |
| P2 | `adk_audit_events(subject_id,created_at DESC,id)` | subject-only 过滤仍 scan；先记录真实 filter 分布 |
| P2 | `execution_seen_fills(created_at)` | 30 万行清理约 `9.679ms → 0.096ms`，但插入约增加 22%，当前仅少量数据且清理低频 |

所有 synthetic 数字只用于比较同一驱动、同一数据分布下的计划，不作为生产延迟承诺。新增索引前仍要测 1K/100K/1M、状态偏斜、写吞吐、WAL 和文件增长。

## 删除或治理候选

- strategy 的单列 `level`、`kind` 索引没有匹配生产查询；在复合索引落地时替换，避免重复写放大。
- execution 的 `updated_at`、broker order/ex 索引当前没有生产 SQL 消费者；只能在全仓调用和外部兼容边界再次确认后，于版本迁移中删除。
- execution quote/watch preview 的 expiry 索引存在，但当前没有对应生命周期清理；先补清理策略，再决定保留。
- watchlist preview expiry 同样是“有索引、无清理消费者”，不是继续加索引可以解决的问题。
- backtest-run 清理检查的主要成本是巨大 JSON payload 扫描。审计样本只有 25 个 run，但 JSON 约 288MB，字节估算约 0.41 秒；应缓存/持久化 payload bytes 或把估算推迟到显式预览。

## 实测与回归口径

2026-07-29 对 `var/jftrade-api` 的只读样本：

- 9 个 catalog 数据库均可识别；其余 6 个业务库的 `quick_check` 与 `foreign_key_check` 全部通过。
- `backtest-runs.db` 的体积主要来自少量大 result JSON，而不是行数。
- `adk-session.db` 实测确认 event Get 为 scan；文件历史删除留下大量 freelist page，不以 VACUUM 掩盖查询计划问题。
- `adk-artifact.db` 当前无 artifact，结论另由同构数据和上游真实 SQL 验证。

查询计划回归应断言“命中目标索引、无全表 scan/无可避免的 temp B-tree”，同时验证业务排序、cursor、total、过滤和错误语义。不要只用一次毫秒 benchmark，也不要在真实开发库执行 `ANALYZE` 改变持久状态。
