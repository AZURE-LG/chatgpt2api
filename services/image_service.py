from __future__ import annotations

from datetime import datetime
import json
from pathlib import Path
from urllib.parse import quote

from PIL import Image, ImageOps, UnidentifiedImageError

from services.config import config

THUMBNAIL_SIZE = 360


def _public_asset_url(base_url: str, prefix: str, relative_path: str) -> str:
    quoted_path = quote(relative_path, safe="/")
    return f"{base_url.rstrip('/')}/{prefix.strip('/')}/{quoted_path}"


def _thumbnail_path(relative_path: str) -> Path:
    return config.image_thumbnails_dir / f"{relative_path}.jpg"


def _metadata_path(thumbnail_path: Path) -> Path:
    return thumbnail_path.with_name(f"{thumbnail_path.name}.json")


def _read_thumbnail_metadata(metadata_path: Path, source_path: Path) -> dict[str, int] | None:
    if not metadata_path.is_file() or metadata_path.stat().st_mtime < source_path.stat().st_mtime:
        return None
    try:
        data = json.loads(metadata_path.read_text(encoding="utf-8"))
    except Exception:
        return None
    width = data.get("width")
    height = data.get("height")
    if isinstance(width, int) and isinstance(height, int) and width > 0 and height > 0:
        return {"width": width, "height": height}
    return None


def _flatten_for_thumbnail(image: Image.Image) -> Image.Image:
    if image.mode in {"RGBA", "LA"} or "transparency" in image.info:
        rgba = image.convert("RGBA")
        background = Image.new("RGB", rgba.size, (255, 255, 255))
        background.paste(rgba, mask=rgba.getchannel("A"))
        return background
    return image.convert("RGB")


def _ensure_thumbnail(source_path: Path, relative_path: str) -> dict[str, object]:
    thumbnail_path = _thumbnail_path(relative_path)
    metadata_path = _metadata_path(thumbnail_path)
    source_mtime = source_path.stat().st_mtime

    if thumbnail_path.is_file() and thumbnail_path.stat().st_mtime >= source_mtime:
        metadata = _read_thumbnail_metadata(metadata_path, source_path)
        return {
            "thumbnail_rel": thumbnail_path.relative_to(config.image_thumbnails_dir).as_posix(),
            **(metadata or {}),
        }

    try:
        with Image.open(source_path) as raw_image:
            image = ImageOps.exif_transpose(raw_image)
            width, height = image.size
            thumbnail = _flatten_for_thumbnail(image)
            thumbnail.thumbnail((THUMBNAIL_SIZE, THUMBNAIL_SIZE), Image.Resampling.LANCZOS)
            thumbnail_path.parent.mkdir(parents=True, exist_ok=True)
            thumbnail.save(thumbnail_path, format="JPEG", quality=72, optimize=True)
            metadata_path.write_text(json.dumps({"width": width, "height": height}), encoding="utf-8")
            return {
                "thumbnail_rel": thumbnail_path.relative_to(config.image_thumbnails_dir).as_posix(),
                "width": width,
                "height": height,
            }
    except (OSError, UnidentifiedImageError):
        return {}


def list_images(base_url: str, start_date: str = "", end_date: str = "") -> dict[str, object]:
    config.cleanup_old_images()
    items = []
    root = config.images_dir
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(root).as_posix()
        parts = rel.split("/")
        day = "-".join(parts[:3]) if len(parts) >= 4 else datetime.fromtimestamp(path.stat().st_mtime).strftime("%Y-%m-%d")
        if start_date and day < start_date:
            continue
        if end_date and day > end_date:
            continue
        thumbnail = _ensure_thumbnail(path, rel)
        items.append({
            "name": path.name,
            "date": day,
            "size": path.stat().st_size,
            "url": _public_asset_url(base_url, "images", rel),
            "thumbnail_url": _public_asset_url(base_url, "image-thumbnails", str(thumbnail["thumbnail_rel"])) if thumbnail.get("thumbnail_rel") else "",
            "width": thumbnail.get("width"),
            "height": thumbnail.get("height"),
            "created_at": datetime.fromtimestamp(path.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S"),
        })
    items.sort(key=lambda item: str(item["created_at"]), reverse=True)
    groups: dict[str, list[dict[str, object]]] = {}
    for item in items:
        groups.setdefault(str(item["date"]), []).append(item)
    return {"items": items, "groups": [{"date": key, "items": value} for key, value in groups.items()]}
