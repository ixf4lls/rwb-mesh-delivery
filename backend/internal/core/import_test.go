package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func inventoryTestStore(t *testing.T) *Store {
	t.Helper()
	if os.Getenv("MESH_TEST_DATABASE_URL") == "" {
		t.Skip("isolated MESH_TEST_DATABASE_URL required")
	}
	s, e := TestStore()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s
}
func inventoryTestRun(t *testing.T, s *Store) string {
	t.Helper()
	_, e := ImportDataset(context.Background(), s, "../../testdata/demo")
	if e != nil {
		t.Fatal(e)
	}
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	r, e := Dispatch(tx, Command{Name: "createDemoRun", Actor: Actor{Subject: "test-" + ID(), Roles: []string{"dispatcher", "demo_admin"}}, Body: M{"datasetId": "backend_demo", "expectedContextVersion": 0}})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	return S(r["id"])
}
func TestImportImmutableSnapshotAndRejectCorruptFile(t *testing.T) {
	s := inventoryTestStore(t)
	ctx := context.Background()
	first, e := ImportDataset(ctx, s, "../../testdata/demo")
	if e != nil {
		t.Fatal(e)
	}
	second, e := ImportDataset(ctx, s, "../../testdata/demo")
	if e != nil || S(first["datasetScope"]) != S(second["datasetScope"]) {
		t.Fatalf("unsafe repeat: %v", e)
	}
	temp := t.TempDir()
	for _, name := range []string{"manifest.json", "initial_state.json"} {
		b, e := os.ReadFile(filepath.Join("../../testdata/demo", name))
		if e != nil {
			t.Fatal(e)
		}
		if name == "manifest.json" {
			var m M
			if e = json.Unmarshal(b, &m); e != nil {
				t.Fatal(e)
			}
			m["id"] = "corrupt-" + ID()
			b, _ = json.Marshal(m)
		} else {
			b = append(b, byte(' '))
		}
		if e = os.WriteFile(filepath.Join(temp, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = ImportDataset(ctx, s, temp); e == nil || !strings.Contains(e.Error(), "checksum mismatch") {
		t.Fatalf("corrupt file accepted: %v", e)
	}
	var m M
	b, _ := os.ReadFile(filepath.Join(temp, "manifest.json"))
	_ = json.Unmarshal(b, &m)
	tx, e := s.Begin(ctx, "")
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.GlobalGet("datasets", S(m["id"])); e == nil {
		t.Fatal("failed import published")
	}
}
func TestImportRejectsInconsistentSnapshot(t *testing.T) {
	s := inventoryTestStore(t)
	temp := t.TempDir()
	b, e := os.ReadFile("../../testdata/demo/initial_state.json")
	if e != nil {
		t.Fatal(e)
	}
	var snap M
	_ = json.Unmarshal(b, &snap)
	Arr(snap["batches"])[0]["availableQuantity"] = -1
	b, _ = json.Marshal(snap)
	if e = os.WriteFile(filepath.Join(temp, "initial_state.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(b)
	m := M{"id": "bad-stock-" + ID(), "schemaVersion": "2.0.0", "initialClock": "2026-01-05T09:00:00Z", "counts": M{"pointCount": 4, "orderIntentCount": 0}, "files": []M{{"path": "initial_state.json", "rows": 1, "sha256": hex.EncodeToString(hash[:])}}}
	b, _ = json.Marshal(m)
	if e = os.WriteFile(filepath.Join(temp, "manifest.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ImportDataset(context.Background(), s, temp); e == nil || !strings.Contains(e.Error(), "invalid availableQuantity") {
		t.Fatalf("inconsistent stock accepted: %v", e)
	}
}

func importWriteSnapshot(t *testing.T, snapshot M) string {
	t.Helper()
	dir := t.TempDir()
	raw, e := json.Marshal(snapshot)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	m := M{"id": "snapshot-" + ID(), "schemaVersion": "2.0.0", "initialClock": "2026-01-05T09:00:00Z", "counts": M{"pointCount": len(Arr(snapshot["pickup-points"])), "orderCount": len(Arr(snapshot["orders"])), "orderIntentCount": len(Arr(snapshot["order-intents"]))}, "files": []M{{"path": "initial_state.json", "sha256": hex.EncodeToString(sum[:]), "rows": 1}}}
	manifest, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "initial_state.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	return dir
}
func importBaseSnapshot(t *testing.T) M {
	t.Helper()
	b, e := os.ReadFile("../../testdata/demo/initial_state.json")
	if e != nil {
		t.Fatal(e)
	}
	var m M
	if e = Decode(b, &m); e != nil {
		t.Fatal(e)
	}
	return m
}
func TestImportRechecksSourceAndRejectsEscapingSymlink(t *testing.T) {
	s := inventoryTestStore(t)
	dir := importWriteSnapshot(t, importBaseSnapshot(t))
	if _, e := ImportDataset(context.Background(), s, dir); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "initial_state.json")
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, append(b, byte(' ')), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ImportDataset(context.Background(), s, dir); e == nil || !strings.Contains(e.Error(), "checksum mismatch") {
		t.Fatalf("repeat skipped checksum: %v", e)
	}
	dir = importWriteSnapshot(t, importBaseSnapshot(t))
	outside := filepath.Join(t.TempDir(), "outside.json")
	path = filepath.Join(dir, "initial_state.json")
	b, e = os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(outside, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(outside, path); e != nil {
		t.Fatal(e)
	}
	if _, e = ImportDataset(context.Background(), s, dir); e == nil {
		t.Fatal("escaping symlink accepted")
	}
}
func TestImportRejectsBrokenSchedulesAndPreservesExactMoney(t *testing.T) {
	s := inventoryTestStore(t)
	snapshot := importBaseSnapshot(t)
	Arr(Arr(snapshot["trips"])[0]["stops"])[1]["nodeId"] = "missing"
	if _, e := ImportDataset(context.Background(), s, importWriteSnapshot(t, snapshot)); e == nil {
		t.Fatal("broken stop reference accepted")
	}
	snapshot = importBaseSnapshot(t)
	Arr(snapshot["products"])[0]["salePriceMinor"] = json.Number("9007199254740993")
	m, e := ImportDataset(context.Background(), s, importWriteSnapshot(t, snapshot))
	if e != nil {
		t.Fatal(e)
	}
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	tx.Scope = S(m["datasetScope"])
	p, e := tx.Get("products", "101")
	if e != nil || I(p["salePriceMinor"]) != 9007199254740993 {
		t.Fatalf("money lost precision: %v %v", p, e)
	}
}
func TestImportRestoresPreparedOrderAndAssociatedReservations(t *testing.T) {
	s := inventoryTestStore(t)
	source := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	actor := Actor{Subject: "prepared", Roles: []string{"dispatcher"}}
	q, e := Dispatch(tx, Command{Name: "createQuote", Actor: actor, Body: M{"pickupPointId": "13", "items": []M{{"productId": "101", "quantity": 1}}}})
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	order, e := Dispatch(tx, Command{Name: "createOrder", Actor: actor, Body: M{"quoteId": q["id"], "optionId": q["recommendedOptionId"]}})
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	snapshot := M{}
	rows, e := tx.SQL.Query(tx.Context, "SELECT DISTINCT kind FROM mesh_records WHERE scope IN($1,$2)", tx.Scope, tx.Dataset)
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	kinds := []string{}
	for rows.Next() {
		var k string
		if e = rows.Scan(&k); e != nil {
			rows.Close()
			tx.Rollback()
			t.Fatal(e)
		}
		kinds = append(kinds, k)
	}
	rows.Close()
	for _, k := range kinds {
		items, e := tx.List(k, nil)
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		snapshot[k] = items
	}
	tx.Rollback()
	dir := importWriteSnapshot(t, snapshot)
	dataset, e := ImportDataset(context.Background(), s, dir)
	if e != nil {
		t.Fatal(e)
	}
	create, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	r, e := Dispatch(create, Command{Name: "createDemoRun", Actor: Actor{Subject: "restore-" + ID()}, Body: M{"datasetId": dataset["id"], "expectedContextVersion": 0}})
	if e != nil {
		create.Rollback()
		t.Fatal(e)
	}
	if e = create.Commit(); e != nil {
		t.Fatal(e)
	}
	restored, e := s.Begin(context.Background(), S(r["id"]))
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Rollback()
	o, e := restored.Get("orders", S(order["id"]))
	if e != nil || I(o["version"]) != I(order["version"]) {
		t.Fatalf("prepared order lost %v %v", o, e)
	}
	batch, e := restored.Get("batches", "41")
	if e != nil || I(batch["reservedQuantity"]) != 1 || I(batch["availableQuantity"]) != 4 {
		t.Fatalf("prepared reserves lost %v %v", batch, e)
	}
	if _, e = restored.Get("reservations", S(order["id"])); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.Get("routes", routeID(S(order["id"]), 1)); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.Get("route-holds", S(order["holdId"])); e != nil {
		t.Fatal(e)
	}
	delete(snapshot, "reservations")
	if _, e = ImportDataset(context.Background(), s, importWriteSnapshot(t, snapshot)); e == nil {
		t.Fatal("prepared order without reservation accepted")
	}
}
func TestImportRejectsOverflowingPhysicalStock(t *testing.T) {
	s := inventoryTestStore(t)
	snapshot := importBaseSnapshot(t)
	b := Arr(snapshot["batches"])[0]
	b["receivedQuantity"] = json.Number("9223372036854775807")
	b["availableQuantity"] = json.Number("9223372036854775807")
	b["reservedQuantity"] = 1
	if _, e := ImportDataset(context.Background(), s, importWriteSnapshot(t, snapshot)); e == nil || !strings.Contains(e.Error(), "exceeds int64") {
		t.Fatalf("overflowing stock accepted: %v", e)
	}
}
func TestImportAllContinuesAfterIndependentFailure(t *testing.T) {
	s := inventoryTestStore(t)
	root := t.TempDir()
	catalog := []M{}
	datasetIDs := []string{}
	for index := 0; index < 2; index++ {
		snapshot := importBaseSnapshot(t)
		if index == 0 {
			delete(snapshot, "batches")
		}
		dir := importWriteSnapshot(t, snapshot)
		b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if e != nil {
			t.Fatal(e)
		}
		var manifest M
		if e = Decode(b, &manifest); e != nil {
			t.Fatal(e)
		}
		id := S(manifest["id"])
		datasetIDs = append(datasetIDs, id)
		if e = os.Rename(dir, filepath.Join(root, id)); e != nil {
			t.Fatal(e)
		}
		catalog = append(catalog, M{"id": id, "name": id, "summary": "Synthetic import-all test", "status": "ready", "seed": 42, "initialClock": "2026-01-05T09:00:00Z", "pointCount": 4, "orderCount": 0})
	}
	raw, e := json.Marshal(M{"items": catalog})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "catalog.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	out, e := ImportAll(context.Background(), s, root)
	if e == nil || len(out) != 1 || S(out[0]["id"]) != datasetIDs[1] {
		t.Fatalf("independent import stopped: out=%v error=%v", out, e)
	}
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.GlobalGet("datasets", datasetIDs[0]); e == nil {
		t.Fatal("incomplete dataset published")
	}
	if _, e = tx.GlobalGet("datasets", datasetIDs[1]); e != nil {
		t.Fatal(e)
	}
}
func TestImportRejectsUnknownSchemaAndMalformedDatasetID(t *testing.T) {
	s := inventoryTestStore(t)
	for _, tc := range []struct{ id, version string }{{"unknown", "2.1.0"}, {"bad id", "2.0.0"}, {"../escape", "2.0.0"}} {
		dir := t.TempDir()
		raw, _ := json.Marshal(M{"id": tc.id, "schemaVersion": tc.version})
		if e := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := ImportDataset(context.Background(), s, dir); e == nil {
			t.Fatalf("accepted unsupported manifest %v", tc)
		}
	}
}
