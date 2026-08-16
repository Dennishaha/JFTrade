"""News and corporate-action routes for both providers."""

from __future__ import annotations

from datetime import date, datetime, timedelta, timezone
from typing import Any

import httpx
import pandas as pd
import pytest

from marketdata_sidecar import akshare_news, akshare_upstream, upstream


def _yahoo_news_items() -> list[dict[str, Any]]:
    return [
        {
            "title": "Apple beats expectations",
            "summary": "Quarterly results.",
            "pubDate": "2026-08-15T14:30:00Z",
            "provider": {"displayName": "Reuters"},
            "canonicalUrl": {"url": "https://example.test/aapl-1"},
        },
        {
            "content": {
                "title": "Legacy payload",
                "summary": "Older schema.",
                "providerPublishTime": 1_753_812_000,
                "publisher": "Bloomberg",
                "link": "https://example.test/aapl-2",
            }
        },
        {"ad": True},
    ]


@pytest.mark.asyncio
async def test_yfinance_news_normalizes_current_and_legacy_items(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_news",
        lambda symbol, limit, **_kw: _yahoo_news_items(),
    )

    response = await client.get("/news/US/AAPL")

    assert response.status_code == 200
    body = response.json()
    assert body["market"] == "US"
    assert body["symbol"] == "AAPL"
    assert body["instrument_id"] == "US.AAPL"
    assert body["source"] == "yfinance-news"
    entries = body["entries"]
    assert len(entries) == 2
    assert entries[0] == {
        "title": "Apple beats expectations",
        "link": "https://example.test/aapl-1",
        "publisher": "Reuters",
        "published_at": "2026-08-15T14:30:00Z",
        "summary": "Quarterly results.",
    }
    assert entries[1]["published_at"] == "2025-07-29T18:00:00Z"
    assert entries[1]["publisher"] == "Bloomberg"
    assert entries[1]["link"] == "https://example.test/aapl-2"


@pytest.mark.asyncio
async def test_yfinance_news_limit_is_clamped_and_forwarded(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[tuple[str, int]] = []

    def fake_news(symbol: str, limit: int, **_kw: Any) -> list[dict[str, Any]]:
        calls.append((symbol, limit))
        return _yahoo_news_items()

    monkeypatch.setattr(upstream, "ticker_news", fake_news)

    limited = await client.get("/providers/yfinance/news/US/AAPL", params={"limit": 1})
    too_small = await client.get("/news/US/AAPL", params={"limit": 0})
    too_large = await client.get("/news/US/AAPL", params={"limit": 51})

    assert limited.status_code == 200
    assert len(limited.json()["entries"]) == 1
    assert calls == [("AAPL", 1)]
    assert too_small.status_code == 400
    assert too_small.json()["error"]["code"] == "invalid_request"
    assert too_large.status_code == 400


@pytest.mark.asyncio
async def test_yfinance_corporate_actions_merge_filter_and_default_window(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    today = datetime.now(timezone.utc).date()
    split_date = (today - timedelta(days=60)).isoformat()
    dividend_date = (today - timedelta(days=30)).isoformat()
    stale_date = (today - timedelta(days=800)).isoformat()
    monkeypatch.setattr(
        upstream,
        "ticker_actions",
        lambda _symbol, **_kw: {
            "dividends": [
                {"date": dividend_date, "value": 0.25},
                {"date": stale_date, "value": 0.1},
            ],
            "splits": [
                {"date": split_date, "value": 4.0},
                {"date": "not-a-date", "value": 2.0},
            ],
        },
    )

    response = await client.get("/corporate-actions/US/AAPL")

    assert response.status_code == 200
    body = response.json()
    assert body["source"] == "yfinance-actions"
    assert body["events"] == [
        {"kind": "split", "ex_date": split_date, "amount": None, "ratio": 4.0},
        {"kind": "dividend", "ex_date": dividend_date, "amount": 0.25, "ratio": None},
    ]


@pytest.mark.asyncio
async def test_yfinance_corporate_actions_from_to_are_inclusive(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        upstream,
        "ticker_actions",
        lambda _symbol, **_kw: {
            "dividends": [
                {"date": "2020-01-01", "value": 0.1},
                {"date": "2020-12-31", "value": 0.2},
                {"date": "2021-01-01", "value": 0.3},
            ],
            "splits": [],
        },
    )

    response = await client.get(
        "/corporate-actions/US/AAPL",
        params={"from": "2020-01-01T00:00:00Z", "to": "2020-12-31T00:00:00Z"},
    )
    inverted = await client.get(
        "/corporate-actions/US/AAPL",
        params={"from": "2021-01-01T00:00:00Z", "to": "2020-01-01T00:00:00Z"},
    )

    assert response.status_code == 200
    assert [event["ex_date"] for event in response.json()["events"]] == [
        "2020-01-01",
        "2020-12-31",
    ]
    assert inverted.status_code == 400
    assert inverted.json()["error"]["code"] == "invalid_time_range"


def _fhps_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "现金分红-现金分红比例": 27.627,
                "送转股份-送转总比例": None,
                "除权除息日": "2026-06-19 00:00:00",
            },
            {
                "代码": "000001",
                "名称": "平安银行",
                "现金分红-现金分红比例": 6.0,
                "送转股份-送转总比例": None,
                "除权除息日": "2026-06-10 00:00:00",
            },
        ]
    )


def _fhps_gift_frame() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "现金分红-现金分红比例": 30.0,
                "送转股份-送转总比例": 10.0,
                "除权除息日": "2024-12-20 00:00:00",
            },
            {
                "代码": "600519",
                "名称": "贵州茅台",
                "现金分红-现金分红比例": 5.0,
                "送转股份-送转总比例": None,
                "除权除息日": None,
            },
        ]
    )


@pytest.mark.asyncio
async def test_akshare_cn_corporate_actions_map_dividends_and_splits(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    report_dates: list[str] = []

    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_fhps_em"
        report_dates.append(kwargs["date"])
        if kwargs["date"] == "20251231":
            return _fhps_frame()
        if kwargs["date"] == "20240630":
            return _fhps_gift_frame()
        return pd.DataFrame()

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get(
        "/providers/akshare/corporate-actions/SH/600519",
        params={"from": "2024-01-01T00:00:00Z", "to": "2026-12-31T00:00:00Z"},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "SH.600519"
    assert body["source"] == "akshare-actions"
    assert body["events"] == [
        {"kind": "dividend", "ex_date": "2024-12-20", "amount": 3.0, "ratio": None},
        {"kind": "split", "ex_date": "2024-12-20", "amount": None, "ratio": 2.0},
        {"kind": "dividend", "ex_date": "2026-06-19", "amount": 2.7627, "ratio": None},
    ]
    assert "20231231" in report_dates
    assert "20260630" in report_dates


@pytest.mark.asyncio
async def test_akshare_news_normalizes_cn_headlines(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fake_call(function_name: str, **kwargs: Any) -> pd.DataFrame:
        assert function_name == "stock_news_em"
        assert kwargs == {"symbol": "600519"}
        return pd.DataFrame(
            [
                {
                    "关键词": "600519",
                    "新闻标题": "贵州茅台发布半年报",
                    "新闻内容": "摘要内容",
                    "发布时间": "2026-08-03 10:00:00",
                    "文章来源": "东方财富",
                    "新闻链接": "https://finance.eastmoney.com/a/1.html",
                }
            ]
        )

    monkeypatch.setattr(akshare_upstream, "call", fake_call)
    response = await client.get("/providers/akshare/news/SH/600519")

    assert response.status_code == 200
    body = response.json()
    assert body["instrument_id"] == "SH.600519"
    assert body["source"] == "akshare-news"
    assert body["entries"] == [
        {
            "title": "贵州茅台发布半年报",
            "link": "https://finance.eastmoney.com/a/1.html",
            "publisher": "东方财富",
            "published_at": "2026-08-03T02:00:00Z",
            "summary": "摘要内容",
        }
    ]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/providers/akshare/news/US/AAPL",
        "/providers/akshare/news/HK/00700",
        "/providers/akshare/corporate-actions/US/AAPL",
        "/providers/akshare/corporate-actions/HK/00700",
    ],
)
async def test_akshare_news_and_actions_reject_non_cn_markets(
    client: httpx.AsyncClient,
    path: str,
) -> None:
    response = await client.get(path)

    assert response.status_code == 400
    assert response.json()["error"]["code"] == "AKSHARE_UNSUPPORTED"


@pytest.mark.asyncio
async def test_akshare_news_upstream_failure_is_a_502(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fail(_function_name: str, **_kwargs: Any) -> pd.DataFrame:
        raise RuntimeError("private news failure")

    monkeypatch.setattr(akshare_upstream, "call", fail)
    response = await client.get("/providers/akshare/news/SH/600519")

    assert response.status_code == 502
    assert response.json()["error"]["code"] == "AKSHARE_UPSTREAM_ERROR"
    assert "private news failure" not in response.text


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/news/US/AAPL",
        "/corporate-actions/US/AAPL",
        "/providers/yfinance/news/US/AAPL",
        "/providers/yfinance/corporate-actions/US/AAPL",
    ],
)
async def test_yfinance_news_and_actions_paths_are_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    monkeypatch.setattr(
        upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(
        upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get(path)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "YFINANCE_RUNTIME_WARMING"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "path",
    [
        "/providers/akshare/news/SH/600519",
        "/providers/akshare/corporate-actions/SH/600519",
    ],
)
async def test_akshare_news_and_actions_paths_are_warming_gated(
    client: httpx.AsyncClient,
    monkeypatch: pytest.MonkeyPatch,
    path: str,
) -> None:
    monkeypatch.setattr(
        akshare_upstream,
        "runtime_snapshot",
        lambda: upstream.RuntimeSnapshot("warming"),
    )
    monkeypatch.setattr(
        akshare_upstream,
        "request_runtime_warmup",
        lambda: upstream.RuntimeSnapshot("warming"),
    )

    response = await client.get(path)

    assert response.status_code == 503
    assert response.json()["error"]["code"] == "AKSHARE_RUNTIME_WARMING"


def test_report_dates_cover_interim_and_annual_periods() -> None:
    # Report periods up to 2024 are always in the past for this suite.
    dates = akshare_news._report_dates(date(2023, 6, 1), date(2024, 3, 1))

    assert dates == [
        "20220630",
        "20221231",
        "20230630",
        "20231231",
        "20240630",
        "20241231",
    ]
