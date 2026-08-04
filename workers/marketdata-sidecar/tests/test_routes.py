from __future__ import annotations

import math
from datetime import datetime, timedelta
from typing import Any
from zoneinfo import ZoneInfo

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import upstream
from marketdata_sidecar.errors import SidecarError
from marketdata_sidecar.routes.common import (
    from_yahoo_symbol,
    normalize_instrument,
    quote_matches_instrument,
    quote_is_supported,
    to_yahoo_symbol,
)


@pytest.fixture
def supported_us_instrument(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "AAPL",
            "exchange": "NMS",
            "quoteType": "EQUITY",
        },
    )


@pytest.mark.parametrize(
    ("market", "symbol", "instrument_id", "yahoo_symbol"),
    [
        ("US", "AAPL", "US.AAPL", "AAPL"),
        ("US", "US.AAPL", "US.AAPL", "AAPL"),
        ("HK", "0700", "HK.00700", "0700.HK"),
        ("HK", "HK.0700", "HK.00700", "0700.HK"),
        ("HK", "9988.HK", "HK.09988", "9988.HK"),
        ("SH", "600519", "SH.600519", "600519.SS"),
        ("SZ", "000001", "SZ.000001", "000001.SZ"),
        ("CN", "SH.600519", "SH.600519", "600519.SS"),
    ],
)
def test_market_symbol_normalization_maps_to_yahoo_tickers(
    market: str,
    symbol: str,
    instrument_id: str,
    yahoo_symbol: str,
) -> None:
    instrument = normalize_instrument(market, symbol)
    assert instrument.instrument_id == instrument_id
    assert instrument.yahoo_symbol == yahoo_symbol
    assert to_yahoo_symbol(instrument.market, instrument.symbol) == yahoo_symbol


@pytest.mark.parametrize(
    ("market", "symbol"),
    [("CN", "600519"), ("SH", "519"), ("SZ", "00001"), ("HK", "ABC")],
)
def test_market_symbol_normalization_rejects_ambiguous_or_malformed_codes(
    market: str,
    symbol: str,
) -> None:
    with pytest.raises(SidecarError):
        normalize_instrument(market, symbol)


def test_yahoo_search_symbol_normalization_preserves_provider_markets() -> None:
    assert from_yahoo_symbol("HK", "0700.HK") == ("00700", "HK")
    assert from_yahoo_symbol("SH", "600519.SS") == ("600519", "SH")
    assert from_yahoo_symbol("SZ", "000001.SZ") == ("000001", "SZ")
    assert from_yahoo_symbol("US", "BRK.B") == ("BRK.B", "US")


def test_direct_quote_metadata_must_match_requested_yahoo_ticker() -> None:
    instrument = normalize_instrument("HK", "0700")
    assert quote_matches_instrument({"symbol": "0700.HK"}, instrument)
    assert quote_matches_instrument({"symbol": "00700.HK"}, instrument)
    assert not quote_matches_instrument({"symbol": "9988.HK"}, instrument)
    assert quote_matches_instrument({}, instrument)
    us_share_class = normalize_instrument("US", "BRK.B")
    assert quote_matches_instrument({"symbol": "BRK-B"}, us_share_class)


@pytest.mark.asyncio
async def test_health_and_markets_do_not_need_upstream(
    client: httpx.AsyncClient,
) -> None:
    health = await client.get("/health")
    markets = await client.get("/markets")

    assert health.status_code == 200
    assert health.json()["ok"] is True
    assert health.json()["yfinance_version"] == "0.2.61"
    assert health.json()["runtime_state"] == "ready"
    assert health.json()["warmup_error"] is None
    assert markets.status_code == 200
    body = markets.json()
    assert [item["code"] for item in body["markets"]] == ["US", "HK", "SH", "SZ"]
    assert body["markets"][0]["aliases"] == ["USA", "NYSE", "NASDAQ", "AMEX"]
    assert body["markets"][1]["timezone"] == "Asia/Hong_Kong"
    assert body["markets"][1]["supports_extended_hours"] is False
    assert [item["resolved_market"] for item in body["markets"]] == [
        "US",
        "HK",
        "CN",
        "CN",
    ]
    assert body["markets"][2]["regular_sessions"] == [
        {"start_minute": 570, "end_minute": 690, "label": "09:30-11:30"},
        {"start_minute": 780, "end_minute": 900, "label": "13:00-15:00"},
    ]


@pytest.mark.asyncio
async def test_search_normalizes_supported_us_results(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, int]] = []

    def fake_search(query: str, limit: int) -> list[dict[str, Any]]:
        calls.append((query, limit))
        return [
            {
                "symbol": "AAPL",
                "longname": "Apple Inc.",
                "quoteType": "EQUITY",
                "exchange": "NMS",
            },
            {
                "symbol": "0700.HK",
                "shortname": "Tencent",
                "quoteType": "EQUITY",
                "exchange": "HKG",
            },
        ]

    monkeypatch.setattr(upstream, "search_quotes", fake_search)

    response = await client.get("/search", params={"q": " apple ", "limit": 5})

    assert response.status_code == 200
    assert calls == [("apple", 5)]
    assert response.json() == {
        "entries": [
            {
                "market": "US",
                "resolved_market": "US",
                "instrument_id": "US.AAPL",
                "code": "AAPL",
                "symbol": "AAPL",
                "name": "Apple Inc.",
                "security_type": "EQUITY",
                "exchange": "NASDAQ",
                "selectable": True,
                "source": "yfinance",
                "supported_periods": [
                    "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"
                ],
            },
            {
                "market": "HK",
                "resolved_market": "HK",
                "instrument_id": "HK.00700",
                "code": "00700",
                "symbol": "00700",
                "name": "Tencent",
                "security_type": "EQUITY",
                "exchange": "HKEX",
                "selectable": True,
                "source": "yfinance",
                "supported_periods": [
                    "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"
                ],
            },
        ]
    }


@pytest.mark.asyncio
async def test_search_empty_is_success_but_upstream_failure_is_502(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "search_quotes", lambda _query, _limit: [])
    empty = await client.get("/search", params={"q": "missing"})
    assert empty.status_code == 200
    assert empty.json() == {"entries": []}

    def fail(_query: str, _limit: int) -> list[dict[str, Any]]:
        raise RuntimeError("provider internals must not leak")

    monkeypatch.setattr(upstream, "search_quotes", fail)
    failed = await client.get("/search", params={"q": "apple"})
    assert failed.status_code == 502
    assert failed.json() == {
        "error": {
            "code": "upstream_error",
            "message": "Yahoo Finance search failed",
        }
    }


@pytest.mark.asyncio
async def test_search_uses_exact_market_code_without_text_search(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[str] = []

    def fail_search(_query: str, _limit: int) -> list[dict[str, Any]]:
        raise AssertionError("qualified codes must not use Yahoo text search")

    def fake_info(symbol: str, **_kwargs: Any) -> dict[str, Any]:
        calls.append(symbol)
        return {
            "symbol": symbol,
            "longName": "Kweichow Moutai",
            "exchange": "SHC",
            "quoteType": "EQUITY",
        }

    monkeypatch.setattr(upstream, "search_quotes", fail_search)
    monkeypatch.setattr(upstream, "ticker_info", fake_info)

    response = await client.get("/search", params={"q": "CN.SH.600519"})

    assert response.status_code == 200
    assert calls == ["600519.SS"]
    assert response.json()["entries"] == [
        {
            "market": "SH",
            "resolved_market": "SH",
            "instrument_id": "SH.600519",
            "code": "600519",
            "symbol": "600519",
            "name": "Kweichow Moutai",
            "security_type": "EQUITY",
            "exchange": "SSE",
            "selectable": True,
            "source": "yfinance",
            "supported_periods": [
                "1m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"
            ],
        }
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("query", "expected_yahoo_symbol", "exchange", "instrument_id"),
    [
        ("HKEX.0700", "0700.HK", "HKG", "HK.00700"),
        ("0700.HK", "0700.HK", "HKG", "HK.00700"),
        ("00700.HK", "0700.HK", "HKG", "HK.00700"),
        ("SHH.600519", "600519.SS", "SHC", "SH.600519"),
        ("600519.SS", "600519.SS", "SHC", "SH.600519"),
        ("SHZ.000001", "000001.SZ", "SHZ", "SZ.000001"),
        ("000001.SZ", "000001.SZ", "SHZ", "SZ.000001"),
    ],
)
async def test_search_uses_exact_numeric_yahoo_suffix_codes(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    query: str,
    expected_yahoo_symbol: str,
    exchange: str,
    instrument_id: str,
) -> None:
    calls: list[str] = []

    def fail_search(_query: str, _limit: int) -> list[dict[str, Any]]:
        raise AssertionError("numeric Yahoo codes must not use text search")

    def fake_info(symbol: str, **_kwargs: Any) -> dict[str, Any]:
        calls.append(symbol)
        return {
            "symbol": expected_yahoo_symbol,
            "longName": "Exact security",
            "exchange": exchange,
            "quoteType": "EQUITY",
        }

    monkeypatch.setattr(upstream, "search_quotes", fail_search)
    monkeypatch.setattr(upstream, "ticker_info", fake_info)

    response = await client.get("/search", params={"q": query})

    assert response.status_code == 200
    assert calls == [expected_yahoo_symbol]
    assert response.json()["entries"][0]["instrument_id"] == instrument_id


@pytest.mark.asyncio
async def test_request_validation_uses_stable_error_envelope(
    client: httpx.AsyncClient,
) -> None:
    response = await client.get("/search", params={"q": "a", "limit": 0})

    assert response.status_code == 400
    assert response.json() == {
        "error": {
            "code": "invalid_request",
            "message": "request validation failed",
        }
    }


@pytest.mark.asyncio
async def test_security_returns_fundamentals_and_json_safe_nulls(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "AAPL",
            "longName": "Apple Inc.",
            "exchange": "NMS",
            "currency": "USD",
            "exchangeTimezoneName": "America/New_York",
            "quoteType": "EQUITY",
            "industry": "Consumer Electronics",
            "sector": "Technology",
            "website": "https://www.apple.com",
            "longBusinessSummary": "Makes devices.",
            "marketCap": 3_100_000_000_000,
            "trailingPE": 31.25,
            "forwardPE": math.nan,
            "trailingEps": 7.0,
            "forwardEps": math.inf,
            "dividendRate": 1.0,
            "dividendYield": 0.004,
            "fiftyTwoWeekHigh": 240.0,
            "fiftyTwoWeekLow": 165.0,
            "averageVolume": 50_000_000,
            "sharesOutstanding": 15_000_000_000,
        },
    )

    response = await client.get("/security/nasdaq/aapl")

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "US"
    assert body["instrument_id"] == "US.AAPL"
    assert body["exchange"] == "NASDAQ"
    assert body["market_cap"] == 3_100_000_000_000
    assert body["forward_pe"] is None
    assert body["forward_eps"] is None
    assert "NaN" not in response.text
    assert "Infinity" not in response.text


@pytest.mark.asyncio
async def test_security_not_found_and_unsupported_market_are_distinct(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(upstream, "ticker_info", lambda _symbol, **_kw: {})

    missing = await client.get("/security/US/MISSING")
    unsupported = await client.get("/security/CN/00700")

    assert missing.status_code == 404
    assert missing.json()["error"]["code"] == "security_not_found"
    assert unsupported.status_code == 400
    assert unsupported.json()["error"]["code"] == "invalid_symbol"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    ["/security/US/AAPL", "/snapshot/US/AAPL", "/candles/US/AAPL"],
)
async def test_instrument_metadata_upstream_errors_do_not_leak_details(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    def fail(_symbol: str, **_kw: Any) -> dict[str, Any]:
        raise RuntimeError("private provider failure")

    monkeypatch.setattr(upstream, "ticker_info", fail)

    response = await client.get(path)

    assert response.status_code == 502
    assert response.json()["error"]["code"] == "upstream_error"
    assert "private provider failure" not in response.text


@pytest.mark.asyncio
async def test_snapshot_returns_regular_baseline_and_preserves_quote_blocks(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "AAPL",
            "exchange": "NMS",
            "quoteType": "EQUITY",
            "currency": "USD",
            "marketState": "POST",
            "postMarketPrice": 211.5,
            "postMarketTime": 1_753_812_600,
            "regularMarketPrice": 210.0,
            "regularMarketTime": 1_753_812_000,
            "bid": math.nan,
            "ask": 211.7,
            "regularMarketOpen": 205.0,
            "regularMarketDayHigh": 212.0,
            "regularMarketDayLow": 204.0,
            "regularMarketPreviousClose": 206.0,
            "regularMarketVolume": 42_000_000,
        },
    )

    response = await client.get("/snapshot/US/aapl")

    assert response.status_code == 200
    body = response.json()
    assert body["price"] == 210.0
    assert body["bid"] is None
    assert body["ask"] == 211.7
    assert body["volume"] == 42_000_000
    assert "session" not in body
    assert "extended_hours" not in body
    assert body["delayed"] is True
    assert body["delay_minutes"] == 15
    assert body["regular_quote"]["price"] == 210.0
    assert body["regular_quote"]["quote_at"].endswith("Z")
    assert body["after_market_quote"]["price"] == 211.5
    assert body["after_market_quote"]["quote_at"].endswith("Z")
    assert body["quote_at"].endswith("Z")
    assert body["observed_at"].endswith("Z")


@pytest.mark.asyncio
async def test_non_us_snapshot_keeps_regular_price_when_yahoo_uses_post_state(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "0700.HK",
            "exchange": "HKG",
            "quoteType": "EQUITY",
            "marketState": "POST",
            "regularMarketPrice": 410.0,
            "regularMarketTime": 1_753_812_000,
            "postMarketPrice": 411.0,
            "postMarketTime": 1_753_812_600,
        },
    )

    response = await client.get("/snapshot/HK/0700")

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "HK"
    assert body["symbol"] == "00700"
    assert body["price"] == 410.0
    assert "session" not in body
    assert "extended_hours" not in body
    assert body["after_market_quote"]["price"] == 411.0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("path", "error_code"),
    [
        ("/security/HK/0700", "security_not_found"),
        ("/snapshot/HK/0700", "snapshot_not_found"),
        ("/candles/HK/0700", "candles_not_found"),
    ],
)
async def test_direct_routes_reject_mismatched_yahoo_ticker_metadata(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
    error_code: str,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "9988.HK",
            "exchange": "HKG",
            "quoteType": "EQUITY",
            "regularMarketPrice": 100.0,
        },
    )

    response = await client.get(path)

    assert response.status_code == 404
    assert response.json()["error"]["code"] == error_code


@pytest.mark.asyncio
async def test_snapshot_without_finite_price_is_404(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda _symbol, **_kw: {
            "symbol": "AAPL",
            "exchange": "NMS",
            "quoteType": "EQUITY",
            "regularMarketPrice": math.nan,
        },
    )

    response = await client.get("/snapshot/US/AAPL")

    assert response.status_code == 404
    assert response.json()["error"]["code"] == "snapshot_not_found"


@pytest.mark.parametrize(
    ("exchange", "quote_type"),
    [
        ("NMS", "EQUITY"),
        ("NYSE", "ETF"),
        ("AMEX", "MUTUALFUND"),
        ("BTS", "INDEX"),
    ],
)
def test_supported_us_security_types_are_selectable(
    exchange: str,
    quote_type: str,
) -> None:
    assert quote_is_supported(
        {"exchange": exchange, "quoteType": quote_type}
    )


@pytest.mark.parametrize(
    "quote_type",
    ["CRYPTOCURRENCY", "CURRENCY", "FUTURE"],
)
def test_non_security_quote_types_are_not_selectable(quote_type: str) -> None:
    assert not quote_is_supported(
        {"exchange": "NMS", "quoteType": quote_type}
    )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("path_template", "error_code"),
    [
        ("/security/US/{symbol}", "security_not_found"),
        ("/snapshot/US/{symbol}", "snapshot_not_found"),
        ("/candles/US/{symbol}", "candles_not_found"),
    ],
)
@pytest.mark.parametrize(
    ("symbol", "info"),
    [
        pytest.param(
            "0700.HK",
            {
                "symbol": "0700.HK",
                "exchange": "HKG",
                "quoteType": "EQUITY",
                "regularMarketPrice": 100.0,
            },
            id="hong-kong-equity",
        ),
        pytest.param(
            "600519.SS",
            {
                "symbol": "600519.SS",
                "exchange": "SHH",
                "quoteType": "EQUITY",
                "regularMarketPrice": 100.0,
            },
            id="shanghai-equity",
        ),
        pytest.param(
            "BTC-USD",
            {
                "symbol": "BTC-USD",
                "exchange": "NMS",
                "quoteType": "CRYPTOCURRENCY",
                "regularMarketPrice": 100.0,
            },
            id="cryptocurrency",
        ),
    ],
)
async def test_direct_routes_reject_non_us_or_non_security_instruments(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path_template: str,
    error_code: str,
    symbol: str,
    info: dict[str, Any],
) -> None:
    monkeypatch.setattr(upstream, "ticker_info", lambda _symbol, **_kw: dict(info))

    def history_must_not_run(*_args: Any, **_kwargs: Any) -> pd.DataFrame:
        raise AssertionError("unsupported instrument must not fetch history")

    monkeypatch.setattr(upstream, "ticker_history", history_must_not_run)

    response = await client.get(path_template.format(symbol=symbol))

    assert response.status_code == 404
    assert response.json()["error"]["code"] == error_code


@pytest.mark.asyncio
async def test_candles_map_period_include_extended_hours_and_apply_limit(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    supported_us_instrument: None,
) -> None:
    calls: list[dict[str, Any]] = []
    frame = pd.DataFrame(
        {
            "Open": [10.0, 11.0, 12.0],
            "High": [11.0, 12.0, 13.0],
            "Low": [9.0, 10.0, 11.0],
            "Close": [10.5, 11.5, 12.5],
            "Volume": [100, 200, 300],
        },
        index=pd.DatetimeIndex(
            [
                "2026-07-28T13:30:00Z",
                "2026-07-28T13:35:00Z",
                "2026-07-28T13:40:00Z",
            ]
        ),
    )

    def fake_history(symbol: str, **kwargs: Any) -> pd.DataFrame:
        calls.append({"symbol": symbol, **kwargs})
        return frame

    monkeypatch.setattr(upstream, "ticker_history", fake_history)

    response = await client.get(
        "/candles/NASDAQ/aapl",
        params={"period": "5m", "limit": 2},
    )

    assert response.status_code == 200
    assert calls == [
        {
            "symbol": "AAPL",
            "interval": "5m",
            "fetch_period": "60d",
            "start": None,
            "end": None,
            "prepost": True,
        }
    ]
    body = response.json()
    assert body["market"] == "US"
    assert body["instrument_id"] == "US.AAPL"
    assert body["period"] == "5m"
    assert body["extended_hours"] is True
    assert body["total_returned"] == 2
    assert [item["at"] for item in body["candles"]] == [
        "2026-07-28T13:35:00Z",
        "2026-07-28T13:40:00Z",
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("path", "yahoo_symbol", "exchange", "expected_market", "expected_symbol"),
    [
        ("/candles/HK/0700", "0700.HK", "HKG", "HK", "00700"),
        ("/candles/SH/600519", "600519.SS", "SHC", "SH", "600519"),
        ("/candles/SZ/000001", "000001.SZ", "SHZ", "SZ", "000001"),
    ],
)
async def test_candles_route_maps_cn_markets_and_disables_yahoo_extended_hours(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
    yahoo_symbol: str,
    exchange: str,
    expected_market: str,
    expected_symbol: str,
) -> None:
    calls: list[dict[str, Any]] = []
    monkeypatch.setattr(
        upstream,
        "ticker_info",
        lambda symbol, **_kwargs: {
            "symbol": symbol,
            "exchange": exchange,
            "quoteType": "EQUITY",
            "regularMarketPrice": 10.0,
        },
    )
    monkeypatch.setattr(
        upstream,
        "ticker_history",
        lambda symbol, **kwargs: calls.append({"symbol": symbol, **kwargs})
        or pd.DataFrame(
            {
                "Open": [10.0],
                "High": [11.0],
                "Low": [9.0],
                "Close": [10.5],
                "Volume": [100],
            },
            index=pd.DatetimeIndex(["2026-07-28T01:30:00Z"]),
        ),
    )

    response = await client.get(path, params={"period": "5m"})

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == expected_market
    assert body["symbol"] == expected_symbol
    assert body["instrument_id"] == f"{expected_market}.{expected_symbol}"
    assert body["extended_hours"] is False
    assert calls[0]["symbol"] == yahoo_symbol
    assert calls[0]["prepost"] is False


@pytest.mark.asyncio
async def test_candle_bounds_are_utc_and_forwarded_to_yfinance(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    supported_us_instrument: None,
) -> None:
    calls: list[dict[str, Any]] = []
    request_day = datetime.now(ZoneInfo("America/New_York")).date()
    local_from = datetime.combine(
        request_day,
        datetime.min.time().replace(hour=9, minute=30),
        tzinfo=ZoneInfo("America/New_York"),
    )
    local_to = local_from + timedelta(minutes=1)
    expected_from = local_from.astimezone(ZoneInfo("UTC"))
    expected_to = local_to.astimezone(ZoneInfo("UTC"))
    frame = pd.DataFrame(
        {
            "Open": [10.0, 10.5],
            "High": [11.0, 11.5],
            "Low": [9.0, 9.5],
            "Close": [10.5, 11.0],
            "Volume": [100, 110],
        },
        index=pd.DatetimeIndex(
            [expected_from.isoformat(), expected_to.isoformat()]
        ),
    )

    def fake_history(symbol: str, **kwargs: Any) -> pd.DataFrame:
        calls.append({"symbol": symbol, **kwargs})
        return frame

    monkeypatch.setattr(upstream, "ticker_history", fake_history)
    response = await client.get(
        "/candles/US/AAPL",
        params={
            "period": "1m",
            "from": local_from.isoformat(),
            "to": local_to.isoformat(),
        },
    )

    assert response.status_code == 200
    assert calls[0]["start"] == expected_from
    assert calls[0]["end"] == expected_from + timedelta(minutes=2)
    assert [item["at"] for item in response.json()["candles"]] == [
        expected_from.isoformat().replace("+00:00", "Z"),
        expected_to.isoformat().replace("+00:00", "Z"),
    ]


@pytest.mark.asyncio
async def test_to_only_candle_pages_are_older_and_non_overlapping(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    supported_us_instrument: None,
) -> None:
    calls: list[dict[str, Any]] = []
    frame = pd.DataFrame(
        {
            "Open": [10.0, 11.0, 12.0, 13.0, 14.0],
            "High": [11.0, 12.0, 13.0, 14.0, 15.0],
            "Low": [9.0, 10.0, 11.0, 12.0, 13.0],
            "Close": [10.5, 11.5, 12.5, 13.5, 14.5],
            "Volume": [100, 110, 120, 130, 140],
        },
        index=pd.DatetimeIndex(
            [
                "2026-07-28T13:10:00Z",
                "2026-07-28T13:15:00Z",
                "2026-07-28T13:20:00Z",
                "2026-07-28T13:25:00Z",
                "2026-07-28T13:30:00Z",
            ]
        ),
    )

    def fake_history(symbol: str, **kwargs: Any) -> pd.DataFrame:
        calls.append({"symbol": symbol, **kwargs})
        return frame.loc[
            (frame.index >= kwargs["start"]) & (frame.index < kwargs["end"])
        ]

    monkeypatch.setattr(upstream, "ticker_history", fake_history)

    first = await client.get(
        "/candles/US/AAPL",
        params={
            "period": "5m",
            "limit": 2,
            "to": "2026-07-28T13:29:59.999999Z",
        },
    )
    second = await client.get(
        "/candles/US/AAPL",
        params={
            "period": "5m",
            "limit": 2,
            "to": "2026-07-28T13:19:59.999999Z",
        },
    )

    assert first.status_code == 200
    assert second.status_code == 200
    first_times = [item["at"] for item in first.json()["candles"]]
    second_times = [item["at"] for item in second.json()["candles"]]
    assert first_times == ["2026-07-28T13:20:00Z", "2026-07-28T13:25:00Z"]
    assert second_times == ["2026-07-28T13:10:00Z", "2026-07-28T13:15:00Z"]
    assert set(first_times).isdisjoint(second_times)
    assert max(second_times) < min(first_times)
    assert all(call["start"] is not None for call in calls)
    assert calls[0]["end"].isoformat() == "2026-07-28T13:34:59.999999+00:00"
    assert calls[1]["end"].isoformat() == "2026-07-28T13:24:59.999999+00:00"


@pytest.mark.asyncio
async def test_weekly_candles_fetch_max_history_for_1000_bar_limit(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    supported_us_instrument: None,
) -> None:
    calls: list[dict[str, Any]] = []
    frame = pd.DataFrame(
        {
            "Open": [10.0],
            "High": [11.0],
            "Low": [9.0],
            "Close": [10.5],
            "Volume": [100],
        },
        index=pd.DatetimeIndex(["2026-07-24T00:00:00Z"]),
    )

    def fake_history(symbol: str, **kwargs: Any) -> pd.DataFrame:
        calls.append({"symbol": symbol, **kwargs})
        return frame

    monkeypatch.setattr(upstream, "ticker_history", fake_history)

    response = await client.get(
        "/candles/US/AAPL",
        params={"period": "1w", "limit": 1000},
    )

    assert response.status_code == 200
    assert calls == [
        {
            "symbol": "AAPL",
            "interval": "1wk",
            "fetch_period": "max",
            "start": None,
            "end": None,
            "prepost": True,
        }
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("params", "code"),
    [
        ({"period": "2m"}, "unsupported_period"),
        (
            {
                "from": "2026-07-28T13:31:00Z",
                "to": "2026-07-28T13:30:00Z",
            },
            "invalid_time_range",
        ),
        ({"from": "2026-07-28 13:30:00"}, "invalid_time"),
    ],
)
async def test_invalid_candle_queries_never_call_upstream(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    params: dict[str, str],
    code: str,
) -> None:
    def must_not_call(*_args: Any, **_kwargs: Any) -> None:
        raise AssertionError("upstream must not be called")

    monkeypatch.setattr(upstream, "ticker_history", must_not_call)

    response = await client.get("/candles/US/AAPL", params=params)

    assert response.status_code == 400
    assert response.json()["error"]["code"] == code


@pytest.mark.asyncio
async def test_candles_empty_and_upstream_failure_are_distinct(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    supported_us_instrument: None,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_history",
        lambda *_args, **_kwargs: pd.DataFrame(),
    )
    empty = await client.get("/candles/US/AAPL")
    assert empty.status_code == 404
    assert empty.json()["error"]["code"] == "candles_not_found"

    def fail(*_args: Any, **_kwargs: Any) -> pd.DataFrame:
        raise RuntimeError("private yfinance failure")

    monkeypatch.setattr(upstream, "ticker_history", fail)
    failed = await client.get("/candles/US/AAPL")
    assert failed.status_code == 502
    assert failed.json()["error"]["code"] == "upstream_error"
    assert "private yfinance failure" not in failed.text
