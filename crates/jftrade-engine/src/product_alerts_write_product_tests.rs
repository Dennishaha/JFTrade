use super::*;
use crate::product::product_alerts_write_port::{
    AlertWriteAction, AlertWritePort, AlertWritePortError, AlertWriteResolution, AlertWriteRoute,
};
use serde_json::{Value, json};
use std::sync::Arc;
use tempfile::tempdir;

#[derive(Debug)]
struct FixtureAlertWritePort {
    mode: &'static str,
}

impl AlertWritePort for FixtureAlertWritePort {
    fn resolve(
        &self,
        _route: AlertWriteRoute,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<AlertWriteResolution, AlertWritePortError> {
        if self.mode == "unavailable" {
            return Err(AlertWritePortError::Unavailable(
                "alert write fixture is unavailable".to_owned(),
            ));
        }
        Ok(AlertWriteResolution {
            broker_id: broker_id.unwrap_or("futu").to_owned(),
            security_firm: "Futu/Moomoo via OpenD".to_owned(),
            capability: "available".to_owned(),
            selection_reason: "explicit_broker".to_owned(),
        })
    }

    fn apply(
        &self,
        _resolution: &AlertWriteResolution,
        action: &AlertWriteAction,
    ) -> Result<Option<Value>, AlertWritePortError> {
        if self.mode == "provider-forbidden" {
            return Err(AlertWritePortError::Provider {
                status: Some(403),
                message: "provider denied alert write".to_owned(),
            });
        }
        Ok(Some(json!({
            "entries": [{
                "accepted": true,
                "featureId": action.feature_id,
                "operation": action.action,
            }]
        })))
    }
}

#[tokio::test]
async fn alerts_write_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_write_port(Arc::new(FixtureAlertWritePort { mode: "success" }));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/alerts/price" })
    );

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/alerts/price?brokerId=futu&accountId=acct-1",
        Some(r#"{"symbol":"US.AAPL","price":190.5}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["provider"]["brokerId"], "futu");
    assert_eq!(response["data"]["entries"][0]["accepted"], true);

    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/alerts/option-events?brokerId=futu",
        Some("{"),
        &[],
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn alerts_write_routes_preserve_provider_failure_and_default_route_isolation() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_write_port(Arc::new(FixtureAlertWritePort {
                mode: "provider-forbidden",
            }));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/alerts/price?brokerId=futu",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 403);
    assert_eq!(response["error"]["code"], "PROVIDER_REQUEST_FAILED");
    handle.shutdown().await.expect("shutdown product");

    let isolated =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(isolated)
        .await
        .expect("start isolated product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/alerts/price",
        Some("{}"),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown isolated product");
}
