use argon2::{
    Algorithm, Argon2, Params, Version,
    password_hash::{PasswordHash, PasswordHasher, PasswordVerifier, SaltString},
};
use std::ops::RangeInclusive;

const PASSWORD_SALT_BYTES: usize = 16;
/// Go `passwordhash.decode` bounds. A verifier is accepted when its PHC fields
/// sit inside these ranges, so migrated verifiers keep working while a corrupt
/// settings file still fails closed before Argon2 allocates its memory.
const VERIFIER_MEMORY_KIB: RangeInclusive<u32> = 19 * 1024..=128 * 1024;
const VERIFIER_ITERATIONS: RangeInclusive<u32> = 2..=5;
const VERIFIER_PARALLELISM: RangeInclusive<u32> = 1..=4;
const VERIFIER_SALT_BYTES: RangeInclusive<usize> = 16..=64;
const VERIFIER_KEY_BYTES: RangeInclusive<usize> = 16..=64;

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
    if !verifier_within_supported_bounds(&parsed) {
        return false;
    }
    Argon2::default()
        .verify_password(value.as_bytes(), &parsed)
        .is_ok()
}

fn verifier_within_supported_bounds(parsed: &PasswordHash<'_>) -> bool {
    if parsed.algorithm.as_str() != "argon2id" || parsed.version != Some(19) {
        return false;
    }
    let (Some(memory), Some(iterations), Some(parallelism)) = (
        parsed.params.get_decimal("m"),
        parsed.params.get_decimal("t"),
        parsed.params.get_decimal("p"),
    ) else {
        return false;
    };
    if !VERIFIER_MEMORY_KIB.contains(&memory)
        || !VERIFIER_ITERATIONS.contains(&iterations)
        || !VERIFIER_PARALLELISM.contains(&parallelism)
    {
        return false;
    }
    let Some(salt) = parsed.salt.as_ref() else {
        return false;
    };
    let mut salt_bytes = [0_u8; 64];
    let Ok(salt_bytes) = salt.decode_b64(&mut salt_bytes) else {
        return false;
    };
    if !VERIFIER_SALT_BYTES.contains(&salt_bytes.len()) {
        return false;
    }
    parsed
        .hash
        .as_ref()
        .is_some_and(|output| VERIFIER_KEY_BYTES.contains(&output.len()))
}

#[cfg(test)]
mod tests {
    use std::time::{Duration, Instant};

    use super::*;

    const GO_PASSWORD: &str = "a long Web passphrase";
    /// 16 raw bytes in unpadded standard Base64, the shape Go writes.
    const GO_SALT: &str = "MDEyMzQ1Njc4OWFiY2RlZg";
    const GO_KEY: &str = "MDEyMzQ1Njc4OWFiY2RlZg";

    fn go_verifier(params: &str, salt: &str, key: &str) -> String {
        format!("$argon2id$v=19${params}${salt}${key}")
    }

    // Parity: go:452dea11:internal/security/passwordhash/passwordhash_test.go:9 TestHashAndVerify
    #[test]
    fn produced_verifier_hides_the_password_and_only_accepts_the_matching_secret() {
        let encoded = hash_argon2id(GO_PASSWORD).expect("hash password");
        assert!(!encoded.contains(GO_PASSWORD));

        let parsed = PasswordHash::new(&encoded).expect("parse produced verifier");
        assert_eq!(parsed.algorithm.as_str(), "argon2id");
        assert_eq!(parsed.version, Some(19));
        assert_eq!(parsed.params.get_decimal("m"), Some(65_536));
        assert_eq!(parsed.params.get_decimal("t"), Some(3));
        assert_eq!(parsed.params.get_decimal("p"), Some(1));
        let mut salt_bytes = [0_u8; 64];
        assert_eq!(
            parsed
                .salt
                .as_ref()
                .expect("verifier salt")
                .decode_b64(&mut salt_bytes)
                .expect("decode salt")
                .len(),
            PASSWORD_SALT_BYTES
        );
        assert_eq!(parsed.hash.as_ref().expect("verifier key").len(), 32);

        assert!(verify_argon2id(&encoded, GO_PASSWORD));
        assert!(!verify_argon2id(&encoded, "wrong password"));
    }

    // Parity: go:452dea11:internal/security/passwordhash/passwordhash_test.go:25 TestVerifyRejectsUnsafeParametersBeforeHashing
    #[test]
    fn parameters_outside_go_bounds_are_rejected_before_argon2_runs() {
        let oversized = go_verifier("m=1048576,t=3,p=1", GO_SALT, GO_KEY);
        let started = Instant::now();
        assert!(!verify_argon2id(&oversized, GO_PASSWORD));
        assert!(
            started.elapsed() < Duration::from_secs(1),
            "unsafe parameters must be rejected from the PHC fields without invoking Argon2"
        );

        for params in [
            "m=19455,t=3,p=1",
            "m=131073,t=3,p=1",
            "m=65536,t=6,p=1",
            "m=65536,t=3,p=5",
        ] {
            let verifier = go_verifier(params, GO_SALT, GO_KEY);
            assert!(
                !verify_argon2id(&verifier, GO_PASSWORD),
                "accepted out-of-bounds parameters {params}"
            );
        }
    }

    // Parity: go:452dea11:internal/security/passwordhash/passwordhash_test.go:32 TestValidRejectsMalformedHashes
    #[test]
    fn malformed_verifiers_are_rejected_without_a_verification_result() {
        let samples = [
            String::new(),
            format!("$argon2i$v=19$m=65536,t=3,p=1${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=18$m=65536,t=3,p=1${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=bad$m=65536,t=3,p=1${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=19$bad-params${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=19$m=1024,t=3,p=1${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=19$m=65536,t=1,p=1${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=19$m=65536,t=3,p=0${GO_SALT}${GO_KEY}"),
            format!("$argon2id$v=19$m=65536,t=3,p=1$%%%${GO_KEY}"),
            format!("$argon2id$v=19$m=65536,t=3,p=1$YWJj${GO_KEY}"),
            format!("$argon2id$v=19$m=65536,t=3,p=1${GO_SALT}$%%%"),
            format!("$argon2id$v=19$m=65536,t=3,p=1${GO_SALT}$YWJj"),
        ];
        for sample in samples {
            assert!(
                !verify_argon2id(&sample, GO_PASSWORD),
                "accepted malformed verifier {sample:?}"
            );
        }
    }

    #[test]
    fn go_bounded_parameters_keep_verifying_after_migration() {
        let params = Params::new(19 * 1024, 2, 1, Some(32)).expect("minimal Go-bounded params");
        let salt = SaltString::encode_b64(b"0123456789abcdef").expect("salt");
        let verifier = Argon2::new(Algorithm::Argon2id, Version::V0x13, params)
            .hash_password(GO_PASSWORD.as_bytes(), &salt)
            .expect("hash password")
            .to_string();

        assert!(verify_argon2id(&verifier, GO_PASSWORD));
        assert!(!verify_argon2id(&verifier, "wrong password"));
    }
}
