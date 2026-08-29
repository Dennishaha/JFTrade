use std::sync::Arc;

use jftrade_settings::MarketDataProvider;

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use super::{ActiveProviderState, ProductionMarketDataOptionsPort};
use crate::product::{
    MarketDataOptionsReadSnapshotError, MarketDataOptionsReadSnapshotPort,
};

#[derive(Debug)]
struct FixtureOptionChainReader;

impl jftrade_integration_futu::OptionChainReadPort for FixtureOptionChainReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionChainQuery,
    ) -> Result<
        Vec<jftrade_integration_futu::OptionChainDate>,
        jftrade_integration_futu::OptionChainQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.symbol, "AAPL");
        assert_eq!(query.begin_time, "2026-09-01");
        assert_eq!(query.end_time, "2026-09-30");
        assert_eq!(query.option_type, Some(1));
        Ok(vec![jftrade_integration_futu::OptionChainDate {
            strike_time: "2026-09-18".to_owned(),
            strike_timestamp: Some(1_789_000_000.0),
            options: Vec::new(),
        }])
    }
}

#[derive(Debug)]
struct DefaultDateOptionChainReader;

impl jftrade_integration_futu::OptionChainReadPort for DefaultDateOptionChainReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionChainQuery,
    ) -> Result<
        Vec<jftrade_integration_futu::OptionChainDate>,
        jftrade_integration_futu::OptionChainQueryError,
    > {
        let format = time::format_description::parse_borrowed::<2>("[year]-[month]-[day]")
            .map_err(|_| {
                jftrade_integration_futu::OptionChainQueryError::InvalidQuery(
                    "test date format".to_owned(),
                )
            })?;
        let begin = time::Date::parse(&query.begin_time, &format).map_err(|_| {
            jftrade_integration_futu::OptionChainQueryError::InvalidQuery(
                "test begin date".to_owned(),
            )
        })?;
        let end = time::Date::parse(&query.end_time, &format).map_err(|_| {
            jftrade_integration_futu::OptionChainQueryError::InvalidQuery(
                "test end date".to_owned(),
            )
        })?;
        assert_eq!(end - begin, time::Duration::days(30));
        Ok(vec![jftrade_integration_futu::OptionChainDate {
            strike_time: query.end_time.clone(),
            strike_timestamp: None,
            options: Vec::new(),
        }])
    }
}

#[derive(Debug)]
struct FixtureOptionExpirationReader;

impl jftrade_integration_futu::OptionExpirationReadPort for FixtureOptionExpirationReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionExpirationQuery,
    ) -> Result<
        Vec<jftrade_integration_futu::OptionExpirationDate>,
        jftrade_integration_futu::OptionExpirationQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.symbol, "AAPL");
        Ok(vec![jftrade_integration_futu::OptionExpirationDate {
            strike_time: Some("2026-09-18".to_owned()),
            strike_timestamp: Some(1_789_000_000.0),
            expiry_date_distance: 21,
            cycle: Some(1),
        }])
    }
}

#[derive(Debug)]
struct FixtureOptionScreenReader;

impl jftrade_integration_futu::OptionScreenReadPort for FixtureOptionScreenReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionScreenQuery,
    ) -> Result<jftrade_integration_futu::OptionScreenPage, jftrade_integration_futu::OptionScreenQueryError>
    {
        assert_eq!(query.market_categories, vec![0]);
        assert_eq!(query.page_count, Some(50));
        Ok(jftrade_integration_futu::OptionScreenPage {
            last_page: true,
            all_count: 1,
            items: vec![jftrade_integration_futu::OptionScreenItem {
                security: jftrade_integration_futu::OptionScreenSecurity {
                    market: "US".to_owned(),
                    code: "AAPL260918C00100000".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL260918C00100000".to_owned(),
                },
                option_name: Some("AAPL Call".to_owned()),
                strike_price: Some(100.0),
                strike_date: Some(20260918),
                option_type: Some(1),
                exercise_type: None,
                expiration_type: None,
                in_the_money: None,
                left_day: Some(20),
                price: Some(1.25),
                mid_price: None,
                bid_price: None,
                ask_price: None,
                bid_ask_spread: None,
                bid_volume: None,
                ask_volume: None,
                change_rate: None,
                volume: None,
                turnover: None,
                open_interest: None,
                bid_ask_volume_ratio: None,
                open_interest_market_cap: None,
                vol_oi_ratio: None,
                premium: None,
                implied_volatility: Some(0.2),
                delta: Some(0.5),
                gamma: None,
                vega: None,
                theta: None,
                rho: None,
                leverage_ratio: None,
                effective_gearing: None,
                itm_probability: None,
                underlying_info: None,
                history_volatility: None,
                iv_hv_ratio: None,
                buy_to_bep: None,
                sell_to_bep: None,
                buy_profit_probability: None,
                sell_profit_probability: None,
                intrinsic_value_per: None,
                time_value_per: None,
                itm_degree: None,
                otm_degree: None,
                otm_probability: None,
                sell_annualized_return: None,
                interval_return: None,
            }],
        })
    }
}

fn ready_port() -> ProductionMarketDataOptionsPort {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_expirations(Some(Arc::new(FixtureOptionExpirationReader)));
    runtime.set_option_chains(Some(Arc::new(FixtureOptionChainReader)));
    runtime.set_option_screens(Some(Arc::new(FixtureOptionScreenReader)));
    ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    }
}

#[test]
fn chain_projection_forwards_typed_query_and_neutral_wire() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/chains/US.AAPL",
            "market=US&operation=chain&beginTime=2026-09-01&endTime=2026-09-30&type=1",
        )
        .expect("chain response");
    assert_eq!(value["provider"]["featureId"], "derivatives.option_chain");
    assert_eq!(value["entries"][0]["strikeTime"], "2026-09-18");
    assert_eq!(value["entries"][0]["option"].as_array().map(Vec::len), Some(0));
    assert_eq!(value["hasMore"], false);
    assert_eq!(value["total"], 1);
}

#[test]
fn screen_projection_forwards_typed_query_and_neutral_wire() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/screens",
            "market=US&operation=screen&pageSize=50",
        )
        .expect("option screen response");
    assert_eq!(value["provider"]["featureId"], "derivatives.option_screen");
    assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.AAPL260918C00100000");
    assert_eq!(value["entries"][0]["strikeDate"], 20260918);
    assert_eq!(value["hasMore"], false);
    assert_eq!(value["total"], 1);
}

#[test]
fn screen_projection_rejects_bad_query_before_reader() {
    let error = ready_port()
        .read(
            "/api/v1/market-data/options/screens",
            "market=CN&pageSize=50",
        )
        .expect_err("unsupported market");
    assert!(matches!(
        error,
        MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
            if code == "BAD_REQUEST"
    ));
}

#[test]
fn screen_projection_is_unavailable_without_typed_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read("/api/v1/market-data/options/screens", "market=US"),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
}

#[test]
fn chain_projection_defaults_omitted_date_range_for_opend_compatibility() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_chains(Some(Arc::new(DefaultDateOptionChainReader)));
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/market-data/options/chains/US.AAPL", "")
        .expect("default chain date range");
    assert_eq!(value["entries"][0]["option"].as_array().map(Vec::len), Some(0));
}

#[test]
fn chain_projection_rejects_bad_query_before_reader() {
    let error = ready_port()
        .read(
            "/api/v1/market-data/options/chains/US.AAPL",
            "beginTime=2026-09-30&endTime=2026-09-01&type=3",
        )
        .expect_err("invalid chain query");
    assert!(matches!(
        error,
        MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
            if code == "BAD_REQUEST"
    ));
}

#[test]
fn expiration_projection_uses_typed_reader_and_neutral_wire() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/expirations/US.AAPL",
            "market=US&operation=expirations",
        )
        .expect("expiration response");
    assert_eq!(value["provider"]["featureId"], "derivatives.option_chain");
    assert_eq!(value["entries"][0]["strikeTime"], "2026-09-18");
    assert_eq!(value["entries"][0]["optionExpiryDateDistance"], 21);
    assert_eq!(value["hasMore"], false);
    assert_eq!(value["total"], 1);
}

#[test]
fn expiration_projection_rejects_mismatched_market_before_opend() {
    let error = ready_port()
        .read(
            "/api/v1/market-data/options/expirations/US.AAPL",
            "market=HK",
        )
        .expect_err("mismatched market");
    assert!(matches!(
        error,
        MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
            if code == "BAD_REQUEST"
    ));
}

#[test]
fn expiration_projection_is_unavailable_without_typed_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read("/api/v1/market-data/options/expirations/US.AAPL", ""),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
}

#[test]
fn chain_projection_is_unavailable_without_typed_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read(
            "/api/v1/market-data/options/chains/US.AAPL",
            "beginTime=2026-09-01&endTime=2026-09-30",
        ),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
}
