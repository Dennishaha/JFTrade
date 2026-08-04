from __future__ import annotations

import math
from datetime import datetime, timezone

import pandas as pd

from marketdata_sidecar.conversion import (
    clean_text,
    convert_history,
    finite_float,
    non_negative_int,
    parse_rfc3339_utc,
    timestamp_as_rfc3339,
)
from marketdata_sidecar.errors import SidecarError


def test_numeric_conversion_rejects_non_finite_and_boolean_values() -> None:
    assert finite_float("12.5") == 12.5
    assert finite_float(math.nan) is None
    assert finite_float(math.inf) is None
    assert finite_float(True) is None
    assert non_negative_int(12.9) == 12
    assert non_negative_int(9_007_199_254_740_993) == 9_007_199_254_740_993
    assert non_negative_int("9007199254740993") == 9_007_199_254_740_993
    assert non_negative_int(-1) is None
    assert clean_text(math.nan) is None
    assert clean_text(" <NA> ") is None


def test_timestamps_are_normalized_to_rfc3339_utc() -> None:
    assert timestamp_as_rfc3339("2026-07-28T09:30:00-04:00") == (
        "2026-07-28T13:30:00Z"
    )
    assert timestamp_as_rfc3339(datetime(2026, 7, 28, 9, 30)) == (
        "2026-07-28T13:30:00Z"
    )
    assert timestamp_as_rfc3339(float("nan")) is None


def test_parse_time_requires_explicit_timezone() -> None:
    parsed = parse_rfc3339_utc("2026-07-28T13:30:00Z", "from")
    assert parsed == datetime(2026, 7, 28, 13, 30, tzinfo=timezone.utc)

    try:
        parse_rfc3339_utc("2026-07-28T13:30:00", "from")
    except SidecarError as exc:
        assert exc.code == "invalid_time"
    else:
        raise AssertionError("timezone-less timestamp must fail")


def test_history_conversion_sorts_limits_and_drops_bad_ohlc() -> None:
    index = pd.DatetimeIndex(
        [
            "2026-07-28T16:05:00-04:00",
            "2026-07-28T09:30:00-04:00",
            "2026-07-28T08:00:00-04:00",
            "2026-07-28T10:00:00-04:00",
        ]
    )
    frame = pd.DataFrame(
        {
            "Open": [11.0, 10.0, 9.0, math.nan],
            "High": [12.0, 11.0, 10.0, 11.0],
            "Low": [10.0, 9.0, 8.0, 9.0],
            "Close": [11.5, 10.5, 9.5, 10.0],
            "Volume": [300, 200, math.nan, 100],
        },
        index=index,
    )

    candles = convert_history(frame, limit=2)

    assert [candle.at for candle in candles] == [
        "2026-07-28T13:30:00Z",
        "2026-07-28T20:05:00Z",
    ]
    assert candles[0].volume == 200
    assert all(math.isfinite(candle.close) for candle in candles)


def test_history_conversion_honors_inclusive_utc_bounds() -> None:
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
                "2026-07-28T13:31:00Z",
                "2026-07-28T13:32:00Z",
            ]
        ),
    )

    candles = convert_history(
        frame,
        limit=10,
        from_time=datetime(2026, 7, 28, 13, 31, tzinfo=timezone.utc),
        to_time=datetime(2026, 7, 28, 13, 32, tzinfo=timezone.utc),
    )

    assert [candle.at for candle in candles] == [
        "2026-07-28T13:31:00Z",
        "2026-07-28T13:32:00Z",
    ]
