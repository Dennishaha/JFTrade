"""Executable module entry point for the market-data sidecar."""

from __future__ import annotations

import sys


def _run() -> None:
    # Keep the frozen version probe independent from FastAPI/pandas/yfinance
    # imports. Importing the full route graph here makes --version needlessly
    # expensive even though the release helper is a PyInstaller onedir bundle.
    if "--version" in sys.argv[1:]:
        from marketdata_sidecar import __version__

        print(f"marketdata-sidecar {__version__}")
        return

    from marketdata_sidecar.main import main

    main()


if __name__ == "__main__":
    _run()
