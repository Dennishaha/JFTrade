//! Provider-owned OpenD global-state projection accessors.

use super::SharedTradeReadRuntime;

impl SharedTradeReadRuntime {
    pub(crate) fn set_server_version(&self, server_version: Option<&str>) {
        *self
            .server_version
            .write()
            .unwrap_or_else(|error| error.into_inner()) = server_version.map(str::to_owned);
    }

    pub(crate) fn server_version_snapshot(&self) -> Option<String> {
        self.server_version
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) fn set_global_state(
        &self,
        global_state: Option<&jftrade_integration_futu::OpenDProbe>,
    ) {
        *self
            .global_state
            .write()
            .unwrap_or_else(|error| error.into_inner()) = global_state.cloned();
    }

    pub(crate) fn global_state_snapshot(&self) -> Option<jftrade_integration_futu::OpenDProbe> {
        self.global_state
            .read()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }
}
