use super::*;
use std::sync::atomic::{AtomicBool, AtomicUsize};
use std::time::{Duration, Instant};

#[derive(Debug)]
struct CancelledNodeRuntime {
    deadline: bool,
    cancelled: AtomicBool,
    started: std::sync::mpsc::Sender<()>,
    attempts: AtomicUsize,
    successors: AtomicUsize,
}

impl AdkChatStreamPort for CancelledNodeRuntime {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let request: Value = serde_json::from_slice(&input.body).unwrap();
        if request["message"] == "successor" {
            self.successors.fetch_add(1, Ordering::SeqCst);
            return Ok(AdkChatPortOutput::Json(
                json!({"run":{"id":"successor","status":"SUCCEEDED"}}),
            ));
        }
        assert_eq!(request["message"], "blocking");
        self.attempts.fetch_add(1, Ordering::SeqCst);
        self.started.send(()).unwrap();
        let started = Instant::now();
        loop {
            if self.cancelled.load(Ordering::Acquire)
                || (self.deadline && started.elapsed() >= Duration::from_millis(100))
            {
                return Err(AdkChatPortError::Failed {
                    status: if self.deadline { 504 } else { 499 },
                    code: if self.deadline {
                        "RUN_TIMEOUT"
                    } else {
                        "RUN_CANCELLED"
                    }
                    .to_owned(),
                    message: if self.deadline {
                        "context deadline exceeded"
                    } else {
                        "context canceled"
                    }
                    .to_owned(),
                });
            }
            assert!(
                started.elapsed() < Duration::from_secs(2),
                "external cancellation deadline"
            );
            std::thread::sleep(Duration::from_millis(1));
        }
    }
}

// Parity: go:452dea11:internal/assistant/engine/adk22regression/native_runtime_test.go:155 TestWorkflowPropagatesExternalCancellationWithoutSuccessorOrRetry
#[test]
fn canvas_cancelled_and_timed_out_nodes_do_not_dispatch_successors_or_repeat_attempts() {
    for deadline in [false, true] {
        let mut cluster = EngineTestCluster::new();
        let (started, observed) = std::sync::mpsc::channel();
        let runtime = Arc::new(CancelledNodeRuntime {
            deadline,
            cancelled: AtomicBool::new(false),
            started,
            attempts: AtomicUsize::new(0),
            successors: AtomicUsize::new(0),
        });
        Arc::get_mut(&mut cluster.port).unwrap().chat_runtime = Some(runtime.clone());
        let agent = cluster.create_agent("external-cancellation");
        let workflow = cluster.create_canvas_workflow(&agent, "external-cancellation", json!({
            "nodes": [
                {"id":"start","type":"start"},
                {"id":"blocking","type":"agent","data":{"message":"blocking"}},
                {"id":"successor","type":"agent","data":{"message":"successor"}}
            ],
            "edges": [{"source":"start","target":"blocking"},{"source":"blocking","target":"successor"}]
        }));
        let (finished, result) = std::sync::mpsc::channel();
        let job = std::thread::spawn(move || {
            let response = cluster.run_workflow(&workflow, json!({}));
            finished.send(response).unwrap();
            cluster
        });
        observed.recv_timeout(Duration::from_secs(1)).unwrap();
        if !deadline {
            runtime.cancelled.store(true, Ordering::Release);
        }
        let response = result
            .recv_timeout(Duration::from_secs(2))
            .unwrap()
            .unwrap();
        let cluster = job.join().unwrap();
        assert_eq!(response["log"]["status"], "FAILED");
        let nodes = response["log"]["nodeRuns"].as_array().unwrap();
        assert_eq!(nodes[1]["nodeId"], "blocking");
        assert_eq!(nodes[1]["status"], "FAILED");
        assert_eq!(
            nodes[1]["error"],
            if deadline {
                "RUN_TIMEOUT: context deadline exceeded"
            } else {
                "RUN_CANCELLED: context canceled"
            }
        );
        assert_eq!(nodes[2]["nodeId"], "successor");
        assert_eq!(nodes[2]["status"], "SKIPPED");
        assert_eq!(runtime.attempts.load(Ordering::SeqCst), 1);
        assert_eq!(runtime.successors.load(Ordering::SeqCst), 0);
        let id = response["log"]["id"].as_str().unwrap();
        assert_eq!(
            cluster
                .port
                .store
                .get_workflow_trigger_log(id)
                .unwrap()
                .unwrap()
                .status,
            "FAILED"
        );
    }
}
