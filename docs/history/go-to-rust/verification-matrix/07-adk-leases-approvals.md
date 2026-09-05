# 领域 7：ADK 审批与并发租约

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.7 领域 7：ADK 审批与并发租约（ADK Multi-DB Lease & Approval）

#### 2.7.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `internal/store/sqliteschema/catalog.go:27-35`
  - `internal/assistant/engine/persistence/session_sqlite.go`
  - `internal/assistant/engine/runner_approval.go:19-78`
- **关键符号**: `DatabaseADK`, `DatabaseADKSession`, `DatabaseADKArtifact`, `ResolveApproval()`, `claimApprovalContinuation()`
- **历史行为**:
  Go 基线物理拆分了三个数据库，在单个 Goroutine 内部顺序操作各库，未引入跨库 2PC。在审批并发控制上，Go 完全依赖内存锁 `approvalRuns: map[string]struct{}`。如果推理长时间阻塞，内存锁一直持有，进程内不会发生租约被窃；但进程崩溃后缺乏任何持久化的租约状态。

#### 2.7.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `crates/jftrade-store-sqlite/src/adk.rs:764-793, 1166-1270, 2028-2120, 2653-3040, 4155-4186`
  - `crates/jftrade-store-sqlite/src/adk_session.rs:86-100`
  - `crates/jftrade-store-sqlite/src/adk_artifact.rs:71-216`
  - `crates/jftrade-engine/src/product_production_ports_adk_mutation_runs.rs:35-60, 324-340`
  - `crates/jftrade-engine/src/product_adk_model_runtime.rs:313-419`
  - `crates/jftrade-engine/src/product_adk_model_runtime_recovery.rs:280-300`
- **关键机制**:
  1. **ATTACH 原生跨库提交与 WAL 拦截**: `adk.rs:4155` 中使用 `ATTACH DATABASE ?1 AS adk_session_events` 将 `adk-session.db` 附加至 `adk.db` 连接。为防崩溃原子性破损，代码在第 4178 行显式检测：若日志模式为 `wal`，立即报错拒绝，强制要求回滚日志模式。
  2. **两阶段审批唤醒补偿**: 阶段 1 在 `Immediate` 事务中将审批置为 `APPROVED`，run 置为 `RUNNING`（`resumeState = "approval_resuming"`）；阶段 2 调用 `runtime.resume_approval`；若失败，执行 `rollback_staged_approval` CAS 将审批回滚回 `PENDING`。
  3. **分布式租约与 Fencing Token**: 租约 TTL 为 30 秒，`RunLeaseGuard` 启动独立线程 `jftrade-adk-run-lease` 每 10 秒刷新一次 `expires_at_unix_ms`。每次租约被抢占时，`fencing_token` 单调递增。

#### 2.7.3 微观差异与破坏性边界失效推演
1. **跨三库会话删除孤儿数据泄漏 (P1-04 缺陷推演)**:
   - 审视 `delete_session`（`adk.rs:764-793`）：仅执行了对 `adk.db` 内部 `adk_runs`, `adk_sessions`, `adk_approvals` 等表的清理。
   - **失效后果**: `delete_session` **完全未连接 `adk-session.db` 与 `adk-artifact.db`**！被删除会话的所有聊天历史事件与大文件工件永久滞留在磁盘上，引发不可逆的空间泄漏。
2. **审批唤醒微秒级硬崩溃死锁悬挂**:
   - 在阶段 1 事务提交到阶段 2 任务调度之间遭遇 `SIGKILL`，补偿机制无法执行。
   - 数据库停留在 `approval_resuming`，恢复守护线程因租约未到期跳过，用户界面至少卡死 30 秒。
3. **慢推理 / 锁饥饿导致租约失窃与重复下单 (P1-03 缺陷推演)**:
   - 若 SQLite 遭遇长时间写锁排队，`RunLeaseGuard` 连续 3 次刷新心跳失败（超过 30 秒）。
   - `DurableRunRecoverySupervisor` 探测到租约过期，派发新 Worker 递增 `fencing_token` 接管。
   - 原 Worker 的 LLM 推理此时返回并试图执行工具发单：若工具调用的 CAS 因 `fencing_token` 失配被拒，而新 Worker 重新执行相同工具调用，外部券商将收到**两笔重复的报单请求**！

#### 2.7.4 Release Qualification 验证清单
- [ ] **RQ-ADK-01（正常流）**: 发起需审批的任务，审批状态由 `PENDING` $\to$ 调用 approve 接口 $\to$ 状态转为 `SUCCEEDED`，事件表记录完整。
- [ ] **RQ-ADK-02（崩溃注入）**: 在 `resolve_and_stage_approval` 提交后插入 panic 杀死进程，重启核验恢复线程在租约过期后接管并推进任务。
- [ ] **RQ-ADK-03（慢推理失窃 - 阻断门禁）**: Mock LLM 延迟 45 秒使租约心跳超时被抢占，核验原 Worker 返回时写库被 `fencing_token` 拦截，工具调用表通过唯一键阻断重复下单。
- [ ] **RQ-ADK-04（级联删除）**: 调用删除会话接口，核验 `adk-session.db` 的 events 表及 `adk-artifact.db` 的 artifacts 表对应记录均被彻底清除。
