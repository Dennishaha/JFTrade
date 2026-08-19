use std::fs;
use std::path::PathBuf;

use jftrade_broker::{
    BrokerError, MarketSegment, OrderKind, ProductClass, QuantityMode, SnapshotAvailabilityError,
    SnapshotAvailabilityKind,
};
use serde::Deserialize;

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Corpus {
    taxonomies: Vec<TaxonomyCase>,
    broker_errors: Vec<BrokerErrorCase>,
    snapshot_availability: Vec<AvailabilityCase>,
}

#[derive(Deserialize)]
struct TaxonomyCase {
    #[serde(rename = "type")]
    kind: String,
    input: String,
    known: bool,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct BrokerErrorCase {
    broker_id: String,
    code: String,
    message: String,
    display: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct AvailabilityCase {
    kind: String,
    fallback_eligible: bool,
}

fn corpus() -> Corpus {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../tests/fixtures/rust-migration/stage2/foundation.json");
    serde_json::from_slice(&fs::read(path).expect("read foundation corpus"))
        .expect("decode foundation corpus")
}

#[test]
fn preserves_known_and_unknown_taxonomy_values() {
    for test in corpus().taxonomies {
        let (known, encoded) = match test.kind.as_str() {
            "productClass" => taxonomy_result(ProductClass::new(&test.input)),
            "marketSegment" => taxonomy_result(MarketSegment::new(&test.input)),
            "quantityMode" => taxonomy_result(QuantityMode::new(&test.input)),
            "orderKind" => taxonomy_result(OrderKind::new(&test.input)),
            other => panic!("unknown taxonomy {other}"),
        };
        assert_eq!(known, test.known, "{} {}", test.kind, test.input);
        assert_eq!(encoded, format!("\"{}\"", test.input));
    }
}

fn taxonomy_result<T>(value: T) -> (bool, String)
where
    T: Taxonomy + serde::Serialize,
{
    (
        value.is_known(),
        serde_json::to_string(&value).expect("serialize taxonomy"),
    )
}

trait Taxonomy {
    fn is_known(&self) -> bool;
}

macro_rules! impl_taxonomy {
    ($($kind:ty),+ $(,)?) => {
        $(impl Taxonomy for $kind {
            fn is_known(&self) -> bool { self.is_known() }
        })+
    };
}

impl_taxonomy!(ProductClass, MarketSegment, QuantityMode, OrderKind);

#[test]
fn preserves_broker_error_display_and_snapshot_fallback_rules() {
    let corpus = corpus();
    for test in corpus.broker_errors {
        let error = BrokerError::new(test.broker_id, test.code, test.message);
        assert_eq!(error.to_string(), test.display);
    }
    for test in corpus.snapshot_availability {
        let error = SnapshotAvailabilityError::new(
            SnapshotAvailabilityKind::new(test.kind),
            "provider unavailable",
        );
        assert_eq!(error.to_string(), "provider unavailable");
        assert_eq!(error.is_fallback_eligible(), test.fallback_eligible);
    }
}
