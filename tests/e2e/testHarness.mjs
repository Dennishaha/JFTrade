/**
 * Test harness for JFTrade Futu Market Data E2E Test Suite.
 * Provides deterministic 2026 trading session fixtures, contract validators,
 * and mathematical specification oracles for R1-R4 requirements.
 */

export const CURRENT_SESSION_TIME = "2026-09-07T09:30:00.000Z";
export const CURRENT_SESSION_TIMESTAMP_SEC = 1788773400; // 2026-09-07 09:30:00 UTC

export const TENCENT_INSTRUMENT = {
  market: "HK",
  code: "00700",
  instrumentId: "HK.00700",
  name: "腾讯控股",
  lotSize: 100,
  pricePrecision: 2,
};

export const ALIBABA_INSTRUMENT = {
  market: "HK",
  code: "09988",
  instrumentId: "HK.09988",
  name: "阿里巴巴－Ｗ",
  lotSize: 100,
  pricePrecision: 2,
};

export const MEITUAN_INSTRUMENT = {
  market: "HK",
  code: "03690",
  instrumentId: "HK.03690",
  name: "美团－Ｗ",
  lotSize: 100,
  pricePrecision: 2,
};

export const APPLE_INSTRUMENT = {
  market: "US",
  code: "AAPL",
  instrumentId: "US.AAPL",
  name: "Apple Inc.",
  lotSize: 1,
  pricePrecision: 2,
};

/**
 * Period durations in seconds for calculation.
 */
export const PERIOD_SECONDS = {
  "1m": 60,
  "3m": 180,
  "5m": 300,
  "10m": 600,
  "15m": 900,
  "30m": 1800,
  "1h": 3600,
  "1d": 86400,
  "1w": 604800,
  "1mo": 2592000,
};

/**
 * Oracle implementation for backward window calculation:
 * lookback = period_duration * limit * 4
 * min lookback = 36h (129600s) for intraday, 45d (3888000s) for daily/weekly/monthly.
 */
export function calculateBackwardWindow(period, limit, sessionEndTime = new Date(CURRENT_SESSION_TIME)) {
  const periodSec = PERIOD_SECONDS[period] || 60;
  const rawLookbackSec = periodSec * limit * 4;
  const isIntraday = !["1d", "1w", "1mo"].includes(period);
  const minLookbackSec = isIntraday ? 36 * 3600 : 45 * 86400;
  const finalLookbackSec = Math.max(rawLookbackSec, minLookbackSec);

  const endTime = new Date(sessionEndTime);
  const beginTime = new Date(endTime.getTime() - finalLookbackSec * 1000);
  return {
    beginTime,
    endTime,
    lookbackSeconds: finalLookbackSec,
  };
}

/**
 * Generate synthetic 2026 trading candles up to current session.
 */
export function generate2026Candles(options = {}) {
  const {
    count = 100,
    period = "1m",
    endTimestamp = CURRENT_SESSION_TIMESTAMP_SEC,
    basePrice = 385.0,
  } = options;

  const intervalSec = PERIOD_SECONDS[period] || 60;
  const candles = [];
  let currentClose = basePrice;

  for (let i = count - 1; i >= 0; i--) {
    const barEnd = endTimestamp - i * intervalSec;
    const barStart = barEnd - intervalSec; // normalized 'at' timestamp
    const open = Number((currentClose + (Math.sin(i) * 0.4)).toFixed(2));
    const high = Number((Math.max(open, currentClose) + Math.abs(Math.cos(i) * 0.6)).toFixed(2));
    const low = Number((Math.min(open, currentClose) - Math.abs(Math.sin(i) * 0.5)).toFixed(2));
    const close = Number((open + (Math.cos(i) * 0.3)).toFixed(2));
    const volume = Math.floor(10000 + Math.abs(Math.sin(i) * 50000));
    const turnover = Number((volume * close).toFixed(2));

    candles.push({
      time: new Date(barStart * 1000).toISOString(),
      at: new Date(barStart * 1000).toISOString(),
      open,
      high,
      low,
      close,
      volume,
      turnover,
      interval: period,
      isRealtime: i === 0, // last bar can be active
    });
    currentClose = close;
  }
  return candles;
}

/**
 * Generate multi-level orderbook depth fixture.
 */
export function generateDepthBook(options = {}) {
  const {
    instrumentId = "HK.00700",
    levels = 10,
    midPrice = 385.4,
    spread = 0.2,
  } = options;

  const bids = [];
  const asks = [];

  for (let i = 0; i < levels; i++) {
    const bidPrice = Number((midPrice - spread / 2 - i * 0.2).toFixed(2));
    const askPrice = Number((midPrice + spread / 2 + i * 0.2).toFixed(2));
    const bidSize = (i + 1) * 2000;
    const askSize = (i + 1) * 1800;

    bids.push({
      price: bidPrice,
      volume: bidSize,
      orders: i + 2,
    });
    asks.push({
      price: askPrice,
      volume: askSize,
      orders: i + 3,
    });
  }

  return {
    type: "market.depth",
    brokerId: "futu",
    instrumentId,
    meta: { instrumentId },
    request: { num: levels },
    depth: { bids, asks },
    at: CURRENT_SESSION_TIME,
  };
}

/**
 * Complete list of expected Futu microstructure & production features for Parity Check (R4).
 */
export const EXPECTED_FUTU_CAPABILITIES = [
  "market.search",
  "market.instrument_profile",
  "market.snapshot",
  "market.snapshots",
  "market.candles",
  "market.intraday",
  "market.ticks",
  "market.depth",
  "market.broker_queue",
  "market.capital_flow",
  "derivatives.warrants",
  "derivatives.futures",
  "derivatives.option_chain",
  "derivatives.option_screen",
  "derivatives.option_analysis",
  "derivatives.option_events",
  "research.instrument",
  "research.financials",
  "research.valuation",
  "research.analyst",
  "research.ownership",
  "research.corporate_actions",
  "research.short_interest",
  "research.news",
  "research.screen",
  "research.calendar",
  "research.macro",
  "research.rankings",
  "research.institutions",
  "research.industry",
  "research.technical_indicators",
];
