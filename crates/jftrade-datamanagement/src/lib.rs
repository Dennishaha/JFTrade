#![forbid(unsafe_code)]

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use thiserror::Error;

#[derive(Clone, Debug, Eq, Ord, PartialEq, PartialOrd, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupCandidate {
    pub id: String,
    pub category: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CleanupPreview {
    pub database_id: String,
    pub candidates: Vec<CleanupCandidate>,
    pub fingerprint: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum MaintenanceError {
    #[error("database id is required")]
    MissingDatabase,
    #[error("database maintenance is busy: {0}")]
    Busy(String),
    #[error("cleanup candidates changed")]
    CandidatesChanged,
}

pub fn preview_cleanup(
    database_id: &str,
    mut candidates: Vec<CleanupCandidate>,
    busy_reason: Option<&str>,
) -> Result<CleanupPreview, MaintenanceError> {
    let database_id = database_id.trim();
    if database_id.is_empty() {
        return Err(MaintenanceError::MissingDatabase);
    }
    if let Some(reason) = busy_reason.map(str::trim).filter(|value| !value.is_empty()) {
        return Err(MaintenanceError::Busy(reason.to_owned()));
    }
    candidates.sort();
    candidates.dedup();
    let fingerprint = candidate_fingerprint(database_id, &candidates);
    Ok(CleanupPreview {
        database_id: database_id.to_owned(),
        candidates,
        fingerprint,
    })
}

pub fn verify_execute(
    preview: &CleanupPreview,
    current_candidates: Vec<CleanupCandidate>,
    busy_reason: Option<&str>,
) -> Result<Vec<CleanupCandidate>, MaintenanceError> {
    let current = preview_cleanup(&preview.database_id, current_candidates, busy_reason)?;
    if current.fingerprint != preview.fingerprint {
        return Err(MaintenanceError::CandidatesChanged);
    }
    Ok(current.candidates)
}

fn candidate_fingerprint(database_id: &str, candidates: &[CleanupCandidate]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(database_id.as_bytes());
    hasher.update([0]);
    for candidate in candidates {
        hasher.update(candidate.category.as_bytes());
        hasher.update([0]);
        hasher.update(candidate.id.as_bytes());
        hasher.update([0xff]);
    }
    let digest = hasher.finalize();
    let mut encoded = String::with_capacity(digest.len() * 2);
    const HEX: &[u8; 16] = b"0123456789abcdef";
    for byte in digest {
        encoded.push(char::from(HEX[usize::from(byte >> 4)]));
        encoded.push(char::from(HEX[usize::from(byte & 0x0f)]));
    }
    encoded
}

#[cfg(test)]
mod tests {
    use super::*;

    fn candidate(id: &str) -> CleanupCandidate {
        CleanupCandidate {
            id: id.into(),
            category: "run".into(),
        }
    }

    #[test]
    fn cleanup_requires_the_exact_approved_candidate_set() {
        let preview = preview_cleanup("assistant", vec![candidate("b"), candidate("a")], None)
            .expect("preview");
        assert_eq!(
            verify_execute(&preview, vec![candidate("a"), candidate("b")], None).expect("same set"),
            [candidate("a"), candidate("b")]
        );
        assert_eq!(
            verify_execute(&preview, vec![candidate("a")], None),
            Err(MaintenanceError::CandidatesChanged)
        );
    }

    #[test]
    fn busy_owner_blocks_preview_and_execute_without_mutation() {
        assert_eq!(
            preview_cleanup("backtest", vec![candidate("run-1")], Some("active run")),
            Err(MaintenanceError::Busy("active run".into()))
        );
    }
}
