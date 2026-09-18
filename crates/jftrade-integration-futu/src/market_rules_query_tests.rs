//! Parity coverage for Go `pkg/futu/adapter_bridge_test.go` market-rule cases.

use super::*;
use jftrade_marketdata::TradeQuoteSnapshot;
use std::sync::atomic::{AtomicUsize, Ordering};

fn snapshot(symbol: &str, lot_size: i32) -> TradeQuoteSnapshot {
    TradeQuoteSnapshot {
        symbol: Some(symbol.to_owned()),
        lot_size: Some(lot_size),
        ..Default::default()
    }
}

#[derive(Clone)]
enum StubResult<T> {
    Ok(T),
    Err(String),
}

impl<T: Clone> StubResult<T> {
    fn to_result(&self) -> Result<T, String> {
        match self {
            StubResult::Ok(value) => Ok(value.clone()),
            StubResult::Err(message) => Err(message.clone()),
        }
    }
}

struct StubStaticInfo {
    items: StubResult<Vec<SecurityInfoItem>>,
    calls: Arc<AtomicUsize>,
}

impl std::fmt::Debug for StubStaticInfo {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.debug_struct("StubStaticInfo").finish()
    }
}

impl SecurityInfoReadPort for StubStaticInfo {
    fn query_static_info(&self, _symbols: &[String]) -> Result<Vec<SecurityInfoItem>, String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        self.items.to_result()
    }
}

struct StubSnapshots {
    items: StubResult<Vec<TradeQuoteSnapshot>>,
    calls: Arc<AtomicUsize>,
}

impl std::fmt::Debug for StubSnapshots {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.debug_struct("StubSnapshots").finish()
    }
}

impl SecuritySnapshotReadPort for StubSnapshots {
    fn query(&self, _instruments: &[String]) -> Result<Vec<TradeQuoteSnapshot>, String> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        self.items.to_result()
    }
}

fn reader(
    static_info: StubResult<Vec<SecurityInfoItem>>,
    snapshots: StubResult<Vec<TradeQuoteSnapshot>>,
) -> (OpenDMarketRulesReader, Arc<AtomicUsize>, Arc<AtomicUsize>) {
    let static_calls = Arc::new(AtomicUsize::new(0));
    let snapshot_calls = Arc::new(AtomicUsize::new(0));
    (
        OpenDMarketRulesReader::with_ports(
            Arc::new(StubStaticInfo {
                items: static_info,
                calls: Arc::clone(&static_calls),
            }),
            Arc::new(StubSnapshots {
                items: snapshots,
                calls: Arc::clone(&snapshot_calls),
            }),
        ),
        static_calls,
        snapshot_calls,
    )
}

#[test]
fn market_rules_use_security_info_lot_size_without_warnings() {
    // Parity: go:452dea11:pkg/futu/adapter_bridge_test.go:124
    // TestBrokerAdapterQueryMarketRulesUsesSecurityInfoLotSize
    let (reader, static_calls, snapshot_calls) = reader(
        StubResult::Ok(vec![SecurityInfoItem {
            symbol: "HK.00700".to_owned(),
            lot_size: Some(100),
        }]),
        StubResult::Ok(vec![snapshot("HK.00700", 999)]),
    );
    let result = reader.query(&["HK.00700".to_owned()]).expect("rules");
    assert!(result.warnings.is_empty(), "no fallback warning expected");
    // Go asserts exactly one static-info call and never reaches the snapshot
    // when the primary read answers a usable lot size.
    assert_eq!(static_calls.load(Ordering::SeqCst), 1);
    assert_eq!(snapshot_calls.load(Ordering::SeqCst), 0);
    assert_eq!(result.rules.len(), 1);
    assert_eq!(result.rules[0].symbol, "HK.00700");
    assert_eq!(result.rules[0].lot_size, Some(100));
}

#[test]
fn market_rules_fall_back_to_security_snapshot_lot_size_and_report_the_primary_error() {
    // Parity: go:452dea11:pkg/futu/adapter_bridge_test.go:155
    // TestBrokerAdapterQueryMarketRulesFallsBackToSecuritySnapshotLotSize.
    // The warning must name the snapshot fallback and carry the primary
    // failure text so callers can distinguish a degraded read.
    let (reader, static_calls, snapshot_calls) = reader(
        StubResult::Err("未知的协议ID".to_owned()),
        StubResult::Ok(vec![snapshot("HK.00700", 100)]),
    );
    let result = reader.query(&["HK.00700".to_owned()]).expect("fallback");
    // Go asserts exactly one call to each source on the fallback path.
    assert_eq!(static_calls.load(Ordering::SeqCst), 1);
    assert_eq!(snapshot_calls.load(Ordering::SeqCst), 1);
    assert_eq!(result.rules.len(), 1);
    assert_eq!(result.rules[0].lot_size, Some(100));
    assert_eq!(result.warnings.len(), 1);
    assert!(
        result.warnings[0].contains("QuerySecuritySnapshot fallback"),
        "warning = {:?}",
        result.warnings[0]
    );
    assert!(
        result.warnings[0].contains("未知的协议ID"),
        "warning = {:?}",
        result.warnings[0]
    );
}

#[test]
fn market_rules_reject_empty_symbol_queries_before_touching_opend() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:150
    // `QueryMarketRules(empty symbols)` must fail before any provider read.
    let (reader, static_calls, snapshot_calls) =
        reader(StubResult::Ok(Vec::new()), StubResult::Ok(Vec::new()));
    assert!(matches!(
        reader.query(&[]),
        Err(MarketRulesQueryError::EmptySymbols)
    ));
    // The empty query is rejected before any provider read is attempted.
    assert_eq!(static_calls.load(Ordering::SeqCst), 0);
    assert_eq!(snapshot_calls.load(Ordering::SeqCst), 0);
}

#[test]
fn market_rules_ignore_blank_symbols_and_non_positive_lots() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:180
    // TestMarketDataRuleHelpersRejectIncompleteBrokerPayloads.
    let (reader, _, _) = reader(
        StubResult::Ok(vec![
            SecurityInfoItem {
                symbol: " ".to_owned(),
                lot_size: Some(100),
            },
            SecurityInfoItem {
                symbol: "HK.00700".to_owned(),
                lot_size: None,
            },
            SecurityInfoItem {
                symbol: "HK.00700".to_owned(),
                lot_size: Some(100),
            },
            SecurityInfoItem {
                symbol: "US.NVDA".to_owned(),
                lot_size: Some(0),
            },
        ]),
        StubResult::Ok(Vec::new()),
    );
    let result = reader.query(&["HK.00700".to_owned()]).expect("rules");
    assert_eq!(result.rules.len(), 1);
    assert_eq!(result.rules[0].symbol, "HK.00700");
    assert_eq!(result.rules[0].lot_size, Some(100));
}

#[test]
fn market_rules_report_no_rules_when_both_sources_are_empty() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:176
    let (reader, static_calls, snapshot_calls) =
        reader(StubResult::Ok(Vec::new()), StubResult::Ok(Vec::new()));
    assert!(matches!(
        reader.query(&["HK.00700".to_owned()]),
        Err(MarketRulesQueryError::NoRules)
    ));
    // Both sources are consulted once before the no-rules error is raised.
    assert_eq!(static_calls.load(Ordering::SeqCst), 1);
    assert_eq!(snapshot_calls.load(Ordering::SeqCst), 1);
}

#[test]
fn market_rules_preserve_the_snapshot_only_failure_when_primary_succeeds_empty() {
    // Parity: go:452dea11:pkg/futu/quote_snapshot_adapter_boundaries_test.go:66
    let (reader, _, _) = reader(
        StubResult::Ok(Vec::new()),
        StubResult::Err("snapshot failure".to_owned()),
    );
    match reader.query(&["HK.00700".to_owned()]) {
        Err(MarketRulesQueryError::Snapshot(message)) => {
            assert_eq!(message, "snapshot failure");
        }
        other => panic!("expected snapshot-only failure, got {other:?}"),
    }
}

#[test]
fn market_rules_combine_primary_and_fallback_failures() {
    // Parity: go:452dea11:pkg/futu/adapter_marketdata_reader.go:648.  Go wraps
    // both errors so a caller cannot mistake the degraded path for success.
    let (reader, _, _) = reader(
        StubResult::Err("static metadata unavailable".to_owned()),
        StubResult::Err("snapshot failure".to_owned()),
    );
    match reader.query(&["HK.00700".to_owned()]) {
        Err(MarketRulesQueryError::FallbackFailed { primary, fallback }) => {
            assert_eq!(primary, "static metadata unavailable");
            assert_eq!(fallback, "snapshot failure");
        }
        other => panic!("expected combined failure, got {other:?}"),
    }
}

#[test]
fn market_rules_fallback_empty_keeps_the_primary_error_in_the_message() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:153
    // TestMarketRuleFallbacksExplainTheirSourceAndFailures.
    //
    // When static info fails *and* the snapshot fallback answers nothing, Go
    // wraps the primary error and appends "returned no market rules" so the
    // caller cannot mistake the degraded path for a successful empty read.
    let (reader, static_calls, snapshot_calls) = reader(
        StubResult::Err("static metadata unavailable".to_owned()),
        StubResult::Ok(Vec::new()),
    );
    match reader.query(&["HK.00700".to_owned()]) {
        Err(MarketRulesQueryError::FallbackEmpty { primary }) => {
            assert_eq!(primary, "static metadata unavailable");
        }
        other => panic!("expected fallback-empty failure, got {other:?}"),
    }
    assert_eq!(static_calls.load(Ordering::SeqCst), 1);
    assert_eq!(snapshot_calls.load(Ordering::SeqCst), 1);
    assert!(
        MarketRulesQueryError::FallbackEmpty {
            primary: "static metadata unavailable".to_owned(),
        }
        .to_string()
        .contains("returned no market rules"),
        "the combined message must name the empty fallback"
    );
}

#[test]
fn market_rules_reject_invalid_symbols_before_touching_opend() {
    // Parity: go:452dea11:pkg/futu/marketdata_reader_boundaries_test.go:17
    // `QuerySecurityInfo(BAD)` must fail instead of silently querying OpenD.
    // Rust validates the `MARKET.CODE` pair in `parse_security` before the
    // static-info request is encoded.
    for symbol in ["BAD", "HK.", "MARS.AAPL", ""] {
        assert!(
            parse_security(symbol).is_err(),
            "symbol {symbol:?} must be rejected"
        );
    }
    let security = parse_security(" us.aapl ").expect("valid symbol");
    assert_eq!(security.market, 11);
    assert_eq!(security.code, "AAPL");
}

#[test]
fn market_rules_skip_snapshot_rows_with_blank_symbols_or_invalid_lot_sizes() {
    // Parity: go:452dea11:pkg/futu/adapter_marketdata_reader.go:658
    // `marketRulesFromSecuritySnapshot` drops rows without a usable lot size.
    let (reader, _, _) = reader(
        StubResult::Err("primary failed".to_owned()),
        StubResult::Ok(vec![
            snapshot(" ", 100),
            snapshot("HK.00700", 0),
            snapshot("HK.00700", 100),
        ]),
    );
    let result = reader.query(&["HK.00700".to_owned()]).expect("fallback");
    assert_eq!(result.rules.len(), 1);
    assert_eq!(result.rules[0].symbol, "HK.00700");
}
