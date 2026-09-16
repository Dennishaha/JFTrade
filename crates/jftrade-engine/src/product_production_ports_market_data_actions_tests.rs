use super::*;

fn action_request(body: &[u8]) -> MarketDataProviderActionsRequest {
    MarketDataProviderActionsRequest {
        method: "POST".to_owned(),
        path: NORMALIZE_INSTRUMENT_PATH.to_owned(),
        query: String::new(),
        body: body.to_vec(),
    }
}

#[tokio::test]
async fn normalize_instrument_resolves_cn_market_with_prefix_inference() {
    let port = ProductionMarketDataProviderActionsPort::new(None);

    // 1. Symbol 600519 with market CN resolves to SH.600519
    let req = action_request(br#"{"market":"CN","symbol":"600519"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");
    assert_eq!(val["symbol"], "SH.600519");

    // 2. Symbol 000001 with market CN resolves to SZ.000001
    let req = action_request(br#"{"market":"CN","symbol":"000001"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "000001");
    assert_eq!(val["instrumentId"], "SZ.000001");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SZ");
    assert_eq!(val["resolvedMarket"], "CN");
    assert_eq!(val["symbol"], "SZ.000001");

    // 3. ChiNext 300750 with market CN resolves to SZ.300750
    let req = action_request(br#"{"market":"CN","code":"300750"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "300750");
    assert_eq!(val["instrumentId"], "SZ.300750");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SZ");
    assert_eq!(val["resolvedMarket"], "CN");

    // 4. STAR Market 688981 with market CN resolves to SH.688981
    let req = action_request(br#"{"market":"CN","instrumentId":"688981"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "688981");
    assert_eq!(val["instrumentId"], "SH.688981");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");
}

#[tokio::test]
async fn normalize_instrument_resolves_qualified_sh_and_sz_prefixes() {
    let port = ProductionMarketDataProviderActionsPort::new(None);

    // Dotted SH.600519 without market parameter
    let req = action_request(br#"{"symbol":"SH.600519"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");

    // Dotted SZ.000001 without market parameter
    let req = action_request(br#"{"symbol":"SZ.000001"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "000001");
    assert_eq!(val["instrumentId"], "SZ.000001");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SZ");
    assert_eq!(val["resolvedMarket"], "CN");

    // Alias prefix CNSH.600519
    let req = action_request(br#"{"symbol":"CNSH.600519"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");

    // Alias prefix CNSZ.000001
    let req = action_request(br#"{"symbol":"CNSZ.000001"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "000001");
    assert_eq!(val["instrumentId"], "SZ.000001");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SZ");
    assert_eq!(val["resolvedMarket"], "CN");
}

#[tokio::test]
async fn normalize_instrument_handles_us_and_hk() {
    let port = ProductionMarketDataProviderActionsPort::new(None);

    let req = action_request(br#"{"market":"US","symbol":"AAPL"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "AAPL");
    assert_eq!(val["instrumentId"], "US.AAPL");
    assert_eq!(val["market"], "US");
    assert_eq!(val["prefix"], "US");
    assert_eq!(val["resolvedMarket"], "US");

    let req = action_request(br#"{"symbol":"HK.00700"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "00700");
    assert_eq!(val["instrumentId"], "HK.00700");
    assert_eq!(val["market"], "HK");
    assert_eq!(val["prefix"], "HK");
    assert_eq!(val["resolvedMarket"], "HK");
}

#[tokio::test]
async fn normalize_instrument_error_cases_match_go_fixture_semantics() {
    let port = ProductionMarketDataProviderActionsPort::new(None);

    // Empty object
    let req = action_request(b"{}");
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, message, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
            assert_eq!(message, "symbol or code is required");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Null body
    let req = action_request(b"null");
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, message, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
            assert_eq!(message, "symbol or code is required");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Invalid JSON
    let req = action_request(b"{");
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, message, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "BAD_REQUEST");
            assert_eq!(message, "invalid normalize request");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Empty symbol
    let req = action_request(br#"{"market":"US","symbol":""}"#);
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, message, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
            assert_eq!(message, "symbol or code is required");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Symbol with prefix takes precedence over bare code
    let req = action_request(br#"{"symbol":"SH.600519","code":"600519"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");

    // Market mismatch returns MARKET_INSTRUMENT_INVALID
    let req = action_request(br#"{"market":"US","symbol":"SH.600519"}"#);
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Unsupported market returns MARKET_INSTRUMENT_INVALID
    let req = action_request(br#"{"market":"INVALID","symbol":"12345"}"#);
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
        }
        other => panic!("unexpected error: {other:?}"),
    }

    // Cross-exchange mismatch within CN returns MARKET_INSTRUMENT_INVALID
    let req = action_request(br#"{"market":"SH","symbol":"SZ.000001"}"#);
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
        }
        other => panic!("unexpected error: {other:?}"),
    }
    let req = action_request(br#"{"market":"SZ","symbol":"SH.600519"}"#);
    let err = port.dispatch(&req).await.unwrap_err();
    match err {
        MarketDataProviderActionsPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "MARKET_INSTRUMENT_INVALID");
        }
        other => panic!("unexpected error: {other:?}"),
    }
}

#[tokio::test]
async fn normalize_instrument_supports_suffix_formats_and_aliases() {
    let port = ProductionMarketDataProviderActionsPort::new(None);

    // Suffix format 600519.SH
    let req = action_request(br#"{"symbol":"600519.SH"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["resolvedMarket"], "CN");

    // Suffix format 000001.SZ
    let req = action_request(br#"{"symbol":"000001.SZ"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "000001");
    assert_eq!(val["instrumentId"], "SZ.000001");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SZ");

    // Suffix format 00700.HK
    let req = action_request(br#"{"symbol":"00700.HK"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "00700");
    assert_eq!(val["instrumentId"], "HK.00700");
    assert_eq!(val["market"], "HK");
    assert_eq!(val["prefix"], "HK");

    // Suffix format AAPL.US
    let req = action_request(br#"{"symbol":"AAPL.US"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "AAPL");
    assert_eq!(val["instrumentId"], "US.AAPL");
    assert_eq!(val["market"], "US");
    assert_eq!(val["prefix"], "US");

    // Yahoo Finance SS suffix
    let req = action_request(br#"{"symbol":"600519.SS"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");
    assert_eq!(val["market"], "CN");
    assert_eq!(val["prefix"], "SH");

    // Snake case instrument_id alias
    let req = action_request(br#"{"instrument_id":"SH.600519"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "600519");
    assert_eq!(val["instrumentId"], "SH.600519");

    // Shanghai composite index code 000001 with SH prefix must stay SH
    let req = action_request(br#"{"symbol":"SH.000001"}"#);
    let val = port.dispatch(&req).await.unwrap();
    assert_eq!(val["code"], "000001");
    assert_eq!(val["instrumentId"], "SH.000001");
    assert_eq!(val["prefix"], "SH");
    assert_eq!(val["market"], "CN");
}

/// Parity: go:452dea11:pkg/futu/adapter_option_fix_test.go:12
/// TestFutuRootAdapterForwardsBatchSnapshotsThroughProtocol3203
///
/// Go's root adapter forwards a batch snapshot request through protocol 3203
/// exactly once. Rust's batch route fans a single instrument out to the
/// snapshot read port (`/api/v1/market-data/snapshots/{market}/{symbol}`),
/// which is the production owner of the Qot_GetSecuritySnapshot(3203) read;
/// the counted port below asserts the same single 3203-backed call and that the
/// requested instrument is named verbatim.
#[derive(Debug, Default)]
struct CountingBatchSnapshotQuotePort {
    calls: std::sync::Mutex<Vec<String>>,
}

impl MarketDataQuoteReadSnapshotPort for CountingBatchSnapshotQuotePort {
    fn read<'a>(
        &'a self,
        path: &'a str,
        _query: &'a str,
    ) -> crate::product::MarketDataQuoteReadFuture<'a> {
        self.calls
            .lock()
            .expect("batch snapshot calls")
            .push(path.to_owned());
        Box::pin(async move {
            Ok(json!({
                "snapshot": {
                    "price": "118.40",
                    "observedAt": "2026-08-29T14:30:00Z",
                },
            }))
        })
    }
}

#[tokio::test]
async fn batch_snapshots_forward_each_instrument_through_the_snapshot_reader_once() {
    let quote_port = Arc::new(CountingBatchSnapshotQuotePort::default());
    let port = ProductionMarketDataProviderActionsPort::new(Some(quote_port.clone()));
    let request = MarketDataProviderActionsRequest {
        method: "POST".to_owned(),
        path: BATCH_SNAPSHOTS_PATH.to_owned(),
        query: String::new(),
        body: br#"{"symbols":["HK.00700"]}"#.to_vec(),
    };
    let value = port.dispatch(&request).await.expect("batch snapshots");
    assert_eq!(value["entries"].as_array().map(Vec::len), Some(1));
    assert_eq!(value["entries"][0]["symbol"], "HK.00700");
    assert_eq!(value["snapshots"]["HK.00700"]["price"], 118.40);
    assert_eq!(
        value["metadata"]["requestedSymbols"][0],
        serde_json::json!("HK.00700")
    );
    let calls = quote_port.calls.lock().expect("batch snapshot calls").clone();
    assert_eq!(
        calls,
        vec!["/api/v1/market-data/snapshots/HK/00700".to_owned()],
        "the batch request must forward exactly one snapshot read per instrument"
    );
}
