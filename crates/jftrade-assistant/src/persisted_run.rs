//! Type checks for the persisted chat run before idempotency comparison.
//!
//! Keep this separate from public Run decoding: persisted rows allow missing
//! fields, null values, unknown runtime extensions and arbitrary status text.
//! Visit every occurrence of a field so a later duplicate cannot hide an
//! earlier type error, as in Go's persistedRun/json.Unmarshal boundary.

use serde::de::{DeserializeSeed, IgnoredAny, MapAccess, SeqAccess, Visitor};
use serde::{Deserialize, Deserializer};
use serde_json::Value;

#[derive(Clone, Copy)]
enum Shape {
    Text,
    Integer,
    Boolean,
    Any,
    Map,
    Object(&'static [(&'static str, Shape)]),
    Array(&'static Shape),
}

use Shape::{Any, Array, Boolean, Integer, Map, Object, Text};

const TOOL: Shape = Object(&[
    ("id", Text),
    ("runId", Text),
    ("toolName", Text),
    ("permission", Text),
    ("status", Text),
    ("input", Map),
    ("output", Any),
    ("error", Text),
    ("requiresUser", Boolean),
    ("idempotencyKey", Text),
    ("createdAt", Text),
    ("startedAt", Text),
    ("updatedAt", Text),
    ("completedAt", Text),
    ("durationMs", Integer),
]);

const APPROVAL: Shape = Object(&[
    ("id", Text),
    ("runId", Text),
    ("agentId", Text),
    ("toolName", Text),
    ("input", Map),
    ("status", Text),
    ("reason", Text),
    ("functionCallId", Text),
    ("confirmationCallId", Text),
    ("createdAt", Text),
    ("updatedAt", Text),
]);

const OPTION: Shape = Object(&[
    ("id", Text),
    ("label", Text),
    ("description", Text),
    ("recommended", Boolean),
]);

const QUESTION: Shape = Object(&[
    ("id", Text),
    ("question", Text),
    ("options", Array(&OPTION)),
    ("allowOther", Boolean),
]);

const ANSWER: Shape = Object(&[
    ("questionId", Text),
    ("optionId", Text),
    ("otherText", Text),
]);

const INPUT: Shape = Object(&[
    ("id", Text),
    ("runId", Text),
    ("agentId", Text),
    ("functionCallId", Text),
    ("title", Text),
    ("status", Text),
    ("questions", Array(&QUESTION)),
    ("answers", Array(&ANSWER)),
    ("createdAt", Text),
    ("updatedAt", Text),
    ("answeredAt", Text),
]);

const WORKFLOW: Shape = Object(&[
    ("taskId", Text),
    ("title", Text),
    ("description", Text),
    ("message", Text),
    ("status", Text),
    ("childRunId", Text),
    ("childAgentId", Text),
    ("childProviderId", Text),
    ("childModel", Text),
    ("childPermissionMode", Text),
    ("dependsOn", Array(&Text)),
    ("iteration", Integer),
    ("order", Integer),
    ("modeHint", Text),
    ("agentRole", Text),
    ("plannerStepId", Text),
    ("planSource", Text),
    ("workflowMode", Text),
    ("objective", Text),
    ("executor", Text),
    ("resultSummary", Text),
    ("plannerWarnings", Array(&Text)),
    ("nodeName", Text),
    ("nodeStatus", Text),
    ("routes", Array(&Text)),
    ("outputSummary", Text),
]);

const USAGE: Shape = Object(&[
    ("modelCalls", Integer),
    ("toolCallsTotal", Integer),
    ("durationMs", Integer),
    ("tokensIn", Integer),
    ("tokensOut", Integer),
]);

const RUN: Shape = Object(&[
    ("id", Text),
    ("sessionId", Text),
    ("agentId", Text),
    ("providerId", Text),
    ("providerName", Text),
    ("model", Text),
    ("reasoningEffort", Text),
    ("reasoningEffortField", Text),
    ("reasoningEffortValue", Text),
    ("maxDurationMs", Integer),
    ("status", Text),
    ("message", Text),
    ("userMessage", Text),
    ("preToolContent", Text),
    ("preToolReasoning", Text),
    ("toolSummaries", Array(&Text)),
    ("failureReason", Text),
    ("errorCode", Text),
    ("degraded", Boolean),
    ("optimizationTaskId", Text),
    ("workMode", Text),
    ("permissionMode", Text),
    ("objective", Text),
    ("parentRunId", Text),
    ("childRunIds", Array(&Text)),
    ("iteration", Integer),
    ("workflowStatus", Text),
    ("workflowEngine", Text),
    ("workflowCursor", Integer),
    ("workflowPlan", Array(&WORKFLOW)),
    ("toolCalls", Array(&TOOL)),
    ("pendingApprovals", Array(&APPROVAL)),
    ("inputRequest", INPUT),
    ("inputRequests", Array(&INPUT)),
    ("resumeState", Text),
    ("pauseRequestedAt", Text),
    ("pausedAt", Text),
    ("pausedReason", Text),
    ("finalMessageId", Text),
    ("usage", USAGE),
    ("createdAt", Text),
    ("startedAt", Text),
    ("updatedAt", Text),
    ("completedAt", Text),
    ("cancelledAt", Text),
]);

/// Validate persisted run field types without changing the authoritative row.
/// Unknown fields are runtime extensions; null has the persisted zero-value
/// semantics. Integer fields retain the signed 64-bit Go storage boundary.
pub fn validate_persisted_run_payload(raw: &str) -> Result<(), serde_json::Error> {
    let mut decoder = serde_json::Deserializer::from_str(raw);
    RUN.deserialize(&mut decoder)?;
    decoder.end()
}

impl<'de> DeserializeSeed<'de> for Shape {
    type Value = ();

    fn deserialize<D: Deserializer<'de>>(self, decoder: D) -> Result<(), D::Error> {
        match self {
            Text => Option::<String>::deserialize(decoder).map(drop),
            Integer => Option::<i64>::deserialize(decoder).map(drop),
            Boolean => Option::<bool>::deserialize(decoder).map(drop),
            Any => validate_untyped_numbers(Value::deserialize(decoder)?),
            Map => {
                let values =
                    Option::<std::collections::BTreeMap<String, Value>>::deserialize(decoder)?;
                for value in values.into_iter().flat_map(|values| values.into_values()) {
                    validate_untyped_numbers(value)?;
                }
                Ok(())
            }
            Object(_) | Array(_) => decoder.deserialize_any(self),
        }
    }
}

// Workspace serde_json enables arbitrary_precision. Go's interface{} numbers
// instead decode as float64, and overflow is an error even inside an output.
fn validate_untyped_numbers<E: serde::de::Error>(value: Value) -> Result<(), E> {
    match value {
        Value::Number(number) if number.as_f64().is_none_or(|value| !value.is_finite()) => {
            Err(E::custom("persisted untyped number exceeds float64 range"))
        }
        Value::Array(values) => values.into_iter().try_for_each(validate_untyped_numbers),
        Value::Object(values) => values.into_values().try_for_each(validate_untyped_numbers),
        _ => Ok(()),
    }
}

impl<'de> Visitor<'de> for Shape {
    type Value = ();

    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str(match self {
            Object(_) => "a persisted run object or null",
            _ => "a persisted run array or null",
        })
    }

    fn visit_unit<E: serde::de::Error>(self) -> Result<(), E> {
        Ok(())
    }

    fn visit_map<M: MapAccess<'de>>(self, mut map: M) -> Result<(), M::Error> {
        let Object(fields) = self else {
            return Err(serde::de::Error::invalid_type(
                serde::de::Unexpected::Map,
                &self,
            ));
        };
        while let Some(key) = map.next_key::<String>()? {
            if let Some((_, field)) = fields
                .iter()
                .find(|(name, _)| folded_name(&key) == folded_name(name))
            {
                map.next_value_seed(*field)?;
            } else {
                map.next_value::<IgnoredAny>()?;
            }
        }
        Ok(())
    }

    fn visit_seq<S: SeqAccess<'de>>(self, mut sequence: S) -> Result<(), S::Error> {
        let Array(item) = self else {
            return Err(serde::de::Error::invalid_type(
                serde::de::Unexpected::Seq,
                &self,
            ));
        };
        while sequence.next_element_seed(*item)?.is_some() {}
        Ok(())
    }
}

// Go encoding/json folds ASCII names plus the Unicode long-s and Kelvin
// equivalents. All persisted schema names above are ASCII.
fn folded_name(name: &str) -> String {
    name.chars()
        .map(|character| match character {
            '\u{017f}' => 's',
            '\u{212a}' => 'k',
            value => value.to_ascii_lowercase(),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::validate_persisted_run_payload;

    #[test]
    fn persisted_run_types_reject_nested_corruption_and_earlier_duplicate_errors() {
        for raw in [
            r#"{"message":42,"message":"valid later"}"#,
            r#"{"Meſſage":false}"#,
            r#"{"toolCalls":[{"durationMs":1.0}]}"#,
            r#"{"toolCalls":[{"output":1e309}]}"#,
            r#"{"pendingApprovals":[{"input":false}]}"#,
            r#"{"inputRequest":{"questions":[{"allowOther":"false"}]}}"#,
            r#"{"workflowPlan":[{"dependsOn":[1]}]}"#,
            r#"{"usage":{"modelCalls":-9223372036854775809}}"#,
            r#"{} {}"#,
            "[]",
        ] {
            assert!(validate_persisted_run_payload(raw).is_err(), "{raw}");
        }
    }

    #[test]
    fn persisted_run_types_accept_zero_values_and_unknown_runtime_extensions() {
        for raw in [
            "null",
            "{}",
            r#"{"MESSAGE":"first","message":"second","message":null}"#,
            r#"{"toolCalls":[null,{"Output":{"anything":[true,1,null]}}]}"#,
            r#"{"inputRequest":{"questions":[null,{"options":[null]}]}}"#,
            r#"{"toolSummaries":[null,"summary"],"usage":{"durationMs":-1}}"#,
            r#"{"future":1e309,"response":{"message":false}}"#,
        ] {
            validate_persisted_run_payload(raw).unwrap_or_else(|error| panic!("{raw}: {error}"));
        }
    }
}
