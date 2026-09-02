use std::fs;
use std::path::PathBuf;

use jftrade_kernel::{DecimalText, Fixed8, WireTimestamp};
use serde::Deserialize;

#[derive(Deserialize)]
struct Corpus {
    decimal: Vec<DecimalCase>,
    fixed8: Vec<Fixed8Case>,
    timestamps: Vec<TimestampCase>,
}

#[derive(Deserialize)]
struct DecimalCase {
    input: String,
    canonical: String,
    json: String,
}

#[derive(Deserialize)]
struct Fixed8Case {
    input: String,
    scaled: i64,
    storage: String,
    json: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct TimestampCase {
    input: String,
    json: String,
    unix_millis: i64,
}

fn corpus() -> Corpus {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/rust-migration/stage2/foundation.json");
    serde_json::from_slice(&fs::read(path).expect("read foundation corpus"))
        .expect("decode foundation corpus")
}

#[test]
fn matches_decimal_and_fixed8_golden_values() {
    let corpus = corpus();
    for test in corpus.decimal {
        let value = test.input.parse::<DecimalText>().expect("parse decimal");
        assert_eq!(value.as_str(), test.canonical, "{}", test.input);
        assert_eq!(
            serde_json::to_string(&value).expect("serialize decimal"),
            test.json
        );
        assert_eq!(
            serde_json::from_str::<DecimalText>(&test.json).expect("deserialize decimal"),
            value
        );
    }
    for test in corpus.fixed8 {
        let value = test.input.parse::<Fixed8>().expect("parse fixed8");
        assert_eq!(value.scaled(), test.scaled, "{}", test.input);
        assert_eq!(value.storage_text(), test.storage, "{}", test.input);
        assert_eq!(
            serde_json::to_string(&value).expect("serialize fixed8"),
            test.json
        );
        assert_eq!(
            serde_json::from_str::<Fixed8>(&test.json).expect("deserialize fixed8"),
            value
        );
    }
}

#[test]
fn matches_rfc3339_and_unix_millisecond_golden_values() {
    for test in corpus().timestamps {
        let value = test
            .input
            .parse::<WireTimestamp>()
            .expect("parse timestamp");
        assert_eq!(
            serde_json::to_string(&value).expect("serialize timestamp"),
            test.json
        );
        assert_eq!(
            value.unix_millis().expect("Unix milliseconds"),
            test.unix_millis
        );
        assert_eq!(
            serde_json::from_str::<WireTimestamp>(&test.json).expect("deserialize timestamp"),
            value
        );
    }
}

#[test]
fn accepts_legacy_json_null_and_numeric_decimal_inputs_without_precision_loss() {
    let zero = serde_json::from_str::<DecimalText>("null").expect("decimal null");
    assert_eq!(zero.as_str(), "0");
    let large = serde_json::from_str::<DecimalText>("123456789012345678901234567890.123456789")
        .expect("large unquoted decimal");
    assert_eq!(large.as_str(), "123456789012345678901234567890.123456789");
    assert_eq!(
        serde_json::from_str::<Fixed8>("null").expect("fixed8 null"),
        Fixed8::ZERO
    );
}
