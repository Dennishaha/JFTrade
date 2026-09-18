//! Request parsing and error mapping for the news / corporate-actions routes.
//!
//! These helpers belong to the same production owner as
//! `ProductionMarketDataNewsPort`; they live in a sibling module so the port
//! bundle stays inside the bounded production-file length.

use crate::product::MarketDataNewsActionsReadSnapshotError;
use crate::product::product_production_ports::product_production_ports_helper_runtime::normalize_helper_remote_error;
use jftrade_integration_marketdata_helper::HttpAdapterError;

pub(super) type NewsActionsHelperRequest = (&'static str, String, String, Vec<(&'static str, String)>);

pub(super) fn news_actions_helper_request(
    path: &str,
    query: &str,
) -> Result<NewsActionsHelperRequest, MarketDataNewsActionsReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/market-data/news/") {
        ("news", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/market-data/corporate-actions/") {
        ("corporate-actions", value)
    } else {
        return Err(news_actions_bad_request("unsupported news/actions path"));
    };
    let mut parts = suffix.split('/');
    let market = parts.next().unwrap_or_default().trim();
    let symbol = parts.next().unwrap_or_default().trim();
    if market.is_empty() || symbol.is_empty() || parts.next().is_some() {
        return Err(news_actions_bad_request("invalid instrument"));
    }

    let query_map = crate::product::product_query::QueryMap::parse(query)
        .map_err(|_| news_actions_bad_request("invalid URL escape"))?;
    let mut query_pairs = Vec::new();
    if operation == "news" {
        if let Some(raw_limit) = query_map.get_first("limit") {
            let limit = raw_limit
                .trim()
                .parse::<u16>()
                .ok()
                .filter(|value| (1..=50).contains(value))
                .ok_or_else(|| news_actions_bad_request("limit must be between 1 and 50"))?;
            query_pairs.push(("limit", limit.to_string()));
        }
    } else {
        let from = parse_corporate_action_time(&query_map, "from")?;
        let to = parse_corporate_action_time(&query_map, "to")?;
        if let (Some((from_at, _)), Some((to_at, _))) = (&from, &to)
            && from_at > to_at
        {
            return Err(news_actions_bad_request("from must not be after to"));
        }
        if let Some((_, value)) = from {
            query_pairs.push(("from", value));
        }
        if let Some((_, value)) = to {
            query_pairs.push(("to", value));
        }
    }
    let (market, symbol) = normalize_news_actions_identity(market, symbol)?;
    Ok((operation, market, symbol, query_pairs))
}

fn parse_corporate_action_time(
    query: &crate::product::product_query::QueryMap,
    key: &'static str,
) -> Result<Option<(time::OffsetDateTime, String)>, MarketDataNewsActionsReadSnapshotError> {
    let Some(raw) = query
        .get_first(key)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(None);
    };
    let parsed =
        time::OffsetDateTime::parse(raw, &time::format_description::well_known::Rfc3339)
            .map_err(|_| news_actions_bad_request(&format!("{key} must be a valid timestamp")))?;
    let normalized = parsed
        .to_offset(time::UtcOffset::UTC)
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|_| news_actions_bad_request(&format!("{key} must be a valid timestamp")))?;
    Ok(Some((parsed, normalized)))
}

fn normalize_news_actions_identity(
    market: &str,
    symbol: &str,
) -> Result<(String, String), MarketDataNewsActionsReadSnapshotError> {
    let mut market = match market.trim().to_ascii_uppercase().as_str() {
        "USA" | "NYSE" | "NASDAQ" | "AMEX" => "US".to_owned(),
        "HKEX" | "HKG" => "HK".to_owned(),
        "CNSH" | "SHH" | "SSE" | "SHSE" => "SH".to_owned(),
        "CNSZ" | "SHZ" | "SZSE" | "SHE" => "SZ".to_owned(),
        value => value.to_owned(),
    };
    let mut symbol = symbol.trim().to_ascii_uppercase();
    if let Some((prefix, code)) = symbol
        .split_once('.')
        .filter(|(prefix, _)| news_actions_market_token(prefix))
    {
        let exchange = match prefix {
            "SH" | "CNSH" | "SHH" | "SSE" | "SHSE" => Some("SH"),
            "SZ" | "CNSZ" | "SHZ" | "SZSE" | "SHE" => Some("SZ"),
            _ => None,
        };
        if market == "CN" {
            if let Some(exchange) = exchange {
                market = exchange.to_owned();
                symbol = code.to_owned();
            } else {
                return Err(news_actions_bad_request("invalid instrument"));
            }
        } else if exchange.is_some_and(|value| value.eq_ignore_ascii_case(&market))
            || prefix.eq_ignore_ascii_case(&market)
        {
            symbol = code.to_owned();
        } else {
            return Err(news_actions_bad_request("invalid instrument"));
        }
    }
    if !matches!(market.as_str(), "US" | "HK" | "SH" | "SZ")
        || symbol.is_empty()
        || symbol.contains('/')
        || symbol.chars().any(char::is_whitespace)
    {
        return Err(news_actions_bad_request("invalid instrument"));
    }
    if market == "HK" && symbol.chars().all(|value| value.is_ascii_digit()) && symbol.len() < 5 {
        symbol = format!("{symbol:0>5}");
    }
    Ok((market, symbol))
}

fn news_actions_market_token(value: &str) -> bool {
    matches!(
        value.to_ascii_uppercase().as_str(),
        "US" | "USA"
            | "NYSE"
            | "NASDAQ"
            | "AMEX"
            | "HK"
            | "HKEX"
            | "HKG"
            | "CN"
            | "SH"
            | "CNSH"
            | "SHH"
            | "SSE"
            | "SHSE"
            | "SZ"
            | "CNSZ"
            | "SHZ"
            | "SZSE"
            | "SHE"
    )
}

pub(super) fn news_actions_bad_request(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

pub(super) fn map_news_actions_helper_error(
    error: HttpAdapterError,
) -> MarketDataNewsActionsReadSnapshotError {
    match error {
        HttpAdapterError::Remote {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let (status, code, message, retry_after_seconds) = normalize_helper_remote_error(
                status,
                &code,
                message,
                retry_after_seconds,
                "BAD_GATEWAY",
            );
            MarketDataNewsActionsReadSnapshotError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            }
        }
        HttpAdapterError::Timeout => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
            retry_after_seconds: None,
        },
        HttpAdapterError::InvalidResponse(message) => {
            MarketDataNewsActionsReadSnapshotError::Failed {
                status: 502,
                code: "BAD_GATEWAY".to_owned(),
                message,
                retry_after_seconds: None,
            }
        }
        HttpAdapterError::Unavailable(message) => {
            MarketDataNewsActionsReadSnapshotError::Unavailable(message)
        }
        other => MarketDataNewsActionsReadSnapshotError::Failed {
            status: 500,
            code: "MARKET_DATA_NEWS_FAILED".to_owned(),
            message: other.to_string(),
            retry_after_seconds: None,
        },
    }
}

pub(super) fn news_actions_capability(message: &str) -> MarketDataNewsActionsReadSnapshotError {
    // The path-style news/corporate-actions routes belong to the market-data
    // transport family, whose owner maps an unsupported capability to
    // MARKET_DATA_CAPABILITY_UNSUPPORTED (frozen news-actions fixture); the
    // product-feature family reports BROKER_CAPABILITY_UNAVAILABLE instead.
    MarketDataNewsActionsReadSnapshotError::Failed {
        status: 409,
        code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
        message: message.to_owned(),
        retry_after_seconds: None,
    }
}

