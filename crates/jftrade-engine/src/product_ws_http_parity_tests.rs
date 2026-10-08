//! Real production upgrades, notification replay and listener teardown.

use super::super::security_stream_tests::{protected_product_config, web_client, web_login};
use super::*;

#[path = "product_ws_depth_http_parity_tests.rs"]
mod depth_parity;

#[path = "product_ws_security_http_parity_tests.rs"]
mod security_parity;

const PROTOCOL: &[(&str, &str)] = &[(
    "Sec-WebSocket-Protocol",
    "jftrade.desktop.v1, aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
)];
const AUTH: &[(&str, &str)] = &[("Authorization", "Bearer aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")];

fn config(directory: &tempfile::TempDir) -> ProductConfig {
    let path = directory.path().join("settings.json");
    product_data_management::initialize_production_databases(&path).expect("databases");
    ProductConfig::desktop_production(
        "127.0.0.1:0".parse().expect("address"),
        &path,
        "a".repeat(32),
    )
    .expect("config")
}

async fn connect(address: std::net::SocketAddr) -> TcpStream {
    let response = websocket_handshake(address, PROTOCOL).await;
    assert_eq!(response.status, 101, "{response:?}");
    let mut socket = response.upgraded_stream.expect("upgraded");
    let heartbeat: serde_json::Value =
        serde_json::from_str(&read_server_text_frame(&mut socket).await).expect("heartbeat");
    assert_eq!(heartbeat["type"], "heartbeat");
    socket
}

async fn notification(socket: &mut TcpStream) -> serde_json::Value {
    tokio::time::timeout(std::time::Duration::from_secs(2), async {
        loop {
            let event: serde_json::Value =
                serde_json::from_str(&read_server_text_frame(socket).await).expect("event");
            if event["type"] == "system.notification" {
                return event;
            }
            assert_eq!(event["type"], "heartbeat", "unexpected event {event}");
        }
    })
    .await
    .expect("notification deadline")
}

fn event(sequence: u64) -> serde_json::Value {
    serde_json::json!({
        "eventId": format!("system.notification|system-notification-{sequence}"),
        "type": "system.notification", "source": "notification",
        "entityId": format!("system-notification-{sequence}"),
        "serverTime": "2026-06-14T00:00:00Z",
        "payload": {"id": format!("system-notification-{sequence}"),
            "sequence": sequence, "at": "2026-06-14T00:00:00Z", "level": "info",
            "title": "ready", "message": "connected", "source": "test", "category": "system"}
    })
}

// Parity: go:452dea11:internal/api/live/handler_test.go:207 TestHandlerRejectsSubscriptionWithoutProvider
#[tokio::test]
async fn production_websocket_missing_provider_closes_and_releases_connection() {
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory)).await.expect("production");
    let address = handle.startup_record().address;
    let mut socket = connect(address).await;
    socket
        .write_all(&masked_text_frame(
            br#"{"type":"subscribe","subscriptions":{"activeInstruments":["US.AAPL"]}}"#,
        ))
        .await
        .expect("original missing provider request");
    let close = tokio::time::timeout(
        std::time::Duration::from_secs(2),
        read_server_close_frame(&mut socket),
    )
    .await
    .expect("close deadline");
    assert_eq!(close, (1008, "subscription policy violation".to_owned()));
    drop(socket);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    assert_eq!(handle.live_hub().snapshot().connected, 0);
    let fresh = connect(address).await;
    wait_for_live_projection_with_headers(address, 1, &[], AUTH).await;
    drop(fresh);
    wait_for_live_projection_with_headers(address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/live/handler_test.go:421 TestHandlerAcceptsSameOriginWebSocket
#[tokio::test]
async fn production_websocket_accepts_authenticated_listener_origin_and_rejects_foreign_port() {
    let directory = tempdir().expect("directory");
    let (config, web_address) = protected_product_config(&directory.path().join("settings.json"));
    let handle = start_product(config).await.expect("production");
    let client = web_client();
    let (cookie, _) = web_login(&client, web_address, "original browser password").await;
    let origin = format!("http://{web_address}");
    let accepted =
        websocket_handshake(web_address, &[("Cookie", &cookie), ("Origin", &origin)]).await;
    assert_eq!(accepted.status, 101);
    let mut socket = accepted.upgraded_stream.expect("same-origin socket");
    let heartbeat: serde_json::Value =
        serde_json::from_str(&read_server_text_frame(&mut socket).await).expect("heartbeat");
    assert_eq!(heartbeat["type"], "heartbeat");
    let foreign_origin = format!("http://{}", handle.startup_record().address);
    let denied = websocket_handshake(
        web_address,
        &[("Cookie", &cookie), ("Origin", &foreign_origin)],
    )
    .await;
    assert_eq!(denied.status, 403);
    let unauthenticated = websocket_handshake(web_address, &[("Origin", &origin)]).await;
    assert_eq!(unauthenticated.status, 401);
    drop(socket);
    wait_for_live_projection_with_headers(handle.startup_record().address, 0, &[], AUTH).await;
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/api/live/handler_test.go:316 TestHandlerNotificationSequenceZeroReplay
#[tokio::test]
async fn production_websocket_replays_preconnection_notification_and_receives_live_successor_once()
{
    let directory = tempdir().expect("directory");
    let handle = start_product(config(&directory)).await.expect("production");
    let hub = handle.live_hub();
    assert!(
        !hub.publish(event(1)),
        "no subscriber must not acknowledge delivery"
    );
    let mut socket = connect(handle.startup_record().address).await;
    let replay = notification(&mut socket).await;
    assert_eq!(replay, event(1));
    // An execution projector may retry a retained event until a client arrives.
    assert!(hub.publish(event(1)));
    assert!(hub.publish(event(2)));
    assert_eq!(notification(&mut socket).await, event(2));
    let silent = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        notification(&mut socket),
    )
    .await;
    assert!(silent.is_err(), "notification delivered twice");
    drop(socket);
    wait_for_live_projection_with_headers(handle.startup_record().address, 0, &[], AUTH).await;
    let mut reconnected = connect(handle.startup_record().address).await;
    assert_eq!(notification(&mut reconnected).await, event(1));
    assert_eq!(notification(&mut reconnected).await, event(2));
    drop(reconnected);
    handle.shutdown().await.expect("shutdown");
}

// Parity: go:452dea11:internal/app/apiserver/lifecycle/lifecycle_test.go:243 TestSeparateWebListenerStartsAlongsideLoopbackDesktopSidecar
#[tokio::test]
async fn production_separate_web_listener_preserves_desktop_fence_and_releases_both_ports() {
    let directory = tempdir().expect("directory");
    let path = directory.path().join("settings.json");
    assert!(matches!(
        ProductConfig::new(
            "0.0.0.0:0".parse().expect("Go bind"),
            &path,
            AccessPolicy::desktop(Some("a".repeat(32)))
        ),
        Err(ProductError::NonLoopbackBind)
    ));
    let (config, web_address) = protected_product_config(&path);
    let handle = start_product(config).await.expect("production");
    let desktop_address = handle.startup_record().address;
    assert_ne!(desktop_address, web_address);
    let client = web_client();
    let root = client
        .get(format!("http://{web_address}/"))
        .send()
        .await
        .expect("Web root");
    assert_eq!(
        root.status(),
        404,
        "no embedded assets installed in this product fixture"
    );
    let (cookie, _) = web_login(&client, web_address, "original browser password").await;
    let origin = format!("http://{web_address}");
    let browser =
        websocket_handshake(web_address, &[("Cookie", &cookie), ("Origin", &origin)]).await;
    assert_eq!(browser.status, 101);
    let mut browser = browser.upgraded_stream.expect("browser");
    assert!(
        read_server_text_frame(&mut browser)
            .await
            .contains("heartbeat")
    );
    let mut desktop = connect(desktop_address).await;
    handle.shutdown().await.expect("shutdown both listeners");
    assert_eq!(read_server_close_frame(&mut browser).await.0, 1001);
    assert_eq!(read_server_close_frame(&mut desktop).await.0, 1001);
    assert!(TcpStream::connect(web_address).await.is_err());
    assert!(TcpStream::connect(desktop_address).await.is_err());
    let _web = std::net::TcpListener::bind(web_address).expect("Web port released");
    let _desktop = std::net::TcpListener::bind(desktop_address).expect("desktop port released");
}
