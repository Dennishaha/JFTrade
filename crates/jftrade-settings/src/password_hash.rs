use argon2::{
    Algorithm, Argon2, Params, Version,
    password_hash::{PasswordHash, PasswordHasher, PasswordVerifier, SaltString},
};

const PASSWORD_SALT_BYTES: usize = 16;

pub(crate) fn hash_argon2id(value: &str) -> Result<String, String> {
    let mut salt_bytes = [0_u8; PASSWORD_SALT_BYTES];
    getrandom::fill(&mut salt_bytes).map_err(|error| error.to_string())?;
    let salt = SaltString::encode_b64(&salt_bytes).map_err(|error| error.to_string())?;
    let params = Params::new(65_536, 3, 1, Some(32)).map_err(|error| error.to_string())?;
    Argon2::new(Algorithm::Argon2id, Version::V0x13, params)
        .hash_password(value.as_bytes(), &salt)
        .map_err(|error| error.to_string())
        .map(|hash| hash.to_string())
}

pub(crate) fn verify_argon2id(value_hash: &str, value: &str) -> bool {
    let Ok(parsed) = PasswordHash::new(value_hash.trim()) else {
        return false;
    };
    if parsed.algorithm.as_str() != "argon2id"
        || parsed.version != Some(19)
        || parsed.params.get_decimal("m") != Some(65_536)
        || parsed.params.get_decimal("t") != Some(3)
        || parsed.params.get_decimal("p") != Some(1)
        || parsed.hash.as_ref().map(|output| output.len()) != Some(32)
    {
        return false;
    }
    Argon2::default()
        .verify_password(value.as_bytes(), &parsed)
        .is_ok()
}
