#![forbid(unsafe_code)]

use std::io::{self, Write};

use jftrade_engine::product_runtime::ProductRuntimeBuilder;
use tracing::info;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .with_writer(io::stderr)
        .init();
    // Register the supervisor signal before binding the API listener.  The
    // launcher contract permits a stop immediately after the port accepts a
    // probe, so installing the handler only after `start()` leaves a small
    // window where SIGTERM uses the process-default termination action.
    let stop_signal = tokio::spawn(wait_for_stop_request());
    let handle = match ProductRuntimeBuilder::from_process_env()?.start().await {
        Ok(handle) => handle,
        Err(error) => {
            stop_signal.abort();
            return Err(error.into());
        }
    };
    let startup_json = serde_json::to_string(handle.startup_record())?;
    {
        let stdout = io::stdout();
        let mut output = stdout.lock();
        writeln!(output, "{startup_json}")?;
        output.flush()?;
    }
    info!(address = %handle.startup_record().address, "Rust product API slice is ready");
    stop_signal.await??;
    handle.shutdown().await?;
    Ok(())
}

/// Waits for the stop requests the previous launcher handled through
/// `signal.NotifyContext`: Ctrl-C on every platform, plus SIGTERM on Unix,
/// which is what service supervisors send when they stop the sidecar.
async fn wait_for_stop_request() -> io::Result<()> {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};
        let mut terminate = signal(SignalKind::terminate())?;
        tokio::select! {
            result = tokio::signal::ctrl_c() => result,
            _ = terminate.recv() => Ok(()),
        }
    }
    #[cfg(not(unix))]
    {
        tokio::signal::ctrl_c().await
    }
}
