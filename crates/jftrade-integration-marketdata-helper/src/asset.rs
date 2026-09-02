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
}
