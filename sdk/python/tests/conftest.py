"""Shared pytest fixtures."""

from __future__ import annotations

import json
from pathlib import Path

import httpx
import pytest
from typer.testing import CliRunner

from acugis_report.cli.app import app
from acugis_report.config import ConfigStore, Profile, AppConfig


@pytest.fixture
def runner() -> CliRunner:
    return CliRunner()


@pytest.fixture
def config_dir(tmp_path: Path) -> Path:
    return tmp_path / "config"


@pytest.fixture
def config_file(config_dir: Path) -> Path:
    path = config_dir / "config.toml"
    store = ConfigStore(path)
    store.save(
        AppConfig(
            active_profile="default",
            profiles={
                "default": Profile(
                    server_url="https://reports.example.com/go",
                    token="jrs_testtoken1234567890",
                    token_name="test-cli",
                    token_id=1,
                    auth_type="token",
                    verify_tls=True,
                )
            },
        )
    )
    return path


@pytest.fixture
def invoke(runner: CliRunner, config_file: Path):
    def _invoke(*args: str, env: dict | None = None):
        cmd = ["--config", str(config_file), *args]
        return runner.invoke(app, cmd, env=env, catch_exceptions=False)

    return _invoke


class MockTransport(httpx.MockTransport):
    """httpx mock transport with route map."""

    def __init__(self, routes: dict):
        self.routes = routes
        super().__init__(self._handler)

    def _handler(self, request: httpx.Request) -> httpx.Response:
        key = (request.method.upper(), request.url.path)
        # Also try without base path quirks
        if key not in self.routes:
            # match path only across methods variants
            for (m, p), resp in self.routes.items():
                if p == request.url.path and m in ("*", request.method.upper()):
                    key = (m, p)
                    break
        if key not in self.routes:
            return httpx.Response(404, json={"error": "not found"})
        spec = self.routes[key]
        if callable(spec):
            return spec(request)
        status = spec.get("status", 200)
        if "content" in spec:
            return httpx.Response(status, content=spec["content"], headers=spec.get("headers", {}))
        return httpx.Response(status, json=spec.get("json", {}), headers=spec.get("headers", {}))
