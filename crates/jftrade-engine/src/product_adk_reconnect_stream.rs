use std::collections::VecDeque;
use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Duration;

use jftrade_api::{ApiStream, SseEvent, encode_event, encode_retry};
use jftrade_store_sqlite::{AdkStore, AdkStreamCursor};
use serde_json::{Value, json};
use tokio_stream::Stream;

pub(super) fn body(store: Arc<AdkStore>, cursor: AdkStreamCursor, after: u64) -> ApiStream {
    ApiStream::from_stream(Box::pin(ReconnectBody {
        store,
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
    cursor: AdkStreamCursor,
    after: u64,
    retry: bool,
    done: bool,
    events: VecDeque<(u64, Value)>,
    delay: Option<Pin<Box<tokio::time::Sleep>>>,
}

impl ReconnectBody {
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
            Ok(page) => {
                // A terminal status is observed under the same connection lock
                // as its page. Drain every page before ending the connection.
                self.done = page.events.len() < 64
                    && matches!(
                        page.status.to_ascii_uppercase().as_str(),
                        "COMPLETED" | "FAILED" | "CANCELLED" | "TIMED_OUT" | "DENIED" | "PENDING"
                    );
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
