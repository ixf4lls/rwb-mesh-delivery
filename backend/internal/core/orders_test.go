package core

import "testing"

func TestOrdersLocalShelfAndVersionAccess(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "11", []M{{"productId": "104", "quantity": 1}})
	o := testOrder(t, tx, q, "mesh_only")
	state := Arr(o["shipments"])[0]
	batch, e := tx.Get("batches", "44")
	if e != nil {
		t.Fatal(e)
	}
	if I(batch["availableQuantity"]) != 9 || I(batch["reservedQuantity"]) != 1 || I(batch["usedSlots"]) != 10 {
		t.Fatal(batch)
	}
	_, e = ApplyOrderEvent(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(o["id"])}, Body: M{"type": "packed", "shipmentId": state["id"], "expectedVersion": 999}})
	if p, ok := e.(*Problem); !ok || p.Code != "stale_version" {
		t.Fatalf("stale version %v", e)
	}
	_, e = ApplyOrderEvent(tx, Command{Actor: Actor{Roles: []string{"pvz_operator"}, PointIDs: []string{"12"}}, Params: map[string]string{"orderId": S(o["id"])}, Body: M{"type": "packed", "shipmentId": state["id"], "expectedVersion": o["version"]}})
	if p, ok := e.(*Problem); !ok || p.Status != 403 {
		t.Fatalf("foreign point %v", e)
	}
	o = testOrderEvent(t, tx, o, "packed", S(state["id"]), "")
	if S(o["status"]) != "ready" {
		t.Fatal(o)
	}
	batch, _ = tx.Get("batches", "44")
	if I(batch["availableQuantity"]) != 9 || I(batch["packedQuantity"]) != 1 || I(batch["usedSlots"]) != 10 {
		t.Fatalf("packing removed physical stock %#v", batch)
	}
	o = testOrderEvent(t, tx, o, "issued", "", "")
	batch, _ = tx.Get("batches", "44")
	if I(batch["availableQuantity"]) != 9 || I(batch["packedQuantity"]) != 0 || I(batch["usedSlots"]) != 9 {
		t.Fatal(batch)
	}
	if S(o["settlementStatus"]) != "posted" {
		t.Fatal(o)
	}
	entries, e := tx.List("ledger", M{"orderId": o["id"]})
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) == 0 {
		entries, e = tx.List("ledger-entries", M{"orderId": o["id"]})
		if e != nil || len(entries) == 0 {
			t.Fatal("issuance did not create accounting", entries, e)
		}
	}
}
func TestOrdersTransportAndSegmentValidation(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := testOrder(t, tx, q, "mesh_only")
	sid := S(Arr(o["shipments"])[0]["id"])
	segment := Arr(Arr(o["plannedShipments"])[0]["segments"])[0]
	o = testOrderEvent(t, tx, o, "packed", sid, "")
	_, e := ApplyOrderEvent(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(o["id"])}, Body: M{"type": "handed_over", "shipmentId": sid, "segmentId": "foreign", "expectedVersion": o["version"]}})
	if p, ok := e.(*Problem); !ok || p.Code != "invalid_segment" {
		t.Fatalf("foreign segment %v", e)
	}
	tx.Clock = Time(segment["scheduledStartAt"])
	o = testOrderEvent(t, tx, o, "handed_over", sid, S(segment["id"]))
	batch, _ := tx.Get("batches", "41")
	if I(batch["usedSlots"]) != 4 || I(batch["packedQuantity"]) != 0 {
		t.Fatalf("handover did not physically remove stock %#v", batch)
	}
	_, e = ApplyOrderEvent(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(o["id"])}, Body: M{"type": "issued", "expectedVersion": o["version"]}})
	if e == nil {
		t.Fatal("issued basket in transit")
	}
	tx.Clock = Time(segment["scheduledEndAt"])
	o = testOrderEvent(t, tx, o, "arrived", sid, S(segment["id"]))
	if S(o["status"]) != "ready" {
		t.Fatal(o)
	}
	o = testOrderEvent(t, tx, o, "issued", "", "")
	if S(o["status"]) != "issued" {
		t.Fatal(o)
	}
	events, e := tx.List("order-events", M{"orderId": o["id"]})
	if e != nil || len(events) != 5 {
		t.Fatal(events, e)
	}
	hold, e := tx.Get("route-holds", S(o["holdId"]))
	if e != nil || S(hold["status"]) != "completed" {
		t.Fatal(hold, e)
	}
}
func TestOrdersMixedWaitsForLastAndSimulationReplays(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}, {"productId": "102", "quantity": 1}})
	o := testOrder(t, tx, q, "hybrid")
	var meshPlan, hubPlan M
	for _, p := range Arr(o["plannedShipments"]) {
		if S(p["fromNodeId"]) == "11" {
			meshPlan = p
		} else {
			hubPlan = p
		}
	}
	meshID, hubID := S(meshPlan["id"]), S(hubPlan["id"])
	o = testOrderEvent(t, tx, o, "packed", meshID, "")
	tx.Clock = Time("2026-01-05T11:00:00Z")
	if e := AdvanceOrders(tx); e != nil {
		t.Fatal(e)
	}
	o, _ = tx.Get("orders", S(o["id"]))
	states := Arr(o["shipments"])
	var hub M
	for _, sh := range states {
		if S(sh["id"]) == hubID {
			hub = sh
		}
	}
	if S(hub["lastKnownNodeId"]) != "22" || S(hub["status"]) != "at_node" {
		t.Fatalf("hub simulation %#v", hub)
	}
	events, _ := tx.List("order-events", M{"orderId": o["id"]})
	before := len(events)
	if e := AdvanceOrders(tx); e != nil {
		t.Fatal(e)
	}
	events, _ = tx.List("order-events", M{"orderId": o["id"]})
	if len(events) != before {
		t.Fatal("simulation replay duplicated transitions")
	}
	tx.Clock = Time("2026-01-05T12:00:00Z")
	meshSegment := Arr(meshPlan["segments"])[0]
	o = testOrderEvent(t, tx, o, "handed_over", meshID, S(meshSegment["id"]))
	hubSegment := Arr(hubPlan["segments"])[1]
	o = testOrderEvent(t, tx, o, "handed_over", hubID, S(hubSegment["id"]))
	tx.Clock = Time("2026-01-05T13:00:00Z")
	o = testOrderEvent(t, tx, o, "arrived", hubID, S(hubSegment["id"]))
	if S(o["status"]) == "ready" {
		t.Fatal("basket ready before last shipment")
	}
	tx.Clock = Time("2026-01-05T15:00:00Z")
	o = testOrderEvent(t, tx, o, "arrived", meshID, S(meshSegment["id"]))
	if S(o["status"]) != "ready" {
		t.Fatal(o)
	}
}
func TestOrdersReroutePreservesPromiseAndStock(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := testOrder(t, tx, q, "mesh_only")
	sid := S(Arr(o["shipments"])[0]["id"])
	o = testOrderEvent(t, tx, o, "packed", sid, "")
	promise, price := S(o["promisedReadyAt"]), I(o["buyerFeeMinor"])
	previous, _ := tx.Get("batches", "41")
	blocked, e := BlockOrderSegment(tx, Command{Params: map[string]string{"orderId": S(o["id"]), "incidentId": "incident1"}, Body: M{"shipmentId": sid, "expectedRouteRevision": 1, "failureTarget": M{"tripId": "31"}}})
	if e != nil {
		t.Fatal(e)
	}
	o, e = ReplaceOrderRoute(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(o["id"])}, Body: M{"incidentId": "incident1", "shipmentId": sid, "failureTarget": M{"tripId": "31"}, "expectedVersion": blocked["version"]}})
	if e != nil {
		t.Fatal(e)
	}
	if S(o["promisedReadyAt"]) != promise || I(o["buyerFeeMinor"]) != price || I(o["routeRevision"]) != 2 {
		t.Fatal(o)
	}
	batch, _ := tx.Get("batches", "41")
	if I(batch["reservedQuantity"]) != I(previous["reservedQuantity"]) || I(batch["packedQuantity"]) != I(previous["packedQuantity"]) {
		t.Fatal("reroute re-reserved stock", batch, previous)
	}
	route, e := tx.Get("routes", routeID(S(o["id"]), 2))
	if e != nil {
		t.Fatal(e)
	}
	seg := Arr(Arr(route["shipments"])[0]["segments"])[0]
	if S(seg["tripId"]) != "32" || I(route["deltaEtaMinutes"]) != 120 {
		t.Fatal(route)
	}
}
func TestOrdersCancellationReleasesWholeBasket(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}, {"productId": "102", "quantity": 1}})
	o := testOrder(t, tx, q, "hybrid")
	o = testOrderEvent(t, tx, o, "canceled", "", "")
	if S(o["status"]) != "canceled" {
		t.Fatal(o)
	}
	b, _ := tx.Get("batches", "41")
	if I(b["availableQuantity"]) != 5 || I(b["reservedQuantity"]) != 0 {
		t.Fatal(b)
	}
	cap, e := PointCapacity(tx, "13")
	if e != nil || I(cap["inboundReservedSlots"]) != 0 {
		t.Fatal(cap, e)
	}
}

func TestOrdersSellerProjectionAndFilters(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	p, e := tx.Get("products", "104")
	if e != nil {
		t.Fatal(e)
	}
	p["sellerId"] = "other-seller"
	if e = tx.Put("products", p); e != nil {
		t.Fatal(e)
	}
	q := testQuote(t, tx, "11", []M{{"productId": "101", "quantity": 1}, {"productId": "104", "quantity": 1}})
	o := testOrder(t, tx, q, "mesh_only")
	seller := Actor{Subject: "seller", Roles: []string{"seller"}, SellerIDs: []string{"2"}}
	visible, e := visibleOrder(Command{Actor: seller}, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(Arr(visible["lines"])) != 1 || S(Arr(visible["lines"])[0]["productId"]) != "101" {
		t.Fatal(visible)
	}
	route, ok, e := ordersHandler(tx, Command{Name: "getOrderRoute", Actor: seller, Params: map[string]string{"orderId": S(o["id"])}})
	if !ok || e != nil {
		t.Fatal(e)
	}
	contents := Arr(Arr(route["shipments"])[0]["contents"])
	if len(contents) != 1 || S(contents[0]["productId"]) != "101" {
		t.Fatalf("foreign seller route contents leaked %#v", contents)
	}
	rows, _, e := ordersHandler(tx, Command{Name: "listOrders", Actor: dispatcher(), Query: map[string][]string{"pointId": {"12"}}})
	if e != nil || len(Arr(rows["items"])) != 0 {
		t.Fatal(rows, e)
	}
	rows, _, e = ordersHandler(tx, Command{Name: "listOrders", Actor: dispatcher(), Query: map[string][]string{"sellerId": {"other-seller"}}})
	if e != nil || len(Arr(rows["items"])) != 1 {
		t.Fatal(rows, e)
	}
}
func TestOrdersBlockedTransitCanConfirmActualArrival(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := testOrder(t, tx, q, "mesh_only")
	sid := S(Arr(o["shipments"])[0]["id"])
	seg := Arr(Arr(o["plannedShipments"])[0]["segments"])[0]
	o = testOrderEvent(t, tx, o, "packed", sid, "")
	tx.Clock = Time(seg["scheduledStartAt"])
	o = testOrderEvent(t, tx, o, "handed_over", sid, S(seg["id"]))
	blocked, e := BlockOrderSegment(tx, Command{Params: map[string]string{"orderId": S(o["id"]), "incidentId": "incident-transit"}, Body: M{"shipmentId": sid, "expectedRouteRevision": 1, "failureTarget": M{"tripId": "31"}}})
	if e != nil {
		t.Fatal(e)
	}
	tx.Clock = Time(seg["scheduledEndAt"])
	o = testOrderEvent(t, tx, blocked, "arrived", sid, S(seg["id"]))
	if S(Arr(o["shipments"])[0]["lastKnownNodeId"]) != "13" {
		t.Fatal(o)
	}
}
func TestOrdersSimulationActionRejectsDifferentCommand(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "102", "quantity": 1}})
	o := testOrder(t, tx, q, "hub_only")
	actions, e := simulatedOrderActions(tx)
	if e != nil || len(actions) != 1 {
		t.Fatal(actions, e)
	}
	a := actions[0]
	actor := dispatcher()
	actor.Service = "demo"
	c := Command{Actor: actor, Params: map[string]string{"orderId": S(o["id"])}, Body: Obj(a["command"]), SimulationID: S(a["id"])}
	_, e = ApplyOrderEvent(tx, c)
	if e != nil {
		t.Fatal(e)
	}
	c.Body = Clone(c.Body)
	c.Body["type"] = "arrived"
	_, e = ApplyOrderEvent(tx, c)
	if p, ok := e.(*Problem); !ok || p.Code != "idempotency_mismatch" {
		t.Fatalf("reused action %v", e)
	}
}

func TestOrdersSimulationRequiresPlannedIdentityAndRecordsDueTime(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "102", "quantity": 1}})
	o := testOrder(t, tx, q, "hub_only")
	actions, e := simulatedOrderActions(tx)
	if e != nil || len(actions) != 1 {
		t.Fatal(actions, e)
	}
	a := actions[0]
	actor := dispatcher()
	actor.Service = "demo"
	c := Command{Actor: actor, Params: map[string]string{"orderId": S(o["id"])}, Body: Obj(a["command"]), SimulationID: "forged"}
	_, e = ApplyOrderEvent(tx, c)
	if p, ok := e.(*Problem); !ok || p.Code != "invalid_simulation_action" {
		t.Fatalf("forged simulation action %v", e)
	}
	c.SimulationID = S(a["id"])
	tx.Clock = Time("2026-01-05T11:00:00Z")
	clock := tx.Now()
	_, e = ApplyOrderEvent(tx, c)
	if e != nil {
		t.Fatal(e)
	}
	event, e := tx.Get("order-events", c.SimulationID)
	if e != nil || S(event["occurredAt"]) != S(a["dueAt"]) || !tx.Now().Equal(clock) {
		t.Fatal("simulation event timestamp or clock incorrect", event, tx.Now(), e)
	}
}
