use jftrade_assistant::{
    AssistantRuntime, InputAnswer, InputDecisionKind, InputOptionDraft, InputQuestionDraft,
    InputRequestDraft, InputRequestStatus, RunStatus, RuntimeError, Session,
};
use jftrade_kernel::WireTimestamp;

fn timestamp() -> WireTimestamp {
    "2026-08-19T00:00:00Z".parse().expect("fixture timestamp")
}

fn later_timestamp() -> WireTimestamp {
    "2026-08-19T00:00:01Z".parse().expect("fixture timestamp")
}

fn runtime_with_run() -> AssistantRuntime {
    let now = timestamp();
    let mut runtime = AssistantRuntime::default();
    runtime.save_session(Session {
        id: "session-input-validation".to_owned(),
        agent_id: "agent-input-validation".to_owned(),
        title: "Input validation contract".to_owned(),
        workflow_id: None,
        created_at: now,
        updated_at: now,
    });
    runtime
        .create_run(
            "run-input-validation",
            "session-input-validation",
            "agent-input-validation",
            now,
        )
        .expect("fixture run");
    runtime
}

fn option(label: &str) -> InputOptionDraft {
    InputOptionDraft {
        label: label.to_owned(),
        description: String::new(),
        recommended: false,
    }
}

fn question(text: &str, options: Vec<InputOptionDraft>, allow_other: bool) -> InputQuestionDraft {
    InputQuestionDraft {
        question: text.to_owned(),
        options,
        allow_other,
    }
}

fn valid_draft() -> InputRequestDraft {
    InputRequestDraft {
        decision_kind: InputDecisionKind::MaterialTradeoff,
        blocking_reason: "The selected mode changes the requested result.".to_owned(),
        title: "Choose a mode".to_owned(),
        questions: vec![question(
            "Which mode should be used?",
            vec![option("Paper"), option("Live")],
            true,
        )],
    }
}

fn two_question_draft() -> InputRequestDraft {
    InputRequestDraft {
        decision_kind: InputDecisionKind::MaterialTradeoff,
        blocking_reason: "The selected mode changes the requested result.".to_owned(),
        title: "Choose execution details".to_owned(),
        questions: vec![
            question(
                "Deployment mode?",
                vec![option("Safe"), option("Fast")],
                true,
            ),
            question(
                "Output format?",
                vec![option("Markdown"), option("JSON")],
                false,
            ),
        ],
    }
}

#[test]
fn input_request_draft_validation_matches_go_business_boundaries() {
    let cases = vec![
        (
            "missing blocking reason",
            InputRequestDraft {
                blocking_reason: "   ".to_owned(),
                ..valid_draft()
            },
            "blockingReason is required",
        ),
        (
            "non-blocking blocking reason",
            InputRequestDraft {
                blocking_reason: "Would you like me to continue?".to_owned(),
                ..valid_draft()
            },
            "non-blocking",
        ),
        (
            "missing question",
            InputRequestDraft {
                questions: Vec::new(),
                ..valid_draft()
            },
            "at least one question",
        ),
        (
            "empty question",
            InputRequestDraft {
                questions: vec![question("  ", vec![option("A"), option("B")], false)],
                ..valid_draft()
            },
            "question is empty",
        ),
        (
            "non-blocking question",
            InputRequestDraft {
                questions: vec![question(
                    "Which part would you like to see first?",
                    vec![option("A"), option("B")],
                    false,
                )],
                ..valid_draft()
            },
            "non-blocking",
        ),
        (
            "too few options",
            InputRequestDraft {
                questions: vec![question("Pick one", vec![option("A")], false)],
                ..valid_draft()
            },
            "two or three options",
        ),
        (
            "too many options",
            InputRequestDraft {
                questions: vec![question(
                    "Pick one",
                    vec![option("A"), option("B"), option("C"), option("D")],
                    false,
                )],
                ..valid_draft()
            },
            "two or three options",
        ),
        (
            "empty option label",
            InputRequestDraft {
                questions: vec![question("Pick one", vec![option("A"), option(" ")], false)],
                ..valid_draft()
            },
            "option labels must not be empty",
        ),
    ];

    for (name, draft, expected_message) in cases {
        let mut runtime = runtime_with_run();
        let error = runtime
            .request_input(
                "run-input-validation",
                format!("request-{name}"),
                "input-call",
                draft,
                timestamp(),
            )
            .expect_err(name);
        assert!(
            matches!(error, RuntimeError::InvalidInputRequest(ref message) if message.contains(expected_message)),
            "{name} error = {error:?}, want message containing {expected_message:?}"
        );
        assert_eq!(
            runtime.checkpoint().runs["run-input-validation"].status,
            RunStatus::Running,
            "{name} must not mutate run state"
        );
    }
}

#[test]
fn input_request_answers_reject_invalid_sets_and_resume_once() {
    let now = timestamp();
    let mut runtime = runtime_with_run();
    let draft = two_question_draft();
    let request = runtime
        .request_input(
            "run-input-validation",
            "input-request",
            "input-call",
            draft,
            now,
        )
        .expect("valid input request");
    assert_eq!(request.questions[0].id, "q1");
    assert_eq!(request.questions[0].options[1].id, "q1-o2");
    assert_eq!(request.questions[1].id, "q2");
    assert_eq!(
        runtime.checkpoint().runs["run-input-validation"].status,
        RunStatus::PendingInput
    );

    let invalid_answers = vec![
        (
            "missing question",
            vec![InputAnswer {
                question_id: "q1".to_owned(),
                option_id: "q1-o1".to_owned(),
                other_text: String::new(),
            }],
        ),
        (
            "unknown option",
            vec![
                InputAnswer {
                    question_id: "q1".to_owned(),
                    option_id: "missing".to_owned(),
                    other_text: String::new(),
                },
                InputAnswer {
                    question_id: "q2".to_owned(),
                    option_id: "q2-o1".to_owned(),
                    other_text: String::new(),
                },
            ],
        ),
        (
            "option and other together",
            vec![
                InputAnswer {
                    question_id: "q1".to_owned(),
                    option_id: "q1-o1".to_owned(),
                    other_text: "Balanced".to_owned(),
                },
                InputAnswer {
                    question_id: "q2".to_owned(),
                    option_id: "q2-o1".to_owned(),
                    other_text: String::new(),
                },
            ],
        ),
        (
            "other is not allowed",
            vec![
                InputAnswer {
                    question_id: "q1".to_owned(),
                    other_text: "Balanced".to_owned(),
                    option_id: String::new(),
                },
                InputAnswer {
                    question_id: "q2".to_owned(),
                    other_text: "XML".to_owned(),
                    option_id: String::new(),
                },
            ],
        ),
        (
            "duplicate question",
            vec![
                InputAnswer {
                    question_id: "q1".to_owned(),
                    option_id: "q1-o1".to_owned(),
                    other_text: String::new(),
                },
                InputAnswer {
                    question_id: "q1".to_owned(),
                    option_id: "q1-o2".to_owned(),
                    other_text: String::new(),
                },
            ],
        ),
        (
            "unknown question",
            vec![
                InputAnswer {
                    question_id: "unknown".to_owned(),
                    option_id: "q1-o1".to_owned(),
                    other_text: String::new(),
                },
                InputAnswer {
                    question_id: "q2".to_owned(),
                    option_id: "q2-o1".to_owned(),
                    other_text: String::new(),
                },
            ],
        ),
    ];

    for (name, answers) in invalid_answers {
        assert_eq!(
            runtime.answer_input(
                "run-input-validation",
                &request.id,
                answers,
                later_timestamp(),
            ),
            Err(RuntimeError::InvalidInputAnswers),
            "{name} answers must be rejected"
        );
        let run = &runtime.checkpoint().runs["run-input-validation"];
        assert_eq!(
            run.status,
            RunStatus::PendingInput,
            "{name} changed run status"
        );
        assert_eq!(run.input_requests[0].status, InputRequestStatus::Pending);
    }

    let answers = vec![
        InputAnswer {
            question_id: "q2".to_owned(),
            option_id: "q2-o2".to_owned(),
            other_text: String::new(),
        },
        InputAnswer {
            question_id: "q1".to_owned(),
            option_id: String::new(),
            other_text: "Balanced".to_owned(),
        },
    ];
    assert!(
        runtime
            .answer_input("run-input-validation", &request.id, answers.clone(), now,)
            .expect("valid answer")
    );
    assert!(
        !runtime
            .answer_input(
                "run-input-validation",
                &request.id,
                answers,
                later_timestamp(),
            )
            .expect("idempotent answer replay")
    );

    let run = &runtime.checkpoint().runs["run-input-validation"];
    assert_eq!(run.status, RunStatus::Running);
    assert!(run.input_request.is_none());
    assert_eq!(run.input_requests[0].status, InputRequestStatus::Answered);
    assert_eq!(run.input_requests[0].answers.len(), 2);
    assert_eq!(run.input_requests[0].answers[0].question_id, "q2");
    assert_eq!(run.input_requests[0].answers[1].other_text, "Balanced");
}
