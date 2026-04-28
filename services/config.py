from __future__ import annotations

from dataclasses import dataclass
import os
import re
import sys
from pathlib import Path
import time

from services.storage.base import StorageBackend

BASE_DIR = Path(__file__).resolve().parents[1]
DATA_DIR = BASE_DIR / "data"
ENV_FILE = BASE_DIR / ".env"
VERSION_FILE = BASE_DIR / "VERSION"

_ENV_KEY_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_EXTERNAL_ENV_KEYS = set(os.environ)

SETTING_ENV_KEYS = {
    "auth-key": "CHATGPT2API_AUTH_KEY",
    "base_url": "CHATGPT2API_BASE_URL",
    "proxy": "CHATGPT2API_PROXY",
    "refresh_account_interval_minute": "CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE",
    "image_retention_days": "CHATGPT2API_IMAGE_RETENTION_DAYS",
    "auto_remove_invalid_accounts": "CHATGPT2API_AUTO_REMOVE_INVALID_ACCOUNTS",
    "auto_remove_rate_limited_accounts": "CHATGPT2API_AUTO_REMOVE_RATE_LIMITED_ACCOUNTS",
    "log_levels": "CHATGPT2API_LOG_LEVELS",
}

PUBLIC_SETTING_KEYS = tuple(key for key in SETTING_ENV_KEYS if key != "auth-key")


@dataclass(frozen=True)
class LoadedSettings:
    auth_key: str
    refresh_account_interval_minute: int


def _normalize_auth_key(value: object) -> str:
    return str(value or "").strip()


def _is_invalid_auth_key(value: object) -> bool:
    return _normalize_auth_key(value) == ""


def _unquote_env_value(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        inner = value[1:-1]
        if value[0] == '"':
            return (
                inner.replace(r"\n", "\n")
                .replace(r"\r", "\r")
                .replace(r"\t", "\t")
                .replace(r"\"", '"')
                .replace(r"\\", "\\")
            )
        return inner
    for index, char in enumerate(value):
        if char == "#" and (index == 0 or value[index - 1].isspace()):
            return value[:index].rstrip()
    return value


def _parse_env_assignment(line: str) -> tuple[str, str] | None:
    stripped = line.strip()
    if not stripped or stripped.startswith("#"):
        return None
    if stripped.startswith("export "):
        stripped = stripped[len("export "):].lstrip()
    key, separator, value = stripped.partition("=")
    if not separator:
        return None
    key = key.strip()
    if not _ENV_KEY_RE.fullmatch(key):
        return None
    return key, _unquote_env_value(value)


def _read_env_object(path: Path, *, name: str) -> dict[str, str]:
    if not path.exists():
        return {}
    if path.is_dir():
        print(
            f"Warning: {name} at '{path}' is a directory, ignoring it and falling back to other configuration sources.",
            file=sys.stderr,
        )
        return {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except Exception:
        return {}
    data: dict[str, str] = {}
    for line in lines:
        assignment = _parse_env_assignment(line)
        if assignment is not None:
            key, value = assignment
            data[key] = value
    return data


def _load_env_file(path: Path) -> None:
    for key, value in _read_env_object(path, name=".env").items():
        os.environ.setdefault(key, value)


def _settings_from_env_values(values: dict[str, str]) -> dict[str, object]:
    settings: dict[str, object] = {}
    for setting_key, env_key in SETTING_ENV_KEYS.items():
        if env_key in values:
            settings[setting_key] = values[env_key]
    return settings


def _parse_bool(value: object) -> bool:
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "on"}
    return bool(value)


def _parse_log_levels(value: object) -> list[str]:
    if isinstance(value, str):
        items = value.split(",")
    elif isinstance(value, list):
        items = value
    else:
        return []
    allowed = {"debug", "info", "warning", "error"}
    return [level for item in items if (level := str(item or "").strip().lower()) in allowed]


def _stringify_env_value(value: object) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, list):
        return ",".join(str(item).strip() for item in value if str(item).strip())
    return str(value or "").strip()


def _format_env_value(value: str) -> str:
    if value == "":
        return ""
    if re.fullmatch(r"[A-Za-z0-9_./:@%+\-,]*", value):
        return value
    return '"' + value.replace("\\", "\\\\").replace('"', r"\"").replace("\n", r"\n") + '"'


def _format_env_assignment(key: str, value: str) -> str:
    return f"{key}={_format_env_value(value)}"


def _write_env_updates(path: Path, updates: dict[str, str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = path.read_text(encoding="utf-8").splitlines() if path.exists() and not path.is_dir() else []
    pending = dict(updates)
    next_lines: list[str] = []

    for line in lines:
        assignment = _parse_env_assignment(line)
        if assignment is None:
            next_lines.append(line)
            continue
        key, _ = assignment
        if key in pending:
            next_lines.append(_format_env_assignment(key, pending.pop(key)))
        else:
            next_lines.append(line)

    if pending:
        if next_lines and next_lines[-1].strip():
            next_lines.append("")
        for key, value in updates.items():
            if key in pending:
                next_lines.append(_format_env_assignment(key, value))

    path.write_text("\n".join(next_lines).rstrip() + "\n", encoding="utf-8")


def _apply_runtime_env(updates: dict[str, str]) -> None:
    for key, value in updates.items():
        if key not in _EXTERNAL_ENV_KEYS:
            os.environ[key] = value


def _load_settings() -> LoadedSettings:
    DATA_DIR.mkdir(parents=True, exist_ok=True)
    raw_env = _read_env_object(ENV_FILE, name=".env")
    auth_key = _normalize_auth_key(os.getenv("CHATGPT2API_AUTH_KEY") or raw_env.get("CHATGPT2API_AUTH_KEY"))
    if _is_invalid_auth_key(auth_key):
        raise ValueError(
            "❌ auth-key 未设置！\n"
            "请在环境变量 CHATGPT2API_AUTH_KEY 中设置，或者在 .env 中填写 CHATGPT2API_AUTH_KEY。"
        )

    try:
        refresh_interval = int(
            os.getenv("CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE")
            or raw_env.get("CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE", 5)
        )
    except (TypeError, ValueError):
        refresh_interval = 5

    return LoadedSettings(
        auth_key=auth_key,
        refresh_account_interval_minute=refresh_interval,
    )


class ConfigStore:
    def __init__(self, path: Path):
        self.path = path
        DATA_DIR.mkdir(parents=True, exist_ok=True)
        _load_env_file(self.path)
        self.data = self._load()
        self._storage_backend: StorageBackend | None = None
        if _is_invalid_auth_key(self.auth_key):
            raise ValueError(
                "❌ auth-key 未设置！\n"
                "请按以下任意一种方式解决：\n"
                "1. 在 Render 的 Environment 变量中添加：\n"
                "   CHATGPT2API_AUTH_KEY = your_real_auth_key\n"
                "2. 或者在 .env 中填写：\n"
                "   CHATGPT2API_AUTH_KEY=your_real_auth_key"
            )

    def _load(self) -> dict[str, object]:
        return _settings_from_env_values(_read_env_object(self.path, name=".env"))

    def _save(self) -> None:
        updates = {
            SETTING_ENV_KEYS[key]: _stringify_env_value(self.data.get(key, ""))
            for key in PUBLIC_SETTING_KEYS
            if key in self.data
        }
        _write_env_updates(self.path, updates)
        _apply_runtime_env(updates)

    def _setting_value(self, key: str, default: object = "") -> object:
        env_key = SETTING_ENV_KEYS[key]
        value = os.getenv(env_key)
        if value is not None:
            return value
        return self.data.get(key, default)

    @property
    def auth_key(self) -> str:
        return _normalize_auth_key(self._setting_value("auth-key"))

    @property
    def accounts_file(self) -> Path:
        return DATA_DIR / "accounts.json"

    @property
    def refresh_account_interval_minute(self) -> int:
        try:
            return int(self._setting_value("refresh_account_interval_minute", 5))
        except (TypeError, ValueError):
            return 5

    @property
    def image_retention_days(self) -> int:
        try:
            return max(1, int(self._setting_value("image_retention_days", 30)))
        except (TypeError, ValueError):
            return 30

    @property
    def auto_remove_invalid_accounts(self) -> bool:
        return _parse_bool(self._setting_value("auto_remove_invalid_accounts", False))

    @property
    def auto_remove_rate_limited_accounts(self) -> bool:
        return _parse_bool(self._setting_value("auto_remove_rate_limited_accounts", False))

    @property
    def log_levels(self) -> list[str]:
        return _parse_log_levels(self._setting_value("log_levels", ""))

    @property
    def images_dir(self) -> Path:
        path = DATA_DIR / "images"
        path.mkdir(parents=True, exist_ok=True)
        return path

    @property
    def image_thumbnails_dir(self) -> Path:
        path = DATA_DIR / "image_thumbnails"
        path.mkdir(parents=True, exist_ok=True)
        return path

    def cleanup_old_images(self) -> int:
        cutoff = time.time() - self.image_retention_days * 86400
        removed = 0
        for directory in (self.images_dir, self.image_thumbnails_dir):
            for path in directory.rglob("*"):
                if path.is_file() and path.stat().st_mtime < cutoff:
                    path.unlink()
                    removed += 1
            for path in sorted((p for p in directory.rglob("*") if p.is_dir()), key=lambda p: len(p.parts), reverse=True):
                try:
                    path.rmdir()
                except OSError:
                    pass
        return removed

    @property
    def base_url(self) -> str:
        return str(self._setting_value("base_url", "") or "").strip().rstrip("/")

    @property
    def app_version(self) -> str:
        try:
            value = VERSION_FILE.read_text(encoding="utf-8").strip()
        except FileNotFoundError:
            return "0.0.0"
        return value or "0.0.0"

    def get(self) -> dict[str, object]:
        data = dict(self.data)
        data["refresh_account_interval_minute"] = self.refresh_account_interval_minute
        data["image_retention_days"] = self.image_retention_days
        data["auto_remove_invalid_accounts"] = self.auto_remove_invalid_accounts
        data["auto_remove_rate_limited_accounts"] = self.auto_remove_rate_limited_accounts
        data["log_levels"] = self.log_levels
        data["proxy"] = self.get_proxy_settings()
        data["base_url"] = self.base_url
        data.pop("auth-key", None)
        return data

    def get_proxy_settings(self) -> str:
        return str(self._setting_value("proxy", "") or "").strip()

    def update(self, data: dict[str, object]) -> dict[str, object]:
        next_data = dict(self.data)
        next_data.update(dict(data or {}))
        self.data = next_data
        self._save()
        return self.get()

    def get_storage_backend(self) -> StorageBackend:
        """获取存储后端实例（单例）"""
        if self._storage_backend is None:
            from services.storage.factory import create_storage_backend
            self._storage_backend = create_storage_backend(DATA_DIR)
        return self._storage_backend


_load_env_file(ENV_FILE)

config = ConfigStore(ENV_FILE)
