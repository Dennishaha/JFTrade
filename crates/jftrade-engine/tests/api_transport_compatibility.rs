use std::path::{Path, PathBuf};
use std::process::Command;

fn fixture_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join("tests/fixtures/compatibility/api-transport")
}

fn run(input: &Path) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_jftrade-api-transport-replay"))
        .args(["--input", input.to_str().expect("UTF-8 fixture path")])
        .output()
        .expect("API transport replay starts")
}

#[test]
fn api_transport_replay_matches_pinned_golden_and_is_deterministic() {
    let root = fixture_root();
    let input = root.join("api-control-plane-corpus.json");
    let expected: serde_json::Value = serde_json::from_slice(
        &std::fs::read(root.join("api-control-plane-corpus.expected.json")).expect("expected"),
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
        assert_eq!(actual["routes"].as_array().map(Vec::len), Some(278));
        assert!(
            actual["routeProbes"][0]["allowed"]
                .as_bool()
                .unwrap_or(false)
        );
        assert!(
            !actual["routeProbes"]
                .as_array()
                .and_then(|probes| probes.last())
                .and_then(|probe| probe["allowed"].as_bool())
                .unwrap_or(true)
        );
        if let Some(previous) = &previous {
            assert_eq!(previous, &output.stdout);
        }
        previous = Some(output.stdout);
    }
}

#[test]
fn api_transport_replay_rejects_truncated_unknown_and_incomplete_input() {
    let root = std::env::temp_dir().join(format!(
        "jftrade-api-transport-invalid-{}",
        std::process::id()
    ));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(&root).expect("temp root");
    for (name, body) in [
        ("truncated.json", b"{\"version\":".as_slice()),
        (
            "unknown.json",
            br#"{"version":"stage7.v1","unexpected":true}"#.as_slice(),
        ),
        (
            "incomplete.json",
            br#"{"version":"stage7.v1","routes":[]}"#.as_slice(),
        ),
    ] {
        let path = root.join(name);
        std::fs::write(&path, body).expect("write fixture");
        let output = run(&path);
        assert!(!output.status.success(), "invalid input {name} succeeded");
    }
    let _ = std::fs::remove_dir_all(root);
}
