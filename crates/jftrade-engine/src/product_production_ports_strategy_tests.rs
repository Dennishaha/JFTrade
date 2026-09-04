use super::*;
use jftrade_store_sqlite::StrategyDefinitionStore;
use tempfile::tempdir;

#[test]
fn shadow_request_matches_go_sample_candles_and_uses_run_script_analysis_mode() {
    let request = pine_shadow_request("shadow-job".to_owned(), "plot(close)");
    assert_eq!(request.job_id, "shadow-job");
    assert_eq!(request.symbol, "JFTRADE.SAMPLE");
    assert_eq!(request.timeframe, "1m");
    assert_eq!(request.mode, "analyze");
    assert_eq!(request.candles.len(), 80);
    assert_eq!(request.candles[0].open_time, 1_704_067_200_000);
    assert_eq!(request.candles[0].close_time, 1_704_067_260_000);
    assert_eq!(request.candles[0].close, 100.0);
    let last = &request.candles[79];
    assert_eq!(last.open_time, 1_704_071_940_000);
    assert_eq!(last.volume, 1_079.0);
    assert_eq!(last.close, 179.0 + (79.0_f64 / 3.0).sin());
}

#[test]
fn shadow_result_derives_go_compatible_signal_count_from_plot_tails() {
    let result = jftrade_integration_pine::PineRunResult {
        plots: vec![jftrade_integration_pine::PinePlot {
            name: "close".to_owned(),
            values: vec![100.0, 101.0],
        }],
        metadata: jftrade_integration_pine::PineWorkerMetadata {
            pine_ts_version: "0.9.31".to_owned(),
            ..Default::default()
        },
        ..Default::default()
    };
    let payload = pine_shadow_result(result);
    assert_eq!(payload["engineVersion"], "0.9.31");
    assert_eq!(payload["plots"]["close"]["data"], json!([100.0, 101.0]));
    assert_eq!(payload["signals"]["close"], 101.0);
}

#[test]
fn strategy_definition_preview_derives_warmup_bars_and_overrides_preview_parameters() {
    let dir = tempdir().expect("tempdir");
    let db_path = dir.path().join("strategy.db");
    let connection = rusqlite::Connection::open(&db_path).expect("create strategy database");
    jftrade_store_sqlite::initialize_current(&connection, "strategy")
        .expect("initialize strategy schema");
    drop(connection);
    let store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &db_path,
            jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .expect("open strategy definition store"),
    );

    let script = r#"//@version=6
strategy("Pine Preview Window", overlay=true)
slow = ta.sma(close, 66)
log.info("close")"#;

    let def = jftrade_store_sqlite::StoredStrategyDefinition {
        id: "dsl-preview-day-window".to_owned(),
        name: "Pine Preview Window".to_owned(),
        version: "0.1.0".to_owned(),
        description: "preview test".to_owned(),
        runtime: "pine-pinets".to_owned(),
        source_format: "pine-v6".to_owned(),
        symbol: "HK.00700".to_owned(),
        interval: "5m".to_owned(),
        script: script.to_owned(),
        visual_model_json: "{}".to_owned(),
        created_at: "2026-06-13T00:00:00Z".to_owned(),
        updated_at: "2026-06-13T00:00:00Z".to_owned(),
        deleted_at: None,
    };
    store.save_definition(def, "2026-06-13T00:00:00Z").expect("save definition");

    let ports = ProductionStrategyDefinitionPort {
        store: Arc::clone(&store),
    };

    // 1. Query with default preview
    let default_preview = StrategyDefinitionPreview::default();
    let default_result = ports
        .get("dsl-preview-day-window", &default_preview)
        .expect("get strategy definition")
        .expect("definition exists");

    assert_eq!(default_result["derivedWarmupBars"], 66);
    assert_eq!(default_result["derivedWarmupInterval"], "5m");
    assert_eq!(default_result["symbol"], "HK.00700");
    assert_eq!(default_result["interval"], "5m");

    // 2. Query with preview overrides (matching Go strategy_preview_test.go)
    let override_preview = StrategyDefinitionPreview {
        symbol: Some("US.AAPL".to_owned()),
        interval: Some("15m".to_owned()),
        use_extended_hours: true,
    };
    let override_result = ports
        .get("dsl-preview-day-window", &override_preview)
        .expect("get strategy definition with preview")
        .expect("definition exists");

    assert_eq!(override_result["derivedWarmupBars"], 66);
    assert_eq!(override_result["derivedWarmupInterval"], "15m");
    assert_eq!(override_result["symbol"], "HK.00700");
    assert_eq!(override_result["interval"], "5m");
}

#[test]
fn test_strategy_preview_symbol_session_aware_warmup_scaling() {
    let dir = tempdir().expect("tempdir");
    let db_path = dir.path().join("strategy_session.db");
    let connection = rusqlite::Connection::open(&db_path).expect("create strategy database");
    jftrade_store_sqlite::initialize_current(&connection, "strategy")
        .expect("initialize strategy schema");
    drop(connection);
    let store = Arc::new(
        StrategyDefinitionStore::open_existing(
            &db_path,
            jftrade_store_sqlite::STRATEGY_DEFINITION_PRODUCTION_PROFILE,
        )
        .expect("open strategy definition store"),
    );

    let script = r#"//@version=6
strategy("Session Warmup", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
fast = ta.sma(close, 10)"#;

    let def = jftrade_store_sqlite::StoredStrategyDefinition {
        id: "strat-session-warmup".to_owned(),
        name: "Session Warmup".to_owned(),
        version: "0.1.0".to_owned(),
        description: "session warmup test".to_owned(),
        runtime: "pine-pinets".to_owned(),
        source_format: "pine-v6".to_owned(),
        symbol: "HK.00700".to_owned(),
        interval: "1m".to_owned(),
        script: script.to_owned(),
        visual_model_json: "{}".to_owned(),
        created_at: "2026-06-13T00:00:00Z".to_owned(),
        updated_at: "2026-06-13T00:00:00Z".to_owned(),
        deleted_at: None,
    };
    store.save_definition(def, "2026-06-13T00:00:00Z").expect("save definition");

    let ports = ProductionStrategyDefinitionPort {
        store: Arc::clone(&store),
    };

    // 1. US AAPL with extended hours on 1m chart: 20 days * 1440 min / 1m = 28800 bars
    let us_ext = ports
        .get("strat-session-warmup", &StrategyDefinitionPreview {
            symbol: Some("US.AAPL".to_owned()),
            interval: Some("1m".to_owned()),
            use_extended_hours: true,
        })
        .unwrap()
        .unwrap();
    assert_eq!(us_ext["derivedWarmupBars"], 28800);

    // 2. US AAPL regular hours on 1m chart: 20 days * 390 min / 1m = 7800 bars
    let us_reg = ports
        .get("strat-session-warmup", &StrategyDefinitionPreview {
            symbol: Some("US.AAPL".to_owned()),
            interval: Some("1m".to_owned()),
            use_extended_hours: false,
        })
        .unwrap()
        .unwrap();
    assert_eq!(us_reg["derivedWarmupBars"], 7800);

    // 3. HK 00700 regular hours on 1m chart: 20 days * 330 min / 1m = 6600 bars
    let hk = ports
        .get("strat-session-warmup", &StrategyDefinitionPreview {
            symbol: Some("HK.00700".to_owned()),
            interval: Some("1m".to_owned()),
            use_extended_hours: false,
        })
        .unwrap()
        .unwrap();
    assert_eq!(hk["derivedWarmupBars"], 6600);

    // 4. SH 600519 regular hours on 1m chart: 20 days * 240 min / 1m = 4800 bars
    let sh = ports
        .get("strat-session-warmup", &StrategyDefinitionPreview {
            symbol: Some("SH.600519".to_owned()),
            interval: Some("1m".to_owned()),
            use_extended_hours: false,
        })
        .unwrap()
        .unwrap();
    assert_eq!(sh["derivedWarmupBars"], 4800);

    // 5. 5m chart: US AAPL regular -> 7800 / 5 = 1560 bars
    let us_5m = ports
        .get("strat-session-warmup", &StrategyDefinitionPreview {
            symbol: Some("US.AAPL".to_owned()),
            interval: Some("5m".to_owned()),
            use_extended_hours: false,
        })
        .unwrap()
        .unwrap();
    assert_eq!(us_5m["derivedWarmupBars"], 1560);

    // 6. Verify stored definition in DB is completely unmutated
    let stored = store.get_definition("strat-session-warmup", false).unwrap().unwrap();
    assert_eq!(stored.symbol, "HK.00700");
    assert_eq!(stored.interval, "1m");
}

#[test]
fn test_strategy_preview_mtf_alignment_and_lower_timeframe_rejection() {
    // 1. Lower timeframe: "1m" target on "5m" chart -> rejected
    let script_lower = r#"//@version=6
strategy("MTF Lower", overlay=true)
lower = request.security(syminfo.tickerid, "1m", ta.sma(close, 20))"#;
    let comp_lower = jftrade_strategy::pine::compile(script_lower);
    assert!(comp_lower.ok);
    let lower_err = comp_lower.requirements.validate_timeframe_alignments("US.AAPL", "5m", false);
    assert!(lower_err.is_err());
    let err_msg = lower_err.unwrap_err();
    assert!(err_msg.contains("lower than strategy interval 5m"));

    // 2. Unaligned intraday: "7m" target on "5m" chart -> rejected
    let script_unaligned = r#"//@version=6
strategy("MTF Unaligned", overlay=true)
unaligned = request.security(syminfo.tickerid, "7m", ta.sma(close, 20))"#;
    let comp_unaligned = jftrade_strategy::pine::compile(script_unaligned);
    assert!(comp_unaligned.ok);
    let unaligned_err = comp_unaligned.requirements.validate_timeframe_alignments("US.AAPL", "5m", false);
    assert!(unaligned_err.is_err());
    let unaligned_msg = unaligned_err.unwrap_err();
    assert!(unaligned_msg.contains("not aligned with strategy interval 5m"));

    // 3. Aligned higher timeframe: "15m" target on "5m" chart alone
    let script_valid = r#"//@version=6
strategy("MTF Valid", overlay=true)
higher = request.security(syminfo.tickerid, "15m", ta.sma(close, 20))"#;
    let comp_valid = jftrade_strategy::pine::compile(script_valid);
    assert!(comp_valid.ok);
    let valid_err = comp_valid.requirements.validate_timeframe_alignments("US.AAPL", "5m", false);
    assert!(valid_err.is_ok());
    assert_eq!(comp_valid.requirements.derived_warmup_bars_with_session("US.AAPL", "5m", false), 60);
}

#[test]
fn test_result_view_warmup_tagging_and_curve_trimming() {
    use crate::product::product_research_backtest_projection::project_result_view;

    let payload = json!({
        "id": "run-warmup-view-test",
        "status": "completed",
        "request": {
            "symbol": "US.AAPL",
            "interval": "1m",
            "startTime": "2026-01-01T09:30:00Z",
            "endTime": "2026-01-01T16:00:00Z",
        },
        "result": {
            "cases": [{
                "id": "case-1",
                "status": "completed",
                "processedBars": 4,
                "cash": "10000.0",
                "finalEquity": "10100.0",
                "realizedPnl": "100.0",
                "orders": [
                    {
                        "orderId": "ord-warmup",
                        "submittedAt": "2026-01-01T09:00:00Z",
                        "status": "filled"
                    },
                    {
                        "orderId": "ord-formal",
                        "submittedAt": "2026-01-01T10:00:00Z",
                        "status": "filled"
                    }
                ],
                "fills": [
                    {
                        "tradeId": "trade-warmup",
                        "orderId": "ord-warmup",
                        "time": "2026-01-01T09:00:00Z",
                        "price": "100.0",
                        "quantity": "10"
                    },
                    {
                        "tradeId": "trade-formal",
                        "orderId": "ord-formal",
                        "time": "2026-01-01T10:00:00Z",
                        "price": "110.0",
                        "quantity": "10"
                    }
                ],
                "equityCurve": [
                    {"time": "2026-01-01T09:00:00Z", "equity": "10000.0"},
                    {"time": "2026-01-01T09:30:00Z", "equity": "10000.0"},
                    {"time": "2026-01-01T10:00:00Z", "equity": "10100.0"}
                ],
                "drawdownCurve": [
                    {"time": "2026-01-01T09:00:00Z", "drawdown": "0.0"},
                    {"time": "2026-01-01T09:30:00Z", "drawdown": "0.0"},
                    {"time": "2026-01-01T10:00:00Z", "drawdown": "0.0"}
                ],
                "warnings": []
            }],
            "candles": [
                {"start": "2026-01-01T09:00:00Z", "close": "100.0"},
                {"start": "2026-01-01T09:30:00Z", "close": "105.0"},
                {"start": "2026-01-01T10:00:00Z", "close": "110.0"}
            ]
        }
    });

    // 1. Chart view: curves and candles before 09:30:00Z must be hidden
    let chart_opts = json!({"view": "chart", "include": ["candles", "pnlCurve", "trades"]});
    let chart_view = project_result_view(&payload, Some(&chart_opts));
    let candles = chart_view["series"]["candles"].as_array().unwrap();
    assert_eq!(candles.len(), 2, "warmup candle at 09:00 must be trimmed");
    assert_eq!(candles[0]["start"], "2026-01-01T09:30:00Z");

    let pnl_curve = chart_view["series"]["pnlCurve"].as_array().unwrap();
    assert_eq!(pnl_curve.len(), 2, "warmup equity point at 09:00 must be trimmed");
    assert_eq!(pnl_curve[0]["time"], "2026-01-01T09:30:00Z");

    let trades = chart_view["series"]["trades"].as_array().unwrap();
    assert_eq!(trades.len(), 2, "both trades must be preserved");
    assert_eq!(trades[0]["warmup"], true, "trade at 09:00 must be marked warmup=true");
    assert_eq!(trades[1]["warmup"], false, "trade at 10:00 must be marked warmup=false");

    // 2. Orders view: both orders preserved with correct warmup flags
    let orders_opts = json!({"view": "orders"});
    let orders_view = project_result_view(&payload, Some(&orders_opts));
    let orders = orders_view["series"]["orderBook"].as_array().unwrap();
    assert_eq!(orders.len(), 2);
    assert_eq!(orders[0]["warmup"], true);
    assert_eq!(orders[1]["warmup"], false);
}
