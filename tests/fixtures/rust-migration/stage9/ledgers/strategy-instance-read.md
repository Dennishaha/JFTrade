# Strategy Instance Read Group Ledger

- Group: `strategy-instance-read`
- Tier: B: the list combines catalog, definition-sync, runtime observation, and persisted activity projections; logs/audit expose mutable runtime activity and pagination boundaries.
- Owner: Go remains the production owner of the strategy catalog, definition store, runtime manager, activity SQLite store, and all strategy lifecycle writes. Rust accepts a complete `StrategyReadSnapshotPort` only in explicit `ProductConfig::test_cutover` wiring and never opens the strategy database, starts PineTS, changes runtime state, or emits activity.
- Fixture: `tests/fixtures/rust-migration/stage9/strategy-instance-read.json`
- Differential: `TestStage9StrategyInstanceReadFixtureMatchesCurrentGoOwner` plus parameterized Rust coverage in `product_strategies_tests.rs`.

| Method | Path | Request and response contract | Error branches |
| --- | --- | --- | --- |
| GET | `/api/v1/strategies` | Returns the Go `[]InstanceView` projection in `CreatedAt` ascending order, including normalized binding/params, definition-sync state, runtime observation precedence, and recent log tail. | Go catalog failures preserve the existing successful projection behavior; the explicit Rust snapshot port is unavailable only outside the fixture contract and fails closed. |
| GET | `/api/v1/strategies/{instanceId}/logs` | Returns `{instanceId, logs, page}`. Query supports `limit`, `offset`, `level`, `fromTime`, and `toTime`; default limit is `500`, values below `1` become `1`, values above `5000` become `5000`, and negative offset becomes `0`. | Malformed query returns `400 BAD_REQUEST` with `invalid logs query`; unknown instances return `404 NOT_FOUND` / `resource not found`. |
| GET | `/api/v1/strategies/{instanceId}/audit` | Returns `{instanceId, entries, page}` with the same pagination/time bounds; `kind` is trimmed without changing case. | Malformed query returns `400 BAD_REQUEST` with `invalid audit query`; unknown instances return `404 NOT_FOUND` / `resource not found`. |

Known behavior and quirks:

- `quirk: activity count/list failures degrade to 200 with an empty page instead of surfacing 5xx | 范围: strategy-instance-read / GET /api/v1/strategies/{instanceId}/logs and /audit | 证据: internal/strategy/catalog/activity.go and logs-degraded fixture case | 分类: go-behavior | 判定: intended | 处置: 复刻，待硬切后修复 | 风险: low | owner: Go | 后续: 硬切前确认是否继续接受 best-effort activity`.
- Logs are serialized as raw strings only; audit entries retain `instanceId`, `kind`, `detail`, and RFC3339Nano `at`. Store ordering is `at_ms DESC, id DESC`; the fixture freezes timestamps and query boundaries.

All three operations are now `cutover-qualified`, `productionOwner=go`, and `goRemovalStatus=retained`, based on the authenticated sidecar wire/error/timeout/crash/restart rehearsal. Strategy lifecycle POST/PUT/DELETE routes, definition writes, PineTS, runtime subscriptions, notifications, and SQLite writes remain outside this slice; Go remains their sole production owner.
