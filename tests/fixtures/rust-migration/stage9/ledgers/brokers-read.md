# Brokers Read Group Ledger

- Group: `brokers-read`
- Tier: B: broker-backed balances, orders, fills, quotes and runtime projections depend on the Go broker lifecycle, so Rust is test-cutover-only.
- Owner: Go remains production owner. Rust accepts a consumer-owned `BrokerReadSnapshotPort` only in `ProductConfig::test_cutover`; it never discovers accounts, connects OpenD, activates a broker, or writes trading state.
- Fixture: `tests/fixtures/rust-migration/stage9/broker-read.json`
- Differential: `TestStage9BrokerReadFixtureMatchesCurrentGoOwner` plus parameterized Rust route tests.

The thirteen GET operations preserve the current Go envelopes and degraded/no-provider fallback. `GET /api/v1/brokers/capabilities` preserves the capability catalog; runtime, funds, positions, orders, fills, cash flows, order fees, margin ratios, max trade quantities, quote, K-lines and securities preserve their broker-neutral response projections and query-bearing paths.

Snapshot adapter failures map to `503 BROKER_READ_UNAVAILABLE`; malformed snapshot requests map to `400 BAD_REQUEST`. The Go fixture freezes clock-dependent `checkedAt`, `observedAt`, and `quoteAt` fields to `fixture-time` without changing any other wire field.

All thirteen operations remain `cutover-test-only`, `productionOwner=go`, and `goRemovalStatus=retained`. Broker order POST/DELETE/unlock operations remain `remaining` and are intentionally outside this read-only batch.
