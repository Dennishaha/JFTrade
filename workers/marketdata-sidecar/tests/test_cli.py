from __future__ import annotations

import pytest

from marketdata_sidecar import main as sidecar_main


def test_parse_args_defaults_and_overrides() -> None:
    defaults = sidecar_main.parse_args([])
    assert defaults.host == "127.0.0.1"
    assert defaults.port == 7788

    custom = sidecar_main.parse_args(["--host", "::1", "--port", "7799"])
    assert custom.host == "::1"
    assert custom.port == 7799


@pytest.mark.parametrize("value", ["0", "65536", "not-a-port"])
def test_parse_args_rejects_invalid_ports(value: str) -> None:
    with pytest.raises(SystemExit):
        sidecar_main.parse_args(["--port", value])


def test_parse_args_reports_version(capsys: pytest.CaptureFixture[str]) -> None:
    with pytest.raises(SystemExit) as exit_info:
        sidecar_main.parse_args(["--version"])

    assert exit_info.value.code == 0
    assert capsys.readouterr().out == "marketdata-sidecar 0.2.0\n"


def test_main_passes_host_and_port_to_uvicorn(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[object, dict[str, object]]] = []

    class FakeUvicorn:
        @staticmethod
        def run(application: object, **options: object) -> None:
            calls.append((application, options))

    monkeypatch.setitem(__import__("sys").modules, "uvicorn", FakeUvicorn)
    sidecar_main.main(["--host", "127.0.0.1", "--port", "7790"])

    assert calls == [
        (
            sidecar_main.app,
            {
                "host": "127.0.0.1",
                "port": 7790,
                "loop": "asyncio",
                "http": "h11",
                "ws": "none",
                "lifespan": "on",
            },
        )
    ]
