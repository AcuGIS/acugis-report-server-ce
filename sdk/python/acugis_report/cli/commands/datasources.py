"""Datasource commands."""

from __future__ import annotations

from typing import Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output
from acugis_report.errors import NotImplementedByServerError

app = typer.Typer(help="Manage report datasources.", no_args_is_help=True)


@app.command("list")
@handle_errors
def list_datasources(ctx: typer.Context) -> None:
    """List datasource names.

    Examples:
      acugis-report datasources list
      acugis-report datasources list --output json

    Related commands:
      acugis-report datasources create
      acugis-report datasources context
    """
    out = get_output(ctx)
    client = get_client(ctx)
    items = client.list_datasources()
    out.emit(items, columns=["name"])


@app.command("create")
@handle_errors
def create(
    ctx: typer.Context,
    name: str = typer.Option(..., "--name", "-n"),
    db_type: str = typer.Option(..., "--db-type", help="e.g. postgres, mysql"),
    host: str = typer.Option(..., "--host"),
    port: Optional[str] = typer.Option(None, "--port"),
    dbname: str = typer.Option(..., "--dbname"),
    username: str = typer.Option(..., "--username", "-u"),
    password: str = typer.Option(..., "--password", "-p", hide_input=True),
    schema_name: Optional[str] = typer.Option(None, "--schema"),
    auth_type: Optional[str] = typer.Option(None, "--auth-type"),
) -> None:
    """Create or upsert a datasource.

    Examples:
      acugis-report datasources create -n postgres --db-type postgres --host db --dbname reports -u jrs -p secret

    Related commands:
      acugis-report datasources list
      acugis-report datasources delete
    """
    out = get_output(ctx)
    client = get_client(ctx)
    fields = {
        "name": name,
        "db_type": db_type,
        "host": host,
        "port": port,
        "dbname": dbname,
        "username": username,
        "password": password,
        "schema_name": schema_name,
        "auth_type": auth_type,
    }
    result = client.create_datasource(**fields)
    out.emit(result)
    out.success(f"Datasource {name} saved")


@app.command("delete")
@handle_errors
def delete_ds(
    ctx: typer.Context,
    datasource_id: str = typer.Argument(..., help="Datasource id or name."),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete a datasource.

    Examples:
      acugis-report datasources delete 3 --yes

    Related commands:
      acugis-report datasources list
    """
    if not yes:
        typer.confirm(f"Delete datasource {datasource_id}?", abort=True)
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.delete_datasource(datasource_id))
    out.success("Deleted")


@app.command("context")
@handle_errors
def context(ctx: typer.Context) -> None:
    """Generate Tomcat context.xml for Jasper datasources.

    Examples:
      acugis-report datasources context

    Related commands:
      acugis-report datasources list
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.datasources_context())


@app.command("show")
@handle_errors
def show(
    ctx: typer.Context,
    datasource_id: str = typer.Argument(...),
) -> None:
    """Show a datasource (when GET endpoint is available).

    Related commands:
      acugis-report datasources list
    """
    out = get_output(ctx)
    client = get_client(ctx)
    try:
        out.emit(client.get_datasource(datasource_id))
    except NotImplementedByServerError as exc:
        out.error(exc.message)
        raise typer.Exit(1) from None


@app.command("update")
@handle_errors
def update(
    ctx: typer.Context,
    datasource_id: str = typer.Argument(...),
) -> None:
    """Update a datasource (when PUT endpoint is available).

    Related commands:
      acugis-report datasources create
    """
    out = get_output(ctx)
    client = get_client(ctx)
    try:
        out.emit(client.update_datasource(datasource_id))
    except NotImplementedByServerError as exc:
        out.error(exc.message)
        raise typer.Exit(1) from None
