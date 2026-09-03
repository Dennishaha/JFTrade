use std::env;
use std::fs;
use std::process::ExitCode;

use jftrade_desktop::{DesktopRuntimeInput, evaluate_desktop_runtime};

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("{error}");
            ExitCode::FAILURE
        }
    }
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let mut arguments = env::args().skip(1);
    let flag = arguments.next().ok_or("missing --input")?;
    if flag != "--input" {
        return Err(format!("unexpected argument {flag:?}; expected --input").into());
    }
    let path = arguments.next().ok_or("missing input path")?;
    if arguments.next().is_some() {
        return Err("unexpected trailing arguments".into());
    }
    let input: DesktopRuntimeInput = serde_json::from_slice(&fs::read(path)?)?;
    let output = evaluate_desktop_runtime(input)?;
    println!("{}", serde_json::to_string(&output)?);
    Ok(())
}
