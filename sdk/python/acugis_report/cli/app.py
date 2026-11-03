"""acugis-report CLI entrypoint."""

from __future__ import annotations

import traceback
from typing import Optional

import typer

from acugis_report import __version__
from acugis_report.cli.ctx import AppState, get_output
from acugis_report.cli.commands import (
    auth,
    datasources,
    doctor,
    embedded_map,
    folders,
    future,
    map_cmds,
    profile,
    reports,
    resources,
)
from acugis_report.errors import EXIT_GENERAL, ReportServerError

app = typer.Typer(
    name="acugis-report",
    help="AcuGIS Report Server CLI — automate reports, folders, and resources.",
    no_args_is_help=True,
    rich_markup_mode="rich",
    context_settings={"help_option_names": ["-h", "--help"]},
)

app.add_typer(profile.app, name="profile")
app.add_typer(reports.app, name="reports")
app.add_typer(folders.app, name="folders")
app.add_typer(resources.app, name="resources")
app.add_typer(datasources.app, name="datasources")
app.add_typer(map_cmds.app, name="map")
app.add_typer(map_cmds.map_report_app, name="map-report")
app.add_typer(embedded_map.app, name="embedded-map")
app.add_typer(future.users_app, name="users")
app.add_typer(future.groups_app, name="groups")
app.add_typer(future.schedules_app, name="schedules")
app.add_typer(future.jobs_app, name="jobs")

# Top-level auth + doctor
app.command("login")(auth.login)
app.command("logout")(auth.logout)
app.command("whoami")(auth.whoami)
app.command("doctor")(doctor.doctor_cmd)


def _version_callback(value: bool) -> None:
    if value:
        typer.echo(f"acugis-report {__version__}")
        raise typer.Exit()


@app.callback()
def main_callback(
    ctx: typer.Context,
    profile_name: Optional[str] = typer.Option(
        None,
        "--profile",
        help="Named connection profile.",
        envvar="ACUGIS_REPORT_PROFILE",
    ),
    output: str = typer.Option(
        "table",
        "--output",
        "-o",
        help="Output format: table, json, or yaml.",
        case_sensitive=False,
    ),
    verbose: bool = typer.Option(False, "--verbose", "-v", help="Verbose logging."),
    debug: bool = typer.Option(False, "--debug", help="Show stack traces on errors."),
    no_color: bool = typer.Option(False, "--no-color", help="Disable colored output."),
    config: Optional[str] = typer.Option(
        None,
        "--config",
        help="Path to config.toml (overrides default location).",
        hidden=True,
    ),
    version: Optional[bool] = typer.Option(
        None,
        "--version",
        callback=_version_callback,
        is_eager=True,
        help="Show version and exit.",
    ),
) -> None:
    """AcuGIS Report Server command-line interface.

    Related commands:
      acugis-report login
      acugis-report doctor
      acugis-report reports list
    """
    fmt = output.lower()
    if fmt not in ("table", "json", "yaml"):
        raise typer.BadParameter("Must be table, json, or yaml", param_hint="--output")
    state = ctx.ensure_object(AppState)
    state.profile = profile_name
    state.output_format = fmt
    state.verbose = verbose
    state.debug = debug
    state.no_color = no_color
    state.config_path = config


def main() -> None:
    try:
        app()
    except ReportServerError as exc:
        # Typer may already handle; this is for direct invocation edge cases
        out = Output_fallback()
        out.error(exc.message)
        raise SystemExit(exc.exit_code) from None
    except Exception:
        raise


def Output_fallback():
    from acugis_report.cli.output import Output

    return Output()


def run() -> int:
    """Entry used by tests: invoke Typer and map SDK errors to exit codes."""
    try:
        app(standalone_mode=False)
        return 0
    except typer.Exit as exc:
        return int(exc.exit_code or 0)
    except ReportServerError as exc:
        try:
            # best-effort stderr
            import sys

            print(f"Error: {exc.message}", file=sys.stderr)
            if "--debug" in sys.argv:
                traceback.print_exc()
        except Exception:  # noqa: BLE001
            pass
        return exc.exit_code
    except Exception as exc:  # noqa: BLE001
        import sys

        print(f"Error: {exc}", file=sys.stderr)
        if "--debug" in sys.argv:
            traceback.print_exc()
        return EXIT_GENERAL


if __name__ == "__main__":
    raise SystemExit(run())
