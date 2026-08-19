use std::collections::BTreeMap;

use thiserror::Error;

use crate::VersionedArtifact;

#[derive(Debug, Error, Eq, PartialEq)]
pub enum ArtifactError {
    #[error("artifact identity is incomplete")]
    IncompleteIdentity,
    #[error("artifact version must be the next monotonic version")]
    NonMonotonicVersion,
}

#[derive(Clone, Debug, Default)]
pub struct ArtifactStore {
    versions: BTreeMap<String, Vec<VersionedArtifact>>,
}

impl ArtifactStore {
    pub fn restore(
        versions: BTreeMap<String, Vec<VersionedArtifact>>,
    ) -> Result<Self, ArtifactError> {
        for (key, artifacts) in &versions {
            for (index, artifact) in artifacts.iter().enumerate() {
                if artifact_key(
                    &artifact.session_id,
                    &artifact.namespace,
                    &artifact.filename,
                ) != *key
                {
                    return Err(ArtifactError::IncompleteIdentity);
                }
                if artifact.version != u64::try_from(index).unwrap_or(u64::MAX) + 1 {
                    return Err(ArtifactError::NonMonotonicVersion);
                }
            }
        }
        Ok(Self { versions })
    }

    pub fn snapshot(&self) -> BTreeMap<String, Vec<VersionedArtifact>> {
        self.versions.clone()
    }

    pub fn save(
        &mut self,
        artifact: VersionedArtifact,
    ) -> Result<VersionedArtifact, ArtifactError> {
        if artifact.session_id.trim().is_empty()
            || artifact.namespace.trim().is_empty()
            || artifact.filename.trim().is_empty()
            || artifact.content_sha256.trim().is_empty()
        {
            return Err(ArtifactError::IncompleteIdentity);
        }
        let key = artifact_key(
            &artifact.session_id,
            &artifact.namespace,
            &artifact.filename,
        );
        let versions = self.versions.entry(key).or_default();
        let expected = versions.last().map_or(1, |current| current.version + 1);
        if artifact.version != expected {
            return Err(ArtifactError::NonMonotonicVersion);
        }
        versions.push(artifact.clone());
        Ok(artifact)
    }

    pub fn latest(
        &self,
        session_id: &str,
        namespace: &str,
        filename: &str,
    ) -> Option<&VersionedArtifact> {
        self.versions
            .get(&artifact_key(session_id, namespace, filename))
            .and_then(|versions| versions.last())
    }

    pub fn load(
        &self,
        session_id: &str,
        namespace: &str,
        filename: &str,
        version: u64,
    ) -> Option<&VersionedArtifact> {
        self.versions
            .get(&artifact_key(session_id, namespace, filename))
            .and_then(|versions| versions.iter().find(|artifact| artifact.version == version))
    }
}

fn artifact_key(session_id: &str, namespace: &str, filename: &str) -> String {
    format!(
        "{}\0{}\0{}",
        session_id.trim(),
        namespace.trim(),
        filename.trim()
    )
}

#[cfg(test)]
mod tests {
    use jftrade_kernel::WireTimestamp;

    use super::*;

    #[test]
    fn restore_rejects_a_version_gap() {
        let now: WireTimestamp = "2026-08-19T00:00:00Z".parse().expect("timestamp");
        let artifact = VersionedArtifact {
            session_id: "session".to_owned(),
            namespace: "analysis".to_owned(),
            filename: "report.md".to_owned(),
            version: 2,
            content_sha256: "sha256:value".to_owned(),
            content_base64: "dmFsdWU=".to_owned(),
            created_at: now,
        };
        let key = artifact_key("session", "analysis", "report.md");
        assert_eq!(
            ArtifactStore::restore(BTreeMap::from([(key, vec![artifact])]))
                .expect_err("version gap"),
            ArtifactError::NonMonotonicVersion
        );
    }
}
