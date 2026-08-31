//! Hashing and constant-time comparison helpers for web sessions.

use sha2::{Digest, Sha256};

pub(super) fn derive_csrf_token(session_token: &str) -> String {
    token_hash(&format!("jftrade.csrf.v1:{session_token}"))
}

pub(super) fn token_hash(token: &str) -> String {
    let mut digest = Sha256::new();
    digest.update(token.as_bytes());
    encode_hex(digest.finalize())
}

pub(super) fn constant_time_eq(a: &str, b: &str) -> bool {
    let a_bytes = a.as_bytes();
    let b_bytes = b.as_bytes();
    if a_bytes.len() != b_bytes.len() {
        return false;
    }
    let mut diff = 0;
    for (x, y) in a_bytes.iter().zip(b_bytes.iter()) {
        diff |= x ^ y;
    }
    diff == 0
}

pub(super) fn encode_hex(bytes: impl AsRef<[u8]>) -> String {
    const HEX_CHARS: &[u8; 16] = b"0123456789abcdef";
    let bytes = bytes.as_ref();
    let mut hex = String::with_capacity(bytes.len() * 2);
    for &byte in bytes {
        hex.push(HEX_CHARS[(byte >> 4) as usize] as char);
        hex.push(HEX_CHARS[(byte & 0x0f) as usize] as char);
    }
    hex
}
