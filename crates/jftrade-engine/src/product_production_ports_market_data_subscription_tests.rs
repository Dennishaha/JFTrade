use super::*;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use jftrade_integration_futu::{PredictionMarketReadError, PredictionMarketSubscriptionPort};
use serde_json::json;
use std::sync::atomic::{AtomicUsize, Ordering};

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
