"""HTTP client for AcuGIS Report Server REST API."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Mapping, MutableMapping

import httpx

from acugis_report.errors import (
    InvalidResponseError,
    NetworkError,
    NotFoundError,
    NotImplementedByServerError,
    ReportServerError,
    ServerError,
    TimeoutError,
    ValidationError,
    error_from_status,
)
from acugis_report.models import (
    REPORT_FORMATS,
    CreateTokenResponse,
    Me,
    PersonalAccessToken,
    ReportList,
)
from acugis_report.services import (
    AuthService,
    DatasourcesService,
    EmbeddedMapsService,
    FoldersService,
    HealthService,
    MapReportsService,
    ReportsService,
    ReservedService,
    ResourcesService,
)


class ReportServer:
    """Python SDK for AcuGIS Report Server.

    Prefer namespaced services (``client.reports.list()``) in new code.
    Flat methods remain for backward compatibility. All CLI commands should
    call this client; do not duplicate HTTP logic.
    """

    def __init__(
        self,
        url: str,
        *,
        token: str | None = None,
        verify: bool = True,
        timeout: float = 60.0,
        transport: httpx.BaseTransport | None = None,
    ):
        self.base_url = (url or "").rstrip("/")
        if not self.base_url:
            raise ValidationError("Server URL is required")
        self.token = token or ""
        self.verify = verify
        from acugis_report import __version__

        headers: dict[str, str] = {
            "Accept": "application/json",
            "User-Agent": f"AcuGIS-Report-Python/{__version__}",
        }
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        # Do not pass base_url with a path segment — absolute request paths
        # would drop it (httpx). Resolve URLs explicitly instead.
        self._client = httpx.Client(
            headers=headers,
            verify=verify,
            timeout=timeout,
            follow_redirects=True,
            transport=transport,
        )

        # Service namespaces (idiomatic; thin wrappers over flat methods).
        self.auth = AuthService(self)
        self.health = HealthService(self)
        self.reports = ReportsService(self)
        self.folders = FoldersService(self)
        self.resources = ResourcesService(self)
        self.datasources = DatasourcesService(self)
        self.map_reports = MapReportsService(self)
        self.embedded_maps = EmbeddedMapsService(self)
        self.users = ReservedService("Users")
        self.groups = ReservedService("Groups")
        self.schedules = ReservedService("Schedules")
        self.jobs = ReservedService("Jobs")
        self.recipients = ReservedService("Recipients")

    def close(self) -> None:
        self._client.close()

    def __enter__(self) -> ReportServer:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    # ------------------------------------------------------------------ auth
    def login(self, username: str, password: str) -> dict[str, Any]:
        return self._request(
            "POST",
            "/api/v1/auth/login",
            json={"username": username, "password": password},
            auth_optional=True,
        )

    def logout(self) -> dict[str, Any]:
        return self._request("POST", "/api/v1/auth/logout", auth_optional=True)

    def me(self) -> Me:
        data = self._request("GET", "/api/v1/auth/me")
        return Me.model_validate(data)

    def list_tokens(self) -> list[PersonalAccessToken]:
        data = self._request("GET", "/api/v1/auth/tokens")
        tokens = data.get("tokens", data) if isinstance(data, dict) else data
        return [PersonalAccessToken.model_validate(t) for t in (tokens or [])]

    def create_token(
        self,
        name: str,
        *,
        description: str | None = None,
        expires_in: str = "never",
        expires_at: str | None = None,
    ) -> CreateTokenResponse:
        body: dict[str, Any] = {"name": name, "expires_in": expires_in}
        if description:
            body["description"] = description
        if expires_at:
            body["expires_at"] = expires_at
        data = self._request("POST", "/api/v1/auth/tokens", json=body)
        return CreateTokenResponse.model_validate(data)

    def delete_token(self, token_id: int, *, purge: bool = False) -> None:
        params = {"purge": "1"} if purge else None
        self._request("DELETE", f"/api/v1/auth/tokens/{token_id}", params=params, expect_json=False)

    def rotate_token(self, token_id: int) -> CreateTokenResponse:
        data = self._request("POST", f"/api/v1/auth/tokens/{token_id}/rotate")
        return CreateTokenResponse.model_validate(data)

    def login_and_create_cli_token(
        self,
        username: str,
        password: str,
        *,
        token_name: str = "acugis-report-cli",
    ) -> CreateTokenResponse:
        """Authenticate with password, then mint a PAT for subsequent Bearer auth."""
        self.login(username, password)
        return self.create_token(token_name, description="Created by acugis-report CLI")

    # --------------------------------------------------------------- health
    # Note: ``self.health`` is a HealthService namespace (callable for back-compat).
    # Prefer ``client.health.check()``. The former ``health()`` method is shadowed.

    def ready(self) -> dict[str, Any]:
        return self._request("GET", "/ready", auth_optional=True, raise_for_status=False)

    def diagnostics(self) -> dict[str, Any]:
        return self._request("GET", "/api/v1/diagnostics", auth_optional=True)

    def jasper_status(self) -> dict[str, Any]:
        return self._request("GET", "/dev/jasper-status", auth_optional=True)

    def doctor(self) -> dict[str, Any]:
        """Aggregate health, readiness, diagnostics, and Jasper status."""
        result: dict[str, Any] = {"checks": {}, "warnings": [], "ok": True}

        def _safe(name: str, fn: Any) -> None:
            try:
                result["checks"][name] = fn()
            except ReportServerError as exc:
                result["checks"][name] = {"error": exc.message, "status_code": exc.status_code}
                result["ok"] = False
                result["warnings"].append(f"{name}: {exc.message}")
            except Exception as exc:  # noqa: BLE001
                result["checks"][name] = {"error": str(exc)}
                result["ok"] = False
                result["warnings"].append(f"{name}: {exc}")

        _safe("health", self.health)
        _safe("ready", self.ready)
        _safe("diagnostics", self.diagnostics)
        _safe("jasper", self.jasper_status)

        ready = result["checks"].get("ready") or {}
        checks = ready.get("checks") if isinstance(ready, dict) else None
        if isinstance(checks, dict):
            for key, value in checks.items():
                ok = value in (True, "ok", "ready", "pass") or (
                    isinstance(value, dict) and value.get("status") in ("ok", "ready", "pass", True)
                )
                if not ok and value not in (True,):
                    # treat nested status
                    if isinstance(value, dict):
                        status = str(value.get("status", value.get("ok", ""))).lower()
                        if status in ("ok", "ready", "pass", "true"):
                            continue
                    result["warnings"].append(f"readiness:{key}")
                    result["ok"] = False

        return result

    # -------------------------------------------------------------- reports
    def list_reports(self) -> ReportList:
        data = self._request("GET", "/api/v1/reports")
        return ReportList.model_validate(data)

    def get_report(self, key: str) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/reports/{key}")

    def publish_report(
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
        data: dict[str, Any] = {
            "data_source": data_source,
            "report_type": report_type,
        }
        if report_name:
            data["report_name"] = report_name
        if report_description:
            data["report_description"] = report_description
        if extra_fields:
            data.update(dict(extra_fields))
        if access_groups:
            # Multipart field; server accepts access_groups[] repeats.
            # httpx dict form sends a single value — join for create, or use list.
            data["access_groups"] = access_groups[0] if len(access_groups) == 1 else access_groups

        files = self._multipart_files(
            jrxml_file=jrxml_file,
            jasper_file=jasper_file,
            thumbnail_file=thumbnail_file,
        )
        return self._request("POST", "/api/v1/reports", data=data, files=files)

    def update_report(
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
        data: dict[str, Any] = {}
        if data_source:
            data["data_source"] = data_source
        if report_name:
            data["report_name"] = report_name
        if report_description:
            data["report_description"] = report_description
        if report_type:
            data["report_type"] = report_type
        if access_groups:
            data["access_groups"] = access_groups[0] if len(access_groups) == 1 else access_groups

        files = self._multipart_files(
            jrxml_file=jrxml_file,
            jasper_file=jasper_file,
            thumbnail_file=thumbnail_file,
        )
        return self._request(
            "PUT",
            f"/api/v1/reports/{key}",
            data=data or None,
            files=files or None,
        )

    def delete_report(self, key: str) -> dict[str, Any]:
        return self._request("DELETE", f"/api/v1/reports/{key}")

    def report_config(self, key: str) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/reports/{key}/config")

    def report_thumbnail(self, key: str) -> bytes:
        return self._request_bytes("GET", f"/api/v1/reports/{key}/thumbnail")

    def parameter_lookup(
        self,
        key: str,
        name: str,
        *,
        params: Mapping[str, Any] | None = None,
    ) -> dict[str, Any]:
        if params:
            return self._request(
                "POST",
                f"/api/v1/reports/{key}/parameters/{name}/lookup",
                json={"params": dict(params)},
            )
        return self._request("GET", f"/api/v1/reports/{key}/parameters/{name}/lookup")

    def run_report(
        self,
        key: str,
        *,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
        method: str = "GET",
    ) -> tuple[bytes, str | None]:
        """Execute a report. Returns (content, content_type)."""
        fmt = format.lower().lstrip(".")
        if fmt not in REPORT_FORMATS:
            raise ValidationError(
                f"Unsupported format {format!r}. Supported: {', '.join(REPORT_FORMATS)}"
            )
        query: dict[str, Any] = {"_repName": key, "_repFormat": fmt}
        if params:
            for k, v in params.items():
                if isinstance(v, (list, tuple)):
                    # handled via multi via list of tuples below
                    continue
                query[k] = v
        # Build multi-value params
        param_pairs: list[tuple[str, Any]] = list(query.items())
        if params:
            for k, v in params.items():
                if isinstance(v, (list, tuple)):
                    for item in v:
                        param_pairs.append((k, item))

        if method.upper() == "POST":
            resp = self._raw_request("POST", "/reports/execute", params=param_pairs)
        else:
            resp = self._raw_request("GET", "/reports/execute", params=param_pairs)
        self._raise_for_response(resp, expect_json=False)
        return resp.content, resp.headers.get("content-type")

    def run_to_file(
        self,
        key: str,
        path: str | Path,
        *,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
        method: str = "GET",
    ) -> tuple[bytes, str | None]:
        """Execute a report and write bytes to ``path``. Returns (content, content_type)."""
        content, content_type = self.run_report(key, format=format, params=params, method=method)
        out = Path(path)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_bytes(content)
        return content, content_type

    # -------------------------------------------------------------- folders
    def folders_tree(self) -> Any:
        return self._request("GET", "/api/v1/folders/tree")

    def folders_browse(self, folder_id: int = 0) -> dict[str, Any]:
        return self._request("GET", "/api/v1/folders/browse", params={"folder_id": folder_id})

    def folders_search(self, q: str) -> dict[str, Any]:
        return self._request("GET", "/api/v1/folders/search", params={"q": q})

    def create_folder(
        self,
        name: str,
        *,
        parent_id: int = 0,
        description: str | None = None,
        icon: str | None = None,
        sort_order: int | None = None,
        acl_inherit: bool | None = None,
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"parent_id": parent_id, "name": name}
        if description is not None:
            body["description"] = description
        if icon is not None:
            body["icon"] = icon
        if sort_order is not None:
            body["sort_order"] = sort_order
        if acl_inherit is not None:
            body["acl_inherit"] = acl_inherit
        return self._request("POST", "/api/v1/folders", json=body)

    def get_folder(self, folder_id: int) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/folders/{folder_id}")

    def update_folder(self, folder_id: int, **fields: Any) -> dict[str, Any]:
        return self._request("PUT", f"/api/v1/folders/{folder_id}", json=fields)

    def delete_folder(self, folder_id: int, *, recursive: bool = False, confirm: bool = True) -> dict[str, Any]:
        params = {
            "recursive": "1" if recursive else "0",
            "confirm": "1" if confirm else "0",
        }
        return self._request("DELETE", f"/api/v1/folders/{folder_id}", params=params)

    def move_folder(self, folder_id: int, parent_id: int, *, sort_order: int | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {"parent_id": parent_id}
        if sort_order is not None:
            body["sort_order"] = sort_order
        return self._request("POST", f"/api/v1/folders/{folder_id}/move", json=body)

    def folder_permissions(self, folder_id: int) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/folders/{folder_id}/permissions")

    def set_folder_permissions(
        self,
        folder_id: int,
        group_names: list[str],
        permission: str,
    ) -> dict[str, Any]:
        return self._request(
            "PUT",
            f"/api/v1/folders/{folder_id}/permissions",
            json={"group_names": group_names, "permission": permission},
        )

    # ------------------------------------------------------------ resources
    def list_resources(
        self,
        *,
        folder_id: int | None = None,
        resource_type: str | None = None,
        q: str | None = None,
    ) -> dict[str, Any]:
        params: dict[str, Any] = {}
        if folder_id is not None:
            params["folder_id"] = folder_id
        if resource_type:
            params["resource_type"] = resource_type
        if q:
            params["q"] = q
        return self._request("GET", "/api/v1/resources", params=params or None)

    def upload_resource(
        self,
        file_path: str | Path,
        *,
        folder_id: int | None = None,
        display_name: str | None = None,
        resource_type: str | None = None,
    ) -> dict[str, Any]:
        path = Path(file_path)
        data: dict[str, str] = {}
        if folder_id is not None:
            data["folder_id"] = str(folder_id)
        if display_name:
            data["display_name"] = display_name
        if resource_type:
            data["resource_type"] = resource_type
        with path.open("rb") as fh:
            files = {"file": (path.name, fh, "application/octet-stream")}
            return self._request("POST", "/api/v1/resources", data=data or None, files=files)

    def get_resource(self, uuid: str) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/resources/{uuid}")

    def download_resource(self, uuid: str) -> bytes:
        return self._request_bytes("GET", f"/api/v1/resources/{uuid}/download")

    def replace_resource(self, uuid: str, file_path: str | Path) -> dict[str, Any]:
        path = Path(file_path)
        with path.open("rb") as fh:
            files = {"file": (path.name, fh, "application/octet-stream")}
            return self._request("PUT", f"/api/v1/resources/{uuid}/content", files=files)

    def delete_resource(self, uuid: str) -> dict[str, Any]:
        return self._request("DELETE", f"/api/v1/resources/{uuid}")

    def list_images(self, *, folder_id: int | None = None, q: str | None = None) -> dict[str, Any]:
        params: dict[str, Any] = {}
        if folder_id is not None:
            params["folder_id"] = folder_id
        if q:
            params["q"] = q
        return self._request("GET", "/api/v1/resources/images", params=params or None)

    # --------------------------------------------------------- datasources
    def list_datasources(self) -> list[dict[str, Any]]:
        data = self._request("GET", "/api/v1/datasources")
        if isinstance(data, list):
            return data
        return data.get("items", data.get("datasources", []))

    def create_datasource(self, **fields: Any) -> dict[str, Any]:
        # API expects form-encoded upsert
        form = {k: str(v) for k, v in fields.items() if v is not None}
        return self._request("POST", "/api/v1/datasources", data=form)

    def delete_datasource(self, datasource_id: str | int) -> dict[str, Any]:
        return self._request("DELETE", f"/api/v1/datasources/{datasource_id}")

    def datasources_context(self) -> dict[str, Any]:
        return self._request("POST", "/api/v1/datasources/context")

    def get_datasource(self, datasource_id: str | int) -> dict[str, Any]:
        """Reserved: activates when GET /api/v1/datasources/{id} exists."""
        try:
            return self._request("GET", f"/api/v1/datasources/{datasource_id}")
        except NotFoundError:
            raise NotImplementedByServerError("datasources show") from None
        except ReportServerError as exc:
            if exc.status_code == 405:
                raise NotImplementedByServerError("datasources show") from exc
            raise

    def update_datasource(self, datasource_id: str | int, **fields: Any) -> dict[str, Any]:
        """Reserved: activates when PUT /api/v1/datasources/{id} exists."""
        try:
            return self._request("PUT", f"/api/v1/datasources/{datasource_id}", json=fields)
        except NotFoundError:
            raise NotImplementedByServerError("datasources update") from None
        except ReportServerError as exc:
            if exc.status_code == 405:
                raise NotImplementedByServerError("datasources update") from exc
            raise

    # ---------------------------------------------------------- map reports
    def map_features(self, key: str, **params: Any) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/map/{key}/features", params=params or None)

    def map_extent(
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
        params: dict[str, Any] = {}
        if srid is not None:
            params["srid"] = srid
        if geometry:
            params["geometry"] = geometry
        for name, val in (("xmin", xmin), ("ymin", ymin), ("xmax", xmax), ("ymax", ymax)):
            if val is not None:
                params[name] = val
        return self._request("GET", f"/api/v1/map/{key}/extent", params=params)

    def map_columns(self, key: str) -> dict[str, Any]:
        return self._request("GET", f"/api/v1/map/{key}/columns")

    def run_map_report(
        self,
        key: str,
        *,
        geometry: str | None = None,
        srid: int | None = None,
        format: str = "pdf",
        params: Mapping[str, Any] | None = None,
    ) -> tuple[bytes, str | None]:
        run_params: dict[str, Any] = dict(params or {})
        if geometry:
            run_params["p_geometry"] = geometry
        if srid is not None:
            run_params["p_srid"] = srid
        return self.run_report(key, format=format, params=run_params)

    # ------------------------------------------------------- embedded maps
    def embedded_map_debug(self, report: str, *, compile: bool = False) -> dict[str, Any]:
        params: dict[str, Any] = {"report": report, "format": "json"}
        if compile:
            params["compile"] = "1"
        return self._request("GET", "/api/v1/embedded-map/debug", params=params)

    # ------------------------------------------------- future / reserved
    def users_list(self) -> Any:
        raise NotImplementedByServerError("users")

    def groups_list(self) -> Any:
        raise NotImplementedByServerError("groups")

    def schedules_list(self) -> Any:
        raise NotImplementedByServerError("schedules")

    def jobs_list(self) -> Any:
        raise NotImplementedByServerError("jobs")

    def recipients_list(self) -> Any:
        raise NotImplementedByServerError("recipients")

    # ----------------------------------------------------------- internals
    def _url(self, path: str) -> str:
        return f"{self.base_url}/{path.lstrip('/')}"

    @staticmethod
    def _multipart_files(
        *,
        jrxml_file: str | Path | None = None,
        jasper_file: str | Path | None = None,
        thumbnail_file: str | Path | None = None,
    ) -> dict[str, tuple[str, bytes, str]] | None:
        files: dict[str, tuple[str, bytes, str]] = {}
        if jrxml_file:
            path = Path(jrxml_file)
            files["jrxml_file"] = (path.name, path.read_bytes(), "application/xml")
        if jasper_file:
            path = Path(jasper_file)
            files["jasper_file"] = (path.name, path.read_bytes(), "application/octet-stream")
        if thumbnail_file:
            path = Path(thumbnail_file)
            files["thumbnail_file"] = (path.name, path.read_bytes(), "application/octet-stream")
        return files or None

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: Any = None,
        json: Any = None,
        data: Any = None,
        files: Any = None,
        auth_optional: bool = False,
        expect_json: bool = True,
        raise_for_status: bool = True,
    ) -> Any:
        resp = self._raw_request(
            method,
            path,
            params=params,
            json=json,
            data=data,
            files=files,
            auth_optional=auth_optional,
        )
        if raise_for_status:
            self._raise_for_response(resp, expect_json=expect_json)
        elif resp.status_code >= 400 and expect_json:
            # still try to parse body for ready endpoint etc.
            pass
        if not expect_json:
            return None
        if resp.status_code == 204 or not resp.content:
            return {}
        try:
            return resp.json()
        except json.JSONDecodeError as exc:
            if raise_for_status:
                raise InvalidResponseError(
                    "Invalid JSON response", status_code=resp.status_code, body=resp.text
                ) from exc
            return {"raw": resp.text, "status_code": resp.status_code}

    def _request_bytes(
        self,
        method: str,
        path: str,
        *,
        params: Any = None,
        json: Any = None,
        data: Any = None,
        files: Any = None,
    ) -> bytes:
        resp = self._raw_request(method, path, params=params, json=json, data=data, files=files)
        self._raise_for_response(resp, expect_json=False)
        return resp.content

    def _raw_request(
        self,
        method: str,
        path: str,
        *,
        params: Any = None,
        json: Any = None,
        data: Any = None,
        files: Any = None,
        auth_optional: bool = False,
    ) -> httpx.Response:
        headers: MutableMapping[str, str] = {}
        if not self.token and not auth_optional:
            # Allow unauthenticated call; server will 401
            pass
        try:
            return self._client.request(
                method,
                self._url(path),
                params=params,
                json=json,
                data=data,
                files=files,
                headers=headers or None,
            )
        except httpx.TimeoutException as exc:
            raise TimeoutError(f"Request timed out: {exc}") from exc
        except httpx.TransportError as exc:
            raise NetworkError(f"Network error: {exc}") from exc

    def _raise_for_response(self, resp: httpx.Response, *, expect_json: bool) -> None:
        if resp.status_code < 400:
            return
        message = _extract_error_message(resp)
        raise error_from_status(resp.status_code, message, body=_safe_body(resp))


def _extract_error_message(resp: httpx.Response) -> str:
    try:
        data = resp.json()
        if isinstance(data, dict):
            for key in ("error", "message", "detail"):
                if key in data and data[key]:
                    return str(data[key])
            if "errors" in data:
                return str(data["errors"])
    except Exception:  # noqa: BLE001
        pass
    text = (resp.text or "").strip()
    if text:
        return text[:500]
    return f"HTTP {resp.status_code}"


def _safe_body(resp: httpx.Response) -> object | None:
    try:
        return resp.json()
    except Exception:  # noqa: BLE001
        text = resp.text
        return text[:1000] if text else None
