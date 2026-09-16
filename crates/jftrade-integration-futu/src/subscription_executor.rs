use std::net::SocketAddr;
use std::time::Duration;

use jftrade_broker::SnapshotAvailabilityKind;
use prost::Message;
use thiserror::Error;

use crate::{
    OpenDInitializedSession, OpenDManagedSessionError, OpenDTcpProbeConfig, ReconcileAction,
    SubscriptionKind, TcpTransportError, TransportError,
};

const RET_TYPE_SUCCEED: i32 = 0;

/// Executes one physical subscription action against an already authenticated
/// OpenD TCP session. The executor owns only protocol I/O; demand ownership,
/// generation fencing and retry policy remain in `OpenDSubscriptionLifecycle`.
pub struct OpenDSubscriptionExecutor {
    session: OpenDInitializedSession,
}

impl OpenDSubscriptionExecutor {
    pub fn connect(
        address: SocketAddr,
        timeout: Duration,
    ) -> Result<Self, SubscriptionExecutorError> {
        let config = OpenDTcpProbeConfig::new(address, timeout);
        let session = OpenDInitializedSession::connect_with_push_notifications(&config, 1)?;
        Ok(Self { session })
    }

    pub fn from_session(session: OpenDInitializedSession) -> Self {
        Self { session }
    }

    pub fn session(&self) -> &OpenDInitializedSession {
        &self.session
    }

    pub fn execute(&mut self, action: &ReconcileAction) -> Result<(), SubscriptionExecutorError> {
        let request = QotSubRequest {
            c2s: Some(qot_sub_request(action)?),
        };
        let response = self
            .session
            .managed_session()
            .call(crate::PROTO_QOT_SUB, &request.encode_to_vec())?;
        let response = QotSubResponse::decode(response.as_slice())
            .map_err(SubscriptionExecutorError::Decode)?;
        let ret_type = response.ret_type.unwrap_or(-400);
        if ret_type != RET_TYPE_SUCCEED {
            return Err(SubscriptionExecutorError::Rejected {
                ret_type,
                error_code: response.err_code.unwrap_or_default(),
                message: response
                    .ret_msg
                    .unwrap_or_else(|| "OpenD Qot_Sub request failed".to_owned()),
            });
        }
        Ok(())
    }
}

#[derive(Debug, Error)]
pub enum SubscriptionExecutorError {
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("connect to OpenD: {0}")]
    Connect(#[from] TcpTransportError),
    #[error("OpenD InitConnect handshake: {0}")]
    Handshake(#[from] crate::OpenDTcpProbeError),
    /// Retained for source compatibility with the pre-managed-session API.
    #[error("OpenD Qot_Sub exchange: {0}")]
    Exchange(#[from] TransportError),
    #[error("OpenD managed session: {0}")]
    Session(#[from] OpenDManagedSessionError),
    #[error("decode OpenD Qot_Sub response: {0}")]
    Decode(#[from] prost::DecodeError),
    #[error("OpenD Qot_Sub returned retType={ret_type} errCode={error_code}: {message}")]
    Rejected {
        ret_type: i32,
        error_code: i32,
        message: String,
    },
    #[error("invalid OpenD subscription instrument {0:?}")]
    InvalidInstrument(String),
    #[error("unsupported OpenD subscription interval {0:?}")]
    UnsupportedInterval(String),
}

impl SubscriptionExecutorError {
    /// Classifies an OpenD Qot_Sub rejection as a quote-access availability
    /// failure that the delayed snapshot fallback may serve.
    ///
    /// Matches the Go OpenD BasicQot classifier in
    /// `pkg/futu/exchange_basicqot.go`: entitlement, quota and
    /// unsupported-symbol rejections are fallback eligible, while transport,
    /// decode and local validation failures keep their normal retry semantics.
    pub fn snapshot_availability_kind(&self) -> Option<SnapshotAvailabilityKind> {
        let SubscriptionExecutorError::Rejected { message, .. } = self else {
            return None;
        };
        classify_snapshot_availability(message)
    }
}

/// Returns the broker-neutral quote-access kind for an OpenD rejection message.
fn classify_snapshot_availability(message: &str) -> Option<SnapshotAvailabilityKind> {
    let normalized = message.to_ascii_lowercase();
    let contains_any =
        |candidates: &[&str]| candidates.iter().any(|value| normalized.contains(value));
    if contains_any(&[
        "entitlement",
        "permission",
        "quote right",
        "qot right",
        "\u{6743}\u{9650}",
    ]) {
        return Some(SnapshotAvailabilityKind::new(
            SnapshotAvailabilityKind::ENTITLEMENT,
        ));
    }
    if contains_any(&[
        "quota",
        "subscription limit",
        "subscribe limit",
        "maximum subscription",
        "subscription is full",
        "\u{8ba2}\u{9605}\u{6570}\u{91cf}",
        "\u{8ba2}\u{9605}\u{5df2}\u{6ee1}",
    ]) {
        return Some(SnapshotAvailabilityKind::new(
            SnapshotAvailabilityKind::QUOTA,
        ));
    }
    if contains_any(&[
        "unknown stock",
        "unknown security",
        "\u{672a}\u{77e5}\u{80a1}\u{7968}",
        "\u{672a}\u{77e5}\u{8bc1}\u{5238}",
        "not support",
        "unsupported",
    ]) {
        return Some(SnapshotAvailabilityKind::new(
            SnapshotAvailabilityKind::UNSUPPORTED,
        ));
    }
    None
}

fn qot_sub_request(action: &ReconcileAction) -> Result<QotSubC2s, SubscriptionExecutorError> {
    let (subscription, subscribe) = match action {
        ReconcileAction::Subscribe { subscription } => (subscription, true),
        ReconcileAction::Unsubscribe { subscription } => (subscription, false),
    };
    let (market, code) = split_instrument(&subscription.instrument_id)?;
    // The coordinator owns the only OpenD session and is therefore the Rust
    // equivalent of Go's stream layer, which registers push delivery for the
    // live contract: `subscribeBasicQotPush` sends `IsRegOrUnRegPush=true` with
    // `IsFirstPush=true` (pkg/futu/stream.go), and
    // `ensureOrderBookPushSubscriptions` sends `IsRegPush=true`
    // (pkg/futu/exchange_orderbook.go). K-line pushes stay disabled because Go
    // reads candles through `Qot_RequestHistoryKL`/`Qot_GetKL`
    // (pkg/futu/exchange_kline.go).
    let (sub_type, register_push, first_push) = match subscription.kind {
        SubscriptionKind::Basic if subscribe => (1, Some(true), Some(true)),
        // Go's `UnsubscribeBasicQuote` both releases the subscription and
        // unregisters its pushes (`setBasicQotSubscription(..., false, false)`).
        SubscriptionKind::Basic => (1, Some(false), None),
        SubscriptionKind::Kline => (
            kline_sub_type(subscription.interval.as_deref())?,
            Some(false),
            None,
        ),
        SubscriptionKind::OrderBook if subscribe => (2, Some(true), None),
        // Go's `UnsubscribeOrderBook` unregisters order-book pushes as well.
        SubscriptionKind::OrderBook => (2, Some(false), None),
    };
    // HK order-book subscriptions may request SF detail; Go groups HK requests
    // into its own batch and only sets the flag for that market
    // (pkg/futu/exchange_orderbook.go::groupOrderBookRequestsForPush).
    let order_book_detail =
        (subscription.kind == SubscriptionKind::OrderBook && market == 1).then_some(true);
    Ok(QotSubC2s {
        security_list: vec![QotSecurity {
            market: Some(market),
            code: Some(code),
        }],
        sub_type_list: vec![sub_type],
        is_sub_or_un_sub: Some(subscribe),
        is_reg_or_un_reg_push: register_push,
        reg_push_rehab_type_list: Vec::new(),
        is_first_push: first_push,
        // Go always sends isUnsubAll (defaulting to false) on Qot_Sub.
        is_unsub_all: Some(false),
        is_sub_order_book_detail: order_book_detail,
        extended_time: None,
        session: None,
    })
}

pub(crate) fn split_instrument(value: &str) -> Result<(i32, String), SubscriptionExecutorError> {
    let (market, code) = value
        .trim()
        .split_once('.')
        .ok_or_else(|| SubscriptionExecutorError::InvalidInstrument(value.to_owned()))?;
    let market = match market.to_ascii_uppercase().as_str() {
        "HK" => 1,
        "US" => 11,
        "SH" | "CNSH" => 21,
        "SZ" | "CNSZ" => 22,
        "SG" => 31,
        "JP" => 41,
        "AU" => 51,
        "MY" => 61,
        "CA" => 71,
        _ => {
            return Err(SubscriptionExecutorError::InvalidInstrument(
                value.to_owned(),
            ));
        }
    };
    let code = code.trim();
    if code.is_empty() || code.contains('.') {
        return Err(SubscriptionExecutorError::InvalidInstrument(
            value.to_owned(),
        ));
    }
    Ok((market, code.to_ascii_uppercase()))
}

fn kline_sub_type(interval: Option<&str>) -> Result<i32, SubscriptionExecutorError> {
    let interval = interval.unwrap_or_default().trim().to_ascii_lowercase();
    let sub_type = match interval.as_str() {
        "1d" | "day" => 6,
        "5m" => 7,
        "15m" => 8,
        "30m" => 9,
        "60m" | "1h" => 10,
        "1m" => 11,
        "1w" | "week" => 12,
        "1mo" | "month" => 13,
        "3mo" | "quarter" => 15,
        "1y" | "year" => 16,
        "3m" => 17,
        "10m" => 18,
        "120m" => 19,
        "180m" => 20,
        "240m" => 21,
        _ => return Err(SubscriptionExecutorError::UnsupportedInterval(interval)),
    };
    Ok(sub_type)
}

#[derive(Clone, PartialEq, Message)]
struct QotSubRequest {
    #[prost(message, optional, tag = "1")]
    c2s: Option<QotSubC2s>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSubC2s {
    #[prost(message, repeated, tag = "1")]
    security_list: Vec<QotSecurity>,
    #[prost(int32, repeated, tag = "2")]
    sub_type_list: Vec<i32>,
    #[prost(bool, optional, tag = "3")]
    is_sub_or_un_sub: Option<bool>,
    #[prost(bool, optional, tag = "4")]
    is_reg_or_un_reg_push: Option<bool>,
    #[prost(int32, repeated, tag = "5")]
    reg_push_rehab_type_list: Vec<i32>,
    #[prost(bool, optional, tag = "6")]
    is_first_push: Option<bool>,
    #[prost(bool, optional, tag = "7")]
    is_unsub_all: Option<bool>,
    #[prost(bool, optional, tag = "8")]
    is_sub_order_book_detail: Option<bool>,
    #[prost(bool, optional, tag = "9")]
    extended_time: Option<bool>,
    #[prost(int32, optional, tag = "10")]
    session: Option<i32>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSecurity {
    #[prost(int32, optional, tag = "1")]
    market: Option<i32>,
    #[prost(string, optional, tag = "2")]
    code: Option<String>,
}

#[derive(Clone, PartialEq, Message)]
struct QotSubResponse {
    #[prost(int32, optional, tag = "1")]
    ret_type: Option<i32>,
    #[prost(string, optional, tag = "2")]
    ret_msg: Option<String>,
    #[prost(int32, optional, tag = "3")]
    err_code: Option<i32>,
}

#[cfg(test)]
mod tests {
    use std::io::{Read, Write};
    use std::net::TcpListener;
    use std::sync::Arc;
    use std::thread;

    use super::*;
    use crate::transport::read_framed_frame;
    use crate::{
        OpenDSessionEvent, OpenDSubscriptionLifecycle, PROTO_UPDATE_BASIC_QOT,
        PhysicalSubscription, decode_frame, encode_frame,
    };
    use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};

    #[derive(Clone, PartialEq, Message)]
    struct InitRequest {
        #[prost(message, optional, tag = "1")]
        c2s: Option<InitRequestState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitRequestState {
        #[prost(bool, optional, tag = "3")]
        recv_notify: Option<bool>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitResponse {
        #[prost(int32, optional, tag = "1")]
        ret_type: Option<i32>,
        #[prost(message, optional, tag = "4")]
        s2c: Option<InitState>,
    }

    #[derive(Clone, PartialEq, Message)]
    struct InitState {
        #[prost(int32, tag = "1")]
        server_ver: i32,
        #[prost(uint64, tag = "3")]
        conn_id: u64,
    }

    fn action(
        kind: SubscriptionKind,
        instrument_id: &str,
        interval: Option<&str>,
    ) -> ReconcileAction {
        ReconcileAction::Subscribe {
            subscription: PhysicalSubscription {
                key: "test".to_owned(),
                kind,
                instrument_id: instrument_id.to_owned(),
                interval: interval.map(str::to_owned),
            },
        }
    }

    #[test]
    fn unsubscribe_request_sets_is_sub_or_un_sub_false_and_keeps_security() {
        // Parity: pkg/futu/opend/new_methods_test.go:352 TestUnsubscribeQuotes
        //
        // Go asserts the unsubscribe path keeps the same security identity and
        // flips `IsSubOrUnSub` to false. The executor shares one builder for
        // subscribe/unsubscribe, so this pins the unsubscribe branch directly.
        let request = match action(SubscriptionKind::Basic, "HK.00700", None) {
            ReconcileAction::Subscribe { subscription } => {
                qot_sub_request(&ReconcileAction::Unsubscribe { subscription })
                    .expect("unsubscribe request")
            }
            _ => unreachable!(),
        };
        assert_eq!(request.is_sub_or_un_sub, Some(false));
        assert_eq!(request.security_list[0].market, Some(1));
        assert_eq!(request.security_list[0].code.as_deref(), Some("00700"));
        assert_eq!(request.sub_type_list, [1]);
    }

    #[test]
    fn basic_subscribe_registers_push_delivery_like_go_stream() {
        // Parity: go:452dea11:pkg/futu/stream.go:352 subscribeBasicQotPush
        // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:255
        // TestBasicQuotePushSubscriptionErrorsAndIdempotency
        //
        // The Rust coordinator owns the only OpenD session, so its demand-driven
        // Basic subscribe is the stream path: it must register push delivery,
        // otherwise Qot_UpdateBasicQot never arrives and the live contract stays
        // empty. Go sends IsRegOrUnRegPush=true with IsFirstPush=true here.
        let request = qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("basic request");
        assert_eq!(request.is_sub_or_un_sub, Some(true));
        assert_eq!(
            request.is_reg_or_un_reg_push,
            Some(true),
            "the live path must register push delivery"
        );
        assert_eq!(
            request.is_first_push,
            Some(true),
            "Go asks for one initial push so the stream starts with data"
        );
    }

    #[test]
    fn basic_unsubscribe_unregisters_push_delivery_like_go() {
        // Parity: go:452dea11:pkg/futu/exchange_basicqot.go:251
        // UnsubscribeBasicQuote
        let request = match action(SubscriptionKind::Basic, "US.AAPL", None) {
            ReconcileAction::Subscribe { subscription } => {
                qot_sub_request(&ReconcileAction::Unsubscribe { subscription })
                    .expect("unsubscribe request")
            }
            _ => unreachable!(),
        };
        assert_eq!(request.is_sub_or_un_sub, Some(false));
        assert_eq!(
            request.is_reg_or_un_reg_push,
            Some(false),
            "releasing a Basic subscription must also unregister its pushes"
        );
    }

    #[test]
    fn hk_order_book_subscribe_requests_detail_and_registers_push() {
        // Parity: go:452dea11:pkg/futu/exchange_orderbook.go:151
        // ensureOrderBookPushSubscriptions (HK batch sets IsSubOrderBookDetail)
        let hk = qot_sub_request(&action(SubscriptionKind::OrderBook, "HK.00700", None))
            .expect("hk order book request");
        assert_eq!(hk.sub_type_list, [2]);
        assert_eq!(hk.is_reg_or_un_reg_push, Some(true));
        assert_eq!(
            hk.is_sub_order_book_detail,
            Some(true),
            "HK order-book push registration asks for SF detail"
        );

        let us = qot_sub_request(&action(SubscriptionKind::OrderBook, "US.AAPL", None))
            .expect("us order book request");
        assert_eq!(us.is_reg_or_un_reg_push, Some(true));
        assert_eq!(
            us.is_sub_order_book_detail, None,
            "order-book detail is HK-only, matching Go's batch grouping"
        );
    }

    #[test]
    fn qot_sub_mapping_matches_go_market_and_interval_values() {
        let request = match action(SubscriptionKind::Kline, "HK.00700", Some("1m")) {
            ReconcileAction::Subscribe { subscription } => {
                qot_sub_request(&ReconcileAction::Subscribe { subscription }).expect("request")
            }
            _ => unreachable!(),
        };
        assert_eq!(request.security_list[0].market, Some(1));
        assert_eq!(request.security_list[0].code.as_deref(), Some("00700"));
        assert_eq!(request.sub_type_list, [11]);
        assert_eq!(request.is_sub_or_un_sub, Some(true));
        assert_eq!(request.is_reg_or_un_reg_push, Some(false));

        let basic = qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("basic request");
        assert_eq!(basic.security_list[0].market, Some(11));
        assert_eq!(basic.sub_type_list, [1]);
        // The coordinator is the stream owner, so Basic subscribes register
        // push delivery (`pkg/futu/stream.go::subscribeBasicQotPush`).
        assert_eq!(basic.is_reg_or_un_reg_push, Some(true));
    }

    #[test]
    fn qot_sub_all_options_match_go_quote_sub_request_encoding() {
        // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:1023 TestSubscribeQuotesAllOptions
        // Parity: go:452dea11:pkg/futu/opend/market_read_boundaries_test.go:21 TestSubscribeQuotesEncodesAdvancedMarketDataOptions
        //
        // Go's QuoteSubRequest is a public option bag; Rust derives the same
        // wire options from subscription kind. This pins the encoding contract
        // for every field Rust supports, and proves the repeated rehab type
        // list stays absent unless a caller sets one.
        let request = QotSubC2s {
            security_list: vec![QotSecurity {
                market: Some(11),
                code: Some("AAPL".to_owned()),
            }],
            sub_type_list: vec![1, 6],
            is_sub_or_un_sub: Some(true),
            is_reg_or_un_reg_push: Some(true),
            reg_push_rehab_type_list: vec![1],
            is_first_push: Some(true),
            is_unsub_all: Some(false),
            is_sub_order_book_detail: None,
            extended_time: Some(true),
            session: Some(1),
        };
        let decoded = super::QotSubRequest { c2s: Some(request) };
        let round_trip =
            super::QotSubRequest::decode(decoded.encode_to_vec().as_slice()).expect("round trip");
        let c2s = round_trip.c2s.expect("c2s");
        assert_eq!(c2s.security_list.len(), 1);
        assert_eq!(c2s.security_list[0].market, Some(11));
        assert_eq!(c2s.security_list[0].code.as_deref(), Some("AAPL"));
        assert_eq!(c2s.sub_type_list, [1, 6]);
        assert_eq!(c2s.is_sub_or_un_sub, Some(true));
        assert_eq!(c2s.is_reg_or_un_reg_push, Some(true));
        assert_eq!(c2s.reg_push_rehab_type_list, [1]);
        assert_eq!(c2s.is_first_push, Some(true));
        assert_eq!(c2s.is_unsub_all, Some(false));
        assert_eq!(c2s.extended_time, Some(true));
        assert_eq!(c2s.session, Some(1));

        // The executor only emits options it can derive from the subscription
        // kind; a Basic subscribe registers pushes with an initial push, while
        // K-line-only options stay unset rather than being fabricated.
        let derived = super::qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("basic request");
        assert_eq!(derived.is_first_push, Some(true));
        assert_eq!(derived.extended_time, None);
        assert_eq!(derived.session, None);
        assert_eq!(
            derived.is_sub_order_book_detail, None,
            "order-book detail is meaningless for a Basic subscribe"
        );
        assert!(derived.reg_push_rehab_type_list.is_empty());
        assert_eq!(derived.is_unsub_all, Some(false));
    }

    #[test]
    fn qot_sub_unsub_all_matches_go_all_flag_encoding() {
        // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:1082 TestSubscribeQuotesUnsubAll
        //
        // Go sets isUnsubAll=true with no security list. Rust has no caller for
        // the all-subscription cancel path, so this pins the wire encoding the
        // Go test asserts and records that no product owner drives it yet.
        let request = QotSubC2s {
            security_list: Vec::new(),
            sub_type_list: Vec::new(),
            is_sub_or_un_sub: Some(true),
            is_reg_or_un_reg_push: None,
            reg_push_rehab_type_list: Vec::new(),
            is_first_push: None,
            is_unsub_all: Some(true),
            is_sub_order_book_detail: None,
            extended_time: None,
            session: None,
        };
        let decoded = super::QotSubRequest { c2s: Some(request) };
        let value =
            super::QotSubRequest::decode(decoded.encode_to_vec().as_slice()).expect("decode");
        let c2s = value.c2s.expect("c2s");
        assert_eq!(c2s.is_unsub_all, Some(true));
        assert!(c2s.security_list.is_empty());
    }

    #[test]
    fn executor_sends_subscribe_and_unsubscribe_over_one_framed_session() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            for exchange in 0..3 {
                let mut header = [0_u8; crate::frame::HEADER_LEN];
                stream.read_exact(&mut header).expect("header");
                let body_len =
                    u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
                let mut body = vec![0_u8; body_len];
                stream.read_exact(&mut body).expect("body");
                let frame =
                    decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("frame");
                let response = if exchange == 0 {
                    assert_eq!(frame.header.proto_id, crate::PROTO_INIT_CONNECT);
                    let init = InitRequest::decode(frame.body.as_slice()).expect("init request");
                    assert_eq!(
                        init.c2s.and_then(|state| state.recv_notify),
                        Some(true),
                        "the subscription data session must receive pushes"
                    );
                    InitResponse {
                        ret_type: Some(0),
                        s2c: Some(InitState {
                            server_ver: 1009,
                            conn_id: 1,
                        }),
                    }
                    .encode_to_vec()
                } else {
                    assert_eq!(frame.header.proto_id, crate::PROTO_QOT_SUB);
                    let request = QotSubRequest::decode(frame.body.as_slice()).expect("request");
                    assert_eq!(
                        request.c2s.as_ref().expect("c2s").is_sub_or_un_sub,
                        Some(exchange == 1)
                    );
                    if exchange == 1 {
                        let push =
                            encode_frame(PROTO_UPDATE_BASIC_QOT, 0, b"push").expect("push frame");
                        stream.write_all(&push).expect("write push");
                    }
                    QotSubResponse {
                        ret_type: Some(0),
                        ret_msg: None,
                        err_code: None,
                    }
                    .encode_to_vec()
                };
                let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                    .expect("response");
                stream.write_all(&packet).expect("write response");
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        executor
            .execute(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect("subscribe");
        let push = executor
            .session()
            .managed_session()
            .receive_event_timeout(Duration::from_secs(1))
            .expect("push event");
        assert!(matches!(
            push,
            OpenDSessionEvent::UnsolicitedFrame { generation: 1, frame }
                if frame.header.proto_id == PROTO_UPDATE_BASIC_QOT && frame.body == b"push"
        ));
        executor
            .execute(&ReconcileAction::Unsubscribe {
                subscription: PhysicalSubscription {
                    key: "test".to_owned(),
                    kind: SubscriptionKind::Basic,
                    instrument_id: "US.AAPL".to_owned(),
                    interval: None,
                },
            })
            .expect("unsubscribe");
        server.join().expect("server");
    }

    #[test]
    fn executor_maps_qot_sub_rejection_without_reporting_success() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            for exchange in 0..2 {
                let mut header = [0_u8; crate::frame::HEADER_LEN];
                stream.read_exact(&mut header).expect("header");
                let body_len =
                    u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
                let mut body = vec![0_u8; body_len];
                stream.read_exact(&mut body).expect("body");
                let frame =
                    decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("frame");
                let response = if exchange == 0 {
                    InitResponse {
                        ret_type: Some(0),
                        s2c: Some(InitState {
                            server_ver: 1009,
                            conn_id: 2,
                        }),
                    }
                    .encode_to_vec()
                } else {
                    assert_eq!(frame.header.proto_id, crate::PROTO_QOT_SUB);
                    QotSubResponse {
                        ret_type: Some(-3),
                        ret_msg: Some("subscription denied".to_owned()),
                        err_code: Some(1001),
                    }
                    .encode_to_vec()
                };
                let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                    .expect("response");
                stream.write_all(&packet).expect("write response");
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        let error = executor
            .execute(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect_err("rejected subscription");
        assert!(matches!(
            error,
            SubscriptionExecutorError::Rejected {
                ret_type: -3,
                error_code: 1001,
                message
            } if message == "subscription denied"
        ));
        server.join().expect("server");
    }

    #[test]
    fn qot_sub_rejection_surfaces_ret_type_err_code_and_message() {
        // Parity: go:452dea11:pkg/futu/opend/new_methods_test.go:716 TestSubscribeQuotesError
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let init = read_framed_frame(&mut stream).expect("init request");
            let response = InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 1,
                }),
            };
            stream
                .write_all(
                    &encode_frame(
                        init.header.proto_id,
                        init.header.serial_no,
                        &response.encode_to_vec(),
                    )
                    .expect("init response"),
                )
                .expect("write init response");

            let request = read_framed_frame(&mut stream).expect("qot sub request");
            let response = QotSubResponse {
                ret_type: Some(-1),
                ret_msg: Some("invalid security".to_owned()),
                err_code: Some(1102),
            };
            stream
                .write_all(
                    &encode_frame(
                        request.header.proto_id,
                        request.header.serial_no,
                        &response.encode_to_vec(),
                    )
                    .expect("sub response"),
                )
                .expect("write sub response");
            let mut byte = [0_u8; 1];
            let _ = stream.read(&mut byte);
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        let error = executor
            .execute(&action(SubscriptionKind::Basic, "US.AAPL", None))
            .expect_err("rejection must surface");
        assert!(matches!(
            error,
            SubscriptionExecutorError::Rejected {
                ret_type: -1,
                error_code: 1102,
                ref message,
            } if message == "invalid security"
        ));
        let _ = executor.session().managed_session().close();
        server.join().expect("server");
    }

    #[test]
    fn qot_sub_availability_classifier_matches_go_keyword_matrix() {
        // Parity: pkg/futu/exchange_basicqot.go snapshot availability classification
        let eligible = [
            ("entitlement", SnapshotAvailabilityKind::ENTITLEMENT),
            (
                "no permission for this market",
                SnapshotAvailabilityKind::ENTITLEMENT,
            ),
            (
                "quote right is missing",
                SnapshotAvailabilityKind::ENTITLEMENT,
            ),
            ("qot right expired", SnapshotAvailabilityKind::ENTITLEMENT),
            (
                "\u{6743}\u{9650}\u{4e0d}\u{8db3}",
                SnapshotAvailabilityKind::ENTITLEMENT,
            ),
            ("quota exceeded", SnapshotAvailabilityKind::QUOTA),
            (
                "subscription limit reached",
                SnapshotAvailabilityKind::QUOTA,
            ),
            ("subscribe limit reached", SnapshotAvailabilityKind::QUOTA),
            (
                "maximum subscription reached",
                SnapshotAvailabilityKind::QUOTA,
            ),
            (
                "OpenD subscription is full",
                SnapshotAvailabilityKind::QUOTA,
            ),
            (
                "\u{8ba2}\u{9605}\u{6570}\u{91cf}\u{8d85}\u{9650}",
                SnapshotAvailabilityKind::QUOTA,
            ),
            (
                "\u{8ba2}\u{9605}\u{5df2}\u{6ee1}",
                SnapshotAvailabilityKind::QUOTA,
            ),
            ("unknown stock", SnapshotAvailabilityKind::UNSUPPORTED),
            ("unknown security", SnapshotAvailabilityKind::UNSUPPORTED),
            (
                "\u{672a}\u{77e5}\u{80a1}\u{7968}",
                SnapshotAvailabilityKind::UNSUPPORTED,
            ),
            (
                "\u{672a}\u{77e5}\u{8bc1}\u{5238}",
                SnapshotAvailabilityKind::UNSUPPORTED,
            ),
            ("market not support", SnapshotAvailabilityKind::UNSUPPORTED),
            ("unsupported symbol", SnapshotAvailabilityKind::UNSUPPORTED),
        ];
        for (message, kind) in eligible {
            let error = SubscriptionExecutorError::Rejected {
                ret_type: -3,
                error_code: 1001,
                message: message.to_owned(),
            };
            assert_eq!(
                error.snapshot_availability_kind(),
                Some(SnapshotAvailabilityKind::new(kind)),
                "message {message:?} should be fallback eligible"
            );
        }

        // Go `TestClassifyBasicQotSubscriptionErrorOnlyMarksAvailabilityFailures`
        // additionally keeps frequency/rate-limit and transport failures
        // ineligible; those must never be routed to the delayed fallback.
        for message in [
            "subscription denied",
            "network unreachable",
            "qot sub internal error",
            "request frequency too high",
            "snapshot rate limited; retry after 1s",
            "",
        ] {
            let error = SubscriptionExecutorError::Rejected {
                ret_type: -3,
                error_code: 1001,
                message: message.to_owned(),
            };
            assert_eq!(
                error.snapshot_availability_kind(),
                None,
                "message {message:?}"
            );
        }

        // Non-rejection errors never classify as availability failures.
        assert_eq!(
            SubscriptionExecutorError::InvalidInstrument("bad".to_owned())
                .snapshot_availability_kind(),
            None
        );
        assert_eq!(
            SubscriptionExecutorError::UnsupportedInterval("7m".to_owned())
                .snapshot_availability_kind(),
            None
        );
    }

    #[test]
    fn lifecycle_rejects_stale_executor_before_qot_sub_io() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).expect("listener");
        let address = listener.local_addr().expect("address");
        let server = thread::spawn(move || {
            let (mut stream, _) = listener.accept().expect("accept");
            let mut header = [0_u8; crate::frame::HEADER_LEN];
            stream.read_exact(&mut header).expect("init header");
            let body_len = u32::from_le_bytes(header[12..16].try_into().expect("length")) as usize;
            let mut body = vec![0_u8; body_len];
            stream.read_exact(&mut body).expect("init body");
            let frame =
                decode_frame(&[header.as_slice(), body.as_slice()].concat()).expect("init frame");
            assert_eq!(frame.header.proto_id, crate::PROTO_INIT_CONNECT);
            let response = InitResponse {
                ret_type: Some(0),
                s2c: Some(InitState {
                    server_ver: 1009,
                    conn_id: 3,
                }),
            }
            .encode_to_vec();
            let packet = encode_frame(frame.header.proto_id, frame.header.serial_no, &response)
                .expect("init response");
            stream.write_all(&packet).expect("write init response");

            stream
                .set_read_timeout(Some(Duration::from_millis(200)))
                .expect("read timeout");
            let mut unexpected = [0_u8; 1];
            match stream.read(&mut unexpected) {
                Ok(0) => false,
                Ok(_) => true,
                Err(error)
                    if matches!(
                        error.kind(),
                        std::io::ErrorKind::WouldBlock | std::io::ErrorKind::TimedOut
                    ) =>
                {
                    false
                }
                Err(error) => panic!("read after init: {error}"),
            }
        });

        let mut executor =
            OpenDSubscriptionExecutor::connect(address, Duration::from_secs(1)).expect("executor");
        assert_eq!(executor.session().managed_session().generation(), 1);

        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(recorder, 0);
        let desired = [InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }];
        lifecycle.reconcile_demand(&desired, 0);
        let replay = lifecycle.reconfigure_for_reconnect(&desired);
        let generation = lifecycle.generation();
        assert_eq!(generation, 2);
        assert_eq!(replay.len(), 1);

        assert!(
            !lifecycle
                .execute_action(&replay[0], 0, generation, &mut executor)
                .expect("stale executor is ignored")
        );
        assert!(
            !server.join().expect("server"),
            "stale executor sent Qot_Sub"
        );
    }

    #[test]
    fn executor_rejects_invalid_instrument_and_unsupported_interval() {
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Basic, "AAPL", None)),
            Err(SubscriptionExecutorError::InvalidInstrument(_))
        ));
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Basic, "US.AAPL.EXTRA", None)),
            Err(SubscriptionExecutorError::InvalidInstrument(_))
        ));
        assert!(matches!(
            qot_sub_request(&action(SubscriptionKind::Kline, "US.AAPL", Some("2m"))),
            Err(SubscriptionExecutorError::UnsupportedInterval(_))
        ));
    }
}
