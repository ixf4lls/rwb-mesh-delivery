package core

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ImportDataset publishes an immutable version only after every listed file and
// reference has been validated. Expected results are deliberately never inputs.
func ImportDataset(ctx context.Context, store *Store, path string) (M, error) {
	root, e := os.OpenRoot(path)
	if e != nil {
		return nil, e
	}
	defer root.Close()
	mf, e := root.Open("manifest.json")
	if e != nil {
		return nil, e
	}
	b, e := io.ReadAll(io.LimitReader(mf, 4<<20))
	mf.Close()
	if e != nil {
		return nil, e
	}
	var manifest M
	if e = Decode(b, &manifest); e != nil {
		return nil, e
	}
	id := S(manifest["id"])
	if id == "" {
		id = S(manifest["datasetId"])
	}
	if id == "" {
		return nil, fmt.Errorf("%s: missing dataset id", path)
	}
	if S(manifest["schemaVersion"]) != "2.0.0" {
		return nil, fmt.Errorf("%s: unsupported schema version %s", id, S(manifest["schemaVersion"]))
	}
	if e = validateImportedIDs(M{"id": id}); e != nil {
		return nil, fmt.Errorf("invalid dataset identifier: %w", e)
	}
	hash := sha256.Sum256(b)
	scope := "dataset:" + id + ":" + hex.EncodeToString(hash[:])
	tx, e := store.Begin(ctx, "")
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.SQL.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", scope); e != nil {
		return nil, e
	}
	dataset := M{"id": id, "name": id, "summary": "Imported immutable dataset", "status": "ready", "seed": I(manifest["seed"]), "initialClock": manifest["initialClock"], "pointCount": I(Obj(manifest["counts"])["pointCount"]), "orderCount": I(Obj(manifest["counts"])["orderCount"]), "datasetScope": scope, "version": hex.EncodeToString(hash[:]), "manifest": manifest}
	// Dataset catalog supplies presentation metadata, not domain state.
	if cb, err := os.ReadFile(filepath.Join(filepath.Dir(path), "catalog.json")); err == nil {
		var c M
		if Decode(cb, &c) == nil {
			for _, item := range Arr(c["items"]) {
				if S(item["id"]) == id {
					for _, k := range []string{"name", "summary"} {
						if v, ok := item[k]; ok {
							dataset[k] = v
						}
					}
					break
				}
			}
		}
	}
	if _, err := time.Parse(time.RFC3339, S(dataset["initialClock"])); err != nil {
		return nil, fmt.Errorf("%s: invalid initialClock", id)
	}
	old, err := tx.GlobalGet("datasets", id)
	already := err == nil && S(old["datasetScope"]) == scope
	tx.Scope = scope
	seenFiles := map[string]bool{}
	for _, file := range Arr(manifest["files"]) {
		if _, e = importInteger(file, "rows", 0); e != nil {
			return nil, fmt.Errorf("%s manifest file: %w", id, e)
		}
		rel := S(file["path"])
		clean := filepath.Clean(rel)
		if filepath.IsAbs(rel) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("%s %s: unsafe path", id, rel)
		}
		if seenFiles[clean] {
			return nil, fmt.Errorf("%s %s: duplicate manifest path", id, rel)
		}
		seenFiles[clean] = true
		f, err := root.Open(clean)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", id, rel, err)
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if hex.EncodeToString(h.Sum(nil)) != S(file["sha256"]) {
			f.Close()
			return nil, fmt.Errorf("%s %s: checksum mismatch", id, rel)
		}
		if already {
			f.Close()
			continue
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			f.Close()
			return nil, e
		}
		count, err := importFile(tx, f, rel)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", id, rel, err)
		}
		if count != I(file["rows"]) {
			return nil, fmt.Errorf("%s %s: row count %d does not match manifest %d", id, rel, count, I(file["rows"]))
		}
	}
	if already {
		return old, nil
	}
	// Validate references against this exact immutable version; prepared snapshots
	// and future intents remain distinct record kinds.
	for _, spec := range []struct{ kind, field, target string }{{"products", "sellerId", "sellers"}, {"products", "sellerSku", "seller-skus"}, {"pickup-points", "cityId", "cities"}, {"nodes", "cityId", "cities"}, {"batches", "productId", "products"}, {"batches", "storagePointId", "nodes"}, {"batches", "tariffVersionId", "tariffs"}, {"seller-skus", "sellerId", "sellers"}, {"trips", "fromNodeId", "nodes"}, {"trips", "toNodeId", "nodes"}, {"order-intents", "pickupPointId", "pickup-points"}} {
		var bad string
		err = tx.SQL.QueryRow(ctx, `SELECT r.id FROM mesh_records r WHERE r.scope=$1 AND r.kind=$2 AND NOT EXISTS(SELECT 1 FROM mesh_records p WHERE p.scope=$1 AND p.kind=$4 AND p.id=r.data->>$3) LIMIT 1`, scope, spec.kind, spec.field, spec.target).Scan(&bad)
		if err == nil {
			return nil, fmt.Errorf("%s %s record %s: unresolved %s", id, spec.kind, bad, spec.field)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	// Dataset count checks apply to actual entities, never expected.json.
	for _, spec := range []struct{ kind, key string }{{"pickup-points", "pointCount"}, {"order-intents", "orderIntentCount"}} {
		expected, exists := Obj(manifest["counts"])[spec.key]
		if !exists {
			continue
		}
		var n int64
		if e = tx.SQL.QueryRow(ctx, "SELECT count(*) FROM mesh_records WHERE scope=$1 AND kind=$2", scope, spec.kind).Scan(&n); e != nil {
			return nil, e
		}
		if n != I(expected) {
			return nil, fmt.Errorf("%s: %s=%d, actual %d", id, spec.key, I(expected), n)
		}
	}

	if e = validateImportedSnapshot(tx, id, Time(dataset["initialClock"])); e != nil {
		return nil, e
	}
	dataset["sourceGaps"] = []string{}
	var n int64
	if e = tx.SQL.QueryRow(ctx, "SELECT count(*) FROM mesh_records WHERE scope=$1 AND kind='batches'", scope).Scan(&n); e != nil {
		return nil, e
	}
	if n == 0 {
		return nil, fmt.Errorf("%s: missing initial batch snapshot; order intents and expected results cannot supply inventory", id)
	}
	var incomplete string
	err = tx.SQL.QueryRow(ctx, `SELECT id FROM mesh_records WHERE scope=$1 AND kind='pickup-points' AND NOT (data ? 'capacity' OR data ? 'totalSlots') LIMIT 1`, scope).Scan(&incomplete)
	if err == nil {
		return nil, fmt.Errorf("%s: point %s missing capacity snapshot", id, incomplete)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if e = tx.GlobalPut("datasets", dataset); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return dataset, nil
}

func ImportAll(ctx context.Context, store *Store, path string) ([]M, error) {
	b, e := os.ReadFile(filepath.Join(path, "catalog.json"))
	if e != nil {
		return nil, e
	}
	var catalog M
	if e = Decode(b, &catalog); e != nil {
		return nil, e
	}
	out := []M{}
	failures := []error{}
	for _, d := range Arr(catalog["items"]) {
		if !validDatasetCatalog(d) {
			failures = append(failures, fmt.Errorf("dataset %s: invalid catalog fields", S(d["id"])))
			continue
		}
		if !Contains([]string{"ready", "unavailable"}, S(d["status"])) {
			failures = append(failures, fmt.Errorf("dataset %s: invalid catalog status", S(d["id"])))
			continue
		}
		if S(d["status"]) != "ready" {
			tx, e := store.Begin(ctx, "")
			if e != nil {
				return nil, e
			}
			e = tx.GlobalPut("datasets", d)
			if e == nil {
				e = tx.Commit()
			} else {
				tx.Rollback()
			}
			if e != nil {
				return nil, e
			}
			continue
		}
		m, e := ImportDataset(ctx, store, filepath.Join(path, S(d["id"])))
		if e != nil {
			failures = append(failures, e)
			continue
		}
		out = append(out, m)
	}
	return out, errors.Join(failures...)
}

func camel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
func importFile(tx *Tx, f *os.File, rel string) (int64, error) {
	var e error
	ext := strings.ToLower(filepath.Ext(rel))
	name := strings.TrimSuffix(filepath.Base(rel), ext)
	if ext == ".json" {
		var v any
		decoder := json.NewDecoder(f)
		decoder.UseNumber()
		if e := decoder.Decode(&v); e != nil {
			return 0, e
		}
		var tail any
		if e = decoder.Decode(&tail); e != io.EOF {
			return 0, fmt.Errorf("extra JSON content after document")
		}
		count := int64(1)
		if a, ok := v.([]any); ok {
			count = int64(len(a))
		}
		if name == "expected" || name == "api_examples" {
			return count, nil
		}
		if name == "tariff" {
			m := Obj(v)
			if S(m["id"]) == "" {
				return 0, fmt.Errorf("tariff missing id")
			}
			return count, tx.Put("tariffs", m)
		}
		if name == "snapshot" || name == "initial_state" {
			for kind, value := range Obj(v) {
				for _, m := range Arr(value) {
					var duplicate bool
					if e = tx.SQL.QueryRow(tx.Context, "SELECT EXISTS(SELECT 1 FROM mesh_records WHERE scope=$1 AND kind=$2 AND id=$3)", tx.Scope, kind, S(m["id"])).Scan(&duplicate); e != nil {
						return 0, e
					}
					if duplicate {
						return 0, fmt.Errorf("snapshot %s duplicate key %s", kind, S(m["id"]))
					}
					if e = tx.Put(kind, m); e != nil {
						return 0, e
					}
				}
			}
		}
		return count, nil
	}
	if ext != ".csv" {
		return 0, fmt.Errorf("unsupported actual input format %s", ext)
	}
	reader := csv.NewReader(f)
	head, e := reader.Read()
	if e != nil {
		return 0, e
	}
	var count int64
	batch := &pgx.Batch{}
	batchRecords := []int64{}
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		results := tx.SQL.SendBatch(tx.Context, batch)
		for _, record := range batchRecords {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return fmt.Errorf("record %d: %w", record, err)
			}
		}
		err := results.Close()
		batch = &pgx.Batch{}
		batchRecords = batchRecords[:0]
		return err
	}
	kind := strings.ReplaceAll(name, "_", "-")
	switch name {
	case "points":
		kind = "pickup-points"
	case "orders":
		kind = "order-intents"
	case "prepared_orders":
		kind = "orders"
	case "batches":
		kind = "batches"
	}
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		count++
		if e != nil {
			return count, fmt.Errorf("record %d: %w", count, e)
		}
		m := M{}
		for i, k := range head {
			v := row[i]
			key := camel(strings.TrimSuffix(k, "_json"))
			if strings.HasSuffix(k, "_json") {
				var parsed any
				if e = Decode([]byte(v), &parsed); e != nil {
					return count, fmt.Errorf("record %d %s: invalid JSON", count, k)
				}
				m[key] = normalizeJSON(parsed)
			} else if strings.HasSuffix(k, "_id") || k == "seller_sku" || k == "request_key" {
				m[key] = v
			} else if v == "true" || v == "false" {
				m[key] = v == "true"
			} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				m[key] = n
			} else {
				m[key] = v
			}
		}
		key := map[string]string{"cities": "cityId", "nodes": "nodeId", "points": "pickupPointId", "products": "productId", "seller_skus": "sellerSku", "sellers": "sellerId", "batches": "batchId", "orders": "requestKey", "prepared_orders": "orderId", "trips": "tripId", "reservations": "orderId", "routes": "id"}[name]
		idKeys := []string{"id", key}
		id := ""
		for _, k := range idKeys {
			if s := S(m[k]); s != "" {
				id = s
				break
			}
		}
		if name == "placement_pairs" {
			id = S(m["productId"]) + "-" + S(m["storagePointId"])
		}
		if name == "demand_history" || name == "return_intents" || name == "placement_pairs" || name == "trips" && S(m["tripId"]) == "" {
			id = fmt.Sprint(count)
		}
		if id == "" {
			id = fmt.Sprint(count)
		}
		m["id"] = id
		switch name {
		case "cities":
			m["name"] = m["cityName"]
		case "products":
			m["name"] = m["productName"]
			if I(m["slotsPerUnit"]) <= 0 || I(m["weightGrams"]) <= 0 {
				return count, fmt.Errorf("record %d: nonpositive product slots or weight", count)
			}
		case "sellers":
			m["name"] = m["sellerName"]
		case "nodes":
			m["name"] = m["nodeName"]
		case "points":
			if S(m["nodeId"]) == "" {
				m["nodeId"] = id
			}
			if _, ok := m["active"]; !ok {
				m["active"] = true
			}
			node, e := tx.Get("nodes", S(m["nodeId"]))
			if e != nil {
				kind := "pickup_only"
				if B(m["storageEnabled"]) {
					kind = "storage_pvz"
				}
				node = M{"id": m["nodeId"], "cityId": m["cityId"], "name": m["pointName"], "kind": kind, "active": m["active"], "mode": "physical"}
				if e = tx.Put("nodes", node); e != nil {
					return count, e
				}
			}
			m["node"] = node
		case "orders":
			m["status"] = "pending"
		}
		encoded, err := json.Marshal(m)
		if err != nil {
			return count, err
		}
		batch.Queue("INSERT INTO mesh_records(scope,kind,id,data) VALUES($1,$2,$3,$4)", tx.Scope, kind, id, encoded)
		batchRecords = append(batchRecords, count)
		if batch.Len() >= 512 {
			if e = flush(); e != nil {
				return count, e
			}
		}

	}
	if e = flush(); e != nil {
		return count, e
	}
	return count, nil
}
func normalizeJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := M{}
		for k, v := range x {
			o[camel(k)] = normalizeJSON(v)
		}
		return o
	case []any:
		for i, v := range x {
			x[i] = normalizeJSON(v)
		}
		return x
	}
	return v
}

func validDatasetCatalog(d M) bool {
	if S(d["id"]) == "" || strings.ContainsAny(S(d["id"]), "/\\") || S(d["id"]) == ".." || S(d["name"]) == "" || S(d["summary"]) == "" || Time(d["initialClock"]).IsZero() {
		return false
	}
	for _, key := range []string{"pointCount", "orderCount"} {
		if _, e := importInteger(d, key, 0); e != nil {
			return false
		}
	}
	if _, ok := d["seed"]; !ok {
		return false
	}
	_, e := strconv.ParseInt(S(d["seed"]), 10, 64)
	return e == nil
}
