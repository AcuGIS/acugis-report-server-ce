"""Service namespace facades for ReportServer.

Thin wrappers around the flat methods on :class:`ReportServer` so callers can
use idiomatic namespaced APIs aligned with the .NET, Java, and Go SDKs:

    client.reports.list()
    client.auth.me()
    client.folders.tree()
"""

from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any, Mapping

from acugis_report.errors import NotImplementedByServerError
from acugis_report.models import CreateTokenResponse, Me, PersonalAccessToken, ReportList

if TYPE_CHECKING:
    from acugis_report.client import ReportServer


class AuthService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def login(self, username: str, password: str) -> dict[str, Any]:
        return self._c.login(username, password)

    def logout(self) -> dict[str, Any]:
        return self._c.logout()

    def me(self) -> Me:
        return self._c.me()

    def list_tokens(self) -> list[PersonalAccessToken]:
        return self._c.list_tokens()

    def create_token(
        self,
        name: str,
        *,
        description: str | None = None,
        expires_in: str = "never",
        expires_at: str | None = None,
    ) -> CreateTokenResponse:
        return self._c.create_token(
            name,
            description=description,
            expires_in=expires_in,
            expires_at=expires_at,
        )

    def delete_token(self, token_id: int, *, purge: bool = False) -> None:
        return self._c.delete_token(token_id, purge=purge)

    def rotate_token(self, token_id: int) -> CreateTokenResponse:
        return self._c.rotate_token(token_id)

    def login_and_create_cli_token(
        self,
        username: str,
        password: str,
        *,
        token_name: str = "acugis-report-cli",
    ) -> CreateTokenResponse:
        return self._c.login_and_create_cli_token(username, password, token_name=token_name)


class HealthService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def __call__(self) -> dict[str, Any]:
        """Backward-compatible: ``client.health()`` == ``client.health.check()``."""
        return self.check()

    def check(self) -> dict[str, Any]:
        # Call the HTTP implementation directly — ``client.health`` is this service.
        return self._c._request("GET", "/health", auth_optional=True)

    def ready(self) -> dict[str, Any]:
        return self._c.ready()

    def diagnostics(self) -> dict[str, Any]:
        return self._c.diagnostics()

    def jasper_status(self) -> dict[str, Any]:
        return self._c.jasper_status()

    def doctor(self) -> dict[str, Any]:
        return self._c.doctor()


class ReportsService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def list(self) -> ReportList:
        return self._c.list_reports()

    def get(self, key: str) -> dict[str, Any]:
        return self._c.get_report(key)

    def publish(
        self,
        jrxml_file: str | Path,
        *,
        data_source: str,
        report_name: str | None = None,
        report_description: str | None = None,
        report_type: str = "jasper",
        jasper_file: str | Path | None = None,
        thumbnail_file: str | Path | None = None,
        access_groups: list[str] | None = None,
        extra_fields: Mapping[str, str] | None = None,
    ) -> dict[str, Any]:
        return self._c.publish_report(
            jrxml_file,
            data_source=data_source,
            report_name=report_name,
            report_description=report_description,
            report_type=report_type,
            jasper_file=jasper_file,
            thumbnail_file=thumbnail_file,
            access_groups=access_groups,
            extra_fields=extra_fields,
        )

    def update(
        self,
        key: str,
        *,
        jrxml_file: str | Path | None = None,
        data_source: str | None = None,
        report_name: str | None = None,
        report_description: str | None = None,
        report_type: str | None = None,
        jasper_file: str | Path | None = None,
        thumbnail_file: str | Path | None = None,
        access_groups: list[str] | None = None,
    ) -> dict[str, Any]:
        return self._c.update_report(
            key,
            jrxml_file=jrxml_file,
            data_source=data_source,
            report_name=report_name,
            report_description=report_description,
            report_type=report_type,
            jasper_file=jasper_file,
            thumbnail_file=thumbnail_file,
            access_groups=access_groups,
        )

    def delete(self, key: str) -> dict[str, Any]:
        return self._c.delete_report(key)

    def config(self, key: str) -> dict[str, Any]:
        return self._c.report_config(key)

    def thumbnail(self, key: str) -> bytes:
        return self._c.report_thumbnail(key)

    def parameter_lookup(
        self,
        key: str,
        name: str,
        *,
        params: Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        return self._c.parameter_lookup(key, name, params=params)

    def run(
        self,
        key: str,
        *,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
        method: str = "GET",
    ) -> tuple[bytes, str | None]:
        return self._c.run_report(key, format=format, params=params, method=method)

    def run_to_file(
        self,
        key: str,
        path: str | Path,
        *,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
        method: str = "GET",
    ) -> tuple[bytes, str | None]:
        return self._c.run_to_file(key, path, format=format, params=params, method=method)


class FoldersService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def tree(self) -> Any:
        return self._c.folders_tree()

    def browse(self, folder_id: int = 0) -> dict[str, Any]:
        return self._c.folders_browse(folder_id)

    def search(self, q: str) -> dict[str, Any]:
        return self._c.folders_search(q)

    def create(
        self,
        name: str,
        *,
        parent_id: int = 0,
        description: str | None = None,
        icon: str | None = None,
        sort_order: int | None = None,
        acl_inherit: bool | None = None,
    ) -> dict[str, Any]:
        return self._c.create_folder(
            name,
            parent_id=parent_id,
            description=description,
            icon=icon,
            sort_order=sort_order,
            acl_inherit=acl_inherit,
        )

    def get(self, folder_id: int) -> dict[str, Any]:
        return self._c.get_folder(folder_id)

    def update(self, folder_id: int, **fields: Any) -> dict[str, Any]:
        return self._c.update_folder(folder_id, **fields)

    def delete(self, folder_id: int, *, recursive: bool = False, confirm: bool = True) -> dict[str, Any]:
        return self._c.delete_folder(folder_id, recursive=recursive, confirm=confirm)

    def move(self, folder_id: int, parent_id: int, *, sort_order: int | None = None) -> dict[str, Any]:
        return self._c.move_folder(folder_id, parent_id, sort_order=sort_order)

    def permissions(self, folder_id: int) -> dict[str, Any]:
        return self._c.folder_permissions(folder_id)

    def set_permissions(
        self,
        folder_id: int,
        group_names: list[str],
        permission: str,
    ) -> dict[str, Any]:
        return self._c.set_folder_permissions(folder_id, group_names, permission)


class ResourcesService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def list(
        self,
        *,
        folder_id: int | None = None,
        resource_type: str | None = None,
        q: str | None = None,
    ) -> dict[str, Any]:
        return self._c.list_resources(folder_id=folder_id, resource_type=resource_type, q=q)

    def upload(
        self,
        file_path: str | Path,
        *,
        folder_id: int | None = None,
        display_name: str | None = None,
        resource_type: str | None = None,
    ) -> dict[str, Any]:
        return self._c.upload_resource(
            file_path,
            folder_id=folder_id,
            display_name=display_name,
            resource_type=resource_type,
        )

    def get(self, uuid: str) -> dict[str, Any]:
        return self._c.get_resource(uuid)

    def download(self, uuid: str) -> bytes:
        return self._c.download_resource(uuid)

    def replace(self, uuid: str, file_path: str | Path) -> dict[str, Any]:
        return self._c.replace_resource(uuid, file_path)

    def delete(self, uuid: str) -> dict[str, Any]:
        return self._c.delete_resource(uuid)

    def list_images(self, *, folder_id: int | None = None, q: str | None = None) -> dict[str, Any]:
        return self._c.list_images(folder_id=folder_id, q=q)


class DatasourcesService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def list(self) -> list[dict[str, Any]]:
        return self._c.list_datasources()

    def create(self, **fields: Any) -> dict[str, Any]:
        return self._c.create_datasource(**fields)

    def delete(self, datasource_id: str | int) -> dict[str, Any]:
        return self._c.delete_datasource(datasource_id)

    def context(self) -> dict[str, Any]:
        return self._c.datasources_context()

    def get(self, datasource_id: str | int) -> dict[str, Any]:
        return self._c.get_datasource(datasource_id)

    def update(self, datasource_id: str | int, **fields: Any) -> dict[str, Any]:
        return self._c.update_datasource(datasource_id, **fields)


class MapReportsService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def features(self, key: str, **params: Any) -> dict[str, Any]:
        return self._c.map_features(key, **params)

    def extent(
        self,
        key: str,
        *,
        srid: int | None = None,
        geometry: str | None = None,
        xmin: float | None = None,
        ymin: float | None = None,
        xmax: float | None = None,
        ymax: float | None = None,
    ) -> dict[str, Any]:
        return self._c.map_extent(
            key,
            srid=srid,
            geometry=geometry,
            xmin=xmin,
            ymin=ymin,
            xmax=xmax,
            ymax=ymax,
        )

    def columns(self, key: str) -> dict[str, Any]:
        return self._c.map_columns(key)

    def run(
        self,
        key: str,
        *,
        geometry: str | None = None,
        srid: int | None = None,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
    ) -> tuple[bytes, str | None]:
        return self._c.run_map_report(
            key,
            geometry=geometry,
            srid=srid,
            format=format,
            params=params,
        )


class EmbeddedMapsService:
    def __init__(self, client: ReportServer) -> None:
        self._c = client

    def debug(self, report: str, *, compile: bool = False) -> dict[str, Any]:
        return self._c.embedded_map_debug(report, compile=compile)


class ReservedService:
    """Placeholder for future Users / Groups / Schedules / Jobs / Recipients APIs."""

    def __init__(self, name: str) -> None:
        self.name = name

    def list(self) -> Any:
        raise NotImplementedByServerError(self.name.lower())

    def __getattr__(self, item: str) -> Any:
        raise NotImplementedByServerError(self.name.lower())
