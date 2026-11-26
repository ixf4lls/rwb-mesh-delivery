package core

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestImportLargeSlice generates input at run time; no large dataset is committed.
// Opt in with MESH_RUN_SCALE=1 and the isolated MESH_TEST_DATABASE_URL.
func TestImportLargeSlice(t *testing.T) {
	if os.Getenv("MESH_RUN_SCALE") != "1" {
		t.Skip("set MESH_RUN_SCALE=1 for reproducible scale measurement")
	}
	s := inventoryTestStore(t)
	dir := t.TempDir()
	files := []M{}
	var inputBytes int64
	addFile := func(name string, rows int64) {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(b)
		files = append(files, M{"path": name, "rows": rows, "sha256": hex.EncodeToString(sum[:])})
		inputBytes += int64(len(b))
	}
	raw, e := os.ReadFile("../../testdata/demo/initial_state.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "initial_state.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	addFile("initial_state.json", 1)
	csvFile := func(name string, header []string, n int, record func(int) []string) {
		f, e := os.Create(filepath.Join(dir, name))
		if e != nil {
			t.Fatal(e)
		}
		w := csv.NewWriter(f)
		if e = w.Write(header); e != nil {
			t.Fatal(e)
		}
		for i := 0; i < n; i++ {
			if e = w.Write(record(i)); e != nil {
				t.Fatal(e)
			}
		}
		w.Flush()
		if e = w.Error(); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		addFile(name, int64(n))
	}
	csvFile("points.csv", []string{"pickup_point_id", "node_id", "city_id", "point_name", "storage_enabled", "active", "accepts_orders", "accepts_returns", "opening_hours_json", "capacity_json"}, 146, func(i int) []string {
		id := fmt.Sprint(1000 + i)
		return []string{id, id, "1", "Scale point " + id, "true", "true", "true", "true", `[{"weekdays":[1,2,3,4,5,6,7],"opens":"00:00","closes":"23:59"}]`, `{"totalSlots":1500,"storageQuotaSlots":1000,"ordinaryUsedSlots":0,"pickupUsedSlots":0,"inboundReservedSlots":0}`}
	})
	csvFile("seller_skus.csv", []string{"seller_sku", "seller_id", "sku_value"}, 19994, func(i int) []string { return []string{fmt.Sprint(800000 + i), "2", fmt.Sprintf("scale-%d", i)} })
	csvFile("products.csv", []string{"product_id", "seller_id", "seller_sku", "product_name", "weight_grams", "slots_per_unit", "sale_price_minor", "unit_cost_minor", "width_m", "height_m", "depth_m"}, 19994, func(i int) []string {
		return []string{fmt.Sprint(500000 + i), "2", fmt.Sprint(800000 + i), fmt.Sprintf("Scale product %d", i), "100", "1", "59000", "26000", "0.1", "0.2", "0.3"}
	})
	csvFile("batches.csv", []string{"batch_id", "seller_id", "product_id", "storage_point_id", "received_at", "received_quantity", "available_quantity", "reserved_quantity", "packed_quantity", "quarantined_quantity", "used_slots", "tariff_version_id", "storage_rate_minor_per_slot_day"}, 19993, func(i int) []string {
		return []string{fmt.Sprint(600000 + i), "2", fmt.Sprint(500000 + i), fmt.Sprint(1000 + i%146), "2026-01-05T09:00:00Z", "1", "1", "0", "0", "0", "1", "51", "200"}
	})
	id := "scale-" + ID()
	manifest := M{"id": id, "schemaVersion": "2.0.0", "initialClock": "2026-01-05T09:00:00Z", "seed": 42, "counts": M{"pointCount": 150, "orderCount": 0, "orderIntentCount": 0}, "files": files}
	raw, e = json.Marshal(manifest)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	inputBytes += int64(len(raw))
	start := time.Now()
	dataset, e := ImportDataset(context.Background(), s, dir)
	importElapsed := time.Since(start)
	if e != nil {
		t.Fatal(e)
	}
	actor := Actor{Subject: "scale-" + ID(), Roles: []string{"dispatcher", "demo_admin"}}
	start = time.Now()
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	r, e := Dispatch(tx, Command{Name: "createDemoRun", Actor: actor, Body: M{"datasetId": dataset["id"], "expectedContextVersion": 0}})
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	runElapsed := time.Since(start)
	tx, e = s.Begin(context.Background(), S(r["id"]))
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	var copied int
	if e = tx.SQL.QueryRow(tx.Context, "SELECT count(*) FROM mesh_records WHERE scope=$1", tx.Scope).Scan(&copied); e != nil {
		t.Fatal(e)
	}
	if copied != 0 {
		t.Fatalf("new run copied %d baseline records", copied)
	}
	for kind, want := range map[string]int{"products": 20000, "batches": 20000, "pickup-points": 150} {
		var n int
		if e = tx.SQL.QueryRow(tx.Context, "SELECT count(*) FROM mesh_records WHERE scope=$1 AND kind=$2", tx.Dataset, kind).Scan(&n); e != nil || n != want {
			t.Fatalf("%s count=%d want=%d error=%v", kind, n, want, e)
		}
	}
	start = time.Now()
	q, e := Dispatch(tx, Command{Name: "createQuote", RunID: S(r["id"]), Actor: actor, Body: M{"pickupPointId": "1000", "items": []M{{"productId": "500000", "quantity": 1}}}})
	quoteElapsed := time.Since(start)
	if e != nil {
		t.Fatal(e)
	}
	if S(q["recommendedOptionId"]) == "" {
		t.Fatalf("local scale quote infeasible: %v", q)
	}
	t.Logf("scale actual points=150 products=20000 batches=20000 bytes=%d import=%s create_run=%s quote=%s copied_baseline_records=%d postgres=17 local Docker synthetic fixed-clock", inputBytes, importElapsed, runElapsed, quoteElapsed, copied)
}
