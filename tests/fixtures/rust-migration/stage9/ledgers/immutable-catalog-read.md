# Immutable catalog read

- Group: `immutable-catalog-read`
- Tier: C: both routes are deterministic, side-effect-free catalog projections.
- Operations: `GET /api/v1/adk/agent-templates` and `GET /api/v1/research/screens/catalog`.
- Production owner: Go remains the owner. Rust serves the same two operations through the authenticated loopback rehearsal; no SQLite, Provider/OpenD, Assistant runtime, notification, or write state is touched.
- Fixture/reference: `scripts/rust-migration/stage9_agent_templates_reference_test.go`, `scripts/rust-migration/stage9_screen_catalog_reference_test.go`, and the pinned catalog fixtures.
- Rust replay: `crates/jftrade-engine/src/product_tests.rs` static template/catalog tests plus the shared product differential.
- Product/restart evidence: `internal/app/apiserver/servercoretest/rehearsal_catalog_routes_test.go` and `internal/app/apiserver/rustrehearsal` readiness/proxy tests.

## Contract and qualification evidence

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/adk/agent-templates` | No request body; returns the versioned built-in template projection with exact envelope, request ID, content type and cache headers. | The Go owner and Rust replay preserve the response; Rust sidecar startup, private bearer, capability digest, timeout and crash paths fail closed without replaying Go. |
| GET | `/api/v1/research/screens/catalog` | `brokerId` is trimmed/lower-cased and `market` trimmed/upper-cased; Futu and embedded yfinance/AKShare catalogs preserve the full factor wire. | Unsupported markets remain `400 BAD_REQUEST`; unknown brokers remain `409 BROKER_CAPABILITY_UNAVAILABLE`; no catalog/provider error is converted into a success response. |

The group corpus covers every embedded-provider/market variant, unsupported market
and unknown broker branch, exact Go response body/headers, authenticated loopback
selection, Rust error/timeout/crash projection, and restart-time Go rollback. The
Rust route catalog remains the same 26-handler read-only profile; qualification
changes only the ledger readiness status for these two operations.

## Three-way review and quirks

The Go handler/reference, pinned fixtures, Rust replay, and authenticated
loopback/restart harness agree. No Go observable quirk, Rust behavioral difference,
fixture drift, or unresolved error/header/null/ordering issue was found in this
group. The catalog data is embedded and deterministic, so no Provider/OpenD
lifecycle or durable recovery evidence is needed for this C-tier group.

Route ownership is now `cutover-qualified` for both operations,
`productionOwner=go`, and `goRemovalStatus=retained`. This does not switch the
production owner, enable a new write path, or close the global unique-owner,
release, signing, security, recovery, hard-cut, or Go/Wails deletion gates.
