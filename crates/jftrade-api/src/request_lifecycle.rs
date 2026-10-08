use axum::body::Body;
use axum::extract::Request;
use axum::http::Response;
use axum::middleware::Next;
use tokio::sync::watch;

use crate::session_lifecycle::wrap_sse;

/// A transport request's independent cancellation signal. It stays outside
/// the serialized ApiRequest and may be carried in the Axum request extensions.
#[derive(Clone, Debug)]
pub struct RequestCancellation(watch::Sender<bool>);

impl Default for RequestCancellation {
    fn default() -> Self {
        let (sender, _) = watch::channel(false);
        Self(sender)
    }
}

impl RequestCancellation {
    pub fn cancel(&self) {
        self.0.send_replace(true);
    }

    pub fn is_cancelled(&self) -> bool {
        *self.0.borrow()
    }

    pub async fn cancelled(self) {
        let mut receiver = self.0.subscribe();
        if *receiver.borrow() {
            return;
        }
        let _ = receiver.changed().await;
    }
}

pub(crate) async fn request_lifecycle(mut request: Request, next: Next) -> Response<Body> {
    let cancellation = request
        .extensions()
        .get::<RequestCancellation>()
        .cloned()
        .unwrap_or_default();
    request.extensions_mut().insert(cancellation.clone());
    let response = next.run(request).await;
    // The request owns this future and body. Check its signal before the first
    // body poll; cancellation releases the reader/producer without a task.
    wrap_sse(response, cancellation.cancelled())
}
