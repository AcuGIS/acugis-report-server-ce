"""Shared CLI context helpers."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import typer

from acugis_report.client import ReportServer
from acugis_report.config import ConfigStore, Profile, resolve_connection
from acugis_report.cli.output import Output
from acugis_report.errors import ValidationError


@dataclass
class AppState:
    profile: Optional[str] = None
    output_format: str = "table"
    verbose: bool = False
    debug: bool = False
    no_color: bool = False
    config_path: Optional[str] = None


def get_state(ctx: typer.Context) -> AppState:
    state = ctx.ensure_object(AppState)
    return state


def get_output(ctx: typer.Context) -> Output:
    state = get_state(ctx)
    return Output(
        format=state.output_format,
        no_color=state.no_color,
        verbose=state.verbose,
        debug=state.debug,
    )


def get_store(ctx: typer.Context) -> ConfigStore:
    state = get_state(ctx)
    from pathlib import Path

    return ConfigStore(Path(state.config_path) if state.config_path else None)


def get_client(ctx: typer.Context, *, require_token: bool = True) -> ReportServer:
    state = get_state(ctx)
    store = get_store(ctx)
    name, profile = resolve_connection(profile=state.profile, store=store)
    if not profile.server_url:
        raise ValidationError(
            "No server URL configured. Run `acugis-report login` or set ACUGIS_REPORT_URL."
        )
    if require_token and not profile.token:
        raise ValidationError(
            "Not authenticated. Run `acugis-report login` or set ACUGIS_REPORT_TOKEN."
        )
    return ReportServer(profile.server_url, token=profile.token or None, verify=profile.verify_tls)


def current_profile(ctx: typer.Context) -> tuple[str, Profile]:
    state = get_state(ctx)
    store = get_store(ctx)
    return resolve_connection(profile=state.profile, store=store)
