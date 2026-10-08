use super::*;

#[derive(Debug)]
struct UncertainWrite {
    calls: Arc<AtomicUsize>,
    structured: bool,
}

impl AdkToolExecutor for UncertainWrite {
    fn supports(&self, name: &str) -> bool {
        name == "orders.submit"
    }

    fn execute(&self, _: &str, _: &Value) -> Result<Value, String> {
        self.calls.fetch_add(1, Ordering::AcqRel);
        if self.structured {
            return Ok(json!({"success":false, "message":"broker rejected submission"}));
        }
        Err("broker disconnected after submission".to_owned())
    }
}

#[test]
fn production_fail_closed_write_error_stays_unknown_and_replays_without_execution() {
    let calls = Arc::new(AtomicUsize::new(0));
    let mut fixture = ToolFixture::with_executor(
        calls.clone(),
        Arc::new(UncertainWrite {
            calls: calls.clone(),
            structured: false,
        }),
    );
    use_declared_write_policy(&mut fixture);
    let provider = Provider::new();
    let mut chat = fixture.seed_with_tool("run-uncertain-write", "once", "orders.submit");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    assert!(
        !fixture
            .runtime
            .tool_catalog
            .requires_idempotency_key("orders.submit")
    );
    assert!(!super::super::super::replay_safe_tool("orders.submit"));
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let completed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    assert_eq!(invocation.status, "UNKNOWN");
    assert_eq!(completed.status, "COMPLETED");
    assert_eq!(payload["degraded"], true);
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(payload["toolCalls"][0]["errorCode"], "SUBMISSION_UNKNOWN");
    let output: Value = serde_json::from_str(&invocation.output_json).unwrap();
    assert_eq!(output["success"], false);
    assert_eq!(output["error"]["code"], "SUBMISSION_UNKNOWN");
    assert_eq!(output["error"]["retryable"], false);
    let message = output["error"]["message"].as_str().unwrap();
    assert!(message.contains("uncertain write failure"), "{output}");
    assert!(
        message.contains("broker disconnected after submission"),
        "{output}"
    );
    assert_eq!(payload["toolCalls"][0]["output"], output);
    assert_eq!(calls.load(Ordering::Acquire), 1);
    let events = fixture.sessions.list_events(&chat.session_id).unwrap();
    fixture.restore_tool_checkpoint(&chat, &lease);
    let checkpoint = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&checkpoint.payload_json).unwrap();
    assert!(matches!(
        fixture.runtime.persist_tool_result(
            &chat,
            &payload["toolCalls"][0],
            "function-call-test",
            json!({"success":true}),
            "SUCCEEDED",
            None,
            lease.owner_id(),
            invocation.fencing_token,
            lease.token(),
        ),
        Err(super::super::super::AdkChatPortError::Conflict(_))
    ));
    assert_eq!(
        fixture.store.get_run(&chat.run_id).unwrap().unwrap(),
        checkpoint
    );
    let replay_provider = Provider::new();
    chat.request.endpoint = replay_provider.endpoint.parse().unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let replayed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&replayed.payload_json).unwrap();
    assert_eq!(replayed.status, "COMPLETED");
    assert_eq!(payload["toolResults"][0]["output"], output);
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(calls.load(Ordering::Acquire), 1);
    assert_eq!(
        fixture
            .store
            .get_tool_invocation(&chat.run_id, "function-call-test")
            .unwrap()
            .unwrap(),
        invocation
    );
    assert_eq!(
        fixture.sessions.list_events(&chat.session_id).unwrap(),
        events
    );
}

#[test]
fn production_fail_closed_structured_rejection_remains_a_completed_failed_call() {
    let calls = Arc::new(AtomicUsize::new(0));
    let mut fixture = ToolFixture::with_executor(
        calls.clone(),
        Arc::new(UncertainWrite {
            calls: calls.clone(),
            structured: true,
        }),
    );
    use_declared_write_policy(&mut fixture);
    let provider = Provider::new();
    let mut chat = fixture.seed_with_tool("run-rejected-write", "once", "orders.submit");
    chat.request.endpoint = provider.endpoint.parse().unwrap();
    let lease = RunLeaseGuard::acquire(fixture.store.clone(), &chat.run_id, "owner").unwrap();
    fixture
        .runtime
        .run_tool_loop(chat.clone(), Arc::new(AtomicBool::new(false)), &lease);
    let completed = fixture.store.get_run(&chat.run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    let invocation = fixture
        .store
        .get_tool_invocation(&chat.run_id, "function-call-test")
        .unwrap()
        .unwrap();
    assert_eq!(invocation.status, "COMPLETED");
    assert_eq!(completed.status, "COMPLETED");
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(
        payload["toolCalls"][0]["errorCode"],
        "TOOL_EXECUTION_FAILED"
    );
    let output: Value = serde_json::from_str(&invocation.output_json).unwrap();
    assert_eq!(output["success"], false);
    assert_eq!(output["error"]["message"], "broker rejected submission");
    assert_eq!(calls.load(Ordering::Acquire), 1);
}

fn use_declared_write_policy(fixture: &mut ToolFixture) {
    fixture.runtime.tool_catalog = Arc::new(
        crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(vec![
            json!({"id":"orders.submit", "permission":"write_internal",
                "idempotencyMode":"fail_closed", "allowedModes":["all"]}),
        ]),
    );
}

#[test]
fn production_catalog_invocation_policy_uses_descriptor_modes_and_read_permission_defaults() {
    let rows = [
        (" fail_closed ", "read_internal", true),
        (" REPLAY_SAFE ", "write_internal", false),
        ("keyed", "write_internal", false),
        ("", " Read_External ", false),
        ("unknown", "write_internal", true),
    ];
    for (mode, permission, expected) in rows {
        let catalog =
            crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(vec![
                json!({"id":"policy-tool", "permission":permission, "idempotencyMode":mode}),
            ]);
        assert_eq!(
            catalog.invocation_fails_closed("policy-tool"),
            Some(expected)
        );
        assert_eq!(catalog.invocation_fails_closed("missing"), None);
    }
}
