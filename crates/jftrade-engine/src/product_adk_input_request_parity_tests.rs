//! Request-user contract tests for Go
//! `internal/assistant/engine/input_request_test.go` rows that are observable
//! through the model-facing tool declaration and the runtime argument
//! validator (`buildInputRequest` / `correctableInputArgsError`).

use std::collections::BTreeMap;

use serde_json::{Value, json};

use super::*;

fn valid_arguments() -> Value {
    json!({
        "decisionKind": "material_tradeoff",
        "blockingReason": "The selected option changes the result.",
        "title": "Choose a plan",
        "questions": [{
            "question": "Pick one",
            "options": [{"label": "A"}, {"label": "B"}],
            "allowOther": true,
        }],
    })
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:72
/// TestInputRequestToolRunReturnsCorrectableFeedbackForInvalidArgs — the
/// validator half.  The end-to-end feedback path is covered by
/// `an_invalid_request_user_call_returns_correctable_feedback_before_parking`.
/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:114
/// `TestInputRequestValidationAndErrorEdges` (validator half; declaration half
/// is `request_user_tool_declaration_publishes_the_two_or_three_option_budget`).
#[test]
fn request_user_arguments_accept_valid_calls_and_report_the_reference_errors() {
    assert_eq!(request_user_arguments_error(&valid_arguments()), None);

    let mut three_options = valid_arguments();
    three_options["questions"][0]["options"] = json!([
        {"label": "A"},
        {"label": "B"},
        {"label": "C"},
    ]);
    assert_eq!(request_user_arguments_error(&three_options), None);

    let mut four_options = valid_arguments();
    four_options["questions"][0]["options"] = json!([
        {"label": "A"},
        {"label": "B"},
        {"label": "C"},
        {"label": "D"},
    ]);
    let error = request_user_arguments_error(&four_options).expect("four options must be rejected");
    assert!(
        error.contains("requires two to 3 options"),
        "four-option error must name the option budget: {error}"
    );

    let mut typed = valid_arguments();
    typed["decisionKind"] = json!(42);
    assert_eq!(
        request_user_arguments_error(&typed).as_deref(),
        Some("decisionKind must be a string")
    );

    let cases: Vec<(Value, &str)> = vec![
        (
            json!({"blockingReason": "Required.", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "decisionKind must describe a supported blocking boundary",
        ),
        (
            json!({"decisionKind": "optional_next_step", "blockingReason": "Required.", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "decisionKind must describe a supported blocking boundary",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "blockingReason is required",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "This is an optional next step.", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "blockingReason describes a non-blocking optional next step",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "需要我继续吗？", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "blockingReason describes a non-blocking optional next step",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "Required.", "questions": [{"question": "Would you like me to continue?", "options": [{"label": "Yes"}, {"label": "No"}]}]}),
            "question 1 asks about a non-blocking optional next step",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "Required.", "questions": [{"question": " ", "options": [{"label": "A"}, {"label": "B"}]}]}),
            "question 1 is empty",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "Required.", "questions": [{"question": "Pick", "options": [{"label": "A"}]}]}),
            "question 1 requires two to 3 options",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "Required.", "questions": [{"question": "Pick", "options": [{"label": "A"}, {"label": " "}]}]}),
            "question 1 option 2 is empty",
        ),
        (
            json!({"decisionKind": "material_tradeoff", "blockingReason": "Required.", "questions": []}),
            "at least one question is required",
        ),
    ];
    for (arguments, expected) in cases {
        assert_eq!(
            request_user_arguments_error(&arguments).as_deref(),
            Some(expected),
            "arguments {arguments} must report {expected}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:556
/// TestRequestUserToolIsLongRunning — the declaration half: Go asserts the
/// published option schema caps at three choices and the guidance says
/// "two or three options".
#[test]
fn request_user_tool_declaration_publishes_the_two_or_three_option_budget() {
    let bindings = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| {
            (
                definition.adapter,
                crate::product::product_production_ports::ProductionAdapterBinding::Ready,
            )
        })
        .collect::<BTreeMap<_, _>>();
    let catalog =
        crate::product::product_production_ports::ProductionToolCatalog::from_bindings(&bindings)
            .expect("complete tool bindings");
    let declaration = catalog
        .openai_tools()
        .into_iter()
        .find(|tool| tool["name"] == "interaction.request_user")
        .expect("interaction.request_user declaration");

    let description = declaration["description"].as_str().unwrap_or_default();
    assert!(
        description.contains("two or three options"),
        "the declaration must guide the model to two or three options: {description}"
    );
    assert!(
        description.contains("originalRequest") && description.contains("continuationInstruction"),
        "the declaration must carry the resume contract: {description}"
    );

    let parameters = &declaration["parameters"];
    let options = &parameters["properties"]["questions"]["items"]["properties"]["options"];
    assert_eq!(options["minItems"], 2);
    assert_eq!(options["maxItems"], 3);
    assert_eq!(
        parameters["properties"]["questions"]["minItems"], 1,
        "at least one question is required"
    );
    assert_eq!(parameters["properties"]["blockingReason"]["minLength"], 1);
    let decision_kinds = parameters["properties"]["decisionKind"]["enum"]
        .as_array()
        .expect("decisionKind enum");
    for kind in [
        "missing_required_context",
        "material_tradeoff",
        "scope_boundary",
    ] {
        assert!(
            decision_kinds.iter().any(|value| value == kind),
            "decisionKind enum must publish {kind}: {decision_kinds:?}"
        );
    }
    assert_eq!(decision_kinds.len(), 3);
    let question_required = parameters["properties"]["questions"]["items"]["required"]
        .as_array()
        .expect("question required list");
    assert!(question_required.iter().any(|value| value == "question"));
    assert!(question_required.iter().any(|value| value == "options"));
}
