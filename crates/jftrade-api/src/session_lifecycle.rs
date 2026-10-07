use std::future::{Future, pending};
use std::pin::Pin;
use std::task::{Context, Poll};

use axum::body::{Body, Bytes};
use axum::http::{HeaderMap, Response};
use tokio::sync::watch;
use tokio_stream::Stream;

use crate::AccessPolicy;

#[derive(Clone)]
pub(crate) struct SessionRevocation(watch::Receiver<u64>);

impl SessionRevocation {
    pub(crate) fn subscribe(policy: &AccessPolicy, headers: &HeaderMap) -> Option<Self> {
        if policy.desktop_trusted(headers) || policy.session_cookie(headers).is_none() {
            return None;
        }
        policy
            .session_validator
            .as_ref()?
            .subscribe_revocation()
            .map(Self)
    }

    pub(crate) fn is_revoked(&self) -> bool {
        self.0.has_changed().unwrap_or(true)
    }

    pub(crate) async fn wait(mut self) {
        let _ = self.0.changed().await;
    }

    pub(crate) fn wrap_sse(self, response: Response<Body>) -> Response<Body> {
        wrap_sse(response, self.wait())
    }
}

pub(crate) fn wrap_sse(
    response: Response<Body>,
    revoked: impl Future<Output = ()> + Send + 'static,
) -> Response<Body> {
    let is_sse = response
        .headers()
        .get("content-type")
        .and_then(|value| value.to_str().ok())
        .is_some_and(|value| value.split(';').next() == Some("text/event-stream"));
    if !is_sse {
        return response;
    }
    let (parts, body) = response.into_parts();
    let stream = RevocableStream {
        body: Some(Box::pin(body.into_data_stream())),
        revoked: Box::pin(revoked),
    };
    Response::from_parts(parts, Body::from_stream(stream))
}

pub(crate) async fn wait_for_revocation(revocation: Option<SessionRevocation>) {
    match revocation {
        Some(revocation) => revocation.wait().await,
        None => pending().await,
    }
}

type BodyStream = Pin<Box<dyn Stream<Item = Result<Bytes, axum::Error>> + Send>>;

struct RevocableStream {
    body: Option<BodyStream>,
    revoked: Pin<Box<dyn Future<Output = ()> + Send>>,
}

impl Stream for RevocableStream {
    type Item = Result<Bytes, axum::Error>;

    fn poll_next(self: Pin<&mut Self>, context: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        let this = self.get_mut();
        if this.revoked.as_mut().poll(context).is_ready() {
            // Drop the receiver immediately; blocked producers observe closure
            // even if the HTTP server retains the exhausted response body.
            this.body = None;
            return Poll::Ready(None);
        }
        match this.body.as_mut() {
            Some(body) => body.as_mut().poll_next(context),
            None => Poll::Ready(None),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ApiStream;
    use tokio_stream::StreamExt;

    #[tokio::test]
    async fn revoked_idle_sse_drops_its_producer_without_waiting_for_a_frame() {
        let (generation, receiver) = watch::channel(0);
        let (stream, producer) = ApiStream::channel(1);
        let response = Response::builder()
            .header("content-type", "text/event-stream")
            .body(Body::from_stream(stream.take_body().expect("body")))
            .expect("response");
        let mut body = SessionRevocation(receiver)
            .wrap_sse(response)
            .into_body()
            .into_data_stream();
        // The revocation happens before the first body poll, pinning the
        // admission-to-body race as well as the idle producer path.
        generation.send_modify(|value| *value += 1);
        assert!(
            tokio::time::timeout(std::time::Duration::from_secs(1), body.next())
                .await
                .expect("revoked body deadline")
                .is_none()
        );
        assert!(
            producer
                .send(b"must not escape revocation".to_vec())
                .is_err()
        );
    }

    #[tokio::test]
    async fn session_owner_disappearance_ends_pending_sse() {
        let (generation, receiver) = watch::channel(0);
        let (stream, producer) = ApiStream::channel(1);
        let response = Response::builder()
            .header("content-type", "text/event-stream")
            .body(Body::from_stream(stream.take_body().expect("body")))
            .expect("response");
        let mut body = SessionRevocation(receiver)
            .wrap_sse(response)
            .into_body()
            .into_data_stream();
        drop(generation);
        assert!(
            tokio::time::timeout(std::time::Duration::from_secs(1), body.next())
                .await
                .expect("owner drop deadline")
                .is_none()
        );
        assert!(producer.send(b"orphaned frame".to_vec()).is_err());
    }
}
