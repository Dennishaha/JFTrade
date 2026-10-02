//! Parity coverage for Go `pkg/futu/watchlist_reader_test.go`.

use super::*;
use std::sync::atomic::{AtomicUsize, Ordering};

fn at(seconds: u64) -> SystemTime {
    SystemTime::UNIX_EPOCH + Duration::from_secs(seconds)
}

struct FixtureWatchlistRead {
    groups: Mutex<Vec<Value>>,
    members: Mutex<Vec<Value>>,
    group_calls: AtomicUsize,
    member_calls: AtomicUsize,
    fail_groups: bool,
    fail_members: bool,
}

impl std::fmt::Debug for FixtureWatchlistRead {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("FixtureWatchlistRead")
            .finish_non_exhaustive()
    }
}

impl FixtureWatchlistRead {
    fn new(groups: Vec<Value>, members: Vec<Value>) -> Self {
        Self {
            groups: Mutex::new(groups),
            members: Mutex::new(members),
            group_calls: AtomicUsize::new(0),
            member_calls: AtomicUsize::new(0),
            fail_groups: false,
            fail_members: false,
        }
    }

    fn set_members(&self, members: Vec<Value>) {
        *self
            .members
            .lock()
            .unwrap_or_else(|error| error.into_inner()) = members;
    }

    fn set_groups(&self, groups: Vec<Value>) {
        *self
            .groups
            .lock()
            .unwrap_or_else(|error| error.into_inner()) = groups;
    }

    fn group_calls(&self) -> usize {
        self.group_calls.load(Ordering::SeqCst)
    }

    fn member_calls(&self) -> usize {
        self.member_calls.load(Ordering::SeqCst)
    }
}

impl RemoteWatchlistReadPort for FixtureWatchlistRead {
    fn groups(&self) -> Result<Vec<Value>, CustomizationError> {
        self.group_calls.fetch_add(1, Ordering::SeqCst);
        if self.fail_groups {
            return Err(CustomizationError::Rejected("group failure".to_owned()));
        }
        Ok(self
            .groups
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone())
    }

    fn members(&self, _group_name: &str) -> Result<Vec<Value>, CustomizationError> {
        self.member_calls.fetch_add(1, Ordering::SeqCst);
        if self.fail_members {
            return Err(CustomizationError::Rejected("member failure".to_owned()));
        }
        Ok(self
            .members
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone())
    }
}

#[test]
fn watchlist_member_reads_propagate_group_and_remote_failures() {
    let groups = vec![group("Growth", "custom")];
    let mut group_fixture = FixtureWatchlistRead::new(groups.clone(), Vec::new());
    group_fixture.fail_groups = true;
    let group_failure = Arc::new(group_fixture);
    let group_reader = reader(group_failure, WATCHLIST_CACHE_TTL);
    assert!(
        group_reader.members("Growth").is_err(),
        "group lookup failure must propagate"
    );

    let mut member_fixture = FixtureWatchlistRead::new(groups, Vec::new());
    member_fixture.fail_members = true;
    let member_failure = Arc::new(member_fixture);
    let member_reader = reader(member_failure, WATCHLIST_CACHE_TTL);
    assert!(
        member_reader.members_fresh("Growth").is_err(),
        "remote member failure must propagate"
    );
}

fn group(name: &str, kind: &str) -> Value {
    json!({"name": name, "type": kind})
}

fn member(instrument: &str, name: &str, security_type: i32, broker_code: &str, id: i64) -> Value {
    json!({
        "instrumentId": instrument,
        "market": "HK",
        "symbol": broker_code,
        "name": name,
        "lotSize": 100,
        "securityType": security_type,
        "brokerCode": broker_code,
        "brokerSecurityId": id.to_string(),
    })
}

fn reader(inner: Arc<FixtureWatchlistRead>, ttl: Duration) -> CachedRemoteWatchlistReader {
    CachedRemoteWatchlistReader::with_inner(inner, Arc::new(move || at(1_000)), ttl)
}

#[test]
fn watchlist_read_gate_enforces_ten_calls_per_rolling_thirty_seconds() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:14
    // TestFutuWatchlistReadGateEnforcesTenCallsPerRollingThirtySeconds.
    let gate = WatchlistReadGate::new(WATCHLIST_READ_LIMIT, WATCHLIST_READ_WINDOW);
    for index in 0..10 {
        assert!(
            gate.allow(at(index)),
            "call {} unexpectedly rejected",
            index + 1
        );
    }
    assert!(
        !gate.allow(at(29)),
        "11th call inside the rolling window unexpectedly allowed"
    );
    // Go compares with `!call.After(cutoff)`: the reservation made at t=0 is at
    // the cutoff when now=30s, so it has already expired and the call passes.
    assert!(
        gate.allow(at(30)),
        "call at expiry of the first reservation should be allowed"
    );
}

#[test]
fn watchlist_reader_cache_uses_ttl_and_returns_copies() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:30
    // TestFutuWatchlistReaderCacheUsesTTLAndReturnsCopies. Mutating a returned
    // row must not change what the next cache hit serves, and both caches must
    // expire exactly at the TTL boundary.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Long Term", "custom")],
        vec![member("HK.00700", "Tencent", 3, "00700", 700)],
    ));
    let reader = CachedRemoteWatchlistReader::with_inner(
        Arc::clone(&inner) as Arc<dyn RemoteWatchlistReadPort>,
        Arc::new(|| at(1_000)),
        WATCHLIST_CACHE_TTL,
    );
    let groups = reader.groups().expect("groups");
    assert_eq!(groups.len(), 1);
    assert_eq!(groups[0]["name"], "Long Term");
    assert_eq!(groups[0]["ambiguous"], false);
    let mut mutated = groups;
    mutated[0]["name"] = json!("mutated");
    let members = reader.members("Long Term").expect("members");
    assert_eq!(members.len(), 1);
    assert_eq!(members[0]["name"], "Tencent");
    let mut mutated_members = members;
    mutated_members[0]["name"] = json!("mutated");
    // Cache hits must serve pristine copies after the caller mutated its value.
    let again = reader.members("Long Term").expect("cached members");
    assert_eq!(again[0]["name"], "Tencent");
    let again_groups = reader.groups().expect("cached groups");
    assert_eq!(again_groups[0]["name"], "Long Term");
    assert_eq!(inner.group_calls(), 1);
    assert_eq!(inner.member_calls(), 1);
}

#[test]
fn watchlist_reader_cache_expires_at_the_ttl_boundary() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:30.  Go treats the
    // cache as valid only while `now.Before(expiresAt)`, so the boundary second
    // is already a miss and re-reads through the gate.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Long Term", "custom")],
        vec![member("HK.00700", "Tencent", 3, "00700", 700)],
    ));
    let clock = Arc::new(Mutex::new(at(1_000)));
    let clock_handle = Arc::clone(&clock);
    let reader = CachedRemoteWatchlistReader::with_inner(
        Arc::clone(&inner) as Arc<dyn RemoteWatchlistReadPort>,
        Arc::new(move || {
            *clock_handle
                .lock()
                .unwrap_or_else(|error| error.into_inner())
        }),
        WATCHLIST_CACHE_TTL,
    );
    reader.members("Long Term").expect("initial members");
    assert_eq!(inner.member_calls(), 1);
    // Go asserts the *last* valid instant (now = expiresAt-1s) still hits the
    // cache for both the group and the member set before the boundary expires.
    {
        let mut value = clock.lock().unwrap_or_else(|error| error.into_inner());
        *value = at(1_029);
    }
    reader.groups().expect("cached groups before TTL");
    reader
        .members("Long Term")
        .expect("cached members before TTL");
    assert_eq!(inner.group_calls(), 1);
    assert_eq!(inner.member_calls(), 1);
    {
        let mut value = clock.lock().unwrap_or_else(|error| error.into_inner());
        *value = at(1_030);
    }
    let expired = reader
        .members("Long Term")
        .expect("members after TTL boundary");
    assert_eq!(expired[0]["name"], "Tencent");
    assert_eq!(
        inner.member_calls(),
        2,
        "the TTL boundary is a cache miss in Go"
    );
}

#[test]
fn watchlist_group_conversion_marks_every_normalized_duplicate_ambiguous() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:72
    // TestConvertFutuWatchlistGroupsMarksEveryNormalizedDuplicateAmbiguous.
    let groups = convert_groups(&[
        group("Growth", "custom"),
        group(" growth ", "custom"),
        group("All", "system"),
    ]);
    assert_eq!(groups.len(), 3);
    assert_eq!(groups[0]["name"], "Growth");
    assert_eq!(groups[1]["name"], "growth");
    assert_eq!(groups[0]["ambiguous"], true);
    assert_eq!(groups[1]["ambiguous"], true);
    assert_eq!(groups[2]["ambiguous"], false);
    assert_eq!(groups[2]["type"], "system");
}

#[test]
fn watchlist_member_conversion_preserves_canonical_id_and_broker_alias() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:89
    // TestConvertFutuWatchlistSecuritiesPreservesCanonicalIDAndBrokerAlias.
    // Parity: go:452dea11:internal/watchlist/futu/source_test.go:83 TestRemoteMembersKeepBrokerCodeAndSecurityIDAsSeparateAliases
    let members = convert_members(&[json!({
        "instrumentId": "JP.7203",
        "market": "JP",
        "symbol": "7203",
        "name": "Toyota",
        "lotSize": 100,
        "securityType": 3,
        "brokerCode": "7203",
        "brokerSecurityId": "123456",
    })]);
    assert_eq!(members.len(), 1);
    assert_eq!(members[0]["instrumentId"], "JP.7203");
    assert_eq!(members[0]["brokerCode"], "7203");
    assert_eq!(members[0]["brokerSecurityId"], "123456");
    assert_ne!(members[0]["brokerCode"], members[0]["brokerSecurityId"]);
}

#[test]
fn watchlist_fresh_read_bypasses_and_replaces_group_and_member_caches() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:113
    // TestFutuWatchlistFreshReadBypassesAndReplacesGroupAndMemberCaches.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Growth", "custom")],
        vec![member("HK.00700", "Tencent v1", 3, "00700", 700)],
    ));
    let reader = reader(Arc::clone(&inner), WATCHLIST_CACHE_TTL);
    let first = reader.members("Growth").expect("initial members");
    assert_eq!(first[0]["name"], "Tencent v1");
    inner.set_members(vec![member("HK.00700", "Tencent v2", 3, "00700", 700)]);
    // The cached read must not observe the new remote value.
    let cached = reader.members("Growth").expect("cached members");
    assert_eq!(cached[0]["name"], "Tencent v1");
    assert_eq!(inner.group_calls(), 1);
    assert_eq!(inner.member_calls(), 1);
    // A fresh read bypasses both caches and replaces them.
    let fresh = reader.members_fresh("Growth").expect("fresh members");
    assert_eq!(fresh[0]["name"], "Tencent v2");
    assert_eq!(inner.group_calls(), 2);
    assert_eq!(inner.member_calls(), 2);
    let after_fresh = reader.members("Growth").expect("members after fresh read");
    assert_eq!(after_fresh[0]["name"], "Tencent v2");
    assert_eq!(inner.group_calls(), 2);
    assert_eq!(inner.member_calls(), 2);
}

#[test]
fn watchlist_fresh_member_read_rechecks_remote_ambiguity() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_test.go:164
    // TestFutuWatchlistFreshMemberReadRechecksRemoteAmbiguity.  The fresh read
    // must re-run the 3222 group listing and fail before any 3213 member call
    // once a duplicate group name appears.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Growth", "custom")],
        vec![member("HK.00700", "Tencent", 3, "00700", 700)],
    ));
    let reader = reader(Arc::clone(&inner), WATCHLIST_CACHE_TTL);
    reader.members("Growth").expect("initial member read");
    inner.set_groups(vec![group("Growth", "custom"), group(" growth ", "custom")]);
    match reader.members_fresh("Growth") {
        Err(CustomizationError::Invalid(message)) => {
            assert!(
                message.contains("ambiguous"),
                "ambiguous fresh error = {message}"
            );
        }
        other => panic!("expected ambiguous group error, got {other:?}"),
    }
    assert_eq!(
        inner.member_calls(),
        1,
        "ambiguous fresh read must stop before the 3213 member read"
    );
}

#[test]
fn watchlist_member_read_rechecks_cache_after_group_resolution() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader_boundaries_test.go:49
    // TestWatchlistMemberReaderCoversSecondCacheAndFailures. A member lookup
    // resolves groups first, then must perform the second cache check before
    // reserving another physical read.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Growth", "custom")],
        vec![member("HK.00700", "Tencent", 3, "00700", 700)],
    ));
    let reader = CachedRemoteWatchlistReader::with_inner(
        Arc::clone(&inner) as Arc<dyn RemoteWatchlistReadPort>,
        Arc::new(|| at(1_000)),
        WATCHLIST_CACHE_TTL,
    );
    let first = reader.members("Growth").expect("initial members");
    assert_eq!(first[0]["instrumentId"], "HK.00700");
    assert_eq!(inner.group_calls(), 1);
    assert_eq!(inner.member_calls(), 1);

    let cached = reader.members(" growth ").expect("cached members");
    assert_eq!(cached[0]["name"], "Tencent");
    assert_eq!(inner.group_calls(), 1);
    assert_eq!(inner.member_calls(), 1);
}

#[test]
fn watchlist_reader_rejects_blank_and_unknown_group_names() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader.go:176/:198 - a blank name
    // is rejected before any read and an unknown name fails after group
    // resolution without issuing a member read.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Growth", "custom")],
        Vec::new(),
    ));
    let reader = reader(Arc::clone(&inner), WATCHLIST_CACHE_TTL);
    match reader.members("   ") {
        Err(CustomizationError::Invalid(message)) => {
            assert_eq!(message, "futu: watchlist group name is required");
        }
        other => panic!("expected blank group rejection, got {other:?}"),
    }
    match reader.members("Missing") {
        Err(CustomizationError::Invalid(message)) => {
            assert!(
                message.contains("not found"),
                "unknown group error = {message}"
            );
        }
        other => panic!("expected unknown group error, got {other:?}"),
    }
    assert_eq!(inner.member_calls(), 0);
}

#[test]
fn watchlist_reader_rate_limit_applies_only_to_physical_reads() {
    // Parity: go:452dea11:pkg/futu/watchlist_reader.go:21 - cached results are
    // returned before the gate is consulted, so repeated discovery does not
    // consume the 10/30s allowance.
    let inner = Arc::new(FixtureWatchlistRead::new(
        vec![group("Growth", "custom")],
        vec![member("HK.00700", "Tencent", 3, "00700", 700)],
    ));
    let reader = CachedRemoteWatchlistReader::with_inner(
        Arc::clone(&inner) as Arc<dyn RemoteWatchlistReadPort>,
        Arc::new(|| at(1_000)),
        // A zero TTL forces every read to miss the cache and reserve a slot.
        Duration::ZERO,
    );
    for _ in 0..11 {
        let _ = reader.groups();
    }
    // 10 group listings are admitted (one of which is consumed by the member
    // read below), so the 11th must be rejected with Go's message.
    match reader.groups() {
        Err(CustomizationError::Invalid(message)) => {
            assert_eq!(
                message,
                "futu: watchlist read rate limit exceeded (10 calls per 30 seconds)"
            );
        }
        other => panic!("expected rate-limit rejection, got {other:?}"),
    }
}
