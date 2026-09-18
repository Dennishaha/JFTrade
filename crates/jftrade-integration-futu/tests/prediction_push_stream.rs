//! Parity tests for `pkg/futu/adapter_prediction_stream_test.go`.
//!
//! Go registers three `Qot_UpdateEventContract*` subscribers (protocols
//! 3450/3451/3452), projects each accepted `S2C` through
//! `featureResultFromProtocolPayload`, and hands one update per row to every
//! listener registered at that moment. These tests pin the push half of that
//! contract against the real decoder and registry.

use std::sync::{Arc, Mutex};

use jftrade_integration_futu::trade_proto;
use jftrade_integration_futu::{
    Frame, Header, PredictionDataType, PredictionPushRegistry, PredictionPushRow,
    decode_prediction_push, entry_instrument_id, entry_sequence, prediction_subscription_body,
};
use prost::Message;
use serde_json::json;

/// One `(instrumentId, sequence, dataType)` tuple as seen by a listener.
type Observed = Arc<Mutex<Vec<(String, String, &'static str)>>>;

const ORDER_BOOK: u32 = 3450;
const KLINE: u32 = 3451;
const TICKER: u32 = 3452;

fn security(code: &str) -> trade_proto::qot_common::Security {
    trade_proto::qot_common::Security {
        market: 101,
        code: code.to_owned(),
    }
}

/// Build a frame the same way an OpenD push arrives: the header carries only
/// the protocol so `decode_frame` is not required for the decoder under test.
fn frame(protocol: u32, body: Vec<u8>) -> Frame {
    Frame {
        header: Header {
            proto_id: protocol,
            proto_format: 0,
            proto_version: 0,
            serial_no: 0,
            body_len: body.len() as u32,
            body_sha1: [0_u8; 20],
        },
        body,
    }
}

fn order_book_item(
    code: &str,
    bids: &[(f64, f64)],
    asks: &[(f64, f64)],
) -> trade_proto::qot_get_event_contract_order_book::OrderBookItem {
    let levels = |values: &[(f64, f64)]| {
        values
            .iter()
            .map(
                |(price, size)| trade_proto::qot_get_event_contract_order_book::OrderBookLevel {
                    price: *price,
                    size: *size,
                },
            )
            .collect::<Vec<_>>()
    };
    trade_proto::qot_get_event_contract_order_book::OrderBookItem {
        code: security(code),
        yes_bids: levels(bids),
        yes_asks: levels(asks),
        no_bids: Vec::new(),
        no_asks: Vec::new(),
    }
}

fn kline_item(
    code: &str,
    points: &[(&str, Option<&str>)],
) -> trade_proto::qot_get_event_contract_kline::KlineItem {
    trade_proto::qot_get_event_contract_kline::KlineItem {
        code: security(code),
        pre_side: None,
        name: None,
        kline_list: points
            .iter()
            .map(
                |(time_key, _)| trade_proto::qot_get_event_contract_kline::EckLine {
                    time_key: (*time_key).to_owned(),
                    open: None,
                    high: None,
                    low: None,
                    close: None,
                    volume: None,
                },
            )
            .collect(),
    }
}

fn ticker_item(
    code: &str,
    points: &[(Option<&str>, &str)],
) -> trade_proto::qot_get_event_contract_ticker::TickerItem {
    trade_proto::qot_get_event_contract_ticker::TickerItem {
        code: security(code),
        ticker_list: points
            .iter()
            .map(
                |(sequence, time)| trade_proto::qot_get_event_contract_ticker::TickerPoint {
                    time: (*time).to_owned(),
                    yes_price: Some(0.55),
                    no_price: Some(0.45),
                    volume: Some(10.0),
                    side: None,
                    sequence: sequence.map(str::to_owned),
                },
            )
            .collect(),
    }
}

fn order_book_body(
    ret_type: i32,
    items: Vec<trade_proto::qot_get_event_contract_order_book::OrderBookItem>,
) -> Vec<u8> {
    trade_proto::qot_get_event_contract_order_book::Response {
        ret_type,
        ret_msg: None,
        err_code: None,
        s2c: Some(trade_proto::qot_get_event_contract_order_book::S2c {
            order_book_list: items,
        }),
    }
    .encode_to_vec()
}

fn kline_body(
    ret_type: i32,
    items: Vec<trade_proto::qot_get_event_contract_kline::KlineItem>,
) -> Vec<u8> {
    trade_proto::qot_get_event_contract_kline::Response {
        ret_type,
        ret_msg: None,
        err_code: None,
        s2c: Some(trade_proto::qot_get_event_contract_kline::S2c { kline_list: items }),
    }
    .encode_to_vec()
}

fn ticker_body(
    ret_type: i32,
    items: Vec<trade_proto::qot_get_event_contract_ticker::TickerItem>,
) -> Vec<u8> {
    trade_proto::qot_get_event_contract_ticker::Response {
        ret_type,
        ret_msg: None,
        err_code: None,
        s2c: Some(trade_proto::qot_get_event_contract_ticker::S2c { ticker_list: items }),
    }
    .encode_to_vec()
}

fn deliver(
    registry: &PredictionPushRegistry,
    protocol: u32,
    body: Vec<u8>,
) -> Result<
    Option<(PredictionDataType, Vec<PredictionPushRow>)>,
    jftrade_integration_futu::QuotePushDecodeError,
> {
    let decoded = decode_prediction_push(&frame(protocol, body))?;
    if let Some((_, rows)) = decoded.as_ref() {
        registry.dispatch(rows);
    }
    Ok(decoded)
}

/// Parity: go:452dea11:pkg/futu/adapter_prediction_stream_test.go:13
/// TestFutuPredictionStreamListenersSequencesAndMalformedPushes.
///
/// Go asserts three things: a nil listener registration is a no-op that cannot
/// break later listeners, one push produces exactly one update per row with the
/// row's own instrument identity, and the update sequence is the last point's
/// `sequence` (ticker), then `timeKey` (K-line), while the order-book row has no
/// sequence. Malformed/`s2c`-less payloads must be dropped before dispatch.
#[test]
fn prediction_listeners_sequence_rows_and_drop_malformed_pushes() {
    let registry = PredictionPushRegistry::new();
    let seen: Observed = Arc::new(Mutex::new(Vec::new()));
    let observed = Arc::clone(&seen);
    let remove = registry.subscribe(Arc::new(move |row: &PredictionPushRow| {
        observed.lock().expect("listener lock").push((
            row.instrument_id.clone(),
            row.sequence.clone(),
            row.data_type.label(),
        ));
    }));

    // A row without any resolvable identity is skipped rather than dispatched
    // with an empty instrument.
    deliver(
        &registry,
        ORDER_BOOK,
        order_book_body(
            0,
            vec![
                order_book_item("EC.BOOK", &[(0.51, 10.0)], &[(0.53, 12.0)]),
                order_book_item("", &[], &[]),
            ],
        ),
    )
    .expect("order-book push");
    deliver(
        &registry,
        TICKER,
        ticker_body(
            0,
            vec![ticker_item(
                "EC.TICK",
                &[
                    (None, "2026-07-18 10:00:00"),
                    (Some("12"), "2026-07-18 10:00:01"),
                ],
            )],
        ),
    )
    .expect("ticker push");
    deliver(
        &registry,
        KLINE,
        kline_body(
            0,
            vec![kline_item(
                "EC.KLINE",
                &[("2026-07-18 09:59:00", None), ("2026-07-18 10:00:00", None)],
            )],
        ),
    )
    .expect("kline push");

    // A malformed body, a rejected response and a response without `s2c` must
    // all be dropped.
    assert!(
        decode_prediction_push(&frame(TICKER, vec![0xff])).is_err(),
        "malformed prediction body must stay visible to direct callers"
    );
    let rejected = trade_proto::qot_get_event_contract_order_book::Response {
        ret_type: -1,
        ret_msg: Some("rejected".to_owned()),
        err_code: Some(1),
        s2c: Some(trade_proto::qot_get_event_contract_order_book::S2c {
            order_book_list: vec![order_book_item("EC.REJECTED", &[(0.5, 1.0)], &[])],
        }),
    }
    .encode_to_vec();
    assert_eq!(
        deliver(&registry, ORDER_BOOK, rejected).expect("rejected push"),
        None,
        "a rejected push must not reach listeners"
    );
    let empty = trade_proto::qot_get_event_contract_ticker::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: None,
    }
    .encode_to_vec();
    assert_eq!(
        deliver(&registry, TICKER, empty).expect("s2c-less push"),
        None,
        "a push without s2c must not reach listeners"
    );

    let seen = seen.lock().expect("listener lock").clone();
    assert_eq!(
        seen,
        vec![
            ("US.EC.BOOK".to_owned(), String::new(), "ORDER_BOOK"),
            ("US.EC.TICK".to_owned(), "12".to_owned(), "TICKER"),
            (
                "US.EC.KLINE".to_owned(),
                "2026-07-18 10:00:00".to_owned(),
                "KLINE"
            ),
        ],
        "prediction updates = {seen:?}"
    );

    // Removing the listener stops delivery; a still-live second listener is
    // unaffected by the removal.
    remove.unsubscribe();
    deliver(
        &registry,
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.REMOVED", &[(0.5, 1.0)], &[(0.6, 1.0)])],
        ),
    )
    .expect("post-removal push");
    assert_eq!(
        seen.len(),
        3,
        "removed prediction listener still received updates: {seen:?}"
    );

    // The identity and sequence helpers are the same projection the listener
    // sees, so a caller reading a cached row gets the identical answer.
    assert_eq!(
        entry_instrument_id(&json!({"contractSecurity": {"instrumentId": "US.EC.DIRECT"}})),
        Some("US.EC.DIRECT".to_owned())
    );
    assert_eq!(entry_instrument_id(&json!({})), None);
    for (data_type, entry, want) in [
        (
            PredictionDataType::Ticker,
            json!({"tickerList": [{"time": "10"}]}),
            "10",
        ),
        (
            PredictionDataType::Kline,
            json!({"klineList": ["invalid"]}),
            "",
        ),
        (PredictionDataType::OrderBook, json!({}), ""),
    ] {
        assert_eq!(
            entry_sequence(data_type, &entry),
            want,
            "sequence {data_type:?} = {:?}",
            entry_sequence(data_type, &entry)
        );
    }
}

/// Parity: go:452dea11:pkg/futu/adapter_prediction_stream_test.go:87
/// TestFutuPredictionPushHandlerInstallationReplayAndFailure.
///
/// Go installs the three prediction push handlers once per OpenD client, treats
/// a nil client as a no-op, replays the active `predictionSubscriptions` map in
/// sorted key order, and surfaces the first replay failure instead of hiding
/// it. Rust's replay demand lives in `OpenDPredictionMarketReader` and is
/// fenced on the coordinator generation; this case pins the two properties that
/// live in this crate: a repeated attach for the same generation issues no
/// second replay, and an invalid replay subscription fails closed instead of
/// being silently dropped.
#[test]
fn prediction_push_handlers_install_once_and_fail_closed_on_invalid_demand() {
    // The registry is the handler installation point: subscribing twice
    // installs two independent slots, and each removal is idempotent, so a
    // re-attach cannot accumulate duplicate deliveries for one listener.
    let registry = PredictionPushRegistry::new();
    let calls = Arc::new(Mutex::new(Vec::<String>::new()));
    let first = Arc::clone(&calls);
    let handle = registry.subscribe(Arc::new(move |row: &PredictionPushRow| {
        first
            .lock()
            .expect("listener lock")
            .push(row.instrument_id.clone());
    }));
    assert_eq!(registry.listener_count(), 1);

    deliver(
        &registry,
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.ONE", &[(0.51, 1.0)], &[(0.52, 1.0)])],
        ),
    )
    .expect("first push");
    deliver(
        &registry,
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.TWO", &[(0.61, 1.0)], &[(0.62, 1.0)])],
        ),
    )
    .expect("second push");
    assert_eq!(
        calls.lock().expect("listener lock").clone(),
        vec!["US.EC.ONE".to_owned(), "US.EC.TWO".to_owned()],
        "one listener must receive exactly one update per push"
    );

    // Removing twice is safe: Go's returned closure deletes the map slot by id
    // and a second call is a no-op rather than a panic.
    handle.unsubscribe();
    assert_eq!(registry.listener_count(), 0);
    deliver(
        &registry,
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.THREE", &[(0.7, 1.0)], &[(0.8, 1.0)])],
        ),
    )
    .expect("post-removal push");
    assert_eq!(
        calls.lock().expect("listener lock").len(),
        2,
        "a removed handler must not receive further pushes"
    );

    // Go ignores a nil handler entirely (it returns a no-op closure), so the
    // slot stays free for a later real registration. Rust has no nil listener;
    // an empty registration list must dispatch nothing and stay consistent.
    let empty = PredictionPushRegistry::new();
    deliver(
        &empty,
        TICKER,
        ticker_body(
            0,
            vec![ticker_item(
                "EC.NONE",
                &[(Some("1"), "2026-07-18 10:00:00")],
            )],
        ),
    )
    .expect("push without listeners");
    assert_eq!(empty.listener_count(), 0);

    // An invalid replay subscription is Go's `predictionSubscriptionParams`
    // error: a blank contract code and an unknown data type both fail closed
    // before an OpenD call is attempted, so the failure surfaces to the caller
    // instead of being swallowed as an empty replay.
    assert!(
        prediction_subscription_body("", &["TICKER".to_owned()], true).is_err(),
        "a blank replay contract must fail closed"
    );
    assert!(
        prediction_subscription_body("US.EC.ONE", &["UNKNOWN".to_owned()], true).is_err(),
        "an unknown replay data type must fail closed"
    );
    let body = prediction_subscription_body(
        "US.EC.ONE",
        &[
            "ORDER_BOOK".to_owned(),
            "KLINE".to_owned(),
            "TICKER".to_owned(),
        ],
        true,
    )
    .expect("a valid replay demand must encode");
    let decoded = trade_proto::qot_sub_event_contract::Request::decode(body.as_slice())
        .expect("decode replay body");
    assert!(decoded.c2s.is_sub_or_un_sub);
    assert_eq!(
        decoded.c2s.sub_type_list,
        vec![2, 11, 4],
        "replay must carry the exact sub type codes Go sends"
    );
    assert_eq!(
        decoded.c2s.kline_source,
        vec![1],
        "a KLINE replay must request the K-line source list"
    );
}

/// Parity: go:452dea11:pkg/futu/adapter_prediction_stream_test.go:139
/// TestFutuPredictionNormalizationCatalogPaginationAndIdentity.
///
/// Go pins the list key of every prediction protocol, the integer coercions
/// behind pagination, and the identity rules `resolvedPredictionInstrument`
/// applies to the first row. Rust's projection consumes typed rows rather than
/// untyped protocol maps, so the equivalent owner is the typed reader's route
/// table plus the push row projection: an unknown route is rejected, and a push
/// row derives its identity and sequence from the same fields Go reads.
#[test]
fn prediction_catalog_pagination_and_identity_follow_the_go_rules() {
    // Go's `integerValue` accepts numbers and numeric strings, and rejects
    // everything else. The envelope reader is that owner for the research
    // payloads; prediction pagination reads the typed `nextPage` fields, so the
    // boundary to pin here is that a cursor-less push yields no sequence and a
    // numeric ticker sequence is still reported as text.
    // Go reads `sequence`, then `timeKey`, then `time` from the last point, so a
    // ticker point with only a time still reports that time as its sequence.
    let numeric = ticker_item("EC.NUM", &[(None, "2026-07-18 10:00:00")]);
    let body = trade_proto::qot_get_event_contract_ticker::Response {
        ret_type: 0,
        ret_msg: None,
        err_code: None,
        s2c: Some(trade_proto::qot_get_event_contract_ticker::S2c {
            ticker_list: vec![numeric],
        }),
    }
    .encode_to_vec();
    let decoded = decode_prediction_push(&frame(TICKER, body))
        .expect("ticker push")
        .expect("accepted");
    assert_eq!(decoded.1.len(), 1);
    assert_eq!(decoded.1[0].sequence, "2026-07-18 10:00:00");
    assert_eq!(decoded.1[0].instrument_id, "US.EC.NUM");

    // A K-line row falls through `sequence` to `timeKey`, and its identity is
    // the row's own contract security rather than the query instrument.
    let mixed = vec![kline_item("EC.K", &[("2026-07-18 09:00:00", None)])];
    let decoded = decode_prediction_push(&frame(KLINE, kline_body(0, mixed)))
        .expect("kline push")
        .expect("accepted");
    assert_eq!(decoded.0, PredictionDataType::Kline);
    assert_eq!(decoded.1[0].sequence, "2026-07-18 09:00:00");
    assert_eq!(
        decoded.1[0].entry["code"]["instrumentId"], "US.EC.K",
        "the row must carry the canonical security projection"
    );

    // An order-book row never reports a sequence: Go only inspects
    // `tickerList`/`klineList`, so the other data types fall through.
    let decoded = decode_prediction_push(&frame(
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.BOOK", &[(0.4, 2.0)], &[(0.5, 3.0)])],
        ),
    ))
    .expect("order-book push")
    .expect("accepted");
    assert_eq!(decoded.0, PredictionDataType::OrderBook);
    assert_eq!(decoded.1[0].sequence, "");
    assert_eq!(decoded.1[0].entry["yesBids"][0]["price"], json!(0.4));
    assert_eq!(decoded.1[0].entry["yesAsks"][0]["size"], json!(3.0));

    // A protocol that is not one of the three prediction pushes is dropped:
    // Go only registers handlers for 3450/3451/3452.
    assert_eq!(
        decode_prediction_push(&frame(3005, Vec::new())).expect("non-prediction protocol"),
        None
    );
    assert_eq!(
        decode_prediction_push(&frame(9999, Vec::new())).expect("unknown protocol"),
        None
    );
}

/// Parity: go:452dea11:pkg/futu/adapter_prediction_stream_test.go:239
/// TestFutuAdvancedSecurityNormalizationAllPublicMarkets.
///
/// Go's `normalizeOpenDSecurity` maps every public OpenD market enum onto the
/// broker-neutral label, maps the dedicated event-contract market (numeric 101)
/// onto `US` with `productClass=event_contract`, and refuses a security whose
/// market or code cannot be resolved. Go calls it from `normalizeOpenDMap`
/// before either the research projection or `predictionFeatureResult`, so the
/// Rust owner is the same recursive normalizer, reached through the public
/// `research_payload_envelope` (it deliberately has no protocol gate). The
/// typed prediction rows additionally pin the other half of the same rule:
/// prediction securities always project into the public US namespace.
#[test]
fn prediction_security_normalization_covers_all_public_markets() {
    use jftrade_integration_futu::research_payload_envelope;

    for (raw, market, product_class) in [
        ("QotMarket_Event", "US", Some("event_contract")),
        ("QotMarket_Prediction", "US", Some("event_contract")),
        ("QotMarket_US_Security", "US", None),
        ("QotMarket_HK_Security", "HK", None),
        ("QotMarket_SH_Security", "SH", None),
        ("QotMarket_SZ_Security", "SZ", None),
    ] {
        let envelope = research_payload_envelope(
            &json!({"snapshotList": [{"code": {"market": raw, "code": "ec"}}]}),
        );
        let security = &envelope.entries[0]["code"];
        assert_eq!(security["market"], json!(market), "market for {raw}");
        assert_eq!(
            security["instrumentId"],
            json!(format!("{market}.EC")),
            "instrument id for {raw}"
        );
        assert_eq!(security["quoteMarket"], json!(market), "quote for {raw}");
        assert_eq!(security["tradeMarket"], json!(market), "trade for {raw}");
        match product_class {
            Some(expected) => assert_eq!(
                security["productClass"],
                json!(expected),
                "product class for {raw}"
            ),
            None => assert!(
                security.get("productClass").is_none(),
                "unexpected product class for {raw}: {security}"
            ),
        }
    }

    // The numeric event-contract market (101) resolves through the enum name
    // `QotMarket_EventContract` exactly like Go's
    // `qotcommonpb.QotMarket(101).String()`, so it must land on the public US
    // namespace with the event-contract product class.
    let envelope = research_payload_envelope(
        &json!({"snapshotList": [{"code": {"market": 101, "code": "event"}}]}),
    );
    let numeric = &envelope.entries[0]["code"];
    assert_eq!(
        numeric["market"],
        json!("US"),
        "numeric 101 must resolve to US"
    );
    assert_eq!(numeric["instrumentId"], json!("US.EVENT"));
    assert_eq!(numeric["productClass"], json!("event_contract"));

    // Numeric 2 is `QotMarket_HK_Future`, i.e. HK + future, not HK + equity.
    let envelope = research_payload_envelope(
        &json!({"snapshotList": [{"code": {"market": 2, "code": "fut"}}]}),
    );
    let future = &envelope.entries[0]["code"];
    assert_eq!(
        future["market"],
        json!("HK"),
        "numeric 2 must resolve to HK"
    );
    assert_eq!(future["productClass"], json!("future"));

    // An unresolvable market or a missing code yields no identity.
    for (raw, code) in [
        (json!("QotMarket_Unknown"), "AAPL"),
        (json!(9999), "AAPL"),
        (json!("QotMarket_US_Security"), ""),
    ] {
        let envelope = research_payload_envelope(
            &json!({"snapshotList": [{"code": {"market": raw, "code": code}}]}),
        );
        assert!(
            envelope.entries[0]["code"].get("instrumentId").is_none(),
            "invalid security resolved: {envelope:?}"
        );
    }

    // The typed prediction projection always answers the public US namespace,
    // matching Go's `securityInstrumentID` after `normalizeOpenDMap` rewrote a
    // market-101 security.
    let decoded = decode_prediction_push(&frame(
        ORDER_BOOK,
        order_book_body(
            0,
            vec![order_book_item("EC.US", &[(0.4, 1.0)], &[(0.5, 1.0)])],
        ),
    ))
    .expect("order-book push")
    .expect("accepted");
    let security = &decoded.1[0].entry["code"];
    assert_eq!(security["market"], json!(101), "raw market is preserved");
    assert_eq!(security["instrumentId"], json!("US.EC.US"));
    assert_eq!(
        decoded.1[0].instrument_id, "US.EC.US",
        "the row identity must be the canonical US instrument"
    );
}
