"""Map report commands."""

from __future__ import annotations

from pathlib import Path
from typing import Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output
from acugis_report.cli.output import write_binary
from acugis_report.models import REPORT_FORMATS

app = typer.Typer(help="Map report metadata and features. Requires Report Server Professional.", no_args_is_help=True)
map_report_app = typer.Typer(help="Run map reports with geometry. Requires Report Server Professional.", no_args_is_help=True)


@app.command("features")
@handle_errors
def features(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Map report key."),
) -> None:
    """Fetch GeoJSON features for a map report.

    Examples:
      acugis-report map features Parcels --output json

    Related commands:
      acugis-report map extent
      acugis-report map-report run
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.map_features(report))


@app.command("extent")
@handle_errors
def extent(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Map report key."),
    srid: Optional[int] = typer.Option(None, "--srid"),
    geometry: Optional[str] = typer.Option(None, "--geometry", help="WKT geometry."),
    geometry_file: Optional[Path] = typer.Option(None, "--geometry-file", exists=True),
    xmin: Optional[float] = typer.Option(None, "--xmin"),
    ymin: Optional[float] = typer.Option(None, "--ymin"),
    xmax: Optional[float] = typer.Option(None, "--xmax"),
    ymax: Optional[float] = typer.Option(None, "--ymax"),
) -> None:
    """Build spatial filter parameters for a map report.

    Examples:
      acugis-report map extent Parcels --srid 2193 --geometry "POLYGON((...))"
      acugis-report map extent Parcels --xmin 1 --ymin 2 --xmax 3 --ymax 4 --srid 4326

    Related commands:
      acugis-report map features
      acugis-report map-report run
    """
    geom = geometry
    if geometry_file:
        geom = geometry_file.read_text(encoding="utf-8").strip()
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(
        client.map_extent(
            report,
            srid=srid,
            geometry=geom,
            xmin=xmin,
            ymin=ymin,
            xmax=xmax,
            ymax=ymax,
        )
    )


@app.command("columns")
@handle_errors
def columns(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Map report key."),
) -> None:
    """Show column/geometry metadata for a map report.

    Examples:
      acugis-report map columns Parcels

    Related commands:
      acugis-report map features
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.map_columns(report))


@map_report_app.command("run")
@handle_errors
def map_report_run(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Map report key."),
    geometry: Optional[str] = typer.Option(None, "--geometry", help="WKT geometry string."),
    geometry_file: Optional[Path] = typer.Option(None, "--geometry-file", exists=True, help="File containing WKT."),
    srid: Optional[int] = typer.Option(None, "--srid"),
    format: str = typer.Option("pdf", "--format", "-f", help=f"Formats: {', '.join(REPORT_FORMATS)}"),
    output: Optional[Path] = typer.Option(None, "--output", help="Output file path."),
    stdout: bool = typer.Option(False, "--stdout"),
) -> None:
    """Run a map report with optional geometry filter.

    Examples:
      acugis-report map-report run Parcels --geometry-file geometry.wkt --srid 2193 --output report.pdf

    Related commands:
      acugis-report map extent
      acugis-report reports run
    """
    geom = geometry
    if geometry_file:
        geom = geometry_file.read_text(encoding="utf-8").strip()
    out = get_output(ctx)
    client = get_client(ctx)
    content, _ = client.run_map_report(report, geometry=geom, srid=srid, format=format)
    path = str(output) if output else f"{report}.{format.lstrip('.')}"
    write_binary(content, output_path=None if stdout else path, stdout=stdout)
    if not stdout:
        out.success(f"Wrote {path} ({len(content)} bytes)")
