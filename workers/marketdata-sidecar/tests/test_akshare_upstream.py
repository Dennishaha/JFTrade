from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor
import threading
import time
from types import SimpleNamespace
import pytest
import requests

from marketdata_sidecar import akshare_upstream
from marketdata_sidecar.errors import SidecarError
from marketdata_sidecar.upstream import RuntimeSnapshot

AK_RUN = akshare_upstream.run
US_MINUTE_ROWS = akshare_upstream.us_minute_rows
HK_MINUTE_ROWS = akshare_upstream.hk_minute_rows


@pytest.mark.parametrize(
    ("source", "expected"),
    [
        (
            "https://72.push2.eastmoney.com/api/qt/clist/get",
            "https://72.push2.eastmoney.com/webguest/api/qt/clist/get?timil=1",
        ),
        (
            "https://push2.eastmoney.com/api/qt/clist/get?pn=2&timil=9",
            "https://push2.eastmoney.com/webguest/api/qt/clist/get?pn=2&timil=9",
        ),
        (
            "https://push2delay.eastmoney.com/api/qt/clist/get?pn=1",
            "https://push2delay.eastmoney.com/api/qt/clist/get?pn=1",
        ),
        (
            "https://push2.eastmoney.com/api/qt/stock/get?secid=105.AAPL",
            "https://push2.eastmoney.com/api/qt/stock/get?secid=105.AAPL",
        ),
    ],
)
def test_eastmoney_catalog_url_uses_current_guest_endpoint(
    source: str,
    expected: str,
) -> None:
    assert akshare_upstream._rewrite_eastmoney_url(source) == expected


def test_akshare_runtime_import_is_independent_and_idempotent(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    imports: list[str] = []
    module = SimpleNamespace()
    monkeypatch.setattr(akshare_upstream, "_runtime_started", False)
    monkeypatch.setattr(akshare_upstream, "_runtime_snapshot", RuntimeSnapshot("warming"))
    monkeypatch.setattr(akshare_upstream, "_runtime_components", None)
    monkeypatch.setattr(
        akshare_upstream.importlib,
        "import_module",
        lambda name: imports.append(name) or module,
    )

    akshare_upstream.warm_runtime()
    akshare_upstream.warm_runtime()

    assert imports == ["akshare"]
    assert akshare_upstream.runtime_snapshot().state == "ready"
    assert akshare_upstream._runtime_components is not None
    assert akshare_upstream._runtime_components.akshare is module


class _FakeResponse:
    def __init__(self, *, text: str = "", payload: object = None) -> None:
        self.text = text
        self._payload = payload

    def raise_for_status(self) -> None:
        return None

    def json(self) -> object:
        return self._payload


def test_sina_us_minute_jsonp_preserves_real_ohlcv(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    seen: dict[str, object] = {}

    def fake_get(url: str, **kwargs: object) -> _FakeResponse:
        seen.update(url=url, **kwargs)
        return _FakeResponse(
            text=(
                "/*redirect guard*/\nvar jftrade_us_minutes=(["
                '{"d":"2026-08-04 09:30:00","o":"100.1","h":"101.2",'
                '"l":"99.8","c":"100.9","v":"123","a":"12400"}'
                "]);"
            )
        )

    monkeypatch.setattr(akshare_upstream, "require_runtime", lambda: None)
    monkeypatch.setattr(requests, "get", fake_get)

    rows = US_MINUTE_ROWS("BABA")

    assert seen["url"] == akshare_upstream.SINA_US_MINUTE_URL
    assert seen["params"] == {"symbol": "BABA", "type": "1"}
    assert rows == [
        {
            "时间": "2026-08-04 09:30:00",
            "开盘": "100.1",
            "最高": "101.2",
            "最低": "99.8",
            "收盘": "100.9",
            "成交量": "123",
            "成交额": "12400",
        }
    ]


def test_hk_minute_response_preserves_real_ohlcv(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    seen: dict[str, object] = {}

    def fake_get(url: str, **kwargs: object) -> _FakeResponse:
        seen.update(url=url, **kwargs)
        return _FakeResponse(
            payload={
                "data": {
                    "trends": [
                        "2026-08-04 09:30,126.8,128.9,129.1,125.7,6227700,795761248,127.4"
                    ]
                }
            }
        )

    monkeypatch.setattr(akshare_upstream, "require_runtime", lambda: None)
    monkeypatch.setattr(requests, "get", fake_get)

    rows = HK_MINUTE_ROWS("09988")

    assert seen["url"] == akshare_upstream.EASTMONEY_HK_MINUTE_URL
    assert seen["params"]["secid"] == "116.09988"  # type: ignore[index]
    assert rows == [
        {
            "时间": "2026-08-04 09:30",
            "开盘": "126.8",
            "收盘": "128.9",
            "最高": "129.1",
            "最低": "125.7",
            "成交量": "6227700",
            "成交额": "795761248",
        }
    ]


@pytest.mark.parametrize(
    ("function_name", "response"),
    [
        ("us_minute_rows", _FakeResponse(text="not jsonp")),
        ("hk_minute_rows", _FakeResponse(payload={"data": None})),
    ],
)
def test_minute_upstream_rejects_malformed_responses(
    function_name: str,
    response: _FakeResponse,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(akshare_upstream, "require_runtime", lambda: None)
    monkeypatch.setattr(requests, "get", lambda *_args, **_kwargs: response)

    with pytest.raises(SidecarError) as error:
        function = US_MINUTE_ROWS if function_name.startswith("us") else HK_MINUTE_ROWS
        function("BABA" if function_name.startswith("us") else "09988")

    assert error.value.status_code == 502
    assert error.value.code == "AKSHARE_SCHEMA_ERROR"


def test_timed_out_call_keeps_its_slot_until_worker_finishes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    release = threading.Event()

    class FakeAKShare:
        @staticmethod
        def slow() -> str:
            assert release.wait(timeout=2)
            return "done"

    executor = ThreadPoolExecutor(max_workers=1)
    monkeypatch.setattr(akshare_upstream, "_executor", executor)
    monkeypatch.setattr(akshare_upstream, "_slots", threading.BoundedSemaphore(1))
    monkeypatch.setattr(akshare_upstream, "CALL_TIMEOUT_SECONDS", 0.01)
    try:
        with pytest.raises(SidecarError) as timeout:
            AK_RUN(FakeAKShare.slow)
        assert timeout.value.code == "AKSHARE_UPSTREAM_TIMEOUT"

        with pytest.raises(SidecarError) as busy:
            AK_RUN(FakeAKShare.slow)
        assert busy.value.code == "AKSHARE_POOL_BUSY"
    finally:
        release.set()
        executor.shutdown(wait=True)


def test_four_inflight_calls_reject_an_unbounded_fifth_queue_item(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    release = threading.Event()
    entered = 0
    condition = threading.Condition()

    class FakeAKShare:
        @staticmethod
        def slow() -> str:
            nonlocal entered
            with condition:
                entered += 1
                condition.notify_all()
            assert release.wait(timeout=2)
            return "done"

    executor = ThreadPoolExecutor(max_workers=4)
    monkeypatch.setattr(akshare_upstream, "_executor", executor)
    monkeypatch.setattr(akshare_upstream, "_slots", threading.BoundedSemaphore(4))
    monkeypatch.setattr(akshare_upstream, "CALL_TIMEOUT_SECONDS", 2)
    callers = ThreadPoolExecutor(max_workers=4)
    try:
        futures = [callers.submit(AK_RUN, FakeAKShare.slow) for _ in range(4)]
        deadline = time.monotonic() + 2
        with condition:
            while entered < 4 and time.monotonic() < deadline:
                condition.wait(timeout=0.05)
        assert entered == 4

        with pytest.raises(SidecarError) as busy:
            AK_RUN(FakeAKShare.slow)
        assert busy.value.code == "AKSHARE_POOL_BUSY"
        release.set()
        assert [future.result(timeout=2) for future in futures] == ["done"] * 4
    finally:
        release.set()
        callers.shutdown(wait=True)
        executor.shutdown(wait=True)
