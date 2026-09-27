use crate::product::product_query::QueryMap;
use jftrade_settings::MarketDataProvider;

use crate::product::MarketDataQuoteReadSnapshotError;

/// Reject an explicit broker selection that this production quote owner cannot
/// serve.  The Go route layer dispatches such requests to a broker reader and
/// returns 409 when that reader is absent; silently using the active
/// helper/OpenD provider would be a legacy fallback with the wrong identity.
pub(super) fn validate_explicit_broker(
    query: &QueryMap,
    provider: MarketDataProvider,
) -> Result<(), MarketDataQuoteReadSnapshotError> {
    let Some(requested) = query
        .get_first("brokerId")
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return Ok(());
    };
    let matches_active = match provider {
        MarketDataProvider::Futu => {
            requested.eq_ignore_ascii_case("futu") || requested.eq_ignore_ascii_case("futu-opend")
        }
        MarketDataProvider::Yfinance => {
            requested.eq_ignore_ascii_case("yfinance")
                || requested.eq_ignore_ascii_case("yahoo-finance")
        }
        MarketDataProvider::Akshare => requested.eq_ignore_ascii_case("akshare"),
    };
    if matches_active {
        return Ok(());
    }
    Err(MarketDataQuoteReadSnapshotError::Failed {
        status: 409,
        code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
        message: format!(
            "market-data capability is unsupported: requested broker \"{requested}\" does not match the active provider"
        ),
        retry_after_seconds: None,
    })
}
