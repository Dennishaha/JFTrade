use std::collections::BTreeMap;

use jftrade_marketdata::{ProviderCapabilities, ProviderConstraints, ProviderDescriptor};

pub fn provider_descriptors() -> [ProviderDescriptor; 2] {
    [yfinance_descriptor(), akshare_descriptor()]
}

pub fn yfinance_descriptor() -> ProviderDescriptor {
    ProviderDescriptor {
        selection_id: "yfinance".to_owned(),
        provider_id: "yahoo-finance".to_owned(),
        display_name: "Yahoo Finance (yfinance)".to_owned(),
        broker_id: Some("yfinance".to_owned()),
        source: "yfinance".to_owned(),
        default_market: "US".to_owned(),
        supported_markets: strings(&["US", "HK", "SH", "SZ"]),
        transports: strings(&["http-poll"]),
        capabilities: ProviderCapabilities {
            snapshots: true,
            historical_candles: true,
            instrument_search: true,
            extended_hours: true,
            candle_intervals: candle_intervals(),
            sessions: strings(&["regular", "pre", "after", "closed"]),
            price_adjustments: strings(&["none", "forward"]),
            historical_lookback_days: BTreeMap::from([
                ("1m".to_owned(), 7),
                ("5m".to_owned(), 60),
                ("15m".to_owned(), 60),
                ("30m".to_owned(), 60),
                ("1h".to_owned(), 730),
            ]),
            ..ProviderCapabilities::default()
        },
        constraints: ProviderConstraints::default(),
        notes: strings(&[
            "Quotes may be delayed by 15 minutes under Yahoo Finance's free data access.",
            "US, HK, SH, and SZ snapshots and historical candles are available through delayed HTTP polling.",
            "Order book depth, streaming quotes, and trading are unavailable; Yahoo does not provide a dependable overnight session.",
        ]),
    }
}

pub fn akshare_descriptor() -> ProviderDescriptor {
    ProviderDescriptor {
        selection_id: "akshare".to_owned(),
        provider_id: "akshare".to_owned(),
        display_name: "AKShare".to_owned(),
        broker_id: Some("akshare".to_owned()),
        source: "akshare".to_owned(),
        default_market: "US".to_owned(),
        supported_markets: strings(&["US", "HK", "SH", "SZ"]),
        transports: strings(&["http-poll"]),
        capabilities: ProviderCapabilities {
            snapshots: true,
            historical_candles: true,
            instrument_search: true,
            candle_intervals: candle_intervals(),
            sessions: strings(&["regular", "closed"]),
            price_adjustments: strings(&["none", "forward", "backward"]),
            historical_lookback_days: BTreeMap::from([
                ("1m".to_owned(), 5),
                ("US:5m".to_owned(), 5),
                ("US:15m".to_owned(), 5),
                ("US:30m".to_owned(), 5),
                ("US:1h".to_owned(), 5),
            ]),
            ..ProviderCapabilities::default()
        },
        constraints: ProviderConstraints::default(),
        notes: strings(&[
            "Quotes are best-effort and may be delayed by the upstream public data source.",
            "US, HK, SH, and SZ securities and historical candles are available through HTTP polling.",
            "Streaming quotes, order book depth, extended hours, and trading are unavailable.",
        ]),
    }
}

fn candle_intervals() -> Vec<String> {
    strings(&["1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"])
}

fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| (*value).to_owned()).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn retained_helper_descriptors_are_static_and_valid() {
        let [yfinance, akshare] = provider_descriptors();
        yfinance.validate().expect("yfinance descriptor");
        akshare.validate().expect("AKShare descriptor");
        assert_eq!(yfinance.capabilities.historical_lookback_days["1m"], 7);
        assert_eq!(akshare.capabilities.historical_lookback_days["US:1h"], 5);
        assert!(!akshare.capabilities.extended_hours);
    }
}
