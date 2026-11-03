"""Unit tests for ReportServer service namespaces."""

from __future__ import annotations

from pathlib import Path

import httpx
import pytest

from acugis_report.client import ReportServer
from acugis_report.errors import NotImplementedByServerError


def _transport(routes: dict) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        key = (request.method.upper(), request.url.path)
        if key not in routes:
            for (m, p), _spec in routes.items():
                if m == request.method.upper() and (request.url.path == p or request.url.path.endswith(p)):
                    key = (m, p)
                    break
        if key not in routes:
            return httpx.Response(404, json={"error": "not found", "path": request.url.path})
        spec = routes[key]
        if callable(spec):
            return spec(request)
        status = spec.get("status", 200)
        if "content" in spec:
            return httpx.Response(status, content=spec["content"], headers=spec.get("headers", {}))
        return httpx.Response(status, json=spec.get("json", {}), headers=spec.get("headers", {}))

    return httpx.MockTransport(handler)


def _client(routes: dict) -> ReportServer:
    return ReportServer("https://reports.example.com", token="jrs_test", transport=_transport(routes))


def test_auth_me_namespace() -> None:
    client = _client(
        {
            ("GET", "/api/v1/auth/me"): {
                "json": {
                    "id": 1,
                    "username": "alice",
                    "groups": ["Administrators"],
                    "is_admin": True,
                }
            }
        }
    )
    me = client.auth.me()
    assert me.username == "alice"
    assert client.me().username == "alice"  # flat still works


def test_reports_list_and_run(tmp_path: Path) -> None:
    routes = {
        ("GET", "/api/v1/reports"): {
            "json": {"items": [{"key": "Invoice", "name": "Invoice"}], "total": 1}
        },
        ("GET", "/reports/execute"): {
            "content": b"%PDF-1.4",
            "headers": {"content-type": "application/pdf"},
        },
    }
    client = _client(routes)
    listed = client.reports.list()
    assert listed.items[0].key == "Invoice"

    content, ctype = client.reports.run("Invoice", format="pdf", params={"city": "NYC"})
    assert content.startswith(b"%PDF")
    assert ctype and "pdf" in ctype

    out = tmp_path / "invoice.pdf"
    written, _ = client.reports.run_to_file("Invoice", out, format="pdf")
    assert out.read_bytes() == written == b"%PDF-1.4"


def test_folders_tree_and_resources_upload(tmp_path: Path) -> None:
    uploaded: dict = {}

    def upload_handler(request: httpx.Request) -> httpx.Response:
        uploaded["path"] = request.url.path
        return httpx.Response(200, json={"uuid": "abc-123", "ok": True})

    client = _client(
        {
            ("GET", "/api/v1/folders/tree"): {"json": {"tree": []}},
            ("POST", "/api/v1/resources"): upload_handler,
            ("GET", "/health"): {"json": {"status": "ok"}},
            ("GET", "/api/v1/datasources"): {"json": {"items": [{"name": "main"}]}},
            ("GET", "/api/v1/embedded-map/debug"): {"json": {"ok": True}},
            ("GET", "/reports/execute"): {
                "content": b"map-pdf",
                "headers": {"content-type": "application/pdf"},
            },
        }
    )
    assert client.folders.tree() == {"tree": []}
    assert client.health.check()["status"] == "ok"
    assert client.health()["status"] == "ok"  # callable back-compat
    assert client.datasources.list()[0]["name"] == "main"
    assert client.embedded_maps.debug("Parcels")["ok"] is True

    sample = tmp_path / "logo.png"
    sample.write_bytes(b"\x89PNG")
    result = client.resources.upload(sample, folder_id=0)
    assert result["uuid"] == "abc-123"
    assert uploaded["path"] == "/api/v1/resources"

    content, _ = client.map_reports.run("Parcels", geometry="POINT(0 0)", srid=4326)
    assert content == b"map-pdf"


def test_reserved_namespaces() -> None:
    client = _client({})
    for attr in ("users", "groups", "schedules", "jobs", "recipients"):
        svc = getattr(client, attr)
        with pytest.raises(NotImplementedByServerError):
            svc.list()
