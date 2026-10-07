//! Captured production Responses requests across turns and context revisions.
use super::*;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

struct Capture {
    endpoint: String,
    stop: Arc<AtomicBool>,
    owner: Option<std::thread::JoinHandle<Result<Vec<Value>, String>>>,
}

impl Capture {
    fn new() -> Self {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("provider listener");
        let endpoint = format!("http://{}/v1", listener.local_addr().expect("address"));
        listener.set_nonblocking(true).expect("nonblocking accept");
        let stop = Arc::new(AtomicBool::new(false));
        let cancelled = stop.clone();
        let owner = std::thread::spawn(move || {
            let mut requests = Vec::new();
            while !cancelled.load(Ordering::Acquire) {
                match listener.accept() {
                    Ok((mut stream, _)) => {
                        stream.set_nonblocking(false).map_err(|e| e.to_string())?;
                        stream
                            .set_read_timeout(Some(Duration::from_secs(2)))
                            .map_err(|e| e.to_string())?;
                        stream
                            .set_write_timeout(Some(Duration::from_secs(2)))
                            .map_err(|e| e.to_string())?;
                        requests.push(read_request(&mut stream)?);
                        let body = scripted_text("provider answer").to_string();
                        std::io::Write::write_all(&mut stream, format!("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", body.len()).as_bytes()).map_err(|e|e.to_string())?;
                    }
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        std::thread::sleep(Duration::from_millis(2));
                    }
                    Err(error) => return Err(error.to_string()),
                }
            }
            Ok(requests)
        });
        Self {
            endpoint,
            stop,
            owner: Some(owner),
        }
    }

    fn finish(mut self) -> Vec<Value> {
        self.stop.store(true, Ordering::Release);
        self.owner
            .take()
            .expect("owned provider")
            .join()
            .expect("join provider")
            .expect("provider requests")
    }
}

impl Drop for Capture {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        if let Some(owner) = self.owner.take() {
            let _ = owner.join();
        }
    }
}

fn read_request(stream: &mut std::net::TcpStream) -> Result<Value, String> {
    let deadline = Instant::now() + Duration::from_secs(3);
    let mut bytes = Vec::new();
    let mut chunk = [0u8; 4096];
    let mut expected = None;
    loop {
        if Instant::now() >= deadline {
            return Err("request deadline".into());
        }
        let count = std::io::Read::read(stream, &mut chunk).map_err(|e| e.to_string())?;
        if count == 0 {
            return Err("incomplete request".into());
        }
        bytes.extend_from_slice(&chunk[..count]);
        if bytes.len() > 8 * 1024 * 1024 {
            return Err("request exceeds fixture budget".into());
        }
        if expected.is_none()
            && let Some(end) = bytes.windows(4).position(|w| w == b"\r\n\r\n")
        {
            let header = String::from_utf8_lossy(&bytes[..end]).to_ascii_lowercase();
            if !header.starts_with("post /v1/responses ") {
                return Err(format!(
                    "unexpected provider path: {}",
                    header.lines().next().unwrap_or_default()
                ));
            }
            let length = header
                .lines()
                .find_map(|s| s.strip_prefix("content-length:"))
                .and_then(|s| s.trim().parse::<usize>().ok())
                .ok_or("content length")?;
            expected = Some((end + 4, length));
        }
        if let Some((start, length)) = expected
            && bytes.len() >= start + length
        {
            return serde_json::from_slice(&bytes[start..start + length])
                .map_err(|e| e.to_string());
        }
    }
}

fn seed_cache_agent(store: &AdkStore, capture: &Capture, agent: &str, instruction: &str) {
    store.upsert_provider("cache-provider", &json!({"id":"cache-provider","displayName":"Cache Provider","baseUrl":capture.endpoint,"model":"test-model","apiKey":"sk-test","enabled":true}).to_string()).expect("provider");
    store.upsert_agent(agent, &json!({"id":agent,"name":agent,"providerId":"cache-provider","instruction":instruction,"tools":["missing.none"],"workMode":"chat","status":"ENABLED","memoryEnabled":false}).to_string()).expect("agent");
}

fn chat(
    runtime: &ProductionAdkChatRuntime,
    agent: &str,
    session: Option<&str>,
    message: &str,
    request: &str,
) -> Value {
    let mut body = json!({"agentId":agent,"message":message});
    if let Some(session) = session {
        body["sessionId"] = json!(session);
    }
    let output = runtime
        .dispatch(AdkChatRoute::Chat, &chat_input(request, body))
        .expect("production chat");
    let AdkChatPortOutput::Json(value) = output else {
        panic!("chat JSON")
    };
    assert_eq!(value["run"]["status"], "COMPLETED", "{value}");
    value
}

// Parity: go:452dea11:internal/assistant/engine/context_cache_test.go:61 TestProviderPayloadKeepsStablePrefixAcrossTurns
#[test]
fn production_provider_keeps_original_system_tools_and_prior_user_prefix_across_two_turns() {
    let (directory, store, sessions) = initialized_stores();
    let capture = Capture::new();
    seed_cache_agent(
        &store,
        &capture,
        "cache-prefix-agent",
        "Stable instruction for automatic prompt cache.",
    );
    let runtime = runtime_with_production_catalog(
        &directory,
        &store,
        &sessions,
        Arc::new(RecordingToolExecutor::new(Vec::new())),
    );
    let first = chat(
        &runtime,
        "cache-prefix-agent",
        None,
        "first cacheable turn",
        "11111111-1111-4111-8111-111111111061",
    );
    let session = first["session"]["id"].as_str().expect("session id");
    chat(
        &runtime,
        "cache-prefix-agent",
        Some(session),
        "second cacheable turn",
        "11111111-1111-4111-8111-111111111062",
    );
    runtime.shutdown();
    let requests = capture.finish();
    assert_eq!(requests.len(), 2);
    let first = requests[0]["input"].as_array().expect("first input");
    let second = requests[1]["input"].as_array().expect("second input");
    assert!(first.len() >= 2 && second.len() >= 4, "{requests:?}");
    assert_eq!(first[0]["role"], "system");
    assert_eq!(second[0]["role"], "system");
    assert_eq!(first[0]["content"], second[0]["content"]);
    assert!(
        first[0]["content"]
            .as_str()
            .expect("instruction")
            .contains("Stable instruction for automatic prompt cache.")
    );
    assert_eq!(requests[0].get("tools"), requests[1].get("tools"));
    assert_eq!(
        first[1],
        json!({"role":"user","content":"first cacheable turn"})
    );
    assert_eq!(second[1], first[1]);
    assert_eq!(
        second.last().expect("current user"),
        &json!({"role":"user","content":"second cacheable turn"})
    );
    for needle in [
        "run-",
        "stream",
        "replay",
        "contextRevisionId",
        "rawBreakdown",
    ] {
        assert!(
            !serde_json::to_string(second)
                .expect("messages")
                .contains(needle),
            "{needle} leaked: {second:?}"
        );
    }
}

// Parity: go:452dea11:internal/assistant/engine/context_cache_test.go:116 TestProviderPayloadUsesOnlyCurrentContextRevisionHandoff
#[test]
fn production_provider_excludes_superseded_active_handoff_and_internal_revision_markers() {
    let (directory, store, sessions) = initialized_stores();
    let capture = Capture::new();
    seed_cache_agent(
        &store,
        &capture,
        "cache-handoff-agent",
        "Base stable instruction.",
    );
    store.upsert_session("cache-handoff-session","cache-handoff-agent",&json!({"id":"cache-handoff-session","agentId":"cache-handoff-agent","title":"Cache Handoff"}).to_string()).expect("session");
    sessions
        .upsert_session("jftrade", "local", "cache-handoff-session", "{}")
        .expect("raw session");
    store.upsert_session_context("cache-handoff-session",&json!({"contextRevisionId":"ctx-current-cache","previousContextRevisionId":"ctx-old-cache","contextRevisionCreatedAt":"2026-06-18T10:00:00Z","compactedEventCount":2}).to_string()).expect("current context");
    for (id, revision, summary) in [
        (
            "handoff-old-cache",
            "ctx-old-cache",
            "OLD_REVISION_SUMMARY_SHOULD_NOT_BE_SENT",
        ),
        (
            "handoff-current-cache",
            "ctx-current-cache",
            "CURRENT_REVISION_SUMMARY_SHOULD_BE_SENT",
        ),
    ] {
        store.save_handoff_segment("cache-handoff-session",id,1,&json!({"id":id,"sessionId":"cache-handoff-session","contextRevisionId":revision,"sequence":1,"startEventIndex":0,"endEventIndex":2,"summary":summary,"mode":"manual","estimatedTokens":8,"active":true}).to_string()).expect("handoff");
    }
    let runtime = runtime_with_production_catalog(
        &directory,
        &store,
        &sessions,
        Arc::new(RecordingToolExecutor::new(Vec::new())),
    );
    chat(
        &runtime,
        "cache-handoff-agent",
        Some("cache-handoff-session"),
        "use current handoff",
        "11111111-1111-4111-8111-111111111116",
    );
    runtime.shutdown();
    let requests = capture.finish();
    assert_eq!(requests.len(), 1);
    let input = requests[0]["input"].as_array().expect("provider input");
    assert!(
        input[0]["content"]
            .as_str()
            .expect("instruction")
            .contains("Base stable instruction.")
    );
    let content = serde_json::to_string(input).expect("input");
    assert!(content.contains("CURRENT_REVISION_SUMMARY_SHOULD_BE_SENT"));
    assert!(
        !content.contains("OLD_REVISION_SUMMARY_SHOULD_NOT_BE_SENT"),
        "old active handoff leaked: {content}"
    );
    for needle in [
        "jftrade:handoff_summary",
        "jftrade-handoff-state",
        "ctx-current-cache",
        "ctx-old-cache",
    ] {
        assert!(!content.contains(needle), "{needle}: {content}");
    }
    // The Rust wire puts durable summaries in a separate system item. Keep
    // the original first-system-message placement requirement as a partial.
    assert!(
        !input[0]["content"]
            .as_str()
            .expect("instruction")
            .contains("CURRENT_REVISION_SUMMARY_SHOULD_BE_SENT")
    );
    assert_eq!(
        store
            .list_handoff_segments("cache-handoff-session", true)
            .expect("retained handoffs")
            .len(),
        2
    );
}

#[test]
fn durable_model_context_without_a_revision_never_adopts_legacy_handoff_cutoffs_or_summaries() {
    let (_directory, store, sessions) = initialized_stores();
    sessions
        .upsert_session("jftrade", "local", "legacy-session", "{}")
        .expect("session");
    sessions
        .record_event(jftrade_store_sqlite::RecordAdkEventParams {
            id: "legacy-user",
            app_name: "jftrade",
            user_id: "local",
            session_id: "legacy-session",
            invocation_id: "previous-turn",
            author: "user",
            content: "durable user history",
        })
        .expect("user history");
    store
        .save_handoff_segment(
            "legacy-session",
            "unversioned",
            1,
            &json!({"active":true,"endEventIndex":100,"summary":"UNVERSIONED_HANDOFF"}).to_string(),
        )
        .expect("legacy segment");
    store.save_handoff_segment("legacy-session", "unanchored", 2,
        &json!({"active":true,"contextRevisionId":"unanchored-revision","endEventIndex":100,"summary":"UNANCHORED_HANDOFF"}).to_string()).expect("unanchored segment");
    let raw = store
        .list_handoff_segments("legacy-session", true)
        .expect("raw handoffs");
    for state in [
        None,
        Some(json!({"contextRevisionId":"ctx-current","compactedEventCount":0})),
    ] {
        if let Some(state) = state {
            store
                .upsert_session_context("legacy-session", &state.to_string())
                .expect("state");
        }
        let input = super::super::durable_context_items(&store, &sessions, "legacy-session", None)
            .expect("durable input");
        assert_eq!(
            input,
            vec![json!({"role":"user","content":"durable user history"})]
        );
        assert_eq!(
            store
                .list_handoff_segments("legacy-session", true)
                .expect("retained rows"),
            raw
        );
    }
}
