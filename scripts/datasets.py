#!/usr/bin/env python3
"""Generate and validate deterministic synthetic dataset fixtures."""
from __future__ import annotations
import argparse
import csv
import hashlib
import json
import math
import re
import subprocess
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / "datasets"
AGENTS = ROOT / "scripts" / "datasets" / "agents"


def digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def csv_rows(path: Path):
    with path.open(encoding="utf-8-sig", newline="") as f:
        yield from csv.DictReader(f)


def numeric_id(value) -> bool:
    return isinstance(value, (str, int)) and re.fullmatch(r"[0-9]+", str(value)) is not None


def load_key_set(folder: Path, filename: str, field: str):
    path = folder / filename
    return {r[field] for r in csv_rows(path)} if path.exists() else None


def validate_dataset(folder: Path) -> list[str]:
    errors = []
    manifest_path = folder / "manifest.json"
    if not manifest_path.exists():
        return [f"{folder.name}: missing manifest.json"]
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    files = manifest.get("files", [])
    if isinstance(files, dict):
        files = [dict({"path": name}, **meta) for name, meta in files.items()]
    for rec in files:
        path = folder / rec["path"]
        if not path.exists():
            errors.append(f"{folder.name}: missing {rec['path']}")
            continue
        if digest(path) != rec.get("sha256"):
            errors.append(f"{folder.name}: checksum mismatch {rec['path']}")
        if rec.get("rows") is not None and path.suffix == ".csv":
            with path.open(encoding="utf-8-sig", newline="") as f:
                count = max(0, sum(1 for _ in csv.reader(f)) - 1)
            if count != rec["rows"]:
                errors.append(f"{folder.name}: row count mismatch {rec['path']}: {count} != {rec['rows']}")
    if not (folder / "expected.json").exists():
        errors.append(f"{folder.name}: missing expected.json")

    id_fields = {"id", "city_id", "pickup_point_id", "point_id", "node_id", "seller_id", "seller_sku",
                 "product_id", "request_key", "source_request_key", "order_id", "order_item_id", "batch_id",
                 "trip_id", "segment_id", "return_id", "district_id", "category_id", "quote_id", "option_id",
                 "shipment_id", "case_id", "hub_id", "sorting_center_id", "regional_hub_id"}
    tables = {}
    for path in folder.glob("*.csv"):
        try:
            rows = list(csv_rows(path))
        except Exception as exc:
            errors.append(f"{folder.name}: cannot parse {path.name}: {exc}")
            continue
        table = path.stem
        tables[table] = rows
        if table in ("cities", "points", "products", "sellers", "seller_skus", "categories", "districts", "nodes"):
            primary = {"cities":"city_id", "points":"pickup_point_id", "products":"product_id", "sellers":"seller_id",
                       "seller_skus":"seller_sku", "categories":"category_id", "districts":"district_id", "nodes":"node_id"}.get(table)
            if primary and primary in (rows[0] if rows else {}):
                ids = [r[primary] for r in rows]
                if len(ids) != len(set(ids)):
                    errors.append(f"{folder.name}: duplicate {primary} in {path.name}")
        for row_no, row in enumerate(rows, 2):
            for key, value in row.items():
                if key in id_fields or key.endswith("_id"):
                    if value and not numeric_id(value):
                        errors.append(f"{folder.name}/{path.name}:{row_no}: {key} is not digit-only: {value}")
                        break
        if table == "products":
            columns = set(rows[0]) if rows else set()
            if not {"width_m", "height_m", "depth_m"} <= columns:
                errors.append(f"{folder.name}: products missing width_m/height_m/depth_m")
            if any(c in columns for c in ("base_area_m2", "footprint_area_m2", "volume_m3", "area_m2")):
                errors.append(f"{folder.name}: derived area/volume columns must not be stored")
            for row_no, row in enumerate(rows, 2):
                try:
                    if any(not math.isfinite(float(row[x])) or float(row[x]) <= 0 for x in ("width_m", "height_m", "depth_m")):
                        errors.append(f"{folder.name}/products.csv:{row_no}: dimensions must be positive meters")
                        break
                except (ValueError, KeyError):
                    errors.append(f"{folder.name}/products.csv:{row_no}: invalid geometry")
                    break
        if table == "orders":
            for row_no, row in enumerate(rows, 2):
                try:
                    items = json.loads(row.get("items_json", "[]"))
                except Exception:
                    errors.append(f"{folder.name}/orders.csv:{row_no}: invalid items_json")
                    continue
                product_ids = [str(x.get("product_id", "")) for x in items]
                if any(not numeric_id(x) for x in product_ids) or len(product_ids) != len(set(product_ids)):
                    errors.append(f"{folder.name}/orders.csv:{row_no}: item product IDs must be unique digit strings")
                    break
                if any(not isinstance(x.get("quantity"), int) or x["quantity"] < 1 for x in items):
                    errors.append(f"{folder.name}/orders.csv:{row_no}: item quantity must be positive integer")
                    break

    # Check API and expectation JSON identifiers without treating display codes as IDs.
    json_id_fields = {"id", "cityId", "pickupPointId", "pointId", "nodeId", "sellerId", "sellerSku", "productId",
                      "requestKey", "orderId", "orderItemId", "batchId", "tripId", "segmentId", "returnId",
                      "districtId", "categoryId", "quoteId", "optionId", "shipmentId", "caseId", "hubId",
                      "sortingCenterId", "regionalHubId", "sourceRequestKey", "resourceId"}
    def walk_json(value, source):
        if isinstance(value, dict):
            for key, child in value.items():
                if key in json_id_fields and child is not None:
                    vals = child if isinstance(child, list) else [child]
                    if any(not numeric_id(x) for x in vals):
                        errors.append(f"{folder.name}/{source}: {key} must contain digit-only IDs")
                walk_json(child, source)
        elif isinstance(value, list):
            for child in value: walk_json(child, source)
    for path in folder.glob("*.json"):
        if path.name == "manifest.json": continue
        try: walk_json(json.loads(path.read_text(encoding="utf-8")), path.name)
        except Exception as exc: errors.append(f"{folder.name}/{path.name}: invalid JSON ({exc})")

    city_ids = load_key_set(folder, "cities.csv", "city_id")
    point_ids = load_key_set(folder, "points.csv", "pickup_point_id")
    product_ids = load_key_set(folder, "products.csv", "product_id")
    seller_ids = load_key_set(folder, "sellers.csv", "seller_id")
    seller_sku_ids = load_key_set(folder, "seller_skus.csv", "seller_sku")
    district_ids = load_key_set(folder, "districts.csv", "district_id")
    category_ids = load_key_set(folder, "categories.csv", "category_id")
    node_ids = load_key_set(folder, "nodes.csv", "node_id")
    if point_ids is not None and city_ids is not None:
        for row in tables.get("points", []):
            if row.get("city_id") not in city_ids:
                errors.append(f"{folder.name}: points.city_id FK missing")
                break
    for table, fk, target in (("points", "district_id", district_ids), ("points", "node_id", node_ids),
                              ("points", "regional_hub_id", node_ids), ("cities", "hub_id", node_ids),
                              ("cities", "sorting_center_id", node_ids), ("products", "seller_id", seller_ids),
                              ("products", "seller_sku", seller_sku_ids), ("products", "category_id", category_ids)):
        if target is not None:
            for row in tables.get(table, []):
                if row.get(fk) and row[fk] not in target:
                    errors.append(f"{folder.name}: {table}.{fk} FK missing")
                    break
    for row in tables.get("orders", []):
        if city_ids is not None and row.get("city_id") not in city_ids:
            errors.append(f"{folder.name}: orders.city_id FK missing"); break
        if point_ids is not None and row.get("pickup_point_id") not in point_ids:
            errors.append(f"{folder.name}: orders.pickup_point_id FK missing"); break
        if product_ids is not None:
            try: item_ids = [str(x["product_id"]) for x in json.loads(row.get("items_json", "[]"))]
            except Exception: continue
            if any(x not in product_ids for x in item_ids):
                errors.append(f"{folder.name}: order product FK missing"); break

    counts = manifest.get("counts", {})
    for table, count_field, fallback in (("orders", "orderCount", "preparedOrderCount"), ("points", "pointCount", None), ("products", "productCount", None)):
        if table in tables:
            expected = counts.get(count_field, counts.get(fallback) if fallback else None)
            if expected is not None and len(tables[table]) != expected:
                errors.append(f"{folder.name}: {table} row count {len(tables[table])} != {expected}")
    return errors


def cross_validate() -> list[str]:
    errors = []
    catalog_path = DATA / "catalog.json"
    if not catalog_path.exists():
        return ["missing datasets/catalog.json"]
    catalog = json.loads(catalog_path.read_text(encoding="utf-8"))
    entries = catalog.get("items", [])
    ids = [x.get("id") for x in entries]
    if len(ids) != len(set(ids)):
        errors.append("catalog has duplicate dataset IDs")
    required = {"id", "name", "summary", "status", "seed", "initialClock", "pointCount", "orderCount"}
    for item in entries:
        if set(item) != required:
            errors.append(f"catalog {item.get('id')}: invalid Dataset fields")
        if item.get("status") not in ("ready", "unavailable"):
            errors.append(f"catalog {item.get('id')}: invalid status")
        if item.get("status") == "ready":
            manifest_path = DATA / item["id"] / "manifest.json"
            if not manifest_path.exists():
                errors.append(f"catalog ready dataset missing: {item['id']}")
                continue
            counts = json.loads(manifest_path.read_text(encoding="utf-8")).get("counts", {})
            if item["pointCount"] != counts.get("pointCount") or item["orderCount"] != counts.get("orderCount", counts.get("preparedOrderCount")):
                errors.append(f"catalog {item['id']}: counts mismatch")
    if "russia10_full" in ids and next(x for x in entries if x["id"] == "russia10_full")["status"] == "ready":
        errors.append("russia10_full cannot be ready in repository catalog")
    shared_path = DATA / "shared" / "catalog_scenarios_1_5.json"
    if not shared_path.exists():
        errors.append("missing shared scenario 1-5 catalog")
    else:
        shared = json.loads(shared_path.read_text(encoding="utf-8"))
        if len(shared.get("sellers", [])) != 3 or len(shared.get("products", [])) != 12:
            errors.append("shared scenarios 1-5 catalog must contain 3 sellers and 12 products")
        classes = Counter((p.get("demand_class"), p.get("size_class")) for p in shared.get("products", []))
        expected_classes = {(demand, size): 2 for demand in ("high", "medium", "low") for size in ("compact", "bulky")}
        if classes != expected_classes:
            errors.append("shared scenarios 1-5 catalog must have two SKUs per demand/size class")
    for dataset in ("dense", "peak"):
        expected = DATA / dataset / "expected.json"
        if expected.exists():
            data = json.loads(expected.read_text(encoding="utf-8"))
            if data.get("demandCanonicalSha256") is None:
                errors.append(f"{dataset}: missing canonical demand checksum")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("generate"); g.add_argument("dataset"); g.add_argument("--seed", type=int, default=42)
    e = sub.add_parser("estimate"); e.add_argument("dataset", choices=["russia10_full"])
    v = sub.add_parser("validate"); v.add_argument("dataset", nargs="?")
    args = parser.parse_args()
    mapping = {
        "demo": ("demo_cases.py", ["generate"]),
        "dense": ("dense_peak.py", ["generate"]), "peak": ("dense_peak.py", ["generate"]),
        "sparse": ("sparse_demand.py", ["generate"]), "slow_returns": ("slow_returns.py", ["generate"]),
        "city150": ("city150.py", ["generate"]), "russia10_smoke": ("russia10.py", ["smoke"]),
        "yoshkar_ola_city": ("russia10.py", ["city", "--city", "yoshkar_ola", "--days", "1"]),
    }
    if args.cmd == "generate":
        if args.dataset == "russia10_full":
            print("Полный набор запускается явно вне репозитория: python3 scripts/datasets/agents/russia10.py full --seed 42 --days 7 --out /private/tmp/russia10_full")
            return 2
        if args.dataset not in mapping:
            print(f"unknown dataset: {args.dataset}"); return 2
        script, command = mapping[args.dataset]
        return subprocess.call([sys.executable, str(AGENTS / script), *command, "--seed", str(args.seed)], cwd=ROOT)
    if args.cmd == "estimate":
        return subprocess.call([sys.executable, str(AGENTS / "russia10.py"), "estimate"], cwd=ROOT)
    folders = [DATA / args.dataset] if args.dataset else sorted(p for p in DATA.iterdir() if p.is_dir() and (p / "manifest.json").exists())
    errors = []
    for folder in folders:
        errors.extend(validate_dataset(folder))
    if args.dataset is None:
        errors.extend(cross_validate())
    if errors:
        print("\n".join("ERROR " + e for e in errors))
        return 1
    print(f"validated {len(folders)} dataset(s): " + ", ".join(p.name for p in folders))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
