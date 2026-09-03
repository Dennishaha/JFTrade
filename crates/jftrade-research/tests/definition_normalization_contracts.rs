use jftrade_research::{DefinitionFieldError, normalize_definition_v2};
use serde::Deserialize;
use serde_json::Value;

const FIXTURE: &str = include_str!(
    "../../../tests/fixtures/compatibility/api-transport/research-definition-normalization.json"
);
const FIXTURE_VERSION: &str = "stage9.research-definition-normalization.v1";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    source_test: String,
    input: Value,
    normalized: Option<Value>,
    error: Option<DefinitionFieldError>,
}

#[test]
fn normalization_and_field_errors_match_the_go_owner_corpus() {
    let fixture: Fixture = serde_json::from_str(FIXTURE).expect("fixture");
    assert_eq!(fixture.version, FIXTURE_VERSION);
    assert_eq!(fixture.cases.len(), 42);
    for case in fixture.cases {
        assert!(
            !case.source_test.is_empty(),
            "{} has no Go source",
            case.name
        );
        match (
            normalize_definition_v2(case.input),
            case.normalized,
            case.error,
        ) {
            (Ok(actual), Some(expected), None) => {
                assert_eq!(actual, expected, "{} normalized output", case.name);
            }
            (Err(actual), None, Some(expected)) => {
                assert_eq!(actual, expected, "{} field error", case.name);
            }
            (actual, normalized, error) => panic!(
                "{} result shape differed: actual={actual:?} normalized={normalized:?} error={error:?}",
                case.name
            ),
        }
    }
}
