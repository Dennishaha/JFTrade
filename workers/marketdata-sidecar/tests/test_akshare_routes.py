from __future__ import annotations

from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from datetime import date, datetime, timedelta, timezone
import threading
import time
from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_provider, akshare_upstream


def _empty() -> pd.DataFrame:
    return pd.DataFrame()


def _cn_stock_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "最新价": "1425.50",
                "今开": "1410.00",
                "最高": "1430.00",
                "最低": "1400.00",
                "昨收": "1405.00",
                "成交量": "12.5",
                "成交额": "17819.25",
                "更新时间": "2026-08-03 15:00:00",
            }
        ]
    )


def _etf_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "510300",
                "名称": "沪深300ETF",
                "最新价": "4.1230",
                "今开": "4.10",
                "最高": "4.20",
                "最低": "4.01",
                "昨收": "4.08",
                "成交量": "100.25",
                "成交额": "413.33",
            },
            {"代码": "159919", "名称": "沪深300ETF深", "最新价": "4.01"},
        ]
    )


def _standard_catalog_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
    if function_name == "stock_sh_a_spot_em":
        return _cn_stock_frame()
    if function_name == "stock_sz_a_spot_em":
        return _empty()
    if function_name == "fund_etf_spot_em":
        return _etf_frame()
    if function_name == "stock_zh_index_spot_em":
        if kwargs["symbol"] == "上证系列指数":
            return pd.DataFrame([{"代码": "000001", "名称": "上证指数", "最新价": "3300"}])
        if kwargs["symbol"] == "中证系列指数":
            return pd.DataFrame([{"代码": "000300", "名称": "沪深300", "最新价": "3900"}])
        return _empty()
    if function_name == "stock_hk_spot_em":
        return _empty()
    if function_name == "stock_hk_index_spot_em":
        return pd.DataFrame([{"代码": "HSI", "名称": "恒生指数", "最新价": "24500"}])
    if function_name == "stock_us_spot_em":
        return pd.DataFrame(
            [
                {
                    "代码": "105.AAPL",
                    "名称": "Apple Inc.",
                    "最新价": "210.125",
                    "成交量": "1234567",
                }
            ]
        )
    if function_name == "index_global_spot_em":
        return pd.DataFrame(
            [
                {"代码": "DJIA", "名称": "道琼斯指数", "最新价": "45000"},
                {"代码": "SPX", "名称": "标普500指数", "最新价": "6200"},
                {"代码": "NDX", "名称": "纳斯达克100指数", "最新价": "23000"},
            ]
        )
    raise AssertionError(f"unexpected AKShare call: {function_name} {kwargs}")


@pytest.mark.parametrize(
    ("market", "code", "market_id", "row", "expected"),
    [
        (
            "US",
            "BABA",
            "106",
            {"instrument_kind": 3},
            ("BABA", "BABA", "stock", None),
        ),
        (
            "US",
            "SPX",
            "100",
            {"instrument_kind": 2},
            (".SPX", "标普500", "index", "INDEX"),
        ),
        (
            "HK",
            "09988",
            "116",
            {"instrument_kind": 3},
            ("09988", "09988", "stock", None),
        ),
        (
            "SH",
            "510300",
            "1",
            {"instrument_kind": 3},
            ("510300", "510300", "etf", "ETF"),
        ),
        (
            "SH",
            "000001",
            "1",
            {"instrument_kind": 2},
            ("000001", "000001", "index", "INDEX"),
        ),
    ],
)
def test_live_spot_identity_preserves_market_and_security_kind(
    market: str,
    code: str,
    market_id: str,
    row: dict[str, Any],
    expected: tuple[str, str, str, str | None],
) -> None:
    assert akshare_provider._spot_identity(market, code, market_id, row) == expected


def test_live_search_uses_qualified_symbol_without_full_market_pagination(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    queries: list[str] = []

    def fake_search(query: str) -> list[dict[str, str]]:
        queries.append(query)
        return [
            {
                "Code": "AAPL",
                "Name": "苹果",
                "Classify": "UsStock",
                "MktNum": "105",
            }
        ]

    monkeypatch.setattr(akshare_upstream, "search_rows", fake_search)

    entries = akshare_provider.search("US.AAPL", 5)

    assert queries == ["AAPL"]
    assert [entry.instrument_id for entry in entries] == ["US.AAPL"]


@pytest.mark.asyncio
async def test_process_and_namespaced_health_are_network_free(
    client: httpx.AsyncClient,
) -> None:
    process = await client.get("/healthz")
    provider = await client.get("/providers/akshare/health")

    assert process.status_code == 200
    assert process.json() == {"ok": True, "version": "0.2.0"}
    assert provider.status_code == 200
    assert provider.json()["provider"] == "akshare"
    assert provider.json()["runtime_state"] == "ready"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("state", "code", "message", "retry_after"),
    [
        (
            "warming",
            "AKSHARE_RUNTIME_WARMING",
            "AKShare runtime is warming up",
            "1",
        ),
        (
            "failed",
            "AKSHARE_RUNTIME_FAILED",
            "AKShare runtime failed to initialize",
            None,
        ),
    ],
)
async def test_namespaced_akshare_health_requires_ready_runtime(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    state: str,
    code: str,
    message: str,
    retry_after: str | None,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "runtime_snapshot",
        lambda: akshare_upstream.RuntimeSnapshot(
            state,
            "private import failure",
        ),
    )
    monkeypatch.setattr(
        akshare_upstream,
        "request_runtime_warmup",
        lambda: akshare_upstream.RuntimeSnapshot(state),
    )

    response = await client.get("/providers/akshare/health")

    assert response.status_code == 503
    assert response.json() == {"error": {"code": code, "message": message}}
    assert response.headers.get("Retry-After") == retry_after
    assert "private import failure" not in response.text


@pytest.mark.asyncio
async def test_namespaced_yfinance_routes_preserve_legacy_behavior(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from marketdata_sidecar import upstream

    monkeypatch.setattr(
        upstream,
        "search_quotes",
        lambda _query, _limit: [
            {
                "symbol": "AAPL",
                "longname": "Apple Inc.",
                "quoteType": "EQUITY",
                "exchange": "NMS",
            }
        ],
    )

    legacy = await client.get("/search", params={"q": "Apple"})
    namespaced = await client.get(
        "/providers/yfinance/search",
        params={"q": "Apple"},
    )

    assert namespaced.status_code == 200
    assert namespaced.json() == legacy.json()
    assert namespaced.json()["entries"][0]["supported_periods"][-1] == "1mo"


@pytest.mark.asyncio
async def test_akshare_search_security_and_decimal_snapshot(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _standard_catalog_call)

    searched = await client.get(
        "/providers/akshare/search",
        params={"q": "SH.510300"},
    )
    security = await client.get("/providers/akshare/security/SH/510300")
    snapshot = await client.get("/providers/akshare/snapshot/SH/510300")

    assert searched.status_code == 200
    entry = searched.json()["entries"][0]
    assert entry["instrument_id"] == "SH.510300"
    assert entry["security_type"] == "ETF"
    assert entry["supported_periods"] == list(akshare_provider.ALL_PERIODS)
    assert security.status_code == 200
    assert security.json()["timezone"] == "Asia/Shanghai"
    assert snapshot.status_code == 200
    body = snapshot.json()
    assert body["price"] == "4.123"
    assert body["volume"] == "10025"
    assert body["turnover"] == "413.33"
    assert body["bid"] is None
    assert body["ask"] is None
    assert body["quote_at"] is None
    assert body["source"] == "akshare:eastmoney"


@pytest.mark.asyncio
async def test_akshare_us_identity_and_core_index_mappings(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "call", _standard_catalog_call)

    stock = await client.get("/providers/akshare/security/US/AAPL")
    index = await client.get(
        "/providers/akshare/search",
        params={"q": "US..DJI"},
    )
    hsi = await client.get("/providers/akshare/security/HK/800000")

    assert stock.status_code == 200
    assert stock.json()["instrument_id"] == "US.AAPL"
    assert stock.json()["security_type"] is None
    assert index.status_code == 200
    assert index.json()["entries"][0]["instrument_id"] == "US..DJI"
    assert index.json()["entries"][0]["supported_periods"] == ["1d", "1w", "1mo"]
    assert hsi.status_code == 200
    assert hsi.json()["instrument_id"] == "HK.800000"


@pytest.mark.asyncio
async def test_batch_prefetches_each_market_once_and_reports_invalid_ids(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: Counter[str] = Counter()

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls[function_name] += 1
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.post(
        "/providers/akshare/snapshots",
        json={
            "instrument_ids": [
                "US.AAPL",
                "US..SPX",
                "US.MISSING",
                "malformed",
            ]
        },
    )

    assert response.status_code == 200
    body = response.json()
    assert [item["instrument_id"] for item in body["entries"]] == ["US.AAPL", "US..SPX"]
    assert [item["instrument_id"] for item in body["errors"]] == ["malformed", "US.MISSING"]
    assert calls["stock_us_spot_em"] == 1
    assert calls["index_global_spot_em"] == 1


@pytest.mark.asyncio
async def test_duplicate_cn_index_series_is_explicitly_ambiguous(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_sh_a_spot_em":
            return _empty()
        if function_name == "fund_etf_spot_em":
            return _empty()
        if function_name == "stock_zh_index_spot_em":
            return pd.DataFrame(
                [{"代码": "000300", "名称": kwargs["symbol"], "最新价": "3900"}]
            )
        raise AssertionError(function_name)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    catalog = akshare_provider.catalog("SH")
    keys = {
        item.upstream_symbol
        for item in catalog
        if item.instrument_id == "SH.000300"
    }
    response = await client.get("/providers/akshare/security/SH/000300")

    assert keys == {"sh:000300", "csi:000300"}
    assert response.status_code == 400
    assert response.json()["error"]["code"] == "ambiguous_instrument"


@pytest.mark.asyncio
async def test_cn_minute_candles_are_utc_validated_and_hand_scaled(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[dict[str, Any]] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_zh_a_hist_min_em":
            calls.append(kwargs)
            return pd.DataFrame(
                [
                    {
                        "时间": "2026-08-03 09:30:00",
                        "开盘": "1410.1",
                        "最高": "1420.2",
                        "最低": "1400.3",
                        "收盘": "1415.4",
                        "成交量": "12.5",
                    },
                    {
                        "时间": "2026-08-03 09:31:00",
                        "开盘": "1415",
                        "最高": "1400",
                        "最低": "1410",
                        "收盘": "1412",
                        "成交量": "9",
                    },
                ]
            )
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get(
        "/providers/akshare/candles/SH/600519",
        params={"period": "1m"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["total_returned"] == 1
    assert body["candles"][0] == {
        "at": "2026-08-03T01:30:00Z",
        "open": "1410.1",
        "high": "1420.2",
        "low": "1400.3",
        "close": "1415.4",
        "volume": "1250",
    }
    assert calls[0]["period"] == "1"
    assert calls[0]["adjust"] == ""


@pytest.mark.asyncio
async def test_sina_daily_history_keeps_share_volume_and_aggregates_periods(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, dict[str, Any]]] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append((function_name, kwargs))
        if function_name == "stock_us_daily":
            assert kwargs == {"symbol": "AAPL", "adjust": ""}
            return pd.DataFrame(
                [
                    {
                        "date": date(2026, 8, 3),
                        "open": "200",
                        "high": "212",
                        "low": "198",
                        "close": "210",
                        "volume": "1000",
                    },
                    {
                        "date": date(2026, 8, 4),
                        "open": "210",
                        "high": "215",
                        "low": "205",
                        "close": "214",
                        "volume": "2500",
                    },
                ]
            )
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "1w", "limit": 1},
    )

    assert response.status_code == 200
    assert response.json()["source"] == "akshare:sina"
    assert response.json()["candles"] == [
        {
            "at": "2026-08-03T04:00:00Z",
            "open": "200",
            "high": "215",
            "low": "198",
            "close": "214",
            "volume": "3500",
        }
    ]
    assert all(name != "stock_us_hist" for name, _kwargs in calls)


@pytest.mark.asyncio
async def test_akshare_candle_cursor_pages_are_strict_and_reach_the_history_boundary(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    today = datetime.now(timezone.utc).date()
    frame = pd.DataFrame(
        [
            {
                "date": today - timedelta(days=days),
                "open": str(200 + index),
                "high": str(201 + index),
                "low": str(199 + index),
                "close": str(200.5 + index),
                "volume": str(1000 + index),
            }
            for index, days in enumerate((5, 4, 3, 2))
        ]
    )

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_us_daily":
            assert kwargs == {"symbol": "AAPL", "adjust": ""}
            return frame
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)

    first = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "1d", "limit": 2},
    )
    assert first.status_code == 200
    first_body = first.json()
    first_times = [item["at"] for item in first_body["candles"]]
    assert first_body["has_more"] is True
    assert first_body["next_before"] == first_times[0]

    second = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "1d", "limit": 2, "before": first_body["next_before"]},
    )
    assert second.status_code == 200
    second_body = second.json()
    second_times = [item["at"] for item in second_body["candles"]]
    assert second_body["has_more"] is False
    assert second_body["next_before"] is None
    assert set(first_times).isdisjoint(second_times)
    assert max(second_times) < min(first_times)

    terminal = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "1d", "limit": 2, "before": second_times[0]},
    )
    assert terminal.status_code == 200
    assert terminal.json()["candles"] == []
    assert terminal.json()["has_more"] is False


@pytest.mark.asyncio
async def test_sina_cn_minutes_are_not_multiplied_as_eastmoney_hands(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_zh_a_minute":
            assert kwargs == {"symbol": "sh600519", "period": "1", "adjust": ""}
            return pd.DataFrame(
                [
                    {
                        "day": "2026-08-04 09:31:00",
                        "open": "1410.1",
                        "high": "1420.2",
                        "low": "1400.3",
                        "close": "1415.4",
                        "volume": "1250",
                    }
                ]
            )
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get(
        "/providers/akshare/candles/SH/600519",
        params={"period": "1m", "limit": 1},
    )

    assert response.status_code == 200
    assert response.json()["source"] == "akshare:sina"
    assert response.json()["candles"][0]["volume"] == "1250"
    assert response.json()["candles"][0]["at"] == "2026-08-04T01:31:00Z"


@pytest.mark.asyncio
async def test_sina_us_index_symbol_mapping_uses_available_history_identity(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_us_daily":
            assert kwargs == {"symbol": ".INX", "adjust": ""}
            return pd.DataFrame(
                [
                    {
                        "date": date(2026, 8, 3),
                        "open": "7504.7",
                        "high": "7610.0",
                        "low": "7500.0",
                        "close": "7600.5",
                        "volume": "3188",
                    }
                ]
            )
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get(
        "/providers/akshare/candles/US/.SPX",
        params={"period": "1d", "limit": 1},
    )

    assert response.status_code == 200
    assert response.json()["instrument_id"] == "US..SPX"
    assert response.json()["source"] == "akshare:sina"


@pytest.mark.asyncio
async def test_us_hk_minutes_and_hk_index_daily_are_deterministically_aggregated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_us_minutes(symbol: str) -> list[dict[str, str]]:
        assert symbol == "AAPL"
        return [
            {
                "时间": f"2026-08-03 09:{minute:02d}:00",
                "开盘": str(100 + minute),
                "最高": str(101 + minute),
                "最低": str(99 + minute),
                "收盘": str(100.5 + minute),
                "成交量": "10",
            }
            for minute in range(30, 36)
        ]

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_hk_famous_spot_em":
            return pd.DataFrame(
                [{"代码": "09988", "名称": "阿里巴巴-W", "最新价": "125.2"}]
            )
        if function_name == "stock_hk_index_daily_em":
            assert kwargs == {"symbol": "HSI"}
            return pd.DataFrame(
                [
                    {"date": date(2026, 8, 3), "open": 10, "high": 13, "low": 9, "latest": 12},
                    {"date": date(2026, 8, 4), "open": 12, "high": 15, "low": 11, "latest": 14},
                ]
            )
        return _standard_catalog_call(function_name, **kwargs)

    def fake_hk_minutes(symbol: str) -> list[dict[str, str]]:
        assert symbol == "09988"
        return [
            {
                "时间": f"2026-08-04 09:{minute:02d}:00",
                "开盘": str(120 + minute),
                "最高": str(121 + minute),
                "最低": str(119 + minute),
                "收盘": str(120.5 + minute),
                "成交量": "10",
            }
            for minute in range(30, 36)
        ]

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    monkeypatch.setattr(akshare_upstream, "us_minute_rows", fake_us_minutes)
    monkeypatch.setattr(akshare_upstream, "hk_minute_rows", fake_hk_minutes)
    us = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "5m"},
    )
    hk = await client.get(
        "/providers/akshare/candles/HK/800000",
        params={"period": "1w"},
    )
    hk_stock = await client.get(
        "/providers/akshare/candles/HK/09988",
        params={"period": "5m"},
    )
    unsupported = await client.get(
        "/providers/akshare/candles/HK/800000",
        params={"period": "1m"},
    )

    assert us.status_code == 200
    assert [item["at"] for item in us.json()["candles"]] == [
        "2026-08-03T13:30:00Z",
        "2026-08-03T13:35:00Z",
    ]
    assert us.json()["candles"][0]["volume"] == "50"
    assert hk.status_code == 200
    assert hk.json()["candles"] == [
        {
            "at": "2026-08-02T16:00:00Z",
            "open": "10",
            "high": "15",
            "low": "9",
            "close": "14",
            "volume": None,
        }
    ]
    assert hk_stock.status_code == 200
    assert hk_stock.json()["source"] == "akshare:eastmoney"
    assert [item["at"] for item in hk_stock.json()["candles"]] == [
        "2026-08-04T01:30:00Z",
        "2026-08-04T01:35:00Z",
    ]
    assert hk_stock.json()["candles"][0]["volume"] == "50"
    assert unsupported.status_code == 400
    assert unsupported.json()["error"]["code"] == "unsupported_period"


@pytest.mark.asyncio
async def test_us_derived_intraday_rejects_ranges_older_than_source_window(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    history_calls = 0

    def fake_us_minutes(_symbol: str) -> list[dict[str, str]]:
        nonlocal history_calls
        history_calls += 1
        return []

    monkeypatch.setattr(akshare_upstream, "us_minute_rows", fake_us_minutes)
    response = await client.get(
        "/providers/akshare/candles/US/AAPL",
        params={"period": "5m", "from": "2026-07-01T00:00:00Z"},
    )

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "UNSUPPORTED_RANGE"
    assert history_calls == 0


@pytest.mark.asyncio
async def test_date_only_quote_time_is_not_fabricated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_sh_a_spot_em":
            frame = _cn_stock_frame()
            frame.loc[0, "更新时间"] = "2026-08-03"
            return frame
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/snapshot/SH/600519")

    assert response.status_code == 200
    assert response.json()["quote_at"] is None
    assert response.json()["observed_at"].endswith("Z")


@pytest.mark.asyncio
async def test_inconsistent_snapshot_ohlc_is_a_schema_error(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        if function_name == "stock_sh_a_spot_em":
            frame = _cn_stock_frame()
            frame.loc[0, "最高"] = "1400"
            return frame
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/snapshot/SH/600519")

    assert response.status_code == 502
    assert response.json()["error"]["code"] == "AKSHARE_SCHEMA_ERROR"


def test_full_market_catalog_cache_singleflights_concurrent_requests(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: Counter[str] = Counter()
    entered = threading.Event()
    release = threading.Event()

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls[function_name] += 1
        if function_name == "stock_us_spot_em":
            entered.set()
            assert release.wait(timeout=2)
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    with ThreadPoolExecutor(max_workers=2) as executor:
        futures = [executor.submit(akshare_provider.catalog, "US") for _ in range(2)]
        assert entered.wait(timeout=2)
        release.set()
        results = [future.result(timeout=3) for future in futures]

    assert all(any(item.instrument_id == "US.AAPL" for item in result) for result in results)
    assert calls["stock_us_spot_em"] == 1
    assert calls["index_global_spot_em"] == 1


@pytest.mark.asyncio
async def test_common_us_batch_uses_compact_catalog_before_full_pagination(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: Counter[str] = Counter()

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls[function_name] += 1
        if function_name == "stock_us_famous_spot_em":
            rows = {
                "科技类": [
                    {"代码": "105.AAPL", "名称": "Apple Inc.", "最新价": "210.125"},
                    {"代码": "105.NVDA", "名称": "NVIDIA", "最新价": "180.50"},
                ],
                "媒体类": [{"代码": "105.TME", "名称": "腾讯音乐", "最新价": "9.08"}],
            }
            return pd.DataFrame(rows.get(kwargs["symbol"], []))
        if function_name == "index_global_spot_em":
            return pd.DataFrame(
                [{"代码": "SPX", "名称": "标普500指数", "最新价": "6200"}]
            )
        raise AssertionError(f"unexpected full-directory call: {function_name}")

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.post(
        "/providers/akshare/snapshots",
        json={"instrument_ids": ["US.AAPL", "US.NVDA", "US.TME", "US..SPX"]},
    )

    assert response.status_code == 200
    assert [entry["instrument_id"] for entry in response.json()["entries"]] == [
        "US.AAPL",
        "US.NVDA",
        "US.TME",
        "US..SPX",
    ]
    assert 0 < calls["stock_us_famous_spot_em"] < len(akshare_provider.US_FAMOUS_CATEGORIES)
    assert calls["stock_us_spot_em"] == 0


def test_catalog_failure_is_shared_with_waiters(monkeypatch: pytest.MonkeyPatch) -> None:
    cache = akshare_provider._TTLCache()
    monkeypatch.setattr(akshare_provider, "CATALOG_FAILURE_CACHE_SECONDS", 1)
    calls = 0
    entered = threading.Event()
    release = threading.Event()

    def fail() -> None:
        nonlocal calls
        calls += 1
        entered.set()
        release.wait(timeout=1)
        raise RuntimeError("catalog unavailable")

    with ThreadPoolExecutor(max_workers=2) as executor:
        futures = [executor.submit(cache.get_or_fetch, "US", fail) for _ in range(2)]
        assert entered.wait(timeout=1)
        release.set()
        errors = []
        for future in futures:
            with pytest.raises(RuntimeError, match="catalog unavailable") as caught:
                future.result(timeout=2)
            errors.append(caught.value)

    assert calls == 1
    assert errors[0] is errors[1]


@pytest.mark.asyncio
async def test_akshare_failure_is_explicit_and_does_not_leak_details(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fail(_function_name: str, **_kwargs: Any) -> pd.DataFrame:
        raise RuntimeError("private upstream detail")

    monkeypatch.setattr(akshare_upstream, "call", fail)
    response = await client.get(
        "/providers/akshare/search",
        params={"q": "AAPL"},
    )

    assert response.status_code == 502
    assert response.json()["error"]["code"] == "AKSHARE_UPSTREAM_ERROR"
    assert "private upstream detail" not in response.text


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("method", "path", "payload"),
    [
        ("GET", "/providers/akshare/search?q=SH.510300", None),
        (
            "POST",
            "/providers/akshare/snapshots",
            {"instrument_ids": ["US.AAPL", "US..SPX"]},
        ),
    ],
)
async def test_complete_multicall_request_has_one_deadline_and_stops_next_call(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    method: str,
    path: str,
    payload: dict[str, Any] | None,
) -> None:
    calls: list[str] = []
    first_finished = threading.Event()

    def slow_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        calls.append(function_name)
        time.sleep(0.05)
        first_finished.set()
        akshare_upstream.ensure_request_active()
        return _standard_catalog_call(function_name, **kwargs)

    monkeypatch.setattr(akshare_upstream, "call", slow_call)
    monkeypatch.setattr(akshare_upstream, "CALL_TIMEOUT_SECONDS", 0.01)

    response = await client.request(method, path, json=payload)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "AKSHARE_UPSTREAM_TIMEOUT"
    assert response.headers["Retry-After"] == "2"
    assert first_finished.wait(timeout=1)
    time.sleep(0.01)
    assert len(calls) == 1
