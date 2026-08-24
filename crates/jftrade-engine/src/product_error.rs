#[derive(Debug, Error)]
pub enum ProductError {
    #[error("invalid Rust product API bind address")]
    InvalidBindAddress(#[source] std::net::AddrParseError),
    #[error("Rust product API may only bind to loopback until Web security ownership moves")]
    NonLoopbackBind,
    #[error("Rust product settings path is required")]
    MissingSettingsPath,
    #[error("{PRODUCT_DESKTOP_TOKEN_ENV} must contain at least 32 characters")]
    MissingDesktopToken,
    #[error("Rust desktop API token must contain at least 32 non-whitespace characters")]
    WeakDesktopToken,
    #[error("resolve the Rust product executable for resource integrity")]
    CurrentExecutable(#[source] std::io::Error),
    #[error("read Rust product executable {path} for resource integrity")]
    ReadExecutable {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("failed to bind Rust product API")]
    Bind(#[source] std::io::Error),
    #[error("failed to inspect Rust product API listener")]
    LocalAddress(#[source] std::io::Error),
    #[error("invalid Rust product route catalog")]
    Routes(#[from] RouteCatalogError),
    #[error("failed to open Rust product settings")]
    Settings(#[source] jftrade_settings::SettingsStoreError),
    #[error("Rust exchange-calendar manager failed")]
    Calendar(#[source] jftrade_calendar::CalendarManagerError),
    #[error("Rust product API task failed")]
    Join(#[source] tokio::task::JoinError),
    #[error("Rust product API transport failed")]
    Transport(#[from] std::io::Error),
}
