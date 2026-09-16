//! Subscription-lease and provider-capability gates shared by the production
//! market-data quote read paths.

use super::*;

impl ProductionMarketDataQuotePort {
    pub(super) fn require_order_book_subscription(
        &self,
        instrument_id: &str,
    ) -> Result<(), MarketDataQuoteReadSnapshotError> {
        let demand = self.subscription_demand()?;
        if demand.entries.iter().any(|entry| {
            entry.channel.eq_ignore_ascii_case("ORDER_BOOK")
                && entry.instrument_id.eq_ignore_ascii_case(instrument_id)
        }) {
            return Ok(());
        }
        Err(subscription_required_error("ORDER_BOOK", instrument_id, None))
    }

    /// Go's `requireBasicSubscriptionDemand` accepts any SNAPSHOT/TICK/KLINE
    /// logical lease for the instrument.  Rust applies the same gate to live
    /// Futu reads whenever the router (the reconciler's owner) is composed;
    /// poll-only providers have no broker-side lease to consume.
    pub(super) fn require_basic_subscription_lease(
        &self,
        instrument_id: &str,
        read_channel: &str,
        interval: Option<&str>,
    ) -> Result<(), MarketDataQuoteReadSnapshotError> {
        let demand = self.subscription_demand()?;
        if demand.entries.iter().any(|entry| {
            entry.instrument_id.eq_ignore_ascii_case(instrument_id)
                && matches!(
                    entry.channel.to_ascii_uppercase().as_str(),
                    "SNAPSHOT" | "TICK" | "KLINE"
                )
        }) {
            return Ok(());
        }
        Err(subscription_required_error(
            read_channel,
            instrument_id,
            interval,
        ))
    }

    fn subscription_demand(
        &self,
    ) -> Result<jftrade_marketdata::DemandSnapshot, MarketDataQuoteReadSnapshotError> {
        let Some(router) = self.router.as_ref() else {
            return Err(MarketDataQuoteReadSnapshotError::Unavailable(
                "market-data provider router is not configured".to_owned(),
            ));
        };
        router
            .lock()
            .map_err(|error| MarketDataQuoteReadSnapshotError::Failed {
                status: 500,
                code: "MARKET_DATA_SUBSCRIPTION_FAILED".to_owned(),
                message: format!("failed to lock market-data provider router: {error}"),
                retry_after_seconds: None,
            })
            .map(|router| router.demand())
    }
}

/// Mirrors Go's `NewSubscriptionRequiredError` message shape so the transport
/// envelope stays actionable for the client.
pub(super) fn subscription_required_error(
    channel: &str,
    instrument_id: &str,
    interval: Option<&str>,
) -> MarketDataQuoteReadSnapshotError {
    let message = match interval.filter(|value| !value.trim().is_empty()) {
        Some(interval) => format!(
            "market-data subscription required: acquire a {channel} lease for {instrument_id}:{interval} before reading live data"
        ),
        None => format!(
            "market-data subscription required: acquire a {channel} lease for {instrument_id} before reading live data"
        ),
    };
    MarketDataQuoteReadSnapshotError::Failed {
        status: 409,
        code: "MARKET_DATA_SUBSCRIPTION_REQUIRED".to_owned(),
        message,
        retry_after_seconds: None,
    }
}

pub(super) fn capability_unsupported_error(
    provider: MarketDataProvider,
    capability: &str,
) -> MarketDataQuoteReadSnapshotError {
    let provider_id = match provider {
        MarketDataProvider::Futu => "futu-opend",
        MarketDataProvider::Yfinance => "yahoo-finance",
        MarketDataProvider::Akshare => "akshare",
    };
    MarketDataQuoteReadSnapshotError::Failed {
        status: 409,
        code: "MARKET_DATA_CAPABILITY_UNSUPPORTED".to_owned(),
        message: format!(
            "market-data capability is unsupported: active provider \"{provider_id}\" does not support {capability}"
        ),
        retry_after_seconds: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Parity: go:8a78fc78:pkg/futu/subscription_lifecycle_test.go:215
    /// TestSubscriptionRequiredErrorFormattingAndNilConnectionGeneration.
    ///
    /// Go's factory renders the channel, instrument and interval into the
    /// public error so the transport can surface the exact missing lease. The
    /// Rust formatter is shared by candle/depth/snapshot gates and must keep
    /// the same actionable shape instead of leaking a provider error string.
    #[test]
    fn subscription_required_error_formats_channel_instrument_and_interval() {
        let message = |error: super::MarketDataQuoteReadSnapshotError| match error {
            super::MarketDataQuoteReadSnapshotError::Failed { message, .. } => message,
            other => panic!("unexpected error: {other:?}"),
        };

        assert_eq!(
            message(subscription_required_error("KLINE", "US.AAPL", Some("1m"))),
            "market-data subscription required: acquire a KLINE lease for US.AAPL:1m before reading live data"
        );

        // A blank interval must not render a trailing colon, matching Go's
        // `normalizeSubscriptionInterval` + empty-detail branch.
        assert_eq!(
            message(subscription_required_error(" basic ", "US.AAPL", Some("   "))),
            "market-data subscription required: acquire a  basic  lease for US.AAPL before reading live data"
        );

        assert_eq!(
            message(subscription_required_error("ORDER_BOOK", "", None)),
            "market-data subscription required: acquire a ORDER_BOOK lease for  before reading live data"
        );
    }
}
