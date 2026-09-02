#![forbid(unsafe_code)]

use jftrade_engine::{EngineConfig, HEALTH_SERVICE_NAME, start_engine};
use tonic::metadata::MetadataValue;
use tonic::transport::Endpoint;
use tonic::{Request, Status};
use tonic_health::pb::HealthCheckRequest;
use tonic_health::pb::health_check_response::ServingStatus;
use tonic_health::pb::health_client::HealthClient;

const TEST_TOKEN: &str = "stage1_integration_token_0123456789ab";

#[tokio::test]
async fn authenticated_health_bridge_reports_serving() {
    let config = EngineConfig::new(
        "127.0.0.1:0".parse().expect("loopback test address"),
        TEST_TOKEN,
    )
    .expect("valid test configuration");
    let handle = start_engine(config).await.expect("start health bridge");
    let endpoint = Endpoint::from_shared(format!("http://{}", handle.startup_record().address))
        .expect("valid local endpoint");
    let channel = endpoint.connect().await.expect("connect to health bridge");
    let authorization: MetadataValue<_> = format!("Bearer {TEST_TOKEN}")
        .parse()
        .expect("valid bearer metadata");
    let mut client = HealthClient::with_interceptor(channel, move |mut request: Request<()>| {
        request
            .metadata_mut()
            .insert("authorization", authorization.clone());
        Ok(request)
    });

    let response = client
        .check(HealthCheckRequest {
            service: HEALTH_SERVICE_NAME.to_owned(),
        })
        .await
        .expect("authenticated health check")
        .into_inner();
    assert_eq!(response.status, ServingStatus::Serving as i32);

    handle.shutdown().await.expect("graceful shutdown");
}

#[tokio::test]
async fn unauthenticated_health_bridge_fails_closed() {
    let config = EngineConfig::new(
        "127.0.0.1:0".parse().expect("loopback test address"),
        TEST_TOKEN,
    )
    .expect("valid test configuration");
    let handle = start_engine(config).await.expect("start health bridge");
    let endpoint = Endpoint::from_shared(format!("http://{}", handle.startup_record().address))
        .expect("valid local endpoint");
    let channel = endpoint.connect().await.expect("connect to health bridge");
    let error: Status = HealthClient::new(channel)
        .check(HealthCheckRequest {
            service: HEALTH_SERVICE_NAME.to_owned(),
        })
        .await
        .expect_err("missing token must be rejected");
    assert_eq!(error.code(), tonic::Code::Unauthenticated);

    handle.shutdown().await.expect("graceful shutdown");
}
