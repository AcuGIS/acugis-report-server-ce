"""Minimal FastAPI example that runs a report and returns PDF bytes.

Environment:
  ACUGIS_REPORT_URL
  ACUGIS_REPORT_TOKEN

Run:
  pip install fastapi uvicorn
  uvicorn examples.fastapi.main:app --reload
"""

from __future__ import annotations

import os

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import Response

from acugis_report import ReportServer, ReportServerError

app = FastAPI(title="AcuGIS Report Server example")


def _client() -> ReportServer:
    url = os.environ.get("ACUGIS_REPORT_URL", "").strip()
    token = os.environ.get("ACUGIS_REPORT_TOKEN", "").strip()
    if not url or not token:
        raise HTTPException(status_code=500, detail="ACUGIS_REPORT_URL and ACUGIS_REPORT_TOKEN required")
    return ReportServer(url, token=token)


@app.get("/me")
def me():
    with _client() as client:
        user = client.auth.me()
        return user.model_dump() if hasattr(user, "model_dump") else user


@app.get("/reports")
def list_reports():
    with _client() as client:
        data = client.reports.list()
        return data.model_dump() if hasattr(data, "model_dump") else data


@app.get("/reports/{key}/pdf")
def run_report_pdf(key: str, invoice_id: int | None = Query(default=None, alias="InvoiceID")):
    params: dict = {}
    if invoice_id is not None:
        params["InvoiceID"] = invoice_id
    try:
        with _client() as client:
            content, content_type = client.reports.run(key, format="pdf", params=params or None)
    except ReportServerError as exc:
        raise HTTPException(status_code=exc.status_code or 502, detail=exc.message) from exc
    return Response(content=content, media_type=content_type or "application/pdf")
