#![forbid(unsafe_code)]

use std::io::{self, Write};

use jftrade_engine::{EngineConfig, start_engine};
use tracing::info;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env())
        .with_writer(io::stderr)
        .init();

    let handle = start_engine(EngineConfig::from_process_env()?).await?;
    let startup_json = serde_json::to_string(handle.startup_record())?;
    {
        let stdout = io::stdout();
        let mut output = stdout.lock();
        writeln!(output, "{startup_json}")?;
        output.flush()?;
    }

    info!(address = %handle.startup_record().address, "engine is ready");
    tokio::signal::ctrl_c().await?;
    info!("engine shutdown requested");
    handle.shutdown().await?;
    Ok(())
}
