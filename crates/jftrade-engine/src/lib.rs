#![forbid(unsafe_code)]

use std::env;
use std::net::SocketAddr;

use serde::Serialize;
use thiserror::Error;
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use tokio_stream::wrappers::TcpListenerStream;
use tonic::service::interceptor::InterceptedService;
use tonic::{Request, Status};
use tonic_health::ServingStatus;
use tonic_health::server::health_reporter;

pub mod stage4;
pub mod stage5;
pub mod stage6;

/// Environment variable containing the per-process bridge bearer token.
pub const AUTH_TOKEN_ENV: &str = "JFTRADE_RUST_ENGINE_TOKEN";
/// Environment variable overriding the loopback bind address.
pub const BIND_ADDRESS_ENV: &str = "JFTRADE_RUST_ENGINE_BIND";
/// Versioned service name used by the standard gRPC health protocol.
pub const HEALTH_SERVICE_NAME: &str = "jftrade.migration.v1.Engine";
/// Version of the private Go/Rust migration bridge contract.
pub const PROTOCOL_VERSION: &str = "migration.v1";

const DEFAULT_BIND_ADDRESS: &str = "127.0.0.1:0";
const MINIMUM_TOKEN_LENGTH: usize = 32;

/// Validated startup configuration for the private migration engine.
pub struct EngineConfig {
    bind_address: SocketAddr,
    auth_token: String,
}

impl EngineConfig {
    /// Creates a configuration and enforces the Stage 1 loopback/auth boundary.
    pub fn new(
        bind_address: SocketAddr,
        auth_token: impl Into<String>,
    ) -> Result<Self, EngineError> {
        let auth_token = auth_token.into();
        validate_bind_address(bind_address)?;
        validate_auth_token(&auth_token)?;
        Ok(Self {
            bind_address,
            auth_token,
        })
    }

    /// Reads configuration from the process environment.
    pub fn from_process_env() -> Result<Self, EngineError> {
        let token = env::var(AUTH_TOKEN_ENV).map_err(|_| EngineError::MissingAuthToken)?;
        let bind_address = env::var(BIND_ADDRESS_ENV)
            .unwrap_or_else(|_| DEFAULT_BIND_ADDRESS.to_owned())
            .parse()
            .map_err(EngineError::InvalidBindAddress)?;
        Self::new(bind_address, token)
    }
}

/// Machine-readable readiness record emitted once the loopback listener is active.
#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct StartupRecord {
    /// Event discriminator for supervisor parsing.
    pub event: &'static str,
    /// Actual listener address. Port zero is resolved before this record is emitted.
    pub address: SocketAddr,
    /// Private bridge protocol version.
    pub protocol_version: &'static str,
    /// Standard health service queried by future Go supervisors.
    pub health_service: &'static str,
}

/// Running migration engine plus its explicit shutdown owner.
pub struct EngineHandle {
    startup_record: StartupRecord,
    shutdown_tx: Option<oneshot::Sender<()>>,
    task: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
}

impl EngineHandle {
    /// Returns the readiness handshake without exposing the bearer token.
    pub const fn startup_record(&self) -> &StartupRecord {
        &self.startup_record
    }

    /// Requests graceful shutdown and waits for the listener task to join.
    pub async fn shutdown(mut self) -> Result<(), EngineError> {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
        if let Some(task) = self.task.take() {
            task.await.map_err(EngineError::Join)??;
        }
        Ok(())
    }
}

impl Drop for EngineHandle {
    fn drop(&mut self) {
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }
    }
}

/// Errors that prevent the private Stage 1 bridge from starting or stopping cleanly.
#[derive(Debug, Error)]
pub enum EngineError {
    /// The mandatory per-process bearer token is absent.
    #[error("{AUTH_TOKEN_ENV} is required")]
    MissingAuthToken,
    /// The bearer token does not satisfy the local bridge policy.
    #[error("the engine auth token must be at least 32 URL-safe ASCII characters")]
    InvalidAuthToken,
    /// The bind address is not parseable.
    #[error("invalid engine bind address")]
    InvalidBindAddress(#[source] std::net::AddrParseError),
    /// Stage 1 never permits a non-loopback listener.
    #[error("the migration engine may only bind to a loopback address")]
    NonLoopbackBind,
    /// The listener could not be opened.
    #[error("failed to bind the migration engine")]
    Bind(#[source] std::io::Error),
    /// The bound local address could not be inspected.
    #[error("failed to inspect the migration engine listener")]
    LocalAddress(#[source] std::io::Error),
    /// The listener task could not be joined.
    #[error("migration engine task failed")]
    Join(#[source] tokio::task::JoinError),
    /// Tonic terminated the gRPC server with an error.
    #[error("migration engine transport failed")]
    Transport(#[from] tonic::transport::Error),
}

/// Starts the authenticated, loopback-only health bridge.
pub async fn start_engine(config: EngineConfig) -> Result<EngineHandle, EngineError> {
    let listener = TcpListener::bind(config.bind_address)
        .await
        .map_err(EngineError::Bind)?;
    let address = listener.local_addr().map_err(EngineError::LocalAddress)?;
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    let (health_reporter, health_service) = health_reporter();
    health_reporter
        .set_service_status(HEALTH_SERVICE_NAME, ServingStatus::Serving)
        .await;

    let expected_authorization = format!("Bearer {}", config.auth_token);
    let authenticated_health = InterceptedService::new(health_service, move |request| {
        authenticate_request(request, &expected_authorization)
    });
    let incoming = TcpListenerStream::new(listener);
    let task = tokio::spawn(async move {
        tonic::transport::Server::builder()
            .add_service(authenticated_health)
            .serve_with_incoming_shutdown(incoming, async {
                let _ = shutdown_rx.await;
            })
            .await
    });

    Ok(EngineHandle {
        startup_record: StartupRecord {
            event: "ready",
            address,
            protocol_version: PROTOCOL_VERSION,
            health_service: HEALTH_SERVICE_NAME,
        },
        shutdown_tx: Some(shutdown_tx),
        task: Some(task),
    })
}

fn validate_bind_address(bind_address: SocketAddr) -> Result<(), EngineError> {
    if bind_address.ip().is_loopback() {
        Ok(())
    } else {
        Err(EngineError::NonLoopbackBind)
    }
}

fn validate_auth_token(token: &str) -> Result<(), EngineError> {
    let valid = token.len() >= MINIMUM_TOKEN_LENGTH
        && token
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_'));
    if valid {
        Ok(())
    } else {
        Err(EngineError::InvalidAuthToken)
    }
}

fn authenticate_request(
    request: Request<()>,
    expected_authorization: &str,
) -> Result<Request<()>, Status> {
    let authorized = request
        .metadata()
        .get("authorization")
        .is_some_and(|value| value.as_bytes() == expected_authorization.as_bytes());
    if authorized {
        Ok(request)
    } else {
        Err(Status::unauthenticated(
            "missing or invalid engine bearer token",
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const TEST_TOKEN: &str = "stage1_test_token_0123456789abcdef";

    #[test]
    fn configuration_rejects_public_listeners_and_weak_tokens() {
        let public_address = "0.0.0.0:0".parse().expect("public test address");
        assert!(matches!(
            EngineConfig::new(public_address, TEST_TOKEN),
            Err(EngineError::NonLoopbackBind)
        ));

        let loopback_address = "127.0.0.1:0".parse().expect("loopback test address");
        assert!(matches!(
            EngineConfig::new(loopback_address, "short"),
            Err(EngineError::InvalidAuthToken)
        ));
        assert!(matches!(
            EngineConfig::new(loopback_address, "invalid token with spaces 123456789"),
            Err(EngineError::InvalidAuthToken)
        ));
    }

    #[test]
    fn authentication_requires_the_exact_bearer_value() {
        let expected = format!("Bearer {TEST_TOKEN}");
        let request = Request::new(());
        assert_eq!(
            authenticate_request(request, &expected)
                .expect_err("missing auth must fail")
                .code(),
            tonic::Code::Unauthenticated
        );

        let mut request = Request::new(());
        request.metadata_mut().insert(
            "authorization",
            expected.parse().expect("valid authorization metadata"),
        );
        assert!(authenticate_request(request, &expected).is_ok());
    }
}
