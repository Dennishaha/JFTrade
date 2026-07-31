"""Executable module entry point for the yfinance sidecar."""

from __future__ import annotations

import sys


def _run() -> None:
    # Keep the frozen version probe independent from FastAPI/pandas/yfinance
    # imports. PyInstaller one-file extraction is already cold-start bound;
    # importing the full route graph here made --version needlessly expensive.
    if "--version" in sys.argv[1:]:
        from yfinance_sidecar import __version__

        print(f"yfinance-sidecar {__version__}")
        return

    from yfinance_sidecar.main import main

    main()


if __name__ == "__main__":
    _run()
