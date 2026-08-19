#![forbid(unsafe_code)]

use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const MIN_WEB_ACCESS_PASSWORD_CHARS: usize = 15;
pub const MAX_WEB_ACCESS_PASSWORD_BYTES: usize = 1024;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SecuritySettingsInput {
    pub web_access_enabled: bool,
    pub public_access_enabled: bool,
    pub web_port: u16,
    pub password: Option<String>,
    pub password_configured: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SecurityUpdatePlan {
    pub web_access_enabled: bool,
    pub public_access_enabled: bool,
    pub web_port: u16,
    pub replace_password: Option<String>,
    pub apply_listener_after_persist: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProviderSelectionPlan {
    pub provider_id: String,
    pub activate_before_persist: bool,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum SettingsError {
    #[error("a Web access password is required before Web access can be enabled")]
    PasswordRequired,
    #[error("web access password must contain at least 15 characters")]
    PasswordTooShort,
    #[error("web access password must contain at most 1024 bytes")]
    PasswordTooLong,
    #[error("web access port must be between 1024 and 65535")]
    InvalidPort,
    #[error("provider id is required")]
    MissingProvider,
}

pub fn plan_security_update(
    input: SecuritySettingsInput,
) -> Result<SecurityUpdatePlan, SettingsError> {
    if input.web_access_enabled && input.web_port < 1024 {
        return Err(SettingsError::InvalidPort);
    }
    let password = input
        .password
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());
    if let Some(password) = password {
        if password.len() > MAX_WEB_ACCESS_PASSWORD_BYTES {
            return Err(SettingsError::PasswordTooLong);
        }
        if password.chars().count() < MIN_WEB_ACCESS_PASSWORD_CHARS {
            return Err(SettingsError::PasswordTooShort);
        }
    }
    if input.web_access_enabled && !input.password_configured && password.is_none() {
        return Err(SettingsError::PasswordRequired);
    }
    Ok(SecurityUpdatePlan {
        web_access_enabled: input.web_access_enabled,
        public_access_enabled: input.web_access_enabled && input.public_access_enabled,
        web_port: input.web_port,
        replace_password: password.map(ToOwned::to_owned),
        apply_listener_after_persist: true,
    })
}

pub fn plan_provider_selection(value: &str) -> Result<ProviderSelectionPlan, SettingsError> {
    let provider_id = value.trim().to_lowercase();
    if provider_id.is_empty() {
        return Err(SettingsError::MissingProvider);
    }
    Ok(ProviderSelectionPlan {
        provider_id,
        activate_before_persist: true,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn web_access_requires_a_strong_password_and_persist_first_listener_plan() {
        assert_eq!(
            plan_security_update(SecuritySettingsInput {
                web_access_enabled: true,
                public_access_enabled: true,
                web_port: 3000,
                password: None,
                password_configured: false,
            }),
            Err(SettingsError::PasswordRequired)
        );
        let plan = plan_security_update(SecuritySettingsInput {
            web_access_enabled: true,
            public_access_enabled: true,
            web_port: 3000,
            password: Some("correct horse battery staple".into()),
            password_configured: false,
        })
        .expect("valid security update");
        assert!(plan.public_access_enabled);
        assert!(plan.apply_listener_after_persist);
    }

    #[test]
    fn disabled_web_access_cannot_leave_public_access_active() {
        let plan = plan_security_update(SecuritySettingsInput {
            web_access_enabled: false,
            public_access_enabled: true,
            web_port: 0,
            password: None,
            password_configured: false,
        })
        .expect("disabled access needs no password");
        assert!(!plan.public_access_enabled);
        assert_eq!(
            plan_provider_selection(" AKShare ")
                .expect("provider")
                .provider_id,
            "akshare"
        );
    }
}
