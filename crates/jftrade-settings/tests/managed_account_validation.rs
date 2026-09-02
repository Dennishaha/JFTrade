use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};

use jftrade_settings::{
    BrokerIntegration, BrokerSettingsError, BrokerSettingsInputs, BrokerSettingsService,
    BrokerSettingsStorePort, FutuIntegrationConfig, ManagedBrokerAccount, SettingsStoreError,
};

#[derive(Default)]
struct RecordingStore {
    create_calls: AtomicUsize,
    last_create: Mutex<Option<ManagedBrokerAccount>>,
}

impl BrokerSettingsStorePort for RecordingStore {
    fn load_broker_settings_inputs(&self) -> Result<BrokerSettingsInputs, SettingsStoreError> {
        Ok(BrokerSettingsInputs {
            effective_config: FutuIntegrationConfig::current_default(),
            ..BrokerSettingsInputs::default()
        })
    }

    fn save_broker_integration(
        &self,
        input: &BrokerIntegration,
        _now: &str,
    ) -> Result<BrokerIntegration, SettingsStoreError> {
        Ok(input.clone())
    }

    fn create_managed_broker_account(
        &self,
        input: &ManagedBrokerAccount,
        _now: &str,
    ) -> Result<ManagedBrokerAccount, SettingsStoreError> {
        self.create_calls.fetch_add(1, Ordering::Relaxed);
        *self.last_create.lock().expect("create capture lock") = Some(input.clone());
        Ok(input.clone())
    }

    fn update_managed_broker_account(
        &self,
        _id: &str,
        input: &ManagedBrokerAccount,
        _now: &str,
    ) -> Result<Option<ManagedBrokerAccount>, SettingsStoreError> {
        Ok(Some(input.clone()))
    }

    fn delete_managed_broker_account(&self, _id: &str) -> Result<bool, SettingsStoreError> {
        Ok(true)
    }
}

#[test]
fn blank_account_id_is_rejected_before_persistence() {
    let store = Arc::new(RecordingStore::default());
    let service = BrokerSettingsService::new(store.clone());

    let error = service
        .create_account(&ManagedBrokerAccount::default(), "2026-09-02T00:00:00Z")
        .expect_err("blank account IDs must be rejected");

    assert_eq!(error, BrokerSettingsError::MissingAccountId);
    assert_eq!(error.to_string(), "accountId is required");
    assert_eq!(store.create_calls.load(Ordering::Relaxed), 0);
}

#[test]
fn create_account_clears_client_owned_identity_and_timestamps() {
    let store = Arc::new(RecordingStore::default());
    let service = BrokerSettingsService::new(store.clone());
    let input = ManagedBrokerAccount {
        id: "client-id".to_owned(),
        account_id: " acc-1 ".to_owned(),
        created_at: "client-created".to_owned(),
        updated_at: "client-updated".to_owned(),
        trading_environment: "SIMULATE".to_owned(),
        ..ManagedBrokerAccount::default()
    };

    let created = service
        .create_account(&input, "2026-09-02T00:00:00Z")
        .expect("managed account creation");
    let persisted = store
        .last_create
        .lock()
        .expect("create capture lock")
        .clone()
        .expect("captured account");

    for account in [&created, &persisted] {
        assert!(
            account.id.is_empty(),
            "generated identity must stay store-owned"
        );
        assert!(
            account.created_at.is_empty(),
            "createdAt must stay store-owned"
        );
        assert!(
            account.updated_at.is_empty(),
            "updatedAt must stay store-owned"
        );
    }
    assert_eq!(persisted.account_id, "acc-1");
}
