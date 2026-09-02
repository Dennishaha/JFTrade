use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};
use thiserror::Error;

#[derive(Clone, Debug)]
pub struct PineBundle<'a> {
    pub file_name: &'a str,
    pub bytes: &'a [u8],
    pub sha256: &'a str,
}

#[derive(Debug, Error)]
pub enum PineBundleError {
    #[error("pine worker asset name is invalid")]
    InvalidName,
    #[error("pine worker bundle checksum mismatch: expected {expected}, actual {actual}")]
    ChecksumMismatch { expected: String, actual: String },
    #[error("materialize pine worker bundle: {0}")]
    Io(#[from] std::io::Error),
}

impl PineBundle<'_> {
    pub fn verify(&self) -> Result<(), PineBundleError> {
        if self.file_name.trim().is_empty()
            || Path::new(self.file_name)
                .file_name()
                .and_then(|value| value.to_str())
                != Some(self.file_name)
        {
            return Err(PineBundleError::InvalidName);
        }
        let actual = encode_hex(&Sha256::digest(self.bytes));
        if !actual.eq_ignore_ascii_case(self.sha256.trim()) {
            return Err(PineBundleError::ChecksumMismatch {
                expected: self.sha256.to_owned(),
                actual,
            });
        }
        Ok(())
    }

    pub fn materialize(
        &self,
        directory: &Path,
        worker_id: &str,
    ) -> Result<PathBuf, PineBundleError> {
        self.verify()?;
        let worker_id = worker_id.trim();
        if worker_id.is_empty() || worker_id.contains(['/', '\\']) {
            return Err(PineBundleError::InvalidName);
        }
        fs::create_dir_all(directory)?;
        let destination = directory.join(format!("{worker_id}-{}", self.file_name));
        let temporary = destination.with_extension(format!("tmp-{}", std::process::id()));
        let mut file = OpenOptions::new()
            .create(true)
            .truncate(true)
            .write(true)
            .open(&temporary)?;
        if let Err(error) = file.write_all(self.bytes).and_then(|()| file.sync_all()) {
            let _ = fs::remove_file(&temporary);
            return Err(PineBundleError::Io(error));
        }
        drop(file);
        if let Err(error) = fs::rename(&temporary, &destination) {
            let _ = fs::remove_file(&temporary);
            return Err(PineBundleError::Io(error));
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
    fn checksum_is_verified_before_worker_asset_is_written() {
        let bundle = PineBundle {
            file_name: "worker.mjs",
            bytes: b"export default true;",
            sha256: "00",
        };
        assert!(matches!(
            bundle.materialize(Path::new("unused"), "pineworker-1"),
            Err(PineBundleError::ChecksumMismatch { .. })
        ));
    }
}
