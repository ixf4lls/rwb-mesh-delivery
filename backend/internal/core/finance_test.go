package core

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func financeFixture(t *testing.T) (*Store, *Tx) {
	t.Helper()
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(tx.Rollback)
	return s, tx
}
func mustFinancePut(t *testing.T, tx *Tx, kind string, m M) {
	t.Helper()
	if e := tx.Put(kind, m); e != nil {
		t.Fatal(e)
	}
}
func TestFinanceSettlementPartialReturnAndScope(t *testing.T) {
	_, tx := financeFixture(t)
	order := M{"id": ID(), "status": "issued", "version": 1, "pickupPointId": "11", "buyerFeeMinor": 1200, "tariffVersionId": "51", "settlementStatus": "posted", "lines": []M{{"id": "line1", "productId": "101", "sellerId": "2", "quantity": 2, "unitPriceMinor": 59000, "unitCostMinor": 26000}}}
	mustFinancePut(t, tx, "orders", order)
	if e := SettleOrder(tx, order, "issued_event"); e != nil {
		t.Fatal(e)
	}
	if e := SettleOrder(tx, order, "issued_event"); e != nil {
		t.Fatal(e)
	}
	entries, e := tx.List("ledger-entries", M{"orderId": order["id"], "partyType": "seller"})
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 3 {
		t.Fatalf("duplicate/missing original entries %d", len(entries))
	}
	body := M{"kind": "return_inspected", "occurredAt": Stamp(tx.Now()), "tariffVersionId": "51", "orderId": order["id"], "returnId": "return1", "lines": []M{{"orderItemId": "line1", "resellableQuantity": 1, "quarantinedQuantity": 0}}, "operatingFacts": []M{}}
	p, e := ApplyPosting(tx, "return_event", body)
	if e != nil {
		t.Fatal(e)
	}
	if len(Arr(p["entries"])) != 4 {
		t.Fatalf("expected seller sale,cogs,commission + marketplace commission reversal: %#v", p)
	}
	if _, e = ApplyPosting(tx, "return_event", body); e != nil {
		t.Fatal(e)
	}
	changed := Clone(body)
	changed["returnId"] = "different"
	if _, e = ApplyPosting(tx, "return_event", changed); e == nil {
		t.Fatal("event fact mismatch accepted")
	}
	econ, e := financeRead(tx, Command{Name: "getSellerOrderEconomics", Actor: Actor{Roles: []string{"seller"}, SellerIDs: []string{"2"}}, Params: map[string]string{"sellerId": "2", "orderId": S(order["id"])}, Query: url.Values{}})
	if e != nil {
		t.Fatal(e)
	}
	if I(econ["salesMinor"]) != 59000 || I(econ["goodsCostMinor"]) != 26000 || I(econ["deductionsMinor"]) != 5900 {
		t.Fatalf("return net economics mismatch %#v", econ)
	}
	if I(econ["profitMinor"]) != 27100 {
		t.Fatalf("partial return profit %v", econ["profitMinor"])
	}
	if _, e = financeRead(tx, Command{Name: "getSellerEconomics", Actor: Actor{Roles: []string{"seller"}, SellerIDs: []string{"other"}}, Params: map[string]string{"sellerId": "2"}, Query: url.Values{}}); e == nil {
		t.Fatal("cross-seller economics exposed")
	}
	page, e := financeRead(tx, Command{Name: "listLedgerEntries", Actor: Actor{Roles: []string{"seller"}, SellerIDs: []string{"other"}}, Query: url.Values{}})
	if e != nil || len(Arr(page["items"])) != 0 {
		t.Fatal("ledger scope leak")
	}
}
func TestFinanceStorageFullPeriodsAndPhysicalSlotChanges(t *testing.T) {
	_, tx := financeFixture(t)
	b, e := tx.Get("batches", "45")
	if e != nil {
		t.Fatal(e)
	}
	start := tx.Now()
	tx.Clock = start.Add(23 * time.Hour)
	cost, e := StorageCostForBatch(tx, b, tx.Now())
	if e != nil || I(cost["accruedMinor"]) != 0 {
		t.Fatalf("partial day billed: %v %v", cost, e)
	}
	tx.Clock = start.Add(36 * time.Hour)
	if e = AdjustBatch(tx, b, -2, 0, 0, 0); e != nil {
		t.Fatal(e)
	}
	b, e = tx.Get("batches", "45")
	if e != nil {
		t.Fatal(e)
	}
	tx.Clock = start.Add(10 * 24 * time.Hour)
	cost, e = StorageCostForBatch(tx, b, tx.Now())
	if e != nil || I(cost["billableSlotDays"]) != 32 || I(cost["accruedMinor"]) != 6400 {
		t.Fatalf("slot duration accounting %v %v", cost, e)
	}
	if e = AccrueStorage(tx); e != nil {
		t.Fatal(e)
	}
	before, e := tx.List("ledger-entries", M{"batchId": "45"})
	if e != nil {
		t.Fatal(e)
	}
	if e = AccrueStorage(tx); e != nil {
		t.Fatal(e)
	}
	after, e := tx.List("ledger-entries", M{"batchId": "45"})
	if e != nil || len(after) != len(before) {
		t.Fatal("repeated storage accrual duplicated ledger")
	}
	cost, e = StorageCostForBatch(tx, b, tx.Now())
	if e != nil || I(cost["postedMinor"]) != 6400 {
		t.Fatalf("posted storage mismatch %v %v", cost, e)
	}
}

func TestFinanceTransportAllocationAndSharedAdditionalStop(t *testing.T) {
	_, tx := financeFixture(t)
	for n := 0; n < 2; n++ {
		order := M{"id": ID(), "status": "issued", "version": 1, "pickupPointId": "13", "buyerFeeMinor": 1200, "tariffVersionId": "51", "lines": []M{{"id": ID(), "productId": "101", "sellerId": "2", "quantity": 1, "unitPriceMinor": 59000, "unitCostMinor": 26000}}, "plannedShipments": []M{{"id": ID(), "segments": []M{{"id": ID(), "type": "city_trip", "mode": "physical", "tripId": "31", "fromNodeId": "11", "toNodeId": "13", "scheduledStartAt": "2026-01-05T12:00:00Z", "estimatedCostMinor": 900, "wholeTripCostMinor": 10000, "allocatedCostMinor": 100, "incrementalCostMinor": 800, "extraStopCostMinor": 800, "stopChargeKey": "31_2026-01-05T12:00:00Z_13", "incrementalStops": 1, "costItems": []M{{"category": "city_trip", "amountMinor": 100}, {"category": "extra_stop", "amountMinor": 800}}}}}}}
		mustFinancePut(t, tx, "orders", order)
		if e := SettleOrder(tx, order, "transport_"+S(order["id"])); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := tx.List("ledger-entries", M{"partyType": "marketplace", "category": "logistics_cost"})
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3 {
		t.Fatalf("want 2 allocated shares + shared stop, got %d: %#v", len(rows), rows)
	}
	var total int64
	for _, r := range rows {
		total += I(r["amountMinor"])
	}
	if total != -1000 {
		t.Fatalf("trip allocated or shared stop misbooked %d", total)
	}
	stops, e := tx.List("billed-stops", nil)
	if e != nil || len(stops) != 1 {
		t.Fatal("additional stop charged per order instead of once")
	}
	hasWhole := false
	for _, r := range rows {
		if I(r["wholeTripCostMinor"]) == 10000 && I(r["allocatedCostMinor"]) == 100 {
			hasWhole = true
		}
	}
	if !hasWhole {
		t.Fatal("whole trip and allocated expense trace lost")
	}
	// A pre-metadata snapshot uses CostItem.category, preserving its expense.
	facts, e := transportFacts([]M{{"id": "old-segment", "type": "city_trip", "mode": "physical", "costItems": []M{{"category": "city_trip", "amountMinor": 321}}}}, M{})
	if e != nil || len(facts) != 1 || I(facts[0]["allocatedCostMinor"]) != 321 || S(facts[0]["code"]) != "city_trip" {
		t.Fatalf("CostItem conversion lost expense %#v %v", facts, e)
	}
}
func TestFinanceInt64Boundaries(t *testing.T) {
	_, tx := financeFixture(t)
	const limit = int64(9223372036854775807)
	body := M{"kind": "order_issued", "occurredAt": Stamp(tx.Now()), "tariffVersionId": "51", "orderId": "large", "buyerFeeMinor": 0, "saleLines": []M{{"orderItemId": "large-line", "sellerId": "2", "quantity": 2, "unitPriceMinor": limit, "unitCostMinor": 0}}, "operatingFacts": []M{}}
	if _, e := ApplyPosting(tx, "overflow", body); e == nil {
		t.Fatal("overflowing sale accepted")
	}
	body["saleLines"] = []M{{"orderItemId": "large-line", "sellerId": "2", "quantity": 1, "unitPriceMinor": limit, "unitCostMinor": 0}}
	posting, e := ApplyPosting(tx, "representable", body)
	if e != nil {
		t.Fatal(e)
	}
	var sale, commission int64
	for _, l := range Arr(posting["entries"]) {
		if S(l["partyType"]) == "seller" {
			if S(l["category"]) == "sale" {
				sale = I(l["amountMinor"])
			}
			if S(l["category"]) == "commission" {
				commission = I(l["amountMinor"])
			}
		}
	}
	if sale != limit || commission != -922337203685477580 {
		t.Fatalf("valid int64 lost precision sale=%d commission=%d", sale, commission)
	}
	b := M{"id": "huge-batch", "receivedAt": Stamp(tx.Now().Add(-48 * time.Hour)), "usedSlots": limit, "storageRateMinorPerSlotDay": 200}
	if _, e = StorageCostForBatch(tx, b, tx.Now()); e == nil {
		t.Fatal("overflowing slot-days accepted")
	}
	mustFinancePut(t, tx, "ledger-entries", M{"id": ID(), "partyType": "seller", "partyId": "2", "category": "sale", "amountMinor": limit, "bookedAt": Stamp(tx.Now())})
	if _, e = financeRead(tx, Command{Name: "getSellerEconomics", Actor: Actor{Roles: []string{"dispatcher"}}, Params: map[string]string{"sellerId": "2"}, Query: url.Values{}}); e == nil {
		t.Fatal("overflowing financial aggregate accepted")
	}
}

func TestFinanceReturnCumulativeRoundingAndZeroPriceQuantity(t *testing.T) {
	_, tx := financeFixture(t)
	for _, price := range []int64{5, 0} {
		order := M{"id": ID(), "status": "issued", "pickupPointId": "11", "tariffVersionId": "51", "buyerFeeMinor": 0, "lines": []M{{"id": "fraction-line", "sellerId": "2", "productId": "101", "quantity": 3, "unitPriceMinor": price, "unitCostMinor": 2}}}
		mustFinancePut(t, tx, "orders", order)
		if e := SettleOrder(tx, order, "fraction_issue_"+S(order["id"])); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 3; i++ {
			_, e := ApplyPosting(tx, ID(), M{"kind": "return_inspected", "occurredAt": Stamp(tx.Now()), "tariffVersionId": "51", "orderId": order["id"], "returnId": ID(), "lines": []M{{"orderItemId": "fraction-line", "resellableQuantity": 1, "quarantinedQuantity": 0}}, "operatingFacts": []M{}})
			if e != nil {
				t.Fatal(e)
			}
		}
		_, e := ApplyPosting(tx, ID(), M{"kind": "return_inspected", "occurredAt": Stamp(tx.Now()), "tariffVersionId": "51", "orderId": order["id"], "returnId": ID(), "lines": []M{{"orderItemId": "fraction-line", "resellableQuantity": 1, "quarantinedQuantity": 0}}, "operatingFacts": []M{}})
		if e == nil {
			t.Fatal("cumulative quantity exceeded issued amount")
		}
		rows, e := tx.List("ledger-entries", M{"orderId": order["id"], "partyType": "seller"})
		if e != nil {
			t.Fatal(e)
		}
		var sum int64
		for _, r := range rows {
			sum += I(r["amountMinor"])
		}
		if sum != 0 {
			t.Fatalf("full return left rounding balance %d", sum)
		}
	}
}
