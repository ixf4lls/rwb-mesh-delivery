"""Complete synthetic fixtures with deterministic API-ready inventory and schedules."""
import csv
import hashlib
import json
import sys
from pathlib import Path
from datetime import datetime, timedelta

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / 'datasets'

def read(folder, name):
    path = folder / (name + '.csv')
    if not path.exists():
        return []
    with path.open(encoding='utf-8-sig', newline='') as f:
        return list(csv.DictReader(f))

def save(folder, name, rows):
    if not rows:
        return
    fields = list(dict.fromkeys(k for row in rows for k in row))
    with (folder / (name + '.csv')).open('w', encoding='utf-8', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=fields, lineterminator='\n')
        writer.writeheader()
        writer.writerows(rows)

def encode(value):
    return json.dumps(value, ensure_ascii=False, indent=2) + '\n'

def generate(item):
    folder = DATA / item['id']
    manifest = json.loads((folder / 'manifest.json').read_text(encoding='utf-8'))
    clock = item['initialClock']
    manifest['initialClock'] = clock
    manifest['counts']['orderCount'] = item['orderCount']
    manifest['counts']['orderIntentCount'] = len(read(folder, 'orders'))
    points = read(folder, 'points')
    nodes = {r['node_id']: r for r in read(folder, 'nodes')}
    for p in points:
        node = p.get('node_id') or p['pickup_point_id']
        storage = p.get('kind') == 'storage_pvz' or p.get('storage_enabled') == 'true'
        p.update(node_id=node, active='true', storage_enabled=str(storage).lower())
        p.setdefault('accepts_orders', 'true')
        p.setdefault('accepts_returns', 'true')
        p.setdefault('opening_hours_json', json.dumps([dict(weekdays=list(range(1,8)), opens='00:00', closes='23:59')]))
        nodes[node] = dict(nodes.get(node, {}), node_id=node, city_id=p['city_id'], node_name=p['point_name'], kind='storage_pvz' if storage else 'pickup_only', active='true', mode='physical')
    for n in nodes.values():
        n.setdefault('active', 'true')
        n['mode'] = 'physical' if n.get('kind') in ('storage_pvz','pickup_only') else 'simulated'
    storage = [p for p in points if p['storage_enabled'] == 'true']
    if not storage:
        storage = [points[0]]
        storage[0]['storage_enabled'] = 'true'
        nodes[storage[0]['node_id']]['kind'] = 'storage_pvz'
    products = read(folder, 'products')
    tariff = json.loads((ROOT/'backend/testdata/demo/initial_state.json').read_text())['tariffs'][0]
    tariff['id'] = '9900000001'
    tariff['effectiveFrom'] = clock
    batches = []
    occupancy = {p['node_id']: 0 for p in points}
    for i, product in enumerate(products):
        p = storage[i % len(storage)]
        qty = 5
        slots = qty * int(product['slots_per_unit'])
        occupancy[p['node_id']] += slots
        batches.append(dict(id=str(9800000001+i), productId=product['product_id'], sellerId=product['seller_id'], storagePointId=p['node_id'], receivedAt=clock, receivedQuantity=qty, availableQuantity=qty, reservedQuantity=0, packedQuantity=0, quarantinedQuantity=0, usedSlots=slots, tariffVersionId=tariff['id'], storageRateMinorPerSlotDay=200))
    for p in points:
        used = occupancy[p['node_id']]
        total = max(int(p['capacity_cells']), used + 100)
        p['capacity_json'] = json.dumps(dict(totalSlots=total, storageQuotaSlots=total if p['storage_enabled']=='true' else 0, ordinaryUsedSlots=0, pickupUsedSlots=0, inboundReservedSlots=0))
    save(folder, 'points', points)
    save(folder, 'nodes', list(nodes.values()))
    # Keep original transport inputs; add complete deterministic daily runtime schedules.
    original = folder / 'trips.csv'
    if original.exists() and not (folder/'source_trips.csv').exists():
        (folder/'source_trips.csv').write_bytes(original.read_bytes())
    trips = []
    by_city = {}
    for p in points:
        by_city.setdefault(p['city_id'], []).append(p['node_id'])
    edges = set()
    hubs = []
    for city, ids in by_city.items():
        hub = next((n['node_id'] for n in nodes.values() if n.get('city_id')==city and n.get('kind')=='hub'), ids[0])
        hubs.append(hub)
        for node in ids:
            if node != hub:
                edges.update(((node,hub),(hub,node)))
    for a,b in zip(hubs,hubs[1:]):
        edges.update(((a,b),(b,a)))
    start = datetime.fromisoformat(clock.replace('Z','+00:00'))
    for day in range(8):
        for a,b in sorted(edges):
            departure = (start + timedelta(days=day,hours=2)).isoformat()
            trips.append(dict(trip_id=str(9700000001+len(trips)), from_node_id=a,to_node_id=b,mode='physical',capacity_slots=200,background_slots=0,departure_at=departure,duration_minutes=60,stops_json=json.dumps([dict(nodeId=a,offsetMinutes=0),dict(nodeId=b,offsetMinutes=60)])))
    save(folder, 'trips', trips)
    (folder/'initial_state.json').write_text(encode({'batches':batches,'tariffs':[tariff]}),encoding='utf-8')
    # Import nodes before pickup points so canonical node bindings are already available.
    paths = [r['path'] for r in manifest['files'] if r['path'] not in ('nodes.csv','points.csv','trips.csv','initial_state.json','tariff.json')]
    paths = ['nodes.csv','points.csv'] + paths + ['trips.csv','initial_state.json']
    records = []
    for name in dict.fromkeys(paths):
        path = folder/name
        if path.suffix == '.csv':
            with path.open(encoding='utf-8-sig',newline='') as f:
                rows = max(0,sum(1 for _ in csv.reader(f))-1)
        else:
            value = json.loads(path.read_text(encoding='utf-8'))
            rows = len(value) if isinstance(value,list) else 1
        records.append(dict(path=name,rows=rows,sha256=hashlib.sha256(path.read_bytes()).hexdigest()))
    manifest['files'] = records
    manifest['runtimeGeneration'] = dict(command='python scripts/prepare_runtime_datasets.py', seed=42, inventory='5 units per product, distributed over storage PVZ', capacity='at least stock slots plus 100', transport='bidirectional city stars and intercity chain, daily for 8 days, 200 slots, 60 minutes')
    (folder/'manifest.json').write_text(encode(manifest),encoding='utf-8')
    print(item['id'], len(points), 'points', len(batches), 'batches', len(trips), 'trips')

if __name__ == '__main__':
    for item in json.loads((DATA/'catalog.json').read_text(encoding='utf-8'))['items']:
        if item['status']=='ready' and (len(sys.argv) == 1 or item['id'] in sys.argv[1:]):
            generate(item)
