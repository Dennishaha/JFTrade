use super::*;
use std::collections::BTreeMap;
use std::io::{BufRead, Read, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Barrier, Condvar, Mutex, mpsc};
use std::thread;
use std::time::Instant;

use super::super::{AdkChatPortError, AdkToolExecutor};
use crate::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use crate::product::product_production_ports::{ProductionAdkPort, ProductionToolCatalog};
use jftrade_store_sqlite::AdkArtifactStore;

#[derive(Debug)]
struct CountedTools {
    counts: Arc<Mutex<BTreeMap<String, usize>>>,
    release: Arc<(Mutex<bool>, Condvar)>,
    started: Mutex<Option<mpsc::Sender<()>>>,
}

impl AdkToolExecutor for CountedTools {
    fn supports(&self, name: &str) -> bool {
        matches!(name, "strategy.save_draft" | "alerts.price.set")
    }

    fn execute(&self, name: &str, _: &Value) -> Result<Value, String> {
        *self
            .counts
            .lock()
            .unwrap()
            .entry(name.to_owned())
            .or_default() += 1;
        if let Some(started) = self.started.lock().unwrap().take() {
            let _ = started.send(());
        }
        let (released, signal) = &*self.release;
        let (released, _) = signal
            .wait_timeout_while(
                released.lock().unwrap(),
                Duration::from_secs(15),
                |released| !*released,
            )
            .unwrap();
        if !*released {
            return Err("fixture release deadline".to_owned());
        }
        Ok(json!({"saved":true}))
    }
}

pub(super) fn response_provider() -> (String, Arc<AtomicBool>, thread::JoinHandle<()>) {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let address = listener.local_addr().unwrap();
    let stop = Arc::new(AtomicBool::new(false));
    let stopped = stop.clone();
    let owner = thread::spawn(move || {
        let mut socket = loop {
            if stopped.load(Ordering::Acquire) {
                return;
            }
            match listener.accept() {
                Ok((socket, _)) => break socket,
                Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                    thread::sleep(Duration::from_millis(5))
                }
                Err(error) => panic!("provider accept: {error}"),
            }
        };
        socket.set_nonblocking(false).unwrap();
        socket
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut reader = std::io::BufReader::new(&mut socket);
        let mut length = None;
        loop {
            let mut line = String::new();
            assert!(reader.read_line(&mut line).unwrap() > 0);
            if line == "\r\n" {
                break;
            }
            if let Some(value) = line.to_ascii_lowercase().strip_prefix("content-length:") {
                length = Some(value.trim().parse::<usize>().unwrap());
            }
        }
        let mut body = vec![0; length.unwrap()];
        reader.read_exact(&mut body).unwrap();
        let request: Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(request["stream"], false);
        drop(reader);
        let body = "{\"output_text\":\"approved tools completed\"}";
        socket.write_all(format!("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", body.len()).as_bytes()).unwrap();
    });
    (format!("http://{address}/v1/responses"), stop, owner)
}

struct ApprovalFixture {
    _directory: tempfile::TempDir,
    store: Arc<AdkStore>,
    runtime: Arc<ProductionAdkChatRuntime>,
    port: Arc<ProductionAdkPort>,
    counts: Arc<Mutex<BTreeMap<String, usize>>>,
    release: Arc<(Mutex<bool>, Condvar)>,
    started: mpsc::Receiver<()>,
    provider_stop: Arc<AtomicBool>,
    provider: Option<thread::JoinHandle<()>>,
    approvals: Vec<String>,
}

impl Drop for ApprovalFixture {
    fn drop(&mut self) {
        self.release_tools();
        self.runtime.shutdown();
        self.provider_stop.store(true, Ordering::Release);
        if let Some(provider) = self.provider.take() {
            let result = provider.join();
            if !thread::panicking() {
                result.expect("fixture provider must finish without panic");
            }
        }
    }
}

impl ApprovalFixture {
    fn new(siblings: usize, block: bool) -> Self {
        let (directory, store, sessions) = initialized_stores();
        let settings = directory.path().join("settings.json");
        std::fs::write(&settings, "{}").unwrap();
        let (endpoint, provider_stop, provider) = response_provider();
        store.upsert_provider("provider-approval", &json!({"id":"provider-approval","baseUrl":endpoint,"model":"fixture","enabled":true,"apiKey":"sk-fixture"}).to_string()).unwrap();
        let tools = ["strategy.save_draft", "alerts.price.set"];
        store.upsert_agent("agent-approval", &json!({"id":"agent-approval","name":"Approval","providerId":"provider-approval","status":"ENABLED","permissionMode":"approval","toolAccessMode":"selected","tools":&tools[..siblings]}).to_string()).unwrap();
        store
            .upsert_session("session-approval", "agent-approval", "{}")
            .unwrap();
        sessions
            .upsert_session("jftrade", "local", "session-approval", "{}")
            .unwrap();
        let approvals: Vec<String> = (0..siblings).map(|i| format!("approval-{i}")).collect();
        let calls: Vec<Value> = tools[..siblings].iter().enumerate().map(|(i,name)| json!({"id":format!("call-{i}"),"name":name,"toolName":name,"arguments":{},"status":"PENDING_APPROVAL","requiresUser":true})).collect();
        let pending: Vec<Value> = approvals.iter().enumerate().map(|(i,id)| json!({"id":id,"runId":"run-approval","agentId":"agent-approval","toolName":tools[i],"status":"PENDING","functionCallId":format!("call-{i}"),"confirmationCallId":format!("confirmation-{i}")})).collect();
        store.create_run(CreateAdkRunParams { id:"run-approval",session_id:"session-approval",agent_id:"agent-approval",status:"PENDING",client_request_id:"request-approval",request_fingerprint:"fingerprint-approval",payload_json:&json!({"id":"run-approval","sessionId":"session-approval","agentId":"agent-approval","status":"PENDING","workMode":"chat","route":"chat","requestMessage":"approve tools","toolCalls":calls,"toolResults":[],"pendingApprovals":pending}).to_string() }).unwrap();
        for (i, id) in approvals.iter().enumerate() {
            store.create_approval(id, "run-approval", "agent-approval", "PENDING", &json!({"id":id,"runId":"run-approval","agentId":"agent-approval","toolName":tools[i],"toolCallId":format!("call-{i}"),"functionCallId":format!("call-{i}"),"confirmationCallId":format!("confirmation-{i}"),"status":"PENDING"}).to_string()).unwrap();
        }
        let counts = Arc::new(Mutex::new(BTreeMap::new()));
        let release = Arc::new((Mutex::new(!block), Condvar::new()));
        let (started, observed) = mpsc::channel();
        let executor = Arc::new(CountedTools {
            counts: counts.clone(),
            release: release.clone(),
            started: Mutex::new(Some(started)),
        });
        let runtime = Arc::new(ProductionAdkChatRuntime::with_tool_executor_for_test(
            store.clone(),
            sessions.clone(),
            &settings,
            Arc::new(RunCancellationRegistry::default()),
            Arc::new(ProductionToolCatalog::empty_for_test()),
            executor,
        ));
        let artifact = directory.path().join("adk-artifact.db");
        File::create(&artifact).unwrap();
        initialize_current(&Connection::open(&artifact).unwrap(), "adk-artifact").unwrap();
        let port = Arc::new(
            ProductionAdkPort::new_for_test(
                store.clone(),
                sessions,
                Arc::new(AdkArtifactStore::open(&artifact).unwrap()),
                settings,
            )
            .with_chat_runtime(runtime.clone()),
        );
        Self {
            _directory: directory,
            store,
            runtime,
            port,
            counts,
            release,
            started: observed,
            provider_stop,
            provider: Some(provider),
            approvals,
        }
    }

    fn release_tools(&self) {
        *self.release.0.lock().unwrap() = true;
        self.release.1.notify_all();
    }

    fn completed(&self) -> Value {
        let deadline = Instant::now() + Duration::from_secs(2);
        loop {
            let row = self.store.get_run("run-approval").unwrap().unwrap();
            let payload: Value = serde_json::from_str(&row.payload_json).unwrap();
            if row.status == "COMPLETED" {
                return payload;
            }
            assert!(
                Instant::now() < deadline,
                "continuation must complete: {payload}"
            );
            thread::sleep(Duration::from_millis(5));
        }
    }
}

fn approve(
    port: &ProductionAdkPort,
    id: &str,
) -> Result<Value, crate::product::product_adk_mutation_port::AdkMutationPortError> {
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::Approve,
        identifiers: BTreeMap::from([("approvalId".to_owned(), id.to_owned())]),
        body: Value::Null,
        webhook_secret: None,
    })
}

// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:12 TestConcurrentResolveApprovalExecutesApprovedToolOnce
#[test]
fn production_duplicate_approval_returns_while_the_single_tool_execution_is_blocked() {
    let fixture = ApprovalFixture::new(1, true);
    approve(&fixture.port, &fixture.approvals[0]).unwrap();
    fixture
        .started
        .recv_timeout(Duration::from_secs(2))
        .unwrap();
    let port = fixture.port.clone();
    let id = fixture.approvals[0].clone();
    let (reply, observed) = mpsc::channel();
    let duplicate = thread::spawn(move || {
        reply.send(approve(&port, &id)).unwrap();
    });
    let result = observed.recv_timeout(Duration::from_secs(2));
    let during = fixture.counts.lock().unwrap().clone();
    fixture.release_tools();
    duplicate.join().unwrap();
    assert_eq!(result.unwrap().unwrap()["approval"]["status"], "APPROVED");
    assert_eq!(
        during,
        BTreeMap::from([("strategy.save_draft".to_owned(), 1)])
    );
    fixture.completed();
    assert_eq!(*fixture.counts.lock().unwrap(), during);
    assert!(
        fixture
            .runtime
            .continuation_supervisor
            .barrier
            .wait_timeout(Duration::from_secs(2))
    );
    assert!(
        fixture
            .runtime
            .continuation_supervisor
            .tasks
            .lock()
            .unwrap()
            .values()
            .all(|task| task.done.load(Ordering::Acquire))
    );
}

// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:92 TestConcurrentSiblingApprovalsAreMergedBeforeContinuation
// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:119 TestConcurrentSiblingAsyncApprovalsEnqueueOneContinuation
#[test]
fn production_concurrent_sibling_approvals_complete_with_one_execution_per_tool() {
    let fixture = ApprovalFixture::new(2, false);
    let start = Arc::new(Barrier::new(3));
    let workers: Vec<_> = fixture
        .approvals
        .iter()
        .map(|id| {
            let (port, id, start) = (fixture.port.clone(), id.clone(), start.clone());
            thread::spawn(move || {
                start.wait();
                approve(&port, &id)
            })
        })
        .collect();
    start.wait();
    for worker in workers {
        assert_eq!(
            worker.join().unwrap().unwrap()["approval"]["status"],
            "APPROVED"
        );
    }
    let completed = fixture.completed();
    assert_eq!(completed["resumeState"], "adk_confirmation_resolved");
    assert_eq!(
        *fixture.counts.lock().unwrap(),
        BTreeMap::from([
            ("strategy.save_draft".to_owned(), 1),
            ("alerts.price.set".to_owned(), 1)
        ])
    );
    assert!(
        fixture
            .runtime
            .continuation_supervisor
            .barrier
            .wait_timeout(Duration::from_secs(2))
    );
}

// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:154 TestAsyncApprovalWaitsForLocalInputContinuationLease
#[test]
fn production_approved_tool_waits_for_the_existing_local_run_lease_to_release() {
    let fixture = ApprovalFixture::new(1, false);
    let lease = RunLeaseGuard::acquire(
        fixture.store.clone(),
        "run-approval",
        &super::super::lease_owner_id("run-approval"),
    )
    .unwrap();
    let before = fixture
        .store
        .get_run_lease("run-approval")
        .unwrap()
        .unwrap();
    let response = approve(&fixture.port, &fixture.approvals[0]).unwrap();
    assert_eq!(response["run"]["resumeState"], "approval_resuming");
    let deadline = Instant::now() + Duration::from_secs(2);
    while !fixture
        .runtime
        .cancellation_registry
        .active
        .lock()
        .unwrap()
        .contains_key("run-approval")
    {
        assert!(
            Instant::now() < deadline,
            "real continuation must enter before observing the lease wait"
        );
        thread::sleep(Duration::from_millis(1));
    }
    // Go observes no tool execution for 50ms. The durable lease is 30 seconds
    // and remains owned until explicit Drop; this wait never drives expiry.
    thread::sleep(Duration::from_millis(50));
    assert_eq!(
        fixture
            .store
            .get_run_lease("run-approval")
            .unwrap()
            .unwrap(),
        before
    );
    assert!(fixture.counts.lock().unwrap().is_empty());
    drop(lease);
    let completed = fixture.completed();
    assert_eq!(completed["resumeState"], "adk_confirmation_resolved");
    assert_eq!(
        *fixture.counts.lock().unwrap(),
        BTreeMap::from([("strategy.save_draft".to_owned(), 1)])
    );
}

// Parity: go:452dea11:internal/assistant/engine/runner_approval_concurrency_test.go:191 TestApprovalLeaseWaitStopsWhenRuntimeContextIsCancelled
#[test]
fn production_lease_wait_precancellation_preserves_the_existing_owner() {
    let fixture = ApprovalFixture::new(1, false);
    let lease =
        RunLeaseGuard::acquire(fixture.store.clone(), "run-approval", "local-input-owner").unwrap();
    let before = fixture
        .store
        .get_run_lease("run-approval")
        .unwrap()
        .unwrap();
    let error = fixture
        .runtime
        .acquire_run_lease_with_retry(
            "run-approval",
            "approval-waiter",
            &Arc::new(AtomicBool::new(true)),
        )
        .unwrap_err();
    assert!(
        matches!(error, AdkChatPortError::Failed { status:499,code,message } if code == "CLIENT_DISCONNECTED" && message == "assistant chat client disconnected")
    );
    assert_eq!(
        fixture
            .store
            .get_run_lease("run-approval")
            .unwrap()
            .unwrap(),
        before
    );
    assert!(fixture.counts.lock().unwrap().is_empty());
    drop(lease);
}
