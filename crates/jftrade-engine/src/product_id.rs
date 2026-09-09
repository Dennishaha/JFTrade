//! Collision-proof RFC 4122 UUID v4 identifier generation.
//!
//! Provides cryptographically secure, high-entropy unique identifiers for
//! workflows, triggers, sessions, invocations, tasks, and agents.
//! Replaces legacy process-local in-memory atomic sequence counters to eliminate
//! identifier collisions across engine restarts and concurrent workers.

use std::fmt;

/// An error that occurred during identifier generation.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct IdGenerationError(String);

impl fmt::Display for IdGenerationError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "failed to generate unique identifier: {}", self.0)
    }
}

impl std::error::Error for IdGenerationError {}

/// Formats 16 bytes conforming to RFC 4122 v4 into a 36-character lowercase canonical UUID string.
#[inline]
fn format_uuid_v4(bytes: &[u8; 16]) -> String {
    format!(
        "{:02x}{:02x}{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}-{:02x}{:02x}{:02x}{:02x}{:02x}{:02x}",
        bytes[0],
        bytes[1],
        bytes[2],
        bytes[3],
        bytes[4],
        bytes[5],
        bytes[6],
        bytes[7],
        bytes[8],
        bytes[9],
        bytes[10],
        bytes[11],
        bytes[12],
        bytes[13],
        bytes[14],
        bytes[15],
    )
}

/// Fallible generation of a standard RFC 4122 UUID v4 string (36 characters, lowercase).
///
/// Uses the system CSPRNG (`getrandom`) to produce 122 bits of cryptographic entropy.
pub fn try_generate_uuid_v4() -> Result<String, IdGenerationError> {
    let mut bytes = [0u8; 16];
    getrandom::fill(&mut bytes).map_err(|err| IdGenerationError(err.to_string()))?;
    // RFC 4122 Section 4.4:
    // Set the four most significant bits (bits 12 through 15) of time_hi_and_version to 4.
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    // Set the two most significant bits (bits 6 and 7) of clock_seq_hi_and_reserved to zero and one.
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    Ok(format_uuid_v4(&bytes))
}

/// Generates a standard RFC 4122 UUID v4 string (36 characters, lowercase).
///
/// # Panics
/// Panics if the operating system CSPRNG is completely unavailable.
#[inline]
pub fn generate_uuid_v4() -> String {
    try_generate_uuid_v4().expect("operating system CSPRNG must be available")
}

/// Generates a collision-proof identifier with a given prefix: `{prefix}-{uuid_v4}`.
///
/// If `prefix` is empty, returns the 36-character UUID directly without a leading hyphen.
#[inline]
pub fn generate_prefixed_id(prefix: &str) -> String {
    let uuid = generate_uuid_v4();
    if prefix.is_empty() {
        uuid
    } else {
        format!("{prefix}-{uuid}")
    }
}

/// Validates whether a given string is a canonical RFC 4122 UUID v4 lowercase string.
pub fn is_valid_uuid_v4(value: &str) -> bool {
    if value.len() != 36 {
        return false;
    }
    let bytes = value.as_bytes();
    if bytes[8] != b'-' || bytes[13] != b'-' || bytes[18] != b'-' || bytes[23] != b'-' {
        return false;
    }
    // Version 4 check at index 14
    if bytes[14] != b'4' {
        return false;
    }
    // Variant 1 check (RFC 4122) at index 19: bits 6..7 = 10 -> high nibble in 8, 9, a, b
    if !matches!(bytes[19], b'8' | b'9' | b'a' | b'b') {
        return false;
    }
    for (i, &b) in bytes.iter().enumerate() {
        if i == 8 || i == 13 || i == 18 || i == 23 {
            continue;
        }
        if !b.is_ascii_hexdigit() || b.is_ascii_uppercase() {
            return false;
        }
    }
    true
}

/// Validates whether a given string has the form `{prefix}-{uuid_v4}` with a valid RFC 4122 UUID v4.
pub fn is_valid_prefixed_id(value: &str, prefix: &str) -> bool {
    if prefix.is_empty() {
        return is_valid_uuid_v4(value);
    }
    let expected_len = prefix.len() + 1 + 36;
    if value.len() != expected_len {
        return false;
    }
    if !value.starts_with(prefix) {
        return false;
    }
    if value.as_bytes()[prefix.len()] != b'-' {
        return false;
    }
    is_valid_uuid_v4(&value[prefix.len() + 1..])
}

/// Extracts the UUID portion from a `{prefix}-{uuid_v4}` identifier if valid.
pub fn extract_uuid_from_prefixed_id<'a>(value: &'a str, prefix: &str) -> Option<&'a str> {
    if is_valid_prefixed_id(value, prefix) {
        if prefix.is_empty() {
            Some(value)
        } else {
            Some(&value[prefix.len() + 1..])
        }
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;
    use std::sync::{Arc, Mutex};
    use std::thread;

    #[test]
    fn uuid_v4_format_and_rfc4122_compliance() {
        let id = generate_uuid_v4();
        assert_eq!(id.len(), 36, "UUID length must be exactly 36 characters");
        assert!(
            is_valid_uuid_v4(&id),
            "Generated UUID must be valid RFC 4122 v4: {id}"
        );

        let bytes = id.as_bytes();
        assert_eq!(bytes[8], b'-');
        assert_eq!(bytes[13], b'-');
        assert_eq!(bytes[18], b'-');
        assert_eq!(bytes[23], b'-');
        // Version 4 marker
        assert_eq!(bytes[14], b'4', "Version nibble must be '4'");
        // Variant marker (8, 9, a, or b)
        assert!(
            matches!(bytes[19], b'8' | b'9' | b'a' | b'b'),
            "Variant nibble must be 8, 9, a, or b, found '{}'",
            bytes[19] as char
        );
    }

    #[test]
    fn prefixed_id_formatting_and_extraction() {
        let prefixes = [
            "workflow",
            "session",
            "workflow-trigger",
            "task",
            "agent",
            "workflow-log",
            "ctx",
        ];

        for prefix in prefixes {
            let id = generate_prefixed_id(prefix);
            assert!(
                id.starts_with(&format!("{prefix}-")),
                "ID must start with '{prefix}-'"
            );
            assert_eq!(
                id.len(),
                prefix.len() + 1 + 36,
                "ID length must equal prefix + 1 + 36"
            );
            assert!(
                is_valid_prefixed_id(&id, prefix),
                "Should validate prefixed ID for {prefix}"
            );

            let extracted = extract_uuid_from_prefixed_id(&id, prefix)
                .expect("Extraction must succeed for valid prefixed ID");
            assert!(
                is_valid_uuid_v4(extracted),
                "Extracted UUID must be valid v4"
            );
            assert_eq!(extracted, &id[prefix.len() + 1..]);
        }

        // Empty prefix fallback
        let raw = generate_prefixed_id("");
        assert_eq!(raw.len(), 36);
        assert!(is_valid_uuid_v4(&raw));
    }

    #[test]
    fn validation_rejections() {
        assert!(!is_valid_uuid_v4(""));
        assert!(!is_valid_uuid_v4("not-a-uuid"));
        assert!(!is_valid_uuid_v4("c9a646d3-9c61-4c69-8d66-70e2f5b89a8")); // 35 chars
        assert!(!is_valid_uuid_v4("c9a646d3-9c61-4c69-8d66-70e2f5b89a800")); // 37 chars
        assert!(!is_valid_uuid_v4("c9a646d3-9c61-5c69-8d66-70e2f5b89a80")); // version 5, not 4
        assert!(!is_valid_uuid_v4("c9a646d3-9c61-4c69-4d66-70e2f5b89a80")); // variant 0, not 1
        assert!(!is_valid_uuid_v4("c9a646d3-9c61-4c69-cd66-70e2f5b89a80")); // variant 2, not 1
        assert!(!is_valid_uuid_v4("C9A646D3-9C61-4C69-8D66-70E2F5B89A80")); // uppercase rejected
        assert!(!is_valid_uuid_v4("c9a646d3_9c61_4c69_8d66_70e2f5b89a80")); // underscore instead of dash

        assert!(!is_valid_prefixed_id(
            "workflow-c9a646d3-9c61-4c69-8d66-70e2f5b89a80",
            "session"
        ));
        assert!(!is_valid_prefixed_id(
            "session-c9a646d3-9c61-5c69-8d66-70e2f5b89a80",
            "session"
        ));
        assert!(!is_valid_prefixed_id("session", "session"));
    }

    #[test]
    fn uniqueness_under_concurrency() {
        const THREAD_COUNT: usize = 10;
        const IDS_PER_THREAD: usize = 5_000;
        let set = Arc::new(Mutex::new(HashSet::with_capacity(
            THREAD_COUNT * IDS_PER_THREAD,
        )));
        let mut handles = Vec::with_capacity(THREAD_COUNT);

        for _ in 0..THREAD_COUNT {
            let set_clone = Arc::clone(&set);
            handles.push(thread::spawn(move || {
                let mut local = Vec::with_capacity(IDS_PER_THREAD);
                for _ in 0..IDS_PER_THREAD {
                    local.push(generate_prefixed_id("concurrent-test"));
                }
                let mut guard = set_clone.lock().expect("mutex poisoned");
                for id in local {
                    let inserted = guard.insert(id);
                    assert!(inserted, "Collision detected in concurrent generation!");
                }
            }));
        }

        for handle in handles {
            handle.join().expect("thread panicked");
        }

        let guard = set.lock().expect("mutex poisoned");
        assert_eq!(
            guard.len(),
            THREAD_COUNT * IDS_PER_THREAD,
            "All generated IDs must be strictly unique"
        );
    }

    #[test]
    fn cross_restart_simulation_zero_collision() {
        // Simulates 100 server restarts.
        // Under the old AtomicU64 implementation, every restart would generate
        // "workflow-session-1" and "workflow-wf1-1", creating instant primary key collision.
        // With UUID v4, every restart generates completely disjoint IDs.
        const SIMULATED_RESTARTS: usize = 100;
        const INVOCATIONS_PER_RESTART: usize = 10;

        let mut all_request_ids = HashSet::new();
        let mut all_session_ids = HashSet::new();

        for _restart in 0..SIMULATED_RESTARTS {
            for _inv in 0..INVOCATIONS_PER_RESTART {
                let request_id = format!("workflow-wf1-{}", generate_uuid_v4());
                let session_id = format!("workflow-session-{}", generate_uuid_v4());

                assert!(
                    all_request_ids.insert(request_id.clone()),
                    "Request ID collision across restart simulation: {request_id}"
                );
                assert!(
                    all_session_ids.insert(session_id.clone()),
                    "Session ID collision across restart simulation: {session_id}"
                );
            }
        }

        assert_eq!(
            all_request_ids.len(),
            SIMULATED_RESTARTS * INVOCATIONS_PER_RESTART
        );
        assert_eq!(
            all_session_ids.len(),
            SIMULATED_RESTARTS * INVOCATIONS_PER_RESTART
        );
    }
}
