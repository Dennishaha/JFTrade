"""Opt-in live contract smoke for the Yahoo and AKShare providers.

This script is intentionally outside the ordinary pytest suite.  It only
contacts an upstream provider when ``JFTRADE_MARKETDATA_LIVE_SMOKE=1`` is set
and records a sanitized JSON report rather than raw market-data responses.
The default mode mounts the source ASGI app in-process; ``--base-url`` can be
used by a manual workflow to probe an already running frozen helper.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
import time
from collections import Counter
from dataclasses import dataclass, field
from datetime import date, timedelta
from pathlib import Path
from typing import Any, Awaitable, Callable, Mapping, Sequence

import httpx

WORKER_ROOT = Path(__file__).resolve().parents[1]
if str(WORKER_ROOT / "src") not in sys.path:
    sys.path.insert(0, str(WORKER_ROOT / "src"))

from marketdata_sidecar.main import app  # noqa: E402

ENABLE_ENV = "JFTRADE_MARKETDATA_LIVE_SMOKE"
DEFAULT_TIMEOUT_SECONDS = 30.0
HEALTH_TIMEOUT_SECONDS = 120.0
HEALTH_POLL_SECONDS = 1.0


class ContractViolation(RuntimeError):
    """A live response was reachable but did not satisfy its contract."""


@dataclass
class Check:
    provider: str
    name: str
    method: str
    path: str
    status: int | None
    duration_ms: int
    ok: bool
    rows: int | None = None
    error: str | None = None
    error_code: str | None = None
    failure_category: str | None = None


@dataclass
class SmokeReport:
    provider: str
    suite: str
    checks: list[Check] = field(default_factory=list)
    sidecar_version: str | None = None
    provider_versions: dict[str, str] = field(default_factory=dict)
    started_at: float = field(default_factory=time.monotonic, repr=False)

    @property
    def failures(self) -> list[Check]:
        return [check for check in self.checks if not check.ok]

    def add(self, check: Check) -> None:
        self.checks.append(check)

    def as_dict(self) -> dict[str, Any]:
        failures = self.failures
        failure_categories = Counter(
            check.failure_category or "unknown" for check in failures
        )
        return {
            "schema_version": 1,
            "provider": self.provider,
            "suite": self.suite,
            "duration_ms": max(0, int((time.monotonic() - self.started_at) * 1000)),
            "versions": {
                "sidecar": self.sidecar_version,
                "providers": dict(sorted(self.provider_versions.items())),
            },
            "endpoints": sorted({check.path for check in self.checks}),
            "ok": not self.failures,
            "checks": [check.__dict__ for check in self.checks],
            "failure_count": len(failures),
            "failure_categories": dict(sorted(failure_categories.items())),
        }

    def write(self, path: str | None) -> None:
        payload = json.dumps(self.as_dict(), ensure_ascii=False, indent=2) + "\n"
        if path:
            destination = Path(path)
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_text(payload, encoding="utf-8")
        summary_path = os.environ.get("GITHUB_STEP_SUMMARY", "").strip()
        if summary_path:
            _write_github_summary(summary_path, self)


Validator = Callable[[Mapping[str, Any]], None]


class LiveClient:
    def __init__(
        self,
        client: httpx.AsyncClient,
        report: SmokeReport,
        provider: str,
    ) -> None:
        self.client = client
        self.report = report
        self.provider = provider

    async def request(
        self,
        name: str,
        method: str,
        path: str,
        *,
        expected_status: Sequence[int] = (200,),
        expected_code: str | None = None,
        params: Mapping[str, Any] | None = None,
        json_body: Mapping[str, Any] | None = None,
        validate: Validator | None = None,
    ) -> dict[str, Any] | None:
        started = time.monotonic()
        status: int | None = None
        rows: int | None = None
        error: str | None = None
        error_code: str | None = None
        failure_category: str | None = None
        body: dict[str, Any] | None = None
        try:
            response = await self.client.request(
                method,
                path,
                params=params,
                json=json_body,
            )
            status = response.status_code
            decoded = response.json()
            if not isinstance(decoded, dict):
                raise ContractViolation("response body is not a JSON object")
            body = decoded
            rows = _row_count(body)
            error_code = _error_code(body)
            if status not in expected_status:
                failure_category = _status_failure_category(status) or "contract"
                raise ContractViolation(
                    f"expected HTTP {tuple(expected_status)}, got {status}"
                    + (f" code={error_code}" if error_code else "")
                )
            if expected_code is not None:
                code = str(body.get("error", {}).get("code", ""))
                if code != expected_code:
                    raise ContractViolation(
                        f"expected error code {expected_code}, got {code or '<empty>'}"
                    )
            if validate is not None:
                validate(body)
        except ContractViolation as exc:
            error = str(exc)
            if failure_category is None:
                failure_category = "contract"
        except httpx.TimeoutException as exc:
            error = type(exc).__name__
            failure_category = "timeout"
        except httpx.RequestError as exc:
            error = type(exc).__name__
            failure_category = "network"
        except Exception as exc:  # noqa: BLE001 - report every matrix row
            error = str(exc)
            failure_category = "harness"
        if error is not None and failure_category is None:
            failure_category = _status_failure_category(status)
        self.report.add(
            Check(
                provider=self.provider,
                name=name,
                method=method,
                path=path,
                status=status,
                duration_ms=max(0, int((time.monotonic() - started) * 1000)),
                ok=error is None,
                rows=rows,
                error=error,
                error_code=error_code,
                failure_category=failure_category,
            )
        )
        return body if error is None else None

    async def wait_for_provider(self, provider: str) -> dict[str, Any] | None:
        path = f"/providers/{provider}/health"
        deadline = time.monotonic() + HEALTH_TIMEOUT_SECONDS
        last: dict[str, Any] | None = None
        last_status: int | None = None
        last_exception: BaseException | None = None
        started = time.monotonic()
        while time.monotonic() < deadline:
            try:
                response = await self.client.get(path)
                last_status = response.status_code
                decoded = response.json()
                if isinstance(decoded, dict):
                    last = decoded
                    if (
                        response.status_code == 503
                        and decoded.get("error", {}).get("code", "").endswith(
                            "_RUNTIME_FAILED"
                        )
                    ):
                        break
                    if (
                        response.status_code == 200
                        and decoded.get("ok") is True
                        and decoded.get("runtime_state") == "ready"
                    ):
                        self.report.add(
                            Check(
                                provider=provider,
                                name="provider health",
                                method="GET",
                                path=path,
                                status=response.status_code,
                                duration_ms=max(
                                    0,
                                    int((time.monotonic() - started) * 1000),
                                ),
                                ok=True,
                            )
                        )
                        provider_version = _provider_version(decoded)
                        if provider_version:
                            self.report.provider_versions[provider] = provider_version
                        return decoded
            except Exception as exc:  # noqa: BLE001 - continue readiness polling
                last_exception = exc
            await asyncio.sleep(HEALTH_POLL_SECONDS)
        state = last.get("runtime_state") if last else "unreachable"
        error = last.get("error") if last else None
        detail = (
            str(error.get("code", ""))
            if isinstance(error, Mapping)
            else f"state={state}"
        )
        if last_exception is not None and last is None:
            detail = type(last_exception).__name__
        error_code = (
            str(error.get("code"))
            if isinstance(error, Mapping) and error.get("code")
            else None
        )
        self.report.add(
            Check(
                provider=self.provider,
                name="provider health",
                method="GET",
                path=path,
                status=last_status,
                duration_ms=max(0, int((time.monotonic() - started) * 1000)),
                ok=False,
                error=f"provider did not become ready ({detail})",
                error_code=error_code,
                failure_category=(
                    "runtime"
                    if last is not None
                    else _status_failure_category(last_status)
                    or (
                        "timeout"
                        if isinstance(last_exception, httpx.TimeoutException)
                        else "network"
                    )
                ),
            )
        )
        return None


async def run_smoke(
    provider: str,
    suite: str,
    *,
    base_url: str | None = None,
    timeout_seconds: float = DEFAULT_TIMEOUT_SECONDS,
) -> SmokeReport:
    report = SmokeReport(provider=provider, suite=suite)
    transport: httpx.AsyncBaseTransport | None = None
    if base_url is None:
        transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(
        transport=transport,
        base_url=base_url or "http://marketdata.live",
        timeout=timeout_seconds,
    ) as client:
        process = LiveClient(client, report, "sidecar")
        process_health = await process.request(
            "sidecar process health",
            "GET",
            "/healthz",
            validate=lambda body: _require_keys(body, "ok", "version"),
        )
        if process_health is not None:
            report.sidecar_version = str(process_health["version"])
        for selected in ("yfinance", "akshare") if provider == "all" else (provider,):
            live = LiveClient(client, report, selected)
            if await live.wait_for_provider(selected) is None:
                continue
            if suite in {"core", "full"}:
                await _run_core(live, selected)
            if suite in {"research", "full"}:
                await _run_research(live, selected)
    return report


async def _run_core(live: LiveClient, provider: str) -> None:
    prefix = f"/providers/{provider}"
    market, symbol = "US", "AAPL"
    await live.request(
        "search returns an instrument",
        "GET",
        f"{prefix}/search",
        params={"q": "AAPL", "limit": 5},
        validate=lambda body: _require_non_empty(body, "entries"),
    )
    await live.request(
        "security details",
        "GET",
        f"{prefix}/security/{market}/{symbol}",
        validate=lambda body: _require_keys(body, "instrument_id", "supported_periods"),
    )
    await live.request(
        "snapshot details",
        "GET",
        f"{prefix}/snapshot/{market}/{symbol}",
        validate=lambda body: _require_keys(body, "instrument_id", "observed_at"),
    )
    await live.request(
        "daily candles",
        "GET",
        f"{prefix}/candles/{market}/{symbol}",
        params={"period": "1d", "limit": 2},
        validate=lambda body: _require_non_empty(body, "candles"),
    )


async def _run_research(live: LiveClient, provider: str) -> None:
    if provider == "yfinance":
        await _run_yfinance_research(live)
    else:
        await _run_akshare_research(live)


async def _run_yfinance_research(live: LiveClient) -> None:
    prefix = "/providers/yfinance"
    for market, symbol in (("US", "AAPL"), ("HK", "00700")):
        await live.request(
            f"{market} company profile",
            "GET",
            f"{prefix}/profile/{market}/{symbol}",
            validate=lambda body: _require_non_empty(body, "groups"),
        )
    for route, name in (("financials", "financials"), ("analyst", "analyst"), ("ownership", "ownership")):
        await live.request(
            f"AAPL {name} research",
            "GET",
            f"{prefix}/{route}/US/AAPL",
            params={"statement": "income"} if route == "financials" else None,
            validate=_research_validator(route),
        )
    await live.request(
        "US active ranking",
        "GET",
        f"{prefix}/rankings",
        params={"market": "US", "kind": "active", "limit": 5},
        validate=lambda body: _require_non_empty(body, "entries"),
    )
    await _check_yfinance_pages(live)
    await live.request(
        "Yahoo rejects non-US screening",
        "POST",
        f"{prefix}/screen",
        json_body={"market": "HK", "limit": 2},
        expected_status=(400,),
        expected_code="unsupported_market",
    )
    await live.request(
        "Yahoo rejects multiple screen sorts",
        "POST",
        f"{prefix}/screen",
        json_body={
            "market": "US",
            "sorts": [
                {"factor_key": "basic.code", "direction": "asc"},
                {"factor_key": "simple.price", "direction": "desc"},
            ],
            "limit": 2,
        },
        expected_status=(400,),
        expected_code="unsupported_kind",
    )


async def _check_yfinance_pages(live: LiveClient) -> None:
    path = "/providers/yfinance/screen"
    first = await live.request(
        "Yahoo screen first page",
        "POST",
        path,
        json_body={
            "market": "US",
            "sorts": [{"factor_key": "basic.code", "direction": "asc"}],
            "offset": 0,
            "limit": 20,
        },
        validate=lambda body: _require_keys(body, "entries", "total", "has_more"),
    )
    if first is None:
        return
    next_offset = first.get("next_offset")
    if not first.get("has_more") or not isinstance(next_offset, int) or next_offset <= 0:
        _record_failure(
            live.report,
            "Yahoo screen cursor advances",
            path,
            "first page did not expose a forward next_offset",
            provider=live.provider,
        )
        return
    first_ids = {str(item.get("instrument_id")) for item in first.get("entries", [])}
    second = await live.request(
        "Yahoo screen second page",
        "POST",
        path,
        json_body={
            "market": "US",
            "sorts": [{"factor_key": "basic.code", "direction": "asc"}],
            "offset": next_offset,
            "limit": 20,
        },
        validate=lambda body: _require_keys(body, "entries", "total", "has_more"),
    )
    if second is not None:
        second_ids = {str(item.get("instrument_id")) for item in second.get("entries", [])}
        if first_ids & second_ids:
            _record_failure(live.report, "Yahoo screen pages overlap", path, "page identities overlap")


async def _run_akshare_research(live: LiveClient) -> None:
    prefix = "/providers/akshare"
    for market, symbol in (("CN", "600519"), ("HK", "00700")):
        await live.request(
            f"{market} company profile",
            "GET",
            f"{prefix}/profile/{market}/{symbol}",
            validate=lambda body: _require_non_empty(body, "groups"),
        )
    await live.request(
        "AKShare rejects Beijing listing research",
        "GET",
        f"{prefix}/profile/CN/830799",
        expected_status=(400,),
        expected_code="unsupported_market",
    )
    for route, name in (("financials", "financials"), ("analyst", "analyst"), ("ownership", "ownership")):
        await live.request(
            f"CN {name} research",
            "GET",
            f"{prefix}/{route}/CN/600519",
            params={"statement": "income"} if route == "financials" else None,
            validate=_research_validator(route),
        )
    for market, kind in (("CN", "gainers"), ("HK", "active")):
        await live.request(
            f"AKShare {market} ranking",
            "GET",
            f"{prefix}/rankings",
            params={"market": market, "kind": kind, "limit": 5},
            validate=lambda body: _require_non_empty(body, "entries"),
        )
    for board_kind in ("industry", "concept"):
        boards = await live.request(
            f"AKShare {board_kind} boards",
            "GET",
            f"{prefix}/industries",
            params={"market": "CN", "kind": board_kind},
            validate=lambda body: _require_non_empty(body, "boards"),
        )
        if boards is not None:
            board = str(boards["boards"][0].get("name", "")).strip()
            if board:
                await live.request(
                    f"AKShare {board_kind} members",
                    "GET",
                    f"{prefix}/industries/{board}/members",
                    params={"market": "CN", "kind": board_kind, "limit": 5},
                    validate=lambda body: _require_list(body, "entries"),
                )
    for market in ("CN", "SH", "SZ", "HK", "US"):
        await live.request(
            f"AKShare {market} screen",
            "POST",
            f"{prefix}/screen",
            json_body={"market": market, "limit": 2},
            validate=lambda body: _require_keys(body, "entries", "total", "has_more"),
        )
    await _run_akshare_calendar(live)


async def _run_akshare_calendar(live: LiveClient) -> None:
    prefix = "/providers/akshare"
    end = date.today()
    begin = end - timedelta(days=30)
    window = {"begin_date": begin.isoformat(), "end_date": end.isoformat()}
    await live.request(
        "AKShare earnings calendar",
        "GET",
        f"{prefix}/calendar/earnings",
        params=window,
        validate=lambda body: _require_list(body, "entries"),
    )
    await live.request(
        "AKShare dividend calendar",
        "GET",
        f"{prefix}/calendar/dividends",
        params={"date": end.isoformat()},
        validate=lambda body: _require_list(body, "entries"),
    )
    economic = await live.request(
        "AKShare 31-day economic calendar",
        "GET",
        f"{prefix}/calendar/economic",
        params=window,
        validate=_validate_economic_calendar,
    )
    await live.request(
        "AKShare IPO calendar",
        "GET",
        f"{prefix}/calendar/ipos",
        validate=lambda body: _require_list(body, "entries"),
    )
    indicators = await live.request(
        "AKShare macro indicator catalog",
        "GET",
        f"{prefix}/macro/indicators",
        validate=lambda body: _require_non_empty(body, "categories"),
    )
    indicator_id = _first_indicator_id(indicators)
    if indicator_id:
        await live.request(
            "AKShare macro indicator history",
            "GET",
            f"{prefix}/macro/indicator-history",
            params={"indicator_id": indicator_id, "limit": 3},
            validate=lambda body: _require_keys(body, "indicator_id", "entries"),
        )


def _research_validator(route: str) -> Validator:
    key = {"financials": "periods", "analyst": "distribution", "ownership": "groups"}[route]
    return lambda body: _require_non_empty(body, key)


def _validate_economic_calendar(body: Mapping[str, Any]) -> None:
    entries = body.get("entries")
    if not isinstance(entries, list):
        raise ContractViolation("economic response entries is not a list")
    for entry in entries:
        if not isinstance(entry, Mapping) or not entry.get("event_date"):
            raise ContractViolation("economic entry has no event_date")


def _first_indicator_id(body: Mapping[str, Any] | None) -> str | None:
    if body is None:
        return None
    for category in body.get("categories", []):
        for indicator in category.get("indicators", []):
            value = str(indicator.get("indicator_id", "")).strip()
            if value:
                return value
    return None


def _require_keys(body: Mapping[str, Any], *keys: str) -> None:
    missing = [key for key in keys if key not in body]
    if missing:
        raise ContractViolation(f"response is missing keys: {', '.join(missing)}")


def _require_list(body: Mapping[str, Any], key: str) -> None:
    if not isinstance(body.get(key), list):
        raise ContractViolation(f"response field {key} is not a list")


def _require_non_empty(body: Mapping[str, Any], key: str) -> None:
    _require_list(body, key)
    if not body[key]:
        raise ContractViolation(f"response field {key} is empty")


def _row_count(body: Mapping[str, Any]) -> int | None:
    for key in ("entries", "candles", "groups", "periods", "boards", "categories"):
        value = body.get(key)
        if isinstance(value, list):
            return len(value)
    return None


def _error_code(body: Mapping[str, Any]) -> str | None:
    error = body.get("error")
    if not isinstance(error, Mapping):
        return None
    value = str(error.get("code", "")).strip()
    return value or None


def _provider_version(body: Mapping[str, Any]) -> str | None:
    for key in ("provider_version", "yfinance_version"):
        value = str(body.get(key, "")).strip()
        if value and value != "unavailable":
            return value
    return None


def _status_failure_category(status: int | None) -> str | None:
    if status is None:
        return None
    if 400 <= status < 500:
        return "http_4xx"
    if 500 <= status < 600:
        return "http_5xx"
    return "contract"


def _record_failure(
    report: SmokeReport,
    name: str,
    path: str,
    error: str,
    *,
    provider: str | None = None,
) -> None:
    report.add(
        Check(
            provider=provider or report.provider,
            name=name,
            method="ASSERT",
            path=path,
            status=None,
            duration_ms=0,
            ok=False,
            error=error,
            failure_category="contract",
        )
    )


def _write_github_summary(path: str, report: SmokeReport) -> None:
    lines = [
        f"### Market-data live smoke: {'PASS' if not report.failures else 'FAIL'}",
        "",
        f"- Provider: `{report.provider}`",
        f"- Suite: `{report.suite}`",
        f"- Checks: `{len(report.checks)}`",
        f"- Failures: `{len(report.failures)}`",
        "",
    ]
    if report.failures:
        lines.extend(["| Provider | Check | Status | Error |", "| --- | --- | ---: | --- |"])
        for failure in report.failures:
            message = (failure.error or "unknown").replace("|", "\\|")
            lines.append(
                f"| `{failure.provider}` | `{failure.name}` | {failure.status or '-'} | {message} |"
            )
    with Path(path).open("a", encoding="utf-8") as handle:
        handle.write("\n".join(lines) + "\n")


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run opt-in live market-data contract checks.")
    parser.add_argument("--provider", choices=("yfinance", "akshare", "all"), default="all")
    parser.add_argument("--suite", choices=("core", "research", "full"), default="full")
    parser.add_argument("--base-url", default=None, help="Probe an already running helper instead of source ASGI.")
    parser.add_argument("--report", default=None, help="Write a sanitized JSON report to this path.")
    parser.add_argument("--timeout", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv)
    if os.environ.get(ENABLE_ENV) != "1":
        print(f"REFUSED: set {ENABLE_ENV}=1 to enable real provider network access", file=sys.stderr)
        return 2
    report: SmokeReport | None = None
    try:
        report = asyncio.run(
            run_smoke(
                args.provider,
                args.suite,
                base_url=args.base_url,
                timeout_seconds=args.timeout,
            )
        )
        return 0 if not report.failures else 1
    except Exception as exc:  # noqa: BLE001 - preserve a report on harness failure
        print(f"live smoke harness failed: {exc}", file=sys.stderr)
        return 1
    finally:
        if report is not None:
            report.write(args.report)
            print(json.dumps(report.as_dict(), ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    raise SystemExit(main())
