# Research Presets Write Group Ledger

- Group: `research-presets-write`
- Tier: A mutation / state change; this worker remains rehearsal-only.
- Operations: 3
  - `POST /api/v1/research/screens/presets`
  - `PATCH /api/v1/research/screens/presets/{presetId}`
  - `DELETE /api/v1/research/screens/presets/{presetId}`
- The independent screen mutation operation is outside this group and is not covered here.
- Current status: integration-reviewed `cutover-test-only`; all three operations are registered only through the explicit mutation test port, with `productionOwner=go` and `goRemovalStatus=retained`.
- Go owner: `internal/api/research/routes.go`, `internal/research/presets.go`, and `internal/store/research` remain the only production handler, normalization, revision, SQLite, and mutation owner.
- Rust boundary: `product_research_preset_write_port.rs` remains a consumer-owned mutation port with no direct SQLite, Provider/OpenD, notification, or production route registration. `jftrade-store-sqlite::ResearchPresetTestCutoverStore` now supplies a separate durable adapter boundary that requires the exact `cutover-test-only.v1` profile, opens only an existing schema-validated Go-compatible database, and holds an exclusive cross-process writer lease; it is not wired into the product port or default profile.
- Fixture: `tests/fixtures/rust-migration/stage9/research-presets-write.json`
- Go reference: `scripts/rust-migration/stage9_research_presets_write_reference_test.go`
- Differential: `scripts/rust-migration/check-stage9-research-presets-write.mjs`
- Rust behavior test: `crates/jftrade-engine/tests/stage9_research_presets_write.rs`
- Authenticated composition rehearsal: `internal/app/apiserver/servercoretest/rehearsal_research_preset_write_routes_test.go`
- Rust durable store/test: `crates/jftrade-store-sqlite/src/research_preset.rs`; `crates/jftrade-store-sqlite/tests/research_preset_store_contracts.rs`

## Contract ledger

| Method | Path | Request and success wire | Error branches and precedence |
| --- | --- | --- | --- |
| POST | `/api/v1/research/screens/presets` | Strict one-value JSON body with `name` and `definition`; Go normalizes name/definition, creates revision `1`, and returns the complete `ScreenPreset` in the standard `{ok,data,timestamp}` envelope with `Content-Type: application/json; charset=utf-8`. | Malformed, trailing, unknown-field, or wrong-type body: `400 RESEARCH_PRESET_INVALID` / `invalid research screen preset payload`; semantic validation: `400 RESEARCH_PRESET_INVALID`; duplicate name: `409 RESEARCH_PRESET_CONFLICT`; repository unavailable: `503 RESEARCH_PRESET_UNAVAILABLE`; other write failure: `500 RESEARCH_PRESET_FAILED`. Body validation precedes store availability. |
| PATCH | `/api/v1/research/screens/presets/{presetId}` | Strict one-value JSON body with optional `name`/`definition` and required-by-service positive `expectedRevision`; Go trims the path ID, merges omitted fields, validates the current revision, increments revision, and returns the complete `ScreenPreset`. | Invalid ID/body or no mutation field: `400 RESEARCH_PRESET_INVALID`; missing preset: `404 RESEARCH_PRESET_NOT_FOUND`; stale revision or atomic repository race: `409 RESEARCH_PRESET_CONFLICT`; repository unavailable: `503 RESEARCH_PRESET_UNAVAILABLE`; other read/update failure: `500 RESEARCH_PRESET_FAILED`. Syntax validation precedes the port; service checks revision before update. |
| DELETE | `/api/v1/research/screens/presets/{presetId}` | Path ID is bound and trimmed by the service; request body is not decoded; success is `{deleted:true}` in the standard envelope. | Invalid/blank ID: `400 RESEARCH_PRESET_INVALID`; missing preset: `404 RESEARCH_PRESET_NOT_FOUND`; unavailable store: `503 RESEARCH_PRESET_UNAVAILABLE`; other delete failure: `500 RESEARCH_PRESET_FAILED`. |

The fixture fixes repository timestamps and replaces only the handler envelope's dynamic `time.Now().UTC().Format(time.RFC3339Nano)` with the fixture timestamp. Preset `createdAt`/`updatedAt` values are deterministic fake-repository values, so the response projection remains byte-stable after the documented timestamp normalization.

## Fixture and behavior matrix

The Go reference uses the actual Gin route registration and `internal/research.Service` with an in-memory repository. It covers:

- create success, empty/unknown/malformed input, unavailable store, generic write failure with unchanged state, duplicate-name conflict, and eight concurrent duplicate creates;
- update name merge, empty mutation, not-found, stale revision conflict, unavailable read, invalid normalized definition, one failed update followed by a successful retry, and eight concurrent revision-fenced updates;
- delete success followed by not-found repeat, not-found, unavailable store, generic failure with unchanged state, malformed body ignored by DELETE, and cancellation/deadline failure followed by recovery.

Each case records response status, exact contract headers, normalized envelope, whether the service boundary was reached, final preset state, and repository call counts. Concurrent response arrays are sorted only for fixture determinism; the set of statuses, response bodies, revision result, and final state are preserved.

## Owner and fencing evidence

- No production route is registered by this worker, and no default profile behavior changes.
- The Rust port accepts a complete Go-shaped result/error projection; it does not normalize `ScreenDefinitionV2`, open SQLite, or implement a second persistence owner.
- The Rust test asserts the exact three-route inventory and contains no fourth mutation operation.
- Failure cases assert the state/observation emitted by the Go repository remains unchanged; retry/recovery cases prove a later operation can proceed after the failed/cancelled attempt.
- The authenticated loopback mutation rehearsal selects only the three exact operations, forwards no public cookie, and proves that success, duplicate conflict, revision-fenced PATCH and DELETE touch only the isolated rehearsal owner. Rust error, timeout and crash responses never replay the Go fallback owner; restart preserves the rehearsal database while a Go-only rollback restarts with its independent database unchanged.
- The rehearsal boundary deliberately delegates to an isolated temporary Go reference owner. This validates composition-root routing, authenticated transport, durable restart and no-double-write fencing, but it does not fabricate a Rust durable preset repository. The group therefore remains `cutover-test-only`.
- The Rust durable test-cutover store never creates or migrates SQLite. It rejects missing, drifted and corrupted databases, refuses non-test profiles, acquires the existing owner-lock sidecar before opening read-write, maps name/primary-key constraints to conflict, applies revision updates atomically, serializes concurrent mutations, and retains state across close/reopen. One of two concurrent updates against the same revision commits and the other fails closed.
- The durable store is not yet a complete product adapter: `jftrade-research` currently validates only the generic object/name/revision boundary, while Go still owns complete `ScreenDefinitionV2` normalization, generated ID policy, transport error mapping and schema migration. Wiring it now would create an incomplete second behavior owner, so all three routes remain `cutover-test-only`.
- The current route ledger is `23 shadow / 133 cutover-test-only / 122 cutover-qualified / 0 remaining / 0 Rust production owner`. The independent `POST /api/v1/research/screens` mutation is also `cutover-test-only` and remains outside this group.

## Quirks and three-way review

quirk: DELETE accepts and ignores an invalid request body because `routes.go` does not call `BindStrictJSON` for DELETE.
范围: `research-presets-write` / DELETE `/api/v1/research/screens/presets/{presetId}`
证据: Go reference `delete-success-and-repeat`, Rust fixture replay, and `routes.go` lines 68-76.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后评估是否修复
风险: medium
owner: Go / 集成分支
后续: hard-cut 前保留 differential；若产品决定收紧 DELETE body 语义，先单独批准公开契约变更。

quirk: A cancelled request or an already-expired deadline returned by the repository maps through the default Go error branch to HTTP 500 `RESEARCH_PRESET_FAILED`, rather than a transport-specific 499/504.
范围: `research-presets-write` / POST `/api/v1/research/screens/presets`, PATCH `/api/v1/research/screens/presets/{presetId}`
证据: Go reference `cancel-create-recovers` and `timeout-update-fails-closed`; Rust replay preserves the exact status/code/message; fixture state remains unchanged and the next request recovers.
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: high
owner: Go / 集成分支
后续: release-blocker until cancellation/timeout policy is explicitly accepted or changed in a separately versioned contract.

quirk: Concurrent revision responses have nondeterministic completion order; the fixture sorts the response multiset by status and canonical envelope while retaining final state and call counts.
范围: `research-presets-write` / PATCH `/api/v1/research/screens/presets/{presetId}`
证据: Go reference `patch-concurrent-revision-fence`, Rust fixture replay, and the dedicated concurrent leaf test.
分类: harness
判定: intended
处置: 保留 canonicalization；不改变 Go observable response bodies or revision fencing
风险: medium
owner: 集成分支
后续: any future live differential must compare an order-aware trace or explicitly document request correlation before qualification.

quirk: Gin's dynamic envelope timestamp is nondeterministic at request time, so fixture comparison normalizes only that field after validating RFC3339Nano; it does not normalize preset timestamps or any response data.
范围: all 3 operations
证据: Go reference capture and fixed-timestamp Rust replay.
分类: fixture
判定: intended
处置: 修复 fixture/harness canonicalization；保留 timestamp format validation
风险: low
owner: 集成分支
后续: production differential must compare timestamp format/clock semantics separately.

## Verification record

- `gofmt -w scripts/rust-migration/stage9_research_presets_write_reference_test.go`: passed
- `cargo fmt --all -- --check`: passed; the check is workspace-wide formatting only and is not the full Rust quality gate
- Go reference fixture drift test: passed (`go test scripts/rust-migration/stage9_research_presets_write_reference_test.go -run '^TestStage9ResearchPresetsWriteFixtureMatchesCurrentGoOwner$' -count=1`)
- Rust leaf/behavior test: passed, 6 tests (`cargo test -p jftrade-engine --test stage9_research_presets_write -- --nocapture`)
- Rust narrow clippy: passed (`cargo clippy -p jftrade-engine --test stage9_research_presets_write -- -D warnings`)
- dedicated Go/Rust differential: passed (`node scripts/rust-migration/check-stage9-research-presets-write.mjs`)
- Go owner regression packages: passed (`go test ./internal/api/research ./internal/research ./internal/store/research -count=1`)
- differential script syntax: passed (`node --check scripts/rust-migration/check-stage9-research-presets-write.mjs`)
- Shared product differential: passed after integration registration (`pnpm run test:rust:stage9:product-differential`)
- Authenticated mutation rehearsal: passed (`go test ./internal/app/apiserver/servercoretest -run '^TestResearchPresetWriteRehearsalFencesOwnersAndRecoversAcrossRestart$' -count=1`)
- Rust durable store contracts: passed, 3 tests (`cargo test -p jftrade-store-sqlite --test research_preset_store_contracts -- --nocapture`)
- Rust durable store Clippy: passed (`cargo clippy -p jftrade-store-sqlite --all-targets -- -D warnings`)
- `pnpm run check:quick`: passed after the authenticated mutation rehearsal was integrated.
- `pnpm run check:rust`: passed in full, including the Go authenticated rehearsal suite and Stage 9 product differential.
- `pnpm run check:generated`: not applicable; OpenAPI/generated contract was not changed

## Handoff state

- Group: `research-presets-write`
- Tier: A
- Operation count: 3 (22 fixture cases)
- Status: integration-reviewed `cutover-test-only`; no production owner or default profile change
- Next qualification action: complete Go-compatible `ScreenDefinitionV2` normalization and a Go/Rust durable-store corpus, then inject this store through the explicit product test-cutover port. Cancellation mapping, backup/restore, security, release and hard-cut gates still block any owner change.
