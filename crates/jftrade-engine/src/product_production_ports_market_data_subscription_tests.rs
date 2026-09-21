use super::*;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use jftrade_integration_futu::{
    PredictionMarketReadError, PredictionMarketSubscriptionPort, TradeAccountSnapshot,
    TradeCashFlowSnapshot, TradeFillSnapshot, TradeFundsSnapshot, TradeHeader,
    TradeMarginRatioSnapshot, TradeMaxTradeQuantityRequest, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeSnapshot, TradeOrderSnapshot, TradePositionSnapshot, TradeSecurity,
    TradeSessionError,
};
use serde_json::json;
use std::sync::atomic::{AtomicUsize, Ordering};

/// Account discovery double for the prediction eligibility gate.  The gate is
/// owned by the shared prediction eligibility helper, so the subscription
/// fixtures must expose the same `FUTUINC`/US account that Go's
/// `featureBroker` discovery returns.
#[derive(Debug, Default)]
struct EligibleAccounts {
    accounts: Vec<TradeAccountSnapshot>,
}

impl EligibleAccounts {
    fn futu_inc(acc_id: u64) -> Self {
        Self {
            accounts: vec![TradeAccountSnapshot {
                trd_env: 1,
                acc_id,
                trd_market_auth_list: vec![11],
                acc_type: None,
                card_num: None,
                security_firm: Some(2),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: None,
                acc_role: None,
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            }],
        }
    }
}

fn unused_session() -> TradeSessionError {
    TradeSessionError::Unsupported("unused".to_owned())
}

impl jftrade_integration_futu::TradeReadPort for EligibleAccounts {
    fn read_accounts(
        &self,
        _: u64,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeAccountSnapshot>, TradeSessionError> {
        Ok(self.accounts.clone())
    }

    fn read_funds(
        &self,
        _: TradeHeader,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
    ) -> Result<TradeFundsSnapshot, TradeSessionError> {
        Err(unused_session())
    }

    fn read_cash_flows(
        &self,
        _: TradeHeader,
        _: String,
        _: Option<i32>,
    ) -> Result<Vec<TradeCashFlowSnapshot>, TradeSessionError> {
        Err(unused_session())
    }

    fn read_order_fees(
        &self,
        _: TradeHeader,
        _: Vec<String>,
    ) -> Result<Vec<TradeOrderFeeSnapshot>, TradeSessionError> {
        Err(unused_session())
    }

    fn read_margin_ratios(
        &self,
        _: TradeHeader,
        _: Vec<TradeSecurity>,
    ) -> Result<Vec<TradeMarginRatioSnapshot>, TradeSessionError> {
        Err(unused_session())
    }

    fn read_max_trade_quantity(
        &self,
        _: TradeMaxTradeQuantityRequest,
    ) -> Result<TradeMaxTradeQuantitySnapshot, TradeSessionError> {
        Err(unused_session())
    }

    #[allow(clippy::too_many_arguments)]
    fn read_positions(
        &self,
        _: TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<f64>,
        _: Option<f64>,
        _: Option<bool>,
        _: Option<i32>,
        _: Option<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradePositionSnapshot>, TradeSessionError> {
        Err(unused_session())
    }

    fn read_orders(
        &self,
        _: TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Vec<i32>,
        _: Option<bool>,
    ) -> Result<Vec<TradeOrderSnapshot>, TradeSessionError> {
        Err(unused_session())
    }

    fn read_fills(
        &self,
        _: TradeHeader,
        _: Option<jftrade_integration_futu::TradeFilter>,
        _: Option<bool>,
    ) -> Result<Vec<TradeFillSnapshot>, TradeSessionError> {
        Err(unused_session())
    }
}

#[derive(Debug, Default)]
struct PredictionFixture {
    subscribes: AtomicUsize,
    unsubscribes: AtomicUsize,
}

impl PredictionMarketSubscriptionPort for PredictionFixture {
    fn subscribe(
        &self,
        code: &str,
        data_types: &[String],
    ) -> Result<Value, PredictionMarketReadError> {
        self.subscribes.fetch_add(1, Ordering::SeqCst);
        Ok(json!({"instrumentId": format!("US.{code}"), "dataTypes": data_types}))
    }

    fn unsubscribe(&self, _code: &str) -> Result<Value, PredictionMarketReadError> {
        self.unsubscribes.fetch_add(1, Ordering::SeqCst);
        Ok(json!({"subscribed": false}))
    }
}

fn prediction_request(
    method: &str,
    path: &str,
    body: &[u8],
) -> MarketDataSubscriptionMutationRequest {
    MarketDataSubscriptionMutationRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: String::new(),
        body: body.to_vec(),
    }
}

#[test]
fn prediction_subscription_uses_reference_counted_leases() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    active.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(EligibleAccounts::futu_inc(7))), Some(true));
    let fixture = Arc::new(PredictionFixture::default());
    runtime.set_prediction_adapters(None, Some(fixture.clone()), None);
    let port = ProductionMarketDataSubscriptionMutationPort::new(active, None, None)
        .with_trade_runtime(Some(runtime));
    let path = "/api/v1/market-data/prediction/contracts/EC-42/subscriptions";
    let body = br#"{"dataTypes":["ticker","ORDER_BOOK","ticker"]}"#;
    let first = port
        .dispatch(&prediction_request("POST", path, body))
        .expect("first prediction lease");
    let second = port
        .dispatch(&prediction_request("POST", path, body))
        .expect("second prediction lease");
    assert_ne!(first["leaseId"], second["leaseId"]);
    assert_eq!(first["dataTypes"], json!(["ORDER_BOOK", "TICKER"]));
    assert_eq!(fixture.subscribes.load(Ordering::SeqCst), 1);

    let release_path = format!("{path}/{}", first["leaseId"].as_str().unwrap());
    port.dispatch(&prediction_request("DELETE", &release_path, b""))
        .expect("first release");
    assert_eq!(fixture.unsubscribes.load(Ordering::SeqCst), 0);
    let release_path = format!("{path}/{}", second["leaseId"].as_str().unwrap());
    port.dispatch(&prediction_request("DELETE", &release_path, b""))
        .expect("last release");
    assert_eq!(fixture.unsubscribes.load(Ordering::SeqCst), 1);
    port.dispatch(&prediction_request("DELETE", &release_path, b""))
        .expect("idempotent release");
    assert_eq!(fixture.unsubscribes.load(Ordering::SeqCst), 1);
}

/// Parity: go:452dea11:internal/productfeatures/service_test.go:12
/// TestPredictionEligibilityRejectsFutuSecuritiesAndAcceptsFutuInc (subscription path)
///
/// Go's `AcquirePredictionSubscription` resolves the broker capability and then
/// runs `predictionEligibility`, so a FUTUSECURITIES/HK account never reaches
/// `SubscribePredictionMarket`.  The read route and this mutation route share
/// one eligibility owner, so the account gate must answer 403 with the
/// `PREDICTION_MARKET_INELIGIBLE` wire code before OpenD is touched.
#[test]
fn prediction_subscription_rejects_an_ineligible_account_before_subscribing() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    active.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(
        Some(Arc::new(EligibleAccounts {
            accounts: vec![TradeAccountSnapshot {
                trd_env: 1,
                acc_id: 9,
                trd_market_auth_list: vec![1],
                acc_type: None,
                card_num: None,
                security_firm: Some(1),
                sim_acc_type: None,
                uni_card_num: None,
                acc_status: None,
                acc_role: None,
                jp_acc_type: Vec::new(),
                competition_acc_name: None,
            }],
        })),
        Some(true),
    );
    let fixture = Arc::new(PredictionFixture::default());
    runtime.set_prediction_adapters(None, Some(fixture.clone()), None);
    let port = ProductionMarketDataSubscriptionMutationPort::new(active, None, None)
        .with_trade_runtime(Some(runtime));
    let mut request = prediction_request(
        "POST",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
        br#"{"dataTypes":["ORDER_BOOK"]}"#,
    );
    request.query = "accountId=9".to_owned();
    match port.dispatch(&request) {
        Err(MarketDataSubscriptionMutationPortError::Failed {
            status,
            code,
            message,
            ..
        }) => {
            assert_eq!(status, 403);
            assert_eq!(code, "PREDICTION_MARKET_INELIGIBLE");
            assert!(
                message.starts_with("prediction market requires an eligible Moomoo US account"),
                "message = {message}"
            );
        }
        other => panic!("expected the 403 ineligible failure, got {other:?}"),
    }
    assert_eq!(
        fixture.subscribes.load(Ordering::SeqCst),
        0,
        "an ineligible account must not reach the OpenD subscription"
    );
}

    // Parity: go:452dea11:internal/app/apiserver/strategyapp/runtime_ports_test.go:137 TestMarketDataHealthReturnsActiveProviderHealth
#[test]
fn prediction_subscription_rejects_invalid_types_and_unready_provider() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    active.set_readiness(false, false, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_prediction_adapters(None, Some(Arc::new(PredictionFixture::default())), None);
    let port = ProductionMarketDataSubscriptionMutationPort::new(active, None, None)
        .with_trade_runtime(Some(runtime));
    let request = prediction_request(
        "POST",
        "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
        br#"{"dataTypes":["UNKNOWN"]}"#,
    );
    assert!(matches!(
        port.dispatch(&request),
        Err(MarketDataSubscriptionMutationPortError::Unavailable(_))
    ));
}

/// Parity: go:452dea11:internal/productfeatures/service_test.go:243
/// TestProductFeatureServiceExhaustiveFailureAndNormalizationBranches (prediction part)
///
/// Go rejects an empty/`.`-only prediction instrument and a blank lease id with
/// `ErrInvalidQuery`, and an empty `dataTypes` list can never produce a lease.
/// The Rust subscription mutation port maps those owner errors onto 400
/// `BAD_REQUEST` before any OpenD call.
#[test]
fn prediction_subscription_rejects_invalid_instrument_types_and_lease_ids() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    active.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(EligibleAccounts::futu_inc(7))), Some(true));
    let fixture = Arc::new(PredictionFixture::default());
    runtime.set_prediction_adapters(None, Some(fixture.clone()), None);
    let port = ProductionMarketDataSubscriptionMutationPort::new(active, None, None)
        .with_trade_runtime(Some(runtime));

    // Go `normalizePredictionInstrumentID` trims/case-folds and strips a `US.`
    // prefix; only an empty result is invalid. `""`, `" "` and `"US."` all
    // normalize to empty, so each must be rejected with `ErrInvalidQuery`.
    for code in ["", "%20", "US."] {
        let path = format!("/api/v1/market-data/prediction/contracts/{code}/subscriptions");
        let error = port
            .dispatch(&prediction_request(
                "POST",
                &path,
                br#"{"dataTypes":["ORDER_BOOK"]}"#,
            ))
            .expect_err("an empty prediction instrument must be rejected");
        assert!(
            matches!(
                error,
                MarketDataSubscriptionMutationPortError::Failed { status: 400, .. }
            ),
            "code {code:?} produced {error:?}"
        );
    }
    // `"."` is *not* empty in Go: it is accepted and becomes `US..`. The Rust
    // owner keeps that exact boundary so the wire string stays byte-identical.
    let dot = port
        .dispatch(&prediction_request(
            "POST",
            "/api/v1/market-data/prediction/contracts/./subscriptions",
            br#"{"dataTypes":["ORDER_BOOK"]}"#,
        ))
        .expect("Go accepts a non-empty dot code");
    assert_eq!(dot["instrumentId"], "US..");

    // Empty and unsupported dataTypes are rejected by the same validation owner.
    for body in [
        br#"{"dataTypes":[]}"#.as_slice(),
        br#"{"dataTypes":["UNKNOWN"]}"#.as_slice(),
        br#"{}"#.as_slice(),
    ] {
        let error = port
            .dispatch(&prediction_request(
                "POST",
                "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
                body,
            ))
            .expect_err("an invalid dataTypes body must be rejected");
        assert!(
            matches!(
                error,
                MarketDataSubscriptionMutationPortError::Failed { status: 400, .. }
            ),
            "body produced {error:?}"
        );
    }

    // A blank lease id is invalid, while an unknown-but-non-blank id is
    // intentionally idempotent.
    let blank = port
        .dispatch(&prediction_request(
            "DELETE",
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/%20",
            b"",
        ))
        .expect_err("a blank lease id must be rejected");
    assert!(matches!(
        blank,
        MarketDataSubscriptionMutationPortError::Failed { status: 400, .. }
    ));
    let unknown = port
        .dispatch(&prediction_request(
            "DELETE",
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions/missing-lease",
            b"",
        ))
        .expect("an unknown lease id is idempotent");
    assert_eq!(unknown["released"], true);

    // Exactly one subscribe reached OpenD: the accepted `"."` boundary above.
    // Every invalid request was rejected by validation before the provider.
    assert_eq!(
        fixture.subscribes.load(Ordering::SeqCst),
        1,
        "only the accepted dot boundary may reach OpenD"
    );
}

/// Parity: go:452dea11:internal/productfeatures/service_test.go:321
/// TestProductFeaturePredictionAndCustomizationFailureBranches
///
/// Go hides no upstream failure: a failing `SubscribePredictionMarket` and a
/// failing customization write both surface to the caller. The Rust
/// subscription owner reports a provider failure as 502
/// `BROKER_FEATURE_FAILED` while keeping the lease table untouched, which is
/// the equivalent "no partial state on failure" contract.
#[test]
fn prediction_subscription_surfaces_a_provider_failure_without_retaining_a_lease() {
    #[derive(Debug)]
    struct FailingSubscription;

    impl PredictionMarketSubscriptionPort for FailingSubscription {
        fn subscribe(
            &self,
            _code: &str,
            _data_types: &[String],
        ) -> Result<Value, PredictionMarketReadError> {
            Err(PredictionMarketReadError::Transport(
                "subscription failed".to_owned(),
            ))
        }

        fn unsubscribe(&self, _code: &str) -> Result<Value, PredictionMarketReadError> {
            Ok(json!({"subscribed": false}))
        }
    }

    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    active.set_readiness(false, true, false);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set(Some(Arc::new(EligibleAccounts::futu_inc(7))), Some(true));
    runtime.set_prediction_adapters(None, Some(Arc::new(FailingSubscription)), None);
    let port = ProductionMarketDataSubscriptionMutationPort::new(active, None, None)
        .with_trade_runtime(Some(runtime));
    let body = br#"{"dataTypes":["ticker","kline","ticker"]}"#;
    let error = port
        .dispatch(&prediction_request(
            "POST",
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
            body,
        ))
        .expect_err("the upstream subscription failure must surface");
    assert!(
        matches!(
            error,
            MarketDataSubscriptionMutationPortError::Failed { status: 502, .. }
        ),
        "upstream subscription failure = {error:?}"
    );

    // A retry after the failure must still issue exactly one subscribe attempt,
    // proving no half-written lease was retained by the failed call.
    let retry = port
        .dispatch(&prediction_request(
            "POST",
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
            body,
        ))
        .expect_err("the provider is still failing");
    assert!(matches!(
        retry,
        MarketDataSubscriptionMutationPortError::Failed { status: 502, .. }
    ));
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:22 TestSubscriptionRoutesUseInstrumentRequestContract
///
/// Go acquires `KLINE:HK:00700:1m` through the instrument request contract and
/// then proves single-target release, consumer-scoped clear and clear-all keep
/// or drop exactly the remaining entries.  Rust keeps demand ownership in the
/// subscription mutation port + `DemandBook`, so the equivalent regression
/// drives acquire/release/clear through that owner and asserts the rendered
/// entry keys, consumers and `totalActiveSubscriptions` after every step.
#[test]
fn subscription_routes_preserve_instrument_request_contract_through_acquire_release_and_clear() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(std::sync::Mutex::new(
        jftrade_marketdata::ProviderRouter::new(32),
    ));
    let port =
        ProductionMarketDataSubscriptionMutationPort::new(active, Some(router), None);

    let acquire = |consumer: &str, instruments: serde_json::Value| {
        port.dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": consumer,
                "instruments": instruments,
            }))
            .expect("acquire body"),
        })
        .expect("acquire subscription")
    };

    // Acquire reuses the caller's channel/interval in the logical key.
    let first = acquire(
        "chart-main",
        json!([{"channel": "KLINE", "market": "hk", "symbol": "00700", "interval": "1m"}]),
    );
    assert_eq!(first["entries"][0]["key"], "KLINE:HK:00700:1m");
    assert_eq!(first["entries"][0]["channel"], "KLINE");
    assert_eq!(first["entries"][0]["interval"], "1m");

    acquire(
        "chart-main",
        json!([{"market": "US", "symbol": "AAPL"}]),
    );
    acquire("other", json!([{"market": "HK", "symbol": "00700"}]));

    // A single-target release only removes that consumer/target pair.
    let released = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/release".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart-main",
                "instruments": [
                    {"channel": "KLINE", "market": "HK", "symbol": "00700", "interval": "1m"}
                ],
            }))
            .expect("release body"),
        })
        .expect("release subscription");
    assert_eq!(released["released"], true);
    assert_eq!(released["totalActiveSubscriptions"], 2);
    let keys = released["entries"]
        .as_array()
        .expect("release entries")
        .iter()
        .map(|entry| entry["key"].as_str().unwrap_or_default().to_owned())
        .collect::<Vec<_>>();
    assert!(
        !keys.contains(&"KLINE:HK:00700:1m".to_owned()),
        "released target still present: {released}"
    );
    assert!(
        keys.contains(&"SNAPSHOT:US:AAPL".to_owned()),
        "chart-main snapshot entry was removed by single release: {released}"
    );
    assert!(
        keys.contains(&"SNAPSHOT:HK:00700".to_owned()),
        "other consumer entry missing after single release: {released}"
    );

    // Clearing one consumer keeps the other consumer's entry untouched.
    let cleared = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: "consumerId=other".to_owned(),
            body: Vec::new(),
        })
        .expect("clear consumer");
    assert_eq!(cleared["cleared"], true);
    let entries = cleared["entries"].as_array().expect("clear entries");
    assert_eq!(entries.len(), 1, "remaining entry: {cleared}");
    assert_eq!(entries[0]["key"], "SNAPSHOT:US:AAPL");
    assert_eq!(entries[0]["consumers"], json!(["chart-main"]));
    assert_eq!(entries[0]["refCount"], 1);

    // Clear-all with no consumer filter drops every unmanaged entry.
    let cleared_all = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: Vec::new(),
        })
        .expect("clear all");
    assert_eq!(cleared_all["totalActiveSubscriptions"], 0);
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:205 TestSubscriptionReleaseConsumerOnlyClearsConsumer
///
/// Go shares one entry (`SNAPSHOT:HK:00700`) between `chart-main` and `other`,
/// then releases only `chart-main` by consumer id.  The shared entry must
/// survive with `other` as its single consumer.
#[test]
fn consumer_only_release_keeps_other_consumer_entry() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(std::sync::Mutex::new(
        jftrade_marketdata::ProviderRouter::new(32),
    ));
    let port =
        ProductionMarketDataSubscriptionMutationPort::new(active, Some(router.clone()), None);

    for (consumer, instruments) in [
        (
            "chart-main",
            json!([{"market": "HK", "symbol": "00700"}, {"market": "US", "symbol": "AAPL"}]),
        ),
        ("other", json!([{"market": "HK", "symbol": "00700"}])),
    ] {
        port.dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": consumer,
                "instruments": instruments,
            }))
            .expect("acquire body"),
        })
        .expect("acquire subscription");
    }
    assert_eq!(
        router.lock().expect("router").demand().logical_count,
        2,
        "shared HK entry must stay logically deduplicated"
    );

    let released = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/release".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({"consumerId": "chart-main"}))
                .expect("release body"),
        })
        .expect("consumer-only release");
    assert_eq!(released["totalActiveSubscriptions"], 1);
    let entries = released["entries"].as_array().expect("release entries");
    assert_eq!(entries.len(), 1, "remaining entry: {released}");
    assert_eq!(entries[0]["key"], "SNAPSHOT:HK:00700");
    assert_eq!(entries[0]["consumers"], json!(["other"]));
    assert_eq!(entries[0]["refCount"], 1);
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:239 TestClearSubscriptionRoutePreservesRunningStrategyLease
///
/// Go holds a managed strategy lease on `KLINE:US:AAPL:5m` and then issues the
/// web `DELETE /subscriptions` (no `consumerId`).  The managed lease must
/// survive and remain visible with the strategy consumer id.
#[test]
fn clear_route_preserves_running_strategy_lease() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(std::sync::Mutex::new(streaming_router()));
    router
        .lock()
        .expect("router")
        .acquire_demand(
            "strategy-runtime:one",
            [jftrade_marketdata::InstrumentRef {
                channel: "KLINE".to_owned(),
                market: "US".to_owned(),
                symbol: "AAPL".to_owned(),
                interval: Some("5m".to_owned()),
            }],
            true,
            10,
        )
        .expect("managed strategy lease");
    let port =
        ProductionMarketDataSubscriptionMutationPort::new(active, Some(router), None);

    port.dispatch(&MarketDataSubscriptionMutationRequest {
        method: "POST".to_owned(),
        path: "/api/v1/market-data/subscriptions".to_owned(),
        query: String::new(),
        body: serde_json::to_vec(&json!({
            "consumerId": "chart-main",
            "instruments": [{"market": "HK", "symbol": "00700"}],
        }))
        .expect("acquire body"),
    })
    .expect("acquire web subscription");

    let cleared = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "DELETE".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: Vec::new(),
        })
        .expect("clear web subscriptions");
    assert_eq!(
        cleared["totalActiveSubscriptions"], 1,
        "web cleanup removed the strategy lease: {cleared}"
    );
    let entries = cleared["entries"].as_array().expect("clear entries");
    assert_eq!(entries.len(), 1, "remaining managed entry: {cleared}");
    assert_eq!(entries[0]["key"], "KLINE:US:AAPL:5m");
    assert_eq!(entries[0]["consumers"], json!(["strategy-runtime:one"]));
}

/// A router that can hold managed (strategy) leases: the active provider must
/// advertise streaming quotes, matching the production Futu descriptor.
fn streaming_router() -> jftrade_marketdata::ProviderRouter {
    use jftrade_marketdata::{
        ActivationMode, HealthStatus, ProviderCapabilities, ProviderConstraints,
        ProviderDescriptor, ProviderReadiness, ProviderRouter,
    };
    let mut router = ProviderRouter::new(32);
    router
        .register(
            ProviderDescriptor {
                selection_id: "futu".to_owned(),
                provider_id: "futu-opend".to_owned(),
                display_name: "Futu OpenD".to_owned(),
                broker_id: Some("futu".to_owned()),
                source: "bbgo:futu".to_owned(),
                default_market: "HK".to_owned(),
                supported_markets: vec!["HK".to_owned(), "US".to_owned()],
                transports: vec!["opend-tcp".to_owned()],
                capabilities: ProviderCapabilities {
                    snapshots: true,
                    streaming_quotes: true,
                    streaming_candles: true,
                    streaming_depth: true,
                    historical_candles: true,
                    tick_candles: true,
                    order_book_depth: true,
                    ..ProviderCapabilities::default()
                },
                constraints: ProviderConstraints::default(),
                notes: Vec::new(),
            },
            HealthStatus {
                connected: true,
                stream_mode: "push-stream".to_owned(),
                readiness: ProviderReadiness::Ready,
                ..HealthStatus::default()
            },
        )
        .expect("register futu descriptor");
    router
        .activate("futu", ActivationMode::Explicit)
        .expect("activate futu");
    router
}

/// Parity: go:452dea11:internal/api/marketdata/routes_test.go:96 TestSubscriptionRoutesUseBrokerNeutralPollingWithoutFutuLease
///
/// Go acquires, heartbeats and releases a subscription with
/// `providerBrokerId:" Alpha "` for a broker-neutral (non-Futu) provider.  The
/// response is the polling projection: normalized broker id, no active
/// logical subscriptions, zero quota, and `snapshot-poll-fallback` transport;
/// the Futu lease book must stay empty the whole time.
#[test]
fn broker_neutral_polling_acquire_heartbeat_release_never_consumes_futu_lease() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(std::sync::Mutex::new(streaming_router()));
    let port = ProductionMarketDataSubscriptionMutationPort::new(
        active,
        Some(router.clone()),
        None,
    );

    let acquired = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart-alpha",
                "providerBrokerId": " Alpha ",
                "instruments": [
                    {"market": "US", "symbol": "AAPL", "channel": "KLINE", "interval": "1m"}
                ],
            }))
            .expect("acquire body"),
        })
        .expect("broker polling acquire");
    assert_eq!(acquired["providerBrokerId"], "alpha");
    assert_eq!(acquired["action"], "acquired");
    assert_eq!(acquired["totalActiveSubscriptions"], 0);
    assert_eq!(acquired["quota"]["totalUsed"], 0);
    assert_eq!(acquired["transport"]["mode"], "snapshot-poll-fallback");
    assert_eq!(
        router.lock().expect("router").demand().logical_count,
        0,
        "broker polling must not consume a Futu logical lease"
    );

    let heartbeat = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/heartbeat".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart-alpha",
                "providerBrokerId": "alpha",
            }))
            .expect("heartbeat body"),
        })
        .expect("broker polling heartbeat");
    assert_eq!(heartbeat["action"], "heartbeat");
    assert_eq!(heartbeat["totalActiveSubscriptions"], 0);

    let released = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/release".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart-alpha",
                "providerBrokerId": "alpha",
                "instruments": [
                    {"market": "US", "symbol": "AAPL", "channel": "KLINE", "interval": "1m"}
                ],
            }))
            .expect("release body"),
        })
        .expect("broker polling release");
    assert_eq!(released["action"], "released");
    assert_eq!(released["transport"]["mode"], "snapshot-poll-fallback");
    assert_eq!(router.lock().expect("router").demand().logical_count, 0);
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:502 TestSubscriptionRequestHelpersPreserveOnlyValidTargets
///
/// Go filters blank market/symbol pairs before validating acquire targets, and
/// treats a release target with only one identity half as invalid rather than
/// silently releasing the whole consumer.  Rust keeps that filtering inside
/// the subscription mutation owner, so the equivalent regression drives a
/// mixed acquire list (two invalid + one valid) and an incomplete release
/// target through the port.
#[test]
fn subscription_request_helpers_preserve_only_valid_targets() {
    let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    let router = Arc::new(std::sync::Mutex::new(streaming_router()));
    let port = ProductionMarketDataSubscriptionMutationPort::new(
        active,
        Some(router.clone()),
        None,
    );

    let acquired = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart",
                "instruments": [
                    {"market": " ", "symbol": "AAPL"},
                    {"market": "US", "symbol": " "},
                    {"channel": "KLINE", "market": "US", "symbol": "AAPL", "interval": "1m"}
                ],
            }))
            .expect("acquire body"),
        })
        .expect("mixed acquire list keeps the valid target");
    let entries = acquired["entries"].as_array().expect("acquire entries");
    assert_eq!(entries.len(), 1, "only the complete target may survive: {acquired}");
    assert_eq!(entries[0]["key"], "KLINE:US:AAPL:1m");
    assert_eq!(
        router.lock().expect("router").demand().logical_count,
        1,
        "blank identity pairs must not enter the demand book"
    );

    // An all-invalid acquire list is rejected instead of acquiring nothing.
    let error = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart",
                "instruments": [{"market": "US"}],
            }))
            .expect("all-invalid acquire body"),
        })
        .expect_err("all-invalid targets must be rejected");
    assert!(matches!(
        error,
        MarketDataSubscriptionMutationPortError::Failed {
            status: 400,
            ref code,
            ref message,
            ..
        } if code == "BAD_REQUEST"
            && message == "consumerId and instruments are required"
    ));

    // A release target missing either half is an input error, not a
    // consumer-wide release.
    let error = port
        .dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions/release".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart",
                "instruments": [{"market": "US"}],
            }))
            .expect("incomplete release body"),
        })
        .expect_err("incomplete release target must be rejected");
    assert!(matches!(
        error,
        MarketDataSubscriptionMutationPortError::Failed {
            status: 400,
            ref code,
            ref message,
            ..
        } if code == "BAD_REQUEST"
            && message == "release target market and symbol are required"
    ));
    assert_eq!(
        router.lock().expect("router").demand().logical_count,
        1,
        "rejected release must keep the acquired demand"
    );
}

/// A physical snapshot port that succeeds until armed, standing in for the Go
/// test's reconciler that cancels the context after the logical cleanup in
/// release/clear (but not while the test acquires its starting demand).
#[derive(Debug, Default)]
struct SwitchablePhysicalSnapshotPort {
    failing: std::sync::atomic::AtomicBool,
}

impl SwitchablePhysicalSnapshotPort {
    fn fail_from_now_on(&self) {
        self.failing.store(true, std::sync::atomic::Ordering::SeqCst);
    }
}

impl jftrade_marketdata::PhysicalSubscriptionSnapshotPort for SwitchablePhysicalSnapshotPort {
    fn physical_subscription_snapshot(
        &self,
    ) -> Result<Option<jftrade_marketdata::PhysicalSubscriptionSnapshot>, String> {
        if self.failing.load(std::sync::atomic::Ordering::SeqCst) {
            return Err("subscription reconciliation canceled".to_owned());
        }
        Ok(None)
    }
}

/// Parity: go:452dea11:internal/api/marketdata/routes_boundaries_test.go:555 TestReleaseAndClearMapSnapshotCancellationAfterLogicalCleanup
///
/// Go cancels the snapshot reconciler from inside `ReconcileSubscriptions`, so
/// release and clear finish their logical bookkeeping but still answer
/// 500 SUBSCRIPTION_FAILED.  Rust keeps the logical book in the demand router
/// and the physical snapshot behind its own port, so the equivalent regression
/// gives the port a failing snapshot read: both mutations must have already
/// dropped the logical demand and must still surface 500 SUBSCRIPTION_FAILED.
#[test]
fn release_and_clear_map_snapshot_failure_after_logical_cleanup() {
    for (name, request, remaining) in [
        (
            "release",
            MarketDataSubscriptionMutationRequest {
                method: "POST".to_owned(),
                path: "/api/v1/market-data/subscriptions/release".to_owned(),
                query: String::new(),
                body: br#"{"consumerId":"chart"}"#.to_vec(),
            },
            0usize,
        ),
        (
            "clear",
            MarketDataSubscriptionMutationRequest {
                method: "DELETE".to_owned(),
                path: "/api/v1/market-data/subscriptions".to_owned(),
                query: String::new(),
                body: Vec::new(),
            },
            0usize,
        ),
    ] {
        let active = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
        let router = Arc::new(std::sync::Mutex::new(streaming_router()));
        let physical = Arc::new(SwitchablePhysicalSnapshotPort::default());
        let port = ProductionMarketDataSubscriptionMutationPort::new(
            active,
            Some(router.clone()),
            Some(physical.clone()),
        );
        port.dispatch(&MarketDataSubscriptionMutationRequest {
            method: "POST".to_owned(),
            path: "/api/v1/market-data/subscriptions".to_owned(),
            query: String::new(),
            body: serde_json::to_vec(&json!({
                "consumerId": "chart",
                "instruments": [{"market": "US", "symbol": "AAPL"}],
            }))
            .expect("acquire body"),
        })
        .expect("acquire subscription");
        assert_eq!(router.lock().expect("router").demand().logical_count, 1);
        physical.fail_from_now_on();

        let error = port
            .dispatch(&request)
            .expect_err("failed physical snapshot must surface after cleanup");
        assert!(
            matches!(
                error,
                MarketDataSubscriptionMutationPortError::Failed {
                    status: 500,
                    ref code,
                    ..
                } if code == "SUBSCRIPTION_FAILED"
            ),
            "{name} did not map the snapshot failure to SUBSCRIPTION_FAILED: {error:?}"
        );
        assert_eq!(
            router.lock().expect("router").demand().logical_count,
            remaining,
            "{name} must clear the logical demand before reporting the snapshot failure"
        );
    }
}
