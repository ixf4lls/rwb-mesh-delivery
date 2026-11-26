package httpapi

import (
	"context"
	"meshdelivery/internal/core"
	"testing"
)

func (a *apiFixture) currentReturn(id string) core.M {
	return a.call("GET", "/api/v1/returns/"+id, nil, 200)
}
func (a *apiFixture) returnEvent(r core.M, body core.M) core.M {
	body["expectedVersion"] = r["version"]
	id := core.S(r["id"])
	a.async("/api/v1/returns/"+id+"/events", body)
	return a.currentReturn(id)
}
func TestPartialReturnHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	o := a.order(a.quote("104", "11", 2), "mesh_only")
	o = a.event(o, "packed", core.S(core.Arr(o["shipments"])[0]["id"]), "")
	o = a.event(o, "issued", "", "")
	line := core.Arr(o["lines"])[0]
	op := a.async("/api/v1/returns", core.M{"orderId": o["id"], "returnPointId": "11", "reason": "synthetic damaged unit", "lines": []core.M{{"orderItemId": line["id"], "quantity": 1}}})
	r := a.currentReturn(core.S(op["resourceId"]))
	r = a.returnEvent(r, core.M{"type": "received_from_customer"})
	r = a.returnEvent(r, core.M{"type": "inspected", "lines": []core.M{{"orderItemId": line["id"], "resellableQuantity": 0, "quarantinedQuantity": 1}}})
	if r["status"] != "closed" {
		t.Fatal(r)
	}
	batches := a.call("GET", "/api/v1/batches?productId=104", nil, 200)
	var available, quarantined int64
	for _, b := range core.Arr(batches["items"]) {
		available += core.I(b["availableQuantity"])
		quarantined += core.I(b["quarantinedQuantity"])
	}
	if available != 8 || quarantined != 1 {
		t.Fatalf("partial damaged return stock available=%d quarantined=%d", available, quarantined)
	}
	entries := a.call("GET", "/api/v1/ledger-entries?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z&orderId="+core.S(o["id"]), nil, 200)
	reversals := 0
	for _, e := range core.Arr(entries["items"]) {
		if core.S(e["originalEntryId"]) != "" {
			reversals++
		}
	}
	if reversals == 0 {
		t.Fatalf("partial return produced no linked reversals: %v", entries)
	}
}
func TestIncidentRerouteHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	o := a.order(a.quote("101", "13", 1), "mesh_only")
	sh := core.Arr(o["shipments"])[0]
	o = a.event(o, "packed", core.S(sh["id"]), "")
	incident := a.call("POST", "/api/v1/incidents", core.M{"orderId": o["id"], "shipmentId": sh["id"], "type": "trip_disrupted", "failureTarget": core.M{"tripId": "31"}, "description": "synthetic failed trip"}, 201)
	o = a.call("GET", "/api/v1/orders/"+core.S(o["id"]), nil, 200)
	promised, fee := o["promisedReadyAt"], o["buyerFeeMinor"]
	a.async("/api/v1/incidents/"+core.S(incident["id"])+"/reroutes", core.M{"expectedOrderVersion": o["version"]})
	fresh := a.call("GET", "/api/v1/orders/"+core.S(o["id"]), nil, 200)
	if core.I(fresh["routeRevision"]) != 2 || fresh["promisedReadyAt"] != promised || fresh["buyerFeeMinor"] != fee {
		t.Fatalf("reroute lost promise/fee or revision: %v", fresh)
	}
	b := a.call("GET", "/api/v1/batches/41", nil, 200)
	if core.I(b["packedQuantity"]) != 1 || core.I(b["usedSlots"]) != 5 {
		t.Fatalf("reroute duplicated/departed stock %v", b)
	}
	a.call("GET", "/api/v1/orders/"+core.S(o["id"])+"/route?revision=1", nil, 200)
	a.call("GET", "/api/v1/orders/"+core.S(o["id"])+"/route?revision=2", nil, 200)
}
func TestLongStorageHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	a.clock("2026-01-15T09:00:00Z")
	c := a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
	cost := core.Obj(c["cost"])
	if core.I(cost["billableSlotDays"]) != 50 || core.I(cost["accruedMinor"]) != 10000 {
		t.Fatalf("wrong model accrual %v", c)
	}
	a.async("/api/v1/storage-cases/storage_45/decisions", core.M{"action": "keep", "reason": "synthetic review", "expectedVersion": c["version"], "recheckAt": "2026-01-16T09:00:00Z"})
	c = a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
	if c["status"] != "deferred" {
		t.Fatal(c)
	}
	a.clock("2026-01-16T09:00:00Z")
	c = a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
	if c["status"] != "open" {
		t.Fatal("deferred threshold did not reopen")
	}
	a.async("/api/v1/storage-cases/storage_45/decisions", core.M{"action": "transfer_to_hub", "reason": "synthetic removal", "expectedVersion": c["version"]})
	c = a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
	route := core.Obj(c["route"])
	var selected core.M
	for _, v := range core.Arr(route["options"]) {
		if v["id"] == c["selectedOptionId"] {
			selected = v
		}
	}
	for _, segment := range core.Arr(core.Arr(selected["shipments"])[0]["segments"]) {
		a.clock(core.S(segment["scheduledStartAt"]))
		a.async("/api/v1/storage-cases/storage_45/transfer-events", core.M{"type": "handed_over", "segmentId": segment["id"], "expectedVersion": c["version"]})
		c = a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
		b := a.call("GET", "/api/v1/batches/45", nil, 200)
		if core.I(b["usedSlots"]) != 0 {
			t.Fatal("slots were not freed at physical handoff")
		}
		a.clock(core.S(segment["scheduledEndAt"]))
		a.async("/api/v1/storage-cases/storage_45/transfer-events", core.M{"type": "arrived", "segmentId": segment["id"], "expectedVersion": c["version"]})
		c = a.call("GET", "/api/v1/storage-cases/storage_45", nil, 200)
	}
	if c["status"] != "closed" {
		t.Fatalf("hub transfer not closed %v", c)
	}
}
func TestCapacityConflictAndLastUnitHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	before := a.call("GET", "/api/v1/pickup-points/11/capacity", nil, 200)
	m := a.call("POST", "/api/v1/batches", core.M{"productId": "101", "storagePointId": "11", "quantity": 1000}, 409)
	if m["code"] != "capacity_exceeded" {
		t.Fatal(m)
	}
	after := a.call("GET", "/api/v1/pickup-points/11/capacity", nil, 200)
	if before["storageUsedSlots"] != after["storageUsedSlots"] {
		t.Fatal("failed receipt mutated capacity")
	}
	q1, q2 := a.quote("106", "13", 1), a.quote("106", "13", 1)
	enqueueOrder := func(q core.M) core.M {
		var id any
		for _, option := range core.Arr(q["options"]) {
			if option["policy"] == "mesh_only" {
				id = option["id"]
			}
		}
		return a.call("POST", "/api/v1/orders", core.M{"quoteId": q["id"], "optionId": id}, 202)
	}
	op1, op2 := enqueueOrder(q1), enqueueOrder(q2)
	for n := 0; n < 2; n++ {
		if _, e := a.db.ProcessOne(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	wins := 0
	for _, op := range []core.M{op1, op2} {
		r := a.call("GET", "/api/v1/operations/"+core.S(op["id"]), nil, 200)
		if r["status"] == "succeeded" {
			wins++
		} else if r["status"] != "failed" {
			t.Fatal(r)
		}
	}
	if wins != 1 {
		t.Fatalf("last unit confirmed %d orders", wins)
	}
	b := a.call("GET", "/api/v1/batches/46", nil, 200)
	if core.I(b["availableQuantity"]) != 0 || core.I(b["reservedQuantity"]) != 1 {
		t.Fatalf("invalid final last-unit stock %v", b)
	}
}
func TestReadContractsAndCursorIsolation(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	for _, p := range []string{"/cities", "/pickup-points", "/products", "/batches", "/orders", "/returns", "/incidents", "/storage-cases", "/notifications", "/demo/datasets", "/demo/context", "/batches/41/storage-cost", "/pickup-points/11/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z", "/sellers/2/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z", "/marketplace/economics?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z"} {
		a.call("GET", "/api/v1"+p, nil, 200)
	}
	first := a.call("GET", "/api/v1/products?limit=1", nil, 200)
	cursor := core.S(first["nextCursor"])
	if cursor == "" {
		t.Fatal("missing cursor")
	}
	next := a.call("GET", "/api/v1/products?limit=1&cursor="+cursor, nil, 200)
	if core.Arr(first["items"])[0]["id"] == core.Arr(next["items"])[0]["id"] {
		t.Fatal("cursor repeated row")
	}
	a.newRun(1)
	a.call("GET", "/api/v1/products?limit=1&cursor="+cursor, nil, 400)
}
func TestMixedBasketWaitsForLastHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	q := a.call("POST", "/api/v1/quotes", core.M{"pickupPointId": "13", "items": []core.M{{"productId": "101", "quantity": 1}, {"productId": "102", "quantity": 1}}}, 201)
	o := a.order(q, "hybrid")
	route := a.call("GET", "/api/v1/orders/"+core.S(o["id"])+"/route", nil, 200)
	var mesh, hub core.M
	for _, p := range core.Arr(route["shipments"]) {
		if p["fromNodeId"] == "11" {
			mesh = p
		} else {
			hub = p
		}
	}
	if mesh == nil || hub == nil {
		t.Fatalf("expected two fulfillment sources: %v", route)
	}
	o = a.event(o, "packed", core.S(mesh["id"]), "")
	a.clock("2026-01-05T11:00:00Z")
	o = a.call("GET", "/api/v1/orders/"+core.S(o["id"]), nil, 200)
	a.clock("2026-01-05T12:00:00Z")
	meshSeg := core.Arr(mesh["segments"])[0]
	hubSeg := core.Arr(hub["segments"])[1]
	o = a.event(o, "handed_over", core.S(mesh["id"]), core.S(meshSeg["id"]))
	o = a.event(o, "handed_over", core.S(hub["id"]), core.S(hubSeg["id"]))
	a.clock("2026-01-05T13:00:00Z")
	o = a.event(o, "arrived", core.S(hub["id"]), core.S(hubSeg["id"]))
	if o["status"] == "ready" {
		t.Fatal("basket ready before final shipment")
	}
	a.clock("2026-01-05T15:00:00Z")
	o = a.event(o, "arrived", core.S(mesh["id"]), core.S(meshSeg["id"]))
	if o["status"] != "ready" {
		t.Fatal("basket not ready after final shipment")
	}
	o = a.event(o, "issued", "", "")
	if o["status"] != "issued" {
		t.Fatal(o)
	}
}
