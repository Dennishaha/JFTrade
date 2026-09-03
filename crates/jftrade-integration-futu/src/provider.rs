use std::collections::BTreeMap;

use jftrade_broker::{
    BrokerMarketCapability, BrokerReadFeatureCapability, BrokerRuntimeDescriptor,
};
use jftrade_marketdata::{ProviderCapabilities, ProviderConstraints, ProviderDescriptor};

pub fn broker_descriptor() -> BrokerRuntimeDescriptor {
    let environments = strings(&["SIMULATE", "REAL"]);
    let real = strings(&["REAL"]);
    let mut read_features = BTreeMap::new();
    read_features.insert("funds".to_owned(), feature(environments.clone()));
    read_features.insert("positions".to_owned(), feature(environments.clone()));
    read_features.insert(
        "orders".to_owned(),
        BrokerReadFeatureCapability {
            supports_history: true,
            ..feature(environments.clone())
        },
    );
    read_features.insert(
        "fills".to_owned(),
        BrokerReadFeatureCapability {
            supports_history: true,
            ..feature(environments.clone())
        },
    );
    read_features.insert(
        "cashFlows".to_owned(),
        BrokerReadFeatureCapability {
            requires_clearing_date: true,
            ..feature(real.clone())
        },
    );
    read_features.insert(
        "orderFees".to_owned(),
        BrokerReadFeatureCapability {
            requires_order_id_ex: true,
            ..feature(real.clone())
        },
    );
    read_features.insert(
        "marginRatios".to_owned(),
        BrokerReadFeatureCapability {
            requires_symbols: true,
            ..feature(real)
        },
    );
    read_features.insert(
        "maxTradeQuantity".to_owned(),
        BrokerReadFeatureCapability {
            requires_price: true,
            ..feature(environments.clone())
        },
    );
    read_features.insert(
        "orderBook".to_owned(),
        BrokerReadFeatureCapability {
            default_num: 10,
            min_num: 1,
            max_num: 50,
            num_presets: vec![5, 10, 20, 50],
            supports_real_time_push: true,
            ..feature(environments.clone())
        },
    );
    BrokerRuntimeDescriptor {
        id: "futu".to_owned(),
        display_name: "Futu".to_owned(),
        environments,
        capabilities: vec![BrokerMarketCapability {
            market: "HK".to_owned(),
            supports_quote: true,
            supports_trade: true,
            read_features,
        }],
        notes: strings(&[
            "Market data is exposed to the frontend through the bbgo exchange boundary.",
            "OpenD WebSocket settings are retained for compatibility and diagnostics; the current hot path uses the native API port.",
        ]),
    }
}

fn feature(supported_environments: Vec<String>) -> BrokerReadFeatureCapability {
    BrokerReadFeatureCapability {
        supported_environments,
        ..BrokerReadFeatureCapability::default()
    }
}

pub fn provider_descriptor() -> ProviderDescriptor {
    ProviderDescriptor {
        selection_id: "futu".to_owned(),
        provider_id: "futu-opend".to_owned(),
        display_name: "Futu OpenD".to_owned(),
        broker_id: Some("futu".to_owned()),
        source: "bbgo:futu".to_owned(),
        default_market: "HK".to_owned(),
        supported_markets: strings(&["HK", "US", "CN", "SH", "SZ"]),
        transports: strings(&["opend-tcp", "push-stream", "snapshot-poll-fallback"]),
        capabilities: ProviderCapabilities {
            snapshots: true,
            streaming_quotes: true,
            streaming_candles: true,
            streaming_depth: true,
            historical_candles: true,
            tick_candles: true,
            order_book_depth: true,
            instrument_search: true,
            extended_hours: true,
            candle_intervals: strings(&["tick", "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"]),
            order_book_levels: vec![1, 5, 10, 25, 50],
            sessions: strings(&["RTH", "ETH", "ALL", "OVERNIGHT"]),
            price_adjustments: strings(&["none", "forward", "backward"]),
            ..ProviderCapabilities::default()
        },
        constraints: ProviderConstraints {
            requires_open_d: true,
            requires_market_data_right: true,
            uses_subscription_quota: true,
        },
        notes: strings(&[
            "Futu-first provider; data entitlement and subscription quota are enforced by Futu OpenD.",
            "Historical candles and real-time pushes can diverge during extended sessions; UI surfaces observed timestamps and transport mode.",
        ]),
    }
}

fn strings(values: &[&str]) -> Vec<String> {
    values.iter().map(|value| (*value).to_owned()).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn futu_descriptor_is_static_and_valid_without_connecting_to_opend() {
        let descriptor = provider_descriptor();
        descriptor.validate().expect("Futu descriptor");
        assert_eq!(descriptor.supported_markets, ["HK", "US", "CN", "SH", "SZ"]);
        assert!(descriptor.constraints.requires_open_d);
        assert!(descriptor.capabilities.streaming_quotes);
    }

    #[test]
    fn broker_descriptor_matches_current_go_wire_fixture() {
        let expected: serde_json::Value = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/broker-descriptor.json"
        ))
        .expect("broker descriptor fixture");
        assert_eq!(
            serde_json::to_value(broker_descriptor()).expect("broker descriptor"),
            expected
        );
    }
}
