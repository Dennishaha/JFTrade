use super::*;
use crate::product::product_adk_chat_stream_port::{AdkChatInput, AdkChatRoute, AdkChatStreamPort};
use sha2::Digest;

#[tokio::test]
async fn production_sync_chat_decodes_durable_fields_before_saved_response_or_conflict() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    let request = "11111111-1111-4111-8111-111111111112";
    port.store.upsert_session("session-sync-typed","agent-sync-typed",
        r#"{"id":"session-sync-typed","agentId":"agent-sync-typed"}"#).unwrap();
    port.session_store.upsert_session("jftrade","local","session-sync-typed","{}").unwrap();
    let original = json!({"clientRequestId":request,"message":"hello"}).to_string();
    let fingerprint = sha2::Sha256::digest(original.as_bytes()).iter()
        .map(|byte| format!("{byte:02x}")).collect::<String>();
    port.store.create_run(CreateAdkRunParams {
        id:"run-sync-typed",session_id:"session-sync-typed",agent_id:"agent-sync-typed",
        status:"COMPLETED",client_request_id:request,request_fingerprint:&fingerprint,payload_json:"{}",
    }).unwrap();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let url = format!("http://{}/api/v1/adk/chat",handle.startup_record().address);
    let mut observed = Vec::new();
    for fields in [
        r#""message":42"#,r#""toolCalls":"invalid""#,
        r#""pendingApprovals":[{"input":false}]"#,
        r#""usage":{"modelCalls":true}"#,
        r#""inputRequest":{"questions":[{"allowOther":"false"}]}"#,
        r#""message":42,"MESSAGE":"later valid message""#,
        r#""message":"hello", "maxDurationMs":1e3"#,
        r#""message":"hello", "toolCalls":[{"output":1e309}]"#,
    ] {
        let payload = format!(r#"{{"route":"chat","response":{{"reply":"saved winner"}},{fields}}}"#);
        port.store.update_run_state("run-sync-typed","COMPLETED",&payload).unwrap();
        let runs = port.store.list_runs().unwrap();
        let audit = port.store.list_audit_events().unwrap();
        let native = port.session_store.list_sessions().unwrap();
        for message in ["hello","changed"] {
            let response = reqwest::Client::new().post(&url)
                .json(&json!({"clientRequestId":request,"message":message}))
                .send().await.unwrap();
            observed.push((fields,message,response.status(),response.text().await.unwrap()));
        }
        assert_eq!(port.store.list_runs().unwrap(),runs);
        assert_eq!(port.store.list_audit_events().unwrap(),audit);
        assert_eq!(port.session_store.list_sessions().unwrap(),native);
    }
    let valid = r#"{"id":"run-sync-typed","sessionId":"session-sync-typed","agentId":"agent-sync-typed","status":"COMPLETED","route":"chat","message":null,"usage":null,"toolCalls":null,"message":"later value","future":{"message":42},"response":{"reply":"saved winner"}}"#;
    port.store.update_run_state("run-sync-typed","COMPLETED",valid).unwrap();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let mut valid_results = Vec::new();
    for message in ["hello","changed"] {
        let response = reqwest::Client::new().post(&url)
            .json(&json!({"clientRequestId":request,"message":message}))
            .send().await.unwrap();
        valid_results.push((message,response.status(),response.json::<Value>().await.unwrap()));
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    assert_eq!(provider.accept().unwrap_err().kind(),io::ErrorKind::WouldBlock);
    for (fields,message,status,body) in observed {
        assert_eq!(status,400,"fields={fields},message={message},body={body}");
        let error:Value=serde_json::from_str(&body).unwrap();
        assert_eq!(error["error"]["code"],"ADK_CHAT_FAILED");
        assert!(!error["error"]["message"].as_str().unwrap().is_empty());
        assert!(error.get("data").is_none());
    }
    assert_eq!(valid_results[0].1,200);
    assert_eq!(valid_results[0].2["data"]["reply"],"saved winner");
    assert_eq!(valid_results[1].1,409);
    assert_eq!(valid_results[1].2["error"]["code"],"ADK_CHAT_IDEMPOTENCY_CONFLICT");
    assert_eq!(port.store.list_runs().unwrap(),runs);
    assert_eq!(port.store.list_audit_events().unwrap(),audit);
}

#[tokio::test]
async fn production_stream_preflight_rejects_typed_run_corruption_before_replay_or_conflict() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let mut observed = Vec::new();
    for (ready, cached) in [(false, true), (true, true), (true, false)] {
        let (_directory, port, provider) = retained_readiness::configured_port();
        let input = AdkChatInput {
            client_request_id: "11111111-1111-4111-8111-111111111114".to_owned(),
            body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111114","message":"hello"}"#.to_vec(),
        };
        // Retain a transport record before creating the authoritative row.
        if cached {
            drop(port.dispatch(AdkChatRoute::Stream, &input).unwrap());
        }
        let fingerprint = sha2::Sha256::digest(&input.body).iter()
            .map(|byte| format!("{byte:02x}")).collect::<String>();
        port.store.create_run(CreateAdkRunParams {
            id: "run-typed-preflight", session_id: "session-typed", agent_id: "agent-typed",
            status: "COMPLETED", client_request_id: &input.client_request_id,
            request_fingerprint: &fingerprint, payload_json: "{}",
        }).unwrap();
        retained_readiness::set_provider(&port, &provider, ready);
        let prepared = prepared_router_with_chat(port.clone(), true).await;
        let handle = crate::product::expose_prepared_product(prepared).unwrap();
        let url = format!("http://{}/api/v1/adk/chat/stream", handle.startup_record().address);
        for fields in [
            r#""message":42"#, r#""MeSsAgE":42"#,
            r#""message":42,"message":"later valid value""#,
            r#""toolCalls":"not an array""#,
            r#""toolCalls":[{"requiresUser":"true"}]"#,
            r#""pendingApprovals":[{"input":[]}]"#,
            r#""inputRequest":{"questions":[{"options":[{"recommended":1}]}]}"#,
            r#""inputRequests":[{"answers":[{"questionId":false}]}]"#,
            r#""workflowPlan":[{"order":1.5}]"#,
            r#""usage":{"tokensIn":9223372036854775808}"#,
            r#""maxDurationMs":1e3"#, r#""reasoningEffortField":[]"#,
            r#""completedAt":true"#,
        ] {
            let payload = format!(r#"{{"route":"stream","streamId":"stream-typed","response":{{"reply":"cached answer"}},{fields}}}"#);
            port.store.update_run_state("run-typed-preflight", "COMPLETED", &payload).unwrap();
            let runs = port.store.list_runs().unwrap();
            let audit = port.store.list_audit_events().unwrap();
            let native = port.session_store.list_sessions().unwrap();
            for message in ["hello", "changed"] {
                let response = reqwest::Client::new().post(&url)
                    .json(&json!({"clientRequestId":input.client_request_id,"message":message}))
                    .send().await.unwrap();
                let status = response.status();
                let has_id = response.headers().contains_key("x-adk-stream-id");
                let body = response.text().await.unwrap();
                observed.push((ready, cached, fields, message, status, has_id, body));
            }
            assert_eq!(port.store.list_runs().unwrap(), runs);
            assert_eq!(port.store.list_audit_events().unwrap(), audit);
            assert_eq!(port.session_store.list_sessions().unwrap(), native);
        }
        handle.shutdown().await.unwrap();
        port.shutdown_with_error().unwrap();
        assert_eq!(provider.accept().unwrap_err().kind(), io::ErrorKind::WouldBlock);
    }
    for (ready, cached, fields, message, status, has_id, body) in observed {
        assert_eq!(status, 500, "ready={ready},cached={cached},fields={fields},message={message},body={body}");
        assert!(!has_id, "typed decode failure must precede the stream handshake");
        let error: Value = serde_json::from_str(&body).unwrap();
        assert_eq!(error["error"]["code"], "ADK_CHAT_FAILED");
        assert!(!error["error"]["message"].as_str().unwrap().is_empty());
    }
}

#[tokio::test]
async fn production_stream_preflight_accepts_persisted_null_duplicate_and_extended_run_fields() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port, provider) = retained_readiness::configured_port();
    retained_readiness::set_provider(&port, &provider, true);
    let input = AdkChatInput {
        client_request_id: "11111111-1111-4111-8111-111111111113".to_owned(),
        body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111113","message":"hello"}"#.to_vec(),
    };
    let fingerprint = sha2::Sha256::digest(&input.body).iter()
        .map(|byte| format!("{byte:02x}")).collect::<String>();
    port.store.create_run(CreateAdkRunParams {
        id: "run-valid-typed", session_id: "session-typed", agent_id: "agent-typed",
        status: "COMPLETED", client_request_id: &input.client_request_id,
        request_fingerprint: &fingerprint, payload_json: "{}",
    }).unwrap();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let url = format!("http://{}/api/v1/adk/chat/stream", handle.startup_record().address);
    let mut responses = Vec::new();
    for fields in [
        r#""message":null,"toolCalls":null,"usage":null,"completedAt":null"#,
        r#""streamEvents":null,"message":"legacy row""#,
        r#""message":"old","message":"new","MESSAGE":null"#,
        r#""toolCalls":[null,{"input":null,"output":[true,42],"error":null}]"#,
        r#""pendingApprovals":[null,{"input":{"anything":[1,true]}}],"inputRequests":[null,{"questions":[null,{"options":[null]}]}]"#,
        r#""workflowPlan":[null,{"dependsOn":[null,"run"],"order":-1}],"usage":{"tokensIn":-1},"maxDurationMs":9223372036854775807"#,
        r#""reasoningEffort":"future-effort","createdAt":"opaque legacy timestamp","future":{"message":42},"response":{"reply":"cached answer","run":{"message":42}}"#,
    ] {
        let payload = format!(r#"{{"route":"stream","streamId":"stream-valid-typed","response":{{"reply":"cached answer"}},{fields}}}"#);
        port.store.update_run_state("run-valid-typed", "COMPLETED", &payload).unwrap();
        let runs = port.store.list_runs().unwrap();
        let audit = port.store.list_audit_events().unwrap();
        let response = reqwest::Client::new().post(&url)
            .header("content-type", "application/json").body(input.body.clone())
            .send().await.unwrap();
        let status = response.status();
        let id = response.headers().get("x-adk-stream-id").cloned();
        responses.push((fields, status, id, response.text().await.unwrap()));
        assert_eq!(port.store.list_runs().unwrap(), runs);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
    }
    handle.shutdown().await.unwrap();
    port.shutdown_with_error().unwrap();
    for (fields, status, id, body) in responses {
        assert_eq!(status, 200, "fields={fields},body={body}");
        assert_eq!(id.unwrap(), "stream-valid-typed");
        let event: Value = serde_json::from_str(body.lines().find_map(|line|line.strip_prefix("data: ")).unwrap()).unwrap();
        assert_eq!(event["type"], "final");
        assert_eq!(event["response"]["reply"], "cached answer");
        assert_eq!(event["replay"], true);
    }
    assert_eq!(provider.accept().unwrap_err().kind(), io::ErrorKind::WouldBlock);
}

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
