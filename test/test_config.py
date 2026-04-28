from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path


class ConfigLoadingTests(unittest.TestCase):
    def setUp(self) -> None:
        from services import config as config_module

        self.config_module = config_module

    def test_load_settings_ignores_directory_env_path(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            base_dir = Path(tmp_dir)
            data_dir = base_dir / "data"
            env_dir = base_dir / ".env"
            os_auth_key = "env-auth"

            env_dir.mkdir()

            module = self.config_module
            old_base_dir = module.BASE_DIR
            old_data_dir = module.DATA_DIR
            old_env_file = module.ENV_FILE
            old_env_auth_key = module.os.environ.get("CHATGPT2API_AUTH_KEY")
            try:
                module.BASE_DIR = base_dir
                module.DATA_DIR = data_dir
                module.ENV_FILE = env_dir
                module.os.environ["CHATGPT2API_AUTH_KEY"] = os_auth_key

                settings = module._load_settings()

                self.assertEqual(settings.auth_key, os_auth_key)
                self.assertEqual(settings.refresh_account_interval_minute, 5)
            finally:
                module.BASE_DIR = old_base_dir
                module.DATA_DIR = old_data_dir
                module.ENV_FILE = old_env_file
                if old_env_auth_key is None:
                    module.os.environ.pop("CHATGPT2API_AUTH_KEY", None)
                else:
                    module.os.environ["CHATGPT2API_AUTH_KEY"] = old_env_auth_key

    def test_config_store_loads_and_updates_env_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            env_file = Path(tmp_dir) / ".env"
            env_file.write_text(
                "\n".join(
                    [
                        "CHATGPT2API_AUTH_KEY=file-auth",
                        "CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE=7",
                        "CHATGPT2API_LOG_LEVELS=warning,error",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            module = self.config_module
            env_keys = [
                "CHATGPT2API_AUTH_KEY",
                "CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE",
                "CHATGPT2API_IMAGE_RETENTION_DAYS",
                "CHATGPT2API_LOG_LEVELS",
            ]
            old_values = {key: os.environ.get(key) for key in env_keys}
            for key in env_keys:
                os.environ.pop(key, None)
            try:
                store = module.ConfigStore(env_file)

                self.assertEqual(store.auth_key, "file-auth")
                self.assertEqual(store.refresh_account_interval_minute, 7)
                self.assertEqual(store.log_levels, ["warning", "error"])

                store.update(
                    {
                        "refresh_account_interval_minute": 11,
                        "image_retention_days": 14,
                        "log_levels": ["debug", "error"],
                    }
                )

                saved = env_file.read_text(encoding="utf-8")
                self.assertIn("CHATGPT2API_REFRESH_ACCOUNT_INTERVAL_MINUTE=11", saved)
                self.assertIn("CHATGPT2API_IMAGE_RETENTION_DAYS=14", saved)
                self.assertIn("CHATGPT2API_LOG_LEVELS=debug,error", saved)
            finally:
                for key, value in old_values.items():
                    if value is None:
                        os.environ.pop(key, None)
                    else:
                        os.environ[key] = value


if __name__ == "__main__":
    unittest.main()
