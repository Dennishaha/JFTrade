impl ProductConfig {
    /// Builds the production desktop profile. All route groups are registered
    /// from the production composition root; missing runtime dependencies fail
    /// closed at the port boundary rather than becoming unknown endpoints.
    pub fn desktop_production(
        bind_address: SocketAddr,
        settings_path: impl Into<PathBuf>,
        desktop_token: impl Into<String>,
    ) -> Result<Self, ProductError> {
        let desktop_token = desktop_token.into();
        if desktop_token.trim().len() < 32 {
            return Err(ProductError::WeakDesktopToken);
        }
        let mut config = Self::new(
            bind_address,
            settings_path,
            AccessPolicy::desktop(Some(desktop_token)),
        )?;
        config.capabilities = ProductCapabilities::all();
        config.production = true;
        Ok(config)
    }

    pub fn from_process_env() -> Result<Self, ProductError> {
        let bind_address = env::var(PRODUCT_BIND_ENV)
            .unwrap_or_else(|_| DEFAULT_PRODUCT_BIND.to_owned())
            .parse()
            .map_err(ProductError::InvalidBindAddress)?;
        let settings_path = env::var_os(PRODUCT_SETTINGS_PATH_ENV)
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from(DEFAULT_SETTINGS_PATH));
        let desktop_token = env::var(PRODUCT_DESKTOP_TOKEN_ENV)
            .ok()
            .filter(|value| value.trim().len() >= 32);
        let access = if let Some(desktop_token) = desktop_token {
            AccessPolicy {
                internal_proxy_protocol: env::var(PRODUCT_INTERNAL_PROXY_PROTOCOL_ENV)
                    .ok()
                    .filter(|value| !value.trim().is_empty()),
                ..AccessPolicy::desktop(Some(desktop_token))
            }
        } else {
            // A missing desktop token selects the browser surface. The
            // browser must authenticate through /auth/login and is never
            // granted desktop bearer trust implicitly.
            AccessPolicy::web()
        };
        let mut config = Self::new(bind_address, settings_path, access)?;
        config.capabilities = ProductCapabilities::all();
        config.production = true;
        Ok(config)
    }
}

include!("product_error.rs");
#[path = "product_production_ports.rs"]
pub(crate) mod product_production_ports;
#[path = "product_production_route_registry.rs"]
mod product_production_route_registry;
