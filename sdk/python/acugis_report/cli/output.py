"""CLI output helpers (table / json / yaml)."""

from __future__ import annotations

import json
import sys
from typing import Any, Iterable, Sequence

import yaml
from rich.console import Console
from rich.table import Table
from rich.text import Text


class Output:
    def __init__(
        self,
        *,
        format: str = "table",
        no_color: bool = False,
        verbose: bool = False,
        debug: bool = False,
    ):
        self.format = format
        self.verbose = verbose
        self.debug = debug
        self.console = Console(
            stderr=False,
            force_terminal=None if not no_color else False,
            no_color=no_color,
            highlight=False,
        )
        self.err = Console(
            stderr=True,
            force_terminal=None if not no_color else False,
            no_color=no_color,
            highlight=False,
        )

    def emit(
        self,
        data: Any,
        *,
        columns: Sequence[str] | None = None,
        title: str | None = None,
    ) -> None:
        if self.format == "json":
            self.console.print_json(json.dumps(data, default=str))
            return
        if self.format == "yaml":
            self.console.print(yaml.safe_dump(data, sort_keys=False, allow_unicode=True), end="")
            return
        self._emit_table(data, columns=columns, title=title)

    def _emit_table(
        self,
        data: Any,
        *,
        columns: Sequence[str] | None = None,
        title: str | None = None,
    ) -> None:
        rows: list[dict[str, Any]]
        if data is None:
            self.console.print("(empty)")
            return
        if isinstance(data, dict):
            # Prefer list-like keys
            for key in ("items", "tokens", "views", "dashboards", "widgets", "datasources", "tables", "options"):
                if key in data and isinstance(data[key], list):
                    rows = [_rowify(x) for x in data[key]]
                    break
            else:
                if all(not isinstance(v, (dict, list)) for v in data.values()):
                    rows = [dict(data)]
                else:
                    # Key/value dump
                    table = Table(title=title, show_header=True, header_style="bold")
                    table.add_column("Field")
                    table.add_column("Value")
                    for k, v in data.items():
                        table.add_row(str(k), _fmt(v))
                    self.console.print(table)
                    return
        elif isinstance(data, list):
            rows = [_rowify(x) for x in data]
        else:
            self.console.print(str(data))
            return

        if not rows:
            self.console.print("(empty)")
            return

        cols = list(columns) if columns else _infer_columns(rows)
        table = Table(title=title, show_header=True, header_style="bold")
        for col in cols:
            table.add_column(col)
        for row in rows:
            table.add_row(*[_fmt(row.get(c, "")) for c in cols])
        self.console.print(table)

    def success(self, message: str) -> None:
        self.err.print(Text(f"✓ {message}", style="green"))

    def warn(self, message: str) -> None:
        self.err.print(Text(f"! {message}", style="yellow"))

    def error(self, message: str) -> None:
        self.err.print(Text(f"✗ {message}", style="red"))

    def info(self, message: str) -> None:
        if self.verbose or self.debug:
            self.err.print(message)


def _rowify(item: Any) -> dict[str, Any]:
    if isinstance(item, dict):
        return item
    if hasattr(item, "model_dump"):
        return item.model_dump()
    return {"value": item}


def _infer_columns(rows: Iterable[dict[str, Any]]) -> list[str]:
    preferred = [
        "key",
        "name",
        "username",
        "uuid",
        "id",
        "display_name",
        "type",
        "datasource",
        "resource_type",
        "description",
        "status",
        "format",
        "has_thumbnail",
        "email",
        "groups",
        "expires_at",
        "created_at",
    ]
    seen: list[str] = []
    keys: set[str] = set()
    for row in rows:
        keys.update(str(k) for k in row.keys())
    for p in preferred:
        if p in keys:
            seen.append(p)
            keys.discard(p)
    # Limit remaining columns for readability
    rest = sorted(keys)
    return seen + rest[:8]


def _fmt(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (list, dict)):
        return json.dumps(value, default=str)
    return str(value)


def write_binary(content: bytes, *, output_path: str | None, stdout: bool) -> None:
    if stdout:
        sys.stdout.buffer.write(content)
        return
    if not output_path:
        raise ValueError("Binary output requires --output PATH or --stdout")
    path = output_path
    with open(path, "wb") as fh:
        fh.write(content)
