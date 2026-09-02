#[path = "../src/product_ws_live.rs"]
mod product_ws_live;

use product_ws_live::{WS_LIVE_ROUTE, WsLiveFixture, replay_fixture_case};

const FIXTURE: &str = include_str!("../../../tests/fixtures/rust-migration/stage9/ws-live.json");

#[test]
fn ws_live_replays_complete_go_corpus() {
    let fixture: WsLiveFixture = serde_json::from_str(FIXTURE).expect("ws-live fixture");
    assert_eq!(fixture.version, "stage9.ws-live.v1");
    assert_eq!(
        (fixture.route.method.as_str(), fixture.route.path.as_str()),
        WS_LIVE_ROUTE
    );
    assert_eq!(fixture.cases.len(), 11);
    for case in &fixture.cases {
        let actual = replay_fixture_case(case)
            .unwrap_or_else(|error| panic!("case {} replay failed: {error}", case.name));
        assert_eq!(actual, case.expected, "case {} diverged", case.name);
    }
}

#[test]
fn ws_live_replay_rejects_unknown_route_shape() {
    let mut fixture: WsLiveFixture = serde_json::from_str(FIXTURE).expect("ws-live fixture");
    fixture.cases[0].request_path = "/api/v1/ws/other".to_owned();
    let error = replay_fixture_case(&fixture.cases[0]).expect_err("unknown route must fail closed");
    assert!(
        error.contains("unsupported ws-live request"),
        "error = {error}"
    );
}
