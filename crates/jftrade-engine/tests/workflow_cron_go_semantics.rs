use jftrade_engine::product_workflow_cron::{next_schedule_run, parse_cron_expr};
use serde_json::json;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
fn at(s: &str) -> OffsetDateTime {
    OffsetDateTime::parse(s, &Rfc3339).unwrap()
}
#[test]
fn numeric_step_and_wildcard_day_rules_match_go() {
    for (expr, expected) in [
        ("5/15 * * * *", "2026-09-08T00:20:00Z"),
        ("0 0 */1 * 1", "2026-09-14T00:00:00Z"),
        ("0 0 */2 * 1", "2026-09-09T00:00:00Z"),
    ] {
        assert_eq!(
            next_schedule_run(
                &json!({"cron":expr,"timezone":"UTC"}),
                at("2026-09-08T00:06:00Z")
            )
            .unwrap(),
            at(expected)
        );
    }
    assert!(parse_cron_expr("0 9 ? JAN-MAR MON-FRI").is_ok());
    assert!(parse_cron_expr("59/4294967295 * * * *").is_ok());
}
#[test]
fn repeated_dst_hour_produces_the_next_actual_instant() {
    let config = json!({"cron":"30 1 * * *", "timezone":"America/New_York"});
    assert_eq!(
        next_schedule_run(&config, at("2026-11-01T05:40:00Z")).unwrap(),
        at("2026-11-01T06:30:00Z")
    );
    let spring = json!({"cron":"30 2 * * *", "timezone":"America/New_York"});
    assert_eq!(
        next_schedule_run(&spring, at("2026-03-08T05:00:00Z")).unwrap(),
        at("2026-03-09T06:30:00Z")
    );
}
