use std::error::Error;

fn main() {
    if let Err(error) = jftrade_desktop::native::run() {
        eprintln!("JFTrade Tauri startup failed: {error}");
        let mut source = error.source();
        while let Some(cause) = source {
            eprintln!("  caused by: {cause}");
            source = cause.source();
        }
        std::process::exit(1);
    }
}
