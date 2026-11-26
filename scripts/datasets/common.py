"""Shared deterministic ID, geometry, CSV, and manifest helpers."""
from __future__ import annotations
import csv
import hashlib
import json
import re
from pathlib import Path

SEED = 42
DATASET_NUMBERS = {
    "demo": 1, "dense": 2, "peak": 3, "sparse": 4, "slow_returns": 5,
    "city150": 6, "russia10_smoke": 7, "yoshkar_ola_city": 8, "russia10_full": 9,
}
OFFSETS = {
    "order": 10_000_000, "request": 10_000_000, "quote": 30_000_000, "option": 40_000_000, "trip": 50_000_000, "run": 60_000_000,
    "product": 100_000_000, "seller_sku": 200_000_000, "seller": 300_000_000,
    "city": 400_000_000, "pickup_point": 500_000_000, "batch": 600_000_000,
    "return": 700_000_000, "segment": 800_000_000, "node": 900_000_000,
    "district": 920_000_000, "category": 950_000_000, "tariff": 975_000_000,
}


def numeric_id(dataset_id: str, kind: str, index: int) -> str:
    """Return a digit-only string ID, namespaced by dataset and entity kind."""
    if dataset_id not in DATASET_NUMBERS or kind not in OFFSETS or index < 1:
        raise ValueError(f"invalid numeric ID request: {dataset_id}/{kind}/{index}")
    if kind in ("order", "request") and index >= 990_000_000:
        raise ValueError("order ID index exceeds reserved dataset range")
    return str(DATASET_NUMBERS[dataset_id] * 1_000_000_000 + OFFSETS[kind] + index)


def stable_id(prefix: str, key: str) -> str:
    """Compatibility helper; returns a deterministic digit-only string."""
    raw = hashlib.sha256(f"{prefix}|{key}".encode("utf-8")).digest()
    return str(int.from_bytes(raw[:8], "big"))


def is_numeric_id(value: object) -> bool:
    return isinstance(value, (str, int)) and re.fullmatch(r"[0-9]+", str(value)) is not None


def product_geometry(index: int) -> dict[str, str]:
    """Stable positive product dimensions in meters; no derived area or volume."""
    return {
        "width_m": f"{0.08 + (index * 17 % 72) / 100:.2f}",
        "height_m": f"{0.06 + (index * 23 % 94) / 100:.2f}",
        "depth_m": f"{0.04 + (index * 31 % 66) / 100:.2f}",
    }



def pickup_point_attributes(seed: int, point_id: str, storage_enabled: bool = False,
                            load_level: str | None = None, capacity_cells: int | None = None) -> dict[str, str]:
    """Stable demo attributes for a pickup point, shared by all dataset generators."""
    digest = hashlib.sha256(f"pvz|{seed}|{point_id}".encode("utf-8")).digest()
    rating_tenths = 10 + int.from_bytes(digest[:4], "big") % 41
    levels = ("низкая", "средняя", "высокая")
    load = load_level if load_level is not None else levels[digest[4] % len(levels)]
    if load not in levels:
        raise ValueError(f"invalid pickup point load level: {load}")
    capacity = capacity_cells if capacity_cells is not None else (100 if storage_enabled else 30 + digest[5] % 71)
    if not isinstance(capacity, int) or capacity < 1:
        raise ValueError("capacity_cells must be a positive integer")
    return {"rating": f"{rating_tenths / 10:.1f}", "load_level": load, "capacity_cells": str(capacity)}

def write_csv(path: Path, rows: list[dict], fields: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields, lineterminator="\n")
        w.writeheader()
        w.writerows(rows)


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def write_manifest(folder: Path, dataset_id: str, seed: int, files: list[Path], counts: dict, period: dict, start: dict) -> None:
    records = []
    for p in sorted(files):
        item = {"path": str(p.relative_to(folder)), "sha256": sha256(p)}
        if p.suffix == ".csv":
            with p.open(encoding="utf-8", newline="") as f:
                item["rows"] = max(0, sum(1 for _ in csv.reader(f)) - 1)
        records.append(item)
    (folder / "manifest.json").write_text(json.dumps({
        "schemaVersion": "2.0.0", "datasetId": dataset_id, "seed": seed,
        "period": period, "start": start, "counts": counts, "files": records,
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
