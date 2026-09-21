//! Frozen-input guards for the retired PineTS proto generator owner.
//!
//! The reference generator (input verification, staged `protoc` run, flat
//! `.go` staging plus a generated-line budget) no longer exists in this
//! workspace.  What remains is the checked-in `proto/pineworker` tree and
//! `build.rs`, which compiles it with prost at build time.  These tests keep
//! that remainder honest: the compile set must stay exactly the three recorded
//! inputs, the tree must keep its frozen digest, and the generated module path
//! must stay consistent with the package declared by the protos.

use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};

const PROTO_DIRECTORY: &str = "proto/pineworker";
const PROTO_INPUTS: [&str; 3] = [
    "pineworker.proto",
    "pineworker_common.proto",
    "pineworker_types.proto",
];
const PROTO_PACKAGE: &str = "jftrade.strategy.pineworker.v1";
const FROZEN_TREE_DIGEST: &str = "7c9a329cf3a88323dcaba46e19c325b624e69881175b3a58af8a1923ca26bc83";
const FROZEN_TREE_FILE_COUNT: usize = 3;

fn repository_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("..").join("..")
}

/// Cargo hands integration tests a per-target scratch directory, so fixtures do
/// not need a temporary-directory dependency or manual cleanup.
fn scratch_directory(name: &str) -> PathBuf {
    let path = PathBuf::from(env!("CARGO_TARGET_TMPDIR")).join(name);
    if path.exists() {
        fs::remove_dir_all(&path).expect("reset scratch directory");
    }
    fs::create_dir_all(&path).expect("create scratch directory");
    path
}

fn encode_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut encoded = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    encoded
}

/// Digest contract shared with the Futu frozen-artifact guards: fold
/// `relative name + 0x00 + content + 0x00` in sorted name order.
fn tree_digest(directory: &Path) -> Result<(String, usize), String> {
    let mut entries = Vec::new();
    for entry in
        fs::read_dir(directory).map_err(|error| format!("read {}: {error}", directory.display()))?
    {
        let entry = entry.map_err(|error| format!("read entry: {error}"))?;
        let file_type = entry
            .file_type()
            .map_err(|error| format!("inspect {}: {error}", entry.path().display()))?;
        if !file_type.is_file() {
            return Err(format!(
                "frozen PineTS proto tree must stay flat: {}",
                entry.path().display()
            ));
        }
        entries.push(entry.path());
    }
    entries.sort();

    let mut digest = Sha256::new();
    for path in &entries {
        let name = path
            .file_name()
            .and_then(std::ffi::OsStr::to_str)
            .ok_or_else(|| format!("non-utf8 proto name: {}", path.display()))?;
        let contents =
            fs::read(path).map_err(|error| format!("read {}: {error}", path.display()))?;
        digest.update(name.as_bytes());
        digest.update([0u8]);
        digest.update(&contents);
        digest.update([0u8]);
    }
    Ok((encode_hex(&digest.finalize()), entries.len()))
}

/// Mirrors the reference generator's input verification: every recorded input
/// must exist as a regular file before any generation command may run.
fn verify_generator_inputs(directory: &Path) -> Result<Vec<String>, String> {
    let mut verified = Vec::with_capacity(PROTO_INPUTS.len());
    for name in PROTO_INPUTS {
        let path = directory.join(name);
        match fs::metadata(&path) {
            Ok(metadata) if metadata.is_file() => verified.push(name.to_owned()),
            Ok(_) => {
                return Err(format!(
                    "inspect PineTS proto file {}: not a regular file",
                    path.display()
                ));
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                return Err(format!("missing Pineworker proto file: {}", path.display()));
            }
            Err(error) => {
                return Err(format!(
                    "inspect PineTS proto file {}: {error}",
                    path.display()
                ));
            }
        }
    }
    Ok(verified)
}

// Parity: go:452dea11:cmd/generate-pineworker-proto/generator_test.go:87 TestGeneratePineworkerProtoChecksInputsBeforeCommands
#[test]
fn pineworker_proto_inputs_are_verified_before_any_generation() {
    let proto_directory = repository_root().join(PROTO_DIRECTORY);
    assert_eq!(
        verify_generator_inputs(&proto_directory).expect("frozen inputs verified"),
        PROTO_INPUTS.to_vec()
    );

    // The Rust owner is `build.rs`: every recorded input is registered as a
    // rebuild trigger and the root proto is the compilation entry point, so a
    // dropped input fails the build instead of generating from a stale tree.
    let build_script =
        fs::read_to_string(repository_root().join("crates/jftrade-integration-pine/build.rs"))
            .expect("read pine integration build script");
    for name in PROTO_INPUTS {
        assert!(
            build_script.contains(&format!("\"{name}\"")),
            "build script must reference {name}: {build_script}"
        );
    }
    assert!(
        build_script.contains("rerun-if-changed"),
        "build script must register rebuild triggers: {build_script}"
    );
    assert!(
        build_script.contains("compile_protos"),
        "build script must compile the frozen protos: {build_script}"
    );

    let scratch = scratch_directory("pineworker-missing-input");
    for name in PROTO_INPUTS {
        fs::copy(proto_directory.join(name), scratch.join(name)).expect("copy frozen input");
    }
    let removed = scratch.join(PROTO_INPUTS[0]);
    fs::remove_file(&removed).expect("drop one input");
    let error = verify_generator_inputs(&scratch).expect_err("missing input must fail");
    assert!(
        error.starts_with("missing Pineworker proto file: "),
        "unexpected error: {error}"
    );
    assert!(error.contains(PROTO_INPUTS[0]), "unexpected error: {error}");
}

#[test]
fn pineworker_proto_tree_matches_the_frozen_digest() {
    let (digest, count) =
        tree_digest(&repository_root().join(PROTO_DIRECTORY)).expect("frozen PineTS proto tree");
    assert_eq!(count, FROZEN_TREE_FILE_COUNT);
    assert_eq!(digest, FROZEN_TREE_DIGEST);

    let names = fs::read_dir(repository_root().join(PROTO_DIRECTORY))
        .expect("read PineTS proto directory")
        .map(|entry| {
            entry
                .expect("proto entry")
                .file_name()
                .to_string_lossy()
                .into_owned()
        })
        .collect::<BTreeSet<_>>();
    assert_eq!(
        names,
        PROTO_INPUTS.iter().map(ToString::to_string).collect()
    );
    for name in PROTO_INPUTS {
        assert!(name.ends_with(".proto"), "unexpected extension: {name}");
        let contents = fs::read_to_string(repository_root().join(PROTO_DIRECTORY).join(name))
            .expect("read frozen proto");
        assert!(!contents.trim().is_empty(), "{name} must not be empty");
    }
}

#[test]
fn pineworker_proto_tree_keeps_the_canonical_package_and_imports() {
    let directory = repository_root().join(PROTO_DIRECTORY);
    for name in PROTO_INPUTS {
        let contents = fs::read_to_string(directory.join(name)).expect("read frozen proto");
        assert!(
            contents.contains("syntax = \"proto3\";"),
            "{name} must declare proto3 syntax"
        );
        assert!(
            contents.contains(&format!("package {PROTO_PACKAGE};")),
            "{name} must declare the frozen package"
        );
        for line in contents.lines() {
            let Some(import) = line.trim().strip_prefix("import \"") else {
                continue;
            };
            let imported = import
                .strip_suffix("\";")
                .expect("proto import must end with a quote and semicolon");
            assert!(
                repository_root().join(imported).is_file(),
                "{name} imports {imported}, which must resolve from the repository root"
            );
        }
    }

    // `compile_protos` uses the repository root as the include path, so the
    // generated module address is the package itself - the Rust equivalent of
    // the reference generator flattening nested output paths into one import.
    for source in ["execution.rs", "mock_worker.rs"] {
        let contents = fs::read_to_string(
            repository_root()
                .join("crates/jftrade-integration-pine/src")
                .join(source),
        )
        .expect("read pine integration source");
        assert!(
            contents.contains(&format!("include_proto!(\"{PROTO_PACKAGE}\")")),
            "{source} must include the frozen package module"
        );
    }
}
