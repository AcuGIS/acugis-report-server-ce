"""Health / doctor command."""

from __future__ import annotations

import typer
from rich.console import Console
from rich.text import Text

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output, get_state
from acugis_report.errors import ValidationError


@handle_errors
def doctor_cmd(ctx: typer.Context) -> None:
    """Check Report Server health and dependencies.

    Aggregates /health, /ready, diagnostics, and Jasper status.

    Examples:
      acugis-report doctor
      acugis-report doctor --output json

    Related commands:
      acugis-report whoami
      acugis-report login
    """
    out = get_output(ctx)
    state = get_state(ctx)
    try:
        client = get_client(ctx, require_token=False)
    except ValidationError:
        # Allow doctor with only URL configured / env
        from acugis_report.cli.ctx import current_profile
        from acugis_report.client import ReportServer

        _, profile = current_profile(ctx)
        if not profile.server_url:
            raise
        client = ReportServer(profile.server_url, token=profile.token or None, verify=profile.verify_tls)

    result = client.doctor()
    client.close()

    if state.output_format in ("json", "yaml"):
        out.emit(result)
        if not result.get("ok"):
            raise typer.Exit(6)
        return

    console = out.console
    checks = result.get("checks") or {}
    ready = checks.get("ready") or {}
    ready_checks = ready.get("checks") if isinstance(ready, dict) else {}

    def _ok(label: str, good: bool, detail: str = "") -> None:
        mark = Text("✓ ", style="green") if good else Text("✗ ", style="red")
        line = Text(label, style="bold")
        console.print(mark, line, detail and f"  {detail}" or "", sep="")

    health = checks.get("health") or {}
    _ok("Server", isinstance(health, dict) and health.get("status") == "ok", health.get("service", ""))

    if isinstance(ready_checks, dict) and ready_checks:
        for name, value in ready_checks.items():
            good = _is_good(value)
            label = {
                "postgres": "Database",
                "database": "Database",
                "filesystem": "Resources",
                "jasper": "Jasper",
            }.get(str(name).lower(), str(name).title())
            _ok(label, good, _detail(value))
    else:
        ready_ok = isinstance(ready, dict) and ready.get("status") in ("ready", "ok")
        _ok("Database", ready_ok)
        _ok("Jasper", ready_ok)
        _ok("Resources", ready_ok)

    diag = checks.get("diagnostics") or {}
    if isinstance(diag, dict) and "error" not in diag:
        _ok("Explorer", True, "diagnostics ok")
        _ok("Map Reports", True)
    else:
        _ok("Explorer", False, _detail(diag))
        _ok("Map Reports", False)

    jasper = checks.get("jasper") or {}
    if isinstance(jasper, dict) and "error" not in jasper:
        # already covered via ready; show extra if present
        pass

    warnings = result.get("warnings") or []
    if warnings:
        console.print()
        console.print(Text("Warnings", style="bold yellow"))
        for w in warnings:
            console.print(Text(f"  ! {w}", style="yellow"))

    if not result.get("ok"):
        raise typer.Exit(6)


def _is_good(value: object) -> bool:
    if value is True:
        return True
    if isinstance(value, str):
        return value.lower() in ("ok", "ready", "pass", "true", "up")
    if isinstance(value, dict):
        status = value.get("status", value.get("ok"))
        return _is_good(status) if status is not None else False
    return False


def _detail(value: object) -> str:
    if isinstance(value, dict):
        if "error" in value:
            return str(value["error"])
        if "status" in value:
            return str(value["status"])
        return ""
    return str(value) if value is not None else ""
