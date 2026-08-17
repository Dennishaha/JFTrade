"""Deterministic checks for the opt-in live smoke harness."""

from __future__ import annotations

import sys
from pathlib import Path

import httpx
import pytest

SCRIPT_ROOT = Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))

import marketdata_live_smoke as live_smoke  # noqa: E402


def test_live_smoke_requires_explicit_network_opt_in(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv(live_smoke.ENABLE_ENV, raising=False)

    assert live_smoke.main([]) == 2


def test_live_smoke_argument_matrix_is_explicit() -> None:
    args = live_smoke.parse_args(
        ["--provider", "akshare", "--suite", "research", "--timeout", "45"]
    )

    assert args.provider == "akshare"
    assert args.suite == "research"
    assert args.timeout == 45


def test_live_smoke_report_contains_only_sanitized_check_metadata() -> None:
    report = live_smoke.SmokeReport(provider="all", suite="full")
    report.add(
        live_smoke.Check(
            provider="yfinance",
            name="AAPL profile",
            method="GET",
            path="/providers/yfinance/profile/US/AAPL",
            status=200,
            duration_ms=123,
            ok=True,
            rows=2,
        )
    )

    payload = report.as_dict()

    assert payload["ok"] is True
    assert payload["schema_version"] == 1
    assert payload["endpoints"] == ["/providers/yfinance/profile/US/AAPL"]
    assert payload["failure_categories"] == {}
    assert payload["versions"] == {"sidecar": None, "providers": {}}
    assert payload["checks"][0] == {
        "provider": "yfinance",
        "name": "AAPL profile",
        "method": "GET",
        "path": "/providers/yfinance/profile/US/AAPL",
        "status": 200,
        "duration_ms": 123,
        "ok": True,
        "rows": 2,
        "error": None,
        "error_code": None,
        "failure_category": None,
    }
    assert "price" not in payload
    assert "business_summary" not in payload


def test_economic_all_day_event_keeps_date_without_timestamp() -> None:
    live_smoke._validate_economic_calendar(
        {"entries": [{"event_date": "2026-08-17", "event_timestamp": None}]}
    )


def test_economic_event_without_date_is_rejected() -> None:
    with pytest.raises(live_smoke.ContractViolation, match="event_date"):
        live_smoke._validate_economic_calendar(
            {"entries": [{"event_timestamp": None}]}
        )


@pytest.mark.asyncio
async def test_live_smoke_report_preserves_provider_error_code() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(
            502,
            json={"error": {"code": "YFINANCE_UPSTREAM_ERROR", "message": "dns"}},
        )
    )
    report = live_smoke.SmokeReport(provider="all", suite="core")
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://mock.live",
    ) as client:
        await live_smoke.LiveClient(client, report, "yfinance").request(
            "network failure",
            "GET",
            "/providers/yfinance/security/US/AAPL",
        )

    assert report.failures[0].provider == "yfinance"
    assert "YFINANCE_UPSTREAM_ERROR" in (report.failures[0].error or "")
    assert report.failures[0].error_code == "YFINANCE_UPSTREAM_ERROR"
    assert report.failures[0].failure_category == "http_5xx"
    assert report.as_dict()["failure_categories"] == {"http_5xx": 1}


@pytest.mark.asyncio
async def test_live_smoke_expected_rejection_is_not_reported_as_failure() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(
            400,
            json={"error": {"code": "unsupported_market", "message": "BJ"}},
        )
    )
    report = live_smoke.SmokeReport(provider="akshare", suite="research")
    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://mock.live",
    ) as client:
        await live_smoke.LiveClient(client, report, "akshare").request(
            "expected rejection",
            "GET",
            "/providers/akshare/profile/CN/830799",
            expected_status=(400,),
            expected_code="unsupported_market",
        )

    assert report.failures == []
    assert report.checks[0].error_code == "unsupported_market"
