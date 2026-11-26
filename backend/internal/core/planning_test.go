package core

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func planningFixture(t *testing.T) (*Store, string) {
	t.Helper()
	s, e := TestStore()
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../../testdata/demo/initial_state.json")
	if e != nil {
		t.Fatal(e)
	}
	var groups map[string][]M
	if e = json.Unmarshal(raw, &groups); e != nil {
		t.Fatal(e)
	}
	scope := "dataset:testplanning:" + ID()
	runID := ID()
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	tx.Scope = scope
	for kind, rows := range groups {
		for _, row := range rows {
			if e = tx.Put(kind, row); e != nil {
				tx.Rollback()
				t.Fatal(e)
			}
		}
	}
	if e = tx.GlobalPut("runs", M{"id": runID, "datasetScope": scope, "clock": "2026-01-05T09:00:00Z"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = s.Pool.Exec(context.Background(), "DELETE FROM mesh_records WHERE scope=$1 OR scope=$2 OR (scope='global' AND kind='runs' AND id=$3)", scope, "run:"+runID, runID)
		s.Close()
	})
	return s, runID
}
func planningTx(t *testing.T, s *Store, id string) *Tx {
	t.Helper()
	tx, e := s.Begin(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(tx.Rollback)
	return tx
}
func dispatcher() Actor { return Actor{Subject: "test", Roles: []string{"dispatcher"}} }
func testQuote(t *testing.T, tx *Tx, pickup string, items []M) M {
	t.Helper()
	q, e := CreateQuote(tx, Command{Actor: dispatcher(), Body: M{"pickupPointId": pickup, "items": items}})
	if e != nil {
		t.Fatal(e)
	}
	return q
}
func policyOption(t *testing.T, q M, policy string) M {
	t.Helper()
	for _, o := range Arr(q["options"]) {
		if S(o["policy"]) == policy {
			return o
		}
	}
	t.Fatalf("missing %s option in %#v", policy, q)
	return nil
}
func testOrder(t *testing.T, tx *Tx, q M, policy string) M {
	t.Helper()
	opt := policyOption(t, q, policy)
	o, e := CreateOrder(tx, Command{Actor: dispatcher(), Body: M{"quoteId": q["id"], "optionId": opt["id"]}})
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func testOrderEvent(t *testing.T, tx *Tx, order M, typ, shipment, segment string) M {
	t.Helper()
	body := M{"type": typ, "expectedVersion": order["version"]}
	if shipment != "" {
		body["shipmentId"] = shipment
	}
	if segment != "" {
		body["segmentId"] = segment
	}
	out, e := ApplyOrderEvent(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(order["id"])}, Body: body})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestPlanningBasketPoliciesAndUnavailable(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	mesh := policyOption(t, q, "mesh_only")
	if !B(mesh["feasible"]) || S(mesh["mode"]) != "city_mesh" || I(mesh["buyerFeeMinor"]) != 1200 || I(mesh["etaMinutes"]) != 360 {
		t.Fatalf("mesh %#v", mesh)
	}
	local := testQuote(t, tx, "11", []M{{"productId": "104", "quantity": 1}})
	o := policyOption(t, local, "mesh_only")
	if S(o["mode"]) != "local_shelf" || I(o["etaMinutes"]) != 0 || I(o["buyerFeeMinor"]) != 0 {
		t.Fatalf("local %#v", o)
	}
	mixed := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}, {"productId": "102", "quantity": 1}})
	hybrid := policyOption(t, mixed, "hybrid")
	if !B(hybrid["feasible"]) || S(hybrid["mode"]) != "mixed" || len(Arr(hybrid["shipments"])) != 2 {
		t.Fatalf("mixed %#v", hybrid)
	}
	unavailable := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 10000}})
	if S(unavailable["recommendedOptionId"]) != "" {
		t.Fatal("unavailable basket recommended")
	}
	for _, o := range Arr(unavailable["options"]) {
		if B(o["feasible"]) {
			t.Fatal("unavailable feasible")
		}
	}
}
func TestPlanningRejectsBasketOversubscription(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	trip, e := tx.Get("trips", "31")
	if e != nil {
		t.Fatal(e)
	}
	trip["capacitySlots"] = 1
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	trip, e = tx.Get("trips", "32")
	if e != nil {
		t.Fatal(e)
	}
	trip["capacitySlots"] = 1
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}, {"productId": "103", "quantity": 1}})
	opt := policyOption(t, q, "mesh_only")
	if B(opt["feasible"]) || S(Arr(opt["reasons"])[0]["code"]) != "trip_capacity" {
		t.Fatalf("oversubscribed basket %#v", opt)
	}
}
func TestPlanningMovementAndTripHold(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	m, e := QuoteMovement(tx, M{"kind": "return", "subjectId": "r1", "fromNodeId": "13", "toNodeId": "11", "items": []M{{"productId": "101", "quantity": 1}}, "notBefore": Stamp(tx.Now()), "tariffVersionId": "51"})
	if e != nil {
		t.Fatal(e)
	}
	if len(Arr(m["options"])) != 1 {
		t.Fatalf("movement %#v", m)
	}
	hold, e := HoldRoute(tx, M{"quoteId": m["id"], "optionId": Arr(m["options"])[0]["id"], "subjectId": "r1", "subjectType": "return"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if S(hold["status"]) != "held" {
		t.Fatal(hold)
	}
	hold, e = UpdateRouteHold(tx, S(hold["id"]), M{"type": "confirm"})
	if e != nil || S(hold["status"]) != "confirmed" {
		t.Fatal(hold, e)
	}
	seg := Arr(Arr(Arr(m["options"])[0]["shipments"])[0]["segments"])[0]
	hold, e = UpdateRouteHold(tx, S(hold["id"]), M{"type": "segment_completed", "segmentId": seg["id"]})
	if e != nil || S(hold["status"]) != "completed" {
		t.Fatal(hold, e)
	}
}
func TestOrdersConcurrentLastUnit(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q1 := testQuote(t, tx, "13", []M{{"productId": "106", "quantity": 1}})
	q2 := testQuote(t, tx, "13", []M{{"productId": "106", "quantity": 1}})
	if e := tx.Commit(); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, q := range []M{q1, q2} {
		wg.Add(1)
		go func(q M) {
			defer wg.Done()
			tr, e := s.Begin(context.Background(), id)
			if e != nil {
				errs <- e
				return
			}
			defer tr.Rollback()
			var opt M
			for _, o := range Arr(q["options"]) {
				if S(o["policy"]) == "mesh_only" {
					opt = o
				}
			}
			_, e = CreateOrder(tr, Command{Actor: dispatcher(), Body: M{"quoteId": q["id"], "optionId": opt["id"]}})
			if e == nil {
				e = tr.Commit()
			}
			errs <- e
		}(q)
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		} else if p, ok := e.(*Problem); !ok || p.Code != "reservation_conflict" {
			t.Fatalf("unexpected losing error %v", e)
		}
	}
	if wins != 1 {
		t.Fatalf("winner count %d", wins)
	}
	tr := planningTx(t, s, id)
	b, e := tr.Get("batches", "46")
	if e != nil {
		t.Fatal(e)
	}
	if I(b["availableQuantity"]) != 0 || I(b["reservedQuantity"]) != 1 {
		t.Fatal(b)
	}
}
func TestPlanningStopOrderAndCutoff(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	tariff, e := routingTariff(tx)
	if e != nil {
		t.Fatal(e)
	}
	trip := M{"id": "stop-test", "fromNodeId": "11", "toNodeId": "12", "departureAt": "2026-01-05T09:00:00Z", "frequency": "daily", "mode": "physical", "type": "city_trip", "capacitySlots": 10, "costMinor": 100, "stops": []M{{"nodeId": "11", "offsetMinutes": 0}, {"nodeId": "13", "offsetMinutes": 60}, {"nodeId": "12", "offsetMinutes": 120}}}
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	segments, ready, _, e := FindRoute(tx, "13", "12", Time("2026-01-05T09:30:00Z"), 1, tariff, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(segments) != 1 || !ready.Equal(Time("2026-01-05T11:00:00Z")) {
		t.Fatalf("stop route %v %v", segments, ready)
	}
	_, _, _, e = FindRoute(tx, "12", "13", Time("2026-01-05T09:30:00Z"), 1, tariff, M{"tripId": "37"})
	if e == nil {
		t.Fatal("reverse stop order unexpectedly allowed")
	}
	tx.Clock = tx.Now().Add(time.Minute)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	if !B(policyOption(t, q, "mesh_only")["feasible"]) {
		t.Fatal(q)
	}
}

func TestPlanningCandidatesRespectOverlayAndIgnoreOtherOrigins(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	trip, e := tx.Get("trips", "31")
	if e != nil {
		t.Fatal(e)
	}
	trip["fromNodeId"] = "21"
	trip["toNodeId"] = "22"
	trip["stops"] = []M{{"nodeId": "21", "offsetMinutes": 0}, {"nodeId": "22", "offsetMinutes": 60}}
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	rows, e := tx.TripsFrom("11")
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		if S(row["id"]) == "31" || S(row["id"]) == "33" {
			t.Fatalf("unrelated or shadowed candidate returned %#v", row)
		}
	}
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := policyOption(t, q, "mesh_only")
	seg := Arr(Arr(o["shipments"])[0]["segments"])[0]
	if S(seg["tripId"]) != "32" {
		t.Fatal(seg)
	}
}
func TestPlanningDailyCapacityAndLocalOpening(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := policyOption(t, q, "mesh_only")
	seg := Arr(Arr(o["shipments"])[0]["segments"])[0]
	if e := tx.Put("route-holds", M{"id": "full-first-occurrence", "status": "confirmed", "legs": []M{{"key": seg["_legKey"], "segmentId": seg["id"], "slots": 100}}}); e != nil {
		t.Fatal(e)
	}
	trip, e := tx.Get("trips", "32")
	if e != nil {
		t.Fatal(e)
	}
	trip["active"] = false
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	q = testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o = policyOption(t, q, "mesh_only")
	if !B(o["feasible"]) || S(o["readyAt"]) != "2026-01-06T15:00:00Z" {
		t.Fatalf("daily successor %#v", o)
	}
	p, e := tx.Get("pickup-points", "11")
	if e != nil {
		t.Fatal(e)
	}
	p["openingHours"] = []M{{"weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "opens": "14:00", "closes": "18:00"}}
	if e = tx.Put("pickup-points", p); e != nil {
		t.Fatal(e)
	}
	local := testQuote(t, tx, "11", []M{{"productId": "104", "quantity": 1}})
	localOption := policyOption(t, local, "mesh_only")
	if S(localOption["readyAt"]) != "2026-01-05T11:00:00Z" {
		t.Fatalf("local opening %#v", localOption)
	}
	order := testOrder(t, tx, local, "mesh_only")
	_, e = ApplyOrderEvent(tx, Command{Actor: dispatcher(), Params: map[string]string{"orderId": S(order["id"])}, Body: M{"type": "packed", "shipmentId": Arr(order["shipments"])[0]["id"], "expectedVersion": order["version"]}})
	if p, ok := e.(*Problem); !ok || p.Code != "node_unavailable" {
		t.Fatalf("closed point packing %v", e)
	}
}
func TestPlanningDailyTripRespectsDST(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	city, e := tx.Get("cities", "1")
	if e != nil {
		t.Fatal(e)
	}
	city["timeZone"] = "Europe/Berlin"
	if e = tx.Put("cities", city); e != nil {
		t.Fatal(e)
	}
	departure, e := tripTime(tx, M{"fromNodeId": "11", "departureAt": "2026-03-28T09:00:00+01:00", "frequency": "daily"}, Time("2026-03-29T06:30:00Z"))
	if e != nil {
		t.Fatal(e)
	}
	if Stamp(departure) != "2026-03-29T07:00:00Z" {
		t.Fatalf("daily local hour shifted across DST: %s", Stamp(departure))
	}
}

func TestPlanningRejectsUnrepresentableBasketTotals(t *testing.T) {
	for _, scenario := range []string{"line_price", "basket_price", "slots", "movement_slots", "transport_cost"} {
		t.Run(scenario, func(t *testing.T) {
			s, id := planningFixture(t)
			tx := planningTx(t, s, id)
			p, e := tx.Get("products", "101")
			if e != nil {
				t.Fatal(e)
			}
			items := []M{{"productId": "101", "quantity": int64(2)}}
			switch scenario {
			case "line_price":
				p["salePriceMinor"] = int64(9223372036854775807)
			case "basket_price":
				p["salePriceMinor"] = int64(4611686018427387904)
				other, e := tx.Get("products", "104")
				if e != nil {
					t.Fatal(e)
				}
				other["salePriceMinor"] = int64(4611686018427387904)
				if e = tx.Put("products", other); e != nil {
					t.Fatal(e)
				}
				items = []M{{"productId": "101", "quantity": int64(1)}, {"productId": "104", "quantity": int64(1)}}
			case "slots", "movement_slots":
				p["slotsPerUnit"] = int64(9223372036854775807)
			case "transport_cost":
				trip, e := tx.Get("trips", "31")
				if e != nil {
					t.Fatal(e)
				}
				trip["costMinor"] = int64(9223372036854775807)
				stops := Arr(trip["stops"])
				stops[1]["additionalStop"] = true
				trip["stops"] = stops
				if e = tx.Put("trips", trip); e != nil {
					t.Fatal(e)
				}
			}
			if e = tx.Put("products", p); e != nil {
				t.Fatal(e)
			}
			if scenario == "movement_slots" {
				_, e = QuoteMovement(tx, M{"kind": "return", "subjectId": "r", "fromNodeId": "13", "toNodeId": "11", "tariffVersionId": "51", "notBefore": Stamp(tx.Now()), "items": items})
			} else {
				_, e = CreateQuote(tx, Command{Actor: dispatcher(), Body: M{"pickupPointId": "13", "items": items}})
			}
			if p, ok := e.(*Problem); !ok || p.Status != 422 || p.Code != "validation_error" {
				t.Fatalf("unrepresentable %s accepted or misreported: %v", scenario, e)
			}
			quotes, e := tx.List("quotes", nil)
			if e != nil || len(quotes) != 0 {
				t.Fatal("failed quote persisted", quotes, e)
			}
		})
	}
}
func TestPlanningPreservesExactInt64PricesAndTransportAllocation(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	p, e := tx.Get("products", "101")
	if e != nil {
		t.Fatal(e)
	}
	precise := int64(9007199254740993)
	p["salePriceMinor"] = precise
	if e = tx.Put("products", p); e != nil {
		t.Fatal(e)
	}
	trip, e := tx.Get("trips", "31")
	if e != nil {
		t.Fatal(e)
	}
	trip["costMinor"] = int64(9223372036854775807)
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": int64(1)}})
	o := testOrder(t, tx, q, "mesh_only")
	if I(Arr(o["lines"])[0]["unitPriceMinor"]) != precise {
		t.Fatalf("price lost precision %#v", o["lines"])
	}
	seg := Arr(Arr(o["plannedShipments"])[0]["segments"])[0]
	if I(seg["wholeTripCostMinor"]) != int64(9223372036854775807) || I(seg["allocatedCostMinor"]) != int64(92233720368547758) || I(seg["incrementalCostMinor"]) != 0 {
		t.Fatalf("transport distinctions %#v", seg)
	}
	if I(o["estimatedCostMinor"]) != int64(92233720368548258) {
		t.Fatal(o)
	}
}
func TestPlanningExtraStopMarginalCostAndPublishedCharge(t *testing.T) {
	s, id := planningFixture(t)
	tx := planningTx(t, s, id)
	trip, e := tx.Get("trips", "31")
	if e != nil {
		t.Fatal(e)
	}
	stops := Arr(trip["stops"])
	stops[1]["additionalStop"] = true
	trip["stops"] = stops
	if e = tx.Put("trips", trip); e != nil {
		t.Fatal(e)
	}
	alternative, e := tx.Get("trips", "32")
	if e != nil {
		t.Fatal(e)
	}
	alternative["active"] = false
	if e = tx.Put("trips", alternative); e != nil {
		t.Fatal(e)
	}
	q := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	o := policyOption(t, q, "mesh_only")
	seg := Arr(Arr(o["shipments"])[0]["segments"])[0]
	if I(seg["wholeTripCostMinor"]) != 1200 || I(seg["allocatedCostMinor"]) != 12 || I(seg["incrementalCostMinor"]) != 800 || I(o["estimatedCostMinor"]) != 1312 || I(o["incrementalCostMinor"]) != 1300 {
		t.Fatal(o, seg)
	}
	key := S(seg["stopChargeKey"])
	if key == "" {
		t.Fatal("extra stop lacks stable charge key")
	}
	if e = tx.Put("billed-stops", M{"id": key, "amountMinor": 800}); e != nil {
		t.Fatal(e)
	}
	next := testQuote(t, tx, "13", []M{{"productId": "101", "quantity": 1}})
	opt := policyOption(t, next, "mesh_only")
	if I(opt["estimatedCostMinor"]) != 512 || I(opt["incrementalCostMinor"]) != 500 {
		t.Fatalf("published stop counted again %#v", opt)
	}
}
