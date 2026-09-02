use std::fs;

use jftrade_settings::{
    DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT, InterfaceSettingsStorePort,
    normalize_live_websocket_connection_limit,
};
use jftrade_store_settings_file::SettingsFileStore;
use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct InterfaceSettingsCorpus {
    version: String,
    cases: Vec<InterfaceSettingsCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct InterfaceSettingsCase {
    name: String,
    document: Value,
    expected_limit: usize,
    expected_error: bool,
}

#[test]
fn live_websocket_limit_reads_go_settings_without_mutating_the_shadow_file() {
    let corpus: InterfaceSettingsCorpus = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/live-websocket-interface-settings.json"
    ))
    .expect("interface settings corpus");
    assert_eq!(
        corpus.version,
        "stage9.live-websocket-interface-settings.v1"
    );
    assert_eq!(DEFAULT_LIVE_WEBSOCKET_CONNECTION_LIMIT, 20);

    for test_case in corpus.cases {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("settings.json");
        fs::write(
            &path,
            serde_json::to_vec(&test_case.document).expect("encode settings case"),
        )
        .expect("seed settings");
        let before = fs::read(&path).expect("settings bytes");
        let opened = SettingsFileStore::open_read_only(&path);
        if test_case.expected_error {
            let error = opened.err().expect("malformed limit must fail");
            assert!(
                error.to_string().contains("decode interfaces"),
                "case {}: {error}",
                test_case.name
            );
            continue;
        }
        let store = opened.expect("open read-only settings");
        let settings = store
            .load_interface_settings()
            .expect("load interface settings");

        assert_eq!(
            normalize_live_websocket_connection_limit(settings.as_ref()),
            test_case.expected_limit,
            "case {}",
            test_case.name
        );
        assert_eq!(fs::read(&path).expect("settings bytes after read"), before);
    }
}
