use std::path::{Path, PathBuf};
use std::process::Command;

fn fixture_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .join("tests/fixtures/rust-migration/stage4")
}

fn run(input: &Path) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_jftrade-stage4-shadow"))
        .args(["--input", input.to_str().expect("UTF-8 fixture path")])
        .output()
        .expect("stage 4 runner starts")
}

#[test]
fn stage4_rust_output_matches_pinned_golden_and_is_deterministic() {
    let root = fixture_root();
    let input = root.join("provider-lifecycle-corpus.json");
    let expected: serde_json::Value = serde_json::from_slice(
        &std::fs::read(root.join("provider-lifecycle-corpus.expected.json")).expect("expected"),
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
        if let Some(previous) = &previous {
            assert_eq!(previous, &output.stdout);
        }
        previous = Some(output.stdout);
    }
}

#[test]
fn stage4_runner_rejects_truncated_and_unknown_input() {
    let root = std::env::temp_dir().join(format!("jftrade-stage4-invalid-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(&root).expect("temp root");
    for (name, body) in [
        ("truncated.json", b"{\"version\":".as_slice()),
        (
            "unknown.json",
            br#"{"version":"stage4.v1","unexpected":true}"#.as_slice(),
        ),
    ] {
        let path = root.join(name);
        std::fs::write(&path, body).expect("write fixture");
        let output = run(&path);
        assert!(!output.status.success(), "invalid input {name} succeeded");
    }
    let _ = std::fs::remove_dir_all(root);
}
