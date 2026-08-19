use std::time::Duration;

use jftrade_marketdata::ProviderReadiness;
use reqwest::{Client, Method, StatusCode, Url};
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use thiserror::Error;

const MAX_RESPONSE_BYTES: usize = 4 << 20;

#[derive(Clone, Debug)]
pub struct HelperClientConfig {
    pub base_url: String,
    pub bearer_token: Option<String>,
    pub request_timeout: Duration,
    pub max_attempts: usize,
    pub retry_delay: Duration,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct HelperHealth {
    pub provider: String,
    pub runtime_state: ProviderReadiness,
    #[serde(default)]
    pub version: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub last_error: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct HelperErrorEnvelope {
    pub error: HelperRemoteError,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct HelperRemoteError {
    #[serde(default)]
    pub code: String,
    pub message: String,
}

#[derive(Debug, Error)]
pub enum HttpAdapterError {
    #[error("invalid market-data helper URL: {0}")]
    InvalidUrl(String),
    #[error("market-data helper token must contain at least 32 non-whitespace characters")]
    WeakToken,
    #[error("market-data helper request timed out")]
    Timeout,
    #[error("market-data helper is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data helper returned invalid response: {0}")]
    InvalidResponse(String),
    #[error("market-data helper rejected request with {status}: [{code}] {message}")]
    Remote {
        status: u16,
        code: String,
        message: String,
    },
}

#[derive(Clone)]
pub struct HelperClient {
    base_url: Url,
    bearer_token: Option<String>,
    client: Client,
    max_attempts: usize,
    retry_delay: Duration,
}

impl HelperClient {
    pub fn new(config: HelperClientConfig) -> Result<Self, HttpAdapterError> {
        let base_url = Url::parse(config.base_url.trim())
            .map_err(|error| HttpAdapterError::InvalidUrl(error.to_string()))?;
        let loopback = base_url.host_str().is_some_and(|host| {
            host.eq_ignore_ascii_case("localhost")
                || host
                    .parse::<std::net::IpAddr>()
                    .is_ok_and(|ip| ip.is_loopback())
        });
        if base_url.scheme() != "http" || !loopback || base_url.port().is_none() {
            return Err(HttpAdapterError::InvalidUrl(
                "helper must use explicit-port HTTP on loopback".to_owned(),
            ));
        }
        let bearer_token = config
            .bearer_token
            .map(|token| token.trim().to_owned())
            .filter(|token| !token.is_empty());
        if bearer_token.as_ref().is_some_and(|token| token.len() < 32) {
            return Err(HttpAdapterError::WeakToken);
        }
        let client = Client::builder()
            .connect_timeout(config.request_timeout)
            .timeout(config.request_timeout)
            .redirect(reqwest::redirect::Policy::none())
            .build()
            .map_err(|error| HttpAdapterError::InvalidUrl(error.to_string()))?;
        Ok(Self {
            base_url,
            bearer_token,
            client,
            max_attempts: config.max_attempts.max(1),
            retry_delay: config.retry_delay,
        })
    }

    pub async fn health(&self, provider: &str) -> Result<HelperHealth, HttpAdapterError> {
        let provider = match provider.trim().to_ascii_lowercase().as_str() {
            "yfinance" => "yfinance",
            "akshare" => "akshare",
            _ => {
                return Err(HttpAdapterError::InvalidUrl(format!(
                    "unsupported provider {provider}"
                )));
            }
        };
        self.request_json(
            Method::GET,
            &["providers", provider, "health"],
            Option::<&()>::None,
        )
        .await
    }

    pub fn endpoint(&self) -> &str {
        self.base_url.as_str()
    }

    pub fn uses_authentication(&self) -> bool {
        self.bearer_token.is_some()
    }

    pub async fn healthz(&self) -> Result<serde_json::Value, HttpAdapterError> {
        self.request_json(Method::GET, &["healthz"], Option::<&()>::None)
            .await
    }

    pub async fn get_json<T: DeserializeOwned>(
        &self,
        segments: &[&str],
    ) -> Result<T, HttpAdapterError> {
        self.request_json(Method::GET, segments, Option::<&()>::None)
            .await
    }

    pub async fn post_json<I: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        segments: &[&str],
        input: &I,
    ) -> Result<T, HttpAdapterError> {
        self.request_json(Method::POST, segments, Some(input)).await
    }

    async fn request_json<I: Serialize + ?Sized, T: DeserializeOwned>(
        &self,
        method: Method,
        segments: &[&str],
        input: Option<&I>,
    ) -> Result<T, HttpAdapterError> {
        let mut endpoint = self.base_url.clone();
        {
            let mut path = endpoint.path_segments_mut().map_err(|()| {
                HttpAdapterError::InvalidUrl("base URL cannot join paths".to_owned())
            })?;
            path.pop_if_empty();
            for segment in segments {
                path.push(segment);
            }
        }
        for attempt in 1..=self.max_attempts {
            let mut request = self.client.request(method.clone(), endpoint.clone());
            if let Some(token) = &self.bearer_token {
                request = request.bearer_auth(token);
            }
            if let Some(value) = input {
                request = request.json(value);
            }
            match request.send().await {
                Ok(response) => {
                    let status = response.status();
                    let retry_after = response
                        .headers()
                        .get(reqwest::header::RETRY_AFTER)
                        .and_then(|value| value.to_str().ok())
                        .and_then(|value| value.trim().parse::<u64>().ok())
                        .map(Duration::from_secs);
                    let body = response.bytes().await.map_err(classify_reqwest)?;
                    if body.len() > MAX_RESPONSE_BYTES {
                        return Err(HttpAdapterError::InvalidResponse(format!(
                            "body exceeds {MAX_RESPONSE_BYTES} bytes"
                        )));
                    }
                    if status.is_success() {
                        return serde_json::from_slice(&body)
                            .map_err(|error| HttpAdapterError::InvalidResponse(error.to_string()));
                    }
                    if retryable(status) && attempt < self.max_attempts {
                        tokio::time::sleep(
                            retry_after
                                .unwrap_or_else(|| self.retry_delay.saturating_mul(attempt as u32)),
                        )
                        .await;
                        continue;
                    }
                    return Err(decode_remote_error(status, &body));
                }
                Err(error)
                    if (error.is_connect() || error.is_timeout())
                        && attempt < self.max_attempts =>
                {
                    tokio::time::sleep(self.retry_delay.saturating_mul(attempt as u32)).await;
                }
                Err(error) => return Err(classify_reqwest(error)),
            }
        }
        Err(HttpAdapterError::Unavailable(
            "retry budget exhausted".to_owned(),
        ))
    }
}

fn retryable(status: StatusCode) -> bool {
    matches!(
        status,
        StatusCode::REQUEST_TIMEOUT | StatusCode::TOO_EARLY | StatusCode::TOO_MANY_REQUESTS
    ) || status.is_server_error()
}

fn classify_reqwest(error: reqwest::Error) -> HttpAdapterError {
    if error.is_timeout() {
        HttpAdapterError::Timeout
    } else {
        HttpAdapterError::Unavailable(error.to_string())
    }
}

fn decode_remote_error(status: StatusCode, body: &[u8]) -> HttpAdapterError {
    match serde_json::from_slice::<HelperErrorEnvelope>(body) {
        Ok(envelope) => HttpAdapterError::Remote {
            status: status.as_u16(),
            code: envelope.error.code,
            message: envelope.error.message,
        },
        Err(_) => HttpAdapterError::Remote {
            status: status.as_u16(),
            code: String::new(),
            message: String::from_utf8_lossy(body).chars().take(512).collect(),
        },
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;
    use std::sync::atomic::{AtomicUsize, Ordering};

    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    use super::*;

    #[test]
    fn client_rejects_non_loopback_and_weak_tokens() {
        let config = |base_url: &str, token: Option<&str>| HelperClientConfig {
            base_url: base_url.to_owned(),
            bearer_token: token.map(str::to_owned),
            request_timeout: Duration::from_secs(1),
            max_attempts: 1,
            retry_delay: Duration::ZERO,
        };
        assert!(matches!(
            HelperClient::new(config("http://0.0.0.0:1234", None)),
            Err(HttpAdapterError::InvalidUrl(_))
        ));
        assert!(matches!(
            HelperClient::new(config("http://127.0.0.1:1234", Some("short"))),
            Err(HttpAdapterError::WeakToken)
        ));
        HelperClient::new(config(
            "http://127.0.0.1:1234",
            Some("0123456789abcdef0123456789abcdef"),
        ))
        .expect("valid client");
    }

    #[tokio::test]
    async fn retries_transient_readiness_and_sends_optional_bearer() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let attempts = Arc::new(AtomicUsize::new(0));
        let server_attempts = Arc::clone(&attempts);
        let server = tokio::spawn(async move {
            for response in [
                "HTTP/1.1 503 Service Unavailable\r\nContent-Length: 2\r\nRetry-After: 0\r\nConnection: close\r\n\r\n{}",
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 15\r\nConnection: close\r\n\r\n{\"status\":\"ok\"}",
            ] {
                let (mut stream, _) = listener.accept().await.expect("accept");
                let mut request = vec![0_u8; 4096];
                let read = stream.read(&mut request).await.expect("read request");
                let request = String::from_utf8_lossy(&request[..read]);
                assert!(request.starts_with("GET /healthz HTTP/1.1\r\n"));
                assert!(
                    request.contains("authorization: Bearer 0123456789abcdef0123456789abcdef\r\n")
                );
                server_attempts.fetch_add(1, Ordering::SeqCst);
                stream
                    .write_all(response.as_bytes())
                    .await
                    .expect("write response");
            }
        });
        let client = HelperClient::new(HelperClientConfig {
            base_url: format!("http://{address}"),
            bearer_token: Some("0123456789abcdef0123456789abcdef".to_owned()),
            request_timeout: Duration::from_secs(1),
            max_attempts: 2,
            retry_delay: Duration::ZERO,
        })
        .expect("client");
        let health = client.healthz().await.expect("readiness after retry");
        assert_eq!(health["status"], "ok");
        server.await.expect("server");
        assert_eq!(attempts.load(Ordering::SeqCst), 2);
    }
}
