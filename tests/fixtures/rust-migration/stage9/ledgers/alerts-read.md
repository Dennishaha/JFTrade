# Alerts Read Group Ledger

- Group: `alerts-read`
- Tier: C in the route inventory, with explicit test-cutover only because the projection depends on the Go broker/OpenD capability provider.
- Owner: Go remains the production owner. Rust accepts a consumer-owned `AlertSnapshotPort` only in `ProductConfig::test_cutover`; no provider, OpenD connection, notification, or write route is started.
- Fixture: `tests/fixtures/rust-migration/stage9/alerts-read.json`
- Differential: `TestStage9AlertsReadFixtureMatchesCurrentGoOwner` plus `product::tests::alerts_read_routes_match_go_fixture_as_cutover_only_batch`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/alerts/price` | Query is normalized as `brokerId`, `market`, `pageSize`, repeated `tag`, and typed `params`; response is the Go envelope data with `asOf`, `entries`, `hasMore`, `metadata`, `provider`, and `total`. | Invalid query/path stays Go-compatible `400 BAD_REQUEST`; unavailable broker/capability or snapshot port is `503 ALERTS_UNAVAILABLE`; provider failures preserve the Go error envelope. |
| GET | `/api/v1/alerts/option-events` | Query is normalized as `brokerId`, `market`, `cursor`, `pageSize`, `operation`, and typed `params`; response uses the same paged provider envelope with option-event entry fields. | Invalid query/path stays `400 BAD_REQUEST`; unavailable broker/capability or snapshot port is `503 ALERTS_UNAVAILABLE`; provider failures preserve the Go error envelope. |

Known quirks: repeated query keys and numeric/boolean parameter coercion are reproduced from the Go feature service. The fixture's provider timestamps are normalized to the fixed corpus time; no behavior is corrected in this slice.

Route ownership for both operations is `cutover-test-only`, `productionOwner=go`, `goRemovalStatus=retained`. The default shadow catalog does not register these routes.
