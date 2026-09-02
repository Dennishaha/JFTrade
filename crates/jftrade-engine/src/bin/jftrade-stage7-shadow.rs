#![forbid(unsafe_code)]

use std::error::Error;
use std::path::PathBuf;

use jftrade_engine::stage7::{Stage7Assembly, Stage7Input};

fn main() {
    if let Err(error) = execute() {
        eprintln!("jftrade-stage7-shadow: {error}");
        std::process::exit(1);
    }
}

fn execute() -> Result<(), Box<dyn Error>> {
    let input_path = parse_input(std::env::args().skip(1))?;
    let input: Stage7Input = serde_json::from_slice(&std::fs::read(input_path)?)?;
    let assembly = Stage7Assembly::new(input.routes.clone())?;
    let output = assembly.evaluate(input)?;
    println!("{}", serde_json::to_string(&output)?);
    Ok(())
}

fn parse_input(mut args: impl Iterator<Item = String>) -> Result<PathBuf, &'static str> {
    match (args.next().as_deref(), args.next(), args.next()) {
        (Some("--input"), Some(path), None) => Ok(PathBuf::from(path)),
        _ => Err("usage: jftrade-stage7-shadow --input <corpus.json>"),
    }
}
