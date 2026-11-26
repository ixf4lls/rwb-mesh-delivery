package core

import (
	"context"
	"sync"
	"testing"
)

func TestInventoryCapacityAndLocalPhysicalIssue(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	actor := Actor{Subject: "synthetic", Roles: []string{"dispatcher"}}
	cap, e := PointCapacity(tx, "11")
	if e != nil {
		t.Fatal(e)
	}
	before := I(cap["storageUsedSlots"])
	_, e = Dispatch(tx, Command{Name: "receiveBatch", Actor: actor, Body: M{"productId": "101", "storagePointId": "11", "quantity": 1000}})
	p, ok := e.(*Problem)
	if !ok || p.Code != "capacity_exceeded" {
		t.Fatalf("capacity failure: %v", e)
	}
	cap, e = PointCapacity(tx, "11")
	if e != nil || I(cap["storageUsedSlots"]) != before {
		t.Fatal("failed receipt changed occupancy")
	}
	reservation := M{"allocations": []M{{"orderItemId": "item", "batchId": "44", "quantity": 1, "shipmentId": "shipment"}}, "shipments": []M{{"shipmentId": "shipment", "fromNodeId": "11", "pickupPointId": "11", "slots": 1}}}
	if _, e = Dispatch(tx, Command{Name: "svcReserveOrderStock", Params: map[string]string{"orderId": "order"}, Body: reservation, Actor: actor}); e != nil {
		t.Fatal(e)
	}
	for _, typ := range []string{"pack", "issue"} {
		_, e = Dispatch(tx, Command{Name: "svcApplyInventoryOrderEvent", Params: map[string]string{"orderId": "order"}, Body: M{"type": typ, "shipmentId": "shipment", "occurredAt": Stamp(tx.Now())}, Actor: actor})
		if e != nil {
			t.Fatal(e)
		}
		b, e := tx.Get("batches", "44")
		if e != nil {
			t.Fatal(e)
		}
		if I(b["availableQuantity"]) != 9 {
			t.Fatal("unexpected sale stock")
		}
		want := int64(10)
		if typ == "issue" {
			want = 9
		}
		if I(b["usedSlots"]) != want {
			t.Fatalf("%s physical slots=%v", typ, b["usedSlots"])
		}
	}
	result, e := Dispatch(tx, Command{Name: "svcApplyInventoryOrderEvent", Params: map[string]string{"orderId": "order"}, Body: M{"type": "issue", "shipmentId": "shipment", "occurredAt": Stamp(tx.Now())}, Actor: actor})
	if e != nil || B(result["applied"]) {
		t.Fatalf("repeat must be no-op %v", e)
	}
}
func TestInventoryLastUnitConcurrentAndRunIsolation(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	other := inventoryTestRun(t, s)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := s.Begin(context.Background(), run)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback()
			order := ID()
			_, e = Dispatch(tx, Command{Name: "svcReserveOrderStock", Params: map[string]string{"orderId": order}, Body: M{"allocations": []M{{"orderItemId": ID(), "batchId": "46", "quantity": 1, "shipmentId": order}}, "shipments": []M{{"shipmentId": order, "fromNodeId": "11", "pickupPointId": "13", "slots": 1}}}})
			if e == nil {
				e = tx.Commit()
			}
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		} else if p, ok := e.(*Problem); !ok || p.Code != "reservation_conflict" {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
	for id, want := range map[string]int64{run: 0, other: 1} {
		tx, e := s.Begin(context.Background(), id)
		if e != nil {
			t.Fatal(e)
		}
		b, e := tx.Get("batches", "46")
		tx.Rollback()
		if e != nil || I(b["availableQuantity"]) != want {
			t.Fatalf("run isolation %s: %v %v", id, b, e)
		}
	}
}
func TestInventoryScopeAndContextVersion(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	_, e = Dispatch(tx, Command{Name: "getBatch", Params: map[string]string{"batchId": "41"}, Actor: Actor{Subject: "wrong", Roles: []string{"pvz_operator"}, PointIDs: []string{"14"}}})
	if p, ok := e.(*Problem); !ok || p.Status != 403 {
		t.Fatalf("foreign batch visible: %v", e)
	}
	_, e = Dispatch(tx, Command{Name: "createDemoRun", Actor: Actor{Subject: S(tx.Run["subject"]), Roles: []string{"demo_admin"}}, Body: M{"datasetId": "backend_demo", "expectedContextVersion": 0}})
	if p, ok := e.(*Problem); !ok || p.Code != "stale_context" {
		t.Fatalf("stale context accepted: %v", e)
	}
}
func TestInventoryCannotArriveOrIssueBeforePhysicalTransfer(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	cmd := Command{Name: "svcReserveOrderStock", Params: map[string]string{"orderId": "early-order"}, Body: M{"allocations": []M{{"orderItemId": "early-item", "batchId": "41", "quantity": 1, "shipmentId": "early-shipment"}}, "shipments": []M{{"shipmentId": "early-shipment", "fromNodeId": "11", "pickupPointId": "13", "slots": 1}}}}
	if _, e = Dispatch(tx, cmd); e != nil {
		t.Fatal(e)
	}
	for _, typ := range []string{"arrive", "issue"} {
		_, e = Dispatch(tx, Command{Name: "svcApplyInventoryOrderEvent", Params: cmd.Params, Body: M{"type": typ, "shipmentId": "early-shipment", "nodeId": "13", "occurredAt": Stamp(tx.Now())}})
		if p, ok := e.(*Problem); !ok || p.Code != "invalid_transition" {
			t.Fatalf("premature %s accepted: %v", typ, e)
		}
	}
	b, e := tx.Get("batches", "41")
	if e != nil || I(b["availableQuantity"]) != 4 || I(b["reservedQuantity"]) != 1 || I(b["usedSlots"]) != 5 {
		t.Fatalf("failed event changed stock: %v %v", b, e)
	}
}

func TestInventoryRejectsSlotAndReservationOverflow(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	p, e := tx.Get("products", "101")
	if e != nil {
		t.Fatal(e)
	}
	p["slotsPerUnit"] = 2
	if e = tx.Put("products", p); e != nil {
		t.Fatal(e)
	}
	_, e = Dispatch(tx, Command{Name: "receiveBatch", Actor: Actor{Roles: []string{"dispatcher"}}, Body: M{"productId": "101", "storagePointId": "11", "quantity": int64(9223372036854775807)}})
	if problem, ok := e.(*Problem); !ok || problem.Status != 422 {
		t.Fatalf("slot overflow accepted: %v", e)
	}
	_, e = Dispatch(tx, Command{Name: "svcReserveOrderStock", Params: map[string]string{"orderId": "overflow"}, Body: M{"allocations": []M{{"orderItemId": "a", "batchId": "41", "shipmentId": "s", "quantity": int64(9223372036854775807)}, {"orderItemId": "b", "batchId": "41", "shipmentId": "s", "quantity": 1}}, "shipments": []M{{"shipmentId": "s", "fromNodeId": "11", "pickupPointId": "13", "slots": 1}}}})
	if problem, ok := e.(*Problem); !ok || problem.Status != 422 {
		t.Fatalf("reservation overflow accepted: %v", e)
	}
	b, e := tx.Get("batches", "41")
	if e != nil || I(b["availableQuantity"]) != 5 || I(b["reservedQuantity"]) != 0 {
		t.Fatalf("overflow changed stock: %v %v", b, e)
	}
}
