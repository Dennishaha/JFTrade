# Storage overview read

- Group: `storage-overview-read`.
- Tier: C: this is a side-effect-free immutable projection while persistent
  task queues remain unowned.
- Operation: `GET /api/v1/system/storage/overview`.
- Owner: Go remains the production owner. Both implementations expose the same
  four empty arrays for pending outbox, jobs, audit logs and execution
  commands; Rust opens no database or queue.

The Rust authenticated product test asserts the complete data projection. The
Go authenticated sidecar rehearsal preserves exact status/body/headers and
verifies error, timeout, crash and restart-time Go rollback.

Verification: `go test ./internal/app/apiserver/servercoretest -run '^TestStorageOverviewReadRehearsalPreservesWireAndRequiresRestartForGoRollback$' -count=1`; `cargo test -p jftrade-engine --lib product::tests::system_control_read_tests::storage_overview_matches_the_go_empty_projection_behind_authentication -- --exact`; `pnpm run test:rust:stage9:product-differential`.

The route is `cutover-qualified`, `productionOwner=go`, and
`goRemovalStatus=retained`. Current coverage is 2 shadow / 133
cutover-test-only / 143 cutover-qualified / 0 remaining / 0 Rust production
owner.
