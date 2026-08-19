#![forbid(unsafe_code)]

use std::env;
use std::process::ExitCode;

use jftrade_store_sqlite::inspect_backtest_snapshot;

fn main() -> ExitCode {
    let mut arguments = env::args_os();
    let _program = arguments.next();
    let Some(path) = arguments.next() else {
        eprintln!("usage: jftrade-sqlite-inspect <snapshot.db>");
        return ExitCode::from(2);
    };
    if arguments.next().is_some() {
        eprintln!("usage: jftrade-sqlite-inspect <snapshot.db>");
        return ExitCode::from(2);
    }
    match inspect_backtest_snapshot(path) {
        Ok(snapshot) => match serde_json::to_string(&snapshot) {
            Ok(json) => {
                println!("{json}");
                ExitCode::SUCCESS
            }
            Err(error) => {
                eprintln!("serialize SQLite snapshot: {error}");
                ExitCode::FAILURE
            }
        },
        Err(error) => {
            eprintln!("{error}");
            ExitCode::FAILURE
        }
    }
}
