from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from PIL import Image

from services import image_service


class ImageServiceTests(unittest.TestCase):
    def test_list_images_returns_cached_thumbnail_url(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_dir:
            base_dir = Path(tmp_dir)
            images_dir = base_dir / "images"
            thumbnails_dir = base_dir / "image_thumbnails"
            source_path = images_dir / "2026" / "04" / "28" / "large.png"
            source_path.parent.mkdir(parents=True)
            Image.new("RGB", (1200, 900), (120, 80, 40)).save(source_path)

            fake_config = SimpleNamespace(
                images_dir=images_dir,
                image_thumbnails_dir=thumbnails_dir,
                cleanup_old_images=lambda: 0,
            )

            with mock.patch.object(image_service, "config", fake_config):
                result = image_service.list_images("http://local.test")

            item = result["items"][0]
            self.assertEqual(item["url"], "http://local.test/images/2026/04/28/large.png")
            self.assertEqual(item["thumbnail_url"], "http://local.test/image-thumbnails/2026/04/28/large.png.jpg")
            self.assertEqual(item["width"], 1200)
            self.assertEqual(item["height"], 900)

            thumbnail_path = thumbnails_dir / "2026" / "04" / "28" / "large.png.jpg"
            self.assertTrue(thumbnail_path.is_file())
            with Image.open(thumbnail_path) as thumbnail:
                self.assertLessEqual(max(thumbnail.size), image_service.THUMBNAIL_SIZE)


if __name__ == "__main__":
    unittest.main()
