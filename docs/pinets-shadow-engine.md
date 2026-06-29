# PineTS Shadow Engine

JFTrade keeps `sourceFormat=pine-v6` and `runtime=pine-go-plan` as the
authoritative production path. The optional `pinets-shadow` engine runs PineTS
side by side for diagnostics and compatibility reporting.

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

The first phase is limited to indicator and signal comparison. Orders, broker
emulation, backtest fills, portfolio accounting, and live trading remain owned
by the existing Go runtime.
