#![forbid(unsafe_code)]

use std::env;
use std::fs;
use std::process::ExitCode;

use jftrade_store_sqlite::inspect_backtest_snapshot;
use rusqlite::Connection;

fn main() -> ExitCode {
    let mut arguments = env::args_os();
    let _program = arguments.next();
    let Some(first) = arguments.next() else {
        print_usage();
        return ExitCode::from(2);
    };
    let path = if first == "--seed-sql" {
        let (Some(sql_path), Some(database_path)) = (arguments.next(), arguments.next()) else {
            print_usage();
            return ExitCode::from(2);
        };
        if arguments.next().is_some() {
            print_usage();
            return ExitCode::from(2);
        }
        if let Err(error) = seed_database(&sql_path, &database_path) {
            eprintln!("seed SQLite fixture: {error}");
            return ExitCode::FAILURE;
        }
        database_path
    } else {
        if arguments.next().is_some() {
            print_usage();
            return ExitCode::from(2);
        }
        first
    };
    render_snapshot(path)
}

fn seed_database(
    sql_path: &std::ffi::OsStr,
    database_path: &std::ffi::OsStr,
) -> Result<(), String> {
    let sql = fs::read_to_string(sql_path).map_err(|error| error.to_string())?;
    let connection = Connection::open(database_path).map_err(|error| error.to_string())?;
    connection
        .execute_batch(&sql)
        .map_err(|error| error.to_string())
}

fn render_snapshot(path: impl AsRef<std::path::Path>) -> ExitCode {
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

fn print_usage() {
    eprintln!("usage: jftrade-sqlite-inspect [--seed-sql <fixture.sql>] <snapshot.db>");
}
