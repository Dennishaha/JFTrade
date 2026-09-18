use super::*;

/// Parity: go:452dea11:internal/productfeatures/service_routing_and_validation_test.go:289
/// TestResearchInstitutionDetailQueriesRequireInstitutionID
///
/// Go's `validateResearchInstitutionQuery` only runs for `research.institutions`
/// and only for the four detail operations (profile / distribution /
/// holding_changes / holdings). It parses `institutionId` with
/// `strconv.ParseInt(..., 10, 32)` and rejects a parse error or a value <= 0
/// with `ErrInvalidQuery: operation %s requires a positive integer
/// institutionId`. Non-detail operations (list) and other feature ids are
/// untouched.
///
/// Rust splits the same contract across two owners: the engine's
/// `read_institutions` parses the query into a typed request (`parse_optional_i32`
/// is the strict 32-bit reader, so 0, negatives, non-integers, floats and
/// out-of-range values all fail before the provider), and the integration
/// crate's `InstitutionQuery::validate` re-checks `institution_id > 0` for the
/// four detail operations. This test locks the observable boundary at the
/// engine entry point.
#[test]
fn institution_detail_operations_require_a_positive_institution_id() {
    // The eight allowed operations, mirroring the Go switch: only the four
    // detail operations demand an institution id.
    let detail_operations = ["profile", "distribution", "holding_changes", "holdings"];
    for operation in detail_operations {
        let query = format!("operation={operation}");
        let error = read_institutions(None, "/api/v1/research/institutions", &query)
            .expect_err("a detail operation without institutionId must be rejected");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Invalid(ref message)
                    if message.contains("institutionId")
            ),
            "operation={operation} produced {error:?}"
        );
    }

    // Every rejected institutionId shape: zero, negative, float, non-numeric,
    // and a value beyond i32 (Go rejects int64(1<<32) with ParseInt bitSize 32).
    for value in ["0", "-1", "1.5", "not-an-id", "4294967296"] {
        let query = format!("operation=holding_changes&institutionId={value}");
        let error = read_institutions(None, "/api/v1/research/institutions", &query)
            .expect_err("an invalid institutionId must be rejected");
        assert!(
            matches!(
                error,
                ResearchReadSnapshotError::Invalid(ref message)
                    if message.contains("institutionId")
            ),
            "institutionId={value} produced {error:?}"
        );
    }

    // An omitted id is only invalid for the detail operations; `list` and other
    // feature ids must reach the runtime (and fail there, not in validation).
    let list_error = read_institutions(None, "/api/v1/research/institutions", "operation=list")
        .expect_err("no institution runtime in this fixture");
    assert!(
        matches!(list_error, ResearchReadSnapshotError::Unavailable(_)),
        "operation=list must pass institutionId validation and stop at the runtime: {list_error:?}"
    );

    // A valid id also passes validation and stops at the (absent) runtime, which
    // proves the guard keys on the value rather than rejecting detail requests.
    let valid = read_institutions(
        None,
        "/api/v1/research/institutions",
        "operation=holding_changes&institutionId=202",
    )
    .expect_err("no institution runtime in this fixture");
    assert!(
        matches!(valid, ResearchReadSnapshotError::Unavailable(_)),
        "a positive institutionId must pass validation: {valid:?}"
    );

    // The typed integration owner enforces the same positivity rule for the
    // four detail operations.
    for operation in [
        jftrade_integration_futu::FutuInstitutionOperation::Profile,
        jftrade_integration_futu::FutuInstitutionOperation::Distribution,
        jftrade_integration_futu::FutuInstitutionOperation::HoldingChanges,
        jftrade_integration_futu::FutuInstitutionOperation::Holdings,
    ] {
        let invalid = jftrade_integration_futu::FutuInstitutionQuery {
            operation,
            market: 11,
            institution_id: None,
            ..Default::default()
        };
        let error = invalid
            .validate()
            .expect_err("a detail operation without institutionId must fail typed validation");
        assert!(
            matches!(
                error,
                jftrade_integration_futu::FutuInstitutionQueryError::InvalidQuery(ref message)
                    if message.contains("positive institutionId")
            ),
            "{operation:?} produced {error:?}"
        );
    }
}
