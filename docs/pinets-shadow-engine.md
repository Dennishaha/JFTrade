# PineTS Shadow Engine

JFTrade's current Pine runtime identity is `sourceFormat=pine-v6` with
`runtime=pine-pinets`. This document covers the separate stdio
`pinets-shadow` harness used for diagnostics, compatibility reporting, and
community AGPL license checks. It does not add a public HTTP port and does not
change order execution or live trading authority.

## Modes

- `off`: default; no PineTS worker is started.
- `shadow`: runs PineTS in the background and reports diagnostics; Go results
  remain authoritative.
- `community-agpl`: same as shadow, with the community AGPL posture made
  explicit for deployments that expose the feature to network users.

Set `JFTRADE_PINETS_MODE=shadow` or `JFTRADE_PINETS_MODE=community-agpl` to
enable the experiment. Set `JFTRADE_PINETS_WORKER_PATH` to override the worker
script path.

## Contract

The Go backend talks to `scripts/pinets-worker.mjs` over newline-delimited JSON.
The worker accepts `engineInfo` and `runIndicator` methods. `runIndicator`
receives Pine source, symbol, timeframe, optional OHLCV candles, warmup bars,
mode, and timeout. It returns engine version, AGPL license metadata, plots,
last-value signals, diagnostics, metadata, and runtime duration.

The shadow harness is limited to indicator and signal comparison. Orders,
broker emulation, backtest fills, portfolio accounting, and live trading remain
outside this stdio report path.

## Corpus Report

Run the real-K-line shadow corpus report with:

```bash
go test ./pkg/backtest -run TestPinetsShadowCorpusReport -count=1 -v
```

The test opens a `FutuKLineStore`, streams OHLCV rows through the backtest store
path, sends those candles to `scripts/pinets-worker.mjs`, and writes a JSON
report. By default it creates a deterministic fixture database with benchmark
K lines. To use an operator-synced database instead, set:

- `JFTRADE_PINETS_REPORT_DB`: path to the SQLite K-line database.
- `JFTRADE_PINETS_REPORT_SYMBOL`: defaults to `US.AAPL`.
- `JFTRADE_PINETS_REPORT_TIMEFRAME`: defaults to `1m`.
- `JFTRADE_PINETS_REPORT_UNTIL`: RFC3339/RFC3339Nano upper bound for operator
  databases; defaults to current UTC time.
- `JFTRADE_PINETS_REPORT_LIMIT`: candle count, defaults to `512`.
- `JFTRADE_PINETS_SHADOW_MAX_CASES`: corpus case cap, defaults to `40`.
- `JFTRADE_PINETS_SHADOW_REPORT_PATH`: explicit JSON report path.

The report includes engine version, AGPL license, configured mode, data source,
case totals, `goCompile`, `goBacktest`, `pinetsRun`, diagnostics, plot
summaries, signals, unsupported reasons, runtime duration, and report-only plot
parity metadata. Pinets runtime failures and parity mismatches are recorded in
the report but do not fail the test; worker protocol failures, report write
failures, and non-AGPL license metadata do fail.
