#![forbid(unsafe_code)]

use std::error::Error;
use std::io::Read;
use std::path::PathBuf;

use jftrade_backtest::{CorpusInput, run_corpus};

struct Options {
    input: Option<PathBuf>,
    repeat: usize,
}

fn main() {
    if let Err(error) = execute() {
        eprintln!("jftrade-backtest-replay: {error}");
        std::process::exit(1);
    }
}

fn execute() -> Result<(), Box<dyn Error>> {
    let options = parse_options(std::env::args().skip(1))?;
    let input = read_input(options.input.as_ref())?;
    let corpus: CorpusInput = serde_json::from_slice(&input)?;
    let mut output = None;
    for _ in 0..options.repeat {
        output = Some(run_corpus(&corpus)?);
        std::hint::black_box(&output);
    }
    let output = output.ok_or("backtest repeat count produced no output")?;
    println!("{}", serde_json::to_string(&output)?);
    Ok(())
}

fn parse_options(arguments: impl Iterator<Item = String>) -> Result<Options, Box<dyn Error>> {
    let mut input = None;
    let mut repeat = 1_usize;
    let mut arguments = arguments.peekable();
    while let Some(argument) = arguments.next() {
        match argument.as_str() {
            "--input" => {
                input = Some(PathBuf::from(
                    arguments.next().ok_or("--input requires a path")?,
                ));
            }
            "--repeat" => {
                repeat = arguments
                    .next()
                    .ok_or("--repeat requires a positive integer")?
                    .parse()?;
                if repeat == 0 {
                    return Err("--repeat must be positive".into());
                }
            }
            _ => return Err(format!("unsupported argument {argument}").into()),
        }
    }
    Ok(Options { input, repeat })
}

fn read_input(path: Option<&PathBuf>) -> Result<Vec<u8>, Box<dyn Error>> {
    if let Some(path) = path {
        return Ok(std::fs::read(path)?);
    }
    let mut input = Vec::new();
    std::io::stdin().read_to_end(&mut input)?;
    Ok(input)
}
