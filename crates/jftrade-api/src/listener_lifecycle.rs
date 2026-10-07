use axum::Router;
use axum::body::Body;
use axum::extract::{Request, State};
use axum::http::{Response, StatusCode};
use axum::middleware::{self, Next};
use tokio::sync::watch;

use crate::envelope::empty_response;
use crate::session_lifecycle::wrap_sse;

#[derive(Clone)]
pub(crate) struct ListenerShutdown(watch::Receiver<bool>);

impl ListenerShutdown {
    pub(crate) fn is_stopped(&self) -> bool {
        *self.0.borrow() || self.0.has_changed().is_err()
    }

    async fn wait(mut self) {
        if self.is_stopped() {
            return;
        }
        let _ = self.0.changed().await;
    }
}

pub(crate) async fn wait_for_listener_shutdown(shutdown: Option<ListenerShutdown>) {
    match shutdown {
        Some(shutdown) => shutdown.wait().await,
        None => std::future::pending().await,
    }
}

/// Attach a listener owner's cancellation signal to HTTP and upgraded WS
/// connections before serving. Signalling this before graceful join releases
/// idle SSE bodies and websocket tasks on just this listener.
pub fn with_listener_shutdown(router: Router, shutdown: watch::Receiver<bool>) -> Router {
    router.layer(middleware::from_fn_with_state(
        ListenerShutdown(shutdown),
        listener_lifecycle,
    ))
}

async fn listener_lifecycle(
    State(shutdown): State<ListenerShutdown>,
    mut request: Request,
    next: Next,
) -> Response<Body> {
    request.extensions_mut().insert(shutdown.clone());
    tokio::select! {
        biased;
        _ = shutdown.clone().wait() => empty_response(StatusCode::SERVICE_UNAVAILABLE),
        response = next.run(request) => wrap_sse(response, shutdown.wait()),
    }
}
