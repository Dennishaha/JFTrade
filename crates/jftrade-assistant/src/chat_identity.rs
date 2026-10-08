//! Provider-neutral request identity from the frozen Assistant contract.

use serde::{Deserialize, Serialize};

#[derive(Default, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Request {
    agent_id: Option<String>,
    session_id: Option<String>,
    message: Option<String>,
    provider_id: Option<String>,
    model: Option<String>,
    reasoning_effort_override: Option<String>,
    work_mode_override: Option<String>,
    permission_mode_override: Option<String>,
    objective: Option<String>,
    run_options: Option<RunOptions>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RunOptions {
    loop_max_iterations: Option<i64>,
}

// Field order is part of Go json.Marshal's fingerprint representation.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct CanonicalRequest {
    agent_id: String,
    session_id: String,
    message: String,
    provider_id: String,
    model: String,
    reasoning_effort_override: String,
    work_mode_override: String,
    permission_mode_override: String,
    objective: String,
    loop_max_iterations: i64,
}

/// Serialize identity fields in the frozen Go order, including defaults and
/// Go's HTML/line-separator escaping. UUID validation belongs to admission;
/// the client request ID and unrelated fields are excluded from the digest.
pub fn canonical_chat_request_json(body: &[u8]) -> Result<String, serde_json::Error> {
    // The HTTP decoder consumes the first JSON value, like Go json.Decoder.
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let request = Option::<Request>::deserialize(&mut decoder)?.unwrap_or_default();
    let message = trimmed(request.message);
    let objective = trimmed(request.objective);
    let reasoning = normalized(request.reasoning_effort_override);
    let work = normalized(request.work_mode_override);
    let permission = normalized(request.permission_mode_override);
    let iterations = request
        .run_options
        .and_then(|options| options.loop_max_iterations)
        .unwrap_or(0);
    let canonical = CanonicalRequest {
        agent_id: trimmed(request.agent_id),
        session_id: trimmed(request.session_id),
        message: message.clone(),
        provider_id: trimmed(request.provider_id),
        model: trimmed(request.model),
        reasoning_effort_override: match reasoning.as_str() {
            "low" | "medium" | "high" | "xhigh" | "max" => reasoning,
            _ => String::new(),
        },
        work_mode_override: mode(work, "chat", &["chat", "loop"]),
        permission_mode_override: mode(
            permission,
            "approval",
            &["approval", "less_approval", "all"],
        ),
        objective: if objective.is_empty() {
            message
        } else {
            objective
        },
        loop_max_iterations: if iterations <= 0 {
            5
        } else {
            iterations.min(20)
        },
    };
    Ok(serde_json::to_string(&canonical)?
        .replace('&', "\\u0026")
        .replace('<', "\\u003c")
        .replace('>', "\\u003e")
        .replace('\u{2028}', "\\u2028")
        .replace('\u{2029}', "\\u2029"))
}

fn trimmed(value: Option<String>) -> String {
    value.unwrap_or_default().trim().to_owned()
}

fn normalized(value: Option<String>) -> String {
    // Go strings.ToLower uses simple (one-rune) Unicode lowercase mappings.
    trimmed(value)
        .chars()
        .map(|character| character.to_lowercase().next().unwrap_or(character))
        .collect()
}

fn mode(value: String, default: &str, accepted: &[&str]) -> String {
    if value.is_empty() {
        default.to_owned()
    } else if accepted.contains(&value.as_str()) {
        value
    } else {
        format!("invalid:{value}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::{Value, json};

    #[test]
    fn canonical_identity_preserves_go_field_order_defaults_and_escaping() {
        let serialized = canonical_chat_request_json(
            br#"{"message":" <hello>&\u2028\u2029end ","agentId":" agent ","reasoningEffortOverride":" HIGH "}"#,
        ).unwrap();
        assert_eq!(
            serialized,
            r#"{"agentId":"agent","sessionId":"","message":"\u003chello\u003e\u0026\u2028\u2029end","providerId":"","model":"","reasoningEffortOverride":"high","workModeOverride":"chat","permissionModeOverride":"approval","objective":"\u003chello\u003e\u0026\u2028\u2029end","loopMaxIterations":5}"#
        );
    }

    #[test]
    fn canonical_identity_normalizes_equivalent_payload_and_distinguishes_each_field() {
        let baseline = canonical_chat_request_json(br#"{"message":"hello"}"#).unwrap();
        let equivalent = json!({"message":" hello ","agentId":null,"sessionId":" ",
            "providerId":"", "model":null,"reasoningEffortOverride":"invalid",
            "workModeOverride":" CHAT ","permissionModeOverride":" APPROVAL ",
            "objective":"hello","runOptions":{"loopMaxIterations":-1},
            "clientRequestId":"ignored","context":{"ignored":true}});
        assert_eq!(
            canonical_chat_request_json(&serde_json::to_vec(&equivalent).unwrap()).unwrap(),
            baseline
        );
        for change in [
            json!({"agentId":"a"}),
            json!({"sessionId":"s"}),
            json!({"message":"changed"}),
            json!({"providerId":"p"}),
            json!({"model":"m"}),
            json!({"reasoningEffortOverride":"low"}),
            json!({"reasoningEffortOverride":"high"}),
            json!({"workModeOverride":"loop"}),
            json!({"workModeOverride":"bad"}),
            json!({"permissionModeOverride":"all"}),
            json!({"permissionModeOverride":"bad"}),
            json!({"objective":"goal"}),
            json!({"runOptions":{"loopMaxIterations":6}}),
        ] {
            let mut body = json!({"message":"hello"});
            body.as_object_mut()
                .unwrap()
                .extend(change.as_object().unwrap().clone());
            assert_ne!(
                canonical_chat_request_json(&serde_json::to_vec(&body).unwrap()).unwrap(),
                baseline,
                "{change}"
            );
        }
        let upper: Value = serde_json::from_str(
            &canonical_chat_request_json(
                br#"{"runOptions":{"loopMaxIterations":21},"workModeOverride":"\u0130"}"#,
            )
            .unwrap(),
        )
        .unwrap();
        assert_eq!(upper["loopMaxIterations"], 20);
        assert_eq!(upper["workModeOverride"], "invalid:i");
    }

    #[test]
    fn canonical_identity_accepts_null_fields_and_rejects_malformed_field_types() {
        assert_eq!(
            canonical_chat_request_json(b"null").unwrap(),
            canonical_chat_request_json(b"{}").unwrap()
        );
        assert_eq!(
            canonical_chat_request_json(br#"{} {}"#).unwrap(),
            canonical_chat_request_json(b"{}").unwrap()
        );
        for body in [
            r#"{"message":1}"#,
            r#"{"runOptions":[]}"#,
            r#"{"runOptions":{"loopMaxIterations":1.5}}"#,
            r#"[]"#,
        ] {
            assert!(
                canonical_chat_request_json(body.as_bytes()).is_err(),
                "{body}"
            );
        }
    }
}
