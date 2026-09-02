use std::sync::Arc;

use super::*;
use crate::{OpenDSubscriptionLifecycle, decode_frame, encode_frame};
use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};

fn frame(protocol: u32, body: Vec<u8>) -> Frame {
    decode_frame(&encode_frame(protocol, 7, &body).expect("frame")).expect("decode frame")
}

#[test]
fn decodes_basic_kline_and_order_book_pushes_with_go_field_semantics() {
    let basic = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(BasicQuoteS2c {
            basic_quotes: vec![WireBasicQuote {
                security: Some(WireSecurity {
                    market: Some(11),
                    code: Some("AAPL".to_owned()),
                }),
                name: Some("Apple".to_owned()),
                is_suspended: Some(false),
                list_time: Some("1980-12-12".to_owned()),
                price_spread: Some(0.01),
                update_time: Some("2026-08-24 09:30:00".to_owned()),
                high_price: Some(124.0),
                open_price: Some(122.0),
                low_price: Some(121.0),
                cur_price: Some(123.45),
                last_close_price: Some(120.0),
                volume: Some(9),
                turnover: Some(1_111.0),
                turnover_rate: Some(0.1),
                amplitude: Some(1.2),
                ..WireBasicQuote::default()
            }],
        }),
    };
    let decoded = decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, basic.encode_to_vec()))
        .expect("basic decode")
        .expect("basic push");
    assert!(matches!(
        decoded,
        QuotePush::Basic(BasicQuotePush { quotes })
            if quotes[0].security == Some(Security {
                market: Some(11),
                code: Some("AAPL".to_owned())
            }) && quotes[0].cur_price == Some(123.45)
    ));

    let kline = KlineResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(KlineS2c {
            rehab_type: Some(0),
            kl_type: Some(11),
            security: Some(WireSecurity {
                market: Some(1),
                code: Some("00700".to_owned()),
            }),
            klines: vec![WireKline {
                time: Some("2026-08-24 09:30:00".to_owned()),
                is_blank: Some(false),
                close_price: Some(380.0),
                ..WireKline::default()
            }],
            name: Some("Tencent".to_owned()),
        }),
    };
    let decoded = decode_quote_push(&frame(PROTO_UPDATE_KL, kline.encode_to_vec()))
        .expect("kline decode")
        .expect("kline push");
    assert!(matches!(
        decoded,
        QuotePush::Kline(KlinePush {
            security: Some(Security { market: Some(1), .. }),
            klines,
            ..
        }) if klines[0].close_price == Some(380.0)
    ));

    let order_book = OrderBookResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(OrderBookS2c {
            security: Some(WireSecurity {
                market: Some(1),
                code: Some("00700".to_owned()),
            }),
            asks: vec![WireOrderBook {
                price: Some(380.1),
                volume: Some(100),
                order_count: Some(2),
                ..WireOrderBook::default()
            }],
            bids: Vec::new(),
            server_receive_time_bid: None,
            server_receive_time_bid_timestamp: None,
            server_receive_time_ask: Some("09:30:00".to_owned()),
            server_receive_time_ask_timestamp: Some(1.0),
            name: Some("Tencent".to_owned()),
            order_book_type: Some(0),
        }),
    };
    let decoded = decode_quote_push(&frame(PROTO_UPDATE_ORDER_BOOK, order_book.encode_to_vec()))
        .expect("order book decode")
        .expect("order book push");
    assert!(matches!(
        decoded,
        QuotePush::OrderBook(OrderBookPush { asks, .. })
            if asks[0].price == Some(380.1) && asks[0].order_count == Some(2)
    ));
}

#[test]
fn decodes_basic_quote_extended_blocks_and_observation_time_without_dropping_fields() {
    let response = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(BasicQuoteS2c {
            basic_quotes: vec![WireBasicQuote {
                security: Some(WireSecurity {
                    market: Some(11),
                    code: Some("AAPL".to_owned()),
                }),
                is_suspended: Some(false),
                list_time: Some("1980-12-12".to_owned()),
                price_spread: Some(0.01),
                update_time: Some("2026-07-15 22:00:00".to_owned()),
                update_timestamp: Some(1_784_156_400.125),
                high_price: Some(201.0),
                open_price: Some(200.0),
                low_price: Some(199.0),
                cur_price: Some(200.0),
                last_close_price: Some(198.0),
                volume: Some(10),
                turnover: Some(1_000.0),
                turnover_rate: Some(0.1),
                amplitude: Some(1.0),
                pre_market: Some(WirePreAfterMarketData {
                    price: Some(199.5),
                    high_price: Some(200.0),
                    low_price: Some(198.5),
                    volume: Some(5),
                    turnover: Some(500.0),
                    change_value: Some(1.5),
                    change_rate: Some(0.75),
                    amplitude: Some(0.5),
                }),
                after_market: Some(WirePreAfterMarketData {
                    price: Some(202.0),
                    high_price: Some(203.0),
                    low_price: Some(198.0),
                    volume: Some(20),
                    turnover: Some(2_000.0),
                    change_value: Some(4.0),
                    change_rate: Some(2.0),
                    amplitude: Some(2.5),
                }),
                overnight: Some(WirePreAfterMarketData {
                    price: Some(201.5),
                    high_price: Some(202.0),
                    low_price: Some(200.5),
                    volume: Some(3),
                    turnover: Some(300.0),
                    change_value: Some(3.5),
                    change_rate: Some(1.75),
                    amplitude: Some(0.75),
                }),
                ..WireBasicQuote::default()
            }],
        }),
    };

    let decoded = decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, response.encode_to_vec()))
        .expect("basic decode")
        .expect("basic push");
    let QuotePush::Basic(BasicQuotePush { quotes }) = decoded else {
        panic!("expected basic quote push");
    };
    let quote = &quotes[0];
    assert_eq!(quote.update_time.as_deref(), Some("2026-07-15 22:00:00"));
    assert_eq!(quote.update_timestamp, Some(1_784_156_400.125));
    let pre_market = quote.pre_market.as_ref().expect("pre-market block");
    assert_eq!(pre_market.price, Some(199.5));
    assert_eq!(pre_market.volume, Some(5));
    let after_market = quote.after_market.as_ref().expect("after-market block");
    assert_eq!(after_market.price, Some(202.0));
    assert_eq!(after_market.turnover, Some(2_000.0));
    let overnight = quote.overnight.as_ref().expect("overnight block");
    assert_eq!(overnight.price, Some(201.5));
    assert_eq!(overnight.change_rate, Some(1.75));
}

#[test]
fn rejected_empty_and_unknown_pushes_are_dropped_like_go() {
    let rejected = BasicQuoteResponse {
        ret_type: Some(-1),
        _ret_msg: Some("denied".to_owned()),
        _err_code: Some(3),
        s2c: None,
    };
    assert_eq!(
        decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, rejected.encode_to_vec()))
            .expect("rejected decode"),
        None
    );

    let empty = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: None,
    };
    assert_eq!(
        decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, empty.encode_to_vec()))
            .expect("empty decode"),
        None
    );
    let empty_s2c = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(BasicQuoteS2c {
            basic_quotes: Vec::new(),
        }),
    };
    assert_eq!(
        decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, empty_s2c.encode_to_vec()))
            .expect("empty s2c decode"),
        None
    );
    let incomplete = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(BasicQuoteS2c {
            basic_quotes: vec![WireBasicQuote::default()],
        }),
    };
    assert_eq!(
        decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, incomplete.encode_to_vec()))
            .expect("incomplete decode"),
        None
    );
    assert_eq!(
        decode_quote_push(&frame(9999, Vec::new())).expect("unknown decode"),
        None
    );
}

#[test]
fn malformed_known_push_is_reported_for_stream_recovery() {
    assert!(matches!(
        decode_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, vec![0xff])),
        Err(QuotePushDecodeError::Decode {
            protocol: PROTO_UPDATE_BASIC_QOT,
            ..
        })
    ));
}

#[test]
fn lifecycle_accepts_active_push_preserves_recorder_failures_and_rejects_stale_push() {
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 0);
    let desired = [InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: None,
    }];
    let actions = lifecycle.reconcile_demand(&desired, 0);
    assert_eq!(actions.len(), 1);
    let generation = lifecycle.generation();
    let now: WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
    assert!(recorder.record_stream_connected(generation));
    assert!(recorder.record_quote_failure(generation, now, "old quote failure"));

    let response = BasicQuoteResponse {
        ret_type: Some(0),
        _ret_msg: None,
        _err_code: None,
        s2c: Some(BasicQuoteS2c {
            basic_quotes: vec![WireBasicQuote {
                security: Some(WireSecurity {
                    market: Some(11),
                    code: Some("AAPL".to_owned()),
                }),
                is_suspended: Some(false),
                list_time: Some("1980-12-12".to_owned()),
                price_spread: Some(0.01),
                update_time: Some("2026-08-24 09:30:00".to_owned()),
                high_price: Some(124.0),
                open_price: Some(122.0),
                low_price: Some(121.0),
                cur_price: Some(123.45),
                last_close_price: Some(120.0),
                volume: Some(9),
                turnover: Some(1_111.0),
                turnover_rate: Some(0.1),
                amplitude: Some(1.2),
                ..WireBasicQuote::default()
            }],
        }),
    };
    let push = frame(PROTO_UPDATE_BASIC_QOT, response.encode_to_vec());
    assert!(
        lifecycle
            .ingest_quote_push(&push, now, generation)
            .expect("active push")
            .is_some()
    );
    let recovered = recorder.snapshot();
    assert!(recovered.connected);
    assert_eq!(recovered.quote_failures, 1);
    assert_eq!(
        recovered.quote_last_error.as_deref(),
        Some("old quote failure")
    );

    assert_eq!(
        lifecycle
            .ingest_quote_push(&frame(PROTO_UPDATE_BASIC_QOT, vec![0xff]), now, generation,)
            .expect("malformed active push is dropped"),
        None
    );
    assert_eq!(recorder.snapshot().stream_failures, 0);

    let next_generation = lifecycle.reconfigure();
    assert_ne!(next_generation, generation);
    assert_eq!(
        lifecycle
            .ingest_quote_push(&push, now, generation)
            .expect("stale push"),
        None
    );
    let stale = recorder.snapshot();
    assert!(!stale.connected);
    assert_eq!(stale.generation, next_generation);
    assert_eq!(stale.quote_failures, 0);
}
