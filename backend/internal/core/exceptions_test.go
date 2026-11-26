package core

import (
	"testing"
	"time"
)

func TestExceptionsReturnInspectionAndQuantityBounds(t *testing.T) {
	_, tx := financeFixture(t)
	actor := Actor{Subject: "operator", Roles: []string{"dispatcher"}}
	order := M{"id": ID(), "status": "issued", "version": 1, "pickupPointId": "11", "buyerFeeMinor": 0, "tariffVersionId": "51", "settlementStatus": "posted", "lines": []M{{"id": "line1", "productId": "101", "sellerId": "2", "quantity": 2, "unitPriceMinor": 59000, "unitCostMinor": 26000}}}
	mustFinancePut(t, tx, "orders", order)
	if e := SettleOrder(tx, order, "issued"); e != nil {
		t.Fatal(e)
	}
	c := Command{Name: "createReturn", Actor: actor, Body: M{"orderId": order["id"], "returnPointId": "11", "reason": "synthetic damaged item", "lines": []M{{"orderItemId": "line1", "quantity": 1}}}}
	r, e := createReturn(tx, c)
	if e != nil {
		t.Fatal(e)
	}
	c.Body["lines"] = []M{{"orderItemId": "line1", "quantity": 2}}
	if _, e = createReturn(tx, c); e == nil {
		t.Fatal("open return quantities ignored")
	}
	ec := Command{Actor: actor, Params: map[string]string{"returnId": S(r["id"])}, Body: M{"type": "received_from_customer", "expectedVersion": 1}}
	r, e = returnEvent(tx, ec)
	if e != nil {
		t.Fatal(e)
	}
	if S(r["status"]) != "awaiting_inspection" {
		t.Fatalf("same point return requires no transport %v", r)
	}
	ec.Body = M{"type": "inspected", "expectedVersion": r["version"], "lines": []M{{"orderItemId": "line1", "resellableQuantity": 0, "quarantinedQuantity": 1}}}
	r, e = returnEvent(tx, ec)
	if e != nil {
		t.Fatal(e)
	}
	if S(r["status"]) != "closed" || S(r["settlementStatus"]) != "posted" {
		t.Fatal("return did not settle")
	}
	stock, e := tx.List("batches", M{"returnId": r["id"]})
	if e != nil || len(stock) != 1 || I(stock[0]["availableQuantity"]) != 0 || I(stock[0]["quarantinedQuantity"]) != 1 {
		t.Fatalf("damaged return leaked to salable stock %v %v", stock, e)
	}
	ec.Body["expectedVersion"] = r["version"]
	if _, e = returnEvent(tx, ec); e == nil {
		t.Fatal("closed return inspected twice")
	}
	ec.Actor = Actor{Roles: []string{"pvz_operator"}, PointIDs: []string{"14"}}
	ec.Body = M{"type": "canceled", "expectedVersion": r["version"]}
	if _, e = returnEvent(tx, ec); e == nil {
		t.Fatal("cross-point return access allowed")
	}
}
func TestExceptionsLongStorageThresholdDecisionAndTransfer(t *testing.T) {
	_, tx := financeFixture(t)
	start := tx.Now()
	tx.Clock = start.Add(10 * 24 * time.Hour)
	result, e := scanLongStorage(tx, tx.Now())
	if e != nil {
		t.Fatal(e)
	}
	if I(result["changedCases"]) == 0 {
		t.Fatal("threshold not detected")
	}
	m, e := tx.Get("storage-cases", "storage_45")
	if e != nil {
		t.Fatal(e)
	}
	if I(Obj(m["cost"])["billableSlotDays"]) != 50 || I(Obj(m["cost"])["accruedMinor"]) != 10000 {
		t.Fatalf("threshold cost mismatch %v", m)
	}
	notices, e := tx.List("notifications", M{"resourceId": m["id"]})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = scanLongStorage(tx, tx.Now()); e != nil {
		t.Fatal(e)
	}
	again, e := tx.List("notifications", M{"resourceId": m["id"]})
	if e != nil || len(again) != len(notices) {
		t.Fatal("threshold duplicated notification")
	}
	actor := Actor{Roles: []string{"dispatcher"}}
	c := Command{Actor: actor, Params: map[string]string{"caseId": S(m["id"])}, Body: M{"action": "keep", "reason": "synthetic demand review", "expectedVersion": m["version"], "recheckAt": Stamp(tx.Now().Add(24 * time.Hour))}}
	m, e = decideStorageCase(tx, c)
	if e != nil {
		t.Fatal(e)
	}
	if S(m["status"]) != "deferred" {
		t.Fatal("keep did not defer")
	}
	tx.Clock = tx.Now().Add(24 * time.Hour)
	if _, e = scanLongStorage(tx, tx.Now()); e != nil {
		t.Fatal(e)
	}
	m, e = tx.Get("storage-cases", S(m["id"]))
	if e != nil || S(m["status"]) != "open" {
		t.Fatal("deferred case not reopened")
	}
	c.Body = M{"action": "transfer_to_hub", "reason": "remove slow stock", "expectedVersion": m["version"]}
	m, e = decideStorageCase(tx, c)
	if e != nil {
		t.Fatal(e)
	}
	b, e := tx.Get("batches", "45")
	if e != nil || I(b["availableQuantity"]) != 0 || I(b["reservedQuantity"]) != 5 || I(b["usedSlots"]) != 5 {
		t.Fatalf("transfer reservation changed physical presence %v %v", b, e)
	}
	for _, seg := range movementSegments(m) {
		tx.Clock = Time(seg["scheduledStartAt"])
		c.Body = M{"type": "handed_over", "segmentId": seg["id"], "expectedVersion": m["version"]}
		if S(seg["mode"]) == "simulated" {
			c.SimulationID = "synthetic_action"
		}
		m, e = storageTransferEvent(tx, c)
		if e != nil {
			t.Fatal(e)
		}
		b, e = tx.Get("batches", "45")
		if e != nil || I(b["usedSlots"]) != 0 {
			t.Fatal("handover did not free physical slots")
		}
		tx.Clock = Time(seg["scheduledEndAt"])
		c.Body = M{"type": "arrived", "segmentId": seg["id"], "expectedVersion": m["version"]}
		m, e = storageTransferEvent(tx, c)
		if e != nil {
			t.Fatal(e)
		}
	}
	if S(m["status"]) != "closed" {
		t.Fatal("transfer did not close")
	}
	stock, e := tx.List("batches", M{"sourceBatchId": "45"})
	if e != nil || len(stock) != 1 || I(stock[0]["availableQuantity"]) != 5 {
		t.Fatal("hub transfer stock missing")
	}
}

func TestExceptionsSimulationIdentityAndScheduledTime(t *testing.T) {
	_, tx := financeFixture(t)
	eventTime := tx.Now().Add(time.Hour)
	segment := M{"id": "sim-segment", "type": "linehaul", "mode": "simulated", "fromNodeId": "11", "toNodeId": "21", "scheduledStartAt": Stamp(eventTime), "scheduledEndAt": Stamp(eventTime.Add(time.Hour))}
	r := M{"id": "sim-return", "orderId": "sim-order", "returnPointId": "11", "inspectionNodeId": "21", "status": "at_return_point", "version": 1, "movementIndex": 0, "movementActive": false, "selectedOptionId": "option", "route": M{"options": []M{{"id": "option", "shipments": []M{{"segments": []M{segment}}}}}}}
	mustFinancePut(t, tx, "returns", r)
	target := tx.Now().Add(5 * time.Hour)
	tx.Clock = target
	c := Command{Actor: Actor{Service: "demo", Roles: []string{"dispatcher"}}, Params: map[string]string{"returnId": "sim-return"}, Body: M{"type": "handed_over", "segmentId": "sim-segment", "expectedVersion": 1}, SimulationID: "forged"}
	if _, e := returnEvent(tx, c); e == nil {
		t.Fatal("arbitrary simulation identity accepted")
	}
	c.SimulationID = "sim-return_sim-segment_handed_over"
	c.Actor.Service = "orders"
	if _, e := returnEvent(tx, c); e == nil {
		t.Fatal("wrong service executed demo action")
	}
	c.Actor.Service = "demo"
	m, e := returnEvent(tx, c)
	if e != nil || S(m["status"]) != "in_transit" {
		t.Fatalf("valid action failed %v %v", m, e)
	}
	if !tx.Now().Equal(target) {
		t.Fatal("simulation altered target model clock")
	}
	c.Body = M{"type": "arrived", "segmentId": "sim-segment", "expectedVersion": 2}
	c.SimulationID = "sim-return_sim-segment_arrived"
	m, e = returnEvent(tx, c)
	if e != nil || S(m["status"]) != "awaiting_inspection" {
		t.Fatalf("arrival failed %v %v", m, e)
	}
	if !tx.Now().Equal(target) {
		t.Fatal("arrival changed model target")
	}
}
