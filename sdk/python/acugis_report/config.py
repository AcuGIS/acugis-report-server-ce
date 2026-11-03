"""Local configuration and profile management."""

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

if sys.version_info >= (3, 11):
    import tomllib
else:
    import tomli as tomllib

import tomli_w


ENV_TOKEN = "ACUGIS_REPORT_TOKEN"
ENV_URL = "ACUGIS_REPORT_URL"
ENV_PROFILE = "ACUGIS_REPORT_PROFILE"
ENV_VERIFY_TLS = "ACUGIS_REPORT_VERIFY_TLS"


def default_config_path() -> Path:
    """Return the platform-specific config file path."""
    if sys.platform == "win32":
        base = Path(os.environ.get("APPDATA", Path.home() / "AppData" / "Roaming"))
        return base / "AcuGIS" / "Report" / "config.toml"
    xdg = os.environ.get("XDG_CONFIG_HOME")
    if xdg:
        return Path(xdg) / "acugis-report" / "config.toml"
    return Path.home() / ".config" / "acugis-report" / "config.toml"


class Profile(BaseModel):
    """A named connection profile."""

    server_url: str = ""
    token: str = ""
    token_name: str = ""
    token_id: int | None = None
    auth_type: str = "token"  # token | password (login still stores a PAT)
    verify_tls: bool = True


class AppConfig(BaseModel):
    """Root configuration document."""

    active_profile: str = "default"
    profiles: dict[str, Profile] = Field(default_factory=lambda: {"default": Profile()})


class ConfigStore:
    """Load and persist CLI configuration."""

    def __init__(self, path: Path | None = None):
        self.path = path or default_config_path()

    def load(self) -> AppConfig:
        if not self.path.exists():
            return AppConfig()
        raw = self.path.read_bytes()
        if not raw.strip():
            return AppConfig()
        data = tomllib.loads(raw.decode("utf-8"))
        return self._from_dict(data)

    def save(self, config: AppConfig) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        payload = self._to_dict(config)
        self.path.write_text(tomli_w.dumps(payload), encoding="utf-8")
        try:
            os.chmod(self.path, 0o600)
        except OSError:
            pass

    def get_profile(self, name: str | None = None) -> tuple[str, Profile]:
        config = self.load()
        profile_name = name or os.environ.get(ENV_PROFILE) or config.active_profile or "default"
        profile = config.profiles.get(profile_name)
        if profile is None:
            profile = Profile()
            config.profiles[profile_name] = profile
        return profile_name, profile

    def upsert_profile(self, name: str, **updates: Any) -> Profile:
        config = self.load()
        profile = config.profiles.get(name) or Profile()
        data = profile.model_dump()
        data.update({k: v for k, v in updates.items() if v is not None})
        profile = Profile(**data)
        config.profiles[name] = profile
        self.save(config)
        return profile

    def set_active(self, name: str) -> None:
        config = self.load()
        if name not in config.profiles:
            config.profiles[name] = Profile()
        config.active_profile = name
        self.save(config)

    def delete_profile(self, name: str) -> None:
        config = self.load()
        if name in config.profiles:
            del config.profiles[name]
        if config.active_profile == name:
            config.active_profile = next(iter(config.profiles), "default")
            if config.active_profile not in config.profiles:
                config.profiles["default"] = Profile()
                config.active_profile = "default"
        self.save(config)

    def list_profiles(self) -> tuple[str, dict[str, Profile]]:
        config = self.load()
        return config.active_profile, config.profiles

    def clear_credentials(self, name: str | None = None) -> None:
        profile_name, _ = self.get_profile(name)
        self.upsert_profile(profile_name, token="", token_name="", token_id=None)

    @staticmethod
    def _from_dict(data: dict[str, Any]) -> AppConfig:
        profiles_raw = data.get("profiles") or {}
        profiles: dict[str, Profile] = {}
        for name, pdata in profiles_raw.items():
            if isinstance(pdata, dict):
                profiles[name] = Profile(**pdata)
        if not profiles:
            # Flat legacy / single-profile style
            flat_keys = {"server_url", "token", "token_name", "token_id", "auth_type", "verify_tls"}
            if any(k in data for k in flat_keys):
                profiles["default"] = Profile(**{k: data[k] for k in flat_keys if k in data})
            else:
                profiles["default"] = Profile()
        active = data.get("active_profile") or "default"
        return AppConfig(active_profile=active, profiles=profiles)

    @staticmethod
    def _to_dict(config: AppConfig) -> dict[str, Any]:
        return {
            "active_profile": config.active_profile,
            "profiles": {name: p.model_dump(exclude_none=True) for name, p in config.profiles.items()},
        }


def resolve_connection(
    *,
    profile: str | None = None,
    server_url: str | None = None,
    token: str | None = None,
    verify_tls: bool | None = None,
    store: ConfigStore | None = None,
) -> tuple[str, Profile]:
    """Resolve effective connection settings from env, flags, and config."""
    store = store or ConfigStore()
    name, cfg = store.get_profile(profile)

    url = server_url or os.environ.get(ENV_URL) or cfg.server_url
    tok = token or os.environ.get(ENV_TOKEN) or cfg.token

    verify = cfg.verify_tls
    if verify_tls is not None:
        verify = verify_tls
    elif ENV_VERIFY_TLS in os.environ:
        verify = os.environ[ENV_VERIFY_TLS].lower() not in ("0", "false", "no", "off")

    resolved = Profile(
        server_url=(url or "").rstrip("/"),
        token=tok or "",
        token_name=cfg.token_name,
        token_id=cfg.token_id,
        auth_type=cfg.auth_type,
        verify_tls=verify,
    )
    return name, resolved
