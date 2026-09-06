//! Canonical input answer representations and resume checkpoints for ADK.

use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct CanonicalInputAnswer {
    pub(crate) question_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) option_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) other_text: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub(crate) struct CanonicalInputAnswers {
    pub(crate) answers: Vec<CanonicalInputAnswer>,
}

impl CanonicalInputAnswers {
    pub(crate) fn from_values(values: &[Value]) -> Self {
        let mut answers = Vec::new();
        for val in values {
            let question_id = val
                .get("questionId")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned();
            if question_id.is_empty() {
                continue;
            }
            let option_id = val
                .get("optionId")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(str::to_owned);
            let other_text = val
                .get("otherText")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(str::to_owned);
            answers.push(CanonicalInputAnswer {
                question_id,
                option_id,
                other_text,
            });
        }
        Self { answers }
    }

    #[allow(dead_code)]
    pub(crate) fn to_values(&self) -> Vec<Value> {
        self.answers
            .iter()
            .map(|a| {
                let mut map = serde_json::Map::new();
                map.insert(
                    "questionId".to_owned(),
                    Value::String(a.question_id.clone()),
                );
                if let Some(ref opt) = a.option_id {
                    map.insert("optionId".to_owned(), Value::String(opt.clone()));
                }
                if let Some(ref other) = a.other_text {
                    map.insert("otherText".to_owned(), Value::String(other.clone()));
                }
                Value::Object(map)
            })
            .collect()
    }

    pub(crate) fn matches(&self, other: &Self) -> bool {
        if self.answers.len() != other.answers.len() {
            return false;
        }
        let mut a1 = self.answers.clone();
        let mut a2 = other.answers.clone();
        a1.sort_by(|x, y| x.question_id.cmp(&y.question_id));
        a2.sort_by(|x, y| x.question_id.cmp(&y.question_id));
        a1 == a2
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct InputResumeCheckpoint {
    pub(crate) request_id: String,
    pub(crate) answers: Vec<CanonicalInputAnswer>,
    pub(crate) tool_results: Value,
    pub(crate) resume_state: String,
    pub(crate) checkpoint_time: String,
}
