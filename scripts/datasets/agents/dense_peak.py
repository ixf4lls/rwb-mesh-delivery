#!/usr/bin/env python3
"""Deterministic dense/peak demo fixtures. Uses only the Python standard library."""
import argparse
import csv
import hashlib
import json
import random
import shutil
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "scripts" / "datasets"))
from common import numeric_id, pickup_point_attributes
SHARED_CATALOG_PATH = ROOT / "datasets" / "shared" / "catalog_scenarios_1_5.json"
SHARED_CATALOG = json.loads(SHARED_CATALOG_PATH.read_text(encoding="utf-8"))
SHARED_BY_ORDINAL = {p["ordinal"]:p for p in SHARED_CATALOG["products"]}
BASE = datetime(2026, 1, 5, 8, tzinfo=timezone.utc)
POINTS = [f"dense-p{i}" for i in range(1, 7)]
STORAGE = POINTS[:2]
PRODUCTS = [f"dense-sku-{i:02}" for i in range(1, 13)]
COMPACT = [PRODUCTS[i-1] for i in (1,2,5,6,9,10)]
BULKY = [PRODUCTS[i-1] for i in (3,4,7,8,11,12)]
PEAK_DAY = 13
PEAK_POINT = STORAGE[1]


def dump(path, obj):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_csv(path, fields, rows):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields, lineterminator="\n")
        w.writeheader()
        w.writerows(rows)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def make_orders(seed):
    rows = []
    for i in range(1600):
        rng = random.Random(f"{seed}:dense-order-{i+1:06}")
        day = i % 28
        hour = rng.randrange(9, 22)
        minute = rng.randrange(60)
        if i < 960:
            point = STORAGE[i % 2]
        else:
            point = POINTS[2 + (i % 4)]
        # Three compact SKUs dominate orders at the two storage points.
        if point in STORAGE and rng.random() < .72:
            product = rng.choice(COMPACT[:3])
        else:
            product = rng.choice(PRODUCTS)
        # Stable cutoff witness: same source demand in dense and peak.
        if i == 13:
            point, product, hour, minute = PEAK_POINT, COMPACT[0], 9, 30
        created = BASE + timedelta(days=day, hours=hour - 8, minutes=minute)
        items = json.dumps([{"product_id": product, "quantity": 1}], separators=(",", ":"))
        rows.append({"request_key": f"dense-request-{i+1:06}", "created_at": created.isoformat().replace("+00:00", "Z"),
                     "city_id": "city-b", "pickup_point_id": point, "items_json": items})
    return rows


def scenario_files(sid, source_orders, seed, peak=False):
    dest = ROOT / "datasets" / sid
    if dest.exists(): shutil.rmtree(dest)
    dest.mkdir(parents=True)
    city_id = numeric_id(sid,"city",1)
    point_ids = {code:numeric_id(sid,"pickup_point",i) for i,code in enumerate(POINTS,1)}
    product_ids = {code:numeric_id(sid,"product",i) for i,code in enumerate(PRODUCTS,1)}
    seller_codes=sorted(x["seller_code"] for x in SHARED_CATALOG["sellers"])
    seller_ids={code:numeric_id(sid,"seller",i) for i,code in enumerate(seller_codes,1)}
    seller_sku_ids={n:numeric_id(sid,"seller_sku",n) for n in range(1,13)}
    order_ids = {row["request_key"]:numeric_id(sid,"request",i) for i,row in enumerate(source_orders,1)}
    trip_ids = {(day,point):numeric_id(sid,"trip",day*6+idx) for day in range(28) for idx,point in enumerate(POINTS,1)}
    hub_node_id = str((2 if sid=="dense" else 3)*1_000_000_000+920_000_001)
    sc_node_id = str((2 if sid=="dense" else 3)*1_000_000_000+920_000_002)

    # Dimension tables hold text labels; facts keep only numeric-string IDs.
    write_csv(dest/"cities.csv",["city_id","city_code","city_name","time_zone"],[{"city_id":city_id,"city_code":"city-b","city_name":"Город Б","time_zone":"Europe/Moscow"}])
    nodes=[{"node_id":hub_node_id,"city_id":str((2 if sid=="dense" else 3)*1_000_000_000+400_000_001),"node_code":"city-a-hub","node_name":"Хаб А","kind":"hub","active":"true","mode":"simulated"},
           {"node_id":sc_node_id,"city_id":city_id,"node_code":"city-b-sc","node_name":"СЦ Б","kind":"sorting_center","active":"true","mode":"city_mesh"}]
    # Hub city is a separate normalized city row.
    city_a_id=str((2 if sid=="dense" else 3)*1_000_000_000+400_000_002)
    cities=[{"city_id":city_a_id,"city_code":"city-a","city_name":"Город А","time_zone":"Europe/Moscow"},{"city_id":city_id,"city_code":"city-b","city_name":"Город Б","time_zone":"Europe/Moscow"}]
    write_csv(dest/"cities.csv",list(cities[0]),cities)
    nodes[0]["city_id"]=city_a_id
    for i,point in enumerate(POINTS,1):
        nodes.append({"node_id":point_ids[point],"city_id":city_id,"node_code":point,"node_name":f"ПВЗ Б-{i}","kind":"storage_pvz" if point in STORAGE else "pickup_only","active":"true","mode":"city_mesh"})
    write_csv(dest/"nodes.csv",list(nodes[0]),nodes)
    points=[]
    for i,point in enumerate(POINTS,1):
        points.append({"pickup_point_id":point_ids[point],"city_id":city_id,"node_id":point_ids[point],"point_code":point,"point_name":f"ПВЗ Б-{i}","storage_enabled":str(point in STORAGE).lower(),"accepts_orders":"true","accepts_returns":"true","latitude":f"55.{i:04d}","longitude":f"49.{i:04d}",**pickup_point_attributes(seed,point_ids[point],point in STORAGE)})
    write_csv(dest/"points.csv",list(points[0]),points)
    seller_names={x["seller_code"]:x["seller_name"] for x in SHARED_CATALOG["sellers"]}
    sellers=[{"seller_id":seller_ids[code],"seller_code":code,"seller_name":seller_names[code]} for code in seller_codes]
    write_csv(dest/"sellers.csv",list(sellers[0]),sellers)
    sku_rows=[]
    products=[]
    for n,product in enumerate(PRODUCTS,1):
        canonical=SHARED_BY_ORDINAL[n]
        seller_code=canonical["seller_code"]
        sku_rows.append({"seller_sku":seller_sku_ids[n],"seller_id":seller_ids[seller_code],"sku_value":canonical["sku_value"]})
        products.append({"product_id":product_ids[product],"seller_id":seller_ids[seller_code],"seller_sku":seller_sku_ids[n],
                         "sku_value":canonical["sku_value"],"product_name":canonical["product_name"],"demand_class":canonical["demand_class"],"size_class":canonical["size_class"],
                         "weight_grams":canonical["weight_grams"],"slots_per_unit":canonical["slots_per_unit"],"sale_price_minor":canonical["sale_price_minor"],
                         "unit_cost_minor":canonical["unit_cost_minor"],"category":canonical["category"],"width_m":canonical["width_m"],"height_m":canonical["height_m"],"depth_m":canonical["depth_m"]})
    write_csv(dest/"seller_skus.csv",list(sku_rows[0]),sku_rows)
    write_csv(dest/"products.csv",list(products[0]),products)

    canonical = "\n".join(",".join(o[k] for k in ("request_key","created_at","pickup_point_id","items_json")) for o in source_orders).encode()
    demand_hash=hashlib.sha256(canonical).hexdigest()
    orders=[]
    for n,o in enumerate(source_orders,1):
        cart=json.loads(o["items_json"])
        new_cart=[{"product_id":product_ids[x["product_id"]],"quantity":x["quantity"]} for x in cart]
        # source_request_key is the same numeric test key for dense and peak;
        # request_key is the isolated numeric ID of this dataset's intent.
        orders.append({"request_key":order_ids[o["request_key"]],"source_request_key":str(9_000_000_000+n),"created_at":o["created_at"],
                       "city_id":city_id,"pickup_point_id":point_ids[o["pickup_point_id"]],"items_json":json.dumps(new_cart,separators=(",",":"))})
    write_csv(dest/"orders.csv",list(orders[0]),orders)

    counts={(p,s):0 for p in POINTS for s in PRODUCTS}
    for order in source_orders:
        product=json.loads(order["items_json"])[0]["product_id"]
        counts[(order["pickup_point_id"],product)]+=1
    history=[]
    for day in range(56):
        for point in POINTS:
            for product in PRODUCTS:
                base=counts[(point,product)]
                q=max(0,(base//28)+(1 if base and (day+len(product))%7==0 else 0))
                history.append({"day":(BASE-timedelta(days=56-day)).date().isoformat(),"city_id":city_id,"pickup_point_id":point_ids[point],"product_id":product_ids[product],"quantity":q})
    write_csv(dest/"demand_history.csv",list(history[0]),history)

    replenishments=[]
    for point in STORAGE:
        for product in COMPACT[:4]:
            for q in (5,10,20):
                replenishments.append({"product_id":product_ids[product],"storage_point_id":point_ids[point],"quantity":q,"source_node_id":hub_node_id,
                                       "calendar":"daily","external_lot_ref":numeric_id(sid,"batch",len(replenishments)+1)})
    write_csv(dest/"replenishments.csv",list(replenishments[0]),replenishments)
    trips=[]
    for day in range(28):
        for idx,point in enumerate(POINTS,1):
            trip_no=day*6+idx
            trips.append({"trip_id":trip_ids[(day,point)],"point_id":point_ids[point],"day":(BASE+timedelta(days=day)).date().isoformat(),
                          "cutoff_at":(BASE+timedelta(days=day,hours=2)).isoformat().replace("+00:00","Z"),"depart_at":(BASE+timedelta(days=day,hours=4)).isoformat().replace("+00:00","Z"),
                          "capacity_slots":80,"background_slots":12,"mode":"city_mesh"})
    write_csv(dest/"trips.csv",list(trips[0]),trips)
    if peak:
        selected=next(o for o in orders if o["pickup_point_id"]==point_ids[PEAK_POINT] and datetime.fromisoformat(o["created_at"].replace("Z","+00:00")).date()==(BASE+timedelta(days=PEAK_DAY)).date())
        event_id=numeric_id(sid,"segment",1)
        dump(dest/"peak_events.json",{"parameters":{"point_id":point_ids[PEAK_POINT],"day":(BASE+timedelta(days=PEAK_DAY)).date().isoformat(),"ordinary_used_slots":90,"total_slots":100,
             "storage_quota_slots":100,"storage_used_slots":0,"pickup_used_slots":0,"inbound_reserved_slots":0,"pre_cutoff_pickup_queue_count":30,"courier_handover_delay_minutes":25},
             "capacity":{"pointId":point_ids[PEAK_POINT],"asOf":"2026-01-18T09:00:00Z","totalSlots":100,"storageQuotaSlots":100,"storageUsedSlots":0,"ordinaryUsedSlots":90,"pickupUsedSlots":0,"inboundReservedSlots":0,"availableStorageSlots":10,"availablePickupSlots":10},
             "rejected_receipt":{"contract":"frontend.yaml","schema":"BatchReceipt","direction":"request","body":{"productId":product_ids[COMPACT[0]],"storagePointId":point_ids[PEAK_POINT],"quantity":20,"externalLotRef":numeric_id(sid,"batch",99)},"expected":{"status":409,"mediaType":"application/problem+json","code":"capacity_exceeded","requiredSlots":20,"availableSlots":10}},
             "cutoff_case":{"request_key":selected["request_key"],"pickup_point_id":point_ids[PEAK_POINT],"trip_id":trip_ids[(PEAK_DAY,PEAK_POINT)],"baseline_handover_at":"2026-01-18T09:50:00Z","delayed_handover_at":"2026-01-18T10:15:00Z","cutoff_at":"2026-01-18T10:00:00Z","baseline":"meets cutoff","peak":"misses cutoff; next daily trip","sla_next_trip":True},
             "ordinary_queue":{"event_id":event_id,"count":30,"window_start":"2026-01-18T09:00:00Z","window_end":"2026-01-18T10:00:00Z","wait_minutes_baseline":0,"wait_minutes_peak":25},
             "notes":"IDs are numeric strings; route ETA/cost is fixture expectation, not an API execution."})
    expected={"demandCanonicalSha256":demand_hash,"orderCount":1600,"lineCount":1600,"storagePointOrderCount":960,"otherPointOrderCount":640,"historyRows":4032,"numericIds":True,"sharedCatalogSha256":sha(SHARED_CATALOG_PATH),"productGeometryUnit":"m"}
    if peak:
        expected["control"]={"capacity":{"ordinaryUsedSlots":90,"totalSlots":100,"availableStorageSlots":10},"rejectedReceipt":{"status":409,"code":"capacity_exceeded"},"delayMinutes":25,"backgroundQueueSeparate":True}
    else: expected.update({"at_storage_points_top3_compact_skus":True,"replenishment_quantities":[5,10,20],"cutoff_examples":["before cutoff","after cutoff"]})
    dump(dest/"expected.json",expected)
    files=[]
    for f in sorted(dest.iterdir()):
        if f.is_file():
            rows=sum(1 for _ in f.open(encoding="utf-8"))-1 if f.suffix==".csv" else None
            files.append({"path":f.name,"sha256":sha(f),"rows":rows})
    dump(dest/"manifest.json",{"schemaVersion":"2.0.0","datasetId":sid,"seed":seed,"period":{"start":BASE.date().isoformat(),"days":28,"timeZone":"Europe/Moscow"},
         "initialClock":"2026-01-05T08:00:00Z","start":{"cityId":city_id,"pickupPointId":point_ids[PEAK_POINT] if peak else point_ids[STORAGE[0]],"at":"2026-01-05T08:00:00Z","showcaseKey":orders[0]["request_key"]},
         "counts":{"pointCount":6,"preparedOrderCount":1600,"orderIntentCount":1600,"lineCount":1600,"orderCount":1600,"historyRows":4032},"files":files,"demandCanonicalSha256":demand_hash,"sharedCatalog":{"path":"datasets/shared/catalog_scenarios_1_5.json","sha256":sha(SHARED_CATALOG_PATH)},
         "idRules":"^[0-9]+$","scenario":{"kind":"peak" if peak else "dense","baseDatasetId":"dense" if peak else None}})


def generate(seed=42):
    orders = make_orders(seed)
    scenario_files("dense", orders, seed, False)
    scenario_files("peak", orders, seed, True)


def validate():
    d,p=ROOT/"datasets/dense",ROOT/"datasets/peak"
    def read(folder,name):
        with (folder/name).open(encoding="utf-8",newline="") as f:return list(csv.DictReader(f))
    def check_folder(folder):
        manifest=json.loads((folder/"manifest.json").read_text(encoding="utf-8"))
        assert manifest["idRules"]=="^[0-9]+$"
        for ent in manifest["files"]:
            f=folder/ent["path"];assert sha(f)==ent["sha256"],ent["path"]
            if f.suffix==".csv":
                with f.open(encoding="utf-8",newline="") as stream:assert sum(1 for _ in csv.reader(stream))-1==ent["rows"]
        cities=read(folder,"cities.csv"); points=read(folder,"points.csv"); sellers=read(folder,"sellers.csv")
        skus=read(folder,"seller_skus.csv"); products=read(folder,"products.csv"); orders=read(folder,"orders.csv")
        assert len(cities)==2 and len(points)==6 and len(sellers)==3 and len(skus)==12 and len(products)==12 and len(orders)==1600
        city_ids={x["city_id"] for x in cities};point_ids={x["pickup_point_id"] for x in points};product_ids={x["product_id"] for x in products};seller_ids={x["seller_id"] for x in sellers};sku_ids={x["seller_sku"] for x in skus}
        assert all(x.isdigit() for x in city_ids|point_ids|product_ids|seller_ids|sku_ids)
        assert all(x["city_id"] in city_ids for x in points)
        assert all(x["seller_id"] in seller_ids and x["seller_sku"] in sku_ids for x in products)
        sku_values={x["seller_sku"]:x["sku_value"] for x in read(folder,"seller_skus.csv")}
        seller_codes={x["seller_id"]:x["seller_code"] for x in sellers}
        shared_by_sku={x["sku_value"]:x for x in SHARED_CATALOG["products"]}
        for item in products:
            assert all(0<float(item[c])<10 for c in ("width_m","height_m","depth_m"))
            assert not any(c in item for c in ("base_area_m2","volume_m3"))
            canonical=shared_by_sku[sku_values[item["seller_sku"]]]
            assert seller_codes[item["seller_id"]]==canonical["seller_code"]
            for field in ("product_name","demand_class","size_class","weight_grams","slots_per_unit","sale_price_minor","unit_cost_minor","category","width_m","height_m","depth_m"):
                assert str(item[field])==str(canonical[field]),(canonical["sku_value"],field)
        for order in orders:
            assert all(order[c].isdigit() for c in ("request_key","source_request_key","city_id","pickup_point_id"))
            assert order["city_id"] in city_ids and order["pickup_point_id"] in point_ids
            items=json.loads(order["items_json"]);ids=[x["product_id"] for x in items]
            assert all(x.isdigit() and x in product_ids for x in ids) and len(ids)==len(set(ids))
        history=read(folder,"demand_history.csv");assert len(history)==4032
        assert all(x["city_id"] in city_ids and x["pickup_point_id"] in point_ids and x["product_id"] in product_ids for x in history)
        expected=json.loads((folder/"expected.json").read_text());assert expected["orderCount"]==1600 and expected["numericIds"]
        return {"orders":orders,"products":products,"points":points,"cities":cities,"expected":expected}
    a,b=check_folder(d),check_folder(p)
    def natural(data):
        product={x["product_id"]:x["seller_sku"] for x in data["products"]};sku={x["seller_sku"]:x["sku_value"] for x in read(d if data is a else p,"seller_skus.csv")}
        point={x["pickup_point_id"]:x["point_code"] for x in data["points"]};city={x["city_id"]:x["city_code"] for x in data["cities"]}
        return [(o["source_request_key"],o["created_at"],city[o["city_id"]],point[o["pickup_point_id"]],[(sku[product[x["product_id"]]],x["quantity"]) for x in json.loads(o["items_json"])]) for o in data["orders"]]
    assert natural(a)==natural(b)
    assert a["expected"]["demandCanonicalSha256"]==b["expected"]["demandCanonicalSha256"]
    assert len(SHARED_CATALOG["products"])==12
    assert all(sum(1 for x in SHARED_CATALOG["products"] if x["demand_class"]==dc and x["size_class"]==sc)==2 for dc in ("high","medium","low") for sc in ("compact","bulky"))
    assert sum(o["pickup_point_id"] in {x["pickup_point_id"] for x in a["points"][:2]} for o in a["orders"])==960
    assert (p/"peak_events.json").exists()
    peak=json.loads((p/"peak_events.json").read_text());assert peak["capacity"]["ordinaryUsedSlots"]==90 and peak["capacity"]["totalSlots"]==100 and peak["rejected_receipt"]["expected"]["status"]==409
    print(f"valid dense/peak: 1600 orders each, canonical demand equal, 4032 history rows, numeric IDs and product dimensions valid; checksum={a['expected']['demandCanonicalSha256']}")


if __name__ == "__main__":
    parser=argparse.ArgumentParser(); parser.add_argument("command",choices=["generate","validate"],default="generate",nargs="?"); parser.add_argument("--seed",type=int,default=42)
    args=parser.parse_args()
    if args.command=="generate": generate(args.seed)
    else: validate()
