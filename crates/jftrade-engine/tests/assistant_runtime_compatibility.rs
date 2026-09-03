use std::path::{Path, PathBuf};
use std::process::Command;

fn fixture_root() -> PathBuf {
    let manifest_dir = std::env::var_os("CARGO_MANIFEST_DIR")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(env!("CARGO_MANIFEST_DIR")));
    Path::new(&manifest_dir)
        .join("../..")
        .join("tests/fixtures/compatibility/assistant-runtime")
}

fn run(input: &Path) -> std::process::Output {
    let replay = std::env::var_os("NEXTEST_BIN_EXE_jftrade_assistant_runtime_replay")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(env!("CARGO_BIN_EXE_jftrade-assistant-runtime-replay")));
    Command::new(replay)
        .args(["--input", input.to_str().expect("UTF-8 fixture path")])
        .output()
        .expect("Assistant runtime replay starts")
}

#[test]
fn assistant_runtime_replay_matches_pinned_golden_and_is_deterministic() {
    let root = fixture_root();
    let input = root.join("assistant-rig-corpus.json");
    let expected: serde_json::Value = serde_json::from_slice(
        &std::fs::read(root.join("assistant-rig-corpus.expected.json")).expect("expected"),
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
        assert_eq!(actual["rig"]["recordTelemetryContent"], false);
        assert_eq!(actual["approval"]["replayResolutionChanged"], false);
        assert_eq!(actual["input"]["replayResolutionChanged"], false);
        if let Some(previous) = &previous {
            assert_eq!(previous, &output.stdout);
        }
        previous = Some(output.stdout);
    }
}

#[test]
fn assistant_runtime_replay_rejects_truncated_unknown_and_incomplete_input() {
    let root = std::env::temp_dir().join(format!(
        "jftrade-assistant-runtime-invalid-{}",
        std::process::id()
    ));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(&root).expect("temp root");
    for (name, body) in [
        ("truncated.json", b"{\"version\":".as_slice()),
        (
            "unknown.json",
            br#"{"version":"stage6.v1","unexpected":true}"#.as_slice(),
        ),
        (
            "incomplete.json",
            br#"{"version":"stage6.v1","now":"2026-08-19T00:00:00Z"}"#.as_slice(),
        ),
    ] {
        let path = root.join(name);
        std::fs::write(&path, body).expect("write fixture");
        let output = run(&path);
        assert!(!output.status.success(), "invalid input {name} succeeded");
    }
    let _ = std::fs::remove_dir_all(root);
}
