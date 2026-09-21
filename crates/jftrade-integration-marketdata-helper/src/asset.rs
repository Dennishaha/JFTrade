use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};
use thiserror::Error;

#[derive(Clone, Debug)]
pub struct AssetBundle<'a> {
    pub file_name: &'a str,
    pub bytes: &'a [u8],
    pub sha256: &'a str,
}

#[derive(Debug, Error)]
pub enum AssetError {
    #[error("market-data helper asset name is invalid")]
    InvalidName,
    #[error("market-data helper asset checksum mismatch: expected {expected}, actual {actual}")]
    ChecksumMismatch { expected: String, actual: String },
    #[error("materialize market-data helper asset: {0}")]
    Io(#[from] std::io::Error),
}

impl AssetBundle<'_> {
    pub fn checksum(&self) -> String {
        encode_hex(&Sha256::digest(self.bytes))
    }

    pub fn verify(&self) -> Result<(), AssetError> {
        if self.file_name.trim().is_empty()
            || Path::new(self.file_name)
                .file_name()
                .and_then(|value| value.to_str())
                != Some(self.file_name)
        {
            return Err(AssetError::InvalidName);
        }
        let actual = self.checksum();
        if !actual.eq_ignore_ascii_case(self.sha256.trim()) {
            return Err(AssetError::ChecksumMismatch {
                expected: self.sha256.to_owned(),
                actual,
            });
        }
        Ok(())
    }

    pub fn materialize(&self, directory: &Path) -> Result<PathBuf, AssetError> {
        self.verify()?;
        fs::create_dir_all(directory)?;
        let destination = directory.join(self.file_name);
        if fs::read(&destination).is_ok_and(|bytes| bytes == self.bytes) {
            return Ok(destination);
        }
        let temporary = directory.join(format!(".{}.tmp-{}", self.file_name, std::process::id()));
        let mut file = OpenOptions::new()
            .create_new(true)
            .write(true)
            .open(&temporary)?;
        if let Err(error) = file.write_all(self.bytes).and_then(|()| file.sync_all()) {
            let _ = fs::remove_file(&temporary);
            return Err(AssetError::Io(error));
        }
        drop(file);
        if let Err(error) = fs::rename(&temporary, &destination) {
            let _ = fs::remove_file(&temporary);
            return Err(AssetError::Io(error));
        }
        Ok(destination)
    }
}

fn encode_hex(bytes: &[u8]) -> String {
    const DIGITS: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        output.push(char::from(DIGITS[usize::from(byte >> 4)]));
        output.push(char::from(DIGITS[usize::from(byte & 0x0f)]));
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;

    fn scratch_root(name: &str) -> PathBuf {
        let root = std::env::temp_dir().join(format!(
            "jftrade-marketdata-helper-{}-{}-{name}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        root
    }

    fn file_identity(metadata: &fs::Metadata) -> Option<u64> {
        #[cfg(unix)]
        {
            use std::os::unix::fs::MetadataExt;
            Some(metadata.ino())
        }
        #[cfg(not(unix))]
        {
            let _ = metadata;
            None
        }
    }

    #[test]
    fn verifies_and_materializes_content_addressed_asset() {
        let root = std::env::temp_dir().join(format!(
            "jftrade-helper-asset-{}-{}",
            std::process::id(),
            std::thread::current().name().unwrap_or("test")
        ));
        let _ = fs::remove_dir_all(&root);
        let bundle = AssetBundle {
            file_name: "helper.bin",
            bytes: b"fixture",
            sha256: "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d",
        };
        let path = bundle.materialize(&root).expect("materialize");
        assert_eq!(fs::read(path).expect("read"), b"fixture");
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn rejects_checksum_mismatch_before_writing() {
        let bundle = AssetBundle {
            file_name: "helper.bin",
            bytes: b"fixture",
            sha256: "00",
        };
        assert!(matches!(
            bundle.verify(),
            Err(AssetError::ChecksumMismatch { .. })
        ));
    }

    #[test]
    fn rejects_a_bundle_whose_bytes_no_longer_match_the_digest() {
        let root = scratch_root("digest-change");
        let bundle = AssetBundle {
            file_name: "marketdata-sidecar-linux-amd64",
            bytes: b"sidecar",
            sha256: "0000000000000000000000000000000000000000000000000000000000000000",
        };
        assert!(matches!(
            bundle.materialize(&root),
            Err(AssetError::ChecksumMismatch { .. })
        ));
        assert!(
            !root.join(bundle.file_name).exists(),
            "a bundle whose bytes changed must not be published"
        );
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn reuses_a_published_asset_without_rewriting_it() {
        let root = scratch_root("reuse");
        let bundle = AssetBundle {
            file_name: "helper.bin",
            bytes: b"fixture",
            sha256: "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d",
        };
        let published = bundle.materialize(&root).expect("publish asset");
        let published_metadata = fs::metadata(&published).expect("published metadata");
        let reused = bundle.materialize(&root).expect("reuse published asset");
        let reused_metadata = fs::metadata(&reused).expect("reused metadata");
        assert_eq!(published, reused);
        assert_eq!(fs::read(&reused).expect("read reused"), b"fixture");
        assert_eq!(
            file_identity(&published_metadata),
            file_identity(&reused_metadata),
            "a reused asset keeps the published file instead of republishing it"
        );
        assert_eq!(
            published_metadata.modified().expect("published mtime"),
            reused_metadata.modified().expect("reused mtime")
        );
        assert!(reused.exists(), "a reused asset is not cleaned up by reuse");
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn rejects_escaping_asset_names_before_writing() {
        let parent = scratch_root("escaping-name");
        let root = parent.join("bundle");
        fs::create_dir_all(&root).expect("create bundle root");
        for name in ["../sidecar", "nested/sidecar"] {
            let bundle = AssetBundle {
                file_name: name,
                bytes: b"payload",
                sha256: "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5",
            };
            assert!(
                matches!(bundle.materialize(&root), Err(AssetError::InvalidName)),
                "{name} must not materialize"
            );
        }
        assert!(
            !parent.join("sidecar").exists(),
            "an escaping bundle name must not write outside the bundle root"
        );
        assert!(
            fs::read_dir(&root)
                .expect("read bundle root")
                .next()
                .is_none(),
            "a rejected bundle name must not publish any file"
        );
        let _ = fs::remove_dir_all(parent);
    }
}
