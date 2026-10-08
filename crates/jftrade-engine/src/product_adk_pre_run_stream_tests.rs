use super::*;
use crate::product::product_adk_chat_stream_port::{AdkChatRoute, AdkChatStreamPort};

fn ready_port() -> (
    tempfile::TempDir,
    Arc<ProductionAdkPort>,
    std::net::TcpListener,
) {
    let (directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    assert!(port.runtime_ready());
    (directory, port, provider)
}

async fn replay_error(port: &ProductionAdkPort, id: &str, message: &str) {
    let replay = port
        .open_stream(&format!("/api/v1/adk/streams/{id}"), "")
        .unwrap()
        .unwrap();
    assert_eq!(
        replay.headers,
        vec![("X-ADK-Stream-ID".to_owned(), id.to_owned())]
    );
    let mut body = replay.body.take_body().unwrap();
    assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    let frame = String::from_utf8(body.next().await.unwrap().unwrap()).unwrap();
    assert!(frame.contains(&format!("id: {id}:1\n")), "{frame}");
    let event: Value = serde_json::from_str(
        frame
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(event["type"], "error");
    assert_eq!(event["streamId"], id);
    assert_eq!(event["sequence"], 1);
    assert_eq!(event["message"], message);
    assert_eq!(event["replay"], true);
    assert!(event.get("runId").is_none());
    assert!(body.next().await.is_none());
}

// Parity: go:452dea11:internal/api/assistant/chat_helpers_test.go:216 TestExecuteADKChatStreamPublishesTerminalErrorForInvalidRequest
#[tokio::test]
async fn production_configured_stream_preserves_prepare_errors_after_retry_disconnect() {
    let (_directory, port, provider) = ready_port();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let sessions = port.store.list_sessions().unwrap();
    let native_sessions = port.session_store.list_sessions().unwrap();
    for (number, fields, message) in [
        (1, json!({"message":"   "}), "message is required"),
        (
            2,
            json!({"message":"hello","agentId":"missing-agent"}),
            "agent not found",
        ),
        (3, json!({"message":"hello","permissionModeOverride":"invalid"}), "invalid permission mode \"invalid\""),
        (4, json!({"message":"hello","workModeOverride":"invalid"}), "invalid work mode \"invalid\""),
        (5, json!({"message":"hello","reasoningEffortOverride":"invalid"}), "invalid reasoning effort \"invalid\""),
    ] {
        let mut payload = fields;
        payload["clientRequestId"] = json!(format!("11111111-1111-4111-8111-{number:012}"));
        let bytes = payload.to_string().into_bytes();
        let id = fail_stream_write(
            port.clone(),
            "/api/v1/adk/chat/stream",
            Some(&bytes),
            b"retry: 3000",
            Some("stream-"),
        )
        .await
        .unwrap();
        replay_error(&port, &id, message).await;
        let input = crate::product::product_adk_chat_stream_port::AdkChatInput {
            body: bytes,
            client_request_id: payload["clientRequestId"].as_str().unwrap().to_owned(),
        };
        let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(replay) =
            port.dispatch(AdkChatRoute::Stream, &input).unwrap()
        else {
            panic!("same retained error stream")
        };
        assert_eq!(replay.headers["X-ADK-Stream-ID"], id);
        replay_error(&port, &id, message).await;
        assert!(matches!(
            port.dispatch(AdkChatRoute::Chat, &input),
            Err(
                crate::product::product_adk_chat_stream_port::AdkChatPortError::Failed {
                    status: 400,
                    ..
                }
            )
        ));
        assert_eq!(port.store.list_runs().unwrap(), runs);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
        assert_eq!(port.store.list_sessions().unwrap(), sessions);
        assert_eq!(port.session_store.list_sessions().unwrap(), native_sessions);
    }
    port.shutdown_with_error().unwrap();
    assert_eq!(
        provider.accept().unwrap_err().kind(),
        io::ErrorKind::WouldBlock
    );
}

#[tokio::test]
async fn production_configured_stream_keeps_existing_durable_payload_failure_out_of_memory_owner() {
    use sha2::Digest;
    let (_directory, port, provider) = ready_port();
    let input = crate::product::product_adk_chat_stream_port::AdkChatInput {
        client_request_id:"11111111-1111-4111-8111-111111111119".to_owned(),
        body:br#"{"clientRequestId":"11111111-1111-4111-8111-111111111119","message":"hello"}"#.to_vec(),
    };
    let fingerprint = sha2::Sha256::digest(&input.body).iter()
        .map(|byte|format!("{byte:02x}")).collect::<String>();
    port.store.create_run(CreateAdkRunParams {
        id:"run-malformed",session_id:"session-malformed",agent_id:"agent-malformed",
        status:"COMPLETED",client_request_id:&input.client_request_id,
        request_fingerprint:&fingerprint,payload_json:"malformed durable payload",
    }).unwrap();
    let before = port.store.list_runs().unwrap();
    let error = port.dispatch(AdkChatRoute::Stream,&input).unwrap_err();
    assert!(matches!(error,crate::product::product_adk_chat_stream_port::AdkChatPortError::Unavailable(_)));
    let changed = crate::product::product_adk_chat_stream_port::AdkChatInput {
        body:br#"{"message":"changed"}"#.to_vec(), ..input.clone()
    };
    assert!(matches!(port.dispatch(AdkChatRoute::Stream,&changed),
        Err(crate::product::product_adk_chat_stream_port::AdkChatPortError::Conflict(_))));
    assert_eq!(port.unavailable_streams.retained_request_count(), 0);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_runs().unwrap(),before);
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}

#[tokio::test]
async fn production_configured_stream_keeps_post_creation_context_failure_with_durable_owner() {
    let (directory,port,provider)=ready_port();
    port.store.upsert_agent("agent-context",&json!({"id":"agent-context","name":"Context Agent",
        "providerId":"provider-readiness","model":"fixture-model"}).to_string()).unwrap();
    port.store.upsert_session("session-context","agent-context",r#"{"id":"session-context","agentId":"agent-context"}"#).unwrap();
    port.session_store.upsert_session("jftrade","local","session-context","{}").unwrap();
    port.store.upsert_session_context("session-context","{}").unwrap();
    let connection=rusqlite::Connection::open(directory.path().join("adk.db")).unwrap();
    connection.execute_batch("CREATE TRIGGER break_context_after_run_creation AFTER INSERT ON adk_runs BEGIN UPDATE adk_session_context_state SET payload_json='malformed' WHERE id='session-context'; END;").unwrap();
    let input=crate::product::product_adk_chat_stream_port::AdkChatInput {
        client_request_id:"11111111-1111-4111-8111-111111111118".to_owned(),
        body:br#"{"clientRequestId":"11111111-1111-4111-8111-111111111118","message":"hello","agentId":"agent-context","sessionId":"session-context"}"#.to_vec(),
    };
    let error=port.dispatch(AdkChatRoute::Stream,&input).unwrap_err();
    assert!(matches!(error,crate::product::product_adk_chat_stream_port::AdkChatPortError::Failed {status:500,code,..} if code=="ADK_STORAGE_CORRUPT"));
    let run=port.store.get_run_by_client_request_id(&input.client_request_id).unwrap().unwrap();
    assert_eq!(run.status,"RUNNING");
    assert_eq!(port.session_store.list_events("session-context").unwrap().len(),1);
    assert_eq!(port.unavailable_streams.retained_request_count(), 0);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run(&run.id).unwrap().unwrap(),run);
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}

#[tokio::test]
async fn production_configured_stream_concurrent_prepare_errors_share_one_retained_identity() {
    let (_directory,port,provider)=ready_port();
    let input=crate::product::product_adk_chat_stream_port::AdkChatInput {
        client_request_id:"11111111-1111-4111-8111-111111111117".to_owned(),
        body:br#"{"clientRequestId":"11111111-1111-4111-8111-111111111117","message":""}"#.to_vec(),
    };
    let barrier=Arc::new(std::sync::Barrier::new(6));
    let workers=(0..6).map(|_|{
        let port=port.clone();let input=input.clone();let barrier=barrier.clone();
        std::thread::spawn(move || {barrier.wait();port.dispatch(AdkChatRoute::Stream,&input).unwrap()})
    }).collect::<Vec<_>>();
    let ids=workers.into_iter().map(|worker|{
        let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(stream)=worker.join().unwrap()
        else {panic!("retained error")};stream.headers["X-ADK-Stream-ID"].clone()
    }).collect::<Vec<_>>();
    assert!(ids.iter().all(|id|id==&ids[0]));
    replay_error(&port,&ids[0],"message is required").await;
    assert!(port.store.list_runs().unwrap().is_empty());
    port.shutdown_with_error().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}

#[tokio::test]
async fn production_configured_stream_existing_identity_keeps_early_validation_with_durable_owner() {
    use sha2::Digest;
    let (_directory,port,provider)=ready_port();
    let input=crate::product::product_adk_chat_stream_port::AdkChatInput {
        client_request_id:"11111111-1111-4111-8111-111111111116".to_owned(),
        body:br#"{"clientRequestId":"11111111-1111-4111-8111-111111111116","message":""}"#.to_vec(),
    };
    let fingerprint=sha2::Sha256::digest(&input.body).iter().map(|byte|format!("{byte:02x}")).collect::<String>();
    port.store.create_run(CreateAdkRunParams {
        id:"run-existing-empty",session_id:"session-existing-empty",agent_id:"agent-existing-empty",
        status:"COMPLETED",client_request_id:&input.client_request_id,
        request_fingerprint:&fingerprint,payload_json:r#"{"route":"stream"}"#,
    }).unwrap();
    let before=port.store.list_runs().unwrap();
    assert!(matches!(port.dispatch(AdkChatRoute::Stream,&input),
        Err(crate::product::product_adk_chat_stream_port::AdkChatPortError::Failed {status:400,message,..}) if message=="message is required"));
    assert_eq!(port.unavailable_streams.retained_request_count(),0);
    port.shutdown_with_error().unwrap();
    assert_eq!(port.store.list_runs().unwrap(),before);
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
}
