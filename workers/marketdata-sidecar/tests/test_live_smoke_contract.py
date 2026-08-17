"""Deterministic checks for the opt-in live smoke harness."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import httpx
import pytest

SCRIPT_ROOT = Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPT_ROOT))

import marketdata_live_smoke as live_smoke  # noqa: E402


def test_live_smoke_requires_explicit_network_opt_in(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv(live_smoke.ENABLE_ENV, raising=False)
    monkeypatch.setattr(
        live_smoke,
        "_source_app",
        lambda: pytest.fail("source app must not load without explicit opt-in"),
    )

    assert live_smoke.main([]) == 2


def test_live_smoke_harness_failure_still_writes_report(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    async def fail_run(*_args: object, **_kwargs: object) -> None:
        raise RuntimeError("sensitive upstream detail")

    report_path = tmp_path / "live-report.json"
    monkeypatch.setenv(live_smoke.ENABLE_ENV, "1")
    monkeypatch.setattr(live_smoke, "run_smoke", fail_run)

    assert live_smoke.main(["--report", str(report_path)]) == 1
    payload = json.loads(report_path.read_text(encoding="utf-8"))
    assert payload["failure_count"] == 1
    assert payload["failure_categories"] == {"harness": 1}
    assert payload["checks"][0]["provider"] == "harness"
    assert payload["checks"][0]["error"] == "RuntimeError"
    assert "sensitive upstream detail" not in report_path.read_text(encoding="utf-8")
    assert "sensitive upstream detail" not in capsys.readouterr().err


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


def test_economic_event_without_timestamp_field_is_rejected() -> None:
    with pytest.raises(live_smoke.ContractViolation, match="event_timestamp"):
        live_smoke._validate_economic_calendar(
            {"entries": [{"event_date": "2026-08-17"}]}
        )


def test_analyst_research_validator_accepts_distribution_object() -> None:
    live_smoke._research_validator("analyst")(
        {"distribution": {"strong_buy": 33.3, "buy": 16.7}}
    )


@pytest.mark.parametrize("distribution", [[], {}])
def test_analyst_research_validator_rejects_invalid_distribution(
    distribution: object,
) -> None:
    with pytest.raises(live_smoke.ContractViolation, match="distribution"):
        live_smoke._research_validator("analyst")({"distribution": distribution})


@pytest.mark.parametrize(
    ("validator", "body", "message"),
    [
        (live_smoke._validate_board_catalog, {"boards": [{}]}, "name"),
        (
            live_smoke._validate_macro_catalog,
            {"categories": [{"indicators": [{}]}]},
            "indicator_id",
        ),
    ],
)
def test_catalog_validators_require_follow_up_identity(
    validator: live_smoke.Validator,
    body: dict[str, object],
    message: str,
) -> None:
    with pytest.raises(live_smoke.ContractViolation, match=message):
        validator(body)


def test_screen_page_requires_stable_instrument_id() -> None:
    with pytest.raises(live_smoke.ContractViolation, match="instrument_id"):
        live_smoke._validate_screen_page(
            {"entries": [{"code": "AAPL"}], "total": 1, "has_more": False}
        )


def test_screen_page_rejects_duplicate_instrument_id() -> None:
    with pytest.raises(live_smoke.ContractViolation, match="duplicate"):
        live_smoke._validate_screen_page(
            {
                "entries": [
                    {"instrument_id": "US.AAPL"},
                    {"instrument_id": "US.AAPL"},
                ],
                "total": 2,
                "has_more": False,
            }
        )


def test_akshare_screen_page_must_not_be_empty() -> None:
    with pytest.raises(live_smoke.ContractViolation, match="empty"):
        live_smoke._validate_non_empty_screen_page(
            {"entries": [], "total": 0, "has_more": False}
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


@pytest.mark.asyncio
async def test_live_smoke_malformed_expected_error_is_contract_failure() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(400, json={"error": "not-an-object"})
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

    assert report.failures[0].failure_category == "contract"
    assert report.failures[0].error_code is None


@pytest.mark.asyncio
async def test_live_smoke_sanitizes_unexpected_validator_error() -> None:
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(200, json={"entries": []})
    )
    report = live_smoke.SmokeReport(provider="akshare", suite="research")

    def fail_validation(_body: object) -> None:
        raise RuntimeError("sensitive upstream detail")

    async with httpx.AsyncClient(
        transport=transport,
        base_url="http://mock.live",
    ) as client:
        await live_smoke.LiveClient(client, report, "akshare").request(
            "validator failure",
            "GET",
            "/providers/akshare/example",
            validate=fail_validation,
        )

    assert report.failures[0].failure_category == "harness"
    assert report.failures[0].error == "RuntimeError"
    assert "sensitive upstream detail" not in json.dumps(report.as_dict())
