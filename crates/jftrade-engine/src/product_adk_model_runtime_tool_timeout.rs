// Tool-deadline enforcement and panic recovery for the ADK tool loop.
//
// Extracted from `product_adk_model_runtime_tool_loop.rs` so the production
// fragment stays under the workspace 800-line architecture limit. It is a
// module-scope `include!`, so every import and helper of the parent module is
// still in scope.

/// Go's `executeRegisteredTool` wraps every handler in a 30s context and
/// recovers panics into `tool panic: %v`.  Rust runs the adapter on a worker
/// thread so a hanging tool cannot wedge the loop, honours the caller's
/// cancellation probe plus the tool deadline, and reports the reference's
/// exact `context.DeadlineExceeded`/`context.Canceled` text so
/// [`classify_tool_failure`] maps them to `TIMEOUT`/`CANCELLED`.
fn tool_timeout_error() -> AdkChatPortError {
    AdkChatPortError::Failed {
        // `context.DeadlineExceeded.Error()`.
        status: 504,
        code: "TOOL_EXECUTION_TIMEOUT".to_owned(),
        message: "context deadline exceeded".to_owned(),
    }
}

fn tool_cancelled_error() -> AdkChatPortError {
    AdkChatPortError::Failed {
        // `context.Canceled.Error()`.
        status: 499,
        code: "TOOL_EXECUTION_CANCELLED".to_owned(),
        message: "context canceled".to_owned(),
    }
}

fn panic_text(payload: Box<dyn std::any::Any + Send>) -> String {
    match payload.downcast::<String>() {
        Ok(message) => *message,
        Err(payload) => match payload.downcast::<&'static str>() {
            Ok(message) => (*message).to_owned(),
            Err(_) => "unknown panic".to_owned(),
        },
    }
}

fn execute_tool_handler(
    executor: &dyn AdkToolExecutor,
    name: &str,
    arguments: &Value,
    invocation: Option<&AdkToolInvocationContext>,
    cancelled: &dyn Fn() -> bool,
) -> Result<Value, AdkChatPortError> {
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| match invocation {
        Some(context) => executor.execute_with_context(name, arguments, context, cancelled),
        None => executor.execute_cancellable(name, arguments, cancelled),
    }));
    if invocation.is_some_and(AdkToolInvocationContext::key_consumption_missing) {
        return Err(AdkChatPortError::Failed {
            status: 500,
            code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
            message: format!("keyed tool {name} did not consume its invocation idempotency key"),
        });
    }
    match outcome {
        Ok(Ok(value)) => Ok(value),
        Ok(Err(message)) => Err(tool_failed(message)),
        Err(payload) => Err(tool_failed(format!("tool panic: {}", panic_text(payload)))),
    }
}

/// Execute one adapter call under Go's tool deadline with panic recovery.
///
/// The worker is detached when the deadline expires: Go cannot kill a running
/// goroutine either, and the durable claim fencing already stops a late result
/// from being committed by a stale owner.
fn execute_tool_with_timeout(
    executor: &Arc<dyn AdkToolExecutor>,
    name: &str,
    arguments: &Value,
    invocation: Option<&AdkToolInvocationContext>,
    cancelled: Arc<dyn Fn() -> bool + Send + Sync>,
    timeout: Duration,
) -> Result<Value, AdkChatPortError> {
    if cancelled() {
        return Err(tool_cancelled_error());
    }
    if invocation.is_some_and(|context| !context.has_key()) {
        return Err(tool_failed("missing idempotency key"));
    }
    let deadline = Instant::now() + timeout;
    let executor = Arc::clone(executor);
    let tool_name = name.to_owned();
    let tool_arguments = arguments.clone();
    let invocation = invocation.cloned();
    let worker_cancelled = Arc::clone(&cancelled);
    let (sender, receiver) = mpsc::channel();
    let worker = thread::Builder::new()
        .name("jftrade-adk-tool-exec".to_owned())
        .spawn(move || {
            let tool_deadline = move || worker_cancelled() || Instant::now() >= deadline;
            let outcome = execute_tool_handler(executor.as_ref(), &tool_name, &tool_arguments,
                invocation.as_ref(), &tool_deadline);
            let _ = sender.send(outcome);
        })
        .map_err(|error| unavailable(format!("assistant tool worker: {error}")))?;
    let mut completed = false;
    let result = loop {
        if cancelled() {
            break Err(tool_cancelled_error());
        }
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            break Err(tool_timeout_error());
        }
        match receiver.recv_timeout(remaining.min(Duration::from_millis(50))) {
            Ok(outcome) => {
                completed = true;
                break outcome;
            }
            Err(mpsc::RecvTimeoutError::Timeout) => continue,
            Err(mpsc::RecvTimeoutError::Disconnected) => {
                completed = true;
                break Err(tool_failed("tool worker stopped without a result"));
            }
        }
    };
    if completed {
        let _ = worker.join();
    }
    // Go re-reads `toolCtx.Err()` after the handler returns, so a result that
    // raced the deadline or a cancellation loses to that context error.
    if cancelled() {
        return Err(tool_cancelled_error());
    }
    if Instant::now() >= deadline {
        return Err(tool_timeout_error());
    }
    result
}
