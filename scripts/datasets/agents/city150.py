#!/usr/bin/env python3
"""Generate and validate the normalized city150 synthetic workload."""
from __future__ import annotations
import argparse
import csv
import hashlib
import json
import math
import shutil
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "scripts" / "datasets"))
from common import numeric_id, product_geometry, sha256, write_manifest, pickup_point_attributes

OUT = ROOT / "datasets" / "city150"
SEED = 42
FIRST = date(2026, 11, 23)
DATASET = "city150"
CITY_ID = numeric_id(DATASET, "city", 1)


def write_csv(path: Path, rows, fields):
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields, lineterminator="\n")
        w.writeheader()
        w.writerows(rows)


def h(seed: int, key: str, field: str) -> int:
    return int.from_bytes(hashlib.sha256(f"{seed}|{key}|{field}".encode()).digest()[:8], "big")


def generate(seed: int = SEED, out: Path = OUT):
    out = Path(out)
    shutil.rmtree(out, ignore_errors=True)
    out.mkdir(parents=True)

    city_code = "yoshkar_ola"
    city_name = "Йошкар-Ола"
    timezone_name = "Europe/Moscow"
    city = [{"city_id": CITY_ID, "city_code": city_code, "city_name": city_name, "time_zone": timezone_name}]
    write_csv(out / "cities.csv", city, ["city_id", "city_code", "city_name", "time_zone"])

    districts = [{"district_id": numeric_id(DATASET, "district", i), "district_code": f"district-{i:02d}", "district_name": f"Район {i:02d}"} for i in range(1, 11)]
    write_csv(out / "districts.csv", districts, ["district_id", "district_code", "district_name"])

    points = []
    point_ids = []
    for i in range(1, 151):
        pid = numeric_id(DATASET, "pickup_point", i)
        point_ids.append(pid)
        angle = i * 2.399963229728653 % (2 * math.pi)
        radius = 0.003 + 0.025 * math.sqrt((i - 1) / 150)
        points.append({
            "pickup_point_id": pid, "city_id": CITY_ID, "point_code": f"city150-pvz-{i:03d}",
            "point_name": f"ПВЗ {i:03d}", "node_id": pid,
            "kind": "storage_pvz" if i <= 15 else "pickup_only", "active": "true", "mode": "physical",
            "accepts_orders": "true", "accepts_returns": "true",
            **pickup_point_attributes(seed, pid, i <= 15, "высокая" if i <= 30 else None),
            "district_id": numeric_id(DATASET, "district", (i - 1) % 10 + 1),
            "latitude": f"{56.62 + radius * math.sin(angle):.6f}",
            "longitude": f"{47.89 + radius * math.cos(angle):.6f}",
        })
    write_csv(out / "points.csv", points, list(points[0]))

    sellers = [{"seller_id": numeric_id(DATASET, "seller", i), "seller_code": f"city150-seller-{i:02d}", "seller_name": f"Продавец {i:02d}"} for i in range(1, 11)]
    write_csv(out / "sellers.csv", sellers, ["seller_id", "seller_code", "seller_name"])
    categories = [{"category_id": numeric_id(DATASET, "category", i), "category_code": code, "category_name": name} for i, (code, name) in enumerate((("home", "Дом"), ("beauty", "Красота"), ("electronics", "Электроника"), ("apparel", "Одежда")), 1)]
    write_csv(out / "categories.csv", categories, ["category_id", "category_code", "category_name"])
    seller_skus = []
    products = []
    for i in range(1, 101):
        seller_ix = (i - 1) // 10 + 1
        seller_id = numeric_id(DATASET, "seller", seller_ix)
        sku_id = numeric_id(DATASET, "seller_sku", i)
        product_id = numeric_id(DATASET, "product", i)
        category_ix = (i - 1) % 4 + 1
        category = categories[category_ix - 1]
        seller_skus.append({"seller_sku": sku_id, "seller_id": seller_id, "sku_value": f"sku-{i:03d}"})
        product = {
            "product_id": product_id, "seller_id": seller_id, "seller_sku": sku_id,
            "product_name": f"Синтетический товар {i:03d}", "category_id": category["category_id"],
            "weight_grams": 250 + i * 17 % 4500, "slots_per_unit": 1 + i % 3,
            "sale_price_minor": 1000 + i * 173 % 200000, "unit_cost_minor": 500 + i * 71 % 90000,
        }
        product.update(product_geometry(i))
        products.append(product)
    write_csv(out / "seller_skus.csv", seller_skus, ["seller_sku", "seller_id", "sku_value"])
    write_csv(out / "products.csv", products, list(products[0]))

    orders = []
    summaries = []
    first_day_peak = []
    order_ix = 0
    for day_ix in range(7):
        local_date = FIRST + timedelta(days=day_ix)
        for point_ix, point_id in enumerate(point_ids, 1):
            peak_count = 8 if day_ix == 0 and point_ix <= 30 else 0
            point_day_orders = []
            for seq in range(20):
                order_ix += 1
                request_id = numeric_id(DATASET, "order", order_ix)
                # Force 8/20 first-day orders for the first 30 points into the hour before 09:00.
                if peak_count and seq < peak_count:
                    minute = (seq * 7 + point_ix) % 60
                    hour = 8
                elif day_ix == 0 and point_ix == 1 and seq in (8, 9):
                    # Same timestamp for a concurrent-reservation witness, outside the peak window.
                    minute, hour = 10, 15
                else:
                    raw_minute = h(seed, request_id, "minute") % (12 * 60)
                    hour, minute = 9 + raw_minute // 60, raw_minute % 60
                local_dt = datetime(local_date.year, local_date.month, local_date.day, hour, minute, tzinfo=timezone(timedelta(hours=3)))
                product_ix = 1 + h(seed, request_id, "product") % 100
                row = {
                    "request_key": request_id, "created_at": local_dt.isoformat(timespec="seconds"),
                    "city_id": CITY_ID, "pickup_point_id": point_id,
                    "items_json": json.dumps([{"product_id": numeric_id(DATASET, "product", product_ix), "quantity": 1}], separators=(",", ":")),
                }
                orders.append(row)
                point_day_orders.append(row)
                if peak_count and seq < peak_count:
                    first_day_peak.append({"request_key": request_id, "pickup_point_id": point_id, "trip_id": numeric_id(DATASET, "trip", (point_ix - 1) // 10 + 1), "created_at": row["created_at"], "cutoff_local": "09:00"})
            summaries.append({
                "local_date": local_date.isoformat(), "pickup_point_id": point_id,
                "district_id": points[point_ix - 1]["district_id"], "order_count": len(point_day_orders),
                "orders_in_cutoff_hour": peak_count,
            })
    orders.sort(key=lambda r: (r["created_at"], int(r["request_key"])))
    write_csv(out / "orders.csv", orders, ["request_key", "created_at", "city_id", "pickup_point_id", "items_json"])
    write_csv(out / "point_day_summary.csv", summaries, ["local_date", "pickup_point_id", "district_id", "order_count", "orders_in_cutoff_hour"])

    network = []
    for i in range(149):
        network.append({
            "segment_id": numeric_id(DATASET, "segment", i + 1),
            "from_point_id": point_ids[i], "to_point_id": point_ids[i + 1],
            "trip_id": numeric_id(DATASET, "trip", i // 10 + 1), "segment_order": i % 10 + 1,
            "capacity_slots": 120, "cutoff_local": "09:00", "departure_local": "10:00", "daily": "true",
        })
    write_csv(out / "network.csv", network, list(network[0]))
    (out / "peak_events.json").write_text(json.dumps({
        "schemaVersion": "2.0.0", "day": FIRST.isoformat(), "cutoffLocal": "09:00",
        "orders": first_day_peak,
        "background": {"ordinaryUsedSlots": 0, "pickupUsedSlots": 0, "inboundReservedSlots": 0},
        "concurrentReservationWitness": {"requestKeys": [numeric_id(DATASET, "order", 9), numeric_id(DATASET, "order", 10)], "samePointId": point_ids[0], "sameCreatedAt": "2026-11-23T10:15:00+03:00"},
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (out / "api_examples.json").write_text(json.dumps([
        {"contract": "frontend.yaml", "schema": "CityPage", "direction": "response", "body": {"items": [{"id": CITY_ID, "name": city_name, "timeZone": timezone_name}]}},
        {"contract": "frontend.yaml", "schema": "PickupPointPage", "direction": "response", "body": {"items": [{"node": {"id": point_ids[0], "cityId": CITY_ID, "name": "ПВЗ 001", "kind": "storage_pvz", "active": True, "mode": "physical"}, "acceptsOrders": True, "acceptsReturns": True, "openingHours": [{"weekdays": [1, 2, 3, 4, 5, 6, 7], "opens": "09:00", "closes": "21:00"}]}]}},
        {"contract": "frontend.yaml", "schema": "ProductPage", "direction": "response", "body": {"items": [{"id": products[0]["product_id"], "sellerId": products[0]["seller_id"], "sellerSku": products[0]["seller_sku"], "name": products[0]["product_name"], "weightGrams": products[0]["weight_grams"], "slotsPerUnit": products[0]["slots_per_unit"], "salePriceMinor": products[0]["sale_price_minor"], "unitCostMinor": products[0]["unit_cost_minor"], "category": "home"}]}},
    ], ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    expected = {
        "schemaVersion": "2.0.0", "scenario": "city150", "seed": seed,
        "pointDayCombinations": 1050, "orderCount": 21000, "storagePointCount": 15,
        "pickupOnlyPointCount": 135, "ordersPerPointPerDay": 20, "days": 7,
        "checks": {"allPointDayPairsUnique": True, "everyPairOrderCount": 20, "allOrdersSingleLine": True,
                   "cutoffPeakEventPoints": 30, "eightOrdersBeforeCutoffPerPoint": True,
                   "sharedTimeReservationWitness": True, "networkSegmentCount": 149},
        "foreignKeys": {"orders.city_id": "cities.city_id", "orders.pickup_point_id": "points.pickup_point_id",
                        "orders.items_json[].product_id": "products.product_id", "products.seller_id": "sellers.seller_id",
                        "products.seller_sku": "seller_skus.seller_sku", "points.district_id": "districts.district_id"},
    }
    (out / "expected.json").write_text(json.dumps(expected, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    files = [p for p in out.iterdir() if p.is_file() and p.name != "manifest.json"]
    counts = {"pointCount": 150, "preparedOrderCount": 21000, "orderIntentCount": 21000, "lineCount": 21000,
              "orderCount": 21000, "storagePointCount": 15, "pointDayCount": 1050, "sellerCount": 10, "productCount": 100}
    start = {"cityId": CITY_ID, "pickupPointId": point_ids[0], "at": f"{FIRST.isoformat()}T07:30:00+03:00", "showcaseRequestKey": orders[0]["request_key"]}
    period = {"from": FIRST.isoformat(), "to": (FIRST + timedelta(days=7)).isoformat(), "days": 7, "timezone": timezone_name}
    write_manifest(out, DATASET, seed, files, counts, period, start)
    return validate(out)


def read_csv(path: Path):
    with path.open(encoding="utf-8", newline="") as f:
        yield from csv.DictReader(f)


def validate(out: Path = OUT):
    out = Path(out)
    expected_files = ["cities.csv", "districts.csv", "points.csv", "sellers.csv", "seller_skus.csv", "categories.csv", "products.csv", "orders.csv", "point_day_summary.csv", "network.csv", "peak_events.json", "api_examples.json", "expected.json"]
    manifest = json.loads((out / "manifest.json").read_text(encoding="utf-8"))
    for item in manifest["files"]:
        path = out / item["path"]
        assert path.is_file() and sha256(path) == item["sha256"], f"checksum: {path}"
        if "rows" in item:
            with path.open(encoding="utf-8", newline="") as f:
                assert sum(1 for _ in csv.reader(f)) - 1 == item["rows"], f"row count: {path}"
    assert all((out / name).is_file() for name in expected_files)
    points = list(read_csv(out / "points.csv"))
    sellers = list(read_csv(out / "sellers.csv"))
    seller_skus = list(read_csv(out / "seller_skus.csv"))
    products = list(read_csv(out / "products.csv"))
    cities = list(read_csv(out / "cities.csv"))
    districts = list(read_csv(out / "districts.csv"))
    orders = list(read_csv(out / "orders.csv"))
    summaries = list(read_csv(out / "point_day_summary.csv"))
    network = list(read_csv(out / "network.csv"))
    assert len(cities) == 1 and len(points) == 150 and len(sellers) == 10 and len(seller_skus) == 100 and len(products) == 100
    assert sum(p["kind"] == "storage_pvz" for p in points) == 15
    assert sum(p["kind"] == "pickup_only" for p in points) == 135
    assert len(orders) == 21000 and len(summaries) == 1050 and len(network) == 149
    id_columns = {
        "cities.csv": ["city_id"], "districts.csv": ["district_id"],
        "points.csv": ["pickup_point_id", "city_id", "node_id", "district_id"],
        "sellers.csv": ["seller_id"], "seller_skus.csv": ["seller_sku", "seller_id"],
        "categories.csv": ["category_id"], "products.csv": ["product_id", "seller_id", "seller_sku", "category_id"],
        "orders.csv": ["request_key", "city_id", "pickup_point_id"],
        "point_day_summary.csv": ["pickup_point_id", "district_id"],
        "network.csv": ["segment_id", "from_point_id", "to_point_id", "trip_id"],
    }
    for filename, columns in id_columns.items():
        for row in read_csv(out / filename):
            for col in columns:
                assert row[col].isdigit(), f"non-numeric ID {filename}.{col}={row[col]!r}"
    point_ids = {r["pickup_point_id"] for r in points}
    product_ids = {r["product_id"] for r in products}
    seller_ids = {r["seller_id"] for r in sellers}
    sku_ids = {r["seller_sku"] for r in seller_skus}
    city_ids = {r["city_id"] for r in cities}
    district_ids = {r["district_id"] for r in districts}
    assert all(p["city_id"] in city_ids and p["district_id"] in district_ids for p in points)
    assert all(p["seller_id"] in seller_ids and p["seller_sku"] in sku_ids and p["category_id"].isdigit() for p in products)
    assert all(len({r[k] for r in read_csv(out / fn)}) == len(list(read_csv(out / fn))) for fn, k in (("points.csv", "pickup_point_id"), ("orders.csv", "request_key"), ("products.csv", "product_id")))
    assert all(float(p[k]) > 0 for p in products for k in ("width_m", "height_m", "depth_m"))
    assert all(float(p[k]) < float("inf") for p in products for k in ("width_m", "height_m", "depth_m"))
    for row in orders:
        assert row["city_id"] in city_ids and row["pickup_point_id"] in point_ids
        items = json.loads(row["items_json"])
        assert len(items) == 1 and items[0]["quantity"] == 1 and items[0]["product_id"].isdigit() and items[0]["product_id"] in product_ids
    assert all(int(r["order_count"]) == 20 for r in summaries)
    assert sum(int(r["orders_in_cutoff_hour"]) == 8 for r in summaries if r["local_date"] == FIRST.isoformat()) == 30
    peak = json.loads((out / "peak_events.json").read_text(encoding="utf-8"))
    assert len(peak["orders"]) == 240 and all(e["pickup_point_id"].isdigit() and e["trip_id"].isdigit() for e in peak["orders"])
    examples = json.loads((out / "api_examples.json").read_text(encoding="utf-8"))
    assert all(e["contract"] == "frontend.yaml" and e["direction"] in ("request", "response") and e["schema"] for e in examples)
    return {"points": len(points), "storage_points": 15, "orders": len(orders), "point_days": len(summaries), "lines": len(orders), "sellers": len(sellers), "products": len(products), "numeric_ids": True, "geometry_fields": ["width_m", "height_m", "depth_m"], "valid": True}


def main():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    gen = sub.add_parser("generate")
    gen.add_argument("--seed", type=int, default=SEED)
    val = sub.add_parser("validate")
    args = parser.parse_args()
    print(json.dumps(generate(args.seed) if args.command == "generate" else validate(), ensure_ascii=False))


if __name__ == "__main__":
    main()
