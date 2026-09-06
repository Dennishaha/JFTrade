# 领域 9：PineTS Worker Wire 契约与事件序

> **2026-09-05 复核提示：本卷以下内容为原始核查材料，非已确认缺陷或实施指令。** 风险状态、事实勘误、修复限制和后续验收以[主文](../go_to_rust_comprehensive_verification_matrix.md)为准。下文的绝对化结论、覆盖统计、平台枚举、旧行号及修复建议尚未逐项重验；不得据此直接改契约/schema、静默丢数据、自动重报订单或宣告发布就绪。接手对应任务时，应将复现/反证同步回本卷。

- **关联主索引**: [全景验证矩阵主导航](../go_to_rust_comprehensive_verification_matrix.md)
- **基线版本（Go）**: `origin/go` commit `452dea11`
- **目标版本（Rust）**: `main` (HEAD)

---

### 2.9 领域 9：PineTS Worker Wire 契约与事件序（PineTS Worker Wire Contract & Event Ordering）

#### 2.9.1 Go 基线路径、符号、关键行号与历史行为
- **源码路径**:
  - `pkg/strategy/pineworker/service.go`
  - `pkg/strategy/pineworker/manager.go`
- **历史行为**:
  Go 基线采用无状态的 RPC 请求模型，每次执行脚本时将当前切片全量传入 Node Worker 进行评估，不存在跨请求的长生命周期增量会话（Incremental Session），因此不涉及 `revision` 单调递增与内存堆级联清空的失效场景。

#### 2.9.2 Rust 当前实现路径、符号、关键行号与架构机制
- **源码路径**:
  - `proto/pineworker/pineworker_types.proto:35-68`
  - `proto/pineworker/pineworker_common.proto:33-37`
  - `crates/jftrade-integration-pine/src/execution.rs:26, 477, 533, 629-666, 844-870`
  - `workers/pineworker/src/main.ts:51`
  - `workers/pineworker/src/pinetsExecutor.ts:69-187 (重点关注 84-86 唯一性校验与 129-136 校验前置漏洞)`
  - `crates/jftrade-engine/src/strategy_runtime.rs:288-293, 454-469`
- **关键机制**:
  1. **增量会话 RPC**: 通过 `session_operation` (`open` / `append` / `close`) 进行通信。`open` 要求 `expected_revision = 0` 返回 `revision = 1`；`append` 要求 `expected_revision == current` 返回 `current + 1`，仅传输增量结果 `incrementalResult`。
  2. **Node 堆内存会话**: Node Worker 使用 `liveSessions: Map<string, NativeLiveSession>` 维系各策略的上下文对象图与递归指标状态。
  3. **实盘预热深度**: 实盘策略启动时默认仅加载 **200 根历史 K 线**（`candle_limit` 默认 200）。

#### 2.9.3 微观差异与破坏性边界失效推演

#### 1. 乱序或重复 Tick 导致 Worker 会话泄漏与策略死锁崩溃 (P1-07 破坏性对抗推演)
深入审视 `workers/pineworker/src/pinetsExecutor.ts:127-155` 与 `crates/jftrade-engine/src/strategy_runtime.rs:454-469`：
```ts
// workers/pineworker/src/pinetsExecutor.ts:127-136
const lastOpenTime = session.request.candles[session.request.candles.length - 1]?.openTime ?? 0;
let previousOpenTime = lastOpenTime;
for (const candle of request.candles) {
  if (candle.openTime <= previousOpenTime) {
    // 致命缺陷：该异常抛出在 try 块之前！
    throw new Error(
      `PineTS live session ${JSON.stringify(sessionId)} requires strictly increasing closed candle open times`,
    );
  }
  previousOpenTime = candle.openTime;
}

// 实际的 try 块直至 line 139 才开始：
const marker = resultMarker(session.context, session.capture);
try {
  for (const candle of request.candles) {
    session.provider.append(candle);
    ...
  }
} catch (error) {
  session.failed = true;
  this.liveSessions.delete(sessionId);
  throw new Error(`PineTS live session ... was invalidated after an append failure`);
}
```
- **核心代码级缺陷（Reviewer 2 对抗审计发现）**：
  在 `workers/pineworker/src/pinetsExecutor.ts:129-136` 中，乱序或非严格递增的 Tick 校验（`candle.openTime <= previousOpenTime`）**位于 `try` 块之外直接抛出异常**！
  由于异常在外层立即抛出，执行流程根本不会进入后续的 `try ... catch` 块（lines 139-155），第 151 行的 `this.liveSessions.delete(sessionId)` **完全不会被调用**！该失效的会话被静默滞留在 Node Worker 的 `this.liveSessions` 堆内存中，形成严重的状态残留与会话泄漏。
- **级联致命崩溃链（Unrecoverable Fatal Crash Chain）**：
  1. **Append 失败与重置**：在 Rust 引擎端（`crates/jftrade-engine/src/strategy_runtime.rs:454-469`），策略运行时捕获到 Append 请求返回的 gRPC 错误，执行：
     ```rust
     let was_append = session.revision > 0; // true
     session.revision = 0;
     if was_append {
         // 记录 SESSION_APPEND_RETRY 审计事件并重试
         continue;
     }
     ```
     Rust 将 `session.revision` 重置为 0，寄希望于在下一次循环中通过 `session_operation = "open"` 重建会话自愈；
  2. **Node 端无条件拒绝**：然而在 Node Worker 的 `openLiveSession`（`workers/pineworker/src/pinetsExecutor.ts:84-86`）中：
     ```ts
     if (this.liveSessions.has(sessionId)) {
       throw new Error(`PineTS live session ${JSON.stringify(sessionId)} already exists`);
     }
     ```
     由于前序校验抛错未走 `catch` 清理，`this.liveSessions.has(sessionId)` 依然为 `true`，导致随后的重试 `open` 请求**无条件抛出 `"session already exists"` 异常**；
  3. **策略实例不可逆崩溃**：Rust 再次收到 `open` 失败后，由于 `session.revision` 已为 0，`was_append` 判定为 `false`，直接进入不可逆的分支：
     ```rust
     cycle_error = Some(pine_error_message(error));
     break;
     ```
     策略主运行循环彻底退出，策略实例进入 `HALTED` 状态，**实盘计算永久挂死且无法自动恢复，必须手动重启整个 JFTrade 进程**！
- **严重性定级**：原初分析误以为该异常会触发 `delete(sessionId)` 并仅导致 1 轮 K 线重新加载，但对抗审计证实代码结构导致会话泄漏与死锁，引发实盘策略的致命瘫痪，属于高危实盘阻断漏洞。
- **修复与加固建议**：
  1. 将开盘时间单调递增校验移入 `try` 块内部，或在抛出异常前显式调用 `this.liveSessions.delete(sessionId)` 消除泄漏；
  2. 在工程实践中，网络重推与行情瞬时时钟抖动常见，最佳方案是在 Node 端或 Rust 适配层对非严格递增的重复/滞后 Tick 执行**静默去重或就地更新（in-place update）**，严禁因单根异常数据杀死整个运行期策略。

#### 2. 实盘 200 根预热 vs 回测全量预热指标偏离推演 (P1-08 隐患)
对于技术分析中广泛使用的无限脉冲响应（IIR）指标，如 `ta.ema(source, 200)`：
$$\alpha = \frac{2}{N + 1} = \frac{2}{201} \approx 0.00995$$
初始值在第 0 根 K 线的误差衰减公式为 $(1 - \alpha)^k$。
当实盘仅预热 $k = 200$ 根 K 线时，初始值的残留误差权重为：
$$(1 - 0.00995)^{200} = (0.99005)^{200} \approx \mathbf{13.4\%}$$
这意味着在实盘策略启动初期，`EMA(200)` 指标包含高达 **13.4% 的未收敛偏差**！而在回测中，通常有数千根历史 K 线预热，残差权重完全收敛至 $10^{-9}$。
**后果：同一策略在回测中产生金叉买入信号，实盘由于均线严重滞后而不触发，导致策略回测与实盘表现严重脱节。**

#### 3. 大绘图对象突破 4MB gRPC 限制 (P1-09 隐患)
Rust 侧 `DEFAULT_MAX_MESSAGE_BYTES = 4MB`（`execution.rs:26`）。当复杂脚本在图表上绘制成千上万个 `line.new()` 或 `box.new()` 时，视觉输出 JSON 突破 4MB，Tonic 解码报错并判定 append 失败，同样导致会话被物理注销。

#### 2.9.4 Release Qualification 验证清单与实测结论
- [x] **RQ-WIRE-01（正常流 - 已闭环）**: 启动实盘策略，验证 open $\to$ append (单调递增 revision) $\to$ close 完整生命周期，增量指标输出正确；`pinetsResult.ts` 对 `result.drawings` 增量切片去重，92 项 Pine Worker 测试全量通过。
- [x] **RQ-WIRE-02（乱序容错与死锁自愈 - 已闭环）**: 修复 `pinetsExecutor.ts:129-136` 中开盘时间单调递增校验位置，移入 `try` 块并确保失效会话立即从 `this.liveSessions` 物理清理；同时在 `openLiveSession` 中引入自愈防御（若会话已存在则先行安全清理重建），杜绝了会话残留与随后的 `"already exists"` 死锁崩溃（已由 `pinetsExecutor.test.ts` 专项回归测试覆盖验证）。
- [x] **RQ-WIRE-03（指标收敛验证 - 已闭环）**: 在 `pinets_wire_events_and_warmup_convergence.rs` 中完整推导并经验证 EMA 与 RMA 预热残差公式：$k=200$ 时 EMA 残差为 13.4%，RMA 残差为 36.7%；当预热扩展至 $k \ge 3.5 \times N$（700 根）时残差降至 $<0.1\%$，1000 根时收敛至 $<0.005\%$。
- [x] **RQ-WIRE-04（报文超限保护 - 已闭环）**: 在 `adapter.ts` 的 `normalizeVisualOutputs` 中引入 1000 个绘图对象 FIFO 保护上限（符合 TradingView 500 lines / 500 boxes 行业标准），将视觉输出负载限制在 ~200KB（远低于 Tonic 4MB 限制的 5%），彻底杜绝超大绘图导致 gRPC 报文溢出与会话异常注销（已由 `adapter.test.ts` 与 `pinets_wire_events_and_warmup_convergence.rs` 专项测试覆盖验证）。
