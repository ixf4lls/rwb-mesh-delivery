package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func importedRows(tx *Tx, kind string, visit func(M) error) error {
	rows, e := tx.SQL.Query(tx.Context, "SELECT data FROM mesh_records WHERE scope=$1 AND kind=$2 ORDER BY id", tx.Scope, kind)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return e
		}
		var m M
		if e = Decode(raw, &m); e != nil {
			return e
		}
		if e = validateImportedIDs(m); e != nil {
			return fmt.Errorf("%s record %s: %w", kind, S(m["id"]), e)
		}
		if e = visit(m); e != nil {
			return fmt.Errorf("%s record %s: %w", kind, S(m["id"]), e)
		}
	}
	return rows.Err()
}
func importInteger(m M, key string, minimum int64) (int64, error) {
	if _, ok := m[key]; !ok {
		return 0, fmt.Errorf("missing %s", key)
	}
	v, e := strconv.ParseInt(S(m[key]), 10, 64)
	if e != nil || v < minimum {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return v, nil
}
func validateImportedSnapshot(tx *Tx, datasetID string, clock time.Time) error {
	fail := func(e error) error {
		if e == nil {
			return nil
		}
		return fmt.Errorf("%s: %w", datasetID, e)
	}
	if e := importedRows(tx, "cities", func(m M) error {
		if _, e := time.LoadLocation(S(m["timeZone"])); e != nil {
			return fmt.Errorf("invalid timeZone")
		}
		return nil
	}); e != nil {
		return fail(e)
	}
	if e := importedRows(tx, "nodes", func(m M) error {
		if !Contains([]string{"storage_pvz", "pickup_only", "hub", "sorting_center"}, S(m["kind"])) || !Contains([]string{"physical", "simulated"}, S(m["mode"])) {
			return fmt.Errorf("invalid node kind or execution mode")
		}
		if _, ok := m["active"].(bool); !ok {
			return fmt.Errorf("missing active flag")
		}
		return nil
	}); e != nil {
		return fail(e)
	}
	if e := importedRows(tx, "products", func(m M) error {
		for _, key := range []string{"weightGrams", "slotsPerUnit"} {
			if _, e := importInteger(m, key, 1); e != nil {
				return e
			}
		}
		for _, key := range []string{"salePriceMinor", "unitCostMinor"} {
			if _, e := importInteger(m, key, 0); e != nil {
				return e
			}
		}
		for _, key := range []string{"widthM", "heightM", "depthM"} {
			if v, ok := m[key]; ok {
				n, e := strconv.ParseFloat(S(v), 64)
				if e != nil || n <= 0 || strings.ContainsAny(S(v), "NnIi") {
					return fmt.Errorf("invalid dimensions in %s", key)
				}
			}
		}
		return nil
	}); e != nil {
		return fail(e)
	}
	nodeRows, e := tx.List("nodes", nil)
	if e != nil {
		return e
	}
	nodeIDs := map[string]bool{}
	for _, n := range nodeRows {
		nodeIDs[S(n["id"])] = true
	}
	if e := importedRows(tx, "trips", func(m M) error {
		if !Contains([]string{"physical", "simulated"}, S(m["mode"])) {
			return fmt.Errorf("invalid trip mode")
		}
		if _, e := importInteger(m, "capacitySlots", 1); e != nil {
			return e
		}
		if background, e := importInteger(m, "backgroundSlots", 0); e != nil || background > I(m["capacitySlots"]) {
			return fmt.Errorf("invalid trip background capacity")
		}
		if Time(m["departureAt"]).IsZero() {
			return fmt.Errorf("missing complete departureAt schedule")
		}
		if _, e := importInteger(m, "durationMinutes", 1); e != nil {
			return e
		}
		stops := Arr(m["stops"])
		if len(stops) < 2 || S(stops[0]["nodeId"]) != S(m["fromNodeId"]) || S(stops[len(stops)-1]["nodeId"]) != S(m["toNodeId"]) {
			return fmt.Errorf("missing ordered endpoint stops")
		}
		last := int64(-1)
		for _, s := range stops {
			if !nodeIDs[S(s["nodeId"])] {
				return fmt.Errorf("stop references unknown node")
			}
			off, e := importInteger(s, "offsetMinutes", 0)
			if e != nil || off < last || off > I(m["durationMinutes"]) {
				return fmt.Errorf("invalid stop sequence")
			}
			last = off
		}
		return nil
	}); e != nil {
		return fail(e)
	}
	// Join product data in one cursor, avoiding one database query per batch.
	rows, e := tx.SQL.Query(tx.Context, `SELECT b.data,p.data,n.data FROM mesh_records b JOIN mesh_records p ON p.scope=b.scope AND p.kind='products' AND p.id=b.data->>'productId' JOIN mesh_records n ON n.scope=b.scope AND n.kind='nodes' AND n.id=b.data->>'storagePointId' WHERE b.scope=$1 AND b.kind='batches' ORDER BY b.id`, tx.Scope)
	if e != nil {
		return e
	}
	for rows.Next() {
		var br, pr, nr []byte
		if e = rows.Scan(&br, &pr, &nr); e != nil {
			rows.Close()
			return e
		}
		var b, p, n M
		for _, pair := range []struct {
			raw []byte
			out *M
		}{{br, &b}, {pr, &p}, {nr, &n}} {
			if e = Decode(pair.raw, pair.out); e != nil {
				rows.Close()
				return e
			}
		}
		var qty int64
		for _, key := range []string{"availableQuantity", "reservedQuantity", "packedQuantity", "quarantinedQuantity"} {
			q, e := importInteger(b, key, 0)
			if e != nil {
				rows.Close()
				return fail(fmt.Errorf("batches record %s: %w", S(b["id"]), e))
			}
			qty, e = CheckedAdd(qty, q)
			if e != nil {
				rows.Close()
				return fail(fmt.Errorf("batches record %s: %w", S(b["id"]), e))
			}
		}
		received, e := importInteger(b, "receivedQuantity", 1)
		if e != nil || received < qty {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: inconsistent receivedQuantity", S(b["id"])))
		}
		slots, e := CheckedMul(qty, I(p["slotsPerUnit"]))
		if e != nil {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: %w", S(b["id"]), e))
		}
		if I(b["usedSlots"]) != slots {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: inconsistent physical slots", S(b["id"])))
		}
		if !Contains([]string{"storage_pvz", "hub"}, S(n["kind"])) {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: stock at non-storage node", S(b["id"])))
		}
		receivedAt := Time(b["receivedAt"])
		if receivedAt.IsZero() || receivedAt.After(clock) {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: invalid receipt time", S(b["id"])))
		}
		if S(b["sellerId"]) != S(p["sellerId"]) {
			rows.Close()
			return fail(fmt.Errorf("batches record %s: seller does not own product", S(b["id"])))
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	// Validate static point capacities and canonical node bindings before aggregate occupancy.
	points, e := tx.List("pickup-points", nil)
	if e != nil {
		return e
	}
	for _, p := range points {
		nodeID := S(p["nodeId"])
		if nodeID == "" {
			nodeID = S(Obj(p["node"])["id"])
		}
		n, e := tx.Get("nodes", nodeID)
		if e != nil {
			return fail(fmt.Errorf("point %s: unresolved node", S(p["id"])))
		}
		if S(n["cityId"]) != S(p["cityId"]) || !Contains([]string{"storage_pvz", "pickup_only"}, S(n["kind"])) {
			return fail(fmt.Errorf("point %s: inconsistent node binding", S(p["id"])))
		}
		cap := Obj(p["capacity"])
		if len(cap) == 0 {
			cap = p
		}
		total, e := importInteger(cap, "totalSlots", 1)
		if e != nil {
			return fail(fmt.Errorf("point %s: %w", S(p["id"]), e))
		}
		quota, e := importInteger(cap, "storageQuotaSlots", 0)
		if e != nil || quota > total {
			return fail(fmt.Errorf("point %s: invalid storage quota", S(p["id"])))
		}
		for _, key := range []string{"ordinaryUsedSlots", "pickupUsedSlots", "inboundReservedSlots"} {
			if _, e = importInteger(cap, key, 0); e != nil {
				return fail(fmt.Errorf("point %s: %w", S(p["id"]), e))
			}
		}
		if len(Arr(p["openingHours"])) == 0 {
			return fail(fmt.Errorf("point %s: missing opening hours", S(p["id"])))
		}
		for _, window := range Arr(p["openingHours"]) {
			if len(Strings(window["weekdays"])) == 0 {
				return fail(fmt.Errorf("point %s: missing weekdays", S(p["id"])))
			}
			for _, key := range []string{"opens", "closes"} {
				if _, e = time.Parse("15:04", S(window[key])); e != nil {
					return fail(fmt.Errorf("point %s: invalid opening hours", S(p["id"])))
				}
			}
		}
		actual, e := PointCapacity(tx, S(p["id"]))
		if e != nil {
			return fail(e)
		}
		occupied, e := inventorySum(I(actual["storageUsedSlots"]), I(actual["ordinaryUsedSlots"]), I(actual["pickupUsedSlots"]), I(actual["inboundReservedSlots"]))
		if e != nil {
			return fail(e)
		}
		if I(actual["storageUsedSlots"]) > quota || occupied > total {
			return fail(fmt.Errorf("point %s: initial occupancy exceeds capacity", S(p["id"])))
		}
	}
	return validatePreparedSnapshot(tx, datasetID)
}
func validatePreparedSnapshot(tx *Tx, datasetID string) error {
	orders, e := tx.List("orders", nil)
	if e != nil {
		return e
	}
	reservations, e := tx.List("reservations", nil)
	if e != nil {
		return e
	}
	byOrder := map[string]M{}
	for _, r := range reservations {
		byOrder[S(r["id"])] = r
	}
	expectedReserved, expectedPacked := map[string]int64{}, map[string]int64{}
	for _, o := range orders {
		id := S(o["id"])
		if I(o["version"]) < 1 || I(o["routeRevision"]) < 1 {
			return fmt.Errorf("%s orders record %s: missing state or route version", datasetID, id)
		}
		route, e := tx.Get("routes", routeID(id, I(o["routeRevision"])))
		if e != nil || S(route["orderId"]) != id {
			return fmt.Errorf("%s orders record %s: missing active route revision", datasetID, id)
		}
		r := byOrder[id]
		if r == nil {
			return fmt.Errorf("%s orders record %s: missing associated inventory reservation", datasetID, id)
		}
		hold, holdError := tx.Get("route-holds", S(o["holdId"]))
		if holdError != nil || S(hold["subjectId"]) != id || S(hold["subjectType"]) != "order" {
			return fmt.Errorf("%s orders record %s: missing associated transport hold", datasetID, id)
		}
		shipments := map[string]bool{}
		for _, sh := range Arr(o["shipments"]) {
			shipments[S(sh["id"])] = true
		}
		lines := map[string]M{}
		for _, line := range Arr(o["lines"]) {
			lines[S(line["id"])] = line
		}
		states := Obj(r["states"])
		allocated := map[string]int64{}
		for _, a := range Arr(r["allocations"]) {
			sid := S(a["shipmentId"])
			line := lines[S(a["orderItemId"])]
			if !shipments[sid] || line == nil || I(a["quantity"]) <= 0 {
				return fmt.Errorf("%s orders record %s: invalid reservation allocation", datasetID, id)
			}
			allocated[S(a["orderItemId"])], e = CheckedAdd(allocated[S(a["orderItemId"])], I(a["quantity"]))
			if e != nil {
				return e
			}
			batch, e := tx.Get("batches", S(a["batchId"]))
			if e != nil || S(batch["productId"]) != S(line["productId"]) {
				return fmt.Errorf("%s orders record %s: missing or inconsistent allocated batch", datasetID, id)
			}
			if B(states["issued"]) || B(states["dispatched:"+sid]) || B(states["release::"]) {
				continue
			}
			if B(states["pack:"+sid+":"]) {
				expectedPacked[S(batch["id"])], e = CheckedAdd(expectedPacked[S(batch["id"])], I(a["quantity"]))
				if e != nil {
					return e
				}
			} else {
				expectedReserved[S(batch["id"])], e = CheckedAdd(expectedReserved[S(batch["id"])], I(a["quantity"]))
				if e != nil {
					return e
				}
			}
		}
		for lineID, line := range lines {
			if allocated[lineID] != I(line["quantity"]) {
				return fmt.Errorf("%s orders record %s: line allocation quantity mismatch", datasetID, id)
			}
		}
	}
	if len(orders) != len(reservations) {
		return fmt.Errorf("%s: orphan order reservation", datasetID)
	}
	return importedRows(tx, "batches", func(b M) error {
		if I(b["reservedQuantity"]) != expectedReserved[S(b["id"])] || I(b["packedQuantity"]) != expectedPacked[S(b["id"])] {
			return fmt.Errorf("prepared order reserves do not match reserved/packed stock")
		}
		return nil
	})
}

var importedIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validateImportedIDs(m M) error {
	for key, value := range m {
		if key != "id" && key != "sellerSku" && !strings.HasSuffix(key, "Id") {
			continue
		}
		s, ok := value.(string)
		if !ok || len(s) > 128 || !importedIDPattern.MatchString(s) {
			return fmt.Errorf("%s must be a nonempty string identifier", key)
		}
	}
	return nil
}
