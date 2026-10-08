use super::*;

struct ExecutionFixture {
    _directory: tempfile::TempDir,
    _definitions: Arc<StrategyDefinitionStore>,
    store: StrategyRuntimeStore,
    execution: MockExecutionPort,
    provider: ActiveProviderState,
    binding: Value,
}

impl ExecutionFixture {
    fn new() -> Self {
        let directory = tempfile::tempdir().expect("tempdir");
        let path = directory.path().join("strategy.db");
        seed_strategy_test_db(&path);
        let definitions = Arc::new(
            StrategyDefinitionStore::open_existing(&path, STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE)
                .expect("definitions"),
        );
        let store = StrategyRuntimeStore::from_definition_store(&definitions);
        store
            .seed_instance("conditional", "RUNNING", "2026-08-30T00:00:00Z")
            .expect("instance");
        Self {
            _directory: directory,
            _definitions: definitions,
            store,
            execution: MockExecutionPort::default(),
            provider: ActiveProviderState::default(),
            binding: json!({"brokerId":"futu", "accountId":"12345", "tradingEnvironment":"SIMULATE"}),
        }
    }

    fn execute(&self, intents: &[PineOrderIntent], position: f64) -> Result<bool, String> {
        execute_strategy_intents(
            StrategyExecutionContext {
                execution: Some(&self.execution),
                execution_store: None,
                provider: &self.provider,
                store: &self.store,
                instance_id: "conditional",
                market: "US",
                symbol: "US.AAPL",
                binding: &self.binding,
                expected_risk_revision: None,
                fallback_price: Some(100.0),
                sellable_quantity: Some(position.abs()),
                current_position: Some(position),
                available_cash: Some(10000.0),
                virtual_account: None,
            },
            intents,
        )
    }

    fn one_payload(&self) -> Value {
        let writes = self.execution.mutations.lock().expect("writes");
        assert_eq!(writes.len(), 1);
        assert_eq!(writes[0].operation, ExecutionWriteOperation::OrderPlace);
        writes[0].payload.clone()
    }

    fn assert_no_writes(&self) {
        assert!(self.execution.mutations.lock().expect("writes").is_empty());
        assert!(
            self.store
                .list_audit_events("conditional")
                .expect("audit")
                .is_empty()
        );
        assert_eq!(
            self.store
                .get_instance("conditional")
                .expect("instance")
                .expect("row")
                .status,
            "RUNNING"
        );
    }
}

fn intent(
    kind: &str,
    direction: &str,
    quantity: Option<f64>,
    limit: Option<f64>,
    stop: Option<f64>,
) -> PineOrderIntent {
    PineOrderIntent {
        kind: kind.to_owned(),
        direction: direction.to_owned(),
        quantity: quantity.unwrap_or(0.0),
        has_quantity: quantity.is_some(),
        limit_price: limit.unwrap_or(0.0),
        has_limit_price: limit.is_some(),
        stop_price: stop.unwrap_or(0.0),
        has_stop_price: stop.is_some(),
        ..PineOrderIntent::default()
    }
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:60 TestCommandFromOrderIntentPreservesShortDirection
#[test]
fn pine_short_entry_dispatches_sell_without_mutating_the_source_direction() {
    let fixture = ExecutionFixture::new();
    let mut input = intent("entry", "short", Some(2.0), None, None);
    input.id = "short".to_owned();
    assert_eq!(fixture.execute(std::slice::from_ref(&input), 0.0), Ok(true));
    let payload = fixture.one_payload();
    assert_eq!(payload["side"], "SELL");
    assert_eq!(payload["orderType"], "MARKET");
    assert_eq!(payload["quantity"], 2.0);
    assert_eq!(input.direction, "short");
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:76 TestCommandFromOrderIntentMapsShortExitToBuy
#[test]
fn pine_short_stop_exit_dispatches_buy_with_the_original_stop_price() {
    let fixture = ExecutionFixture::new();
    let mut input = intent("exit", "short", None, None, Some(110.0));
    input.id = "short-stop".to_owned();
    input.from_entry = "short".to_owned();
    assert_eq!(
        fixture.execute(std::slice::from_ref(&input), -2.0),
        Ok(true)
    );
    let payload = fixture.one_payload();
    assert_eq!(payload["side"], "BUY");
    assert_eq!(payload["orderType"], "STOP");
    assert_eq!(payload["stopPrice"], 110.0);
    assert_eq!(payload["reduceOnly"], true);
    assert_eq!(input.direction, "short");
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:93 TestScopedExitQuantitySurvivesGoExecution
#[test]
fn pine_scoped_stop_exit_preserves_explicit_quantity_instead_of_closing_the_net_position() {
    let fixture = ExecutionFixture::new();
    let mut input = intent("exit", "long", Some(1.5), None, Some(95.0));
    input.id = "close-a-half".to_owned();
    input.from_entry = "A".to_owned();
    assert_eq!(fixture.execute(&[input], 7.0), Ok(true));
    let payload = fixture.one_payload();
    assert_eq!(payload["side"], "SELL");
    assert_eq!(payload["quantity"], 1.5);
    assert_eq!(payload["orderType"], "STOP");
    assert_eq!(payload["stopPrice"], 95.0);
    assert_eq!(payload["reduceOnly"], true);
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:130 TestCommandFromOrderIntentMapsConditionalOrderTypes
#[test]
fn pine_conditional_intents_dispatch_stop_and_stop_limit_without_changing_prices() {
    for (input, expected_type) in [
        (intent("entry", "long", None, None, Some(101.0)), "STOP"),
        (intent("exit", "long", None, None, Some(95.0)), "STOP"),
        (
            intent("entry", "long", None, Some(102.0), Some(101.0)),
            "STOP_LIMIT",
        ),
        (
            intent("order", "short", None, Some(98.0), Some(99.0)),
            "STOP_LIMIT",
        ),
    ] {
        let fixture = ExecutionFixture::new();
        assert_eq!(fixture.execute(std::slice::from_ref(&input), 7.0), Ok(true));
        let payload = fixture.one_payload();
        assert_eq!(payload["orderType"], expected_type, "{input:?}");
        assert_eq!(
            payload.get("price").and_then(Value::as_f64).unwrap_or(0.0),
            input.limit_price
        );
        assert_eq!(
            payload
                .get("stopPrice")
                .and_then(Value::as_f64)
                .unwrap_or(0.0),
            input.stop_price
        );
    }
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:267 TestCommandFromOrderIntentRejectsUnsupportedIntent
#[test]
fn pine_invalid_intents_reject_the_entire_batch_before_any_execution_write() {
    let mut reduce_entry = intent("entry", "long", Some(1.0), None, None);
    reduce_entry.reduce_only = true;
    let mut child_entry = reduce_entry.clone();
    child_entry.reduce_only = false;
    child_entry.parent_id = "parent".to_owned();
    let mut unscoped_oco = intent("exit", "long", Some(1.0), None, None);
    unscoped_oco.oco_group_id = "oco".to_owned();
    let mut grouped_cancel = intent("cancel", "", None, None, None);
    grouped_cancel.id = "grouped-cancel".to_owned();
    grouped_cancel.atomic_group_id = "atomic".to_owned();
    let invalid = [
        (intent("bracket", "", None, None, None), "unsupported"),
        (intent("entry", "", None, None, None), "requires long/short"),
        (
            intent("entry", "long", None, Some(f64::INFINITY), None),
            "positive and finite",
        ),
        (
            intent("entry", "long", None, None, Some(f64::NAN)),
            "positive and finite",
        ),
        (
            intent("entry", "long", None, Some(0.0), None),
            "positive and finite",
        ),
        (
            intent("entry", "long", None, None, Some(-1.0)),
            "positive and finite",
        ),
        (reduce_entry, "protective-order metadata"),
        (child_entry, "protective-order metadata"),
        (unscoped_oco, "atomic group"),
        (grouped_cancel, "placement relationships"),
        (
            intent("close", "long", Some(1.0), Some(101.0), Some(99.0)),
            "cannot combine",
        ),
        (intent("unsupported", "", None, None, None), "unsupported"),
    ];
    let mut violations = Vec::new();
    for (input, message) in invalid {
        for prepend_valid in [false, true] {
            let fixture = ExecutionFixture::new();
            let mut batch = Vec::new();
            if prepend_valid {
                batch.push(intent("entry", "long", Some(1.0), None, None));
            }
            batch.push(input.clone());
            let result = fixture.execute(&batch, 7.0);
            if !matches!(&result, Err(error) if error.contains(message)) {
                violations.push(format!(
                    "{input:?}, valid prefix={prepend_valid}: {result:?}"
                ));
            }
            let writes = fixture.execution.mutations.lock().expect("writes").len();
            let audit = fixture
                .store
                .list_audit_events("conditional")
                .expect("audit");
            if writes != 0 || !audit.is_empty() {
                violations.push(format!(
                    "{input:?}, valid prefix={prepend_valid}: {writes} writes, {} audit events",
                    audit.len()
                ));
            }
            assert_eq!(
                fixture
                    .store
                    .get_instance("conditional")
                    .expect("instance")
                    .expect("row")
                    .status,
                "RUNNING"
            );
        }
    }
    assert!(violations.is_empty(), "{}", violations.join("\n"));
}

// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:180 TestCommandFromOrderIntentRejectsUnsupportedExitBracket
// Parity: go:452dea11:internal/strategy/pine_live_command_test.go:194 TestCommandsFromOrderIntentsExpandsAtomicOCOExit
#[test]
fn pine_exit_brackets_cannot_be_downgraded_to_unprotected_limit_orders() {
    for grouped in [false, true] {
        let fixture = ExecutionFixture::new();
        let mut bracket = intent(
            "exit",
            "long",
            Some(1.0),
            Some(110.0),
            Some(if grouped { 95.0 } else { 90.0 }),
        );
        bracket.id = "protect".to_owned();
        bracket.from_entry = "long".to_owned();
        if grouped {
            bracket.parent_id = "long".to_owned();
            bracket.atomic_group_id = "bracket-1".to_owned();
            bracket.oco_group_id = "protect-oco".to_owned();
            bracket.reduce_only = true;
        }
        let error = fixture
            .execute(&[bracket], 7.0)
            .expect_err("reject unsupported bracket");
        assert!(error.contains("OCO bracket"), "{error}");
        fixture.assert_no_writes();
    }
}

#[test]
fn pine_same_bar_protective_group_rejects_before_the_parent_entry_is_submitted() {
    let fixture = ExecutionFixture::new();
    let mut parent = intent("entry", "long", Some(1.0), None, None);
    parent.id = "long".to_owned();
    parent.atomic_group_id = "bracket-1".to_owned();
    let mut protection = intent("exit", "long", Some(1.0), Some(110.0), Some(95.0));
    protection.id = "protect".to_owned();
    protection.from_entry = "long".to_owned();
    protection.parent_id = "long".to_owned();
    protection.atomic_group_id = "bracket-1".to_owned();
    protection.oco_group_id = "protect-oco".to_owned();
    protection.reduce_only = true;
    let error = fixture
        .execute(&[parent, protection], 0.0)
        .expect_err("unsupported atomic capability");
    assert!(
        error.contains("unsupported parent or atomic group"),
        "{error}"
    );
    fixture.assert_no_writes();
}

#[test]
fn pine_real_entry_still_requires_explicit_quantity_after_conditional_validation() {
    let mut fixture = ExecutionFixture::new();
    fixture.binding["tradingEnvironment"] = json!("REAL");
    let input = intent("entry", "long", None, Some(102.0), Some(101.0));
    let error = fixture
        .execute(&[input], 0.0)
        .expect_err("missing real quantity");
    assert!(error.contains("positive finite quantity"), "{error}");
    fixture.assert_no_writes();
}
