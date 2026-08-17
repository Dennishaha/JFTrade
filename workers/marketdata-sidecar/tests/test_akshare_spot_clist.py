"""Unit tests for the Eastmoney clist direct spot fetcher.

Covers the HTTP boundary (page fetch, pagination, schema errors) and the
frame builder (column mapping, US code assembly, numeric coercion, the
1-based 序号 ranking).  The catalog/screen/rankings consumers are covered
by the route tests with ``fetch_spot_frame_clist`` mocked.
"""

from __future__ import annotations

from typing import Any
from unittest import mock

import pandas as pd
import pytest

from marketdata_sidecar import akshare_spot_clist, akshare_upstream
from marketdata_sidecar.errors import SidecarError


def _clist_payload(rows: list[dict[str, Any]], total: int | None = None) -> dict[str, Any]:
    return {"data": {"total": total if total is not None else len(rows), "diff": rows}}


def _us_row(**overrides: Any) -> dict[str, Any]:
    row: dict[str, Any] = {
        "f2": 210.125,
        "f3": 1.2,
        "f5": 1234567,
        "f12": "AAPL",
        "f13": 105,
        "f14": "Apple Inc.",
        "f20": 3.2e12,
        "f23": 42.0,
        "f115": 28.4,
    }
    row.update(overrides)
    return row


@pytest.mark.parametrize(
    "market",
    ["US", "HK"],
)
def test_clist_fetch_all_pages_paginates_until_total(monkeypatch: pytest.MonkeyPatch, market: str) -> None:
    calls: list[int] = []
    page_two_rows = [
        _us_row(f12="MSFT", f13=106) if market == "US" else {"f12": "00005", "f14": "汇丰控股"}
    ]

    def fake_page(fs: str, page: int) -> tuple[int, list[dict[str, Any]]]:
        assert fs == akshare_spot_clist._MARKET_FS[market]
        calls.append(page)
        if page == 1:
            return 1500, [_us_row(), _us_row(f12="NVDA", f13=107)]
        return 1500, page_two_rows

    monkeypatch.setattr(akshare_spot_clist, "_clist_page", fake_page)
    records = akshare_spot_clist._fetch_all_pages(akshare_spot_clist._MARKET_FS[market])
    assert calls == [1, 2]
    assert len(records) == 3


def test_clist_page_raises_on_invalid_schema(monkeypatch: pytest.MonkeyPatch) -> None:
    response = mock.Mock()
    response.json.return_value = {"data": {"total": 1, "diff": "not-a-list"}}
    monkeypatch.setattr(akshare_upstream, "require_runtime", lambda: None)
    with mock.patch("requests.get", return_value=response) as get:
        with pytest.raises(SidecarError) as exc:
            akshare_spot_clist._clist_page("m:105,m:106,m:107", 1)
    assert exc.value.code == "AKSHARE_SCHEMA_ERROR"
    assert get.call_args.kwargs["params"]["pz"] == "1000"


def test_clist_page_passes_expected_params(monkeypatch: pytest.MonkeyPatch) -> None:
    response = mock.Mock()
    response.json.return_value = _clist_payload([_us_row()])
    monkeypatch.setattr(akshare_upstream, "require_runtime", lambda: None)
    with mock.patch("requests.get", return_value=response) as get:
        akshare_spot_clist._clist_page("m:105,m:106,m:107", 2)
    params = get.call_args.kwargs["params"]
    assert params["pn"] == "2"
    assert params["fs"] == "m:105,m:106,m:107"
    assert "f23" in params["fields"]
    assert "f115" in params["fields"]


def test_build_us_frame_assembles_eastmoney_code_and_columns() -> None:
    frame = akshare_spot_clist._build_frame(
        "US",
        [
            _us_row(f12="AAPL", f13=105, f3=-0.5),
            _us_row(f12="NVDA", f13=107, f3=2.5),
        ],
    )
    assert list(frame["代码"]) == ["107.NVDA", "105.AAPL"]  # 按涨跌幅降序重排
    assert frame.loc[1, "名称"] == "Apple Inc."
    # 数值列已数值化；序号按涨跌幅降序 1 基排名
    assert frame.loc[0, "序号"] == 1  # NVDA +2.5 在前
    assert frame.loc[1, "序号"] == 2  # AAPL -0.5 在后
    assert float(frame.loc[1, "市净率"]) == 42.0
    assert float(frame.loc[1, "市盈率-动态"]) == 28.4
    assert float(frame.loc[1, "总市值"]) == 3.2e12


def test_build_hk_frame_keeps_digit_code_and_hk_ohlc_names() -> None:
    frame = akshare_spot_clist._build_frame(
        "HK",
        [
            {"f12": "00700", "f14": "腾讯控股", "f15": 381.0, "f20": 3.5e12, "f23": 3.4},
        ],
    )
    assert list(frame["代码"]) == ["00700"]
    assert "今开" in frame.columns  # HK 帧的 OHLC 叫法与 akshare 一致
    assert "开盘价" not in frame.columns
    assert float(frame.loc[0, "总市值"]) == 3.5e12


def test_build_frame_handles_missing_fcodes_as_nan() -> None:
    frame = akshare_spot_clist._build_frame("US", [_us_row(f20=None, f23=None)])
    assert pd.isna(frame.loc[0, "总市值"])
    assert pd.isna(frame.loc[0, "市净率"])
