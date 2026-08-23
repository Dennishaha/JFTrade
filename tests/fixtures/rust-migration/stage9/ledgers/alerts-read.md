# Alerts Read Group Ledger

- Group: `alerts-read`
- Tier: C in the route inventory. Qualification uses an authenticated Go-sidecar rehearsal; the Rust snapshot port remains explicit test-cutover-only because the projection depends on the Go broker/OpenD capability provider.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `AlertSnapshotPort` only in `ProductConfig::test_cutover`; no provider, OpenD connection, notification, or write route is started. The default shadow profile does not register these routes.
- Fixture: `tests/fixtures/rust-migration/stage9/alerts-read.json`
- Differential: `TestStage9AlertsReadFixtureMatchesCurrentGoOwner` plus `product::tests::alerts_read_routes_match_go_fixture_as_cutover_only_batch`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/alerts/price` | Query is normalized as `brokerId`, `market`, `pageSize`, repeated `tag`, and typed `params`; response is the Go envelope data with `asOf`, `entries`, `hasMore`, `metadata`, `provider`, and `total`. | Invalid query/path stays Go-compatible `400 BAD_REQUEST`; missing broker capability is `409 BROKER_CAPABILITY_UNAVAILABLE`; unavailable snapshot port is `503 ALERTS_UNAVAILABLE`; provider failures preserve the Go error envelope. |
| GET | `/api/v1/alerts/option-events` | Query is normalized as `brokerId`, `market`, `cursor`, `pageSize`, `operation`, and typed `params`; response uses the same paged provider envelope with option-event entry fields. | Invalid query/path stays `400 BAD_REQUEST`; missing broker capability is `409 BROKER_CAPABILITY_UNAVAILABLE`; unavailable snapshot port is `503 ALERTS_UNAVAILABLE`; provider failures preserve the Go error envelope. |

Known quirks: repeated query keys and numeric/boolean parameter coercion are reproduced from the Go feature service. The fixture's provider timestamps are normalized to the fixed corpus time; no behavior is corrected in this slice.

Route ownership for both operations is `cutover-qualified`, `productionOwner=go`, `goRemovalStatus=retained`, based on the authenticated sidecar wire/restart rehearsal. The default shadow catalog still does not register these snapshot-port routes.

Quirk: the original ownership evidence referenced a non-existent alert-specific differential command. This harness/ledger drift was confirmed against `package.json` and the shared Stage 9 runner; it is corrected to `pnpm run test:rust:stage9:product-differential` without changing observable behavior.

## Quirk review

### Q1: snapshot-port error classes were narrower than the Go route

quirk: the existing Rust `AlertSnapshotError` only represented unavailable
snapshots and mapped every injected snapshot failure to `503 ALERTS_UNAVAILABLE`.
The Go owner distinguishes capability failures (`409 BROKER_CAPABILITY_UNAVAILABLE`)
from generic/provider failures (`502 BROKER_FEATURE_FAILED`, or a preserved
provider 4xx response).

范围: `alerts-read` / `GET /api/v1/alerts/price` and
`GET /api/v1/alerts/option-events`.

证据: Go `internal/api/productfeatures/routes.go` `writeQueryError`, current
Rust `crates/jftrade-engine/src/product_snapshot_errors.rs` and
`crates/jftrade-engine/src/product_wire.rs`, pending alerts wire fixture and
Rust replay.

分类: rust-implementation

判定: confirmed and resolved in the Rust compatibility patch; the integration
branch reviewed the shared mapping before qualification.

处置: Rust preserves the Go capability (`409 BROKER_CAPABILITY_UNAVAILABLE`)
and provider (`502 BROKER_FEATURE_FAILED`) mappings while retaining the
existing explicit snapshot-port-unavailable `503 ALERTS_UNAVAILABLE` fence.

风险: medium

owner: Rust worker

后续: preserve the explicit unavailable-port fence and rerun the exact alerts
replay/rehearsal tests before any future owner change.

三方复核: Go reference `wireCases`, Rust replay, and the alerts rehearsal
proxy now agree on success/empty/error envelopes, JSON content type, request
ID forwarding, timeout/crash fail-closed behavior, and restart-time Go
rollback. The error-class difference was Rust implementation drift; no Go
observable behavior was changed and no live broker/OpenD was used.

## Verification record

- Go observable fixture: `go test ./scripts/rust-migration -run '^TestStage9AlertsReadFixtureMatchesCurrentGoOwner$' -count=1`.
- Go authenticated proxy rehearsal: `go test ./internal/app/apiserver/servercoretest -run '^TestAlertsReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`.
- Rust replay/auth/fail-closed tests: `cargo test -p jftrade-engine --lib 'product::tests::alerts_read_tests::' -- --nocapture` (5 alerts-read tests).
- Shared differential: `pnpm run test:rust:stage9:product-differential`; the integration branch also records the exact sidecar rehearsal and promotes both GET operations to `cutover-qualified` while retaining Go as production owner.
