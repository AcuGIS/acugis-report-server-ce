"""Authentication commands."""

from __future__ import annotations

from typing import Optional

import typer

from acugis_report.client import ReportServer
from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import current_profile, get_client, get_output, get_state, get_store
from acugis_report.errors import ValidationError


@handle_errors
def login(
    ctx: typer.Context,
    token: bool = typer.Option(
        False,
        "--token",
        help="Log in with an existing API token (jrs_…). Prompts: Server URL, Paste API Token.",
    ),
    server: Optional[str] = typer.Option(
        None,
        "--server",
        "-s",
        help="Report Server base URL, e.g. https://reports.example.com/go",
    ),
    username: Optional[str] = typer.Option(None, "--username", "-u", help="Username (password login only)."),
    password: Optional[str] = typer.Option(
        None,
        "--password",
        "-p",
        help="Password (prefer prompting; avoid shell history).",
        hide_input=True,
    ),
    token_value: Optional[str] = typer.Option(
        None,
        "--token-value",
        help="API token string for non-interactive token login (use with --token).",
        hide_input=True,
    ),
    verify_tls: bool = typer.Option(True, "--verify-tls/--no-verify-tls", help="Verify TLS certificates."),
) -> None:
    """Authenticate and store a bearer token locally (never stores passwords).

    Two modes:

    1. Password login (default) — prompts for Server URL, Username, Password;
       creates a personal access token and saves it.

    2. Token login — use --token; prompts for Server URL and Paste API Token.
       Create a token in the UI: Profile → API Tokens.

    Examples:
      # Interactive token login
      acugis-report login --token
      acugis-report login --token --server https://reports.example.com/go

      # Non-interactive token login (CI / scripts)
      acugis-report login --token --server https://reports.example.com/go \\
        --token-value jrs_xxxxxxxxx

      # Or skip login and use env vars
      export ACUGIS_REPORT_URL=https://reports.example.com/go
      export ACUGIS_REPORT_TOKEN=jrs_xxxxxxxxx

      # Password login
      acugis-report login
      acugis-report login --server https://reports.example.com/go -u alice

    Related commands:
      acugis-report logout
      acugis-report whoami
      acugis-report profile use
    """
    state = get_state(ctx)
    store = get_store(ctx)
    profile_name, existing = current_profile(ctx)
    out = get_output(ctx)

    url = server or existing.server_url
    if not url:
        url = typer.prompt("Server URL")
    url = url.rstrip("/")

    if token or token_value:
        tok = token_value
        if not tok:
            tok = typer.prompt("Paste API Token", hide_input=True)
        if not tok.strip():
            raise ValidationError("Token is required")
        # Validate by calling /me
        with ReportServer(url, token=tok.strip(), verify=verify_tls) as client:
            me = client.me()
        store.upsert_profile(
            profile_name,
            server_url=url,
            token=tok.strip(),
            token_name="",
            token_id=None,
            auth_type="token",
            verify_tls=verify_tls,
        )
        store.set_active(profile_name)
        out.success(f"Logged in as {me.username or 'unknown'} ({profile_name})")
        return

    user = username or typer.prompt("Username")
    pwd = password or typer.prompt("Password", hide_input=True)
    with ReportServer(url, verify=verify_tls) as client:
        created = client.login_and_create_cli_token(user, pwd)
    store.upsert_profile(
        profile_name,
        server_url=url,
        token=created.token,
        token_name=created.name or "acugis-report-cli",
        token_id=created.id,
        auth_type="password",
        verify_tls=verify_tls,
    )
    store.set_active(profile_name)
    out.success(f"Logged in as {user} ({profile_name})")
    if created.message:
        out.info(created.message)


@handle_errors
def logout(ctx: typer.Context) -> None:
    """Clear stored credentials for the active profile.

    Examples:
      acugis-report logout
      acugis-report --profile staging logout

    Related commands:
      acugis-report login
      acugis-report whoami
    """
    state = get_state(ctx)
    store = get_store(ctx)
    profile_name, profile = current_profile(ctx)
    out = get_output(ctx)

    if profile.server_url and profile.token:
        try:
            with ReportServer(profile.server_url, token=profile.token, verify=profile.verify_tls) as client:
                # Best-effort revoke CLI token if we know the id
                if profile.token_id:
                    try:
                        client.delete_token(profile.token_id)
                    except Exception:  # noqa: BLE001
                        pass
                client.logout()
        except Exception:  # noqa: BLE001
            pass

    store.clear_credentials(profile_name)
    out.success(f"Logged out ({profile_name})")


@handle_errors
def whoami(ctx: typer.Context) -> None:
    """Show the current authenticated identity.

    Displays username, groups, authentication type, server, token name, and expiration.

    Examples:
      acugis-report whoami
      acugis-report whoami --output json

    Related commands:
      acugis-report login
      acugis-report logout
    """
    out = get_output(ctx)
    profile_name, profile = current_profile(ctx)
    client = get_client(ctx)
    me = client.me()

    token_name = profile.token_name or ""
    expires_at = ""
    if profile.token_id:
        try:
            for t in client.list_tokens():
                if t.id == profile.token_id:
                    token_name = t.name or token_name
                    expires_at = t.expires_at or "never"
                    break
        except Exception:  # noqa: BLE001
            pass
    elif profile.token:
        # Match by prefix when possible
        prefix = profile.token[:12]
        try:
            for t in client.list_tokens():
                if t.token_prefix and prefix.startswith(t.token_prefix):
                    token_name = t.name or token_name
                    expires_at = t.expires_at or "never"
                    break
        except Exception:  # noqa: BLE001
            pass

    payload = {
        "username": me.username,
        "groups": me.groups,
        "authentication_type": profile.auth_type or "token",
        "server": profile.server_url,
        "profile": profile_name,
        "token_name": token_name or None,
        "expiration": expires_at or None,
        "is_admin": me.is_admin,
        "email": me.email,
    }
    out.emit(payload, columns=["username", "groups", "authentication_type", "server", "token_name", "expiration"])
