# Alerts Write Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `alerts-write`
- Tier: A, mutation operations
- Operations: `POST /api/v1/alerts/price`; `POST /api/v1/alerts/option-events`
- Current production owner: Go product feature API/service and broker `CustomizationService`; Rust has no production owner.
- Current route ownership: `cutover-qualified`; both operations register only when the explicit product test-cutover profile supplies `AlertWritePort`. Go remains the production owner and `goRemovalStatus=retained`.
- Fixture: `tests/fixtures/rust-migration/stage9/alerts-write.json`
- Go reference: `scripts/rust-migration/stage9_alerts_write_reference_test.go`
- Rust leaf/test: `crates/jftrade-engine/src/product_alerts_write_port.rs`; `crates/jftrade-engine/tests/product_alerts_write_tests.rs`
- Differential: `node scripts/rust-migration/check-stage9-alerts-write.mjs`

| Method | Path | Request, response, and side-effect contract | Error branches covered |
| --- | --- | --- | --- |
| POST | `/api/v1/alerts/price` | JSON object, JSON `null`, and empty object are accepted by the Go binder. The action is `set`, feature ID is `alerts.price.set`, query `brokerId` and `accountId` use the first value, and the fake broker records one `ApplyCustomization` call plus the normalized payload state. Success is `200` with the Go provider attribution envelope. | Empty, malformed, and array bodies are `400 BAD_REQUEST` before capability resolution; missing or unavailable broker capability and missing `CustomizationService` are `409 BROKER_CAPABILITY_UNAVAILABLE`; provider 4xx is `PROVIDER_REQUEST_FAILED`; provider/internal failure is `502 BROKER_FEATURE_FAILED`; snapshot rate limiting is `429 MARKET_SNAPSHOT_RATE_LIMITED` with rounded-up `Retry-After`. |
| POST | `/api/v1/alerts/option-events` | Same wire and mutation rules with feature ID `alerts.option_event.set`; repeated `brokerId` preserves the first value; payload and call evidence are fixture-owned. | Same body precedence, capability, provider, internal-failure, and rate-limit mappings as the price route. |

## Three-way review and quirks

The Go reference uses the real Gin route and product feature service with an injected in-memory broker. The Rust replay uses only the consumer-owned `AlertWritePort` fixture adapter; it does not connect OpenD/Futu, a provider, SQLite, or production state. The fixture compares status, `Content-Type`, optional `Retry-After`, complete JSON envelope, normalized query/action payload, and `ApplyCustomization` call count.

quirk: A JSON `null` body is accepted by `ShouldBindJSON` as a nil customization payload; a nil customization result is normalized by the Go service and the `entries` field remains omitted.
范围: `alerts-write` / `POST /api/v1/alerts/price`
证据: Go reference case `price-null-body-nil-result`; fixture response and call trace; Rust fixture replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Preserve null and omitted-field behavior through qualification; do not add Rust-only body validation.

quirk: An empty JSON object is accepted and sent as an empty payload; an empty customization result also omits `entries` from the success data projection.
范围: `alerts-write` / `POST /api/v1/alerts/option-events`
证据: Go reference case `option-events-empty-object`; fixture payload state, call trace, and response; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Keep empty-object/null/omitted distinctions explicit in any future production adapter.

quirk: Repeated `brokerId` query keys use the first value, while the write action receives the resolved broker ID and preserves `accountId` only when non-empty.
范围: `alerts-write` / `POST /api/v1/alerts/option-events`
证据: Go reference case `option-events-success-repeated-broker-query`; fixture action and provider attribution; Rust query replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: low
owner: Go until cutover
后续: Keep first-value query semantics in the shared integration adapter; do not normalize duplicate query keys differently.

quirk: The shared Go error mapper labels a customization rate-limit failure `MARKET_SNAPSHOT_RATE_LIMITED` and rounds a 2.5 second delay up to `Retry-After: 3`.
范围: `alerts-write` / `POST /api/v1/alerts/price`
证据: Go reference case `generic-rate-limit-retry-after`; `internal/api/productfeatures/routes.go`; fixture headers and error envelope; Rust replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切后修复
风险: medium
owner: Go/integration branch
后续: Preserve the exact code and header until a separate public-contract review approves a correction.

quirk: Repeating the same alert write is forwarded to the broker port twice; the current Go contract does not provide an idempotency key or deduplicate repeated requests.
范围: `alerts-write` / both POST routes
证据: Go reference case `price-repeated-write-is-forwarded-twice`; two recorded `ApplyCustomization` calls; Rust leaf and product replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切前明确幂等策略
风险: high
owner: Go until an approved Rust production owner and idempotency policy exist
后续: Preserve the observable forwarding behavior for compatibility; complete the hard-cut idempotency decision and durable writer evidence before switching ownership.

quirk: A failed broker write does not poison the next request: the first request maps to `502 BROKER_FEATURE_FAILED`, and the next identical request is forwarded and can succeed.
范围: `alerts-write` / `POST /api/v1/alerts/option-events`
证据: Go reference case `option-events-failed-write-recovers-on-next-request`; sequential call trace; Rust product replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切前补齐 production recovery evidence
风险: medium
owner: Go until cutover
后续: Keep failure/recovery state request-local in the test adapter; prove durable owner recovery before any production switch.

quirk: Client cancellation and deadline errors are forwarded to the broker port and map to `502 BROKER_FEATURE_FAILED`; neither path retries or falls back to Go inside the request.
范围: `alerts-write` / both POST routes
证据: Go reference cases `price-cancelled-request-defaults-to-broker-failure` and `option-events-deadline-request-defaults-to-broker-failure`; authenticated rehearsal timeout/error/crash cases; Rust product replay
分类: go-behavior
判定: intended
处置: 复刻，待硬切前补齐 real cancellation fencing
风险: high
owner: Go/integration branch
后续: Preserve status/message precedence and no-request-level fallback; complete production cancellation, timeout, and lock-release evidence before owner switch.

quirk: The authenticated rehearsal restart returns to the Go owner only after the sidecar is closed and a new Go router is constructed; settings bytes remain unchanged across success, error, timeout, crash, fallback, and restart.
范围: `alerts-write` / product rehearsal boundary
证据: `TestAlertsWriteRehearsalFencesOwnersAndRecoversAcrossRestart`; Rust product restart/settings-byte assertions
分类: harness
判定: intended
处置: 复刻，保留 Go-only rollback
风险: medium
owner: Go/integration branch
后续: Keep rollback restart-scoped and fail closed; backup/restore, release, and final unique-owner gates remain external to this local qualification.

quirk: The first local Go reference attempt passed `*broker.CustomizationAction` to a value helper and failed to compile before fixture generation.
范围: `alerts-write` / Go reference fixture harness
证据: initial `go test` failure in `scripts/rust-migration/stage9_alerts_write_reference_test.go`; corrected helper call; regenerated fixture; Rust replay
分类: harness
判定: deviated
处置: 修复 fixture/harness
风险: low
owner: worker
后续: Keep the corrected reference test and rerun the group checker when the Go owner or fixture changes.

## Cutover-qualified status

The Go reference fixture, Rust leaf replay, authenticated product rehearsal, and explicit product test-cutover adapter are green. The 18 fixture cases cover success, null/empty/malformed input, capability/provider/internal/rate-limit failures, duplicate writes, failure-to-recovery, cancellation, and deadline mapping. The rehearsal covers success, error, timeout, crash, Go fallback, restart, private authentication context, and settings-byte isolation. The two POST routes are absent from the default profile and are registered only with an injected `AlertWritePort`; Go remains the only production owner. The adapter does not connect OpenD/Futu, a provider, SQLite, notification/task runtime, or production state.

This group is `cutover-qualified`, not a production migration. The Go-compatible repeated-write behavior is explicitly non-idempotent and remains a high-risk hard-cut policy item. Production-owner evidence remains open for durable transaction/rollback boundaries, production cancellation/timeout fencing, notification/task isolation, four-platform release and signing, independent security review, SBOM, backup/restore, and final unique-owner approval. No real provider, OpenD, broker lifecycle, or production state mutation is permitted in this leaf.

## Integration Review

- Product wiring adds a private `AlertWritePort`, `AlertsWrite` capability, and exact POST dispatch through the existing product envelope. The default profile reports 48 routes; the explicit alert test port reports 50.
- The unified product differential runs the Go reference and both product integration cases, while the group checker replays the leaf fixture. `route-ownership.json` records both operations as `cutover-qualified` with `productionOwner=go` and `goRemovalStatus=retained`.
- The local qualification evidence is closed for contract, differential, error precedence, recovery rehearsal, authenticated fencing, default-profile isolation, and no-local-side-effect checks. Formal production-owner, release/signing, security, SBOM, backup/restore, and hard-cut gates remain open in the Stage 9 closeout manifest.
