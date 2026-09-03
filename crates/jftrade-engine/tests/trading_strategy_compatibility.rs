use std::path::{Path, PathBuf};
use std::process::Command;

fn fixture_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join("tests/fixtures/compatibility/trading-strategy")
}

fn run(input: &Path) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_jftrade-trading-strategy-replay"))
        .args(["--input", input.to_str().expect("UTF-8 fixture path")])
        .output()
        .expect("trading-strategy replay starts")
}

#[test]
fn trading_strategy_replay_matches_pinned_golden_and_never_dispatches() {
    let root = fixture_root();
    let input = root.join("trading-strategy-corpus.json");
    let expected: serde_json::Value = serde_json::from_slice(
        &std::fs::read(root.join("trading-strategy-corpus.expected.json")).expect("expected"),
    )
    .expect("expected JSON");
    let mut previous = None;
    for _ in 0..5 {
        let output = run(&input);
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
        let actual: serde_json::Value =
            serde_json::from_slice(&output.stdout).expect("output JSON");
        assert_eq!(actual, expected);
        assert!(!contains_true_dispatch(&actual));
        if let Some(previous) = &previous {
            assert_eq!(previous, &output.stdout);
        }
        previous = Some(output.stdout);
    }
}

#[test]
fn trading_strategy_replay_rejects_truncated_unknown_and_malformed_fixed_point_input() {
    let root = std::env::temp_dir().join(format!(
        "jftrade-trading-strategy-invalid-{}",
        std::process::id()
    ));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(&root).expect("temp root");
    for (name, body) in [
        ("truncated.json", b"{\"version\":".as_slice()),
        (
            "unknown.json",
            br#"{"version":"stage5.v1","unexpected":true}"#.as_slice(),
        ),
        (
            "malformed.json",
            br#"{"version":"stage5.v1","plannedAt":"now","riskConfig":{"realTradingEnabled":true,"killSwitchActive":false,"maxOrderQuantity":"not-a-number","maxOrderNotional":null,"hardStops":[]},"statusCases":[],"transitions":[],"commands":[],"events":[],"sessionOperations":[],"protocols":[],"strategyScenarios":[]}"#.as_slice(),
        ),
    ] {
        let path = root.join(name);
        std::fs::write(&path, body).expect("write fixture");
        let output = run(&path);
        assert!(!output.status.success(), "invalid input {name} succeeded");
    }
    let _ = std::fs::remove_dir_all(root);
}

fn contains_true_dispatch(value: &serde_json::Value) -> bool {
    match value {
        serde_json::Value::Array(values) => values.iter().any(contains_true_dispatch),
        serde_json::Value::Object(fields) => {
            fields
                .get("dispatch")
                .is_some_and(|dispatch| dispatch == true)
                || fields.values().any(contains_true_dispatch)
        }
        _ => false,
    }
}
