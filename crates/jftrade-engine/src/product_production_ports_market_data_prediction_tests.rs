use super::*;
use serde_json::json;

fn port(
    provider: Option<MarketDataProvider>,
    opend_ready: bool,
) -> ProductionMarketDataPredictionPort {
    let state = Arc::new(ActiveProviderState::new(provider));
    state.set_readiness(false, opend_ready, false);
    ProductionMarketDataPredictionPort {
        active_provider_state: state,
        trade_runtime: None,
    }
}

#[test]
fn prediction_read_query_accepts_go_route_defaults() {
    validate_prediction_read_request(
        "/api/v1/market-data/prediction/events",
        "brokerId=futu&accountId=acct-1&tradingEnvironment=SIMULATE&category=politics&pageSize=100&refresh=true",
    ).expect("valid prediction query");
}

#[test]
fn prediction_read_query_rejects_invalid_schema_before_provider_check() {
    let invalid = [
        (
            "/api/v1/market-data/prediction/events",
            "pageSize=0",
            "pageSize must be between 1 and 300",
        ),
        (
            "/api/v1/market-data/prediction/events",
            "refresh=maybe",
            "refresh must be true or false",
        ),
        (
            "/api/v1/market-data/prediction/events",
            "category=%FF",
            "invalid prediction query encoding",
        ),
        (
            "/api/v1/market-data/prediction/contracts/US.EC-42/snapshot",
            "operation=events",
            "operation must be snapshot",
        ),
    ];
    for (path, query, message) in invalid {
        assert!(matches!(
            validate_prediction_read_request(path, query),
            Err(MarketDataPredictionReadSnapshotError::Invalid(actual)) if actual == message
        ));
    }
}

#[test]
fn prediction_read_query_rejects_invalid_path_segments() {
    for path in [
        "/api/v1/market-data/prediction/contracts//snapshot",
        "/api/v1/market-data/prediction/contracts/US.EC%2F42/snapshot",
        "/api/v1/market-data/prediction/events/EVENT%2042/contracts",
    ] {
        assert!(matches!(
            validate_prediction_read_request(path, ""),
            Err(MarketDataPredictionReadSnapshotError::Invalid(_))
        ));
    }
}

#[test]
fn prediction_read_port_fails_closed_for_missing_or_unready_provider() {
    assert!(matches!(
        port(None, false).read("/api/v1/market-data/prediction/categories", ""),
        Err(MarketDataPredictionReadSnapshotError::Unavailable(message))
            if message == "prediction market-data provider is not configured"
    ));
    assert!(matches!(
        port(Some(MarketDataProvider::Futu), false)
            .read("/api/v1/market-data/prediction/categories", ""),
        Err(MarketDataPredictionReadSnapshotError::Unavailable(message))
            if message == "Futu prediction market-data provider is not ready"
    ));
}

/// A `TradeReadPort` double that returns a fixed account list so prediction
/// eligibility can be asserted without an OpenD session. Only `read_accounts`
/// is meaningful; every other read is unavailable because the eligibility owner
/// never calls it.
#[derive(Debug)]
struct EligibilityAccounts {
    accounts: Vec<jftrade_integration_futu::TradeAccountSnapshot>,
    error: Option<String>,
}

impl jftrade_integration_futu::TradeReadPort for EligibilityAccounts {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeAccountSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        match &self.error {
            Some(message) => Err(jftrade_integration_futu::TradeSessionError::Unsupported(
                message.clone(),
            )),
            None => Ok(self.accounts.clone()),
        }
    }

    fn read_funds(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<
        jftrade_integration_futu::TradeFundsSnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_cash_flows(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeCashFlowSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_order_fees(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<String>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderFeeSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_margin_ratios(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Vec<jftrade_integration_futu::TradeSecurity>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeMarginRatioSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_max_trade_quantity(
        &self,
        _: jftrade_integration_futu::TradeMaxTradeQuantityRequest,
    ) -> Result<
        jftrade_integration_futu::TradeMaxTradeQuantitySnapshot,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    #[allow(clippy::too_many_arguments)]
    fn read_positions(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradePositionSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_orders(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeOrderSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }

    fn read_fills(
        &self,
        _: jftrade_integration_futu::TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<
        Vec<jftrade_integration_futu::TradeFillSnapshot>,
        jftrade_integration_futu::TradeSessionError,
    > {
        Err(unused())
    }
}

fn unused() -> jftrade_integration_futu::TradeSessionError {
    jftrade_integration_futu::TradeSessionError::Unsupported("unused".to_owned())
}

fn account(
    id: u64,
    firm: Option<i32>,
    authorities: Vec<i32>,
) -> jftrade_integration_futu::TradeAccountSnapshot {
    jftrade_integration_futu::TradeAccountSnapshot {
        trd_env: 1,
        acc_id: id,
        trd_market_auth_list: authorities,
        acc_type: None,
        card_num: None,
        security_firm: firm,
        sim_acc_type: None,
        uni_card_num: None,
        acc_status: None,
        acc_role: None,
        jp_acc_type: Vec::new(),
        competition_acc_name: None,
    }
}

fn runtime_with_accounts(
    accounts: Vec<jftrade_integration_futu::TradeAccountSnapshot>,
) -> Arc<SharedTradeReadRuntime> {
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(Arc::new(EligibilityAccounts {
            accounts,
            error: None,
        })),
        Some(true),
    );
    runtime
}

/// Parity: go:452dea11:internal/productfeatures/service_test.go:12
/// TestPredictionEligibilityRejectsFutuSecuritiesAndAcceptsFutuInc
///
/// Go resolves the prediction security firm from the discovered accounts: an HK
/// authority or a non-`FUTUINC` firm is rejected, and only a `FUTUINC` account
/// with US authority (or no authority list) returns `"FUTUINC"`.
#[test]
// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:14 TestProductFeatureServiceRemainingRoutingAndDegradationBranches
fn prediction_eligibility_rejects_futu_securities_and_accepts_futu_inc() {
    let hk = runtime_with_accounts(vec![account(1, Some(1), vec![1])]);
    let error = prediction_account_eligibility(&hk, "accountId=1")
        .expect_err("FUTUSECURITIES/HK authority must be ineligible");
    assert!(
        error.contains("no eligible Moomoo US (FUTUINC) account"),
        "error = {error}"
    );

    let us = runtime_with_accounts(vec![account(2, Some(2), vec![11])]);
    assert_eq!(
        prediction_account_eligibility(&us, "accountId=2").expect("FUTUINC/US is eligible"),
        "FUTUINC"
    );

    // No authority list at all is still eligible: Go only filters when the list is
    // non-empty, so an account that never reported authorities is trusted.
    let no_authorities = runtime_with_accounts(vec![account(3, Some(2), Vec::new())]);
    assert_eq!(
        prediction_account_eligibility(&no_authorities, "").expect("no authority list is eligible"),
        "FUTUINC"
    );
}

/// Parity: go:452dea11:internal/productfeatures/service_test.go:443
/// TestProductFeatureDirectAdapterCacheAndEligibilityBranches (eligibility cases)
///
/// Go's account cases: a discovery failure, a nil security firm, and a `FUTUINC`
/// account whose authorities exclude US are all ineligible.
#[test]
fn prediction_eligibility_rejects_discovery_failure_nil_firm_and_wrong_authority() {
    let failing = Arc::new(SharedTradeReadRuntime::default());
    failing.set(
        Some(Arc::new(EligibilityAccounts {
            accounts: Vec::new(),
            error: Some("accounts unavailable".to_owned()),
        })),
        Some(true),
    );
    let error =
        prediction_account_eligibility(&failing, "").expect_err("discovery failure is ineligible");
    assert!(
        error.contains("account eligibility could not be verified"),
        "error = {error}"
    );

    let missing_client = Arc::new(SharedTradeReadRuntime::default());
    let error = prediction_account_eligibility(&missing_client, "")
        .expect_err("missing trade client is ineligible");
    assert!(
        error.contains("account eligibility could not be verified"),
        "error = {error}"
    );

    let nil_firm = runtime_with_accounts(vec![account(1, None, vec![11])]);
    assert!(prediction_account_eligibility(&nil_firm, "accountId=1").is_err());

    let wrong_authority = runtime_with_accounts(vec![account(1, Some(2), vec![1])]);
    assert!(prediction_account_eligibility(&wrong_authority, "accountId=1").is_err());

    // A requested id that matches no account is ineligible even when another
    // eligible account exists, and the filter stays optional.
    let mismatched = runtime_with_accounts(vec![account(9, Some(2), vec![11])]);
    assert!(prediction_account_eligibility(&mismatched, "accountId=1").is_err());
    assert_eq!(
        prediction_account_eligibility(&mismatched, "").expect("unfiltered eligibility"),
        "FUTUINC"
    );
}

/// The read route must fail closed with the Go wire error before any reader call
/// when the account is ineligible.
#[test]
fn prediction_read_returns_403_for_an_ineligible_account_before_the_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, false);
    let runtime = runtime_with_accounts(vec![account(1, Some(1), vec![1])]);
    let port = ProductionMarketDataPredictionPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read(
            "/api/v1/market-data/prediction/categories",
            "brokerId=futu&accountId=1&market=US",
        )
        .expect_err("ineligible account must fail closed");
    match error {
        MarketDataPredictionReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            assert_eq!(status, 403);
            assert_eq!(code, PREDICTION_INELIGIBLE_CODE);
            assert!(message.starts_with(PREDICTION_INELIGIBLE_MESSAGE), "message = {message}");
            assert!(retry_after_seconds.is_none());
        }
        other => panic!("expected the 403 ineligible failure, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:346
/// TestQueryUsesFreshPredictionPushBeforePolling
///
/// Go resolves the prediction capability, then asks `predictionPushResult`
/// before it touches the broker: a fresh push sample answers the read and the
/// adapter records zero query calls. The Rust equivalent is the prediction read
/// port consulting the runtime push cache before `prediction_reader_available`
/// and `prediction_read`.
#[test]
fn prediction_read_prefers_a_fresh_push_sample_over_the_reader() {
    /// Counts every reader call so the push-served read can prove it never
    /// polled OpenD.
    #[derive(Debug, Default)]
    struct CountingPredictionReader {
        calls: std::sync::atomic::AtomicUsize,
    }

    impl jftrade_integration_futu::PredictionMarketReadPort for CountingPredictionReader {
        fn read(
            &self,
            _path: &str,
            _query: &str,
        ) -> Result<Value, jftrade_integration_futu::PredictionMarketReadError> {
            self.calls
                .fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            Ok(json!({"entries": [{"price": 9.99}]}))
        }
    }

    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(EligibilityAccounts { accounts: vec![account(1, Some(2), vec![11])], error: None })), Some(true));
    let reader = Arc::new(CountingPredictionReader::default());
    runtime.set_prediction_adapters(
        Some(reader.clone()
            as Arc<dyn jftrade_integration_futu::PredictionMarketReadPort>),
        None,
        None,
    );
    runtime.prediction_push_cache.store(
        "futu",
        "US.EC-42",
        "ORDER_BOOK",
        "7",
        "2026-07-18T12:00:00Z",
        vec![json!({"price": 0.63})],
    );
    let port = ProductionMarketDataPredictionPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };

    let value = port
        .read(
            "/api/v1/market-data/prediction/contracts/EC-42/order-book",
            "brokerId=futu&accountId=1",
        )
        .expect("a fresh push must serve the prediction read");
    assert_eq!(value["entries"][0]["price"], 0.63);
    assert_eq!(value["metadata"]["source"], "push");
    assert_eq!(value["metadata"]["dataType"], "ORDER_BOOK");
    assert_eq!(value["metadata"]["sequence"], "7");
    assert_eq!(
        reader.calls.load(std::sync::atomic::Ordering::SeqCst),
        0,
        "a push-served read must not poll the prediction reader"
    );

    // The KLINE/TICKER operations map onto the same cache keyed by route.
    for (path, data_type) in [
        (
            "/api/v1/market-data/prediction/contracts/EC-42/candles",
            "KLINE",
        ),
        ("/api/v1/market-data/prediction/contracts/EC-42/ticks", "TICKER"),
    ] {
        let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        state.set_readiness(false, true, false);
        let runtime = Arc::new(SharedTradeReadRuntime::default());
        runtime.set(
            Some(Arc::new(EligibilityAccounts {
                accounts: vec![account(1, Some(2), vec![11])],
                error: None,
            })),
            Some(true),
        );
        let reader = Arc::new(CountingPredictionReader::default());
        runtime.set_prediction_adapters(
            Some(reader.clone()
                as Arc<dyn jftrade_integration_futu::PredictionMarketReadPort>),
            None,
            None,
        );
        runtime.prediction_push_cache.store(
            "futu",
            "US.EC-42",
            data_type,
            "9",
            "2026-07-18T12:00:00Z",
            vec![json!({"price": 0.77})],
        );
        let port = ProductionMarketDataPredictionPort {
            active_provider_state: state,
            trade_runtime: Some(runtime),
        };
        let value = port
            .read(path, "brokerId=futu&accountId=1")
            .unwrap_or_else(|error| panic!("{path} push read: {error:?}"));
        assert_eq!(value["metadata"]["dataType"], data_type);
        assert_eq!(
            reader.calls.load(std::sync::atomic::Ordering::SeqCst),
            0,
            "{path} must be served from the push cache"
        );
    }

    // Without a push sample the read falls through to the reader exactly once.
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(Arc::new(EligibilityAccounts {
            accounts: vec![account(1, Some(2), vec![11])],
            error: None,
        })),
        Some(true),
    );
    let reader = Arc::new(CountingPredictionReader::default());
    runtime.set_prediction_adapters(
        Some(reader.clone()
            as Arc<dyn jftrade_integration_futu::PredictionMarketReadPort>),
        None,
        None,
    );
    let port = ProductionMarketDataPredictionPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read(
            "/api/v1/market-data/prediction/contracts/EC-42/order-book",
            "brokerId=futu&accountId=1",
        )
        .expect("cache miss falls through to the reader");
    assert_eq!(value["entries"][0]["price"], 9.99);
    assert_eq!(reader.calls.load(std::sync::atomic::Ordering::SeqCst), 1);
}
