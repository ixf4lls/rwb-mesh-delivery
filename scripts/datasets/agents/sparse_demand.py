#!/usr/bin/env python3
"""Generate and validate a normalized sparse-demand test dataset."""
import argparse, csv, json, random, re, sys
from datetime import date, datetime, time, timedelta, timezone
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]
sys.path.insert(0,str(ROOT/'scripts'/'datasets'))
from common import numeric_id, product_geometry, write_manifest, sha256
OUT=ROOT/'datasets'/'sparse'; DATASET='sparse'; SEED=42; START=date(2026,1,5)
CANONICAL=json.loads((ROOT/'datasets'/'shared'/'catalog_scenarios_1_5.json').read_text(encoding='utf-8'))
CANONICAL_PRODUCTS=CANONICAL['products']
SELLER_VALUES=[x['seller_code'] for x in CANONICAL['sellers']]
CITY=numeric_id(DATASET,'city',1)
POINT_IDS=[numeric_id(DATASET,'pickup_point',i) for i in range(1,7)]
PRODUCT_IDS=[numeric_id(DATASET,'product',i) for i in range(1,13)]
SELLER_IDS=[numeric_id(DATASET,'seller',i) for i in range(1,4)]
SKU_IDS=[numeric_id(DATASET,'seller_sku',i) for i in range(1,13)]
ORDER_IDS=[numeric_id(DATASET,'order',i) for i in range(1,401)]
FIXTURE_KEYS=[f'sparse-order-{i:06d}' for i in range(1,401)]

def write_csv(path, rows, fields):
    with path.open('w',encoding='utf-8',newline='') as f:
        w=csv.DictWriter(f,fieldnames=fields,lineterminator='\n'); w.writeheader(); w.writerows(rows)

def generate(seed=SEED,out=OUT):
    out=Path(out)
    if out.exists():
        import shutil; shutil.rmtree(out)
    out.mkdir(parents=True)
    rng=random.Random(seed)
    point_codes=[f'pvz-b-{i:02d}' for i in range(1,7)]
    product_codes=[p['sku_value'] for p in CANONICAL_PRODUCTS]
    write_csv(out/'cities.csv',[{'city_id':CITY,'city_code':'city-b','city_name':'Город Б','time_zone':'Europe/Moscow'}],['city_id','city_code','city_name','time_zone'])
    write_csv(out/'points.csv',[{'pickup_point_id':POINT_IDS[i],'city_id':CITY,'point_code':point_codes[i],'point_name':f'ПВЗ {i+1}','kind':'storage_pvz' if i<2 else 'pickup_only','accepts_orders':'true','accepts_returns':'true'} for i in range(6)],['pickup_point_id','city_id','point_code','point_name','kind','accepts_orders','accepts_returns'])
    write_csv(out/'sellers.csv',[{'seller_id':SELLER_IDS[i],'seller_code':s['seller_code'],'seller_name':s['seller_name']} for i,s in enumerate(CANONICAL['sellers'])],['seller_id','seller_code','seller_name'])
    seller_idx={value:i for i,value in enumerate(SELLER_VALUES)}
    seller_for_product=[SELLER_IDS[seller_idx[p['seller_code']]] for p in CANONICAL_PRODUCTS]
    write_csv(out/'seller_skus.csv',[{'seller_sku':SKU_IDS[i],'seller_id':seller_for_product[i],'sku_value':product_codes[i]} for i in range(12)],['seller_sku','seller_id','sku_value'])
    prod=[]
    for i,pid in enumerate(PRODUCT_IDS):
        c=CANONICAL_PRODUCTS[i]
        prod.append({'product_id':pid,'seller_id':seller_for_product[i],'seller_sku':SKU_IDS[i],'sku_value':c['sku_value'],'product_name':c['product_name'],'category':c['category'],'demand_class':c['demand_class'],'size_class':c['size_class'],'weight_grams':c['weight_grams'],'slots_per_unit':c['slots_per_unit'],'sale_price_minor':c['sale_price_minor'],'unit_cost_minor':c['unit_cost_minor'],'width_m':c['width_m'],'height_m':c['height_m'],'depth_m':c['depth_m']})
    write_csv(out/'products.csv',prod,['product_id','seller_id','seller_sku','sku_value','product_name','category','demand_class','size_class','weight_grams','slots_per_unit','sale_price_minor','unit_cost_minor','width_m','height_m','depth_m'])
    quotas=[67,67,67,67,66,66]
    forced=[(1,7,0),(2,7,1),(4,14,2)]
    remaining=[n-sum(1 for p,_,_ in forced if p==i) for i,n in enumerate(quotas)]
    assignment=[i for i,n in enumerate(remaining) for _ in range(n)]; rng.shuffle(assignment)
    orders=[]; keymap=[]
    for i in range(1,401):
        if i<=len(forced): p,offset,sku=forced[i-1]
        else: p,offset=assignment[i-len(forced)-1],rng.randrange(28); sku=rng.randrange(12)
        d=START+timedelta(days=offset); minute=rng.choice([rng.randrange(7*60,11*60),rng.randrange(17*60,22*60)])
        created=datetime.combine(d,time(minute//60,minute%60),timezone.utc)
        orders.append({'request_key':ORDER_IDS[i-1],'created_at':created.isoformat().replace('+00:00','Z'),'city_id':CITY,'pickup_point_id':POINT_IDS[p],'items_json':json.dumps([{'product_id':PRODUCT_IDS[sku],'quantity':1}],separators=(',',':'))})
        keymap.append({'request_key':ORDER_IDS[i-1],'fixture_key':FIXTURE_KEYS[i-1]})
    orders.sort(key=lambda r:(r['created_at'],int(r['request_key'])))
    write_csv(out/'orders.csv',orders,['request_key','created_at','city_id','pickup_point_id','items_json'])
    write_csv(out/'order_key_map.csv',keymap,['request_key','fixture_key'])
    counts={(r['pickup_point_id'],json.loads(r['items_json'])[0]['product_id'],r['created_at'][:10]):0 for r in orders}
    for r in orders:
        k=(r['pickup_point_id'],json.loads(r['items_json'])[0]['product_id'],r['created_at'][:10]); counts[k]+=1
    hist=[]
    for day in range(-28,28):
        d=(START+timedelta(days=day)).isoformat()
        for p in POINT_IDS:
            for sku in PRODUCT_IDS: hist.append({'local_date':d,'pickup_point_id':p,'product_id':sku,'order_count':counts.get((p,sku,d),0)})
    write_csv(out/'demand_history.csv',hist,['local_date','pickup_point_id','product_id','order_count'])
    fit_trip=numeric_id(DATASET,'trip',1); extra_trip=numeric_id(DATASET,'trip',2)
    stop_fit=numeric_id(DATASET,'segment',1); stop_extra=numeric_id(DATASET,'segment',2)
    write_csv(out/'trip_key_map.csv',[{'trip_id':fit_trip,'trip_code':'trip-sparse-fit-20260112'},{'trip_id':extra_trip,'trip_code':'trip-sparse-extra-20260119'}],['trip_id','trip_code'])
    expected={'schemaVersion':'2.0.0','scenario':'sparse','seed':seed,
      'controlDateFit':{'localDate':'2026-01-12','orderIds':ORDER_IDS[:2],'tripId':fit_trip,'segmentType':'city_trip','requestedSlots':2,'backgroundSlots':8,'freeTripSlots':4,'incrementalStops':0,'costItems':[{'category':'trip_share','amountMinor':1200}],'plannedPickupPointIds':[POINT_IDS[1],POINT_IDS[2]],'reservedStockUnits':2},
      'controlDateExtraStop':{'localDate':'2026-01-19','orderIds':[ORDER_IDS[2]],'tripId':extra_trip,'segmentType':'city_trip','requestedSlots':1,'backgroundSlots':10,'freeTripSlots':0,'incrementalStops':1,'distanceMeters':8200,'waitMinutes':18,'costItems':[{'category':'trip_share','amountMinor':600},{'category':'extra_stop','amountMinor':950}],'changedStopIds':[stop_extra],'reservedStockUnits':1},
      'assumptions':{'tripCapacitySlots':10,'extraStopRateMinor':950,'extraStopDistanceMeters':8200,'extraStopMinutes':18,'tariffVersionId':numeric_id(DATASET,'segment',3),'currency':'RUB','commonSchedule':'daily city trips, 09:00 local'},
      'fixtureKeys':[{'request_key':ORDER_IDS[i],'fixture_key':FIXTURE_KEYS[i]} for i in range(3)]}
    (out/'expected.json').write_text(json.dumps(expected,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    examples=[{'contract':'frontend.yaml','schema':'QuoteRequest','direction':'request','case':'sparse_fit','method':'POST','path':'/quotes','body':{'pickupPointId':POINT_IDS[1],'items':[{'productId':PRODUCT_IDS[0],'quantity':1}]}}, {'contract':'frontend.yaml','schema':'QuoteRequest','direction':'request','case':'sparse_extra_stop','method':'POST','path':'/quotes','body':{'pickupPointId':POINT_IDS[4],'items':[{'productId':PRODUCT_IDS[2],'quantity':1}]}}]
    (out/'api_examples.json').write_text(json.dumps(examples,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    files=[p for p in out.iterdir() if p.is_file() and p.name!='manifest.json']
    write_manifest(out,DATASET,seed,files,{'pointCount':6,'preparedOrderCount':400,'orderIntentCount':400,'lineCount':400,'historyRows':4032,'zeroDemandRows':sum(r['order_count']==0 for r in hist)}, {'start':START.isoformat(),'days':28,'historyDays':56,'timezone':'Europe/Moscow'}, {'cityId':CITY,'pickupPointId':POINT_IDS[0],'initialClock':'2026-01-05T00:00:00Z','recordKey':ORDER_IDS[0]})
    validate(out); return json.loads((out/'manifest.json').read_text())

def validate(out=OUT):
    out=Path(out); m=json.loads((out/'manifest.json').read_text())
    for f in m['files']: assert sha256(out/f['path'])==f['sha256'],f['path']
    with (out/'orders.csv').open(encoding='utf-8',newline='') as f: orders=list(csv.DictReader(f))
    assert len(orders)==400 and len({o['request_key'] for o in orders})==400
    ids=[o['request_key'] for o in orders]+[o['city_id'] for o in orders]+[o['pickup_point_id'] for o in orders]
    assert all(re.fullmatch(r'[0-9]+',x) for x in ids)
    counts={p:sum(o['pickup_point_id']==p for o in orders) for p in POINT_IDS}; assert sorted(counts.values())==[66,66,67,67,67,67]
    products=list(csv.DictReader((out/'products.csv').open(encoding='utf-8'))); assert len(products)==12
    sellers=list(csv.DictReader((out/'sellers.csv').open(encoding='utf-8')))
    seller_id_by_code={s['seller_code']:s['seller_id'] for s in sellers}
    sku_rows=list(csv.DictReader((out/'seller_skus.csv').open(encoding='utf-8')))
    sku_by_value={s['sku_value']:s for s in sku_rows}
    assert len(sellers)==3 and len(sku_rows)==12
    for row,canonical in zip(products,CANONICAL_PRODUCTS):
        assert row['sku_value']==canonical['sku_value']
        assert row['seller_id']==seller_id_by_code[canonical['seller_code']]
        assert row['seller_sku']==sku_by_value[canonical['sku_value']]['seller_sku']
        assert sku_by_value[canonical['sku_value']]['seller_id']==row['seller_id']
        for field in ('product_name','category','demand_class','size_class','weight_grams','slots_per_unit','sale_price_minor','unit_cost_minor'):
            assert row[field]==str(canonical[field]),(field,row[field],canonical[field])
        for field in ('width_m','height_m','depth_m'):
            assert abs(float(row[field])-float(canonical[field]))<1e-9
    assert all(re.fullmatch(r'[0-9]+',p[k]) for p in products for k in ('product_id','seller_id','seller_sku'))
    assert all(float(p[k])>0 for p in products for k in ('width_m','height_m','depth_m'))
    assert not {'base_area_m2','volume_m3'} & set(products[0])
    history=list(csv.DictReader((out/'demand_history.csv').open(encoding='utf-8'))); assert len(history)==4032 and sum(int(r['order_count']) for r in history)==400
    assert sum(int(r['order_count'])==0 for r in history)>0
    for o in orders:
        items=json.loads(o['items_json']); assert all(re.fullmatch(r'[0-9]+',i['product_id']) for i in items)
    return {'orders':len(orders),'historyRows':len(history),'ordersByPoint':counts,'zeroDemandRows':sum(int(r['order_count'])==0 for r in history),'products':len(products)}

if __name__=='__main__':
    ap=argparse.ArgumentParser(); ap.add_argument('command',choices=['generate','validate']); ap.add_argument('--seed',type=int,default=SEED); ap.add_argument('--out',type=Path,default=OUT); a=ap.parse_args()
    if a.command=='generate': print(json.dumps(generate(a.seed,a.out),ensure_ascii=False,indent=2))
    else: print(json.dumps(validate(a.out),ensure_ascii=False,indent=2))
