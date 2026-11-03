"""Profile management commands."""

from __future__ import annotations

from typing import Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_output, get_store
from acugis_report.config import Profile
from acugis_report.errors import ValidationError

app = typer.Typer(help="Manage named server profiles.", no_args_is_help=True)


@app.command("add")
@handle_errors
def add(
    ctx: typer.Context,
    name: str = typer.Argument(..., help="Profile name."),
    server: Optional[str] = typer.Option(None, "--server", "-s", help="Server URL."),
) -> None:
    """Add a named profile.

    Examples:
      acugis-report profile add production --server https://reports.example.com/go
      acugis-report profile add staging

    Related commands:
      acugis-report profile use
      acugis-report profile list
      acugis-report login
    """
    store = get_store(ctx)
    out = get_output(ctx)
    active, profiles = store.list_profiles()
    if name in profiles and profiles[name].server_url:
        raise ValidationError(f"Profile {name!r} already exists")
    store.upsert_profile(name, server_url=(server or "").rstrip("/"))
    out.success(f"Added profile {name}")


@app.command("use")
@handle_errors
def use(
    ctx: typer.Context,
    name: str = typer.Argument(..., help="Profile name to activate."),
) -> None:
    """Set the active profile.

    Examples:
      acugis-report profile use production

    Related commands:
      acugis-report profile list
      acugis-report login --profile production
    """
    store = get_store(ctx)
    out = get_output(ctx)
    _, profiles = store.list_profiles()
    if name not in profiles:
        store.upsert_profile(name)
    store.set_active(name)
    out.success(f"Using profile {name}")


@app.command("list")
@handle_errors
def list_profiles(ctx: typer.Context) -> None:
    """List configured profiles.

    Examples:
      acugis-report profile list
      acugis-report profile list --output json

    Related commands:
      acugis-report profile add
      acugis-report profile use
    """
    store = get_store(ctx)
    out = get_output(ctx)
    active, profiles = store.list_profiles()
    rows = []
    for name, p in profiles.items():
        rows.append(
            {
                "name": name,
                "active": name == active,
                "server_url": p.server_url,
                "authenticated": bool(p.token),
                "token_name": p.token_name or "",
            }
        )
    out.emit(rows, columns=["name", "active", "server_url", "authenticated", "token_name"])


@app.command("remove")
@handle_errors
def remove(
    ctx: typer.Context,
    name: str = typer.Argument(..., help="Profile name."),
) -> None:
    """Remove a profile.

    Examples:
      acugis-report profile remove staging

    Related commands:
      acugis-report profile list
    """
    store = get_store(ctx)
    out = get_output(ctx)
    store.delete_profile(name)
    out.success(f"Removed profile {name}")


@app.command("show")
@handle_errors
def show(
    ctx: typer.Context,
    name: Optional[str] = typer.Argument(None, help="Profile name (default: active)."),
) -> None:
    """Show profile details (token redacted).

    Examples:
      acugis-report profile show
      acugis-report profile show production
    """
    store = get_store(ctx)
    out = get_output(ctx)
    active, profiles = store.list_profiles()
    target = name or active
    p = profiles.get(target) or Profile()
    out.emit(
        {
            "name": target,
            "active": target == active,
            "server_url": p.server_url,
            "auth_type": p.auth_type,
            "token_name": p.token_name,
            "token_id": p.token_id,
            "token": ("***" + p.token[-4:]) if p.token and len(p.token) > 4 else ("set" if p.token else ""),
            "verify_tls": p.verify_tls,
        }
    )
