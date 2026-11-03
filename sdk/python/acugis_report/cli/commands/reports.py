"""Report commands."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output
from acugis_report.cli.output import write_binary
from acugis_report.errors import ValidationError
from acugis_report.models import REPORT_FORMATS

app = typer.Typer(help="Manage and run Jasper reports.", no_args_is_help=True)


def _parse_params(
    param: Optional[list[str]],
    params_file: Optional[Path],
) -> dict[str, Any]:
    result: dict[str, Any] = {}
    if params_file:
        data = json.loads(params_file.read_text(encoding="utf-8"))
        if not isinstance(data, dict):
            raise ValidationError("Parameter file must contain a JSON object")
        result.update(data)
    if param:
        for item in param:
            if "=" not in item:
                raise ValidationError(f"Invalid --param {item!r}; expected key=value")
            key, value = item.split("=", 1)
            if key in result:
                existing = result[key]
                if isinstance(existing, list):
                    existing.append(value)
                else:
                    result[key] = [existing, value]
            else:
                result[key] = value
    return result


@app.command("list")
@handle_errors
def list_reports(ctx: typer.Context) -> None:
    """List visible reports.

    Examples:
      acugis-report reports list
      acugis-report reports list --output json

    Related commands:
      acugis-report reports show
      acugis-report reports run
    """
    out = get_output(ctx)
    client = get_client(ctx)
    data = client.list_reports()
    out.emit(
        data.model_dump(),
        columns=["key", "name", "type", "datasource", "description", "has_thumbnail"],
        title="Reports",
    )


@app.command("show")
@handle_errors
def show_report(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
) -> None:
    """Show a report definition.

    Examples:
      acugis-report reports show Invoice
      acugis-report reports show Invoice --output yaml

    Related commands:
      acugis-report reports list
      acugis-report reports config
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.get_report(report))


@app.command("publish")
@handle_errors
def publish(
    ctx: typer.Context,
    jrxml: Path = typer.Argument(..., exists=True, readable=True, help="JRXML file to publish."),
    data_source: str = typer.Option(..., "--data-source", "-d", help="Datasource name."),
    name: Optional[str] = typer.Option(None, "--name", help="Report display name."),
    description: Optional[str] = typer.Option(None, "--description", help="Report description."),
    report_type: str = typer.Option("jasper", "--type", help="jasper or map."),
    jasper_file: Optional[Path] = typer.Option(None, "--jasper-file", help="Optional compiled .jasper."),
    thumbnail: Optional[Path] = typer.Option(None, "--thumbnail", help="Optional thumbnail image."),
    group: Optional[list[str]] = typer.Option(None, "--group", help="Access group (repeatable)."),
) -> None:
    """Publish a JRXML report.

    Examples:
      acugis-report reports publish invoice.jrxml --data-source postgres
      acugis-report reports publish map.jrxml -d postgres --type map --name "Parcels"

    Related commands:
      acugis-report reports update
      acugis-report reports run
    """
    out = get_output(ctx)
    client = get_client(ctx)
    result = client.publish_report(
        jrxml,
        data_source=data_source,
        report_name=name,
        report_description=description,
        report_type=report_type,
        jasper_file=jasper_file,
        thumbnail_file=thumbnail,
        access_groups=group,
    )
    out.emit(result)
    out.success(f"Published {result.get('key', '')}")


@app.command("update")
@handle_errors
def update(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
    jrxml: Optional[Path] = typer.Option(None, "--jrxml", exists=True, help="Replacement JRXML."),
    data_source: Optional[str] = typer.Option(None, "--data-source", "-d"),
    name: Optional[str] = typer.Option(None, "--name"),
    description: Optional[str] = typer.Option(None, "--description"),
    report_type: Optional[str] = typer.Option(None, "--type"),
    jasper_file: Optional[Path] = typer.Option(None, "--jasper-file"),
    thumbnail: Optional[Path] = typer.Option(None, "--thumbnail"),
    group: Optional[list[str]] = typer.Option(None, "--group"),
) -> None:
    """Update an existing report.

    Examples:
      acugis-report reports update Invoice --jrxml invoice.jrxml
      acugis-report reports update Invoice --name "Invoice v2"

    Related commands:
      acugis-report reports publish
      acugis-report reports show
    """
    out = get_output(ctx)
    client = get_client(ctx)
    result = client.update_report(
        report,
        jrxml_file=jrxml,
        data_source=data_source,
        report_name=name,
        report_description=description,
        report_type=report_type,
        jasper_file=jasper_file,
        thumbnail_file=thumbnail,
        access_groups=group,
    )
    out.emit(result)
    out.success(f"Updated {report}")


@app.command("delete")
@handle_errors
def delete_report(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
    yes: bool = typer.Option(False, "--yes", "-y", help="Skip confirmation."),
) -> None:
    """Delete a report.

    Examples:
      acugis-report reports delete OldInvoice --yes

    Related commands:
      acugis-report reports list
    """
    if not yes:
        typer.confirm(f"Delete report {report}?", abort=True)
    out = get_output(ctx)
    client = get_client(ctx)
    result = client.delete_report(report)
    out.emit(result)
    out.success(f"Deleted {report}")


@app.command("config")
@handle_errors
def config(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
) -> None:
    """Show viewer/config metadata for a report.

    Examples:
      acugis-report reports config Invoice --output json

    Related commands:
      acugis-report reports show
      acugis-report reports run
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.report_config(report))


@app.command("thumbnail")
@handle_errors
def thumbnail(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
    output: Optional[Path] = typer.Option(None, "--output", "-o", help="Write PNG to path."),
    stdout: bool = typer.Option(False, "--stdout", help="Write binary to stdout."),
) -> None:
    """Download a report thumbnail.

    Examples:
      acugis-report reports thumbnail Invoice -o thumb.png

    Related commands:
      acugis-report reports show
    """
    out = get_output(ctx)
    client = get_client(ctx)
    content = client.report_thumbnail(report)
    path = str(output) if output else None
    write_binary(content, output_path=path, stdout=stdout)
    if not stdout:
        out.success(f"Wrote {path or 'thumbnail'}")


@app.command("run")
@handle_errors
def run(
    ctx: typer.Context,
    report: str = typer.Argument(..., help="Report key."),
    param: Optional[list[str]] = typer.Option(
        None,
        "--param",
        help="Report parameter key=value (repeatable; repeat key for multi-value).",
    ),
    params: Optional[Path] = typer.Option(None, "--params", exists=True, help="JSON parameter file."),
    format: str = typer.Option("pdf", "--format", "-f", help=f"Output format: {', '.join(REPORT_FORMATS)}"),
    output: Optional[Path] = typer.Option(None, "--output", help="Write report bytes to this path."),
    stdout: bool = typer.Option(False, "--stdout", help="Write binary to stdout."),
) -> None:
    """Execute a report and save the result.

    Binary output is never printed to the terminal unless --stdout is set.

    Examples:
      acugis-report reports run Invoice --param InvoiceID=123 --output invoice.pdf
      acugis-report reports run Sales --param region=West --param region=North -f xlsx --output sales.xlsx
      acugis-report reports run Invoice --params params.json --format pdf --output invoice.pdf

    Related commands:
      acugis-report reports list
      acugis-report reports config
      acugis-report map-report run
    """
    out = get_output(ctx)
    client = get_client(ctx)
    run_params = _parse_params(param, params)
    content, _ctype = client.run_report(report, format=format, params=run_params)

    out_path = str(output) if output else None
    if not stdout and not out_path:
        # Default filename from report + format
        out_path = f"{report}.{format.lstrip('.')}"
    write_binary(content, output_path=out_path, stdout=stdout)
    if not stdout:
        out.success(f"Wrote {out_path} ({len(content)} bytes)")
