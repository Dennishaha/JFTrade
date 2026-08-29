use std::sync::Arc;

use jftrade_settings::MarketDataProvider;

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use super::{ActiveProviderState, ProductionMarketDataOptionsPort};
use crate::product::{MarketDataOptionsReadSnapshotError, MarketDataOptionsReadSnapshotPort};

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
    ) -> Result<
        jftrade_integration_futu::OptionScreenPage,
        jftrade_integration_futu::OptionScreenQueryError,
    > {
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

#[derive(Debug)]
struct FixtureOptionQuoteReader;

impl jftrade_integration_futu::OptionQuoteReadPort for FixtureOptionQuoteReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionQuoteQuery,
    ) -> Result<
        Vec<jftrade_integration_futu::OptionQuote>,
        jftrade_integration_futu::OptionQuoteQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL260918C00100000");
        Ok(vec![jftrade_integration_futu::OptionQuote {
            security: jftrade_integration_futu::OptionQuoteSecurity {
                market: "US".to_owned(),
                code: query.code.clone(),
                quote_market: "US".to_owned(),
                trade_market: "US".to_owned(),
                instrument_id: format!("US.{}", query.code),
            },
            price: Some(1.25),
            implied_volatility: Some(0.2),
            option_type: Some(1),
            expire_time: Some("2026-09-18".to_owned()),
            ..empty_option_quote()
        }])
    }
}

#[derive(Debug)]
struct FixtureOptionVolatilityReader;

impl jftrade_integration_futu::OptionVolatilityReadPort for FixtureOptionVolatilityReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionVolatilityQuery,
    ) -> Result<
        jftrade_integration_futu::OptionVolatilitySnapshot,
        jftrade_integration_futu::OptionVolatilityQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.query_time_period, Some(2));
        assert_eq!(query.hv_time_period, Some(30));
        Ok(jftrade_integration_futu::OptionVolatilitySnapshot {
            security: jftrade_integration_futu::OptionVolatilitySecurity {
                market: "US".to_owned(),
                code: "AAPL".to_owned(),
                quote_market: "US".to_owned(),
                trade_market: "US".to_owned(),
                instrument_id: "US.AAPL".to_owned(),
            },
            items: vec![jftrade_integration_futu::OptionVolatilityItem {
                timestamp: Some(1_756_000_000),
                timestamp_str: Some("2026-08-29".to_owned()),
                implied_volatility: Some(25.0),
                history_volatility: Some(20.0),
                volatility_premium: Some(5.0),
            }],
            average_impvol: Some(25.0),
            impvol_status: Some("ImpvolOvervalued".to_owned()),
            analysis: Some("elevated".to_owned()),
        })
    }
}

#[derive(Debug)]
struct FixtureOptionExerciseProbabilityReader;

impl jftrade_integration_futu::OptionExerciseProbabilityReadPort
    for FixtureOptionExerciseProbabilityReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionExerciseProbabilityQuery,
    ) -> Result<
        jftrade_integration_futu::OptionExerciseProbabilitySnapshot,
        jftrade_integration_futu::OptionExerciseProbabilityQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL260918C00100000");
        Ok(
            jftrade_integration_futu::OptionExerciseProbabilitySnapshot {
                security: jftrade_integration_futu::OptionExerciseProbabilitySecurity {
                    market: "US".to_owned(),
                    code: query.code.clone(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: format!("US.{}", query.code),
                },
                items: vec![jftrade_integration_futu::OptionExerciseProbabilityItem {
                    timestamp: Some(1_756_000_000),
                    timestamp_str: Some("2026-08-29".to_owned()),
                    security_price: Some(225.0),
                    strike_probability: Some(41.869),
                }],
            },
        )
    }
}

#[derive(Debug)]
struct FixtureOptionUnderlyingOverviewReader;

impl jftrade_integration_futu::OptionUnderlyingOverviewReadPort
    for FixtureOptionUnderlyingOverviewReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionUnderlyingOverviewQuery,
    ) -> Result<
        jftrade_integration_futu::OptionUnderlyingOverviewSnapshot,
        jftrade_integration_futu::OptionUnderlyingOverviewQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.index_option_type, Some(1));
        Ok(jftrade_integration_futu::OptionUnderlyingOverviewSnapshot {
            items: vec![jftrade_integration_futu::OptionUnderlyingOverviewItem {
                security: jftrade_integration_futu::OptionUnderlyingOverviewSecurity {
                    market: "US".to_owned(),
                    code: "AAPL".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL".to_owned(),
                },
                code: Some("AAPL".to_owned()),
                name: Some("Apple".to_owned()),
                call_volume: Some(120),
                put_volume: Some(80),
                call_open_interest: Some(900),
                put_open_interest: Some(700),
                iv: Some(25.0),
                iv_rank: Some(60.0),
                iv_percentile: Some(55.0),
                hv_list: vec![jftrade_integration_futu::OptionUnderlyingHvItem {
                    time_range: 1,
                    hv: 20.0,
                    hv_percentile: Some(45.0),
                }],
                pre_iv: Some(24.0),
            }],
        })
    }
}

#[derive(Debug)]
struct FixtureOptionUnderlyingRankReader;

impl jftrade_integration_futu::OptionUnderlyingRankReadPort for FixtureOptionUnderlyingRankReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionUnderlyingRankQuery,
    ) -> Result<
        jftrade_integration_futu::OptionUnderlyingRankSnapshot,
        jftrade_integration_futu::OptionUnderlyingRankQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.sort_type, 7);
        assert_eq!(query.is_asc, Some(true));
        assert_eq!(query.count, Some(25));
        assert_eq!(query.trading_date.as_deref(), Some("2026-08-29"));
        assert_eq!(query.page.as_deref(), Some("next"));
        Ok(jftrade_integration_futu::OptionUnderlyingRankSnapshot {
            market: "US".to_owned(),
            sort_type: 7,
            trading_date: Some("2026-08-29".to_owned()),
            trading_timestamp: Some(1_756_000_000.0),
            items: vec![jftrade_integration_futu::OptionUnderlyingRankItem {
                security: jftrade_integration_futu::OptionUnderlyingRankSecurity {
                    market: "US".to_owned(),
                    code: "AAPL".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL".to_owned(),
                },
                name: Some("Apple".to_owned()),
                total_volume: Some(1200),
                total_open_interest: Some(900),
                volume_ratio: Some(80.0),
                open_interest_ratio: None,
                iv: Some(25.0),
                iv_rank: Some(60.0),
                iv_percentile: None,
                price: Some(225.0),
                change_rate: Some(1.2),
                iv_change: None,
                hv: Some(20.0),
                hv_change: None,
                market_cap: Some(3_000_000_000_000.0),
            }],
            next_page: Some("next-2".to_owned()),
            all_count: Some(42),
        })
    }
}

#[derive(Debug)]
struct FixtureOptionUnderlyingHisVolatilityReader;

impl jftrade_integration_futu::OptionUnderlyingHisVolatilityReadPort
    for FixtureOptionUnderlyingHisVolatilityReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionUnderlyingHisVolatilityQuery,
    ) -> Result<
        jftrade_integration_futu::OptionUnderlyingHisVolatilitySnapshot,
        jftrade_integration_futu::OptionUnderlyingHisVolatilityQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.index_option_type, Some(1));
        assert_eq!(query.begin_time, "2025-08-29");
        assert_eq!(query.end_time, "2026-08-29");
        assert_eq!(query.next_page_key, vec![1, 2, 3]);
        Ok(
            jftrade_integration_futu::OptionUnderlyingHisVolatilitySnapshot {
                security: jftrade_integration_futu::OptionUnderlyingHisVolatilitySecurity {
                    market: "US".to_owned(),
                    code: "AAPL".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL".to_owned(),
                },
                code: Some("AAPL".to_owned()),
                name: Some("Apple".to_owned()),
                items: vec![
                    jftrade_integration_futu::OptionUnderlyingHisVolatilityItem {
                        time: "2026-08-29".to_owned(),
                        timestamp: Some(1_756_000_000.0),
                        iv: Some(25.0),
                        hv: Some(20.0),
                        underlying_price: Some(225.0),
                    },
                ],
                next_page_key: vec![4, 5, 6],
            },
        )
    }
}

#[derive(Debug)]
struct FixtureOptionMarketStatisticReader;

impl jftrade_integration_futu::OptionMarketStatisticReadPort
    for FixtureOptionMarketStatisticReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionMarketStatisticQuery,
    ) -> Result<
        jftrade_integration_futu::OptionMarketStatisticSnapshot,
        jftrade_integration_futu::OptionMarketStatisticQueryError,
    > {
        assert_eq!(query.option_market, 1);
        assert_eq!(query.data_type, 0);
        assert_eq!(query.begin_time, "2026-08-01");
        assert_eq!(query.end_time, "2026-08-29");
        assert_eq!(query.next_page_key, vec![1, 2, 3]);
        Ok(jftrade_integration_futu::OptionMarketStatisticSnapshot {
            option_market: 1,
            market: "US".to_owned(),
            data_type: 0,
            items: vec![jftrade_integration_futu::OptionMarketStatisticItem {
                time: "2026-08-29".to_owned(),
                timestamp: Some(1_756_000_000.0),
                call_value: 100,
                put_value: 80,
                total_value: Some(180),
                ratio: Some(0.8),
            }],
            next_page_key: vec![4, 5, 6],
        })
    }
}

#[derive(Debug)]
struct FixtureOptionUnderlyingHisStatisticReader;

impl jftrade_integration_futu::OptionUnderlyingHisStatisticReadPort
    for FixtureOptionUnderlyingHisStatisticReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionUnderlyingHisStatisticQuery,
    ) -> Result<
        jftrade_integration_futu::OptionUnderlyingHisStatisticSnapshot,
        jftrade_integration_futu::OptionUnderlyingHisStatisticQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.index_option_type, Some(1));
        assert_eq!(query.begin_time, "2025-08-29");
        assert_eq!(query.end_time, "2026-08-29");
        assert_eq!(query.next_page_key, vec![1, 2, 3]);
        Ok(
            jftrade_integration_futu::OptionUnderlyingHisStatisticSnapshot {
                security: jftrade_integration_futu::OptionUnderlyingHisStatisticSecurity {
                    market: "US".to_owned(),
                    code: "AAPL".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL".to_owned(),
                },
                code: Some("AAPL".to_owned()),
                name: Some("Apple".to_owned()),
                items: vec![jftrade_integration_futu::OptionUnderlyingHisStatisticItem {
                    time: "2026-08-29".to_owned(),
                    timestamp: Some(1_754_000_000.0),
                    option_volume: Some(180),
                    call_volume: 100,
                    put_volume: 80,
                    put_call_volume_ratio: Some(0.8),
                    option_open_interest: Some(1_600),
                    call_open_interest: 900,
                    put_open_interest: 700,
                    put_call_open_interest_ratio: Some(0.777),
                    underlying_price: Some(225.0),
                }],
                next_page_key: vec![4, 5, 6],
            },
        )
    }
}

#[derive(Debug)]
struct FixtureOptionStrategySpreadReader;

impl jftrade_integration_futu::OptionStrategySpreadReadPort for FixtureOptionStrategySpreadReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionStrategySpreadQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategySpreadSnapshot,
        jftrade_integration_futu::OptionStrategySpreadQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.option_strategy, 4);
        assert_eq!(query.expire_time, "2026-09-18");
        assert_eq!(query.far_expire_time, None);
        assert_eq!(query.index_option_type, Some(1));
        Ok(jftrade_integration_futu::OptionStrategySpreadSnapshot {
            items: vec![
                jftrade_integration_futu::OptionStrategySpreadItem { spread: 10.0 },
                jftrade_integration_futu::OptionStrategySpreadItem { spread: 20.0 },
            ],
        })
    }
}

#[derive(Debug)]
struct FixtureOptionStrategyReader;

impl jftrade_integration_futu::OptionStrategyReadPort for FixtureOptionStrategyReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionStrategyQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategySnapshot,
        jftrade_integration_futu::OptionStrategyQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.code, "AAPL");
        assert_eq!(query.option_strategy, 4);
        assert_eq!(query.expire_time.as_deref(), Some("2026-09-18"));
        assert_eq!(query.far_expire_time, None);
        assert_eq!(query.spread, Some(10.0));
        assert_eq!(query.option_type, Some(1));
        assert_eq!(query.strike_price, Some(100.0));
        assert_eq!(query.index_option_type, Some(1));
        let security = jftrade_integration_futu::OptionStrategySecurity {
            market: "US".to_owned(),
            code: "AAPL".to_owned(),
            quote_market: "US".to_owned(),
            trade_market: "US".to_owned(),
            instrument_id: "US.AAPL".to_owned(),
        };
        Ok(jftrade_integration_futu::OptionStrategySnapshot {
            items: vec![jftrade_integration_futu::OptionStrategyItem {
                code: "AAPL260918C/P100".to_owned(),
                name: "AAPL vertical".to_owned(),
                option_strategy: 4,
                stock_owner: security.clone(),
                multi_legs: vec![jftrade_integration_futu::OptionStrategyLeg {
                    security,
                    side: Some(1),
                    qty_ratio: Some(1.0),
                    position_id: None,
                    pred_side: None,
                }],
            }],
        })
    }
}

#[derive(Debug)]
struct FixtureOptionStrategyAnalysisReader;

impl jftrade_integration_futu::OptionStrategyAnalysisReadPort
    for FixtureOptionStrategyAnalysisReader
{
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionStrategyAnalysisQuery,
    ) -> Result<
        jftrade_integration_futu::OptionStrategyAnalysisSnapshot,
        jftrade_integration_futu::OptionStrategyAnalysisQueryError,
    > {
        assert_eq!(query.multi_legs.len(), 2);
        assert_eq!(
            query.multi_legs[0].security.instrument_id,
            "US.AAPL260918C00100000"
        );
        assert_eq!(query.multi_legs[1].side, Some(2));
        Ok(jftrade_integration_futu::OptionStrategyAnalysisSnapshot {
            code: "AAPL260918C/P100".to_owned(),
            name: "AAPL vertical".to_owned(),
            option_strategy: 4,
            bid1: Some(1.0),
            ask1: Some(2.0),
            max_profit: Some(100.0),
            max_loss: Some(-100.0),
            breakeven_points: vec![100.0],
            prob_of_profit: Some(0.5),
            delta: Some(0.1),
            theta: Some(-0.2),
        })
    }
}

#[derive(Debug)]
struct FixtureOptionContractRankReader;

impl jftrade_integration_futu::OptionContractRankReadPort for FixtureOptionContractRankReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionContractRankQuery,
    ) -> Result<
        jftrade_integration_futu::OptionContractRankSnapshot,
        jftrade_integration_futu::OptionContractRankQueryError,
    > {
        assert_eq!(query.market, 11);
        assert_eq!(query.sort_type, 10);
        assert_eq!(query.count, Some(25));
        assert_eq!(query.trading_date.as_deref(), Some("2026-08-29"));
        assert_eq!(query.is_asc, Some(true));
        assert_eq!(query.page.as_deref(), Some("next"));
        Ok(jftrade_integration_futu::OptionContractRankSnapshot {
            market: "US".to_owned(),
            sort_type: 10,
            trading_date: Some("2026-08-29".to_owned()),
            trading_timestamp: Some(1_756_000_000.0),
            items: vec![jftrade_integration_futu::OptionContractRankItem {
                security: jftrade_integration_futu::OptionContractRankSecurity {
                    market: "US".to_owned(),
                    code: "AAPL260918C00100000".to_owned(),
                    quote_market: "US".to_owned(),
                    trade_market: "US".to_owned(),
                    instrument_id: "US.AAPL260918C00100000".to_owned(),
                },
                name: Some("AAPL Call".to_owned()),
                option_type: Some(1),
                oi_increment: Some(20),
                oi_decrement: None,
                oi_market_cap_increment: None,
                oi_market_cap_decrement: None,
                volume: Some(1200),
                turnover: Some(1500.0),
                open_interest: Some(900),
                open_interest_market_cap: None,
                iv: Some(25.0),
                option_price: Some(1.25),
                change_rate: Some(5.0),
                mid_price: Some(1.24),
                bid_price: Some(1.2),
                bid_volume: Some(10),
                ask_price: Some(1.3),
                ask_volume: Some(12),
                delta: Some(0.5),
                gamma: Some(0.01),
                theta: Some(-0.1),
                vega: Some(0.2),
                rho: Some(0.05),
            }],
            next_page: Some("next-2".to_owned()),
            all_count: Some(42),
        })
    }
}

#[derive(Debug)]
struct FixtureOptionEventReader {
    result: Result<jftrade_integration_futu::OptionEventPage, String>,
}

#[derive(Debug)]
struct FixtureZeroDteScreenerReader;

impl jftrade_integration_futu::OptionZeroDteScreenerReadPort for FixtureZeroDteScreenerReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionZeroDteScreenerQuery,
    ) -> Result<jftrade_integration_futu::OptionZeroDteScreenerPage, jftrade_integration_futu::OptionZeroDteScreenerQueryError> {
        assert_eq!(query.option_market, 1);
        assert_eq!(query.count, 50);
        assert_eq!(query.filters.len(), 1);
        let owner = jftrade_integration_futu::OptionEventSecurity { market: "US".into(), code: "AAPL".into(), quote_market: "US".into(), trade_market: "US".into(), instrument_id: "US.AAPL".into() };
        Ok(jftrade_integration_futu::OptionZeroDteScreenerPage { items: vec![jftrade_integration_futu::OptionZeroDteScreenerItem { owner, name: Some("Apple".into()), price: Some(100.0), change_rate: None, market_cap: None, iv: None, iv_rank: None, iv_percentile: None, hv: None, volume: Some(10), open_interest: None, last_trading_time: None, earnings_timestamp: None, earnings_time: None, earnings_pub_type: None, chain_info: Some(jftrade_integration_futu::OptionZeroDteChainInfo { strike_date_timestamp: Some(1), product_code: Some("AAPL".into()), multiplier: Some(100.0), contract_share_size: Some(100.0), expiration_type: Some(2), underlying: None }) }], next_page: Some("zero-next".into()), update_timestamp: Some(2.0) })
    }
}

#[derive(Debug)]
struct FixtureEarningsScreenerReader;

impl jftrade_integration_futu::OptionEarningsScreenerReadPort for FixtureEarningsScreenerReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionEarningsScreenerQuery,
    ) -> Result<jftrade_integration_futu::OptionEarningsScreenerPage, jftrade_integration_futu::OptionEarningsScreenerQueryError> {
        assert_eq!(query.option_market, 1);
        assert_eq!(query.count, 50);
        assert_eq!(query.filters.len(), 1);
        let owner = jftrade_integration_futu::OptionEventSecurity { market: "US".into(), code: "AAPL".into(), quote_market: "US".into(), trade_market: "US".into(), instrument_id: "US.AAPL".into() };
        Ok(jftrade_integration_futu::OptionEarningsScreenerPage { items: vec![jftrade_integration_futu::OptionEarningsScreenerItem { owner, name: Some("Apple".into()), price: Some(100.0), change_rate: None, market_cap: None, iv: Some(20.0), iv_rank: None, iv_percentile: None, hv: None, volume: Some(10), open_interest: None, earnings_timestamp: Some(1.0), earnings_time: Some("2026-09-01".into()), earnings_pub_type: Some(1), earnings_quarter: Some("2026Q3".into()), last_report_iv_crush: None, history_report_iv_crush: None, last_report_chg_rate: None, history_report_chg_rate: None, estimate_eps_yoy: None, estimate_revenue_yoy: None, expected_move_ratio: Some(3.0) }], next_page: None, update_timestamp: Some(2.0), all_count: Some(1) })
    }
}

impl jftrade_integration_futu::OptionEventReadPort for FixtureOptionEventReader {
    fn query(
        &self,
        query: &jftrade_integration_futu::OptionEventQuery,
    ) -> Result<
        jftrade_integration_futu::OptionEventPage,
        jftrade_integration_futu::OptionEventQueryError,
    > {
        if query.owner.is_some() {
            assert_eq!(query.market, 1);
            assert_eq!(query.underlying_product_class, Some(1));
            assert_eq!(
                query
                    .owner
                    .as_ref()
                    .map(|owner| owner.instrument_id.as_str()),
                Some("US.AAPL")
            );
            assert_eq!(query.count, 25);
            assert_eq!(query.page.as_deref(), Some("next"));
            assert_eq!(query.sort.map(|sort| sort.indicator_type), Some(302));
        }
        self.result.clone().map_err(|message| {
            jftrade_integration_futu::OptionEventQueryError::Rejected {
                ret_type: 1,
                err_code: 429,
                message,
            }
        })
    }
}

fn sample_option_event() -> jftrade_integration_futu::OptionEvent {
    let security = jftrade_integration_futu::OptionEventSecurity {
        market: "US".to_owned(),
        code: "AAPL260918C00100000".to_owned(),
        quote_market: "US".to_owned(),
        trade_market: "US".to_owned(),
        instrument_id: "US.AAPL260918C00100000".to_owned(),
    };
    let owner = jftrade_integration_futu::OptionEventSecurity {
        market: "US".to_owned(),
        code: "AAPL".to_owned(),
        quote_market: "US".to_owned(),
        trade_market: "US".to_owned(),
        instrument_id: "US.AAPL".to_owned(),
    };
    jftrade_integration_futu::OptionEvent {
        option: security,
        owner,
        symbol: Some("AAPL".to_owned()),
        fill_time: Some("2026-08-29 14:30:00".to_owned()),
        fill_timestamp: Some(1_756_000_000.0),
        ticker_type: Some(1),
        price: Some(1.25),
        volume: Some(100),
        turnover: Some(125.0),
        option_type: Some(1),
        strike_price: Some(100.0),
        strike_time: Some("2026-09-18".to_owned()),
        strike_timestamp: None,
        dte: Some(20),
        underlying_price: Some(225.0),
        otm: Some(0.1),
        bid_price: Some(1.2),
        ask_price: Some(1.3),
        iv: Some(0.2),
        total_volume: Some(1000),
        total_open_interest: Some(500),
        vo_ratio: Some(2.0),
        delta: Some(0.5),
        gamma: Some(0.01),
        vega: Some(0.2),
        theta: Some(-0.1),
        rho: Some(0.05),
        sentiment: Some(1),
        order_type_list: vec![1],
        strategy_type: Some(1),
        earnings_time: Some("2026-10-01".to_owned()),
        earnings_pub_type: Some(1),
        corporate_action_list: Vec::new(),
        industry_plate_list: Vec::new(),
        concept_plate_list: Vec::new(),
    }
}

fn empty_option_quote() -> jftrade_integration_futu::OptionQuote {
    jftrade_integration_futu::OptionQuote {
        security: jftrade_integration_futu::OptionQuoteSecurity {
            market: "US".to_owned(),
            code: "AAPL260918C00100000".to_owned(),
            quote_market: "US".to_owned(),
            trade_market: "US".to_owned(),
            instrument_id: "US.AAPL260918C00100000".to_owned(),
        },
        price: None,
        chg: None,
        chg_rate: None,
        vol: None,
        turnover: None,
        high: None,
        low: None,
        mid: None,
        open: None,
        pre_close: None,
        open_interest: None,
        premium: None,
        implied_volatility: None,
        delta: None,
        gamma: None,
        vega: None,
        theta: None,
        rho: None,
        option_type: None,
        expire_time: None,
        strike: None,
        contract_size: None,
        contract_multiplier: None,
        exercise_type: None,
        days_to_expiry: None,
        net_open_interest: None,
        contract_value: None,
        equal_underlying: None,
        index_option_type: None,
        intrinsic_value: None,
        time_value: None,
        breakeven_point: None,
        dist_to_breakeven: None,
        prob_of_profit: None,
        seller_roi: None,
        mark_price: None,
        leverage_ratio: None,
        effective_gearing: None,
    }
}

fn ready_port() -> ProductionMarketDataOptionsPort {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_expirations(Some(Arc::new(FixtureOptionExpirationReader)));
    runtime.set_option_chains(Some(Arc::new(FixtureOptionChainReader)));
    runtime.set_option_screens(Some(Arc::new(FixtureOptionScreenReader)));
    runtime.set_option_quotes(Some(Arc::new(FixtureOptionQuoteReader)));
    runtime.set_option_volatility(Some(Arc::new(FixtureOptionVolatilityReader)));
    runtime.set_option_exercise_probability(Some(Arc::new(FixtureOptionExerciseProbabilityReader)));
    runtime.set_option_underlying_overview(Some(Arc::new(FixtureOptionUnderlyingOverviewReader)));
    runtime.set_option_underlying_his_volatility(Some(Arc::new(
        FixtureOptionUnderlyingHisVolatilityReader,
    )));
    runtime.set_option_market_statistic(Some(Arc::new(FixtureOptionMarketStatisticReader)));
    runtime.set_option_underlying_his_statistic(Some(Arc::new(
        FixtureOptionUnderlyingHisStatisticReader,
    )));
    runtime.set_option_strategy_spread(Some(Arc::new(FixtureOptionStrategySpreadReader)));
    runtime.set_option_strategy(Some(Arc::new(FixtureOptionStrategyReader)));
    runtime.set_option_strategy_analysis(Some(Arc::new(FixtureOptionStrategyAnalysisReader)));
    runtime.set_option_underlying_rank(Some(Arc::new(FixtureOptionUnderlyingRankReader)));
    runtime.set_option_contract_rank(Some(Arc::new(FixtureOptionContractRankReader)));
    runtime.set_option_events(Some(Arc::new(FixtureOptionEventReader {
        result: Ok(jftrade_integration_futu::OptionEventPage {
            events: vec![sample_option_event()],
            next_page: Some("next-2".to_owned()),
            all_count: Some(2),
            update_timestamp: Some(1_756_000_001.0),
        }),
    })));
    runtime.set_option_zero_dte_screener(Some(Arc::new(FixtureZeroDteScreenerReader)));
    runtime.set_option_earnings_screener(Some(Arc::new(FixtureEarningsScreenerReader)));
    ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    }
}

#[test]
fn analysis_projection_forwards_quote_query_and_neutral_wire() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL260918C00100000",
            "market=US&operation=quote",
        )
        .expect("option quote response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(
        value["entries"][0]["security"]["instrumentId"],
        "US.AAPL260918C00100000"
    );
    assert_eq!(value["entries"][0]["price"], 1.25);
    assert_eq!(value["entries"][0]["impliedVolatility"], 0.2);
    assert_eq!(value["total"], 1);
}

#[test]
fn analysis_projection_rejects_bad_operation_or_market() {
    for query in ["operation=greeks", "operation=quote&market=HK", "market=US"] {
        let error = ready_port()
            .read(
                "/api/v1/market-data/options/analysis/US.AAPL260918C00100000",
                query,
            )
            .expect_err("invalid analysis query");
        assert!(matches!(
            error,
            MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
                if code == "BAD_REQUEST"
        ));
    }
}

#[test]
fn volatility_projection_forwards_typed_query_and_preserves_summary() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=volatility&queryTimePeriod=month&hvTimePeriod=30",
        )
        .expect("option volatility response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["timestampStr"], "2026-08-29");
    assert_eq!(value["entries"][0]["impliedVolatility"], 25.0);
    assert_eq!(value["metadata"]["averageImpvol"], 25.0);
    assert_eq!(value["metadata"]["impvolStatus"], "ImpvolOvervalued");
    assert_eq!(value["total"], 1);
}

#[test]
fn exercise_probability_projection_forwards_typed_query_and_metrics() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL260918C00100000",
            "market=US&operation=exercise_probability",
        )
        .expect("option exercise probability response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["timestampStr"], "2026-08-29");
    assert_eq!(value["entries"][0]["securityPrice"], 225.0);
    assert_eq!(value["entries"][0]["strikeProbability"], 41.869);
    assert_eq!(value["total"], 1);
}

#[test]
fn underlying_overview_projection_forwards_typed_query_and_metrics() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=underlying_overview&indexOptionType=1",
        )
        .expect("option underlying overview response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["iv"], 25.0);
    assert_eq!(value["entries"][0]["hvList"][0]["hv"], 20.0);
    assert_eq!(value["total"], 1);
}

#[test]
fn underlying_rank_projection_forwards_typed_query_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=underlying_rank&sortType=7&isAsc=true&count=25&tradingDate=2026-08-29&page=next",
        )
        .expect("option underlying rank response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["security"]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["totalVolume"], 1200);
    assert_eq!(value["metadata"]["sortType"], 7);
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["nextCursor"], "next-2");
    assert_eq!(value["total"], 42);
}

#[test]
fn historical_volatility_projection_forwards_typed_query_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=historical_volatility&indexOptionType=1&beginTime=2025-08-29&endTime=2026-08-29&cursor=AQID",
        )
        .expect("option historical volatility response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["time"], "2026-08-29");
    assert_eq!(value["entries"][0]["iv"], 25.0);
    assert_eq!(value["entries"][0]["hv"], 20.0);
    assert_eq!(value["metadata"]["code"], "AAPL");
    assert_eq!(value["metadata"]["name"], "Apple");
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["nextCursor"], "BAUG");
    assert_eq!(value["total"], 1);
}

#[test]
fn market_statistics_projection_forwards_scope_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=market_statistics&optionMarket=1&dataType=0&beginTime=2026-08-01&endTime=2026-08-29&cursor=AQID",
        )
        .expect("option market statistic response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["callValue"], 100);
    assert_eq!(value["entries"][0]["putValue"], 80);
    assert_eq!(value["metadata"]["optionMarket"], 1);
    assert_eq!(value["metadata"]["dataType"], 0);
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["nextCursor"], "BAUG");
}

#[test]
fn historical_statistics_projection_forwards_owner_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=historical_statistics&indexOptionType=1&beginTime=2025-08-29&endTime=2026-08-29&cursor=AQID",
        )
        .expect("option underlying historical statistic response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["optionVolume"], 180);
    assert_eq!(value["entries"][0]["putCallOpenInterestRatio"], 0.777);
    assert_eq!(value["metadata"]["code"], "AAPL");
    assert_eq!(value["metadata"]["name"], "Apple");
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["nextCursor"], "BAUG");
}

#[test]
fn strategy_spread_projection_forwards_typed_query_and_lists_spreads() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=strategy_spread&optionStrategy=vertical&expireTime=2026-09-18&indexOptionType=1",
        )
        .expect("option strategy spread response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["spread"], 10.0);
    assert_eq!(value["entries"][1]["spread"], 20.0);
    assert_eq!(value["metadata"]["optionStrategy"], 4);
    assert_eq!(value["metadata"]["expireTime"], "2026-09-18");
    assert_eq!(value["hasMore"], false);
    assert_eq!(value["total"], 2);
}

#[test]
fn strategy_projection_forwards_filters_and_lists_combinations() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=strategy&optionStrategy=vertical&expireTime=2026-09-18&spread=10&optionType=1&strikePrice=100&indexOptionType=1",
        )
        .expect("option strategy response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["code"], "AAPL260918C/P100");
    assert_eq!(value["entries"][0]["stockOwner"]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["multiLegs"][0]["qtyRatio"], 1.0);
    assert_eq!(value["metadata"]["optionStrategy"], 4);
    assert_eq!(value["total"], 1);
}

#[test]
fn strategy_analysis_projection_forwards_combo_legs_and_metrics() {
    use percent_encoding::{NON_ALPHANUMERIC, utf8_percent_encode};

    let legs = serde_json::json!([
        {"security":{"market":"US","code":"AAPL260918C00100000"},"side":1,"qtyRatio":1},
        {"security":{"market":"US","code":"AAPL260918C00110000"},"side":2,"qtyRatio":1}
    ]);
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            &format!(
                "market=US&operation=strategy_analysis&multiLegs={}",
                utf8_percent_encode(&legs.to_string(), NON_ALPHANUMERIC)
            ),
        )
        .expect("option strategy analysis response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(value["entries"][0]["optionStrategy"], 4);
    assert_eq!(value["entries"][0]["bid1"], 1.0);
    assert_eq!(value["entries"][0]["breakevenPoints"][0], 100.0);
    assert_eq!(value["metadata"]["multiLegCount"], 2);
    assert_eq!(value["total"], 1);
}

#[test]
fn contract_rank_projection_forwards_typed_query_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "market=US&operation=contract_rank&sortType=10&isAsc=true&count=25&tradingDate=2026-08-29&page=next",
        )
        .expect("option contract rank response");
    assert_eq!(
        value["provider"]["featureId"],
        "derivatives.option_analysis"
    );
    assert_eq!(
        value["entries"][0]["security"]["instrumentId"],
        "US.AAPL260918C00100000"
    );
    assert_eq!(value["entries"][0]["optionPrice"], 1.25);
    assert_eq!(value["metadata"]["sortType"], 10);
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["nextCursor"], "next-2");
    assert_eq!(value["total"], 42);
}

#[test]
fn analysis_projection_rejects_underlying_instrument() {
    let error = ready_port()
        .read(
            "/api/v1/market-data/options/analysis/US.AAPL",
            "operation=quote",
        )
        .expect_err("underlying must not be treated as quote");
    assert!(matches!(
        error,
        MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
            if code == "BAD_REQUEST"
    ));
}

#[test]
fn analysis_projection_is_unavailable_without_typed_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read(
            "/api/v1/market-data/options/analysis/US.AAPL260918C00100000",
            "operation=quote",
        ),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
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
    assert_eq!(
        value["entries"][0]["option"].as_array().map(Vec::len),
        Some(0)
    );
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
    assert_eq!(
        value["entries"][0]["security"]["instrumentId"],
        "US.AAPL260918C00100000"
    );
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
    assert_eq!(
        value["entries"][0]["option"].as_array().map(Vec::len),
        Some(0)
    );
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

#[test]
fn event_projection_forwards_unusual_query_and_paginates() {
    let value = ready_port()
        .read(
            "/api/v1/market-data/options/events",
            "operation=unusual&market=US&underlyingProductClass=equity&underlying=US.AAPL&pageSize=25&cursor=next&sort=volume&sortAsc=true",
        )
        .expect("option event response");
    assert_eq!(value["provider"]["featureId"], "derivatives.option_events");
    assert_eq!(
        value["entries"][0]["option"]["instrumentId"],
        "US.AAPL260918C00100000"
    );
    assert_eq!(value["entries"][0]["owner"]["instrumentId"], "US.AAPL");
    assert_eq!(value["entries"][0]["price"], 1.25);
    assert_eq!(value["hasMore"], true);
    assert_eq!(value["total"], 2);
    assert_eq!(value["nextCursor"], "next-2");
}

#[test]
fn event_projection_rejects_unsupported_operation_market_and_product_class() {
    for query in [
        "operation=unknown",
        "operation=unusual&market=CN",
        "operation=unusual&underlyingProductClass=option_chain",
        "operation=unusual&underlying=HK.AAPL",
    ] {
        let error = ready_port()
            .read("/api/v1/market-data/options/events", query)
            .expect_err("invalid option event query");
        assert!(matches!(
            error,
            MarketDataOptionsReadSnapshotError::Failed { status: 400, ref code, .. }
                if code == "BAD_REQUEST"
        ));
    }
}

#[test]
fn event_projection_dispatches_zero_dte_and_earnings_readers() {
    let port = ready_port();
    let zero = port
        .read(
            "/api/v1/market-data/options/events",
            "operation=zero_dte&market=US&underlying=US.AAPL",
        )
        .expect("0DTE response");
    assert_eq!(zero["entries"][0]["owner"]["instrumentId"], "US.AAPL");
    assert_eq!(zero["entries"][0]["drilldownContext"]["chain"]["productCode"], "AAPL");
    assert_eq!(zero["nextCursor"], "zero-next");
    let earnings = port
        .read(
            "/api/v1/market-data/options/events",
            "operation=earnings&market=US&underlying=US.AAPL",
        )
        .expect("earnings response");
    assert_eq!(earnings["entries"][0]["earningsQuarter"], "2026Q3");
    assert_eq!(earnings["total"], 1);
}

#[test]
fn event_projection_reports_screener_reader_unavailable_independently() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_events(Some(Arc::new(FixtureOptionEventReader {
        result: Ok(jftrade_integration_futu::OptionEventPage {
            events: Vec::new(),
            next_page: None,
            all_count: Some(0),
            update_timestamp: None,
        }),
    })));
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    assert!(matches!(
        port.read(
            "/api/v1/market-data/options/events",
            "operation=zero_dte&market=US"
        ),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
}

#[test]
fn event_projection_is_unavailable_without_typed_reader() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(Arc::new(SharedTradeReadRuntime::default())),
    };
    assert!(matches!(
        port.read("/api/v1/market-data/options/events", "operation=unusual"),
        Err(MarketDataOptionsReadSnapshotError::Unavailable(_))
    ));
}

#[test]
fn event_projection_maps_reader_failure_to_bad_gateway() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_events(Some(Arc::new(FixtureOptionEventReader {
        result: Err("OpenD rate limited".to_owned()),
    })));
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    let error = port
        .read("/api/v1/market-data/options/events", "operation=unusual")
        .expect_err("reader failure");
    assert!(matches!(
        error,
        MarketDataOptionsReadSnapshotError::Failed { status: 502, ref code, .. }
            if code == "BAD_GATEWAY"
    ));
}

#[test]
fn event_projection_accepts_empty_valid_result() {
    let state = Arc::new(ActiveProviderState::new(Some(MarketDataProvider::Futu)));
    state.set_readiness(false, true, true);
    let runtime = Arc::new(SharedTradeReadRuntime::default());
    runtime.set_option_events(Some(Arc::new(FixtureOptionEventReader {
        result: Ok(jftrade_integration_futu::OptionEventPage {
            events: Vec::new(),
            next_page: None,
            all_count: Some(0),
            update_timestamp: None,
        }),
    })));
    let port = ProductionMarketDataOptionsPort {
        active_provider_state: state,
        trade_runtime: Some(runtime),
    };
    let value = port
        .read("/api/v1/market-data/options/events", "operation=unusual")
        .expect("empty event response");
    assert_eq!(value["entries"], serde_json::json!([]));
    assert_eq!(value["hasMore"], false);
    assert_eq!(value["total"], 0);
}
