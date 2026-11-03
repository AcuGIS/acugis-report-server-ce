"""Reserved command groups for upcoming Report Server APIs."""

from __future__ import annotations

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_output
from acugis_report.errors import NotImplementedByServerError

users_app = typer.Typer(help="User administration (reserved).", no_args_is_help=True)
groups_app = typer.Typer(help="Group administration (reserved).", no_args_is_help=True)
schedules_app = typer.Typer(help="Report schedules (reserved).", no_args_is_help=True)
jobs_app = typer.Typer(help="Scheduled job executions (reserved).", no_args_is_help=True)


def _stub(ctx: typer.Context, feature: str) -> None:
    out = get_output(ctx)
    exc = NotImplementedByServerError(feature)
    out.error(exc.message)
    raise typer.Exit(exc.exit_code)


@users_app.callback(invoke_without_command=True)
@handle_errors
def users_root(ctx: typer.Context) -> None:
    """Not yet implemented by Report Server."""
    if ctx.invoked_subcommand is None:
        _stub(ctx, "users")


@users_app.command("list")
@handle_errors
def users_list(ctx: typer.Context) -> None:
    """List users (when REST API exists)."""
    _stub(ctx, "users")


@groups_app.callback(invoke_without_command=True)
@handle_errors
def groups_root(ctx: typer.Context) -> None:
    """Not yet implemented by Report Server."""
    if ctx.invoked_subcommand is None:
        _stub(ctx, "groups")


@groups_app.command("list")
@handle_errors
def groups_list(ctx: typer.Context) -> None:
    """List groups (when REST API exists)."""
    _stub(ctx, "groups")


@schedules_app.callback(invoke_without_command=True)
@handle_errors
def schedules_root(ctx: typer.Context) -> None:
    """Not yet implemented by Report Server."""
    if ctx.invoked_subcommand is None:
        _stub(ctx, "schedules")


@schedules_app.command("list")
@handle_errors
def schedules_list(ctx: typer.Context) -> None:
    """List schedules (when REST API exists)."""
    _stub(ctx, "schedules")


@jobs_app.callback(invoke_without_command=True)
@handle_errors
def jobs_root(ctx: typer.Context) -> None:
    """Not yet implemented by Report Server."""
    if ctx.invoked_subcommand is None:
        _stub(ctx, "jobs")


@jobs_app.command("list")
@handle_errors
def jobs_list(ctx: typer.Context) -> None:
    """List jobs (when REST API exists)."""
    _stub(ctx, "jobs")
