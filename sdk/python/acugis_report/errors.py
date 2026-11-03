"""SDK exception hierarchy with CLI exit codes."""

from __future__ import annotations

EXIT_SUCCESS = 0
EXIT_GENERAL = 1
EXIT_AUTH = 2
EXIT_VALIDATION = 3
EXIT_NOT_FOUND = 4
EXIT_PERMISSIONS = 5
EXIT_SERVER = 6
EXIT_NETWORK = 7


class ReportServerError(Exception):
    """Base error for Report Server client failures."""

    exit_code: int = EXIT_GENERAL

    def __init__(self, message: str, *, status_code: int | None = None, body: object | None = None):
        super().__init__(message)
        self.message = message
        self.status_code = status_code
        self.body = body


class AuthenticationError(ReportServerError):
    exit_code = EXIT_AUTH


class ValidationError(ReportServerError):
    exit_code = EXIT_VALIDATION


class NotFoundError(ReportServerError):
    exit_code = EXIT_NOT_FOUND


class PermissionError(ReportServerError):  # noqa: A001 — mirrors HTTP 403 semantics
    exit_code = EXIT_PERMISSIONS


class ConflictError(ReportServerError):
    exit_code = EXIT_VALIDATION


class ServerError(ReportServerError):
    exit_code = EXIT_SERVER


class NetworkError(ReportServerError):
    exit_code = EXIT_NETWORK


class TimeoutError(ReportServerError):  # noqa: A001 — SDK timeout, not builtins.TimeoutError
    exit_code = EXIT_NETWORK


class InvalidResponseError(ReportServerError):
    exit_code = EXIT_GENERAL


class NotImplementedByServerError(ReportServerError):
    """Reserved for APIs that do not exist on Report Server yet."""

    exit_code = EXIT_GENERAL

    def __init__(self, feature: str):
        super().__init__(f"Not yet implemented by Report Server: {feature}")
        self.feature = feature


def error_from_status(status_code: int, message: str, body: object | None = None) -> ReportServerError:
    """Map an HTTP status code to a typed SDK error."""
    if status_code in (401,):
        return AuthenticationError(message, status_code=status_code, body=body)
    if status_code in (403,):
        return PermissionError(message, status_code=status_code, body=body)
    if status_code in (404,):
        return NotFoundError(message, status_code=status_code, body=body)
    if status_code in (409,):
        return ConflictError(message, status_code=status_code, body=body)
    if status_code in (400, 422):
        return ValidationError(message, status_code=status_code, body=body)
    if status_code >= 500:
        return ServerError(message, status_code=status_code, body=body)
    return ReportServerError(message, status_code=status_code, body=body)
