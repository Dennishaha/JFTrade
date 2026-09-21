//! Frozen-artifact guards for the retired `cmd/generate-futu-proto` owner.
//!
//! The Go generator (checksum verification of the upstream proto drop,
//! `go_package` rewriting, staged protoc run, repository digest) no longer
//! exists in this workspace.  What remains is the checked-in `proto/futu` tree,
//! the checksum manifest and repository digest it wrote, and `build.rs`, which
//! compiles the protos with prost at build time.  These tests keep that
//! remainder honest: the proto tree must still match the recorded digest and
//! manifest, so editing or dropping a staged proto fails here instead of
//! silently changing the Futu wire contract.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};

const DIGEST_FILE: &str = "scripts/futu-proto-generated-10.9.6908.digest";
const MANIFEST_FILE: &str = "scripts/futu-proto-10.9.6908.sha256";
const PROTO_DIRECTORY: &str = "proto/futu";
const PROTO_FILE_COUNT: usize = 184;

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

fn is_lower_hex_digest(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

#[derive(Debug, Eq, PartialEq)]
struct RepositoryDigest {
    proto: (String, usize),
    generated: (String, usize),
}

fn parse_repository_digest(path: &Path) -> Result<RepositoryDigest, String> {
    let contents = fs::read_to_string(path).map_err(|error| {
        format!(
            "open generated repository digest {}: {error}",
            path.display()
        )
    })?;
    let mut proto = None;
    let mut generated = None;
    for line in contents.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let fields: Vec<&str> = line.split_whitespace().collect();
        if fields.len() != 3
            || !matches!(fields[0], "proto" | "pb")
            || !is_lower_hex_digest(fields[1])
        {
            return Err(format!(
                "invalid generated repository digest entry in {}",
                path.display()
            ));
        }
        let count: usize = fields[2].parse().map_err(|_| {
            format!(
                "invalid generated repository file count in {}",
                path.display()
            )
        })?;
        if count == 0 {
            return Err(format!(
                "invalid generated repository file count in {}",
                path.display()
            ));
        }
        let slot = if fields[0] == "proto" {
            &mut proto
        } else {
            &mut generated
        };
        if slot.is_some() {
            return Err(format!(
                "duplicate generated repository digest entry {:?} in {}",
                fields[0],
                path.display()
            ));
        }
        *slot = Some((fields[1].to_owned(), count));
    }
    match (proto, generated) {
        (Some(proto), Some(generated)) => Ok(RepositoryDigest { proto, generated }),
        _ => Err(format!(
            "generated repository digest {} must contain proto and pb entries",
            path.display()
        )),
    }
}

/// Lists a generated tree the way the retired inspector did: flat, regular
/// files only, every name carrying the expected extension.
fn tree_file_names(root: &Path, extension: &str) -> Result<Vec<String>, String> {
    let entries = fs::read_dir(root)
        .map_err(|error| format!("inspect generated tree {}: {error}", root.display()))?;
    let mut names = Vec::new();
    for entry in entries {
        let entry =
            entry.map_err(|error| format!("inspect generated tree {}: {error}", root.display()))?;
        let name = entry.file_name().to_string_lossy().into_owned();
        let file_type = entry
            .file_type()
            .map_err(|error| format!("inspect generated tree {}: {error}", root.display()))?;
        if file_type.is_dir() {
            return Err(format!("checked-in Futu proto must remain flat: {name}/"));
        }
        if !file_type.is_file() {
            return Err(format!(
                "unexpected non-regular generated file: {}",
                entry.path().display()
            ));
        }
        if Path::new(&name)
            .extension()
            .and_then(|value| value.to_str())
            != Some(&extension[1..])
        {
            return Err(format!(
                "unexpected file in generated tree: {}",
                entry.path().display()
            ));
        }
        names.push(name);
    }
    names.sort();
    if names.is_empty() {
        return Err(format!(
            "generated tree contains no {extension} files: {}",
            root.display()
        ));
    }
    Ok(names)
}

fn digest_tree(root: &Path, extension: &str) -> Result<(String, Vec<String>), String> {
    let names = tree_file_names(root, extension)?;
    let mut hasher = Sha256::new();
    for name in &names {
        let content = fs::read(root.join(name))
            .map_err(|error| format!("read generated file {name}: {error}"))?;
        hasher.update(name.as_bytes());
        hasher.update([0]);
        hasher.update(&content);
        hasher.update([0]);
    }
    Ok((encode_hex(&hasher.finalize()), names))
}

fn verify_repository_digest(digest_path: &Path, tree_root: &Path) -> Result<(), String> {
    let recorded = parse_repository_digest(digest_path)?;
    let (actual_digest, actual_files) = digest_tree(tree_root, ".proto")?;
    if recorded.proto != (actual_digest, actual_files.len()) {
        return Err(format!(
            "checked-in OpenD generated outputs do not match {}",
            digest_path.display()
        ));
    }
    Ok(())
}

fn parse_checksum_manifest(path: &Path) -> Result<BTreeMap<String, String>, String> {
    let contents = fs::read_to_string(path)
        .map_err(|error| format!("open checksum manifest {}: {error}", path.display()))?;
    let mut checksums = BTreeMap::new();
    for (index, raw) in contents.lines().enumerate() {
        let line = raw.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let fields: Vec<&str> = line.split_whitespace().collect();
        if fields.len() != 2 || !is_lower_hex_digest(fields[0]) {
            return Err(format!(
                "invalid checksum manifest entry at {}:{}",
                path.display(),
                index + 1
            ));
        }
        let filename = fields[1];
        let bare = !filename.contains(['/', '\\'])
            && Path::new(filename)
                .file_name()
                .map(|name| name.to_string_lossy() == filename)
                .unwrap_or(false);
        if filename.is_empty() || !bare || checksums.contains_key(filename) {
            return Err(format!(
                "invalid checksum manifest filename at {}:{}",
                path.display(),
                index + 1
            ));
        }
        checksums.insert(filename.to_owned(), fields[0].to_owned());
    }
    Ok(checksums)
}

fn manifest_file_names(path: &Path) -> Result<Vec<String>, String> {
    let checksums = parse_checksum_manifest(path)?;
    if checksums.is_empty() {
        return Err(format!(
            "futu proto checksum manifest contains no inputs: {}",
            path.display()
        ));
    }
    Ok(checksums.into_keys().collect())
}

fn manifest_file_differences(
    checksums: &BTreeMap<String, String>,
    expected: &[String],
) -> Option<String> {
    let missing: Vec<String> = expected
        .iter()
        .filter(|name| !checksums.contains_key(*name))
        .cloned()
        .collect();
    let unexpected: Vec<String> = checksums
        .keys()
        .filter(|name| !expected.contains(*name))
        .cloned()
        .collect();
    if missing.is_empty() && unexpected.is_empty() {
        return None;
    }
    let mut details = Vec::new();
    if !missing.is_empty() {
        details.push(format!("missing: {}", missing.join(", ")));
    }
    if !unexpected.is_empty() {
        details.push(format!("unexpected: {}", unexpected.join(", ")));
    }
    Some(format!(
        "futu proto checksum manifest does not match input list ({})",
        details.join("; ")
    ))
}

fn verify_repository_proto_names(
    tree_root: &Path,
    manifest_path: &Path,
) -> Result<Vec<String>, String> {
    let expected = manifest_file_names(manifest_path)?;
    let actual = tree_file_names(tree_root, ".proto")?;
    if expected != actual {
        return Err(format!(
            "checked-in Futu proto filenames do not match {}",
            manifest_path.display()
        ));
    }
    Ok(actual)
}

// Parity: go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:12 TestRepositoryDigestDetectsGeneratedOutputDrift
#[test]
fn frozen_proto_tree_matches_the_recorded_repository_digest() {
    let root = repository_root();
    let digest_path = root.join(DIGEST_FILE);
    let recorded = parse_repository_digest(&digest_path).expect("frozen repository digest");
    assert_eq!(recorded.proto.1, PROTO_FILE_COUNT);

    let (actual_digest, files) =
        digest_tree(&root.join(PROTO_DIRECTORY), ".proto").expect("digest the proto tree");
    assert_eq!(files.len(), PROTO_FILE_COUNT);
    assert_eq!(actual_digest, recorded.proto.0);
    verify_repository_digest(&digest_path, &root.join(PROTO_DIRECTORY))
        .expect("the checked-in proto tree verifies");

    // Drift: a tree that no longer matches the recorded digest fails closed.
    let drift = scratch_directory("digest-drift");
    fs::write(drift.join("Common.proto"), b"package Tampered;\n").expect("write drifted proto");
    let error =
        verify_repository_digest(&digest_path, &drift).expect_err("tampered tree must not verify");
    assert!(
        error.contains("checked-in OpenD generated outputs do not match"),
        "{error}"
    );
}

// Parity: go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:41 TestRepositoryDigestRejectsUnexpectedFilesAndProtoNames
#[test]
fn frozen_proto_names_and_extensions_match_the_checksum_manifest() {
    let root = repository_root();
    let manifest_path = root.join(MANIFEST_FILE);
    let names = verify_repository_proto_names(&root.join(PROTO_DIRECTORY), &manifest_path)
        .expect("the checked-in proto tree matches the manifest");
    assert_eq!(names.len(), PROTO_FILE_COUNT);
    assert!(names.iter().all(|name| name.ends_with(".proto")));

    let fixtures = scratch_directory("tree-extension");
    fs::write(fixtures.join("Unexpected.txt"), b"x").expect("write unexpected file");
    let error = tree_file_names(&fixtures, ".proto").expect_err("unexpected extension");
    assert!(
        error.contains("unexpected file in generated tree"),
        "{error}"
    );

    let nested = scratch_directory("tree-nested");
    fs::create_dir(nested.join("common")).expect("create nested directory");
    fs::write(nested.join("common").join("Common.proto"), b"").expect("write nested proto");
    let error = tree_file_names(&nested, ".proto").expect_err("nested proto");
    assert!(error.contains("must remain flat"), "{error}");

    let mismatched = scratch_directory("tree-mismatch");
    fs::write(mismatched.join("Other.proto"), b"").expect("write other proto");
    let error =
        verify_repository_proto_names(&mismatched, &manifest_path).expect_err("manifest mismatch");
    assert!(error.contains("filenames do not match"), "{error}");
}

// Parity: go:452dea11:cmd/generate-futu-proto/repository_verify_test.go:59 TestParseRepositoryDigestValidation
#[test]
fn frozen_digest_parser_rejects_malformed_entries() {
    let root = repository_root();
    let recorded = parse_repository_digest(&root.join(DIGEST_FILE)).expect("frozen digest parses");
    assert_eq!(recorded.proto.1, PROTO_FILE_COUNT);
    assert!(recorded.generated.1 > 0);

    let fixture = scratch_directory("digest-parser");
    let path = fixture.join("futu-proto-generated-10.9.6908.digest");
    let proto = "a".repeat(64);
    let generated = "b".repeat(64);

    fs::write(&path, format!("proto {} 1\n", "\0".repeat(64))).expect("write digest");
    assert!(parse_repository_digest(&path).is_err());

    fs::write(&path, format!("proto {proto} 1\n")).expect("write digest");
    let error = parse_repository_digest(&path).expect_err("missing pb entry");
    assert!(
        error.contains("must contain proto and pb entries"),
        "{error}"
    );

    fs::write(
        &path,
        format!("proto {proto} 1\nproto {proto} 1\npb {generated} 1\n"),
    )
    .expect("write digest");
    let error = parse_repository_digest(&path).expect_err("duplicate entry");
    assert!(
        error.contains("duplicate generated repository digest entry"),
        "{error}"
    );

    fs::write(&path, format!("proto {proto} 0\npb {generated} 1\n")).expect("write digest");
    let error = parse_repository_digest(&path).expect_err("zero count");
    assert!(
        error.contains("invalid generated repository file count"),
        "{error}"
    );
}

// Parity: go:452dea11:cmd/generate-futu-proto/rewrite_test.go:12 TestRewriteGoPackageReplacesOrInsertsOption
#[test]
fn frozen_proto_tree_keeps_the_generator_rewrite_postconditions() {
    let root = repository_root();
    let tree = root.join(PROTO_DIRECTORY);
    for name in tree_file_names(&tree, ".proto").expect("proto names") {
        let content = fs::read_to_string(tree.join(&name)).expect("UTF-8 proto");
        assert!(!content.contains('\r'), "{name} still contains CRLF");
        for (index, line) in content.lines().enumerate() {
            assert_eq!(
                line.trim_end_matches([' ', '\t']),
                line,
                "{name}:{} keeps trailing whitespace",
                index + 1
            );
        }
        let options: Vec<&str> = content
            .lines()
            .filter(|line| line.trim_start().starts_with("option go_package"))
            .collect();
        assert_eq!(options.len(), 1, "{name} must carry one go_package option");
        let package = content
            .lines()
            .find_map(|line| {
                let trimmed = line.trim();
                trimmed
                    .strip_prefix("package ")
                    .map(|rest| rest.trim_end_matches(';').trim().to_owned())
            })
            .unwrap_or_else(|| panic!("{name} has no package declaration"));
        let go_package = package.to_ascii_lowercase().replace('_', "");
        let expected =
            format!("github.com/jftrade/jftrade-main/pkg/futu/pb/{go_package};{go_package}");
        assert!(
            options[0].ends_with(&format!("\"{expected}\";")),
            "{name} go_package option = {}",
            options[0]
        );
    }
}

// Parity: go:452dea11:cmd/generate-futu-proto/manifest_test.go:15 TestParseChecksumManifest
// Parity: go:452dea11:cmd/generate-futu-proto/manifest_test.go:38 TestParseChecksumManifestRejectsInvalidEntries
#[test]
fn frozen_manifest_parser_rejects_invalid_entries() {
    let root = repository_root();
    let manifest_path = root.join(MANIFEST_FILE);
    let checksums = parse_checksum_manifest(&manifest_path).expect("frozen manifest parses");
    assert_eq!(checksums.len(), PROTO_FILE_COUNT);

    let fixture = scratch_directory("manifest-parser");
    let path = fixture.join("manifest.sha256");
    let digest = "a".repeat(64);
    fs::write(&path, format!("# comment\n\n{digest}  Common.proto\n")).expect("write manifest");
    let parsed = parse_checksum_manifest(&path).expect("comments and blank lines are skipped");
    assert_eq!(
        parsed.get("Common.proto").map(String::as_str),
        Some(&digest[..])
    );

    for (content, expected) in [
        (
            "ABC Common.proto\n".to_owned(),
            "invalid checksum manifest entry",
        ),
        (
            format!("{digest} nested/Common.proto\n"),
            "invalid checksum manifest filename",
        ),
        (
            format!("{digest} nested\\Common.proto\n"),
            "invalid checksum manifest filename",
        ),
        (
            format!("{digest} Common.proto\n{digest} Common.proto\n"),
            "invalid checksum manifest filename",
        ),
    ] {
        fs::write(&path, content).expect("write manifest");
        let error = parse_checksum_manifest(&path).expect_err("invalid manifest entry");
        assert!(error.contains(expected), "{error}");
    }
}

// Parity: go:452dea11:cmd/generate-futu-proto/manifest_test.go:25 TestManifestFileNamesReturnsSortedInputs
// Parity: go:452dea11:cmd/generate-futu-proto/manifest_test.go:60 TestValidateManifestFilesReportsAllDifferences
#[test]
fn frozen_manifest_names_match_the_checked_in_proto_tree() {
    let root = repository_root();
    let manifest_path = root.join(MANIFEST_FILE);
    let expected = manifest_file_names(&manifest_path).expect("sorted manifest names");
    let actual = tree_file_names(&root.join(PROTO_DIRECTORY), ".proto").expect("proto names");
    assert_eq!(expected, actual);
    assert_eq!(expected.len(), PROTO_FILE_COUNT);
    assert!(expected.windows(2).all(|pair| pair[0] < pair[1]));

    let fixture = scratch_directory("manifest-names");
    let unsorted = fixture.join("manifest.sha256");
    let digest = "a".repeat(64);
    fs::write(&unsorted, format!("{digest}  Z.proto\n{digest}  A.proto\n"))
        .expect("write manifest");
    assert_eq!(
        manifest_file_names(&unsorted).expect("sorted names"),
        vec!["A.proto".to_owned(), "Z.proto".to_owned()]
    );

    let empty = fixture.join("empty.sha256");
    fs::write(&empty, "# only a comment\n").expect("write empty manifest");
    let error = manifest_file_names(&empty).expect_err("empty manifest");
    assert!(error.contains("contains no inputs"), "{error}");

    let checksums = parse_checksum_manifest(&manifest_path).expect("frozen manifest parses");
    let differences = manifest_file_differences(
        &checksums,
        &["Common.proto".to_owned(), "Missing.proto".to_owned()],
    )
    .expect("differences");
    assert!(
        differences.contains("missing: Missing.proto"),
        "{differences}"
    );
    assert!(differences.contains("unexpected: "), "{differences}");
    assert!(
        manifest_file_differences(&checksums, &expected).is_none(),
        "the checked-in tree must match the manifest exactly"
    );
}
