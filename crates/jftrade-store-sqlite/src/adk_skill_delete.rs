use super::*;

#[derive(Debug, thiserror::Error)]
pub enum AdkSkillDeleteError {
    #[error(transparent)]
    Store(#[from] AdkStoreError),
    #[error("remove skill files: {0}")]
    Cleanup(#[source] std::io::Error),
}

impl AdkStore {
    /// Keep the DELETE uncommitted until the mutation owner's file cleanup
    /// succeeds. A rejected DELETE never invokes cleanup; cleanup failure
    /// drops the transaction and preserves the original stored row.
    /// The callback must not reenter this store while its connection is held.
    pub fn delete_skill_with_cleanup(
        &self,
        id: &str,
        cleanup: impl FnOnce() -> std::io::Result<()>,
    ) -> Result<bool, AdkSkillDeleteError> {
        let mut connection = self.lock_connection()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(AdkStoreError::Query)?;
        let changed = transaction
            .execute("DELETE FROM adk_skills WHERE id = ?1", params![id])
            .map_err(AdkStoreError::Query)?;
        cleanup().map_err(AdkSkillDeleteError::Cleanup)?;
        transaction.commit().map_err(AdkStoreError::Query)?;
        Ok(changed > 0)
    }
}
