"""Compatibility wrapper for the original AKShare live smoke command."""

from __future__ import annotations

import os
import sys

from marketdata_live_smoke import ENABLE_ENV, main


if __name__ == "__main__":
    # Keep the old opt-in name working for local operators while making the
    # generic smoke's network gate explicit.
    if os.environ.get(ENABLE_ENV) != "1":
        if os.environ.get("JFTRADE_AKSHARE_LIVE_SMOKE") == "1":
            os.environ[ENABLE_ENV] = "1"
    raise SystemExit(main(["--provider", "akshare", "--suite", "full", *sys.argv[1:]]))
