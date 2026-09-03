#![forbid(unsafe_code)]

use std::error::Error;
use std::path::PathBuf;

use jftrade_engine::api_transport_compatibility::{ApiTransportInput, ApiTransportReplay};

fn main() {
    if let Err(error) = execute() {
        eprintln!("jftrade-api-transport-replay: {error}");
        std::process::exit(1);
    }
}

fn execute() -> Result<(), Box<dyn Error>> {
    let input_path = parse_input(std::env::args().skip(1))?;
    let input: ApiTransportInput = serde_json::from_slice(&std::fs::read(input_path)?)?;
    let assembly = ApiTransportReplay::new(input.routes.clone())?;
    let output = assembly.evaluate(input)?;
    println!("{}", serde_json::to_string(&output)?);
    Ok(())
}

fn parse_input(mut args: impl Iterator<Item = String>) -> Result<PathBuf, &'static str> {
    match (args.next().as_deref(), args.next(), args.next()) {
        (Some("--input"), Some(path), None) => Ok(PathBuf::from(path)),
        _ => Err("usage: jftrade-api-transport-replay --input <corpus.json>"),
    }
}
