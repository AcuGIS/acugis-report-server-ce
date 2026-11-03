# Building a Python integration

Expose a thin FastAPI service that runs the Invoice report through Report Server.

## Setup

```bash
pip install fastapi uvicorn acugis-report
# or: pip install ./sdk/python fastapi uvicorn
export ACUGIS_REPORT_URL=https://reports.example.com
export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx
```

## App

```python
import os
from fastapi import FastAPI, HTTPException
from fastapi.responses import Response
from acugis_report import ReportServer

app = FastAPI(title="Invoice bridge")
URL = os.environ["ACUGIS_REPORT_URL"]
TOKEN = os.environ["ACUGIS_REPORT_TOKEN"]


@app.get("/invoices/{invoice_id}.pdf")
def invoice_pdf(invoice_id: int):
    with ReportServer(url=URL, token=TOKEN) as client:
        try:
            pdf, content_type = client.Reports.Run(
                "Invoice",
                format="pdf",
                params={"InvoiceID": invoice_id},
            )
        except Exception as exc:  # narrow in production
            raise HTTPException(status_code=502, detail=str(exc)) from exc
    return Response(
        content=pdf,
        media_type=content_type or "application/pdf",
        headers={"Content-Disposition": f'inline; filename="invoice-{invoice_id}.pdf"'},
    )
```

Run:

```bash
uvicorn main:app --reload
```

Open `/invoices/123.pdf`.

## Notes

- Keep the API Token in a secret store; never embed it in frontend code
- For multi-tenant apps, map your auth to a Report Server user/token with least privilege
- Flat helper `client.run_report(...)` is equivalent if `Reports.Run` is not yet wired in your installed package version
