#!/usr/bin/env python3
"""Generate and validate normalized slow-inventory and return fixtures."""
from __future__ import annotations
import argparse,csv,json,random,re,shutil,sys
from datetime import datetime,timedelta,timezone
from pathlib import Path
ROOT=Path(__file__).resolve().parents[3]; sys.path.insert(0,str(ROOT/'scripts'/'datasets'))
from common import numeric_id,product_geometry,write_manifest,sha256
OUT=ROOT/'datasets'/'slow_returns'; DATASET='slow_returns'; SEED=42
CANONICAL=json.loads((ROOT/'datasets'/'shared'/'catalog_scenarios_1_5.json').read_text(encoding='utf-8'))
CANONICAL_PRODUCTS=CANONICAL['products']; SELLER_VALUES=[x['seller_code'] for x in CANONICAL['sellers']]
START=datetime(2026,1,1,9,tzinfo=timezone.utc); CITY=numeric_id(DATASET,'city',1)
POINT_IDS=[numeric_id(DATASET,'pickup_point',i) for i in range(1,7)]
PRODUCT_IDS=[numeric_id(DATASET,'product',i) for i in range(1,13)]
SELLER_IDS=[numeric_id(DATASET,'seller',i) for i in range(1,4)]
SKU_IDS=[numeric_id(DATASET,'seller_sku',i) for i in range(1,13)]
ORDER_IDS=[numeric_id(DATASET,'order',i) for i in range(1,401)]

def write_csv(path,rows,fields):
    with path.open('w',encoding='utf-8',newline='') as f:
        w=csv.DictWriter(f,fieldnames=fields,lineterminator='\n'); w.writeheader(); w.writerows(rows)
def generate(seed=SEED,out_dir=OUT):
    rng=random.Random(seed); out_dir=Path(out_dir)
    if out_dir.exists(): shutil.rmtree(out_dir)
    out_dir.mkdir(parents=True)
    point_codes=[f'pvz-b-{i:02d}' for i in range(1,7)]; product_codes=[p['sku_value'] for p in CANONICAL_PRODUCTS]
    write_csv(out_dir/'cities.csv',[{'city_id':CITY,'city_code':'city-b','city_name':'Город Б','time_zone':'Europe/Moscow'}],['city_id','city_code','city_name','time_zone'])
    write_csv(out_dir/'points.csv',[{'pickup_point_id':POINT_IDS[i],'city_id':CITY,'point_code':point_codes[i],'point_name':f'ПВЗ {i+1}','kind':'storage_pvz' if i<2 else 'pickup_only','accepts_orders':'true','accepts_returns':'true'} for i in range(6)],['pickup_point_id','city_id','point_code','point_name','kind','accepts_orders','accepts_returns'])
    write_csv(out_dir/'sellers.csv',[{'seller_id':SELLER_IDS[i],'seller_code':s['seller_code'],'seller_name':s['seller_name']} for i,s in enumerate(CANONICAL['sellers'])],['seller_id','seller_code','seller_name'])
    seller_idx={value:i for i,value in enumerate(SELLER_VALUES)}
    seller_for_product=[SELLER_IDS[seller_idx[p['seller_code']]] for p in CANONICAL_PRODUCTS]
    write_csv(out_dir/'seller_skus.csv',[{'seller_sku':SKU_IDS[i],'seller_id':seller_for_product[i],'sku_value':product_codes[i]} for i in range(12)],['seller_sku','seller_id','sku_value'])
    prod=[]
    for i,pid in enumerate(PRODUCT_IDS):
        c=CANONICAL_PRODUCTS[i]
        prod.append({'product_id':pid,'seller_id':seller_for_product[i],'seller_sku':SKU_IDS[i],'sku_value':c['sku_value'],'product_name':c['product_name'],'category':c['category'],'demand_class':c['demand_class'],'size_class':c['size_class'],'weight_grams':c['weight_grams'],'slots_per_unit':c['slots_per_unit'],'sale_price_minor':c['sale_price_minor'],'unit_cost_minor':c['unit_cost_minor'],'width_m':c['width_m'],'height_m':c['height_m'],'depth_m':c['depth_m']})
    write_csv(out_dir/'products.csv',prod,['product_id','seller_id','seller_sku','sku_value','product_name','category','demand_class','size_class','weight_grams','slots_per_unit','sale_price_minor','unit_cost_minor','width_m','height_m','depth_m'])
    orders=[]; keymap=[]
    for i in range(400):
        day=i%28; fixture=f'sr-order-{i+1:04d}'; oid=ORDER_IDS[i]
        is_slow=i<3; point=POINT_IDS[0] if is_slow else POINT_IDS[(i*5+day)%6]; sku=PRODUCT_IDS[11] if is_slow else PRODUCT_IDS[(i*7+3)%11]
        if is_slow: day=[2,12,23][i]
        created=START+timedelta(days=day,hours=(i*11)%24,minutes=(i*17)%60)
        orders.append({'request_key':oid,'created_at':created.isoformat().replace('+00:00','Z'),'city_id':CITY,'pickup_point_id':point,'items_json':json.dumps([{'product_id':sku,'quantity':1}],separators=(',',':'))})
        keymap.append({'request_key':oid,'fixture_key':fixture})
    orders.sort(key=lambda x:(x['created_at'],int(x['request_key'])))
    write_csv(out_dir/'orders.csv',orders,['request_key','created_at','city_id','pickup_point_id','items_json']); write_csv(out_dir/'order_key_map.csv',keymap,['request_key','fixture_key'])
    returned=[o for o in orders if o['request_key'] not in set(ORDER_IDS[:3])][:8]; returns=[]
    for j,o in enumerate(returned):
        issued=datetime.fromisoformat(o['created_at'].replace('Z','+00:00'))+timedelta(days=2); request_at=issued+timedelta(days=1+(j*3)%14); quarantine=j>=6
        product=json.loads(o['items_json'])[0]['product_id']; returns.append({'return_id':numeric_id(DATASET,'return',j+1),'request_key':o['request_key'],'order_item_id':numeric_id(DATASET,'segment',j+1),'product_id':product,'return_point_id':o['pickup_point_id'],'issued_at':issued.isoformat().replace('+00:00','Z'),'requested_at':request_at.isoformat().replace('+00:00','Z'),'reason':'Товар не подошёл','quantity':1,'received_event':'received_from_customer','inspection_event':'inspected','expected_return_version':2,'resellable_quantity':0 if quarantine else 1,'quarantined_quantity':1 if quarantine else 0,'reverse_route_id':numeric_id(DATASET,'trip',j+1),'responsible_party':'seller'})
    write_csv(out_dir/'return_intents.csv',returns,['return_id','request_key','order_item_id','product_id','return_point_id','issued_at','requested_at','reason','quantity','received_event','inspection_event','expected_return_version','resellable_quantity','quarantined_quantity','reverse_route_id','responsible_party'])
    hist=[]; hist_start=START-timedelta(days=56)
    for d in range(56):
        for point in POINT_IDS:
            for product in PRODUCT_IDS:
                n=1 if ((d*13+int(point)%97*7+int(product)%89*3)%19==0) else 0
                hist.append({'date':(hist_start+timedelta(days=d)).date().isoformat(),'city_id':CITY,'pickup_point_id':point,'product_id':product,'units':n})
    write_csv(out_dir/'demand_history.csv',hist,['date','city_id','pickup_point_id','product_id','units'])
    batch_id=numeric_id(DATASET,'batch',1); slow_product=PRODUCT_IDS[11]
    expected={'slowBatch':{'batch_id':batch_id,'product_id':slow_product,'storage_point_id':POINT_IDS[0],'received_at':'2025-12-31T09:12:00Z','received_quantity':20,'slots_per_unit':1,'units_sold':3,'remaining_after_three_issues':17,'additional_replenishments':0,'tariff':{'currency':'RUB','storage_threshold_days':10,'storage_rate_minor_per_slot_day':200}},'storageDecision':{'action':'keep','reason':'Сверить остаток на контрольном срезе','expectedVersion':1,'recheckAt':'2026-01-30T09:00:00Z','expectedStatus':'deferred','followupAction':'transfer_to_hub'},'returns':{'intentCount':8,'distinctOrders':8,'resellableCount':6,'quarantinedCount':2,'afterCohortWindowCount':sum(datetime.fromisoformat(r['requested_at'].replace('Z','+00:00'))>=START+timedelta(days=28) for r in returns),'returnIds':[r['return_id'] for r in returns]},'slowBatchStorageProjection':{'totalSlotDays':499,'accruedStorageCostMinor':99800,'ageDaysAtCohortEnd':29,'plannedTransferCostMinor':1200,'note':'Illustrative accrued full-24-hour slot storage through cohort end.'}}
    (out_dir/'expected.json').write_text(json.dumps(expected,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    examples=[{'contract':'frontend.yaml','schema':'ReturnCreate','direction':'request','method':'POST','path':'/returns','body':{'orderId':ORDER_IDS[10],'returnPointId':POINT_IDS[0],'reason':'Товар не подошёл','lines':[{'orderItemId':numeric_id(DATASET,'segment',1),'quantity':1}]}},{'contract':'frontend.yaml','schema':'ReturnEvent','direction':'request','method':'POST','path':f"/returns/{numeric_id(DATASET,'return',1)}/events",'body':{'type':'received_from_customer','expectedVersion':1}},{'contract':'frontend.yaml','schema':'ReturnInspection','direction':'request','method':'POST','path':f"/returns/{numeric_id(DATASET,'return',1)}/events",'body':{'type':'inspected','expectedVersion':2,'lines':[{'orderItemId':numeric_id(DATASET,'segment',1),'resellableQuantity':1,'quarantinedQuantity':0}]}}]
    (out_dir/'api_examples.json').write_text(json.dumps({'apiRecords':examples},ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    files=[p for p in out_dir.iterdir() if p.is_file() and p.name!='manifest.json']
    counts={'pointCount':6,'preparedOrderCount':400,'orderIntentCount':400,'orderCount':400,'lineCount':400,'returnIntentCount':8,'lineHistoryCount':4032}
    write_manifest(out_dir,DATASET,seed,files,counts,{'timezone':'Europe/Moscow','from':'2026-01-01','to':'2026-01-29','historyDays':56},{'cityId':CITY,'pickupPointId':POINT_IDS[0],'initialClock':'2025-12-31T09:12:00Z','showcaseKey':ORDER_IDS[0]})
    return out_dir

def validate(out_dir=OUT):
    out_dir=Path(out_dir); m=json.loads((out_dir/'manifest.json').read_text())
    for f in m['files']: assert sha256(out_dir/f['path'])==f['sha256'],f['path']
    orders=list(csv.DictReader((out_dir/'orders.csv').open(encoding='utf-8'))); returns=list(csv.DictReader((out_dir/'return_intents.csv').open(encoding='utf-8'))); hist=list(csv.DictReader((out_dir/'demand_history.csv').open(encoding='utf-8')))
    assert len(orders)==400 and len({o['request_key'] for o in orders})==400
    slow=[o for o in orders if json.loads(o['items_json'])[0]['product_id']==PRODUCT_IDS[11]]; assert len(slow)==3 and all(o['pickup_point_id']==POINT_IDS[0] for o in slow)
    products=list(csv.DictReader((out_dir/'products.csv').open(encoding='utf-8')))
    sellers=list(csv.DictReader((out_dir/'sellers.csv').open(encoding='utf-8'))); seller_id_by_code={s['seller_code']:s['seller_id'] for s in sellers}
    sku_rows=list(csv.DictReader((out_dir/'seller_skus.csv').open(encoding='utf-8'))); sku_by_value={s['sku_value']:s for s in sku_rows}
    assert len(products)==12 and len(sellers)==3 and len(sku_rows)==12
    for row,canonical in zip(products,CANONICAL_PRODUCTS):
        assert row['sku_value']==canonical['sku_value']
        assert row['seller_id']==seller_id_by_code[canonical['seller_code']]
        assert row['seller_sku']==sku_by_value[canonical['sku_value']]['seller_sku']
        assert sku_by_value[canonical['sku_value']]['seller_id']==row['seller_id']
        for field in ('product_name','category','demand_class','size_class','weight_grams','slots_per_unit','sale_price_minor','unit_cost_minor'):
            assert row[field]==str(canonical[field]),(field,row[field],canonical[field])
        for field in ('width_m','height_m','depth_m'):
            assert abs(float(row[field])-float(canonical[field]))<1e-9
    assert len(returns)==8 and len({r['request_key'] for r in returns})==8
    assert sum(int(r['resellable_quantity']) for r in returns)==6 and sum(int(r['quarantined_quantity']) for r in returns)==2
    assert len(hist)==4032
    id_fields=['request_key','city_id','pickup_point_id']
    assert all(re.fullmatch(r'[0-9]+',row[k]) for row in orders for k in id_fields)
    assert all(re.fullmatch(r'[0-9]+',row[k]) for row in returns for k in ('return_id','request_key','order_item_id','product_id','return_point_id','reverse_route_id'))
    products=list(csv.DictReader((out_dir/'products.csv').open(encoding='utf-8'))); assert len(products)==12 and all(float(x[k])>0 for x in products for k in ('width_m','height_m','depth_m'))
    assert not {'base_area_m2','volume_m3'} & set(products[0])
    return {'orders':len(orders),'slowSkuOrders':len(slow),'returnIntents':len(returns),'resellable':6,'quarantined':2,'historyRows':len(hist),'products':len(products)}

if __name__=='__main__':
    p=argparse.ArgumentParser(); p.add_argument('command',choices=['generate','validate']); p.add_argument('--seed',type=int,default=SEED); p.add_argument('--out',type=Path,default=OUT); a=p.parse_args()
    if a.command=='generate': generate(a.seed,a.out); print(json.dumps(validate(a.out),ensure_ascii=False))
    else: print(json.dumps(validate(a.out),ensure_ascii=False))
