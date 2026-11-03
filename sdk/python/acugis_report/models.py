"""Pydantic models for common API responses."""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class FlexibleModel(BaseModel):
    model_config = ConfigDict(extra="allow")


class Me(FlexibleModel):
    id: int | str | None = None
    username: str | None = None
    email: str | None = None
    groups: list[str] = Field(default_factory=list)
    is_admin: bool = False
    permissions: dict[str, Any] | list[Any] | None = None


class PersonalAccessToken(FlexibleModel):
    id: int | None = None
    name: str | None = None
    description: str | None = None
    token_prefix: str | None = None
    created_at: str | None = None
    expires_at: str | None = None
    last_used_at: str | None = None
    revoked_at: str | None = None


class CreateTokenResponse(FlexibleModel):
    token: str
    id: int | None = None
    created_at: str | None = None
    name: str | None = None
    message: str | None = None


class ReportSummary(FlexibleModel):
    key: str
    name: str | None = None
    description: str | None = None
    datasource: str | None = None
    type: str | None = None
    format: str | None = None
    has_thumbnail: bool | None = None


class ReportList(FlexibleModel):
    items: list[ReportSummary] = Field(default_factory=list)
    count: int = 0


class DatasourceName(FlexibleModel):
    name: str


class Resource(FlexibleModel):
    id: int | None = None
    uuid: str | None = None
    folder_id: int | None = None
    display_name: str | None = None
    resource_type: str | None = None
    original_filename: str | None = None
    extension: str | None = None
    mime_type: str | None = None
    sha256: str | None = None
    size_bytes: int | None = None
    created_at: str | None = None
    updated_at: str | None = None


REPORT_FORMATS = (
    "html",
    "pdf",
    "xlsx",
    "xls",
    "csv",
    "docx",
    "rtf",
    "odt",
    "ods",
    "pptx",
    "xml",
)
