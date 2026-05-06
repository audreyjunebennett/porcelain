#!/usr/bin/env python3
"""Launch Locus (creative workspace server) from its new home with quiet warning filters."""
from __future__ import annotations

import os
import runpy
import warnings
from pathlib import Path


LOCUS_ROOT = Path(r"D:\Previously Claudia Core")
LOCUS_SCRIPT = LOCUS_ROOT / "Scripts" / "mobile_orchestrator_api.py"


def main() -> None:
    if not LOCUS_SCRIPT.exists():
        raise SystemExit(f"Missing Locus script: {LOCUS_SCRIPT}")

    warnings.filterwarnings(
        "ignore",
        message=r".*WindowsSelectorEventLoopPolicy.*",
        category=DeprecationWarning,
    )
    warnings.filterwarnings(
        "ignore",
        message=r".*set_event_loop_policy.*",
        category=DeprecationWarning,
    )

    os.chdir(LOCUS_ROOT)
    runpy.run_path(str(LOCUS_SCRIPT), run_name="__main__")


if __name__ == "__main__":
    main()
