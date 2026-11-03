"""Minimal console example for the AcuGIS Report Server Python SDK.

Reads ACUGIS_REPORT_URL and ACUGIS_REPORT_TOKEN from the environment.
Calls /api/v1/auth/me, lists reports, runs the first report (or REPORT_KEY),
and writes PDF output to disk.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

from acugis_report import ReportServer


def main() -> int:
    url = os.environ.get("ACUGIS_REPORT_URL", "").strip()
    token = os.environ.get("ACUGIS_REPORT_TOKEN", "").strip()
    if not url or not token:
        print("Set ACUGIS_REPORT_URL and ACUGIS_REPORT_TOKEN", file=sys.stderr)
        return 1

    with ReportServer(url, token=token) as client:
        me = client.auth.me()
        print(f"Authenticated as: {me.username}")

        reports = client.reports.list()
        items = getattr(reports, "items", None) or getattr(reports, "reports", None) or []
        print(f"Reports: {len(items)}")

        key = os.environ.get("REPORT_KEY", "").strip()
        if not key and items:
            first = items[0]
            key = first.get("key") if isinstance(first, dict) else getattr(first, "key", "")
        if not key:
            print("No report key available (set REPORT_KEY)", file=sys.stderr)
            return 1

        out = Path(os.environ.get("REPORT_OUT", f"{key}.pdf"))
        client.reports.run_to_file(key, out, format="pdf")
        print(f"Wrote {out.resolve()}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
