//! Runtime-owned ADK model adapter.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::mpsc::{self, Sender};
use std::sync::{Arc, Condvar, Mutex};
use std::thread::{self, JoinHandle};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use reqwest::{Client, StatusCode, Url};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use jftrade_api::ApiStream;
use jftrade_store_sqlite::{
    AdkApprovalStage, AdkRunEvent, AdkSessionStore, AdkStore, AdkStoreError,
    AdkToolInvocationClaim, CreateAdkRunParams, StoredAdkRun, StoredAdkRunLease,
    StoredAdkToolInvocation,
};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};

#[path = "product_adk_model_stream.rs"]
mod stream_adapter;
use stream_adapter::execute_model_stream;

#[path = "product_adk_model_runtime_stream.rs"]
mod runtime_stream;

#[path = "product_adk_model_runtime_recovery.rs"]
mod runtime_recovery;
use runtime_recovery::DurableRunRecoverySupervisor;

#[path = "product_adk_model_runtime_projection.rs"]
mod runtime_projection;

include!("product_adk_model_runtime_lifecycle.rs");

#[cfg(test)]
#[path = "product_adk_model_runtime_fencing_tests.rs"]
mod fencing_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_takeover_tests.rs"]
mod takeover_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_gate_tests.rs"]
mod gate_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_tool_failure_tests.rs"]
mod tool_failure_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_catalog_policy_tests.rs"]
mod catalog_policy_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_run_projection_tests.rs"]
mod run_projection_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_chat_turn_tests.rs"]
mod chat_turn_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_tool_deadline_tests.rs"]
mod tool_deadline_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_terminal_audit_tests.rs"]
mod terminal_audit_tests;

#[cfg(test)]
#[path = "product_adk_model_runtime_expiry_tests.rs"]
mod expiry_tests;

#[cfg(test)]
#[path = "product_adk_session_context_tests.rs"]
mod session_context_tests;

#[cfg(test)]
#[path = "product_adk_input_request_parity_tests.rs"]
mod input_request_parity_tests;

include!("product_adk_model_runtime_audit.rs");

const MAX_RESPONSE_BYTES: usize = 4 << 20;
/// Go `assistantmodel.DefaultProviderRequestTimeout`: the fallback used when a
/// provider row predates the persisted `requestTimeoutMs` normalization.
const DEFAULT_TIMEOUT_MS: u64 = 180_000;
/// Go's provider health probe bounds each request independently even when the
/// persisted provider timeout is configured for a much longer chat turn.
const MAX_PROVIDER_PROBE_TIMEOUT_MS: u64 = 30_000;
/// Go `assistantmodel.MaxMessageLength`: a chat message longer than this many
/// runes is rejected before any run is created.
const MAX_MESSAGE_LENGTH: usize = 50_000;
/// Go `assistantmodel.MaxConcurrentRuns`: the runtime admits at most this many
/// live chat runs.  Go's `Runtime.prepareChatRequest` uses a non-blocking
/// channel send, so the request after the limit is rejected with
/// "maximum concurrent runs (10) reached, please try again later" instead of
/// queueing behind the current runs.
const MAX_CONCURRENT_RUNS: usize = 10;
const DEFAULT_BUILTIN_AGENT_ID: &str = "jftrade-default";
const DEFAULT_BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

fn provider_probe_timeout_ms(configured: u64) -> u64 {
    let configured = if configured == 0 {
        DEFAULT_TIMEOUT_MS
    } else {
        configured
    };
    configured
        .clamp(15_000, 600_000)
        .min(MAX_PROVIDER_PROBE_TIMEOUT_MS)
}

#[derive(Debug)]
pub(crate) struct ProductionAdkChatRuntime {
    store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    secrets_path: PathBuf,
    /// Settings document backing Go's `RuntimeLimits` snapshot; the run
    /// timeout is re-read from it on every new and resumed goal run.
    settings_path: PathBuf,
    cancellation_registry: Arc<RunCancellationRegistry>,
    tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
    tool_executor: Arc<dyn AdkToolExecutor>,
    pub(crate) continuation_supervisor: Arc<ContinuationSupervisor>,
    /// Process-wide admission gate shared by every facade derived from this
    /// runtime, mirroring Go's single `Runtime.runSem`.
    run_gate: Arc<RunGate>,
    /// Process-wide supervisor for durable RUNNING runs whose model provider
    /// temporarily failed.  This is optional only on short-lived runtime
    /// facades created by continuation workers; the production root always
    /// installs the self-referential supervisor from [`new`].
    recovery_supervisor: Option<Arc<DurableRunRecoverySupervisor>>,
}

include!("product_adk_model_runtime_cancellation.rs");

/// Go's `Runtime.runSem` admission gate for live chat runs.
///
/// The gate is deliberately non-blocking: Go's `prepareChatRequest` performs a
/// `select` with a `default` arm, so an over-limit request fails immediately
/// with `maximum concurrent runs (N) reached, please try again later` rather
/// than waiting for a slot.
#[derive(Debug, Default)]
pub(crate) struct RunGate {
    active: Mutex<usize>,
}

/// RAII slot held for the whole lifetime of one admitted chat run.
#[derive(Debug)]
pub(crate) struct RunGateGuard {
    gate: Arc<RunGate>,
}

impl RunGate {
    fn try_acquire(self: &Arc<Self>) -> Result<RunGateGuard, AdkChatPortError> {
        let mut active = self
            .active
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        if *active >= MAX_CONCURRENT_RUNS {
            return Err(chat_failed(format!(
                "maximum concurrent runs ({MAX_CONCURRENT_RUNS}) reached, please try again later"
            )));
        }
        *active += 1;
        Ok(RunGateGuard {
            gate: Arc::clone(self),
        })
    }

    /// Currently admitted runs; used by the concurrency regression tests.
    #[cfg(test)]
    pub(crate) fn active(&self) -> usize {
        *self
            .active
            .lock()
            .unwrap_or_else(|error| error.into_inner())
    }
}

impl Drop for RunGateGuard {
    fn drop(&mut self) {
        let mut active = self
            .gate
            .active
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        *active = active.saturating_sub(1);
    }
}

/// Synchronization barrier tracking active background continuation tasks.
/// Enforces graceful shutdown with timeout before SQLite lease lock release.
#[derive(Debug, Default)]
pub(crate) struct CompletionBarrier {
    active_count: Mutex<usize>,
    cvar: Condvar,
}

pub(crate) struct BarrierGuard {
    barrier: Arc<CompletionBarrier>,
}

impl Drop for BarrierGuard {
    fn drop(&mut self) {
        if let Ok(mut count) = self.barrier.active_count.lock() {
            *count = count.saturating_sub(1);
            if *count == 0 {
                self.barrier.cvar.notify_all();
            }
        }
    }
}

impl CompletionBarrier {
    pub(crate) fn enter(self: &Arc<Self>) -> BarrierGuard {
        if let Ok(mut count) = self.active_count.lock() {
            *count += 1;
        }
        BarrierGuard {
            barrier: Arc::clone(self),
        }
    }

    pub(crate) fn wait_timeout(&self, timeout: Duration) -> bool {
        let Ok(mut count) = self.active_count.lock() else {
            return false;
        };
        let deadline = Instant::now() + timeout;
        while *count > 0 {
            let now = Instant::now();
            if now >= deadline {
                return false;
            }
            match self.cvar.wait_timeout(count, deadline - now) {
                Ok((new_guard, timeout_result)) => {
                    count = new_guard;
                    if timeout_result.timed_out() && *count > 0 {
                        return false;
                    }
                }
                Err(poisoned) => {
                    let (new_guard, timeout_result) = poisoned.into_inner();
                    count = new_guard;
                    if timeout_result.timed_out() && *count > 0 {
                        return false;
                    }
                }
            }
        }
        true
    }
}

/// Owns every approval continuation task started by a production ADK runtime.
/// Tasks execute via `tokio::task::spawn_blocking` and coordinate graceful shutdown
/// using a `CompletionBarrier` with a 5-second timeout.
#[derive(Debug)]
pub(crate) struct ContinuationSupervisor {
    tasks: Arc<Mutex<BTreeMap<String, Arc<ContinuationTask>>>>,
    barrier: Arc<CompletionBarrier>,
    stopping: AtomicBool,
}

#[derive(Debug)]
struct ContinuationTask {
    #[allow(dead_code)]
    cancellation: Arc<AtomicBool>,
    done: AtomicBool,
}

struct ContinuationTaskGuard {
    state: Arc<ContinuationTask>,
    tasks: Arc<Mutex<BTreeMap<String, Arc<ContinuationTask>>>>,
    run_id: String,
    _barrier: BarrierGuard,
}

impl Drop for ContinuationTaskGuard {
    fn drop(&mut self) {
        self.state.done.store(true, Ordering::Release);
        let mut tasks = self.tasks.lock().unwrap_or_else(|e| e.into_inner());
        if tasks
            .get(&self.run_id)
            .is_some_and(|current| Arc::ptr_eq(current, &self.state))
        {
            tasks.remove(&self.run_id);
        }
    }
}

impl Default for ContinuationSupervisor {
    fn default() -> Self {
        Self {
            tasks: Arc::new(Mutex::new(BTreeMap::new())),
            barrier: Arc::new(CompletionBarrier::default()),
            stopping: AtomicBool::new(false),
        }
    }
}

impl ContinuationSupervisor {
    fn spawn<F>(self: &Arc<Self>, run_id: &str, task: F) -> Result<(), AdkChatPortError>
    where
        F: FnOnce(Arc<AtomicBool>) + Send + 'static,
    {
        if self.stopping.load(Ordering::Acquire) {
            return Err(unavailable("assistant continuation supervisor is stopping"));
        }
        let cancellation = Arc::new(AtomicBool::new(false));
        let state = Arc::new(ContinuationTask {
            cancellation: Arc::clone(&cancellation),
            done: AtomicBool::new(false),
        });
        // Reserve the run id before starting the task.  This closes the
        // check-then-spawn race when two approvals are resolved concurrently.
        let mut tasks = self
            .tasks
            .lock()
            .map_err(|_| unavailable("assistant continuation supervisor lock failed"))?;
        if self.stopping.load(Ordering::Acquire) {
            return Err(unavailable("assistant continuation supervisor is stopping"));
        }
        if let Some(existing) = tasks.get(run_id) {
            if !existing.done.load(Ordering::Acquire) {
                return Err(AdkChatPortError::Conflict(
                    "assistant continuation is already running".to_owned(),
                ));
            }
            tasks.remove(run_id);
        }
        tasks.insert(run_id.to_owned(), Arc::clone(&state));
        // Reserve barrier participation under the same lock as admission.
        // shutdown cannot observe a task that is absent from its join count.
        let guard = ContinuationTaskGuard {
            state: Arc::clone(&state),
            tasks: Arc::clone(&self.tasks),
            run_id: run_id.to_owned(),
            _barrier: self.barrier.enter(),
        };
        drop(tasks);

        let runner = move || {
            let _guard = guard;
            task(cancellation);
        };

        if let Ok(handle) = tokio::runtime::Handle::try_current() {
            handle.spawn_blocking(runner);
        } else {
            thread::Builder::new()
                .name("jftrade-adk-approval-resume".to_owned())
                .spawn(runner)
                .map_err(|error| unavailable(format!("start assistant continuation: {error}")))?;
        }
        Ok(())
    }

    #[allow(dead_code)]
    fn shutdown(&self) {
        if !self.shutdown_with_timeout(Duration::from_secs(5)) {
            tracing::error!("assistant continuations did not finish before shutdown deadline");
        }
    }

    fn shutdown_with_timeout(&self, timeout: Duration) -> bool {
        self.stopping.store(true, Ordering::Release);
        let tasks = self
            .tasks
            .lock()
            .map(|tasks| tasks.values().cloned().collect::<Vec<_>>())
            .unwrap_or_default();
        for task in &tasks {
            task.cancellation.store(true, Ordering::Release);
        }
        self.barrier.wait_timeout(timeout)
    }
}

const RUN_LEASE_TTL: Duration = Duration::from_secs(30);
const RUN_LEASE_HEARTBEAT: Duration = Duration::from_secs(10);
const TOOL_CLAIM_TTL: Duration = Duration::from_secs(30);
const TOOL_CLAIM_HEARTBEAT: Duration = Duration::from_secs(10);
static STREAM_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);

fn unix_now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .ok()
        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
        .unwrap_or(i64::MAX)
}

/// Durable conflicts are expected coordination outcomes, not storage
/// failures.  Keeping their classification in the runtime prevents a live
/// worker, a stale completion, or a run-lease takeover from being converted
/// into a terminal FAILED projection.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum DurableErrorClass {
    LeaseHeldOrLost,
    RevisionConflict,
    InvariantViolation,
    StorageFailure,
}

pub(crate) fn classify_durable_store_error(error: &AdkStoreError) -> DurableErrorClass {
    match error {
        AdkStoreError::RunLeaseHeld { .. } | AdkStoreError::LeaseLost(_) => {
            DurableErrorClass::LeaseHeldOrLost
        }
        AdkStoreError::RevisionChanged(_) => DurableErrorClass::RevisionConflict,
        AdkStoreError::ToolOutcomeUnknown(_) => DurableErrorClass::StorageFailure,
        AdkStoreError::StaleToolClaim(_)
        | AdkStoreError::Invariant(_)
        | AdkStoreError::Conflict(_) => DurableErrorClass::InvariantViolation,
        _ => DurableErrorClass::StorageFailure,
    }
}

pub(crate) fn runtime_store_error(error: AdkStoreError) -> AdkChatPortError {
    match &error {
        AdkStoreError::ToolOutcomeUnknown(message) => AdkChatPortError::Failed {
            status: 500,
            code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
            message: message.clone(),
        },
        _ => match classify_durable_store_error(&error) {
            DurableErrorClass::StorageFailure => storage_unavailable(error),
            DurableErrorClass::LeaseHeldOrLost | DurableErrorClass::RevisionConflict => {
                AdkChatPortError::Conflict(error.to_string())
            }
            DurableErrorClass::InvariantViolation => AdkChatPortError::Failed {
                status: 500,
                code: "ADK_STORAGE_CORRUPT".to_owned(),
                message: error.to_string(),
            },
        },
    }
}

pub(crate) fn is_nonfatal_durable_error(error: &AdkChatPortError) -> bool {
    matches!(error, AdkChatPortError::Conflict(_))
}

/// RAII guard for a durable ADK run lease and its heartbeat worker.
#[derive(Debug)]
pub(crate) struct RunLeaseGuard {
    store: Arc<AdkStore>,
    lease: StoredAdkRunLease,
    stop: Arc<AtomicBool>,
    wake: Sender<()>,
    heartbeat: Option<JoinHandle<()>>,
    lost: Arc<AtomicBool>,
}

impl RunLeaseGuard {
    fn acquire(
        store: Arc<AdkStore>,
        run_id: &str,
        owner_id: &str,
    ) -> Result<Self, AdkChatPortError> {
        let lease = store
            .claim_run_lease(run_id, owner_id, RUN_LEASE_TTL)
            .map_err(runtime_store_error)?;
        Self::from_lease(store, lease)
    }

    fn from_lease(
        store: Arc<AdkStore>,
        lease: StoredAdkRunLease,
    ) -> Result<Self, AdkChatPortError> {
        let stop = Arc::new(AtomicBool::new(false));
        let lost = Arc::new(AtomicBool::new(false));
        let (wake, receiver) = mpsc::channel();
        let heartbeat_store = Arc::clone(&store);
        let heartbeat_stop = Arc::clone(&stop);
        let heartbeat_lost = Arc::clone(&lost);
        let mut heartbeat_lease = lease.clone();
        let heartbeat = match thread::Builder::new()
            .name("jftrade-adk-run-lease".to_owned())
            .spawn(move || {
                while !heartbeat_stop.load(Ordering::Acquire) {
                    if receiver.recv_timeout(RUN_LEASE_HEARTBEAT).is_ok() {
                        break;
                    }
                    match heartbeat_store.heartbeat_run_lease(&heartbeat_lease, RUN_LEASE_TTL) {
                        Ok(refreshed) => heartbeat_lease = refreshed,
                        Err(_) => {
                            heartbeat_lost.store(true, Ordering::Release);
                            break;
                        }
                    }
                }
            }) {
            Ok(handle) => handle,
            Err(error) => {
                let _ = store.release_run_lease(&lease);
                return Err(unavailable(format!(
                    "assistant run lease unavailable: {error}"
                )));
            }
        };
        Ok(Self {
            store,
            lease,
            stop,
            wake,
            heartbeat: Some(heartbeat),
            lost,
        })
    }

    fn token(&self) -> i64 {
        self.lease.fencing_token
    }

    fn owner_id(&self) -> &str {
        &self.lease.owner_id
    }

    fn is_lost(&self) -> bool {
        if self.lost.load(Ordering::Acquire) {
            return true;
        }
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .ok()
            .and_then(|duration| i64::try_from(duration.as_millis()).ok())
            .unwrap_or(i64::MAX);
        let current = self
            .store
            .get_run_lease(&self.lease.run_id)
            .ok()
            .flatten()
            .is_some_and(|lease| {
                lease.owner_id == self.lease.owner_id
                    && lease.fencing_token == self.lease.fencing_token
                    && lease.expires_at_unix_ms > now_ms
            });
        if !current {
            self.lost.store(true, Ordering::Release);
        }
        !current
    }
}

/// Heartbeats one tool claim while its synchronous adapter is executing. The
/// heartbeat stops before commit, leaving a full TTL window for the fenced
/// transaction. A crashed process becomes reclaimable within `TOOL_CLAIM_TTL`.
#[derive(Debug)]
struct ToolClaimHeartbeat {
    stop: Arc<AtomicBool>,
    wake: Sender<()>,
    heartbeat: Option<JoinHandle<()>>,
    lost: Arc<AtomicBool>,
}

impl ToolClaimHeartbeat {
    fn start(
        store: Arc<AdkStore>,
        invocation: StoredAdkToolInvocation,
    ) -> Result<Self, AdkChatPortError> {
        let stop = Arc::new(AtomicBool::new(false));
        let lost = Arc::new(AtomicBool::new(false));
        let (wake, receiver) = mpsc::channel();
        let heartbeat_stop = Arc::clone(&stop);
        let heartbeat_lost = Arc::clone(&lost);
        let heartbeat = thread::Builder::new()
            .name("jftrade-adk-tool-claim".to_owned())
            .spawn(move || {
                while !heartbeat_stop.load(Ordering::Acquire) {
                    if receiver.recv_timeout(TOOL_CLAIM_HEARTBEAT).is_ok() {
                        break;
                    }
                    if store
                        .heartbeat_tool_invocation(&invocation, TOOL_CLAIM_TTL)
                        .is_err()
                    {
                        heartbeat_lost.store(true, Ordering::Release);
                        break;
                    }
                }
            })
            .map_err(|error| unavailable(format!("assistant tool claim heartbeat: {error}")))?;
        Ok(Self {
            stop,
            wake,
            heartbeat: Some(heartbeat),
            lost,
        })
    }

    fn stop(&mut self) -> bool {
        self.stop.store(true, Ordering::Release);
        let _ = self.wake.send(());
        if let Some(handle) = self.heartbeat.take() {
            let _ = handle.join();
        }
        self.lost.load(Ordering::Acquire)
    }
}

impl Drop for ToolClaimHeartbeat {
    fn drop(&mut self) {
        let _ = self.stop();
    }
}

impl ProductionAdkChatRuntime {
    /// Wait for a live lease owner to finish before taking over. A retrying
    /// continuation never steals an unexpired lease; once the durable expiry
    /// is reached, the store's fencing token decides takeover atomically.
    pub(crate) fn acquire_run_lease_with_retry(
        &self,
        run_id: &str,
        owner_id: &str,
        cancellation: &Arc<AtomicBool>,
    ) -> Result<RunLeaseGuard, AdkChatPortError> {
        loop {
            if self
                .continuation_supervisor
                .stopping
                .load(Ordering::Acquire)
            {
                return Err(unavailable("assistant runtime is stopping"));
            }
            if cancellation.load(Ordering::Acquire) || self.run_is_cancelled(run_id) {
                return Err(cancellation_error());
            }
            match RunLeaseGuard::acquire(Arc::clone(&self.store), run_id, owner_id) {
                Ok(lease) => return Ok(lease),
                Err(AdkChatPortError::Conflict(_)) => {
                    let now_ms = SystemTime::now()
                        .duration_since(UNIX_EPOCH)
                        .ok()
                        .and_then(|duration| i64::try_from(duration.as_millis()).ok())
                        .unwrap_or(i64::MAX);
                    let wait_ms = self
                        .store
                        .get_run_lease(run_id)
                        .ok()
                        .flatten()
                        .map(|lease| lease.expires_at_unix_ms.saturating_sub(now_ms))
                        .filter(|remaining| *remaining > 0)
                        .map(|remaining| remaining.min(1_000) as u64)
                        .unwrap_or(250);
                    thread::sleep(Duration::from_millis(wait_ms.max(25)));
                }
                Err(error) => return Err(error),
            }
        }
    }
}

pub(super) fn lease_owner_id(run_id: &str) -> String {
    format!(
        "rust-adk:{}:{}:{:?}",
        std::process::id(),
        run_id,
        thread::current().id()
    )
}

impl Drop for RunLeaseGuard {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        let _ = self.wake.send(());
        if let Some(handle) = self.heartbeat.take() {
            let _ = handle.join();
        }
        let _ = self.store.release_run_lease(&self.lease);
    }
}

#[allow(clippy::large_enum_variant)]
#[derive(Debug)]
enum PreparedChat {
    Existing(AdkChatPortOutput),
    /// `New` carries the durable run lease plus Go's `Runtime.runSem` slot, so
    /// the slot stays held for exactly the lifetime of one admitted run.
    New(ChatExecution, RunLeaseGuard, RunGateGuard),
}

/// Model-visible tool scope for one agent, mirroring Go's
/// `ToolDescriptorsForAgent`: `all` exposes every callable tool, `none`
/// exposes nothing, and `selected` exposes only the agent allowlist (plus the
/// implicit `backtest.kline_sync_status` companion, exactly like Go).
#[derive(Clone, Debug, Default)]
enum AgentToolScope {
    #[default]
    All,
    None,
    Selected(std::collections::BTreeSet<String>),
}

impl AgentToolScope {
    fn exposes(&self, name: &str) -> bool {
        match self {
            Self::All => true,
            Self::None => false,
            Self::Selected(tools) => {
                tools.contains(name)
                    || (name == "backtest.kline_sync_status"
                        && (tools.contains("strategy.research_backtest")
                            || tools.contains("strategy.optimize")))
            }
        }
    }
}

#[cfg(test)]
impl ChatExecution {
    /// Test constructor: the policy tests drive staging and the tool loop
    /// without a model provider, so they build the execution directly.
    #[allow(clippy::too_many_arguments)]
    pub(super) fn for_test(
        run_id: String,
        session_id: String,
        agent_id: String,
        permission_mode: String,
        request: ModelRequest,
    ) -> Self {
        Self {
            route: AdkChatRoute::Chat,
            run_id,
            session_id,
            agent_id,
            resumed: false,
            resumed_from_input: false,
            permission_mode,
            context_deltas: Vec::new(),
            request,
        }
    }
}

#[derive(Clone, Debug)]
struct ChatExecution {
    route: AdkChatRoute,
    run_id: String,
    session_id: String,
    agent_id: String,
    /// True when this execution is a continuation of a resolved approval, so
    /// the terminal projection records `resumeState=adk_confirmation_resolved`
    /// exactly like the reference runtime's `hydrateResumedRun`.
    resumed: bool,
    /// True when this execution is a continuation of an answered input
    /// request.  Go's `completeInputContinuation` finishes that run with
    /// `resumeState=input_resolved` and `failInputContinuation` fails it with
    /// `input_resume_failed`, so the terminal projection needs the kind of
    /// resume, not only the fact that the run was resumed.
    resumed_from_input: bool,
    /// The agent's resolved permission mode.  `ToolRequiresApproval` combines
    /// it with each descriptor's permission/risk to decide whether a requested
    /// call runs immediately or waits for the operator.
    permission_mode: String,
    /// Auto-compaction deltas published before the run existed.  Go forwards
    /// them through `onDelta` ahead of the run events, so the live stream
    /// emits the notice and context frames first.
    context_deltas: Vec<SessionContextDelta>,
    request: ModelRequest,
}

fn text_field(object: &serde_json::Map<String, Value>, field: &str) -> Option<String> {
    object
        .get(field)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

include!("product_adk_model_runtime_events.rs");
include!("product_adk_model_runtime_tool_result.rs");
include!("product_adk_model_runtime_tool_loop.rs");
include!("product_adk_model_runtime_adapters.rs");
include!("product_adk_model_runtime_readiness.rs");
include!("product_adk_model_runtime_retry.rs");

#[path = "product_adk_tool_executor.rs"]
mod tool_executor;
pub(crate) use tool_executor::{AdkToolExecutor, ProductionAdkToolExecutor};
