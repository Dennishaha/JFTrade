use super::*;
use crate::product::product_adk_chat_stream_port::{AdkChatInput, AdkChatRoute, AdkChatStreamPort};
use sha2::Digest;

#[tokio::test]
async fn production_stream_preflight_rejects_corrupt_durable_payload_before_conflict_or_cached_replay()
 {
    let _ = rustls::crypto::ring::default_provider().install_default();
    for (ready, same_fingerprint, payload) in [
        (false, true, "malformed"),
        (true, true, "malformed"),
        (false, false, "malformed"),
        (true, false, "malformed"),
        (false, true, "[]"),
        (true, true, r#""invalid run""#),
    ] {
        let (_directory, port, provider) = retained_readiness::configured_port();
        let input = AdkChatInput {
            client_request_id: "11111111-1111-4111-8111-111111111115".to_owned(),
            body:
                br#"{"clientRequestId":"11111111-1111-4111-8111-111111111115","message":"hello"}"#
                    .to_vec(),
        };
        let crate::product::product_adk_chat_stream_port::AdkChatPortOutput::LiveStream(retained) =
            port.dispatch(AdkChatRoute::Stream, &input).unwrap()
        else {
            panic!("retained terminal error before configuration")
        };
        let id = retained.headers["X-ADK-Stream-ID"].clone();
        let mut reader = retained.stream.take_body().unwrap();
        assert_eq!(reader.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
        let initial = reader.next().await.unwrap().unwrap();
        assert!(reader.next().await.is_none());
        let fingerprint = if same_fingerprint {
            sha2::Sha256::digest(&input.body)
                .iter()
                .map(|byte| format!("{byte:02x}"))
                .collect::<String>()
        } else {
            "different".to_owned()
        };
        port.store
            .create_run(CreateAdkRunParams {
                id: "run-corrupt-preflight",
                session_id: "session-preflight",
                agent_id: "agent-preflight",
                status: "COMPLETED",
                client_request_id: &input.client_request_id,
                request_fingerprint: &fingerprint,
                payload_json: payload,
            })
            .unwrap();
        let runs = port.store.list_runs().unwrap();
        let audit = port.store.list_audit_events().unwrap();
        retained_readiness::set_provider(&port, &provider, ready);
        assert_eq!(port.runtime_ready(), ready);
        let prepared = prepared_router_with_chat(port.clone(), true).await;
        let handle = crate::product::expose_prepared_product(prepared).unwrap();
        let base = format!("http://{}", handle.startup_record().address);
        for body in [
            input.body.clone(),
            br#"{"clientRequestId":"11111111-1111-4111-8111-111111111115","message":"changed"}"#
                .to_vec(),
        ] {
            let response = reqwest::Client::new()
                .post(format!("{base}/api/v1/adk/chat/stream"))
                .header("content-type", "application/json")
                .body(body)
                .send()
                .await
                .unwrap();
            assert_eq!(
                response.status(),
                500,
                "ready={ready},same={same_fingerprint},payload={payload}"
            );
            assert!(response.headers().get("x-adk-stream-id").is_none());
            let error = response.json::<Value>().await.unwrap();
            assert_eq!(error["error"]["code"], "ADK_CHAT_FAILED");
            assert!(!error["error"]["message"].as_str().unwrap().is_empty());
        }
        let response = reqwest::get(format!("{base}/api/v1/adk/streams/{id}"))
            .await
            .unwrap();
        assert_eq!(response.status(), 200);
        let replay = response.text().await.unwrap();
        let mut event: Value = serde_json::from_slice(
            initial
                .split(|byte| *byte == b'\n')
                .find_map(|line| line.strip_prefix(b"data: "))
                .unwrap(),
        )
        .unwrap();
        event["replay"] = json!(true);
        let actual: Value = serde_json::from_str(
            replay
                .lines()
                .find_map(|line| line.strip_prefix("data: "))
                .unwrap(),
        )
        .unwrap();
        assert_eq!(actual, event);
        handle.shutdown().await.unwrap();
        port.shutdown_with_error().unwrap();
        assert_eq!(port.store.list_runs().unwrap(), runs);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
        assert_eq!(
            provider.accept().unwrap_err().kind(),
            io::ErrorKind::WouldBlock
        );
    }
}
