"""Shared CLI command utilities."""

from __future__ import annotations

import functools
import traceback
from typing import Any, Callable, TypeVar

import typer

from acugis_report.cli.ctx import get_output, get_state
from acugis_report.errors import ReportServerError

F = TypeVar("F", bound=Callable[..., Any])


def handle_errors(fn: F) -> F:
    """Wrap a command so SDK errors become concise CLI exits."""

    @functools.wraps(fn)
    def wrapper(*args: Any, **kwargs: Any) -> Any:
        ctx: typer.Context | None = None
        if args and isinstance(args[0], typer.Context):
            ctx = args[0]
        elif "ctx" in kwargs and isinstance(kwargs["ctx"], typer.Context):
            ctx = kwargs["ctx"]
        try:
            return fn(*args, **kwargs)
        except ReportServerError as exc:
            out = get_output(ctx) if ctx else None
            if out:
                out.error(exc.message)
                state = get_state(ctx)
                if state.debug:
                    traceback.print_exc()
            else:
                typer.echo(f"Error: {exc.message}", err=True)
            raise typer.Exit(exc.exit_code) from None
        except typer.Exit:
            raise
        except Exception as exc:  # noqa: BLE001
            out = get_output(ctx) if ctx else None
            msg = str(exc)
            if out:
                out.error(msg)
                state = get_state(ctx)
                if state.debug:
                    traceback.print_exc()
            else:
                typer.echo(f"Error: {msg}", err=True)
            raise typer.Exit(1) from None

    return wrapper  # type: ignore[return-value]
