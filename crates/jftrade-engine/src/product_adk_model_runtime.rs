//! Runtime-owned ADK model adapter.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, Sender};
use std::sync::{Arc, Mutex};
use std::thread::{self, JoinHandle};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use reqwest::{Client, StatusCode, Url};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use jftrade_api::ApiStream;
use jftrade_store_sqlite::{
    AdkApprovalStage, AdkRunEvent, AdkSessionStore, AdkStore, AdkStoreError,
    AdkToolInvocationClaim, CreateAdkRunParams, StoredAdkRunLease,
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

include!("product_adk_model_runtime_lifecycle.rs");

const MAX_RESPONSE_BYTES: usize = 4 << 20;
const DEFAULT_TIMEOUT_MS: u64 = 120_000;
const DEFAULT_BUILTIN_AGENT_ID: &str = "jftrade-default";
const DEFAULT_BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

#[derive(Debug)]
pub(crate) struct ProductionAdkChatRuntime {
    store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    secrets_path: PathBuf,
    cancellation_registry: Arc<RunCancellationRegistry>,
    tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
    tool_executor: Arc<dyn AdkToolExecutor>,
    pub(crate) continuation_supervisor: Arc<ContinuationSupervisor>,
}

/// Process-local cancellation fan-out for active provider calls.
#[derive(Debug, Default)]
pub(crate) struct RunCancellationRegistry {
    active: Mutex<BTreeMap<String, Arc<AtomicBool>>>,
}

impl RunCancellationRegistry {
    fn register(&self, run_id: &str) -> Arc<AtomicBool> {
        let token = Arc::new(AtomicBool::new(false));
        self.register_token(run_id, token)
    }

    fn register_token(&self, run_id: &str, token: Arc<AtomicBool>) -> Arc<AtomicBool> {
        if let Ok(mut active) = self.active.lock() {
            active.insert(run_id.to_owned(), Arc::clone(&token));
        }
        token
    }

    fn unregister(&self, run_id: &str) {
        if let Ok(mut active) = self.active.lock() {
            active.remove(run_id);
        }
    }

    pub(crate) fn cancel(&self, run_id: &str) -> bool {
        let token = self
            .active
            .lock()
            .ok()
            .and_then(|active| active.get(run_id).cloned());
        if let Some(token) = token {
            token.store(true, Ordering::Release);
            true
        } else {
            false
        }
    }

    #[allow(dead_code)]
    fn cancel_all(&self) {
        if let Ok(active) = self.active.lock() {
            for token in active.values() {
                token.store(true, Ordering::Release);
            }
        }
    }
}

/// Owns every approval continuation thread started by a production ADK
/// runtime. A runtime-owned reaper joins completed workers immediately, while
/// shutdown cancels and joins every worker that is still active.
#[derive(Debug)]
pub(crate) struct ContinuationSupervisor {
    tasks: Arc<Mutex<BTreeMap<String, Arc<ContinuationTask>>>>,
    completion_tx: Mutex<Option<Sender<ContinuationCompletion>>>,
    reaper: Mutex<Option<JoinHandle<()>>>,
    stopping: AtomicBool,
}

#[derive(Debug)]
struct ContinuationTask {
    #[allow(dead_code)]
    cancellation: Arc<AtomicBool>,
    done: AtomicBool,
    join: Mutex<Option<JoinHandle<()>>>,
}

#[derive(Debug)]
struct ContinuationCompletion {
    run_id: String,
    task: Arc<ContinuationTask>,
}

impl Default for ContinuationSupervisor {
    fn default() -> Self {
        let tasks = Arc::new(Mutex::new(BTreeMap::new()));
        let (completion_tx, completion_rx) = mpsc::channel::<ContinuationCompletion>();
        let reaper_tasks = Arc::clone(&tasks);
        let reaper = thread::Builder::new()
            .name("jftrade-adk-continuation-reaper".to_owned())
            .spawn(move || {
                while let Ok(completion) = completion_rx.recv() {
                    let completed = reaper_tasks.lock().ok().and_then(|mut tasks| {
                        let current = tasks.get(&completion.run_id)?;
                        if !Arc::ptr_eq(current, &completion.task) {
                            return None;
                        }
                        tasks.remove(&completion.run_id)
                    });
                    if let Some(completed) = completed
                        && let Ok(mut join) = completed.join.lock()
                        && let Some(handle) = join.take()
                    {
                        let _ = handle.join();
                    }
                }
            })
            .ok();
        Self {
            tasks,
            completion_tx: Mutex::new(reaper.as_ref().map(|_| completion_tx)),
            reaper: Mutex::new(reaper),
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
            join: Mutex::new(None),
        });
        // Reserve the run id before starting the OS thread.  This closes the
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
            let stale = tasks
                .remove(run_id)
                .expect("continuation reservation disappeared");
            if let Ok(mut guard) = stale.join.lock()
                && let Some(handle) = guard.take()
            {
                let _ = handle.join();
            }
        }
        tasks.insert(run_id.to_owned(), Arc::clone(&state));
        drop(tasks);
        // Do not let a very short continuation finish before its join handle
        // is published in the reservation.  The start gate closes the race
        // where a second approval request observes `done = true` and removes
        // the task before shutdown can take ownership of the handle.
        let (start_tx, start_rx) = mpsc::sync_channel(0);
        let state_for_thread = Arc::clone(&state);
        let completion_tx = self
            .completion_tx
            .lock()
            .ok()
            .and_then(|sender| sender.as_ref().cloned());
        let completion_run_id = run_id.to_owned();
        let handle = match thread::Builder::new()
            .name("jftrade-adk-approval-resume".to_owned())
            .spawn(move || {
                if start_rx.recv().is_err() {
                    state_for_thread.done.store(true, Ordering::Release);
                    if let Some(sender) = completion_tx.as_ref() {
                        let _ = sender.send(ContinuationCompletion {
                            run_id: completion_run_id,
                            task: state_for_thread,
                        });
                    }
                    return;
                }
                task(cancellation);
                state_for_thread.done.store(true, Ordering::Release);
                if let Some(sender) = completion_tx.as_ref() {
                    let _ = sender.send(ContinuationCompletion {
                        run_id: completion_run_id,
                        task: state_for_thread,
                    });
                }
            }) {
            Ok(handle) => handle,
            Err(error) => {
                if let Ok(mut tasks) = self.tasks.lock() {
                    tasks.remove(run_id);
                }
                return Err(unavailable(format!(
                    "assistant continuation unavailable: {error}"
                )));
            }
        };
        match state.join.lock() {
            Ok(mut guard) => *guard = Some(handle),
            Err(poisoned) => {
                // Keep ownership of the thread even if a previous panic
                // poisoned this bookkeeping mutex; dropping `handle` would
                // detach the continuation from shutdown supervision.
                let mut guard = poisoned.into_inner();
                *guard = Some(handle);
            }
        }
        let _ = start_tx.send(());
        Ok(())
    }

    #[allow(dead_code)]
    fn shutdown(&self) {
        self.stopping.store(true, Ordering::Release);
        let tasks = self
            .tasks
            .lock()
            .map(|mut tasks| {
                std::mem::take(&mut *tasks)
                    .into_values()
                    .collect::<Vec<_>>()
            })
            .unwrap_or_default();
        for task in &tasks {
            task.cancellation.store(true, Ordering::Release);
        }
        for task in tasks {
            if let Ok(mut guard) = task.join.lock()
                && let Some(handle) = guard.take()
                && handle.thread().id() != thread::current().id()
            {
                let _ = handle.join();
            }
        }
        if let Ok(mut sender) = self.completion_tx.lock() {
            sender.take();
        }
        if let Ok(mut reaper) = self.reaper.lock()
            && let Some(handle) = reaper.take()
            && handle.thread().id() != thread::current().id()
        {
            let _ = handle.join();
        }
    }
}

const RUN_LEASE_TTL: Duration = Duration::from_secs(30);
const RUN_LEASE_HEARTBEAT: Duration = Duration::from_secs(10);

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
            .map_err(|error| match error {
                AdkStoreError::Conflict(message) => AdkChatPortError::Conflict(message),
                error => storage_unavailable(error),
            })?;
        let stop = Arc::new(AtomicBool::new(false));
        let lost = Arc::new(AtomicBool::new(false));
        let (wake, receiver) = mpsc::channel();
        let heartbeat_store = Arc::clone(&store);
        let heartbeat_stop = Arc::clone(&stop);
        let heartbeat_lost = Arc::clone(&lost);
        let heartbeat_lease = lease.clone();
        let heartbeat = match thread::Builder::new()
            .name("jftrade-adk-run-lease".to_owned())
            .spawn(move || {
                while !heartbeat_stop.load(Ordering::Acquire) {
                    if receiver.recv_timeout(RUN_LEASE_HEARTBEAT).is_ok() {
                        break;
                    }
                    if heartbeat_store
                        .heartbeat_run_lease(&heartbeat_lease, RUN_LEASE_TTL)
                        .is_err()
                    {
                        heartbeat_lost.store(true, Ordering::Release);
                        break;
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

pub(super) fn lease_owner_id(run_id: &str) -> String {
    format!(
        "rust-adk:{}:{}:{}",
        std::process::id(),
        run_id,
        format!("{:?}", thread::current().id())
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

#[derive(Debug)]
enum PreparedChat {
    Existing(AdkChatPortOutput),
    New(ChatExecution),
}

#[derive(Clone, Debug)]
struct ChatExecution {
    route: AdkChatRoute,
    run_id: String,
    session_id: String,
    agent_id: String,
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
include!("product_adk_model_runtime_tool_loop.rs");
include!("product_adk_model_runtime_adapters.rs");

#[path = "product_adk_tool_executor.rs"]
mod tool_executor;
use tool_executor::{AdkToolExecutor, ProductionAdkToolExecutor};
