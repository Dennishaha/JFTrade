use std::collections::VecDeque;
use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Duration;

use jftrade_api::{ApiStream, SseEvent, encode_event, encode_retry};
use jftrade_store_sqlite::{AdkSessionStore, AdkStore, AdkStreamCursor};
use serde_json::{Value, json};
use tokio_stream::Stream;

pub(super) fn body(
    store: Arc<AdkStore>,
    sessions: Arc<AdkSessionStore>,
    cursor: AdkStreamCursor,
    after: u64,
) -> ApiStream {
    ApiStream::from_stream(Box::pin(ReconnectBody {
        store,
        sessions,
        cursor,
        after,
        retry: true,
        done: false,
        events: VecDeque::new(),
        delay: None,
    }))
}

// The HTTP consumer owns this reader. Drop releases the timer, unread page and
// store reference; no detached poller, second writer or recovery owner exists.
struct ReconnectBody {
    store: Arc<AdkStore>,
    sessions: Arc<AdkSessionStore>,
    cursor: AdkStreamCursor,
    after: u64,
    retry: bool,
    done: bool,
    events: VecDeque<(u64, Value)>,
    delay: Option<Pin<Box<tokio::time::Sleep>>>,
}

impl ReconnectBody {
    fn recover_final(&mut self, events: &mut Vec<(u64, Value)>) -> Result<(), io::Error> {
        use crate::product::product_adk_model_runtime::recover_terminal_stream_event;
        let Some(event) =
            recover_terminal_stream_event(&self.store, &self.sessions, &self.cursor.run_id)
                .map_err(|error| io::Error::other(format!("{error:?}")))?
        else {
            return Ok(());
        };
        let Some(sequence) = event["sequence"].as_u64() else {
            return Ok(());
        };
        if self.cursor.terminal_at_open {
            self.cursor.replay_until = self.cursor.replay_until.max(sequence);
        }
        if let Some((_, current)) = events.iter_mut().find(|(id, _)| *id == sequence) {
            if current["type"] == "error" {
                *current = event;
            }
        } else if self.done && sequence > self.after {
            events.push((sequence, event));
        }
        Ok(())
    }

    fn next_event(&mut self) -> Option<Result<Vec<u8>, io::Error>> {
        let (sequence, mut data) = self.events.pop_front()?;
        self.after = sequence;
        if let Some(object) = data.as_object_mut() {
            object.insert(
                "replay".to_owned(),
                json!(sequence <= self.cursor.replay_until),
            );
        }
        Some(
            encode_event(&SseEvent {
                id: Some(sequence.to_string()),
                data,
            })
            .map(String::into_bytes)
            .map_err(io::Error::other),
        )
    }
}

impl Stream for ReconnectBody {
    type Item = Result<Vec<u8>, io::Error>;

    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        if self.retry {
            self.retry = false;
            return Poll::Ready(Some(Ok(encode_retry(3000).into_bytes())));
        }
        if let Some(event) = self.next_event() {
            return Poll::Ready(Some(event));
        }
        if self.done {
            return Poll::Ready(None);
        }
        if let Some(delay) = self.delay.as_mut() {
            if delay.as_mut().poll(cx).is_pending() {
                return Poll::Pending;
            }
            self.delay = None;
        }
        match self
            .store
            .read_stream_page(&self.cursor.run_id, self.after, 64)
        {
            Ok(mut page) => {
                // A terminal status is observed under the same connection lock
                // as its page. Drain every page before ending the connection.
                self.done = page.events.len() < 64
                    && matches!(
                        page.status.to_ascii_uppercase().as_str(),
                        "COMPLETED" | "FAILED" | "CANCELLED" | "TIMED_OUT" | "DENIED" | "PENDING"
                    );
                if (self.done
                    || page
                        .events
                        .iter()
                        .any(|(_, event)| event["type"] == "error"))
                    && let Err(error) = self.recover_final(&mut page.events)
                {
                    self.done = true;
                    return Poll::Ready(Some(Err(error)));
                }
                self.events = page.events.into();
            }
            Err(error) => {
                self.done = true;
                return Poll::Ready(Some(Err(io::Error::other(error))));
            }
        }
        if let Some(event) = self.next_event() {
            return Poll::Ready(Some(event));
        }
        if self.done {
            return Poll::Ready(None);
        }
        let mut delay = Box::pin(tokio::time::sleep(Duration::from_millis(50)));
        let _ = delay.as_mut().poll(cx);
        self.delay = Some(delay);
        Poll::Pending
    }
}
