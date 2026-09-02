# Market-Data Provider Actions Group Ledger

> Historical/rehearsal evidence notice (pre-2026-08-31): owner labels, route counts, and Go/retained statuses in this ledger describe the qualification snapshot at capture time, not current production ownership.
>
> Current route truth is derived from `node scripts/rust-migration/check-stage9-route-coverage.mjs` and `tests/fixtures/rust-migration/stage9/route-ownership.json`; formal release truth is `node scripts/rust-migration/check-stage9-closeout.mjs --check`. The original evidence below is intentionally retained verbatim.

- Group: `market-data-provider-actions`
- Tier: B; provider-backed action and snapshot rehearsal only.
- Operations: 5 unique POST operations. The duplicated baseline entry for option analysis is represented once; prediction subscription lease mutations are excluded.
- Production owner: Go. Rust is limited to an explicitly injected `MarketDataProviderActionsPort` and replay API; it must not start Provider/OpenD, open SQLite, create subscriptions, or mutate provider state.
- Fixture: `tests/fixtures/rust-migration/stage9/market-data-provider-actions.json` (50 cases)
- Go reference: `scripts/rust-migration/stage9_market_data_provider_actions_reference_test.go`
- Rust replay: `crates/jftrade-engine/tests/stage9_market_data_provider_actions.rs`
- Differential: `node scripts/rust-migration/check-stage9-market-data-provider-actions.mjs`
- Status: `cutover-test-only`; `productionOwner=go`; `goRemovalStatus=retained`
- Product evidence: authenticated Rust product replay and Go loopback rehearsal cover browser/private-boundary forwarding, unavailable/rate-limit/error responses, timeout, cancellation, crash fail-closed behavior, Go rollback/restart recovery, default-profile isolation, and unchanged settings bytes.

## Operation Set

| Method | Path | Scope |
| --- | --- | --- |
| POST | `/api/v1/market-data/instruments/normalize` | Provider-backed instrument normalization. |
| POST | `/api/v1/market-data/options/analysis/{instrumentId}` | Option analysis query; existing GET read route and subscription routes are outside this group. |
| POST | `/api/v1/market-data/options/events/zero-dte-contracts` | Zero-DTE contract action using a caller-supplied chain locator. |
| POST | `/api/v1/market-data/prediction/combos/quotes` | Prediction combo RFQ/quote action. |
| POST | `/api/v1/market-data/snapshots` | Bounded non-subscription batch snapshot action. |

## Three-Way Review

quirk: Normalize maps any provider error to `400 MARKET_INSTRUMENT_INVALID`, preserving the Go error text. This includes a fixture provider failure; Rust must replay the observable mapping rather than improve it.
scope: instrument normalization
evidence: Go handler, frozen normalize-provider-error case, Rust replay
classification: go-behavior
judgment: intended compatibility
disposition: reproduce until the Go owner is cut over
risk: medium
owner: Go until cutover

quirk: The option analysis POST handler copies the JSON body over query-derived parameters. An empty object leaves query-derived values in place, while a body `operation` replaces repeated query `operation` values. Go also accepts the current empty-object underlying case without an operation-specific error.
scope: option analysis
evidence: Go route query/body merge, body-precedence and empty-object fixture cases, Rust raw request replay
classification: go-behavior
judgment: intended compatibility
disposition: keep normalization and validation in Go; Rust preserves the captured projection
risk: high
owner: Go until cutover

quirk: Zero-DTE body broker/account/trading-environment values take precedence over query values; market, underlying, expiry, chain, sort, and option type come from the JSON body. Non-US, missing chain context, warming, busy, 403, 422, and 502 branches have distinct Go status/code/message behavior.
scope: zero-DTE contracts
evidence: Go handler/service, body-context fixture, error matrix, Rust replay
classification: go-behavior
judgment: intended compatibility
disposition: do not parse or reconstruct this precedence in Rust
risk: high
owner: Go until cutover

quirk: Prediction combo quotes persist a quote through `SavePredictionQuote` and derive a 30-second expiry from Go receipt time. This is a write/state side effect even though the public operation is a quote action.
scope: prediction combo quotes
evidence: `internal/productfeatures/prediction_quotes.go`, fixture quote store, Rust port design
classification: ownership
judgment: qualification blocker
disposition: Rust remains a mock projection only; no SQLite quote store or duplicate write is permitted
risk: critical
owner: integration plus Go owner

quirk: Batch snapshots append `instrumentIds` before `symbols`, trim and uppercase each value, and stable-deduplicate. JSON `null` and omitted arrays are distinct input spellings but both can produce the same normalized request when the other array supplies a symbol; no subscription is created.
scope: batch snapshots
evidence: `normalizeSnapshotSymbols`, order/dedup/null/omitted fixture cases, Rust raw port replay
classification: go-behavior
judgment: intended compatibility
disposition: retain exact fixture projection; do not normalize in Rust
risk: high
owner: Go until cutover

quirk: Snapshot and combo rate-limit errors round `6.5s` up to `Retry-After: 7`; warming and busy use `1` and `2`. Provider 4xx errors preserve their status with `PROVIDER_REQUEST_FAILED`, while generic failures map to `502 BROKER_FEATURE_FAILED`.
scope: all provider-backed action errors
evidence: Go `writeQueryError`, frozen 403/422/429/502/warming/busy cases, Rust `ApiFailure` mapping
classification: wire
judgment: intended compatibility
disposition: preserve status, code, message, and retry metadata exactly
risk: high
owner: integration transport for final header emission

quirk: Repeated scenario requests share the same public method/path/query/body, so the Rust fixture port uses explicit per-key occurrence queues. This is fixture injection for deterministic replay, not provider-state emulation.
scope: provider error matrix
evidence: repeated combo and snapshot request paths in the fixture, Rust replay port
classification: fixture
judgment: harness design
disposition: keep occurrence ordering explicit and regenerate the fixture from Go before changing cases
risk: medium
owner: worker

## Qualification Blockers

- Go remains the sole production owner and no Rust default route registration is present in this worker commit.
- The combo quote persistence side effect is not Rust-qualified; a real store adapter, ownership transfer, recovery behavior, and no-double-write evidence are required before qualification.
- Provider lifecycle, capability selection, decimal/normalization semantics, four-platform packaging, signing, security, recovery, hard-cut, and serial Go/Wails deletion remain integration/release gates.
- The authenticated product composition is test-cutover-only: it accepts an injected fixture port and leaves default route registration unchanged. No Provider/OpenD, durable quote store, subscription registry, SQLite, notification, or user-visible side effect is enabled.

## Completed Evidence

- `go test ./internal/app/apiserver/servercoretest -run '^TestMarketDataProviderActionsRehearsalPreservesBrowserBoundaryAndRecoversAcrossRestart$' -count=1` passed.
- `cargo test -p jftrade-engine --lib market_data_provider_actions -- --nocapture` passed.
- `node scripts/rust-migration/check-stage9-market-data-provider-actions.mjs` passed for the 50-case Go fixture and Rust leaf replay.
- `pnpm run test:rust:stage9:product-differential`, `pnpm run check:quick`, and `pnpm run check:rust` passed; these gates do not change the `cutover-test-only` status or Go production ownership.
- Keep `/api/v1/market-data/prediction/contracts/{code}/subscriptions` and its DELETE lease route under the existing Go owner; preserve `ApiFailure` retry headers through shared transport.
