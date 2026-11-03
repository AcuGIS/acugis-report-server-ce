"""Embedded map commands."""

from __future__ import annotations

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output

app = typer.Typer(help="Embedded map diagnostics.", no_args_is_help=True)


@app.command("debug")
@handle_errors
def debug(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
    compile: bool = typer.Option(False, "--compile", help="Also compile JRXML."),
) -> None:
    """Diagnose embedded map configuration for a report.

    Examples:
      acugis-report embedded-map debug Invoice
      acugis-report embedded-map debug Invoice --compile --output json

    Related commands:
      acugis-report reports show
      acugis-report map features
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.embedded_map_debug(report, compile=compile))
