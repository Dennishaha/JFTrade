use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct OnboardingSettings {
    pub completed: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub completed_at: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub dismissed_at: String,
    pub last_broker_id: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct OnboardingWriteRequest {
    pub completed: bool,
    pub dismissed: bool,
    pub last_broker_id: String,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct OnboardingInputs {
    pub state: OnboardingSettings,
    pub broker_enabled: bool,
    pub broker_configured: bool,
    pub enabled_accounts: usize,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct OnboardingReason {
    pub code: &'static str,
    pub severity: &'static str,
    pub message: &'static str,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OnboardingReadiness {
    pub state: OnboardingSettings,
    pub should_show_oobe: bool,
    pub reasons: Vec<OnboardingReason>,
    pub broker_enabled: bool,
    pub broker_configured: bool,
}

pub trait OnboardingSettingsStorePort: Send + Sync {
    fn load_onboarding_inputs(&self) -> Result<OnboardingInputs, SettingsStoreError>;

    fn save_onboarding_settings(
        &self,
        settings: &OnboardingSettings,
    ) -> Result<OnboardingSettings, SettingsStoreError>;
}

#[derive(Clone)]
pub struct OnboardingSettingsService {
    store: Arc<dyn OnboardingSettingsStorePort>,
}

impl OnboardingSettingsService {
    pub fn new(store: Arc<dyn OnboardingSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn readiness(
        &self,
        all_required_dependencies_satisfied: bool,
    ) -> Result<OnboardingReadiness, SettingsStoreError> {
        let mut inputs = self.store.load_onboarding_inputs()?;
        inputs.state = normalize_onboarding_settings(&inputs.state);
        let mut reasons = Vec::with_capacity(2);
        if !all_required_dependencies_satisfied {
            reasons.push(OnboardingReason {
                code: "RUNTIME_DEPENDENCY_UNSATISFIED",
                severity: "warning",
                message: "Required runtime dependencies are missing or do not meet the minimum version.",
            });
        }
        if inputs.enabled_accounts == 0 {
            reasons.push(OnboardingReason {
                code: "NO_MANAGED_ACCOUNTS",
                severity: "info",
                message: "No managed broker accounts have been configured.",
            });
        }
        let should_show_oobe = !all_required_dependencies_satisfied
            || (!inputs.state.completed && !reasons.is_empty());
        Ok(OnboardingReadiness {
            state: inputs.state,
            should_show_oobe,
            reasons,
            broker_enabled: inputs.broker_enabled,
            broker_configured: inputs.broker_configured,
        })
    }

    pub fn save(
        &self,
        request: &OnboardingWriteRequest,
        now: &str,
    ) -> Result<OnboardingSettings, SettingsStoreError> {
        let existing = self.store.load_onboarding_inputs()?.state;
        let mut next = existing.clone();
        next.last_broker_id.clone_from(&request.last_broker_id);
        if next.last_broker_id.trim().is_empty() {
            next.last_broker_id = existing.last_broker_id;
        }
        if request.completed || request.dismissed {
            next.completed = true;
            if request.dismissed {
                next.dismissed_at = now.to_owned();
            }
            if next.completed_at.is_empty() {
                next.completed_at = now.to_owned();
            }
        } else {
            next.completed = false;
            next.completed_at.clear();
            next.dismissed_at.clear();
        }
        self.store.save_onboarding_settings(&next)
    }
}

pub fn normalize_onboarding_settings(input: &OnboardingSettings) -> OnboardingSettings {
    let mut settings = input.clone();
    settings.last_broker_id = settings.last_broker_id.trim().to_owned();
    if !settings.completed {
        settings.completed_at.clear();
    }
    settings
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    struct Store(RwLock<OnboardingInputs>);

    impl OnboardingSettingsStorePort for Store {
        fn load_onboarding_inputs(&self) -> Result<OnboardingInputs, SettingsStoreError> {
            self.0
                .read()
                .map(|inputs| inputs.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_onboarding_settings(
            &self,
            settings: &OnboardingSettings,
        ) -> Result<OnboardingSettings, SettingsStoreError> {
            let normalized = normalize_onboarding_settings(settings);
            self.0
                .write()
                .map(|mut inputs| {
                    inputs.state = normalized.clone();
                    normalized
                })
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    #[test]
    fn readiness_matches_dependency_account_and_completion_rules() {
        let service =
            OnboardingSettingsService::new(Arc::new(Store(RwLock::new(OnboardingInputs {
                state: OnboardingSettings {
                    completed: true,
                    completed_at: "2026-08-19T00:00:00Z".to_owned(),
                    last_broker_id: " futu ".to_owned(),
                    ..OnboardingSettings::default()
                },
                broker_enabled: true,
                broker_configured: true,
                enabled_accounts: 1,
            }))));
        let ready = service.readiness(true).expect("ready onboarding");
        assert!(!ready.should_show_oobe);
        assert!(ready.reasons.is_empty());
        assert_eq!(ready.state.last_broker_id, "futu");

        let degraded = service.readiness(false).expect("degraded onboarding");
        assert!(degraded.should_show_oobe);
        assert_eq!(degraded.reasons[0].code, "RUNTIME_DEPENDENCY_UNSATISFIED");
    }

    #[test]
    fn save_matches_completion_dismissal_and_existing_broker_rules() {
        let service =
            OnboardingSettingsService::new(Arc::new(Store(RwLock::new(OnboardingInputs {
                state: OnboardingSettings {
                    completed: true,
                    completed_at: "2026-08-19T00:00:00Z".to_owned(),
                    dismissed_at: "old-dismissal".to_owned(),
                    last_broker_id: "futu".to_owned(),
                },
                ..OnboardingInputs::default()
            }))));
        let reset = service
            .save(&OnboardingWriteRequest::default(), "2026-08-20T00:00:00Z")
            .expect("reset onboarding");
        assert_eq!(
            reset,
            OnboardingSettings {
                last_broker_id: "futu".to_owned(),
                ..OnboardingSettings::default()
            }
        );
        let dismissed = service
            .save(
                &OnboardingWriteRequest {
                    dismissed: true,
                    last_broker_id: " other ".to_owned(),
                    ..OnboardingWriteRequest::default()
                },
                "2026-08-20T00:00:00Z",
            )
            .expect("dismiss onboarding");
        assert!(dismissed.completed);
        assert_eq!(dismissed.completed_at, "2026-08-20T00:00:00Z");
        assert_eq!(dismissed.dismissed_at, "2026-08-20T00:00:00Z");
        assert_eq!(dismissed.last_broker_id, "other");
    }
}
