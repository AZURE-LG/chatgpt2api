from __future__ import annotations

import tempfile
import unittest
import os
from pathlib import Path
from unittest import mock

os.environ.setdefault("CHATGPT2API_AUTH_KEY", "chatgpt2api")

from fastapi.testclient import TestClient

import api.support as support_module
from api.app import create_app


class SpaFallbackTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.web_dist = Path(self.temp_dir.name)
        (self.web_dist / "assets").mkdir()
        (self.web_dist / "index.html").write_text("<div id=\"root\"></div>", encoding="utf-8")
        (self.web_dist / "assets" / "app.js").write_text("console.log('ok')", encoding="utf-8")

        self.web_dist_patcher = mock.patch.object(support_module, "WEB_DIST_DIR", self.web_dist)
        self.web_dist_patcher.start()
        self.addCleanup(self.web_dist_patcher.stop)

        self.client = TestClient(create_app())

    def test_frontend_route_falls_back_to_spa_index(self):
        response = self.client.get("/accounts")

        self.assertEqual(response.status_code, 200, response.text)
        self.assertEqual(response.text, "<div id=\"root\"></div>")

    def test_existing_asset_is_served_directly(self):
        response = self.client.get("/assets/app.js")

        self.assertEqual(response.status_code, 200, response.text)
        self.assertEqual(response.text, "console.log('ok')")

    def test_missing_asset_does_not_fall_back_to_spa_index(self):
        response = self.client.get("/assets/missing.js")

        self.assertEqual(response.status_code, 404)

    def test_missing_file_with_extension_does_not_fall_back_to_spa_index(self):
        response = self.client.get("/favicon.svg")

        self.assertEqual(response.status_code, 404)


if __name__ == "__main__":
    unittest.main()
