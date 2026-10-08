use super::*;
use tokio_stream::StreamExt;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};

fn response_events() -> String {
    [
        json!({"type":"response.created","response":{"id":"resp-idempotent","model":"fixture-model"}}),
        json!({"type":"response.output_text.delta","delta":"single answer"}),
        json!({"type":"response.completed","response":{"id":"resp-idempotent","model":"fixture-model","usage":{"total_tokens":2}}}),
    ].into_iter().map(|event|format!("data: {event}\n\n")).collect::<String>()+"data: [DONE]\n\n"
}

#[tokio::test]
async fn production_sync_responses_rejects_incomplete_malformed_and_failed_streams() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    for (body,message) in [
        ("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: [DONE]\n\n","ended before response.completed"),
        ("data: malformed\n\n","decode model stream event"),
        ("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"provider failed turn\"}}}\n\n","provider failed turn"),
    ] {
        let (_directory,port,provider)=retained_readiness::configured_port();
        retained_readiness::set_provider(&port,&provider,true);
        port.store.upsert_agent("agent-sync-failure",&json!({"id":"agent-sync-failure","providerId":"provider-readiness","model":"fixture-model"}).to_string()).unwrap();
        let calls=Arc::new(AtomicUsize::new(0));let observed=calls.clone();
        let mock=axum::Router::new().route("/v1/responses",axum::routing::post(move || {
            observed.fetch_add(1,Ordering::SeqCst);
            async move { ([("content-type","text/event-stream")],body) }
        }));
        let socket=TcpListener::from_std(provider).unwrap();let (stop,stopped)=oneshot::channel();
        let server=tokio::spawn(axum::serve(socket,mock).with_graceful_shutdown(async {let _=stopped.await;}).into_future());
        let input=AdkChatInput{client_request_id:"abcdefab-cdef-4abc-8def-abcdefabcdef".to_owned(),body:json!({"clientRequestId":"abcdefab-cdef-4abc-8def-abcdefabcdef","agentId":"agent-sync-failure","message":"hello"}).to_string().into_bytes()};
        let active=port.clone();
        let result=tokio::task::spawn_blocking(move ||active.dispatch(AdkChatRoute::Chat,&input)).await.unwrap().unwrap();
        let AdkChatPortOutput::Json(response)=result else {panic!("persisted failed projection")};
        assert_eq!(response["run"]["status"],"FAILED");
        let runs=port.store.list_runs().unwrap();assert_eq!(runs.len(),1);assert_eq!(runs[0].status,"FAILED");
        let stored:Value=serde_json::from_str(&runs[0].payload_json).unwrap();
        assert_eq!(stored["errorCode"],"MODEL_CALL_FAILED");
        assert!(stored["errorMessage"].as_str().unwrap().contains(message),"{stored}");
        assert_ne!(response["reply"],"partial");
        port.shutdown_with_error().unwrap();stop.send(()).unwrap();server.await.unwrap().unwrap();
        assert_eq!(calls.load(Ordering::SeqCst),1);
    }
}

#[tokio::test]
async fn production_sync_responses_cancellation_ends_idle_body_and_preserves_cancelled_owner() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory,port,provider)=retained_readiness::configured_port();
    retained_readiness::set_provider(&port,&provider,true);
    port.store.upsert_agent("agent-sync-cancel",&json!({"id":"agent-sync-cancel","providerId":"provider-readiness","model":"fixture-model"}).to_string()).unwrap();
    let entered=Arc::new(tokio::sync::Semaphore::new(0));let reached=entered.clone();
    let calls=Arc::new(AtomicUsize::new(0));let observed=calls.clone();
    let mock=axum::Router::new().route("/v1/responses",axum::routing::post(move || {
        observed.fetch_add(1,Ordering::SeqCst);let entered=reached.clone();
        async move {
            let body=tokio_stream::once(()).map(move |_| {
                entered.add_permits(1);
                Ok::<_,io::Error>(b": provider idle\n\n".to_vec())
            }).chain(tokio_stream::pending());
            ([("content-type","text/event-stream")],axum::body::Body::from_stream(body))
        }
    }));
    let socket=TcpListener::from_std(provider).unwrap();let (stop,stopped)=oneshot::channel();
    let server=tokio::spawn(axum::serve(socket,mock).with_graceful_shutdown(async {let _=stopped.await;}).into_future());
    let input=AdkChatInput{client_request_id:"abcdefab-cdef-4abc-8def-abcdefabcdef".to_owned(),body:json!({"clientRequestId":"abcdefab-cdef-4abc-8def-abcdefabcdef","agentId":"agent-sync-cancel","message":"hello"}).to_string().into_bytes()};
    let active=port.clone();let chat=tokio::task::spawn_blocking(move ||active.dispatch(AdkChatRoute::Chat,&input));
    tokio::time::timeout(Duration::from_secs(10),entered.acquire()).await.unwrap().unwrap().forget();
    let runs=port.store.list_runs().unwrap();assert_eq!(runs.len(),1);
    assert!(port.cancel_run(&runs[0].id));
    let result=tokio::time::timeout(Duration::from_secs(3),chat).await.unwrap().unwrap();
    assert!(matches!(result,Err(AdkChatPortError::Failed{status:499,code,..}) if code=="CLIENT_DISCONNECTED"));
    let runs=port.store.list_runs().unwrap();assert_eq!(runs[0].status,"CANCELLED");
    let events=port.session_store.list_events(&runs[0].session_id).unwrap();
    let audit=port.store.list_audit_events().unwrap();
    port.shutdown_with_error().unwrap();stop.send(()).unwrap();
    tokio::time::timeout(Duration::from_secs(3),server).await.unwrap().unwrap().unwrap();
    assert_eq!(calls.load(Ordering::SeqCst),1);
    assert_eq!(port.store.list_runs().unwrap(),runs);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert_eq!(port.session_store.list_events(&runs[0].session_id).unwrap(),events);
}

// Parity: go:452dea11:internal/assistant/engine/chat_request_idempotency_test.go:48 TestConcurrentResponsesRequestReusesOneRunAndNativeAssistantEvent
#[tokio::test]
async fn production_concurrent_sync_responses_reuse_one_run_and_link_one_native_assistant_answer() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    port.store.upsert_agent("agent-concurrent",&json!({"id":"agent-concurrent","name":"Concurrent","providerId":"provider-readiness","model":"fixture-model"}).to_string()).unwrap();
    port.store.upsert_session("session-concurrent","agent-concurrent",&json!({"id":"session-concurrent","agentId":"agent-concurrent","title":"idempotency"}).to_string()).unwrap();
    let calls = Arc::new(AtomicUsize::new(0));
    let entered = Arc::new(tokio::sync::Semaphore::new(0));
    let (release,released) = oneshot::channel();
    let released = Arc::new(tokio::sync::Mutex::new(Some(released)));
    let observed = calls.clone();
    let reached = entered.clone();
    let mock = axum::Router::new().route("/v1/responses",axum::routing::post(move |axum::Json(body):axum::Json<Value>| {
        let calls=observed.clone();let entered=reached.clone();let released=released.clone();
        async move {
            assert_eq!(body["stream"],false);
            assert_eq!(calls.fetch_add(1,Ordering::SeqCst),0,"only the durable owner calls provider");
            entered.add_permits(1);
            released.lock().await.take().unwrap().await.unwrap();
            ([("content-type","text/event-stream")],response_events())
        }
    }));
    let socket=TcpListener::from_std(provider).unwrap();
    let (stop,stopped)=oneshot::channel();
    let server=tokio::spawn(axum::serve(socket,mock).with_graceful_shutdown(async {let _=stopped.await;}).into_future());
    let input = AdkChatInput {
        client_request_id:"abcdefab-cdef-4abc-8def-abcdefabcdef".to_owned(),
        body:json!({"clientRequestId":"abcdefab-cdef-4abc-8def-abcdefabcdef","agentId":"agent-concurrent","sessionId":"session-concurrent","message":"hello"}).to_string().into_bytes(),
    };
    let first_port=port.clone();let first_input=input.clone();
    let first=tokio::task::spawn_blocking(move ||first_port.dispatch(AdkChatRoute::Chat,&first_input));
    tokio::time::timeout(Duration::from_secs(10),entered.acquire()).await.unwrap().unwrap().forget();
    // The first owner is held at the provider; the second request must reuse
    // its RUNNING projection, as Go ChatResponseForExistingRun does.
    let second_port=port.clone();let second_input=input.clone();
    let second=tokio::task::spawn_blocking(move ||second_port.dispatch(AdkChatRoute::Chat,&second_input)).await.unwrap();
    release.send(()).unwrap();
    let first=first.await.unwrap();
    let AdkChatPortOutput::Json(first)=first.unwrap() else {panic!("first Chat must succeed")};
    let AdkChatPortOutput::Json(second)=second.unwrap() else {panic!("second Chat must succeed")};
    let run_id=first["run"]["id"].as_str().filter(|id|!id.is_empty()).unwrap();
    assert_eq!(second["run"]["id"],run_id);
    assert_eq!(first["run"]["status"],"COMPLETED");
    assert_eq!(first["reply"],"single answer");
    assert_eq!(calls.load(Ordering::SeqCst),1);
    let runs=port.store.list_runs().unwrap();assert_eq!(runs.len(),1);
    let stored:Value=serde_json::from_str(&runs[0].payload_json).unwrap();
    let final_id=stored["finalMessageId"].as_str().filter(|id|!id.is_empty()).unwrap();
    let events=port.session_store.list_events("session-concurrent").unwrap();
    let native=events.iter().filter(|event|event.author=="agent-concurrent").collect::<Vec<_>>();
    assert_eq!(native.len(),1);
    assert_eq!(native[0].id,final_id);
    assert_eq!(native[0].content,"single answer");
    assert_eq!(native[0].invocation_id,run_id);
    let audit=port.store.list_audit_events().unwrap();
    let replay_port=port.clone();let replay_input=input.clone();
    let replay=tokio::task::spawn_blocking(move ||replay_port.dispatch(AdkChatRoute::Chat,&replay_input)).await.unwrap().unwrap();
    let AdkChatPortOutput::Json(replay)=replay else {panic!("replay JSON")};
    assert_eq!(replay,first);
    let mut changed:Value=serde_json::from_slice(&input.body).unwrap();changed["message"]=json!("different");
    let conflict_port=port.clone();let conflict_input=AdkChatInput{body:changed.to_string().into_bytes(),..input};
    let conflict=tokio::task::spawn_blocking(move ||conflict_port.dispatch(AdkChatRoute::Chat,&conflict_input)).await.unwrap();
    assert!(matches!(conflict,Err(AdkChatPortError::Conflict(_))));
    port.shutdown_with_error().unwrap();stop.send(()).unwrap();server.await.unwrap().unwrap();
    assert_eq!(calls.load(Ordering::SeqCst),1);
    assert_eq!(port.store.list_runs().unwrap(),runs);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
    assert_eq!(port.session_store.list_events("session-concurrent").unwrap(),events);
}
