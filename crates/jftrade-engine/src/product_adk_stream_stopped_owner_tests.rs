use super::*;
use std::net::TcpListener;
use std::sync::atomic::Ordering;
use tokio_stream::StreamExt;

struct StreamFixture {
    _directory: tempfile::TempDir,
    store: Arc<AdkStore>,
    sessions: Arc<AdkSessionStore>,
    runtime: Arc<ProductionAdkChatRuntime>,
    chat: ChatExecution,
    probe: TcpListener,
}

impl StreamFixture {
    fn new(status: &str) -> Self {
        let (directory, store, sessions) = initialized_stores();
        let runtime = runtime_for(&directory, &store, &sessions);
        let mut chat = chat_for("run-stream-stopped");
        chat.route = super::super::super::AdkChatRoute::Stream;
        let probe = TcpListener::bind("127.0.0.1:0").unwrap();
        probe.set_nonblocking(true).unwrap();
        chat.request.endpoint = format!("http://{}/v1/responses", probe.local_addr().unwrap())
            .parse()
            .unwrap();
        create_running_run(&store, &chat.run_id, json!([]));
        let mut payload: Value =
            serde_json::from_str(&store.get_run(&chat.run_id).unwrap().unwrap().payload_json)
                .unwrap();
        payload["status"] = json!(status);
        payload["streamEvents"] = if status == "RUNNING" {
            json!([])
        } else {
            json!([{"type":"final", "sequence":1, "runId":chat.run_id,
                "response":{"run":{"id":chat.run_id,"status":status},"reply":"retained"}}])
        };
        payload["providerEvents"] = json!([]);
        store
            .update_run_state(&chat.run_id, status, &payload.to_string())
            .unwrap();
        sessions
            .upsert_session("jftrade", "local", &chat.session_id, "{}")
            .unwrap();
        sessions
            .record_event(jftrade_store_sqlite::RecordAdkEventParams {
                id: "retained-session-event",
                app_name: "jftrade",
                user_id: "local",
                session_id: &chat.session_id,
                invocation_id: &chat.run_id,
                author: "assistant",
                content: "retained",
            })
            .unwrap();
        store
            .record_audit_event(
                "retained-audit",
                "run.retained",
                &chat.run_id,
                &json!({"status":status}).to_string(),
            )
            .unwrap();
        Self {
            _directory: directory,
            store,
            sessions,
            runtime,
            chat,
            probe,
        }
    }

    fn run(&self, sender: jftrade_api::ApiStreamSender, precancelled: bool) {
        let lease =
            RunLeaseGuard::acquire(self.store.clone(), &self.chat.run_id, "stream-owner").unwrap();
        let token = self
            .runtime
            .cancellation_registry
            .register(&self.chat.run_id);
        token.store(precancelled, Ordering::Release);
        self.runtime
            .run_live_stream(self.chat.clone(), sender, token, lease);
        assert!(
            !self.runtime.cancellation_registry.cancel(&self.chat.run_id),
            "execution registration released"
        );
        let replacement =
            RunLeaseGuard::acquire(self.store.clone(), &self.chat.run_id, "replacement-owner")
                .unwrap();
        assert!(replacement.token() > 1, "execution lease released");
    }

    fn assert_no_provider_request(&self) {
        assert!(
            matches!(self.probe.accept(), Err(error) if error.kind() == std::io::ErrorKind::WouldBlock),
            "a stopped stream execution must not connect to the model provider"
        );
    }
}

impl Drop for StreamFixture {
    fn drop(&mut self) {
        self.runtime.shutdown();
    }
}

// Supplementary full stream-owner control; the frozen terminal CancelRun
// original is separately anchored on its existing mutation-port evidence.
#[test]
fn production_stopped_stream_owner_replays_terminal_frame_without_a_provider_request() {
    for status in [
        "COMPLETED",
        "FAILED",
        "CANCELLED",
        "TIMED_OUT",
        "DENIED",
        "PENDING",
    ] {
        let fixture = StreamFixture::new(status);
        let before = fixture
            .store
            .get_run(&fixture.chat.run_id)
            .unwrap()
            .unwrap();
        let events = fixture
            .sessions
            .list_events(&fixture.chat.session_id)
            .unwrap();
        let audits = fixture.store.list_audit_events().unwrap();
        for disconnected in [false, true] {
            let (stream, sender) = jftrade_api::ApiStream::channel(8);
            let mut body = stream.take_body().unwrap();
            if disconnected {
                drop(body);
                fixture.run(sender, false);
            } else {
                fixture.run(sender, false);
                tokio::runtime::Builder::new_current_thread()
                    .enable_all()
                    .build()
                    .unwrap()
                    .block_on(async {
                        let frame = String::from_utf8(body.next().await.unwrap().unwrap()).unwrap();
                        assert!(frame.contains("\"type\":\"final\""), "{frame}");
                        assert!(frame.contains("retained"), "{frame}");
                        let data = frame
                            .lines()
                            .find_map(|line| line.strip_prefix("data: "))
                            .unwrap();
                        let emitted: Value = serde_json::from_str(data).unwrap();
                        let retained: Value = serde_json::from_str(&before.payload_json).unwrap();
                        assert_eq!(emitted, retained["streamEvents"][0]);
                        assert!(body.next().await.is_none());
                    });
            }
            fixture.assert_no_provider_request();
            assert_eq!(
                fixture
                    .store
                    .get_run(&fixture.chat.run_id)
                    .unwrap()
                    .unwrap(),
                before,
                "{status}"
            );
            assert_eq!(
                fixture
                    .sessions
                    .list_events(&fixture.chat.session_id)
                    .unwrap(),
                events,
                "{status}"
            );
            assert_eq!(
                fixture.store.list_audit_events().unwrap(),
                audits,
                "{status}"
            );
        }
    }
}

#[test]
fn production_stream_precancellation_and_body_disconnect_keep_one_durable_terminal_error() {
    for precancelled in [false, true] {
        let fixture = StreamFixture::new("RUNNING");
        let (stream, sender) = jftrade_api::ApiStream::channel(8);
        let body = stream.take_body().unwrap();
        if !precancelled {
            drop(body);
        }
        fixture.run(sender, precancelled);
        fixture.assert_no_provider_request();
        let run = fixture
            .store
            .get_run(&fixture.chat.run_id)
            .unwrap()
            .unwrap();
        let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
        assert_eq!(run.status, "CANCELLED");
        assert_eq!(payload["errorCode"], "RUN_CANCELLED");
        assert_eq!(payload["providerEvents"], json!([]));
        assert_eq!(payload["streamEvents"].as_array().unwrap().len(), 1);
        assert_eq!(payload["streamEvents"][0]["type"], "error");
        assert!(
            !payload["streamEvents"][0]["message"]
                .as_str()
                .unwrap()
                .is_empty()
        );
        let events = fixture
            .sessions
            .list_events(&fixture.chat.session_id)
            .unwrap();
        let audits = fixture.store.list_audit_events().unwrap();
        assert_eq!(
            audits
                .iter()
                .filter(|event| event.kind == "run.cancelled")
                .count(),
            1
        );
        let (replay, sender) = jftrade_api::ApiStream::channel(8);
        drop(replay.take_body().unwrap());
        fixture.run(sender, false);
        fixture.assert_no_provider_request();
        assert_eq!(
            fixture
                .store
                .get_run(&fixture.chat.run_id)
                .unwrap()
                .unwrap(),
            run
        );
        assert_eq!(
            fixture
                .sessions
                .list_events(&fixture.chat.session_id)
                .unwrap(),
            events
        );
        assert_eq!(fixture.store.list_audit_events().unwrap(), audits);
    }
}

#[test]
fn production_running_stream_owner_contacts_provider_and_persists_its_timeout() {
    let fixture = StreamFixture::new("RUNNING");
    let (stream, sender) = jftrade_api::ApiStream::channel(8);
    let _body = stream.take_body().unwrap();
    fixture.run(sender, false);
    let (_socket, _) = fixture
        .probe
        .accept()
        .expect("running stream contacts the loopback provider");
    let run = fixture
        .store
        .get_run(&fixture.chat.run_id)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(run.status, "TIMED_OUT");
    // Frozen Go RunErrorCode maps the provider deadline at the run layer.
    assert_eq!(payload["errorCode"], "RUN_TIMED_OUT");
}
