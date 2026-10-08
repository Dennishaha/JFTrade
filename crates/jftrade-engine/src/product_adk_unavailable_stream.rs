//! Terminal records for requests rejected before a model run can be created.

use std::collections::BTreeMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};

use jftrade_api::{ApiStream, SseEvent, encode_event, encode_retry};
use serde_json::{Value, json};

use crate::product::AdkReadLiveStream;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatLiveStream, AdkChatPortError, AdkChatPortOutput,
};

const RETENTION: Duration = Duration::from_secs(30 * 60);
pub(super) const UNAVAILABLE: &str =
    "assistant model runtime is unavailable; configure and attach a production model provider";

#[derive(Debug)]
struct TerminalRecord {
    id: String,
    request_body: Vec<u8>,
    expires_at: Instant,
    event: Value,
}

// This port owns the pre-run records. They never acquire a SQLite writer or
// spawn an execution worker; terminal publication precedes HTTP body polling.
#[derive(Debug, Default)]
pub(crate) struct UnavailableStreams(Mutex<BTreeMap<String, TerminalRecord>>);

impl UnavailableStreams {
    pub(super) fn replay(
        &self,
        input: &AdkChatInput,
    ) -> Result<Option<AdkChatPortOutput>, AdkChatPortError> {
        let mut records = self.0.lock().map_err(|_| unavailable())?;
        let now = Instant::now();
        records.retain(|_, record| record.expires_at > now);
        records
            .get(&input.client_request_id)
            .map(|record| {
                check_identity(record, input)?;
                Ok(output(record, true))
            })
            .transpose()
    }

    pub(super) fn dispatch(
        &self,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let mut records = self.0.lock().map_err(|_| unavailable())?;
        let now = Instant::now();
        records.retain(|_, record| record.expires_at > now);
        let (record, replay) = match records.entry(input.client_request_id.clone()) {
            std::collections::btree_map::Entry::Occupied(entry) => {
                check_identity(entry.get(), input)?;
                (entry.into_mut(), true)
            }
            std::collections::btree_map::Entry::Vacant(entry) => {
                let mut bytes = [0_u8; 16];
                getrandom::fill(&mut bytes).map_err(|_| unavailable())?;
                let id = format!(
                    "stream-{}",
                    bytes
                        .iter()
                        .map(|byte| format!("{byte:02x}"))
                        .collect::<String>()
                );
                let event =
                    json!({"type":"error", "streamId":id, "sequence":1, "message":UNAVAILABLE});
                (
                    entry.insert(TerminalRecord {
                        id,
                        request_body: input.body.clone(),
                        expires_at: now + RETENTION,
                        event,
                    }),
                    false,
                )
            }
        };
        Ok(output(record, replay))
    }

    pub(super) fn open(
        &self,
        id: &str,
        after: u64,
    ) -> Result<Option<AdkReadLiveStream>, AdkChatPortError> {
        let mut records = self.0.lock().map_err(|_| unavailable())?;
        let now = Instant::now();
        records.retain(|_, record| record.expires_at > now);
        Ok(records
            .values()
            .find(|record| record.id == id)
            .map(|record| AdkReadLiveStream {
                headers: vec![("X-ADK-Stream-ID".to_owned(), record.id.clone())],
                body: body(record, after, true),
            }))
    }
}

fn check_identity(record: &TerminalRecord, input: &AdkChatInput) -> Result<(), AdkChatPortError> {
    if record.request_body != input.body {
        return Err(AdkChatPortError::Conflict(format!(
            "clientRequestId {} was already used with a different chat request",
            input.client_request_id
        )));
    }
    Ok(())
}

fn output(record: &TerminalRecord, replay: bool) -> AdkChatPortOutput {
    AdkChatPortOutput::LiveStream(AdkChatLiveStream {
        headers: BTreeMap::from([("X-ADK-Stream-ID".to_owned(), record.id.clone())]),
        stream: body(record, 0, replay),
    })
}

fn unavailable() -> AdkChatPortError {
    AdkChatPortError::Unavailable(UNAVAILABLE.to_owned())
}

fn body(record: &TerminalRecord, after: u64, replay: bool) -> ApiStream {
    let mut event = record.event.clone();
    if replay {
        event["replay"] = json!(true);
    }
    let event = (after < 1).then_some(SseEvent {
        id: Some(format!("{}:1", record.id)),
        data: event,
    });
    ApiStream::from_chunks(std::iter::once(Ok(encode_retry(3000).into_bytes())).chain(
        event.into_iter().map(|event| {
            encode_event(&event)
                .map(String::into_bytes)
                .map_err(std::io::Error::other)
        }),
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn expired_unavailable_stream_removes_request_identity_and_allows_new_record() {
        let streams = UnavailableStreams::default();
        let input = AdkChatInput {
            client_request_id: "request-retention".to_owned(),
            body: b"hello".to_vec(),
        };
        streams.dispatch(&input).unwrap();
        let id = {
            let mut records = streams.0.lock().unwrap();
            let record = records.get_mut(&input.client_request_id).unwrap();
            record.expires_at = Instant::now() - Duration::from_secs(1);
            record.id.clone()
        };
        assert!(streams.open(&id, 0).unwrap().is_none());
        assert!(streams.0.lock().unwrap().is_empty());
        streams.dispatch(&input).unwrap();
        assert_ne!(streams.0.lock().unwrap()[&input.client_request_id].id, id);
    }
}
