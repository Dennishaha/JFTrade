use sha2::{Digest, Sha256};

use super::product_adk_chat_stream_port::AdkChatPortError;

/// New requests use the frozen Go canonical digest. Old Rust rows retain
/// their raw-byte digest and can replay the exact original body without a
/// schema migration or a second state writer.
pub(super) struct ChatRequestIdentity {
    pub(super) canonical: String,
    legacy: String,
}

impl ChatRequestIdentity {
    pub(super) fn decode(body: &[u8]) -> Result<Self, AdkChatPortError> {
        let canonical = jftrade_assistant::canonical_chat_request_json(body).map_err(|error| {
            AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: format!("invalid chat payload: {error}"),
            }
        })?;
        Ok(Self {
            canonical: digest(canonical.as_bytes()),
            legacy: digest(body),
        })
    }

    pub(super) fn matches(&self, stored: &str) -> bool {
        stored == self.canonical || stored == self.legacy
    }
}

fn digest(bytes: &[u8]) -> String {
    Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}
