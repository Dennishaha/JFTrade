use super::*;

fn subscription(
    market: &str,
    symbol: &str,
    instrument_id: &str,
    num: i64,
) -> WsLiveDepthSubscription {
    WsLiveDepthSubscription {
        market: market.to_owned(),
        symbol: symbol.to_owned(),
        instrument_id: instrument_id.to_owned(),
        num,
    }
}

fn security(market: &str, symbol: &str, instrument_id: &str) -> WsLiveSecuritySubscription {
    WsLiveSecuritySubscription {
        market: market.to_owned(),
        symbol: symbol.to_owned(),
        instrument_id: instrument_id.to_owned(),
    }
}

fn security_wire(value: &WsLiveSecuritySubscription) -> (String, String, String) {
    (
        value.market.clone(),
        value.symbol.clone(),
        value.instrument_id.clone(),
    )
}

fn depth_wire(value: &WsLiveDepthSubscription) -> (String, String, String, i64) {
    (
        value.market.clone(),
        value.symbol.clone(),
        value.instrument_id.clone(),
        value.num,
    )
}

// Parity: go:452dea11:internal/live/client_test.go:8 TestNormalizeSubscriptions
#[test]
fn subscription_normalization_matches_the_go_table() {
    let normalized = normalized_subscriptions(Some(&WsLiveSubscriptions {
        provider_broker_id: " Alpha ".to_owned(),
        active_instruments: vec![
            " us.aapl ".to_owned(),
            "HK.00700".to_owned(),
            "US.AAPL".to_owned(),
            String::new(),
        ],
        security_details: vec![
            security(" hk ", " 00700 ", " hk.00700 "),
            security("us", "MSFT", "US.MSFT"),
            security("HK", "IGNORED", "HK.00700"),
            security("", "AAPL", "US.AAPL"),
        ],
        depth: vec![
            subscription("", "AAPL", "US.AAPL", 10),
            subscription("HK", "00700", "HK.00700", 10),
            subscription(" us ", " tme ", " us.tme ", 0),
            subscription("US", "TME", "US.TME", 100),
            subscription("US", "TME", "US.TME", 50),
        ],
        console_refresh: true,
    }));

    assert_eq!(normalized.provider_broker_id, "alpha");
    assert_eq!(normalized.active_instruments, ["HK.00700", "US.AAPL"]);
    assert_eq!(
        normalized
            .security_details
            .iter()
            .map(security_wire)
            .collect::<Vec<_>>(),
        vec![
            ("HK".to_owned(), "00700".to_owned(), "HK.00700".to_owned()),
            ("US".to_owned(), "MSFT".to_owned(), "US.MSFT".to_owned()),
        ]
    );
    assert_eq!(
        normalized.depth.iter().map(depth_wire).collect::<Vec<_>>(),
        vec![
            (
                "HK".to_owned(),
                "00700".to_owned(),
                "HK.00700".to_owned(),
                10
            ),
            ("US".to_owned(), "TME".to_owned(), "US.TME".to_owned(), 1),
            ("US".to_owned(), "TME".to_owned(), "US.TME".to_owned(), 50),
        ]
    );
    assert!(normalized.console_refresh);

    let empty = normalized_subscriptions(None);
    assert!(empty.provider_broker_id.is_empty());
    assert!(empty.active_instruments.is_empty());
    assert!(empty.security_details.is_empty());
    assert!(empty.depth.is_empty());
    assert!(!empty.console_refresh);
}

// Parity: go:452dea11:internal/api/live/dispatcher_boundaries_test.go:59 TestDispatcherAuxiliarySubscriptionBranches
// Parity: go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:215 TestLiveWebSocketSendsConsoleRefresh
// The reference owner keeps the console refresh, skips only the provider
// that failed (`securityErr`/`depthErr`) and still writes the remaining
// auxiliary frames; the frozen projection must do the same.
#[test]
fn auxiliary_provider_failure_skips_only_the_failing_subscription_family() {
    let subscription = WsLiveSubscriptions {
        provider_broker_id: "futu".to_owned(),
        security_details: vec![security("US", "AAPL", "US.AAPL")],
        depth: vec![subscription("US", "AAPL", "US.AAPL", 10)],
        console_refresh: true,
        ..WsLiveSubscriptions::default()
    };
    let frame_types = |input: &WsLiveInput| {
        subscription_auxiliary_frames(input, &subscription)
            .iter()
            .map(|frame| {
                let envelope: Value = serde_json::from_str(frame).expect("auxiliary envelope json");
                envelope["type"].as_str().expect("frame type").to_owned()
            })
            .collect::<Vec<_>>()
    };

    assert_eq!(
        frame_types(&WsLiveInput::default()),
        ["console.refresh", "market.security-details", "market.depth"]
    );
    assert_eq!(
        frame_types(&WsLiveInput {
            security_error: true,
            ..WsLiveInput::default()
        }),
        ["console.refresh", "market.depth"]
    );
    assert_eq!(
        frame_types(&WsLiveInput {
            depth_error: true,
            ..WsLiveInput::default()
        }),
        ["console.refresh", "market.security-details"]
    );
    assert_eq!(
        frame_types(&WsLiveInput {
            security_error: true,
            depth_error: true,
            ..WsLiveInput::default()
        }),
        ["console.refresh"]
    );
}

// Parity: go:452dea11:internal/api/live/handler_test.go:441 TestDispatcherDeduplicatesTickObservedAt
// Parity: go:452dea11:internal/api/live/handler_test.go:480 TestDispatcherProviderSwitchTagsAndDoesNotDeduplicateNewProvider
// The reference owner deduplicates ticks per instrument + observedAt while
// preserving the payload source, and re-emits the same observation once the
// provider changes; the projection keys the dedupe set the same way.
#[test]
fn repeated_tick_observations_are_deduplicated_per_provider() {
    let tick = || WsLiveTick {
        instrument_id: " us.aapl ".to_owned(),
        observed_at: "2026-06-14T00:00:00Z".to_owned(),
        payload: json!({
            "type": "market-data.tick",
            "at": "2026-06-14T00:00:00Z",
            "source": "bbgo:futu",
            "price": "100.5",
        }),
    };
    let input = WsLiveInput {
        ticks: vec![tick(), tick()],
        ..WsLiveInput::default()
    };
    let subscribed = |provider: &str| WsLiveSubscriptions {
        provider_broker_id: provider.to_owned(),
        active_instruments: vec!["US.AAPL".to_owned()],
        ..WsLiveSubscriptions::default()
    };
    let decode = |frames: Vec<String>| {
        frames
            .iter()
            .map(|frame| serde_json::from_str::<Value>(frame).expect("tick envelope json"))
            .collect::<Vec<_>>()
    };

    let futu = decode(tick_frames(&input, &subscribed("futu")));
    assert_eq!(futu.len(), 1, "same observation must be written once");
    assert_eq!(futu[0]["entityId"], "US.AAPL");
    assert_eq!(futu[0]["payload"]["source"], "bbgo:futu");
    assert_eq!(futu[0]["payload"]["brokerId"], "futu");

    let alpha = decode(tick_frames(&input, &subscribed("alpha")));
    assert_eq!(alpha.len(), 1, "a new provider re-emits the observation");
    assert_eq!(alpha[0]["payload"]["brokerId"], "alpha");

    let distinct = WsLiveInput {
        ticks: vec![
            tick(),
            WsLiveTick {
                observed_at: "2026-06-14T00:00:01Z".to_owned(),
                ..tick()
            },
        ],
        ..WsLiveInput::default()
    };
    assert_eq!(decode(tick_frames(&distinct, &subscribed("futu"))).len(), 2);
}
