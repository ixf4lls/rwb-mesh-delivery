#!/usr/bin/env python3
"""Deterministic smoke and city slices for the synthetic Russia-10 workload."""
from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
import re
import shutil
import time
from collections import Counter
from datetime import date, datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
DATA = ROOT / "datasets"
import sys
sys.path.insert(0, str(ROOT / "scripts"))
from datasets.common import is_numeric_id, numeric_id, product_geometry, pickup_point_attributes
CITIES = [
    ("moscow", "Москва", "Europe/Moscow", 10_000),
    ("saint_petersburg", "Санкт-Петербург", "Europe/Moscow", 5_000),
    ("yekaterinburg", "Екатеринбург", "Asia/Yekaterinburg", 1_800),
    ("kazan", "Казань", "Europe/Moscow", 1_500),
    ("nizhny_novgorod", "Нижний Новгород", "Europe/Moscow", 1_200),
    ("samara", "Самара", "Europe/Samara", 900),
    ("cheboksary", "Чебоксары", "Europe/Moscow", 400),
    ("yoshkar_ola", "Йошкар-Ола", "Europe/Moscow", 150),
    ("volzhsk", "Волжск", "Europe/Moscow", 40),
    ("innopolis", "Иннополис", "Europe/Moscow", 1),
]
FIELDS = ["request_key", "created_at", "city_id", "pickup_point_id", "items_json"]
SEED = 42
START_DAY = date(2026, 1, 5)


def h(seed: int, key: str, field: str) -> int:
    return int.from_bytes(hashlib.sha256(f"{seed}|{key}|{field}".encode()).digest()[:8], "big")


def digest(path: Path) -> str:
    x = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            x.update(chunk)
    return x.hexdigest()


CITY_RANK = {city[0]: i for i, city in enumerate(CITIES, 1)}
POINT_START = {}
_point_cursor = 0
for _city_id, _, _, _count in CITIES:
    POINT_START[_city_id] = _point_cursor
    _point_cursor += _count


def point_id(dataset_id: str, city_id: str, n: int) -> str:
    return numeric_id(dataset_id, "pickup_point", POINT_START[city_id] + n)


def order_rows(dataset_id: str, city_id: str, city_numeric_id: str, pickup_point_numeric_id: str,
               city_name: str, timezone_name: str, point_no: int, day_no: int, seed: int):
    pkey = f"{CITY_RANK[city_id]}|{point_no}|{day_no}"
    count = 150 + h(seed, pkey, "daily_count") % 101
    for seq in range(count):
        # Numeric stable source key shared by smoke/city slices; no display code is packed into an ID.
        key = str(CITY_RANK[city_id] * 10_000_000_000 + day_no * 100_000_000 + point_no * 1_000 + seq + 1)
        lines_n = 1 + h(seed, key, "line_count") % 5
        sku_ids = sorted({1 + h(seed, key, f"sku_{j}") % 20_000 for j in range(lines_n)})
        # Resolve collisions deterministically while preserving the requested line count.
        fill_attempt = 0
        while len(sku_ids) < lines_n:
            candidate = 1 + h(seed, key, f"sku_fill_{len(sku_ids)}_{fill_attempt}") % 20_000
            fill_attempt += 1
            if candidate not in sku_ids:
                sku_ids.append(candidate)
                sku_ids.sort()
        items = [{"product_id": numeric_id(dataset_id, "product", sku), "quantity": 1} for sku in sku_ids]
        minute = h(seed, key, "minute") % 1440
        # Mild daytime/evening peaks by mapping a stable quantile through weighted windows.
        quantile = h(seed, key, "daypart") % 100
        if quantile < 35:
            minute = 9 * 60 + h(seed, key, "peak_day") % (5 * 60)
        elif quantile < 75:
            minute = 17 * 60 + h(seed, key, "peak_evening") % (5 * 60)
        day = START_DAY + timedelta(days=day_no)
        # API timestamps are RFC 3339; local zone is retained as the correct UTC offset.
        # All currently configured zones use their standard offsets on the January fixture date.
        offsets = {"Europe/Moscow": "+03:00", "Europe/Samara": "+04:00", "Asia/Yekaterinburg": "+05:00"}
        local = datetime(day.year, day.month, day.day, tzinfo=timezone(timedelta(hours=int(offsets[timezone_name][1:3])))) + timedelta(minutes=minute)
        yield {"request_key": key, "created_at": local.isoformat(timespec="seconds"), "city_id": city_numeric_id,
               "pickup_point_id": pickup_point_numeric_id, "items_json": json.dumps(items, separators=(",", ":"))}


def return_intent(row: dict, seed: int):
    """A stable buyer intention; execution must wait for actual issued Order.id/line IDs."""
    key = row["request_key"]
    if h(seed, key, "return_intent") % 30 != 0:
        return None
    items = json.loads(row["items_json"])
    if len(items) == 1 or h(seed, key, "return_all") % 100 >= 80:
        chosen = items
    else:
        chosen = [items[i] for i in range(len(items)) if h(seed, key, f"return_line_{i}") % 2]
        if not chosen:
            chosen = [items[h(seed, key, "return_fallback") % len(items)]]
        if len(chosen) == len(items):
            chosen = chosen[:-1]
    return {"request_key": key, "delay_days_after_issued": 1 + h(seed, key, "return_delay") % 14,
            "requested_product_ids_json": json.dumps([x["product_id"] for x in chosen], separators=(",", ":")),
            "return_point_id": row["pickup_point_id"], "inspection_outcome": "quarantine" if h(seed, key, "return_condition") % 10 < 2 else "resellable"}


def generate(out_dir: Path, cities, max_points: int | None, days: int, seed: int):
    dataset_id = out_dir.name
    start_clock = "2026-01-05T00:00:00+03:00"
    if dataset_id not in ("russia10_smoke", "yoshkar_ola_city", "russia10_full"):
        raise ValueError(f"unsupported dataset ID namespace: {dataset_id}")
    if out_dir.exists():
        shutil.rmtree(out_dir)
    out_dir.mkdir(parents=True)
    counts_by_city = {}
    city_ids = {c[0]: numeric_id(dataset_id, "city", CITY_RANK[c[0]]) for c in cities}
    city_positions = {c[0]: CITY_RANK[c[0]] for c in cities}
    hub_group = {"moscow": 1, "saint_petersburg": 2, "yekaterinburg": 3}
    for city_id, _, _, _ in CITIES:
        if city_id not in hub_group:
            hub_group[city_id] = 1
    hub_ids = {group: numeric_id(dataset_id, "node", group) for group in (1, 2, 3)}
    sc_ids = {c[0]: numeric_id(dataset_id, "node", 3 + CITY_RANK[c[0]]) for c in cities}
    category_names = ["home", "beauty", "electronics", "apparel"]
    category_ids = {name: numeric_id(dataset_id, "category", i) for i, name in enumerate(category_names, 1)}
    paths = {name: out_dir / f"{name}.csv" for name in (
        "cities", "nodes", "districts", "points", "sellers", "seller_skus",
        "categories", "products", "orders", "return_intents")}
    counts_by_city = {c[0]: min(c[3], max_points) if max_points is not None else c[3] for c in cities}
    point_rows = 0
    with paths["cities"].open("w", encoding="utf-8", newline="") as cf, \
         paths["nodes"].open("w", encoding="utf-8", newline="") as nf, \
         paths["districts"].open("w", encoding="utf-8", newline="") as df, \
         paths["points"].open("w", encoding="utf-8", newline="") as pf:
        cw = csv.DictWriter(cf, ["city_id", "city_code", "city_name", "time_zone", "hub_id", "sorting_center_id"], lineterminator="\n"); cw.writeheader()
        nw = csv.DictWriter(nf, ["node_id", "city_id", "node_code", "node_name", "kind", "mode"], lineterminator="\n"); nw.writeheader()
        dw = csv.DictWriter(df, ["district_id", "city_id", "district_code", "district_name"], lineterminator="\n"); dw.writeheader()
        pw = csv.DictWriter(pf, ["pickup_point_id", "city_id", "point_code", "point_name", "point_no", "district_id", "latitude", "longitude", "storage_enabled", "regional_hub_id", "rating", "load_level", "capacity_cells"], lineterminator="\n"); pw.writeheader()
        active_hubs = sorted({hub_group[c[0]] for c in cities})
        representative = {group: next(c[0] for c in cities if hub_group[c[0]] == group) for group in active_hubs}
        for group in active_hubs:
            city_key = representative[group]
            nw.writerow({"node_id": hub_ids[group], "city_id": city_ids[city_key], "node_code": f"hub-{group}",
                         "node_name": f"Региональный хаб {group}", "kind": "hub", "mode": "simulated"})
        for city_id, name, tz, _ in cities:
            city_fk = city_ids[city_id]
            hub_fk = hub_ids[hub_group[city_id]]
            sc_fk = sc_ids[city_id]
            cw.writerow({"city_id": city_fk, "city_code": city_id, "city_name": name, "time_zone": tz,
                         "hub_id": hub_fk, "sorting_center_id": sc_fk})
            nw.writerow({"node_id": sc_fk, "city_id": city_fk, "node_code": f"sc-{city_id}",
                         "node_name": f"Сортировочный центр — {name}", "kind": "sorting_center", "mode": "simulated"})
            npoints = counts_by_city[city_id]
            storage_n = max(1, math.floor(npoints * .10 + .5))
            district_width = max(1, math.ceil(npoints / 10))
            for d_no in range(1, 11):
                d_id = numeric_id(dataset_id, "district", (CITY_RANK[city_id] - 1) * 10 + d_no)
                dw.writerow({"district_id": d_id, "city_id": city_fk, "district_code": f"{city_id}-d{d_no:02d}",
                             "district_name": f"Район {d_no:02d} — {name}"})
            for n in range(1, npoints + 1):
                angle = (n * 2.399963229728653) % (2 * math.pi)
                radius = .003 + .025 * math.sqrt((n - 1) / max(npoints, 1))
                lat0 = 55.75 + (h(seed, city_id, "lat") % 1000 - 500) / 10000
                lon0 = 49.1 + (h(seed, city_id, "lon") % 1000 - 500) / 10000
                district_no = 1 + (n - 1) // district_width
                district_fk = numeric_id(dataset_id, "district", (CITY_RANK[city_id] - 1) * 10 + district_no)
                point_no_global = POINT_START[city_id] + n
                pw.writerow({"pickup_point_id": point_id(dataset_id, city_id, n), "city_id": city_fk,
                             "point_code": f"{city_id}-p{n:05d}", "point_name": f"ПВЗ {n:05d} — {name}",
                             "point_no": n, "district_id": district_fk,
                             "latitude": f"{lat0 + radius*math.sin(angle):.6f}", "longitude": f"{lon0 + radius*math.cos(angle):.6f}",
                             "storage_enabled": str(n <= storage_n).lower(), "regional_hub_id": hub_fk, **pickup_point_attributes(seed, point_id(dataset_id, city_id, n), n <= storage_n)})
                point_rows += 1
    with paths["sellers"].open("w", encoding="utf-8", newline="") as sf, \
         paths["seller_skus"].open("w", encoding="utf-8", newline="") as skf, \
         paths["categories"].open("w", encoding="utf-8", newline="") as cf, \
         paths["products"].open("w", encoding="utf-8", newline="") as pf:
        sw = csv.DictWriter(sf, ["seller_id", "seller_code", "seller_name"], lineterminator="\n"); sw.writeheader()
        skw = csv.DictWriter(skf, ["seller_sku", "seller_id", "sku_value"], lineterminator="\n"); skw.writeheader()
        ctw = csv.DictWriter(cf, ["category_id", "category_code", "category_name"], lineterminator="\n"); ctw.writeheader()
        pw = csv.DictWriter(pf, ["product_id", "seller_id", "seller_sku", "product_name", "category_id", "weight_grams", "slots_per_unit", "sale_price_minor", "unit_cost_minor", "width_m", "height_m", "depth_m"], lineterminator="\n"); pw.writeheader()
        for cat_no, cat in enumerate(category_names, 1):
            ctw.writerow({"category_id": category_ids[cat], "category_code": cat, "category_name": cat.title()})
        for seller_no in range(1, 201):
            sw.writerow({"seller_id": numeric_id(dataset_id, "seller", seller_no), "seller_code": f"seller-{seller_no:03d}", "seller_name": f"Продавец {seller_no:03d}"})
        for sku in range(1, 20_001):
            seller_no = 1 + (sku - 1) // 100
            seller_fk = numeric_id(dataset_id, "seller", seller_no)
            seller_sku_fk = numeric_id(dataset_id, "seller_sku", sku)
            geom = product_geometry(sku)
            sw_sku = f"sku-{sku:05d}"
            skw.writerow({"seller_sku": seller_sku_fk, "seller_id": seller_fk, "sku_value": sw_sku})
            cat = category_names[sku % len(category_names)]
            pw.writerow({"product_id": numeric_id(dataset_id, "product", sku), "seller_id": seller_fk,
                         "seller_sku": seller_sku_fk, "product_name": f"Синтетический товар {sku:05d}",
                         "category_id": category_ids[cat], "weight_grams": 100 + (sku * 97) % 4900,
                         "slots_per_unit": 1 + (sku % 4 == 0), "sale_price_minor": 5000 + (sku * 173) % 200000,
                         "unit_cost_minor": 2500 + (sku * 71) % 90000, **geom})
    order_count = sum(150 + h(seed, f"{CITY_RANK[c]}|{p}|{d}", "daily_count") % 101
                      for c in counts_by_city for p in range(1, counts_by_city[c] + 1) for d in range(days))
    line_count = 0
    intent_count = 0
    with paths["orders"].open("w", encoding="utf-8", newline="") as f, paths["return_intents"].open("w", encoding="utf-8", newline="") as rf:
        w = csv.DictWriter(f, FIELDS, lineterminator="\n"); w.writeheader()
        rw = csv.DictWriter(rf, ["return_id", "request_key", "delay_days_after_issued", "requested_product_ids_json", "return_point_id", "inspection_outcome"], lineterminator="\n"); rw.writeheader()
        for city_id, name, tz, _ in cities:
            city_fk = city_ids[city_id]
            for p_no in range(1, counts_by_city[city_id] + 1):
                point_fk = point_id(dataset_id, city_id, p_no)
                for d_no in range(days):
                    for row in order_rows(dataset_id, city_id, city_fk, point_fk, name, tz, p_no, d_no, seed):
                        w.writerow(row)
                        line_count += len(json.loads(row["items_json"]))
                        intent = return_intent(row, seed)
                        if intent:
                            intent_count += 1
                            intent["return_id"] = numeric_id(dataset_id, "return", intent_count)
                            rw.writerow(intent)
    files = [paths[x] for x in paths]
    row_counts = {"cities.csv": len(cities), "nodes.csv": len(cities) + len({hub_group[c[0]] for c in cities}), "districts.csv": len(cities) * 10,
                  "points.csv": point_rows, "sellers.csv": 200, "seller_skus.csv": 20_000,
                  "categories.csv": len(category_names), "products.csv": 20_000,
                  "orders.csv": order_count, "return_intents.csv": intent_count}
    manifest = {"schemaVersion": "2.0.0", "datasetId": dataset_id, "seed": seed,
      "period": {"startLocalDate": START_DAY.isoformat(), "days": days, "timezone": "per-city"},
      "start": {"cityId": city_ids.get("yoshkar_ola", next(iter(city_ids.values()))),
                "pickupPointId": point_id(dataset_id, "yoshkar_ola", 1) if "yoshkar_ola" in counts_by_city else point_id(dataset_id, cities[0][0], 1),
                "initialClock": start_clock, "showcaseRequestKey": str(CITY_RANK.get("yoshkar_ola", CITY_RANK[cities[0][0]]) * 10_000_000_000 + 1_001)} ,
      "counts": {"pointCount": point_rows, "preparedOrderCount": order_count, "orderIntentCount": intent_count,
                 "lineCount": line_count, "cityPointCounts": counts_by_city,
                 "storagePointCount": sum(max(1, math.floor(n * .10 + .5)) for n in counts_by_city.values()),
                 "pointDayCount": point_rows * days, "sellerCount": 200, "productCount": 20_000},
      "files": [{"path": p.name, "sha256": digest(p), "rows": row_counts[p.name]} for p in files],
      "notes": ["Synthetic workload parameters; not operational geography.", "CSV used because Parquet writer is unavailable.", "All entity IDs are digit-only strings; names/codes are normalized dimensions."]}
    (out_dir / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    expected = {"validation": {"cityCount": len(cities), "pointCount": point_rows, "pointDayCount": point_rows * days,
                "orderCount": order_count, "returnIntentCount": intent_count, "lineCount": line_count,
                "minOrdersPerPointDay": 150, "maxOrdersPerPointDay": 250, "minLinesPerOrder": 1, "maxLinesPerOrder": 5},
                "returnIntents": "1/30 per numeric stable request key; actual return execution resolves OrderLine.id in the selected DemoRun",
                "smokeAndCityOverlap": "same numeric request_key and canonical basket for overlapping Yoshkar-Ola points/days"}
    (out_dir / "expected.json").write_text(json.dumps(expected, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return manifest


def read_csv(path):
    with path.open(encoding="utf-8", newline="") as f:
        yield from csv.DictReader(f)


def validate(folder: Path):
    m = json.loads((folder / "manifest.json").read_text(encoding="utf-8"))
    for item in m["files"]:
        p = folder / item["path"]
        assert digest(p) == item["sha256"], f"checksum mismatch: {p}"
        with p.open(encoding="utf-8", newline="") as f:
            assert sum(1 for _ in f) - 1 == item["rows"], f"row count mismatch: {p}"
    cities = list(read_csv(folder / "cities.csv"))
    points = list(read_csv(folder / "points.csv"))
    products = list(read_csv(folder / "products.csv"))
    sellers = list(read_csv(folder / "sellers.csv"))
    seller_skus = list(read_csv(folder / "seller_skus.csv"))
    categories = list(read_csv(folder / "categories.csv"))
    districts = list(read_csv(folder / "districts.csv"))
    nodes = list(read_csv(folder / "nodes.csv"))
    city_ids = {r["city_id"] for r in cities}
    point_to_city = {r["pickup_point_id"]: r["city_id"] for r in points}
    point_meta = {r["pickup_point_id"]: r for r in points}
    product_ids = {r["product_id"] for r in products}
    seller_ids = {r["seller_id"] for r in sellers}
    sku_ids = {r["seller_sku"] for r in seller_skus}
    category_ids = {r["category_id"] for r in categories}
    district_ids = {r["district_id"] for r in districts}
    node_ids = {r["node_id"] for r in nodes}
    assert len(point_to_city) == len(points) == m["counts"]["pointCount"]
    assert len(product_ids) == len(products) == m["counts"]["productCount"]
    assert len(seller_ids) == len(sellers) == m["counts"]["sellerCount"]
    assert all(is_numeric_id(x) for group in (city_ids, point_to_city, product_ids, seller_ids, sku_ids, category_ids, district_ids, node_ids) for x in group)
    assert all(r["city_id"] in city_ids and r["district_id"] in district_ids and r["regional_hub_id"] in node_ids for r in points)
    assert all(r["city_id"] in city_ids and r["hub_id"] in node_ids and r["sorting_center_id"] in node_ids for r in cities)
    assert all(r["seller_id"] in seller_ids and r["seller_sku"] in sku_ids and r["category_id"] in category_ids for r in products)
    for product in products:
        assert all(float(product[k]) > 0 for k in ("width_m", "height_m", "depth_m"))
    assert {p["city_id"] for p in points} <= city_ids
    counts = Counter()
    line_count = order_count = intent_count = 0
    previous_order_tuple = None
    last_group = None
    expected_seq = 0
    next_intents = iter(read_csv(folder / "return_intents.csv"))
    intent = next(next_intents, None)
    city_by_id = {r["city_id"]: r["city_code"] for r in cities}
    city_rank = {c[0]: i for i, c in enumerate(CITIES, 1)}
    for row in read_csv(folder / "orders.csv"):
        assert is_numeric_id(row["request_key"]) and is_numeric_id(row["city_id"]) and is_numeric_id(row["pickup_point_id"])
        request_key = int(row["request_key"])
        assert row["city_id"] in city_ids and row["pickup_point_id"] in point_to_city
        assert point_to_city[row["pickup_point_id"]] == row["city_id"]
        local_date = date.fromisoformat(row["created_at"][:10])
        day_no = (local_date - START_DAY).days
        assert 0 <= day_no < m["period"]["days"]
        point = point_meta[row["pickup_point_id"]]
        code = city_by_id[row["city_id"]]
        point_no = int(point["point_no"])
        rank = city_rank[code]
        group = (rank, point_no, day_no)
        if group != last_group:
            if last_group is not None:
                old_rank, old_point, old_day = last_group
                expected_count = 150 + h(m["seed"], f"{old_rank}|{old_point}|{old_day}", "daily_count") % 101
                assert expected_seq == expected_count, f"missing orders in point-day {last_group}"
            last_group = group
            expected_seq = 0
        order_tuple = (rank, point_no, day_no, expected_seq)
        assert previous_order_tuple is None or order_tuple > previous_order_tuple, f"duplicate/out-of-order request key {row['request_key']}"
        previous_order_tuple = order_tuple
        expected_key = rank * 10_000_000_000 + day_no * 100_000_000 + point_no * 1_000 + expected_seq + 1
        assert request_key == expected_key, f"request key mismatch for point-day/order sequence: {row['request_key']}"
        expected_seq += 1
        items = json.loads(row["items_json"])
        item_ids = [x["product_id"] for x in items]
        assert 1 <= len(items) <= 5 and len(item_ids) == len(set(item_ids))
        assert all(is_numeric_id(x) and x in product_ids for x in item_ids)
        assert all(isinstance(x["quantity"], int) and x["quantity"] == 1 for x in items)
        counts[(code, int(point["point_no"]), day_no)] += 1
        line_count += len(items)
        order_count += 1
        if intent and intent["request_key"] == row["request_key"]:
            returned = set(json.loads(intent["requested_product_ids_json"]))
            assert returned and returned <= set(item_ids)
            assert intent["return_point_id"] == row["pickup_point_id"]
            assert is_numeric_id(intent["return_id"])
            assert 1 <= int(intent["delay_days_after_issued"]) <= 14
            intent_count += 1
            intent = next(next_intents, None)
    if last_group is not None:
        old_rank, old_point, old_day = last_group
        expected_count = 150 + h(m["seed"], f"{old_rank}|{old_point}|{old_day}", "daily_count") % 101
        assert expected_seq == expected_count, f"missing orders in final point-day {last_group}"
    assert intent is None
    assert all(150 <= n <= 250 for n in counts.values())
    assert len(counts) == m["counts"]["pointDayCount"]
    assert order_count == m["counts"]["preparedOrderCount"]
    assert line_count == m["counts"]["lineCount"]
    assert intent_count == m["counts"]["orderIntentCount"]
    return {"cities": len(cities), "points": len(points), "orders": order_count, "returnIntents": intent_count,
            "lines": line_count, "pointDays": len(counts), "numericIds": True, "normalizedLookups": True,
            "productGeometryMeters": True}


def overlap_check(smoke: Path, city: Path):
    def canonical(folder):
        city_by_id = {r["city_id"]: r["city_code"] for r in read_csv(folder / "cities.csv")}
        point_by_id = {r["pickup_point_id"]: r for r in read_csv(folder / "points.csv")}
        sku_value_by_id = {r["seller_sku"]: r["sku_value"] for r in read_csv(folder / "seller_skus.csv")}
        product_sku = {r["product_id"]: sku_value_by_id[r["seller_sku"]] for r in read_csv(folder / "products.csv")}
        result = {}
        for r in read_csv(folder / "orders.csv"):
            point = point_by_id[r["pickup_point_id"]]
            if city_by_id[r["city_id"]] == "yoshkar_ola" and int(point["point_no"]) <= 3 and r["created_at"].startswith(START_DAY.isoformat()):
                items = json.loads(r["items_json"])
                canonical_items = sorted((product_sku[item["product_id"]], item["quantity"]) for item in items)
                result[r["request_key"]] = (r["created_at"], city_by_id[r["city_id"]], point["point_code"], canonical_items)
        return result
    left, right = canonical(smoke), canonical(city)
    assert left == right and len(left) > 0, f"overlap differs: {len(left)} vs {len(right)}"
    return len(left)


def main():
    ap=argparse.ArgumentParser(); sub=ap.add_subparsers(dest="cmd",required=True)
    for name in ("smoke","city"):
        p=sub.add_parser(name); p.add_argument("--seed",type=int,default=SEED); p.add_argument("--days",type=int,default=1); p.add_argument("--out",type=Path)
        if name=="city": p.add_argument("--city",default="yoshkar_ola")
    p=sub.add_parser("full"); p.add_argument("--seed",type=int,default=SEED); p.add_argument("--days",type=int,default=7); p.add_argument("--out",type=Path,default=DATA/"russia10_full")
    estimate_parser = sub.add_parser("estimate")
    estimate_parser.add_argument("--full-path", type=Path, default=Path("/private/tmp/russia10_full"))
    p=sub.add_parser("validate"); p.add_argument("path",type=Path)
    p=sub.add_parser("overlap"); p.add_argument("smoke",type=Path); p.add_argument("city",type=Path)
    args=ap.parse_args(); started=time.perf_counter()
    if args.cmd=="smoke":
        out=args.out or DATA/"russia10_smoke"; m=generate(out,CITIES,3,args.days,args.seed)
        print(json.dumps({"out":str(out),"result":validate(out),"seconds":round(time.perf_counter()-started,3),"bytes":sum(p.stat().st_size for p in out.iterdir() if p.is_file())},ensure_ascii=False))
    elif args.cmd=="city":
        cities=[c for c in CITIES if c[0]==args.city]
        if not cities: raise SystemExit(f"unknown city {args.city}")
        out=args.out or DATA/"yoshkar_ola_city"; m=generate(out,cities,None,args.days,args.seed)
        print(json.dumps({"out":str(out),"result":validate(out),"seconds":round(time.perf_counter()-started,3),"bytes":sum(p.stat().st_size for p in out.iterdir() if p.is_file())},ensure_ascii=False))
    elif args.cmd=="full":
        generate(args.out,CITIES,None,args.days,args.seed)
        print(json.dumps({"out":str(args.out),"result":validate(args.out)},ensure_ascii=False))
    elif args.cmd=="estimate":
        full_path = args.full_path
        if (full_path / "manifest.json").exists():
            m = json.loads((full_path / "manifest.json").read_text(encoding="utf-8"))
            total_bytes = sum(p.stat().st_size for p in full_path.iterdir() if p.is_file())
            result = {"fullCommand":"python3 scripts/datasets/agents/russia10.py full --seed 42 --days 7 --out /private/tmp/russia10_full",
                      "expectedPointDays":146937,"orderRange":[22040550,36734250],"expectedOrdersApprox":29387400,
                      "actualOrders":m["counts"]["preparedOrderCount"],"actualPositions":m["counts"]["lineCount"],
                      "actualReturnIntents":m["counts"]["orderIntentCount"],"actualTotalBytes":total_bytes,
                      "actualGiB":round(total_bytes/(1024**3),2),"parquet":"not available; CSV output"}
        else:
            city = DATA / "yoshkar_ola_city"
            m = json.loads((city / "manifest.json").read_text(encoding="utf-8"))
            scale = 146937 / m["counts"]["pointDayCount"]
            orders_bytes = (city / "orders.csv").stat().st_size
            returns_bytes = (city / "return_intents.csv").stat().st_size
            points_bytes = (city / "points.csv").stat().st_size
            total_city_bytes = sum(p.stat().st_size for p in city.iterdir() if p.is_file())
            fixed_bytes = total_city_bytes - orders_bytes - returns_bytes - points_bytes
            projected = round(orders_bytes * scale + returns_bytes * scale + points_bytes * (20991/150) + fixed_bytes)
            result = {"expectedPointDays":146937,"orderRange":[22040550,36734250],"expectedOrdersApprox":29387400,
                      "measuredCityOrders":m["counts"]["preparedOrderCount"],"measuredCityBytes":total_city_bytes,
                      "estimatedTotalBytes":projected,"estimatedGiB":round(projected/(1024**3),2),
                      "uncertainty":"roughly ±20%; estimate separates fixed catalog bytes from rows that scale with orders/points"}
        print(json.dumps(result,ensure_ascii=False))
    elif args.cmd=="validate": print(json.dumps(validate(args.path),ensure_ascii=False))
    else: print(json.dumps({"matchingOrders":overlap_check(args.smoke,args.city)},ensure_ascii=False))


if __name__=="__main__": main()
