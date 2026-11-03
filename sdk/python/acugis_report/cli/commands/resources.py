"""Resource commands."""

from __future__ import annotations

from pathlib import Path
from typing import Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output
from acugis_report.cli.output import write_binary

app = typer.Typer(help="Manage content resources and images.", no_args_is_help=True)


@app.command("list")
@handle_errors
def list_resources(
    ctx: typer.Context,
    folder_id: Optional[int] = typer.Option(None, "--folder-id", "-f"),
    resource_type: Optional[str] = typer.Option(None, "--type", "-t"),
    q: Optional[str] = typer.Option(None, "--query", "-q"),
) -> None:
    """List resources.

    Examples:
      acugis-report resources list
      acugis-report resources list --folder-id 3 --type image

    Related commands:
      acugis-report resources upload
      acugis-report resources images
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(
        client.list_resources(folder_id=folder_id, resource_type=resource_type, q=q),
        columns=["uuid", "display_name", "resource_type", "mime_type", "size_bytes", "folder_id"],
    )


@app.command("upload")
@handle_errors
def upload(
    ctx: typer.Context,
    file: Path = typer.Argument(..., exists=True, readable=True),
    folder_id: Optional[int] = typer.Option(None, "--folder-id", "-f"),
    display_name: Optional[str] = typer.Option(None, "--name"),
    resource_type: Optional[str] = typer.Option(None, "--type"),
) -> None:
    """Upload a resource file.

    Examples:
      acugis-report resources upload logo.png --folder-id 3

    Related commands:
      acugis-report resources list
      acugis-report resources download
    """
    out = get_output(ctx)
    client = get_client(ctx)
    result = client.upload_resource(
        file,
        folder_id=folder_id,
        display_name=display_name,
        resource_type=resource_type,
    )
    out.emit(result)
    out.success("Uploaded")


@app.command("download")
@handle_errors
def download(
    ctx: typer.Context,
    uuid: str = typer.Argument(..., help="Resource UUID."),
    output: Optional[Path] = typer.Option(None, "--output", "-o"),
    stdout: bool = typer.Option(False, "--stdout"),
) -> None:
    """Download a resource.

    Examples:
      acugis-report resources download <uuid> -o file.bin

    Related commands:
      acugis-report resources list
      acugis-report resources replace
    """
    out = get_output(ctx)
    client = get_client(ctx)
    content = client.download_resource(uuid)
    path = str(output) if output else uuid
    write_binary(content, output_path=None if stdout else path, stdout=stdout)
    if not stdout:
        out.success(f"Wrote {path}")


@app.command("replace")
@handle_errors
def replace(
    ctx: typer.Context,
    uuid: str = typer.Argument(..., help="Resource UUID."),
    file: Path = typer.Argument(..., exists=True, readable=True),
) -> None:
    """Replace resource file content.

    Examples:
      acugis-report resources replace <uuid> new-logo.png

    Related commands:
      acugis-report resources download
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.replace_resource(uuid, file))
    out.success("Replaced")


@app.command("delete")
@handle_errors
def delete_resource(
    ctx: typer.Context,
    uuid: str = typer.Argument(..., help="Resource UUID."),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete a resource.

    Examples:
      acugis-report resources delete <uuid> --yes

    Related commands:
      acugis-report resources list
    """
    if not yes:
        typer.confirm(f"Delete resource {uuid}?", abort=True)
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.delete_resource(uuid))
    out.success("Deleted")


@app.command("images")
@handle_errors
def images(
    ctx: typer.Context,
    folder_id: Optional[int] = typer.Option(None, "--folder-id", "-f"),
    q: Optional[str] = typer.Option(None, "--query", "-q"),
) -> None:
    """List image resources.

    Examples:
      acugis-report resources images

    Related commands:
      acugis-report resources list
      acugis-report resources upload
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.list_images(folder_id=folder_id, q=q))
