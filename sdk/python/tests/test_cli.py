"""CLI and SDK tests with mocked HTTP."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import patch

import httpx
import pytest
from typer.testing import CliRunner

from acugis_report.cli.app import app
from acugis_report.client import ReportServer
from acugis_report.config import AppConfig, ConfigStore, Profile
from acugis_report.errors import (
    EXIT_AUTH,
    EXIT_GENERAL,
    EXIT_NOT_FOUND,
    EXIT_VALIDATION,
    AuthenticationError,
    NotFoundError,
    ValidationError,
)


def _transport(routes: dict) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        method = request.method.upper()
        path = request.url.path
        key = (method, path)
        if key not in routes:
            # Strip common mount prefixes (e.g. /go)
            for prefix in ("/go",):
                if path.startswith(prefix + "/") or path == prefix:
                    stripped = path[len(prefix) :] or "/"
                    alt = (method, stripped)
                    if alt in routes:
                        key = alt
                        break
            else:
                for (m, p), _spec in routes.items():
                    if m == method and (path == p or path.endswith(p)):
                        key = (m, p)
                        break
        if key not in routes:
            return httpx.Response(404, json={"error": "not found", "path": path})
        spec = routes[key]
        if callable(spec):
            return spec(request)
        status = spec.get("status", 200)
        if "content" in spec:
            return httpx.Response(
                status,
                content=spec["content"],
                headers=spec.get("headers", {}),
            )
        return httpx.Response(status, json=spec.get("json", {}))

    return httpx.MockTransport(handler)


@pytest.fixture
def runner() -> CliRunner:
    return CliRunner()


@pytest.fixture
def cfg(tmp_path: Path) -> Path:
    path = tmp_path / "config.toml"
    ConfigStore(path).save(
        AppConfig(
            active_profile="default",
            profiles={
                "default": Profile(
                    server_url="https://reports.example.com/go",
                    token="jrs_testtoken1234567890",
                    token_name="test-cli",
                    token_id=1,
                    auth_type="token",
                )
            },
        )
    )
    return path


def test_login_with_token(runner: CliRunner, tmp_path: Path) -> None:
    cfg = tmp_path / "config.toml"
    routes = {
        ("GET", "/api/v1/auth/me"): {
            "json": {
                "id": 1,
                "username": "alice",
                "groups": ["Administrators"],
                "is_admin": True,
            }
        }
    }
    transport = _transport(routes)

    def fake_client(url, token=None, verify=True, **kwargs):
        return ReportServer(url, token=token, verify=verify, transport=transport)

    with patch("acugis_report.cli.commands.auth.ReportServer", side_effect=fake_client):
        result = runner.invoke(
            app,
            [
                "--config",
                str(cfg),
                "login",
                "--token",
                "--server",
                "https://reports.example.com/go",
                "--token-value",
                "jrs_abcdef",
            ],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    store = ConfigStore(cfg)
    _, profile = store.get_profile()
    assert profile.token == "jrs_abcdef"
    assert profile.server_url == "https://reports.example.com/go"


def test_login_password_creates_pat(runner: CliRunner, tmp_path: Path) -> None:
    cfg = tmp_path / "config.toml"
    routes = {
        ("POST", "/api/v1/auth/login"): {"json": {"ok": True}},
        ("POST", "/api/v1/auth/tokens"): {
            "status": 201,
            "json": {
                "token": "jrs_newpat",
                "id": 9,
                "name": "acugis-report-cli",
                "message": "shown once",
            },
        },
    }
    transport = _transport(routes)

    def fake_client(url, token=None, verify=True, **kwargs):
        return ReportServer(url, token=token, verify=verify, transport=transport)

    with patch("acugis_report.cli.commands.auth.ReportServer", side_effect=fake_client):
        result = runner.invoke(
            app,
            [
                "--config",
                str(cfg),
                "login",
                "--server",
                "https://reports.example.com/go",
                "--username",
                "alice",
                "--password",
                "secret",
            ],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    _, profile = ConfigStore(cfg).get_profile()
    assert profile.token == "jrs_newpat"
    assert profile.token_id == 9
    assert "password" not in cfg.read_text(encoding="utf-8").lower() or "secret" not in cfg.read_text(
        encoding="utf-8"
    )


def test_whoami_json(runner: CliRunner, cfg: Path) -> None:
    routes = {
        ("GET", "/api/v1/auth/me"): {
            "json": {
                "id": 1,
                "username": "alice",
                "email": "a@example.com",
                "groups": ["Administrators"],
                "is_admin": True,
            }
        },
        ("GET", "/api/v1/auth/tokens"): {
            "json": {
                "tokens": [
                    {
                        "id": 1,
                        "name": "test-cli",
                        "expires_at": None,
                        "token_prefix": "jrs_testtoken",
                    }
                ]
            }
        },
    }
    transport = _transport(routes)

    def fake_get_client(ctx, require_token=True):
        return ReportServer(
            "https://reports.example.com/go",
            token="jrs_testtoken1234567890",
            transport=transport,
        )

    with patch("acugis_report.cli.commands.auth.get_client", fake_get_client):
        result = runner.invoke(
            app,
            ["--config", str(cfg), "--output", "json", "whoami"],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    data = json.loads(result.stdout)
    assert data["username"] == "alice"
    assert data["server"] == "https://reports.example.com/go"


def test_reports_list_json(runner: CliRunner, cfg: Path) -> None:
    routes = {
        ("GET", "/api/v1/reports"): {
            "json": {
                "items": [
                    {
                        "key": "Invoice",
                        "name": "Invoice",
                        "datasource": "postgres",
                        "type": "jasper",
                        "has_thumbnail": False,
                    }
                ],
                "count": 1,
            }
        }
    }
    transport = _transport(routes)

    with patch(
        "acugis_report.cli.commands.reports.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="t", transport=transport
        ),
    ):
        result = runner.invoke(
            app,
            ["--config", str(cfg), "--output", "json", "reports", "list"],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    data = json.loads(result.stdout)
    assert data["count"] == 1
    assert data["items"][0]["key"] == "Invoice"


def test_reports_run_writes_file(runner: CliRunner, cfg: Path, tmp_path: Path) -> None:
    pdf = b"%PDF-1.4 mock"
    routes = {
        ("GET", "/reports/execute"): {"content": pdf, "headers": {"content-type": "application/pdf"}},
    }
    transport = _transport(routes)
    out = tmp_path / "invoice.pdf"

    with patch(
        "acugis_report.cli.commands.reports.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="t", transport=transport
        ),
    ):
        result = runner.invoke(
            app,
            [
                "--config",
                str(cfg),
                "reports",
                "run",
                "Invoice",
                "--param",
                "InvoiceID=123",
                "--output",
                str(out),
            ],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    assert out.read_bytes() == pdf


def test_reports_publish(runner: CliRunner, cfg: Path, tmp_path: Path) -> None:
    jrxml = tmp_path / "report.jrxml"
    jrxml.write_text("<jasperReport/>", encoding="utf-8")
    routes = {
        ("POST", "/api/v1/reports"): {
            "status": 201,
            "json": {"ok": True, "key": "report", "message": "created"},
        }
    }
    transport = _transport(routes)

    with patch(
        "acugis_report.cli.commands.reports.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="t", transport=transport
        ),
    ):
        result = runner.invoke(
            app,
            [
                "--config",
                str(cfg),
                "--output",
                "json",
                "reports",
                "publish",
                str(jrxml),
                "--data-source",
                "postgres",
            ],
            catch_exceptions=False,
        )
    assert result.exit_code == 0, result.output
    assert "report" in result.stdout


def test_folders_browse(runner: CliRunner, cfg: Path) -> None:
    routes = {
        ("GET", "/api/v1/folders/browse"): {
            "json": {"folder": {"id": 0, "name": "Home"}, "folders": [], "items": []}
        }
    }
    transport = _transport(routes)
    with patch(
        "acugis_report.cli.commands.folders.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="t", transport=transport
        ),
    ):
        result = runner.invoke(
            app,
            ["--config", str(cfg), "--output", "json", "folders", "browse"],
            catch_exceptions=False,
        )
    assert result.exit_code == 0
    data = json.loads(result.stdout)
    assert data["folder"]["name"] == "Home"


# Data Explorer and dashboard CLI commands are not part of this Open Source
# client. Those routes require Report Server Professional.


def test_invalid_command(runner: CliRunner, cfg: Path) -> None:
    result = runner.invoke(app, ["--config", str(cfg), "does-not-exist"])
    assert result.exit_code != 0


def test_users_not_implemented(runner: CliRunner, cfg: Path) -> None:
    result = runner.invoke(app, ["--config", str(cfg), "users", "list"], catch_exceptions=False)
    assert result.exit_code == EXIT_GENERAL
    assert "Not yet implemented" in result.output


def test_exit_code_auth(runner: CliRunner, cfg: Path) -> None:
    routes = {("GET", "/api/v1/auth/me"): {"status": 401, "json": {"error": "unauthorized"}}}
    transport = _transport(routes)
    with patch(
        "acugis_report.cli.commands.auth.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="bad", transport=transport
        ),
    ):
        result = runner.invoke(app, ["--config", str(cfg), "whoami"], catch_exceptions=False)
    assert result.exit_code == EXIT_AUTH


def test_exit_code_not_found(runner: CliRunner, cfg: Path) -> None:
    routes = {("GET", "/api/v1/reports/Missing"): {"status": 404, "json": {"error": "not found"}}}
    transport = _transport(routes)
    with patch(
        "acugis_report.cli.commands.reports.get_client",
        lambda ctx, require_token=True: ReportServer(
            "https://reports.example.com/go", token="t", transport=transport
        ),
    ):
        result = runner.invoke(
            app,
            ["--config", str(cfg), "reports", "show", "Missing"],
            catch_exceptions=False,
        )
    assert result.exit_code == EXIT_NOT_FOUND


def test_sdk_validation_format() -> None:
    client = ReportServer("https://example.com", token="t", transport=_transport({}))
    with pytest.raises(ValidationError):
        client.run_report("X", format="exe")


def test_sdk_auth_error() -> None:
    client = ReportServer(
        "https://example.com",
        token="t",
        transport=_transport({("GET", "/api/v1/auth/me"): {"status": 401, "json": {"error": "nope"}}}),
    )
    with pytest.raises(AuthenticationError):
        client.me()


def test_profile_commands(runner: CliRunner, tmp_path: Path) -> None:
    cfg = tmp_path / "config.toml"
    result = runner.invoke(
        app,
        ["--config", str(cfg), "profile", "add", "production", "--server", "https://prod.example/go"],
        catch_exceptions=False,
    )
    assert result.exit_code == 0
    result = runner.invoke(app, ["--config", str(cfg), "profile", "use", "production"], catch_exceptions=False)
    assert result.exit_code == 0
    result = runner.invoke(
        app, ["--config", str(cfg), "--output", "json", "profile", "list"], catch_exceptions=False
    )
    assert result.exit_code == 0
    data = json.loads(result.stdout)
    assert any(p["name"] == "production" and p["active"] for p in data)


def test_multi_param_parsing() -> None:
    from acugis_report.cli.commands.reports import _parse_params

    params = _parse_params(["region=West", "region=North", "id=1"], None)
    assert params["region"] == ["West", "North"]
    assert params["id"] == "1"


def test_logout_clears_token(runner: CliRunner, cfg: Path) -> None:
    routes = {
        ("DELETE", "/api/v1/auth/tokens/1"): {"status": 204},
        ("POST", "/api/v1/auth/logout"): {"json": {"ok": True}},
    }
    transport = _transport(routes)

    def fake_rs(url, token=None, verify=True, **kwargs):
        return ReportServer(url, token=token, verify=verify, transport=transport)

    with patch("acugis_report.cli.commands.auth.ReportServer", side_effect=fake_rs):
        result = runner.invoke(app, ["--config", str(cfg), "logout"], catch_exceptions=False)
    assert result.exit_code == 0
    _, profile = ConfigStore(cfg).get_profile()
    assert profile.token == ""
