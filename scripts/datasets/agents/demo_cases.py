"""Generate and validate the eight deterministic end-to-end demo cases.

Usage: python3 scripts/datasets/agents/demo_cases.py generate [--seed 42]
       python3 scripts/datasets/agents/demo_cases.py validate
"""
from __future__ import annotations
import argparse, csv, hashlib, json, shutil
from pathlib import Path
from datetime import datetime, timezone
import sys
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "datasets"))
from common import pickup_point_attributes

ROOT = Path(__file__).resolve().parents[3]
DEFAULT_OUT = ROOT / "datasets" / "demo"
CLOCK = "2026-01-05T09:00:00Z"
SHARED_CATALOG_PATH = ROOT / "datasets" / "shared" / "catalog_scenarios_1_5.json"
SHARED_CATALOG = json.loads(SHARED_CATALOG_PATH.read_text(encoding="utf-8"))
SHARED_BY_ORDINAL = {p["ordinal"]: p for p in SHARED_CATALOG["products"]}
DEMO_ORDINAL = {p["id"]: i for i,p in enumerate(PRODUCTS,1)} if "PRODUCTS" in globals() else {}

NETWORK = {
    "cities": [
        {"id":"city-a","name":"Город А","timeZone":"Europe/Moscow"},
        {"id":"city-b","name":"Город Б","timeZone":"Europe/Moscow"}],
    "nodes": [
        {"id":"hub-a","cityId":"city-a","name":"Хаб А","kind":"hub","active":True,"mode":"simulated"},
        {"id":"sc-a","cityId":"city-a","name":"СЦ А","kind":"sorting_center","active":True,"mode":"physical"},
        {"id":"sc-b","cityId":"city-b","name":"СЦ Б","kind":"sorting_center","active":True,"mode":"physical"},
        *[{"id":f"s{i}","cityId":"city-b","name":f"ПВЗ хранения S{i}","kind":"storage_pvz","active":True,"mode":"physical"} for i in (1,2)],
        *[{"id":f"p{i}","cityId":"city-b","name":f"ПВЗ P{i}","kind":"pickup_only","active":True,"mode":"physical"} for i in range(2,6)]
    ],
    "pickupPoints": [
        {"nodeId":f"s{i}","acceptsOrders":True,"acceptsReturns":True,"openingHours":[{"weekdays":[1,2,3,4,5,6,7],"opens":"09:00","closes":"21:00"}]} for i in (1,2)
    ] + [
        {"nodeId":f"p{i}","acceptsOrders":True,"acceptsReturns":True,"openingHours":[{"weekdays":[1,2,3,4,5,6,7],"opens":"09:00","closes":"21:00"}]} for i in range(2,6)
    ],
    "trips": [
        {"id":"line-ab-daily","type":"intercity","fromNodeId":"hub-a","toNodeId":"sc-b","frequency":"daily","departureLocal":"12:00","mode":"simulated"},
        {"id":"city-b-east","type":"city_trip","fromNodeId":"sc-b","toNodeId":"p2","frequency":"daily","departureLocal":"16:00","mode":"physical"},
        {"id":"city-b-west","type":"city_trip","fromNodeId":"sc-b","toNodeId":"p4","frequency":"daily","departureLocal":"16:00","mode":"physical"},
        {"id":"city-b-alt","type":"city_trip","fromNodeId":"sc-b","toNodeId":"p2","frequency":"daily","departureLocal":"18:00","mode":"physical"}
    ],
    "tariff": {"id":"tariff-demo-1","currency":"RUB","effectiveFrom":"2026-01-01T00:00:00Z","storageThresholdDays":10,"maximumDeliveryMinutes":4320,"rates":[{"code":"storage_slot_day","unit":"slot_day","amount":200},{"code":"handling","unit":"order","amount":500},{"code":"city_trip","unit":"shipment","amount":1200},{"code":"intercity","unit":"shipment","amount":3500},{"code":"extra_stop","unit":"stop","amount":800}]}
}

PRODUCTS = [
 {"id":"prod-mug","sellerId":"seller-1","sellerSku":"mug-01","name":"Кружка","weightGrams":300,"slotsPerUnit":1,"salePriceMinor":59000,"unitCostMinor":26000,"category":"home"},
 {"id":"prod-book","sellerId":"seller-2","sellerSku":"book-01","name":"Книга","weightGrams":250,"slotsPerUnit":1,"salePriceMinor":45000,"unitCostMinor":18000,"category":"books"},
 {"id":"prod-lamp","sellerId":"seller-3","sellerSku":"lamp-01","name":"Лампа","weightGrams":900,"slotsPerUnit":2,"salePriceMinor":129000,"unitCostMinor":61000,"category":"home"},
 {"id":"prod-tea","sellerId":"seller-2","sellerSku":"tea-01","name":"Чай","weightGrams":150,"slotsPerUnit":1,"salePriceMinor":25000,"unitCostMinor":9000,"category":"food"},
 {"id":"prod-high-compact-1","sellerId":"seller-1","sellerSku":"hc-01","name":"Бестселлер компактный 1","weightGrams":200,"slotsPerUnit":1,"salePriceMinor":39000,"unitCostMinor":16000,"category":"compact"},
 {"id":"prod-high-compact-2","sellerId":"seller-1","sellerSku":"hc-02","name":"Бестселлер компактный 2","weightGrams":240,"slotsPerUnit":1,"salePriceMinor":42000,"unitCostMinor":17000,"category":"compact"},
 {"id":"prod-high-bulky-1","sellerId":"seller-1","sellerSku":"hb-01","name":"Бестселлер объёмный 1","weightGrams":1800,"slotsPerUnit":3,"salePriceMinor":149000,"unitCostMinor":75000,"category":"bulky"},
 {"id":"prod-high-bulky-2","sellerId":"seller-1","sellerSku":"hb-02","name":"Бестселлер объёмный 2","weightGrams":2100,"slotsPerUnit":4,"salePriceMinor":179000,"unitCostMinor":89000,"category":"bulky"},
 {"id":"prod-mid-compact-1","sellerId":"seller-2","sellerSku":"mc-01","name":"Средний спрос компактный 1","weightGrams":180,"slotsPerUnit":1,"salePriceMinor":31000,"unitCostMinor":12000,"category":"compact"},
 {"id":"prod-mid-compact-2","sellerId":"seller-2","sellerSku":"mc-02","name":"Средний спрос компактный 2","weightGrams":260,"slotsPerUnit":1,"salePriceMinor":35000,"unitCostMinor":14000,"category":"compact"},
 {"id":"prod-mid-bulky-1","sellerId":"seller-2","sellerSku":"mb-01","name":"Средний спрос объёмный 1","weightGrams":1500,"slotsPerUnit":3,"salePriceMinor":119000,"unitCostMinor":59000,"category":"bulky"},
 {"id":"prod-mid-bulky-2","sellerId":"seller-2","sellerSku":"mb-02","name":"Средний спрос объёмный 2","weightGrams":2300,"slotsPerUnit":4,"salePriceMinor":159000,"unitCostMinor":79000,"category":"bulky"}
]
DEMO_ORDINAL = {p["id"]: i for i,p in enumerate(PRODUCTS,1)}
ORDERS = [
 ["city-mesh-1", "2026-01-05T09:10:00Z", "city-b", "p2", [{"product_id":"prod-mug","quantity":1}]],
 ["intercity-hub-1", "2026-01-05T09:15:00Z", "city-b", "p4", [{"product_id":"prod-book","quantity":1}]],
 ["mixed-return-1", "2026-01-05T09:20:00Z", "city-b", "s2", [{"product_id":"prod-mug","quantity":1},{"product_id":"prod-book","quantity":1}]],
 ["trip-disruption-1", "2026-01-05T09:25:00Z", "city-b", "p3", [{"product_id":"prod-lamp","quantity":1}]],
 ["local-shelf-1", "2026-01-05T09:30:00Z", "city-b", "s1", [{"product_id":"prod-tea","quantity":1}]],
 ["race-first-1", "2026-01-05T09:35:00Z", "city-b", "p2", [{"product_id":"prod-mug","quantity":1}]],
 ["race-second-1", "2026-01-05T09:35:00Z", "city-b", "p4", [{"product_id":"prod-mug","quantity":1}]],
]

EXAMPLES = [
 {"step":1,"caseKey":"city_mesh","role":"dispatcher","contract":"frontend","schema":"QuoteRequest","direction":"request","method":"POST","path":"/quotes","body":{"pickupPointId":"p2","items":[{"productId":"prod-mug","quantity":1}]},"headers":{"X-Demo-Run-Id":"<active-run-id>"},"expected":{"status":200,"option":"mesh_only","mode":"city_mesh","sourceBatch":"batch-city-mesh","sourceRemaining":4,"shipmentStatus":["packed","in_transit","ready","issued"],"cutoffAt":"2026-01-05T15:00:00+03:00","readyAt":"2026-01-05T18:00:00+03:00","buyerFeeMinor":1200,"etaMinutes":540}},
 {"step":2,"caseKey":"intercity_hub","role":"dispatcher","contract":"frontend","schema":"QuoteRequest","direction":"request","method":"POST","path":"/quotes","body":{"pickupPointId":"p4","items":[{"productId":"prod-book","quantity":1}]},"headers":{"X-Demo-Run-Id":"<active-run-id>"},"expected":{"status":200,"options":[{"id":"mesh_only","feasible":False,"reasons":["insufficient_stock"]},{"id":"hub_only","feasible":True,"estimatedCostMinor":6700,"buyerFeeMinor":1800,"etaMinutes":1980}],"path":["hub-a","sc-a","sc-b","p4"],"hybrid":{"feasible":True,"buyerFeeMinor":2200,"etaMinutes":2340}}},
 {"step":3,"caseKey":"mixed_return","role":"dispatcher","contract":"frontend","schema":"ReturnCreate","direction":"request","method":"POST","path":"/returns","body":{"orderId":"<order-id: mixed_return-1>","returnPointId":"s2","reason":"Демонстрационный возврат одной позиции","lines":[{"orderItemId":"<line-id: prod-mug>","quantity":1}]},"headers":{"X-Demo-Run-Id":"<active-run-id>","Idempotency-Key":"demo-mixed-return-v1"},"expected":{"issuedLines":2,"returnAccepted":True,"resellableQuantity":1,"quarantinedQuantity":0,"availableDelta":{"prod-mug":1,"prod-book":0},"reversedLine":"prod-mug","secondLineRemainsSold":True,"mode":"mixed","readyAfterLastShipment":True}},
 {"step":4,"caseKey":"trip_disruption","role":"dispatcher","contract":"frontend","schema":"IncidentCreate","direction":"request","method":"POST","path":"/incidents","body":{"orderId":"<order-id: trip-disruption-1>","shipmentId":"<shipment-id: trip-disruption-1>","type":"trip_disrupted","failureTarget":{"tripId":"city-b-east"},"description":"Рейс отменён после сборки"},"headers":{"X-Demo-Run-Id":"<active-run-id>","Idempotency-Key":"demo-disruption-v1"},"expected":{"status":"rerouted","failedTrip":"city-b-east","alternativeTrip":"city-b-alt","newRevision":{"previousRevision":1,"revision":2,"lastKnownNodeId":"sc-b","deltaCostMinor":800,"deltaEtaMinutes":120},"notification":"route_changed","buyerFeeMinorUnchanged":True,"promisedReadyAtUnchanged":True,"duplicateReserve":False}},
 {"step":5,"caseKey":"long_storage","role":"dispatcher","contract":"frontend","schema":"ClockAdvance","direction":"request","method":"POST","path":"/demo/clock/advances","body":{"to":"2026-01-15T09:00:00Z"},"headers":{"X-Demo-Run-Id":"<active-run-id>","Idempotency-Key":"demo-storage-clock-v1"},"expected":{"batch":"batch-storage-5","orders":0,"billableSlotDays":50,"accruedMinor":10000,"storageCase":"open","notification":"long_storage","decision":{"contract":"frontend","schema":"StorageDecision","direction":"request","body":{"action":"transfer_to_hub","reason":"Партия не востребована","expectedVersion":1}},"afterTransfer":{"pointSlotsFreed":5,"caseStatus":"closed"}}},
 {"step":6,"caseKey":"local_shelf","role":"pvz_operator","contract":"frontend","schema":"QuoteRequest","direction":"request","method":"POST","path":"/quotes","body":{"pickupPointId":"s1","items":[{"productId":"prod-tea","quantity":1}]},"headers":{"X-Demo-Run-Id":"<active-run-id>"},"expected":{"option":"mesh_only","mode":"local_shelf","segmentType":"on_site","packedStatus":"ready","beforeIssuedAvailable":9,"afterIssuedAvailable":9,"issuedPhysicalDelta":-1,"buyerFeeMinor":0,"etaMinutes":0}},
 {"step":7,"caseKey":"last_unit_race","role":"dispatcher","contract":"frontend","schema":"QuoteRequest","direction":"request","method":"POST","path":"/quotes","body":{"pickupPointId":"p2","items":[{"productId":"prod-mug","quantity":1}]},"headers":{"X-Demo-Run-Id":"<active-run-id>"},"expected":{"concurrentQuotes":2,"availableUnits":1,"winnerCount":1,"loser":"stale_quote_or_reservation_conflict","loserPreliminaryReservationsReleased":True,"retry":{"quote":"fresh","option":"hub_only","sameAcceptedOrderNotSilentlySwitched":True}}},
 {"step":8,"caseKey":"capacity_conflict","role":"pvz_operator","contract":"frontend","schema":"BatchReceipt","direction":"request","method":"POST","path":"/batches","body":{"productId":"prod-mug","storagePointId":"s1","quantity":5,"externalLotRef":"demo-capacity-reject"},"headers":{"X-Demo-Run-Id":"<active-run-id>","Idempotency-Key":"demo-capacity-reject-v1"},"expected":{"status":409,"contentType":"application/problem+json","code":"capacity_exceeded","context":{"requiredSlots":5,"availableSlots":4},"batchCreated":False,"beforeAfter":{"storageUsedSlots":6,"availableStorageSlots":4}}}
]

def _sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _ids():
    import sys
    sys.path.insert(0, str(ROOT / "scripts" / "datasets"))
    from common import numeric_id
    city_ids={x["id"]:numeric_id("demo","city",i) for i,x in enumerate(NETWORK["cities"],1)}
    point_codes=[n["id"] for n in NETWORK["nodes"] if n["kind"] in ("storage_pvz","pickup_only")]
    point_ids={code:numeric_id("demo","pickup_point",i) for i,code in enumerate(point_codes,1)}
    node_ids={n["id"]:(point_ids[n["id"]] if n["id"] in point_ids else str(1_000_000_000+920_000_000+i)) for i,n in enumerate(NETWORK["nodes"],1)}
    product_ids={x["id"]:numeric_id("demo","product",i) for i,x in enumerate(PRODUCTS,1)}
    seller_codes=sorted({SHARED_BY_ORDINAL[i]["seller_code"] for i in range(1,len(PRODUCTS)+1)})
    seller_ids={x:numeric_id("demo","seller",i) for i,x in enumerate(seller_codes,1)}
    sku_ids={x["sellerSku"]:numeric_id("demo","seller_sku",i) for i,x in enumerate(PRODUCTS,1)}
    request_ids={r[0]:numeric_id("demo","request",i) for i,r in enumerate(ORDERS,1)}
    trip_ids={x["id"]:numeric_id("demo","trip",i) for i,x in enumerate(NETWORK["trips"],1)}
    batch_ids={x:str(1_000_000_000+600_000_000+i) for i,x in enumerate(["batch-city-mesh","batch-storage-5"],1)}
    line_ids={x["id"]:str(1_000_000_000+800_000_000+i) for i,x in enumerate(PRODUCTS,1)}
    return city_ids,point_ids,node_ids,product_ids,seller_ids,sku_ids,request_ids,trip_ids,batch_ids,line_ids


def _convert(value, maps):
    city_ids,point_ids,node_ids,product_ids,seller_ids,sku_ids,request_ids,trip_ids,batch_ids,line_ids=maps
    allmap={**city_ids,**point_ids,**node_ids,**product_ids,**seller_ids,**sku_ids,**request_ids,**trip_ids,**batch_ids}
    if isinstance(value,dict):
        out={}
        for k,v in value.items():
            nk=allmap.get(k,k)
            if k == "id" and v in ("mesh_only", "hub_only"):
                option_index = 1 if v == "mesh_only" else 2
                out["id"] = str(1_000_000_000 + 40_000_000 + option_index)
                out["policy"] = v
                continue
            if k == "id" and v == "tariff-demo-1":
                out["id"] = str(1_000_000_000 + 975_000_000 + 1)
                continue
            out[nk]=_convert(v,maps)
        return out
    if isinstance(value,list): return [_convert(x,maps) for x in value]
    if isinstance(value,str):
        if value=="<active-run-id>": return "1001000001"
        if value.startswith("<order-id: ") and value.endswith(">"): return request_ids[value[11:-1].replace("_","-")]
        if value.startswith("<line-id: ") and value.endswith(">"): return line_ids[value[10:-1]]
        if value.startswith("<shipment-id: ") and value.endswith(">"): return request_ids[value[14:-1]]
        for old,table in ((value,allmap),):
            if old in table: return table[old]
        return value
    return value


def generate(seed: int = 42, out_dir: Path | str = DEFAULT_OUT):
    out=Path(out_dir)
    if out.exists(): shutil.rmtree(out)
    out.mkdir(parents=True)
    maps=_ids(); city_ids,point_ids,node_ids,product_ids,seller_ids,sku_ids,request_ids,trip_ids,batch_ids,line_ids=maps
    files={}
    def write_json(name,obj):
        p=out/name;p.write_text(json.dumps(obj,ensure_ascii=False,indent=2)+"\n",encoding="utf-8");files[name]={"sha256":_sha(p),"rows":len(obj) if isinstance(obj,list) else 1}
    def write_rows(name,fields,rows):
        p=out/name
        with p.open("w",encoding="utf-8",newline="") as f:
            w=csv.DictWriter(f,fieldnames=fields,lineterminator="\n");w.writeheader();w.writerows(rows)
        files[name]={"sha256":_sha(p),"rows":len(rows)}
    cities=[{"city_id":city_ids[x["id"]],"city_code":x["id"],"city_name":x["name"],"time_zone":x["timeZone"]} for x in NETWORK["cities"]]
    write_rows("cities.csv",list(cities[0]),cities)
    nodes=[]
    for n in NETWORK["nodes"]:
        nodes.append({"node_id":node_ids[n["id"]],"city_id":city_ids[n["cityId"]],"node_code":n["id"],"node_name":n["name"],"kind":n["kind"],"active":str(n["active"]).lower(),"mode":n["mode"]})
    write_rows("nodes.csv",list(nodes[0]),nodes)
    accepts={p["nodeId"]:p for p in NETWORK["pickupPoints"]}
    points=[]
    for n in NETWORK["nodes"]:
        if n["id"] not in accepts: continue
        a=accepts[n["id"]]
        points.append({"pickup_point_id":point_ids[n["id"]],"city_id":city_ids[n["cityId"]],"node_id":node_ids[n["id"]],"point_code":n["id"],"point_name":n["name"],"kind":n["kind"],"active":str(n["active"]).lower(),"accepts_orders":str(a["acceptsOrders"]).lower(),"accepts_returns":str(a["acceptsReturns"]).lower(),"opening_hours_json":json.dumps(a["openingHours"],separators=(",",":")),**pickup_point_attributes(seed,point_ids[n["id"]],n["kind"]=="storage_pvz")})
    write_rows("points.csv",list(points[0]),points)
    seller_names={x["seller_code"]:x["seller_name"] for x in SHARED_CATALOG["sellers"]}
    sellers=[{"seller_id":seller_ids[c],"seller_code":c,"seller_name":seller_names[c]} for c in sorted(seller_ids)]
    write_rows("sellers.csv",list(sellers[0]),sellers)
    skus=[]
    for i,x in enumerate(PRODUCTS,1):
        shared=SHARED_BY_ORDINAL[i]
        skus.append({"seller_sku":sku_ids[x["sellerSku"]],"seller_id":seller_ids[shared["seller_code"]],"sku_value":shared["sku_value"]})
    write_rows("seller_skus.csv",list(skus[0]),skus)
    products=[]
    for i,x in enumerate(PRODUCTS,1):
        shared=SHARED_BY_ORDINAL[i]
        products.append({"product_id":product_ids[x["id"]],"seller_id":seller_ids[shared["seller_code"]],"seller_sku":sku_ids[x["sellerSku"]],
                         "sku_value":shared["sku_value"],"product_name":shared["product_name"],"demand_class":shared["demand_class"],"size_class":shared["size_class"],
                         "weight_grams":shared["weight_grams"],"slots_per_unit":shared["slots_per_unit"],"sale_price_minor":shared["sale_price_minor"],
                         "unit_cost_minor":shared["unit_cost_minor"],"category":shared["category"],"width_m":shared["width_m"],"height_m":shared["height_m"],"depth_m":shared["depth_m"]})
    write_rows("products.csv",list(products[0]),products)
    pair_products=["prod-high-compact-1","prod-high-compact-2","prod-high-bulky-1","prod-high-bulky-2","prod-mid-compact-1","prod-mid-compact-2","prod-mid-bulky-1","prod-mid-bulky-2"]
    product_by_code={x["id"]:x for x in PRODUCTS}
    pairs=[{"product_id":product_ids[code],"storage_point_id":point_ids["s1" if i%2==0 else "s2"],"placement_units":[0,5,10,20][(i//2)%4],"slots_per_unit":product_by_code[code]["slotsPerUnit"],"reserved_units":0} for i,code in enumerate(pair_products)]
    write_rows("placement_pairs.csv",list(pairs[0]),pairs)
    trips=[]
    node_by_code=node_ids
    for i,t in enumerate(NETWORK["trips"],1):
        trips.append({"trip_id":trip_ids[t["id"]],"trip_code":t["id"],"trip_type":t["type"],"from_node_id":node_by_code[t["fromNodeId"]],"to_node_id":node_by_code[t["toNodeId"]],"frequency":t["frequency"],"departure_local":t["departureLocal"],"mode":t["mode"]})
    write_rows("trips.csv",list(trips[0]),trips)
    orders=[]
    for i,(key,created,city,point,items) in enumerate(ORDERS,1):
        encoded=[{"product_id":product_ids[x["product_id"]],"quantity":x["quantity"]} for x in items]
        orders.append({"request_key":request_ids[key],"created_at":created,"city_id":city_ids[city],"pickup_point_id":point_ids[point],"items_json":json.dumps(encoded,separators=(",",":"))})
    write_rows("orders.csv",list(orders[0]),orders)
    tariff={"id":str(1_000_000_000+950_000_000+1),**_convert(NETWORK["tariff"],maps)}
    write_json("tariff.json",tariff)
    write_json("api_examples.json",_convert(EXAMPLES,maps))
    expected={"schemaVersion":"2.0","cases": [
      {"key":"city_mesh","requestKeys":[request_ids["city-mesh-1"]],"initialBatch":batch_ids["batch-city-mesh"],"expectedAvailableAfterDispatch":4,"expectedIssued":1},
      {"key":"intercity_hub","requestKeys":[request_ids["intercity-hub-1"]],"expectedHubOnly":True,"meshOnlyReason":"insufficient_stock","comparison":{"ordinary":{"buyerFeeMinor":1800,"etaMinutes":1980},"hybrid":{"buyerFeeMinor":2200,"etaMinutes":2340}}},
      {"key":"mixed_return","requestKeys":[request_ids["mixed-return-1"]],"lineCount":2,"returnedProduct":product_ids["prod-mug"],"returnedQuantity":1,"resellableQuantity":1,"quarantinedQuantity":0},
      {"key":"trip_disruption","requestKeys":[request_ids["trip-disruption-1"]],"failedTrip":trip_ids["city-b-east"],"alternateTrip":trip_ids["city-b-alt"],"expectedRouteRevision":2,"duplicateReserve":False},
      {"key":"long_storage","requestKeys":[],"initialClock":CLOCK,"advanceTo":"2026-01-15T09:00:00Z","quantity":5,"slotsPerUnit":1,"rateMinorPerSlotDay":200,"billableSlotDays":50,"accruedMinor":10000},
      {"key":"local_shelf","requestKeys":[request_ids["local-shelf-1"]],"beforeIssueAvailable":9,"afterIssueAvailable":9,"issuedPhysicalDelta":-1},
      {"key":"last_unit_race","requestKeys":[request_ids["race-first-1"],request_ids["race-second-1"]],"availableUnits":1,"expectedWinnerCount":1,"retryOption":"hub_only"},
      {"key":"capacity_conflict","requestKeys":[],"requiredSlots":5,"availableSlots":4,"status":409,"code":"capacity_exceeded"}],
      "unitChecks":{"cityMesh":{"received":5,"dispatched":1,"remainingAtS1":4,"issuedAtP2":1},"returns":{"soldLines":2,"returnedLines":1,"remainingSoldLines":1}},
      "sharedCatalog":{"path":"datasets/shared/catalog_scenarios_1_5.json","sha256":_sha(SHARED_CATALOG_PATH),"skuValues":[p["sku_value"] for p in SHARED_CATALOG["products"]]}}
    write_json("expected.json",expected)
    records=[]
    for name,meta in files.items(): records.append({"path":name,**meta})
    manifest={"schemaVersion":"2.0.0","id":"demo","seed":seed,"period":{"from":"2026-01-05T09:00:00Z","to":"2026-01-15T09:00:00Z","timezone":"Europe/Moscow"},"initialClock":CLOCK,"start":{"cityId":city_ids["city-b"],"pickupPointId":point_ids["s1"],"at":CLOCK,"caseKey":"city_mesh"},"counts":{"pointCount":6,"preparedOrderCount":7,"orderIntentCount":7,"lineCount":8,"orderCount":7,"caseCount":8},"files":records,"sharedCatalog":{"path":"datasets/shared/catalog_scenarios_1_5.json","sha256":_sha(SHARED_CATALOG_PATH)},"apiContracts":{"frontend":"docs/openapi/frontend.yaml","internal":"docs/openapi/internal.yaml","version":"0.1.0"},"idRules":"^[0-9]+$","reproduction":"python3 scripts/datasets/agents/demo_cases.py generate --seed 42"}
    # Manifest excludes its own checksum to avoid a recursive digest.
    (out/"manifest.json").write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    return out


def validate(out_dir: Path | str = DEFAULT_OUT):
    out=Path(out_dir);manifest=json.loads((out/"manifest.json").read_text(encoding="utf-8"))
    check_id=lambda x: isinstance(x,str) and x.isdigit()
    with (out/"orders.csv").open(encoding="utf-8",newline="") as f: orders=list(csv.DictReader(f))
    assert len(orders)==7 and len({r["request_key"] for r in orders})==7
    for r in orders:
        assert all(check_id(r[k]) for k in ("request_key","city_id","pickup_point_id"))
        items=json.loads(r["items_json"]); ids=[x["product_id"] for x in items]
        assert all(check_id(x) for x in ids) and len(ids)==len(set(ids)) and all(x["quantity"]>0 for x in items)
    with (out/"products.csv").open(encoding="utf-8",newline="") as f: products=list(csv.DictReader(f))
    assert len(products)==12 and len({x["seller_id"] for x in products})==3
    for p in products:
        assert all(check_id(p[k]) for k in ("product_id","seller_id","seller_sku"))
        assert all(float(p[k])>0 for k in ("width_m","height_m","depth_m"))
        assert not any(k in p for k in ("base_area_m2","volume_m3"))
    with (out/"placement_pairs.csv").open(encoding="utf-8",newline="") as f: pairs=list(csv.DictReader(f))
    assert len(pairs)==8 and {int(x["placement_units"]) for x in pairs}=={0,5,10,20}
    ex=json.loads((out/"expected.json").read_text(encoding="utf-8"));assert len(ex["cases"])==8
    assert sum(len(json.loads(r["items_json"])) for r in orders)==8
    for ent in manifest["files"]:
        p=out/ent["path"];assert p.exists() and _sha(p)==ent["sha256"],ent["path"]
        if p.suffix==".csv":
            with p.open(encoding="utf-8",newline="") as f: assert sum(1 for _ in csv.reader(f))-1==ent["rows"]
    for dim,fk,pk in (("cities.csv","city_id","city_id"),("points.csv","pickup_point_id","pickup_point_id"),("products.csv","product_id","product_id")):
        with (out/dim).open(encoding="utf-8",newline="") as f: rows=list(csv.DictReader(f))
        assert all(check_id(x[pk]) for x in rows)
    shared_skus={x["sku_value"]:x for x in SHARED_CATALOG["products"]}
    local_sku={x["seller_sku"]:x["sku_value"] for x in csv.DictReader((out/"seller_skus.csv").open(encoding="utf-8",newline=""))}
    shared_sellers={x["seller_code"]:x["seller_name"] for x in SHARED_CATALOG["sellers"]}
    local_sellers={x["seller_id"]:x["seller_code"] for x in csv.DictReader((out/"sellers.csv").open(encoding="utf-8",newline=""))}
    for product in products:
        sku=shared_skus[local_sku[product["seller_sku"]]]
        assert product["sku_value"]==sku["sku_value"] and local_sellers[product["seller_id"]]==sku["seller_code"]
        for field in ("product_name","demand_class","size_class","weight_grams","slots_per_unit","sale_price_minor","unit_cost_minor","category","width_m","height_m","depth_m"):
            assert str(product[field])==str(sku[field]),(sku["sku_value"],field)
    assert {x["demand_class"] for x in SHARED_CATALOG["products"]}=={"high","medium","low"}
    assert {x["size_class"] for x in SHARED_CATALOG["products"]}=={"compact","bulky"}
    assert all(sum(1 for x in SHARED_CATALOG["products"] if x["demand_class"]==d and x["size_class"]==z)==2 for d in ("high","medium","low") for z in ("compact","bulky"))
    return {"dataset":"demo","cases":8,"orders":len(orders),"lines":8,"points":6,"products":len(products),"placementPairs":len(pairs),"numericIDs":True,"sharedCatalog":True,"valid":True}

if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("command",choices=["generate","validate"]);parser.add_argument("--seed",type=int,default=42);parser.add_argument("--out",type=Path,default=DEFAULT_OUT);a=parser.parse_args()
    if a.command=="generate":generate(a.seed,a.out)
    print(json.dumps(validate(a.out),ensure_ascii=False))
