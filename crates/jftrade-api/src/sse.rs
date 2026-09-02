use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SseEvent {
    pub id: Option<String>,
    pub data: Value,
}

pub fn encode_retry(milliseconds: u64) -> String {
    if milliseconds == 0 {
        String::new()
    } else {
        format!("retry: {milliseconds}\n\n")
    }
}

pub fn encode_event(event: &SseEvent) -> Result<String, serde_json::Error> {
    let mut frame = String::new();
    if let Some(id) = event.id.as_deref() {
        frame.push_str("id: ");
        frame.push_str(id);
        frame.push('\n');
    }
    frame.push_str("data: ");
    frame.push_str(&serde_json::to_string(&event.data)?);
    frame.push_str("\n\n");
    Ok(frame)
}

pub fn encode_comment(comment: &str) -> String {
    format!(": {comment}\n\n")
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn frames_preserve_go_retry_id_data_and_comment_shape() {
        assert_eq!(encode_retry(3000), "retry: 3000\n\n");
        assert_eq!(
            encode_event(&SseEvent {
                id: Some("7".into()),
                data: json!({"type": "progress"}),
            })
            .expect("json"),
            "id: 7\ndata: {\"type\":\"progress\"}\n\n"
        );
        assert_eq!(encode_comment("heartbeat"), ": heartbeat\n\n");
    }
}
