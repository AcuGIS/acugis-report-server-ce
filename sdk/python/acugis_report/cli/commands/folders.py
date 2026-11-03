"""Folder commands."""

from __future__ import annotations

from typing import Optional

import typer

from acugis_report.cli.commands._util import handle_errors
from acugis_report.cli.ctx import get_client, get_output

app = typer.Typer(help="Browse and manage content folders.", no_args_is_help=True)


@app.command("tree")
@handle_errors
def tree(ctx: typer.Context) -> None:
    """Show the ACL-filtered folder tree.

    Examples:
      acugis-report folders tree
      acugis-report folders tree --output json

    Related commands:
      acugis-report folders browse
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.folders_tree())


@app.command("browse")
@handle_errors
def browse(
    ctx: typer.Context,
    folder_id: int = typer.Option(0, "--folder-id", "-f", help="Folder id (0 = root)."),
) -> None:
    """Browse folder contents.

    Examples:
      acugis-report folders browse
      acugis-report folders browse --folder-id 12

    Related commands:
      acugis-report folders tree
      acugis-report folders create
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.folders_browse(folder_id))


@app.command("create")
@handle_errors
def create(
    ctx: typer.Context,
    name: str = typer.Argument(..., help="Folder name."),
    parent_id: int = typer.Option(0, "--parent-id", help="Parent folder id."),
    description: Optional[str] = typer.Option(None, "--description"),
) -> None:
    """Create a folder.

    Examples:
      acugis-report folders create Finance --parent-id 0

    Related commands:
      acugis-report folders browse
      acugis-report folders move
    """
    out = get_output(ctx)
    client = get_client(ctx)
    result = client.create_folder(name, parent_id=parent_id, description=description)
    out.emit(result)
    out.success(f"Created folder {name}")


@app.command("update")
@handle_errors
def update(
    ctx: typer.Context,
    folder_id: int = typer.Argument(..., help="Folder id."),
    name: Optional[str] = typer.Option(None, "--name"),
    description: Optional[str] = typer.Option(None, "--description"),
    icon: Optional[str] = typer.Option(None, "--icon"),
    sort_order: Optional[int] = typer.Option(None, "--sort-order"),
) -> None:
    """Update folder metadata.

    Examples:
      acugis-report folders update 12 --name Billing

    Related commands:
      acugis-report folders browse
    """
    fields = {k: v for k, v in {
        "name": name,
        "description": description,
        "icon": icon,
        "sort_order": sort_order,
    }.items() if v is not None}
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.update_folder(folder_id, **fields))


@app.command("delete")
@handle_errors
def delete_folder(
    ctx: typer.Context,
    folder_id: int = typer.Argument(..., help="Folder id."),
    recursive: bool = typer.Option(False, "--recursive", "-r"),
    yes: bool = typer.Option(False, "--yes", "-y"),
) -> None:
    """Delete a folder.

    Examples:
      acugis-report folders delete 12 --yes
      acugis-report folders delete 12 --recursive --yes

    Related commands:
      acugis-report folders browse
    """
    if not yes:
        typer.confirm(f"Delete folder {folder_id}?", abort=True)
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.delete_folder(folder_id, recursive=recursive, confirm=True))
    out.success(f"Deleted folder {folder_id}")


@app.command("move")
@handle_errors
def move(
    ctx: typer.Context,
    folder_id: int = typer.Argument(..., help="Folder id."),
    parent_id: int = typer.Option(..., "--parent-id", help="New parent folder id."),
) -> None:
    """Move a folder under a new parent.

    Examples:
      acugis-report folders move 12 --parent-id 3

    Related commands:
      acugis-report folders tree
    """
    out = get_output(ctx)
    client = get_client(ctx)
    out.emit(client.move_folder(folder_id, parent_id))
    out.success(f"Moved folder {folder_id}")


@app.command("permissions")
@handle_errors
def permissions(
    ctx: typer.Context,
    folder_id: int = typer.Argument(..., help="Folder id."),
    set_groups: Optional[list[str]] = typer.Option(
        None,
        "--set-group",
        help="Replace grants with these groups (repeatable).",
    ),
    permission: str = typer.Option("view", "--permission", help="Permission when setting grants."),
) -> None:
    """Get or set folder permissions.

    Examples:
      acugis-report folders permissions 12
      acugis-report folders permissions 12 --set-group Analysts --permission edit

    Related commands:
      acugis-report folders browse
    """
    out = get_output(ctx)
    client = get_client(ctx)
    if set_groups is not None:
        result = client.set_folder_permissions(folder_id, set_groups, permission)
        out.emit(result)
        out.success("Permissions updated")
        return
    out.emit(client.folder_permissions(folder_id))
