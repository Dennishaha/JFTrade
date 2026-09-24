//! Trade request construction with a configured default market.
//!
//! The constructors here apply the Futu default trade market (`tradeMarket`)
//! when a request omits `market`, keeping the raw query untouched so a blank
//! `market` parameter can never shadow the configured value. Downstream
//! helpers (`market_label`, `securities`, `max_trade_symbol`) observe the
//! default through [`TradeRequest::default_market`].

use super::product_production_ports_trade_requests::TradeRequest;

impl TradeRequest {
    /// Parse a broker request, filling an omitted `market` from the configured
    /// Futu default trade market.
    ///
    /// Parity: `go:452dea11:internal/trading/broker_test.go:735`
    /// `TestNormalizeSymbolsAndRuntimeDefaults` applies `WithDefaultMarket`
    /// when the caller omits the market. An explicit (non-blank) `market`
    /// always wins; a missing or blank configured default keeps the `HK`
    /// fallback.
    pub(crate) fn parse_with_default_market(
        path: &str,
        raw_query: &str,
        default_market: Option<&str>,
    ) -> Result<Self, String> {
        Self::parse_with_prefix_and_default_market(
            path,
            raw_query,
            "/api/v1/brokers/",
            default_market,
        )
    }

    pub(crate) fn parse_with_prefix_and_default_market(
        path: &str,
        raw_query: &str,
        prefix: &str,
        default_market: Option<&str>,
    ) -> Result<Self, String> {
        let mut request = Self::parse_with_prefix(path, raw_query, prefix)?;
        request.default_market = default_market
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned);
        Ok(request)
    }
}
