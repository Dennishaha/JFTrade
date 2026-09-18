//! Shared classification of market-data helper runtime sentinel codes.
//!
//! The yfinance and AKShare sidecar clients surface runtime lifecycle pressure
//! as provider-specific HTTP error codes. Both Go providers classify those
//! codes into the market-data lifecycle sentinels so the transport can answer
//! `503 MARKET_DATA_PROVIDER_WARMING` (Retry-After 1) or
//! `503 MARKET_DATA_PROVIDER_BUSY` (Retry-After 2) instead of leaking a
//! provider-specific code to the console. The classification is shared by
//! every helper-backed owner: quotes, news, corporate actions, rankings,
//! industry boards, calendar/macro, company research, news search, and the
//! embedded stock screen.

/// Provider code plus the public failure identity it maps to.
pub(crate) struct HelperRuntimeFailure {
    pub(crate) status: u16,
    pub(crate) code: &'static str,
    pub(crate) message: &'static str,
    pub(crate) retry_after_seconds: u64,
}

/// Normalize one helper remote error into the public failure identity.
///
/// `classify_helper_runtime_code` owns the provider warming/busy sentinel; every
/// other code keeps the helper's own status/message and falls back to
/// `default_code` when the helper did not send one.
pub(crate) fn normalize_helper_remote_error(
    status: u16,
    code: &str,
    message: String,
    retry_after_seconds: Option<u64>,
    default_code: &str,
) -> (u16, String, String, Option<u64>) {
    if let Some(runtime) = classify_helper_runtime_code(code) {
        return (
            runtime.status,
            runtime.code.to_owned(),
            runtime.message.to_owned(),
            Some(runtime.retry_after_seconds),
        );
    }
    let code = if code.is_empty() {
        default_code.to_owned()
    } else {
        code.to_owned()
    };
    (status, code, message, retry_after_seconds)
}

/// Classify a helper error code into the provider warming/busy sentinel.
///
/// Returns `None` for any code that is not a runtime lifecycle sentinel, so
/// callers keep the helper's own status/code/message for everything else.
pub(crate) fn classify_helper_runtime_code(code: &str) -> Option<HelperRuntimeFailure> {
    match code.trim().to_ascii_uppercase().as_str() {
        "YFINANCE_RUNTIME_WARMING" | "AKSHARE_RUNTIME_WARMING" | "PROVIDER_RUNTIME_WARMING" => {
            Some(HelperRuntimeFailure {
                status: 503,
                code: "MARKET_DATA_PROVIDER_WARMING",
                message: "行情服务正在预热，请稍后重试",
                retry_after_seconds: 1,
            })
        }
        "AKSHARE_POOL_BUSY" | "AKSHARE_UPSTREAM_TIMEOUT" => Some(HelperRuntimeFailure {
            status: 503,
            code: "MARKET_DATA_PROVIDER_BUSY",
            message: "行情服务当前繁忙，请稍后重试",
            retry_after_seconds: 2,
        }),
        _ => None,
    }
}
