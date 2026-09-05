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
fn instantiate_persists_the_same_normalized_binding_as_runtime_update() {
    let dir = tempdir().expect("tempdir");
    let db_path = dir.path().join("strategy-instantiate.db");
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
    store
        .save_definition(
            jftrade_store_sqlite::StoredStrategyDefinition {
                id: "normalize-instantiate".to_owned(),
                name: "Normalize instantiate".to_owned(),
                version: "0.1.0".to_owned(),
                description: String::new(),
                runtime: "pine-pinets".to_owned(),
                source_format: "pine-v6".to_owned(),
                symbol: "US.AAPL".to_owned(),
                interval: "5m".to_owned(),
                script: "//@version=6\nstrategy(\"x\")".to_owned(),
                visual_model_json: "{}".to_owned(),
                created_at: "2026-09-05T00:00:00Z".to_owned(),
                updated_at: "2026-09-05T00:00:00Z".to_owned(),
                deleted_at: None,
            },
            "2026-09-05T00:00:00Z",
        )
        .expect("save definition");
    let port = ProductionStrategyDefinitionPort { store: Arc::clone(&store) };
    let result = port
        .mutate(&StrategyDefinitionWriteInput {
            operation: StrategyDefinitionWriteOperation::Instantiate,
            definition_id: Some("normalize-instantiate".to_owned()),
            definition: None,
            binding: Some(json!({
                "symbol": "aapl",
                "executionMode": "notify_only",
                "chartType": "standard"
            })),
            binding_error: None,
        })
        .expect("instantiate strategy");
    assert_eq!(result["binding"]["symbols"], json!(["US.AAPL"]));
    assert_eq!(result["binding"]["executeOrders"], false);
    assert_eq!(result["binding"]["interval"], "5m");
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

#[test]
fn test_result_view_all_six_views_and_validation() {
    use crate::product::product_research_backtest_projection::{
        project_authoritative_result_view, validate_result_view_request,
    };
    use crate::product::{BacktestResultViewError, BacktestResultViewRequest};

    let sample_run = json!({
        "id": "run-views-test",
        "status": "completed",
        "marketDataProvider": "futu",
        "request": {
            "symbol": "HK.00700",
            "interval": "1m",
            "startTime": "2026-01-01T09:30:00Z",
            "endTime": "2026-01-01T16:00:00Z"
        },
        "result": {
            "cases": [{
                "finalEquity": "110000.0",
                "realizedPnl": "10000.0",
                "cash": "110000.0",
                "maxDrawdown": "0.05",
                "currentDrawdown": "0.01",
                "totalTrades": 1,
                "winRate": "1.0",
                "totalFees": "25.0",
                "processedBars": 100,
                "warnings": ["warning 1"],
                "orders": [{
                    "orderId": "ord-1",
                    "side": "BUY",
                    "quantity": "100",
                    "status": "FILLED",
                    "filledQuantity": "100",
                    "filledPrice": "300.0",
                    "submittedAt": "2026-01-01T10:00:00Z",
                    "filledAt": "2026-01-01T10:00:00Z"
                }],
                "fills": [{
                    "tradeId": "trd-1",
                    "orderId": "ord-1",
                    "side": "BUY",
                    "price": "300.0",
                    "quantity": "100",
                    "quoteQuantity": "30000.0",
                    "time": "2026-01-01T10:00:00Z",
                    "totalFee": "25.0",
                    "realizedPnl": "0.0"
                }],
                "equityCurve": [{"time": "2026-01-01T10:00:00Z", "equity": "110000.0"}],
                "drawdownCurve": [{"time": "2026-01-01T10:00:00Z", "drawdown": "0.0"}]
            }],
            "candles": [{
                "time": "2026-01-01T10:00:00Z",
                "open": 300.0,
                "high": 305.0,
                "low": 299.0,
                "close": 302.0,
                "volume": 1000.0
            }],
            "logs": [{"time": "2026-01-01T10:00:00Z", "message": "strategy executed"}],
            "runtimeErrors": [{"time": "2026-01-01T10:00:00Z", "error": "simulated non-fatal"}]
        }
    });

    // 1. Verify all 6 views
    for (view_name, expected_series_key) in [
        ("summary", None),
        ("chart", Some("candles")),
        ("orders", Some("orderBook")),
        ("logs", Some("logs")),
        ("warnings", Some("warnings")),
        ("errors", Some("runtimeErrors")),
    ] {
        let req = BacktestResultViewRequest {
            run_id: "run-views-test".to_owned(),
            view: Some(view_name.to_owned()),
            ..Default::default()
        };
        let res = project_authoritative_result_view(&sample_run, None, &req).expect("valid view");
        assert_eq!(res["view"], view_name);
        assert_eq!(res["run"]["id"], "run-views-test");
        assert!(res["summary"].is_object());
        assert!(res["window"].is_object());
        if let Some(key) = expected_series_key {
            assert!(res["series"][key].is_array(), "missing series key {key} for {view_name}");
        }
    }

    // 2. Strict validation assertions
    let bad_view_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        view: Some("invalid_view".to_owned()),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_view_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_include_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        view: Some("orders".to_owned()),
        include: Some(vec!["candles".to_owned()]),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_include_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_series_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["unsupported".to_owned()]),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_series_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_time_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        start_time: Some("not-rfc3339".to_owned()),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_time_req), Err(BacktestResultViewError::Invalid(_))));

    let time_order_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        start_time: Some("2026-01-01T12:00:00Z".to_owned()),
        end_time: Some("2026-01-01T10:00:00Z".to_owned()),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&time_order_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_cursor_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        cursor: Some("-5".to_owned()),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_cursor_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_limit_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        limit: Some(3000),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_limit_req), Err(BacktestResultViewError::Invalid(_))));

    let bad_res_req = BacktestResultViewRequest {
        run_id: "run-1".to_owned(),
        resolution: Some("invalid_res".to_owned()),
        ..Default::default()
    };
    assert!(matches!(validate_result_view_request(&bad_res_req), Err(BacktestResultViewError::Invalid(_))));
}

#[test]
fn test_result_view_order_fee_aggregation_and_numeric_types() {
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;
    use crate::product::BacktestResultViewRequest;

    let payload = json!({
        "id": "run-fee-test",
        "status": "completed",
        "result": {
            "cases": [{
                "orders": [{
                    "orderId": "order-split",
                    "side": "BUY",
                    "quantity": "200.0",
                    "status": "FILLED",
                    "filledQuantity": "200.0",
                    "filledPrice": "150.5",
                    "submittedAt": "2026-01-01T10:00:00Z"
                }],
                "fills": [
                    {
                        "tradeId": "fill-1",
                        "orderId": "order-split",
                        "side": "BUY",
                        "price": "150.0",
                        "quantity": "100.0",
                        "quoteQuantity": "15000.0",
                        "time": "2026-01-01T10:00:01Z",
                        "totalFee": "12.5",
                        "realizedPnl": "0.0"
                    },
                    {
                        "tradeId": "fill-2",
                        "orderId": "order-split",
                        "side": "BUY",
                        "price": "151.0",
                        "quantity": "100.0",
                        "quoteQuantity": "15100.0",
                        "time": "2026-01-01T10:00:02Z",
                        "totalFee": "13.5",
                        "realizedPnl": "0.0"
                    }
                ]
            }]
        }
    });

    let ord_req = BacktestResultViewRequest {
        run_id: "run-fee-test".to_owned(),
        view: Some("orders".to_owned()),
        ..Default::default()
    };
    let ord_res = project_authoritative_result_view(&payload, None, &ord_req).unwrap();
    let orders = ord_res["series"]["orderBook"].as_array().unwrap();
    assert_eq!(orders.len(), 1);
    let order = &orders[0];
    assert_eq!(order["orderId"], "order-split");
    // Aggregated total fee = 12.5 + 13.5 = 26.0 (number type!)
    assert_eq!(order["totalFee"], 26.0);
    assert_eq!(order["totalFees"], 26.0);
    assert_eq!(order["quantity"], "200.0");
    assert_eq!(order["filledQuantity"], "200.0");
    assert_eq!(order["filledPrice"], "150.5");

    let chart_req = BacktestResultViewRequest {
        run_id: "run-fee-test".to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["trades".to_owned()]),
        ..Default::default()
    };
    let chart_res = project_authoritative_result_view(&payload, None, &chart_req).unwrap();
    let trades = chart_res["series"]["trades"].as_array().unwrap();
    assert_eq!(trades.len(), 2);
    assert_eq!(trades[0]["price"], "150.0");
    assert_eq!(trades[0]["qty"], "100.0");
    assert_eq!(trades[0]["quantity"], "100.0");
    assert_eq!(trades[0]["totalFee"], 12.5);
}

#[test]
fn test_result_view_resolution_downsampling() {
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;
    use crate::product::BacktestResultViewRequest;

    let payload = json!({
        "id": "run-downsample-test",
        "status": "completed",
        "result": {
            "candles": [
                {"time": "2026-01-01T10:00:00Z", "open": 100.0, "high": 105.0, "low": 99.0, "close": 102.0, "volume": 10.0},
                {"time": "2026-01-01T10:01:00Z", "open": 102.0, "high": 108.0, "low": 101.0, "close": 107.0, "volume": 20.0},
                {"time": "2026-01-01T10:05:00Z", "open": 107.0, "high": 110.0, "low": 106.0, "close": 108.0, "volume": 30.0},
                {"time": "2026-01-01T10:06:00Z", "open": 108.0, "high": 112.0, "low": 107.0, "close": 111.0, "volume": 40.0}
            ],
            "equityCurve": [
                {"time": "2026-01-01T10:00:00Z", "equity": 1000.0},
                {"time": "2026-01-01T10:01:00Z", "equity": 1050.0},
                {"time": "2026-01-01T10:05:00Z", "equity": 1080.0},
                {"time": "2026-01-01T10:06:00Z", "equity": 1120.0}
            ]
        }
    });

    let req = BacktestResultViewRequest {
        run_id: "run-downsample-test".to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["candles".to_owned(), "pnlCurve".to_owned()]),
        resolution: Some("5m".to_owned()),
        ..Default::default()
    };
    let res = project_authoritative_result_view(&payload, None, &req).unwrap();
    let candles = res["series"]["candles"].as_array().unwrap();
    assert_eq!(candles.len(), 2, "4 minutes bucketed into two 5m candles");
    assert_eq!(candles[0]["open"], 100.0);
    assert_eq!(candles[0]["high"], 108.0);
    assert_eq!(candles[0]["low"], 99.0);
    assert_eq!(candles[0]["close"], 107.0);
    assert_eq!(candles[0]["volume"], 30.0);

    let pnl = res["series"]["pnlCurve"].as_array().unwrap();
    assert_eq!(pnl.len(), 4, "curves are not downsampled under Go contract");
    assert_eq!(pnl[0]["equity"], 1000.0);
    assert_eq!(pnl[3]["equity"], 1120.0);
}

#[test]
fn test_result_view_seed_preservation_and_metadata_stripping() {
    let raw_payload = json!({
        "symbol": "HK.00700",
        "interval": "1m",
        "marketDataProvider": "futu",
        "__resultViewSeed": {
            "formal_candles": [{"time": "2026-01-01T09:30:00Z", "open": 300.0}],
            "formalStartTimeMs": 1704097800000_i64,
            "warmupBars": 10,
            "symbol": "HK.00700",
            "nativeInterval": "1m"
        }
    });

    let raw_str = raw_payload.to_string();
    let (decoded, provider) = crate::product::product_production_ports::product_production_ports_execution::decode_request_metadata(&raw_str).unwrap();
    assert_eq!(provider.as_deref(), Some("futu"));
    assert!(decoded.get("__resultViewSeed").is_none(), "seed must be stripped in public projection");
    assert!(decoded.get("marketDataProvider").is_none());
    assert!(decoded.get("__marketDataProvider").is_none());
    assert_eq!(decoded["symbol"], "HK.00700");
}

#[test]
fn test_mcp_result_view_and_research_result_view_identity() {
    use crate::product::product_research_backtest_execution::build_result_view_request_from_options;
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;
    use crate::product::BacktestResultViewRequest;

    let options = json!({
        "view": "chart",
        "include": ["candles", "trades"],
        "startTime": "2026-01-01T10:00:00Z",
        "endTime": "2026-01-01T11:00:00Z",
        "limit": 100,
        "resolution": "5m"
    });

    let run_id = "test-run-123";
    let research_req = build_result_view_request_from_options(run_id, Some(&options));

    let mcp_req = BacktestResultViewRequest {
        run_id: run_id.to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["candles".to_owned(), "trades".to_owned()]),
        start_time: Some("2026-01-01T10:00:00Z".to_owned()),
        end_time: Some("2026-01-01T11:00:00Z".to_owned()),
        cursor: None,
        limit: Some(100),
        resolution: Some("5m".to_owned()),
    };

    assert_eq!(
        research_req, mcp_req,
        "both execution pathways must construct identical view requests"
    );

    let payload = json!({
        "id": run_id,
        "status": "completed",
        "request": {
            "symbol": "HK.00700",
            "interval": "1m",
            "initialBalance": 100000.0
        },
        "result": {
            "candles": [
                {"time": "2026-01-01T10:00:00Z", "open": 100.0, "high": 105.0, "low": 99.0, "close": 102.0, "volume": 10.0}
            ],
            "trades": []
        }
    });
    let research_view = project_authoritative_result_view(&payload, None, &research_req).unwrap();
    let mcp_view = project_authoritative_result_view(&payload, None, &mcp_req).unwrap();
    assert_eq!(
        research_view, mcp_view,
        "both execution pathways must produce identical result view JSON"
    );
}

#[test]
fn test_result_view_contract_alignment_and_edge_cases() {
    use crate::product::product_research_backtest_projection::project_authoritative_result_view;
    use crate::product::BacktestResultViewRequest;

    let payload = json!({
        "id": "run-contract-align",
        "status": "completed",
        "request": {
            "definitionId": "def-1",
            "definitionVersion": "v1",
            "market": "HK",
            "code": "00700",
            "symbol": "HK.00700",
            "instrumentType": "stock",
            "marketDataProvider": "futu",
            "interval": "5m",
            "startDate": "2026-01-01",
            "endDate": "2026-01-02",
            "startTime": "2026-01-01T09:30:00Z",
            "endTime": "2026-01-02T16:00:00Z",
            "marketTimezone": "Asia/Hong_Kong",
            "initialBalance": 100000.0,
            "rehabType": "forward",
            "chartType": "candlestick",
            "executionModel": "bar_close",
            "useExtendedHours": false,
            "tradingCosts": {"commissionRate": 0.0003}
        },
        "result": {
            "candles": [
                {"time": "2026-01-01T10:00:00Z", "open": 100.0, "high": 105.0, "low": 99.0, "close": 102.0, "volume": 10.0},
                {"time": "2026-01-01T10:05:00Z", "open": 102.0, "high": 108.0, "low": 101.0, "close": 107.0, "volume": 20.0}
            ],
            "pnl": 5000.0,
            "logs": [{"time": "2026-01-01T10:00:00Z", "message": "log 1"}],
            "warnings": [{"time": "2026-01-01T10:05:00Z", "message": "warning 1"}],
            "runtimeErrors": [{"time": "2026-01-01T10:06:00Z", "message": "error 1"}]
        }
    });

    // 1. Explicit resolution="auto" is supported
    let req_auto = BacktestResultViewRequest {
        run_id: "run-contract-align".to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["candles".to_owned()]),
        resolution: Some("auto".to_owned()),
        ..Default::default()
    };
    let res_auto = project_authoritative_result_view(&payload, None, &req_auto).unwrap();
    assert_eq!(res_auto["window"]["nativeInterval"], "5m");
    assert!(res_auto["window"].get("resolution").is_some());

    // 2. Reject finer resolution than native interval
    let req_finer = BacktestResultViewRequest {
        run_id: "run-contract-align".to_owned(),
        view: Some("chart".to_owned()),
        include: Some(vec!["candles".to_owned()]),
        resolution: Some("1m".to_owned()),
        ..Default::default()
    };
    let err = project_authoritative_result_view(&payload, None, &req_finer).unwrap_err();
    assert!(err.to_string().contains("is finer than native interval"));

    // 3. Enriched summary and 22 run payload fields
    let req_summary = BacktestResultViewRequest {
        run_id: "run-contract-align".to_owned(),
        view: Some("summary".to_owned()),
        ..Default::default()
    };
    let res_summary = project_authoritative_result_view(&payload, None, &req_summary).unwrap();
    assert_eq!(res_summary["summary"]["quoteCurrency"], "HKD");
    assert_eq!(res_summary["summary"]["totalReturn"], 0.05);
    assert!(res_summary["summary"].get("latestLog").is_some());
    assert!(res_summary["summary"].get("latestWarning").is_some());
    assert!(res_summary["summary"].get("latestRuntimeError").is_some());

    let run_meta = &res_summary["run"];
    assert_eq!(run_meta["definitionId"], "def-1");
    assert_eq!(run_meta["symbol"], "HK.00700");
    assert_eq!(run_meta["executionModel"], "bar_close");
    assert_eq!(run_meta["chartType"], "candlestick");
}
