package core

import (
	"encoding/json"
)

func init() { Register(inventoryHandler) }
func PointCapacity(tx *Tx, pointID string) (M, error) {
	p, e := tx.Get("pickup-points", pointID)
	if e != nil {
		return nil, e
	}
	config := Obj(p["capacity"])
	if len(config) == 0 {
		config = p
	}
	total := I(config["totalSlots"])
	quota := I(config["storageQuotaSlots"])
	if total <= 0 {
		return nil, Fail(422, "capacity_unconfigured", "point has no declared physical capacity")
	}
	batches, e := tx.List("batches", M{"storagePointId": pointID})
	if e != nil {
		return nil, e
	}
	var used int64
	for _, b := range batches {
		used, e = CheckedAdd(used, I(b["usedSlots"]))
		if e != nil {
			return nil, e
		}
	}
	occ, e := tx.Get("point-occupancy", pointID)
	if e != nil {
		if p, ok := e.(*Problem); !ok || p.Status != 404 {
			return nil, e
		}
		occ = M{}
	}
	ordinary, e := CheckedAdd(I(config["ordinaryUsedSlots"]), I(occ["ordinaryUsedSlots"]))
	if e != nil {
		return nil, e
	}
	pickup, e := CheckedAdd(I(config["pickupUsedSlots"]), I(occ["pickupUsedSlots"]))
	if e != nil {
		return nil, e
	}
	inbound, e := CheckedAdd(I(config["inboundReservedSlots"]), I(occ["inboundReservedSlots"]))
	if e != nil {
		return nil, e
	}
	holds, e := tx.List("capacity-holds", M{"pointId": pointID, "state": "held"})
	if e != nil {
		return nil, e
	}
	for _, h := range holds {
		inbound, e = CheckedAdd(inbound, I(h["slots"]))
		if e != nil {
			return nil, e
		}
	}
	occupied, e := inventorySum(used, ordinary, pickup, inbound)
	if e != nil {
		return nil, e
	}
	free := max64(0, total-occupied)
	return M{"pointId": pointID, "asOf": Stamp(tx.Now()), "totalSlots": total, "storageQuotaSlots": quota, "storageUsedSlots": used, "ordinaryUsedSlots": ordinary, "pickupUsedSlots": pickup, "inboundReservedSlots": inbound, "availableStorageSlots": min64(free, max64(0, quota-used)), "availablePickupSlots": free}, nil
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func AdjustBatch(tx *Tx, b M, available, reserved, packed, quarantine int64) error {
	before := I(b["usedSlots"])
	b = Clone(b)
	for k, d := range map[string]int64{"availableQuantity": available, "reservedQuantity": reserved, "packedQuantity": packed, "quarantinedQuantity": quarantine} {
		n, e := CheckedAdd(I(b[k]), d)
		if e != nil {
			return e
		}
		if n < 0 {
			return Fail(409, "reservation_conflict", "insufficient "+k)
		}
		b[k] = n
	}
	p, e := tx.Get("products", S(b["productId"]))
	if e != nil {
		return e
	}
	quantity, e := inventorySum(I(b["availableQuantity"]), I(b["reservedQuantity"]), I(b["packedQuantity"]), I(b["quarantinedQuantity"]))
	if e != nil {
		return e
	}
	b["usedSlots"], e = CheckedMul(quantity, I(p["slotsPerUnit"]))
	if e != nil {
		return e
	}
	if I(b["usedSlots"]) != before {
		if e = tx.Put("batch-history", M{"id": ID(), "batchId": b["id"], "occurredAt": Stamp(tx.Now()), "usedSlots": before, "afterUsedSlots": b["usedSlots"]}); e != nil {
			return e
		}
	}
	return tx.Put("batches", b)
}
func pointOccupancy(tx *Tx, id, key string, delta int64) error {
	m, e := tx.Get("point-occupancy", id)
	if e != nil {
		if p, ok := e.(*Problem); !ok || p.Status != 404 {
			return e
		}
		m = M{"id": id}
	}
	v, e := CheckedAdd(I(m[key]), delta)
	if e != nil {
		return e
	}
	if v < 0 {
		return Fail(409, "reservation_conflict", "occupancy underflow")
	}
	m[key] = v
	return tx.Put("point-occupancy", m)
}
func capacityError(required, available int64) error {
	return &Problem{Status: 409, Code: "capacity_exceeded", Detail: "insufficient point slots", Extra: M{"requiredSlots": required, "availableSlots": available}}
}
func inventoryVisible(c Command, m M) bool {
	if c.Actor.Has("dispatcher") {
		return true
	}
	if c.Actor.Has("seller") && c.Actor.Seller(S(m["sellerId"])) {
		return true
	}
	return c.Actor.Has("pvz_operator") && c.Actor.Point(S(m["storagePointId"]))
}
func inventoryHandler(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "listCities", "svcListCities":
		rows, e := tx.List("cities", nil)
		return Page(rows), true, e
	case "listPickupPoints", "svcListPickupPoints":
		filters := M{}
		if city := c.Query.Get("cityId"); city != "" {
			filters["cityId"] = city
		}
		rows, e := tx.List("pickup-points", filters)
		return Page(rows), true, e
	case "svcGetNode":
		m, e := tx.Get("nodes", c.Params["nodeId"])
		return m, true, e
	case "getPointCapacity":
		id := c.Params["pointId"]
		if !c.Actor.Point(id) {
			return nil, true, Fail(403, "forbidden", "point outside scope")
		}
		m, e := PointCapacity(tx, id)
		return m, true, e
	case "listProducts":
		filters := M{}
		if s := c.Query.Get("sellerId"); s != "" {
			filters["sellerId"] = s
		}
		rows, e := tx.List("products", filters)
		if c.Actor.Has("seller") && !c.Actor.Has("dispatcher") {
			filtered := []M{}
			for _, r := range rows {
				if c.Actor.Seller(S(r["sellerId"])) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}
		return Page(rows), true, e
	case "createProduct":
		if !c.Actor.Seller(S(c.Body["sellerId"])) {
			return nil, true, Fail(403, "forbidden", "seller outside scope")
		}
		if _, e := tx.Get("sellers", S(c.Body["sellerId"])); e != nil {
			return nil, true, e
		}
		rows, e := tx.List("products", M{"sellerId": c.Body["sellerId"], "sellerSku": c.Body["sellerSku"]})
		if e != nil {
			return nil, true, e
		}
		if len(rows) > 0 {
			return nil, true, Fail(409, "product_exists", "seller SKU already exists")
		}
		m := Clone(c.Body)
		m["id"] = ID()
		return m, true, tx.Put("products", m)
	case "listBatches":
		filters := M{}
		for _, k := range []string{"sellerId", "productId", "storagePointId"} {
			if s := c.Query.Get(k); s != "" {
				filters[k] = s
			}
		}
		if s := c.Query.Get("pointId"); s != "" {
			filters["storagePointId"] = s
		}
		rows, e := tx.List("batches", filters)
		out := []M{}
		for _, r := range rows {
			if inventoryVisible(c, r) {
				out = append(out, r)
			}
		}
		return Page(out), true, e
	case "getBatch", "getStorageCost":
		b, e := tx.Get("batches", c.Params["batchId"])
		if e != nil {
			return nil, true, e
		}
		if !inventoryVisible(c, b) {
			return nil, true, Fail(403, "forbidden", "batch outside scope")
		}
		if c.Name == "getStorageCost" {
			asOf := tx.Now()
			if q := c.Query.Get("asOf"); q != "" {
				asOf = Time(q)
			}
			m, e := StorageCostForBatch(tx, b, asOf)
			return m, true, e
		}
		return b, true, nil
	case "receiveBatch":
		id := S(c.Body["storagePointId"])
		if !c.Actor.Point(id) {
			return nil, true, Fail(403, "forbidden", "point outside scope")
		}
		product, e := tx.Get("products", S(c.Body["productId"]))
		if e != nil {
			return nil, true, e
		}
		p, e := tx.Get("pickup-points", id)
		if e != nil {
			return nil, true, e
		}
		if S(Obj(p["node"])["kind"]) != "storage_pvz" {
			return nil, true, Fail(422, "invalid_storage_point", "stock requires storage-enabled PVZ")
		}
		q := I(c.Body["quantity"])
		if q <= 0 {
			return nil, true, Fail(400, "invalid_quantity", "quantity must be positive")
		}
		cap, e := PointCapacity(tx, id)
		if e != nil {
			return nil, true, e
		}
		slots, e := CheckedMul(q, I(product["slotsPerUnit"]))
		if e != nil {
			return nil, true, e
		}
		if slots > I(cap["availableStorageSlots"]) {
			return nil, true, capacityError(slots, I(cap["availableStorageSlots"]))
		}
		tariffs, e := tx.List("tariffs", nil)
		if e != nil {
			return nil, true, e
		}
		var tariff M
		for _, t := range tariffs {
			if !Time(t["effectiveFrom"]).After(tx.Now()) && (tariff == nil || Time(t["effectiveFrom"]).After(Time(tariff["effectiveFrom"]))) {
				tariff = t
			}
		}
		if tariff == nil {
			return nil, true, Fail(422, "tariff_unconfigured", "no effective tariff")
		}
		var rate int64
		for _, r := range Arr(tariff["rates"]) {
			if S(r["code"]) == "storage_slot_day" {
				rate = I(r["amount"])
			}
		}
		b := M{"id": ID(), "sellerId": product["sellerId"], "productId": product["id"], "storagePointId": id, "receivedAt": Stamp(tx.Now()), "receivedQuantity": q, "availableQuantity": q, "reservedQuantity": int64(0), "packedQuantity": int64(0), "quarantinedQuantity": int64(0), "usedSlots": slots, "tariffVersionId": tariff["id"], "storageRateMinorPerSlotDay": rate}
		if s := S(c.Body["externalLotRef"]); s != "" {
			b["externalLotRef"] = s
		}
		return b, true, tx.Put("batches", b)
	case "svcSearchAvailability":
		offers, prices := []M{}, []M{}
		for _, item := range Arr(c.Body["items"]) {
			p, e := tx.Get("products", S(item["productId"]))
			if e != nil {
				return nil, true, e
			}
			prices = append(prices, M{"productId": p["id"], "sellerId": p["sellerId"], "unitPriceMinor": p["salePriceMinor"], "unitCostMinor": p["unitCostMinor"]})
			bs, e := tx.List("batches", M{"productId": p["id"]})
			if e != nil {
				return nil, true, e
			}
			for _, b := range bs {
				if I(b["availableQuantity"]) > 0 {
					offers = append(offers, M{"productId": p["id"], "batchId": b["id"], "nodeId": b["storagePointId"], "availableQuantity": b["availableQuantity"], "slotsPerUnit": p["slotsPerUnit"], "readyAt": Stamp(tx.Now())})
				}
			}
		}
		return M{"asOf": Stamp(tx.Now()), "offers": offers, "prices": prices}, true, nil
	case "svcReserveOrderStock":
		m, e := reserveInventory(tx, c)
		return m, true, e
	case "svcGetOrderReservation":
		m, e := tx.Get("reservations", c.Params["orderId"])
		return m, true, e
	case "svcApplyInventoryOrderEvent":
		m, e := applyInventoryEvent(tx, c)
		return m, true, e
	case "svcApplyStockMovement":
		m, e := applyStockMovement(tx, c)
		return m, true, e
	case "svcListStorageAccruals":
		rows, e := tx.List("batches", nil)
		if e != nil {
			return nil, true, e
		}
		out := []M{}
		for _, b := range rows {
			cost, e := StorageCostForBatch(tx, b, Time(c.Body["asOf"]))
			if e != nil {
				return nil, true, e
			}
			if I(cost["billableSlotDays"]) > 0 {
				out = append(out, M{"batchId": b["id"], "pointId": b["storagePointId"], "sellerId": b["sellerId"], "tariffVersionId": b["tariffVersionId"], "periodStart": b["receivedAt"], "periodEnd": c.Body["asOf"], "billableSlotDays": cost["billableSlotDays"]})
			}
		}
		return Page(out), true, nil
	}
	return nil, false, nil
}
func reserveInventory(tx *Tx, c Command) (M, error) {
	id := c.Params["orderId"]
	existing, e := tx.Get("reservations", id)
	if e == nil {
		a, _ := json.Marshal(M{"allocations": existing["allocations"], "shipments": existing["shipments"]})
		b, _ := json.Marshal(c.Body)
		if string(a) != string(b) {
			return nil, Fail(409, "reservation_conflict", "order already reserved differently")
		}
		return existing, nil
	}
	if p, ok := e.(*Problem); !ok || p.Status != 404 {
		return nil, e
	}
	quantities := map[string]int64{}
	for _, a := range Arr(c.Body["allocations"]) {
		if I(a["quantity"]) <= 0 {
			return nil, Fail(400, "invalid_quantity", "reservation quantity must be positive")
		}
		quantities[S(a["batchId"])], e = CheckedAdd(quantities[S(a["batchId"])], I(a["quantity"]))
		if e != nil {
			return nil, e
		}
	}
	for id, q := range quantities {
		b, e := tx.Get("batches", id)
		if e != nil {
			return nil, e
		}
		if q > I(b["availableQuantity"]) {
			return nil, Fail(409, "reservation_conflict", "stock no longer available")
		}
	}
	inbound := map[string]int64{}
	for _, s := range Arr(c.Body["shipments"]) {
		if I(s["slots"]) <= 0 {
			return nil, Fail(422, "validation_error", "shipment slots must be positive")
		}
		if S(s["fromNodeId"]) != S(s["pickupPointId"]) {
			inbound[S(s["pickupPointId"])], e = CheckedAdd(inbound[S(s["pickupPointId"])], I(s["slots"]))
			if e != nil {
				return nil, e
			}
		}
	}
	for point, slots := range inbound {
		cap, e := PointCapacity(tx, point)
		if e != nil {
			return nil, e
		}
		if slots > I(cap["availablePickupSlots"]) {
			return nil, capacityError(slots, I(cap["availablePickupSlots"]))
		}
	}
	for id, q := range quantities {
		b, _ := tx.Get("batches", id)
		if e = AdjustBatch(tx, b, -q, q, 0, 0); e != nil {
			return nil, e
		}
	}
	for point, slots := range inbound {
		if e = pointOccupancy(tx, point, "inboundReservedSlots", slots); e != nil {
			return nil, e
		}
	}
	r := Clone(c.Body)
	r["id"] = id
	r["states"] = M{}
	return r, tx.Put("reservations", r)
}
func applyInventoryEvent(tx *Tx, c Command) (M, error) {
	r, e := tx.Get("reservations", c.Params["orderId"])
	if e != nil {
		return nil, e
	}
	states := Obj(r["states"])
	typ := S(c.Body["type"])
	shID := S(c.Body["shipmentId"])
	key := typ + ":" + shID + ":" + S(c.Body["segmentId"])
	if B(states[key]) {
		return M{"applied": false, "changedBatchIds": []string{}}, nil
	}
	var sh M
	for _, s := range Arr(r["shipments"]) {
		if S(s["shipmentId"]) == shID {
			sh = s
		}
	}
	if typ != "issue" && typ != "release" && sh == nil {
		return nil, Fail(422, "invalid_shipment", "shipment does not belong to reservation")
	}
	if typ == "issue" {
		for _, s := range Arr(r["shipments"]) {
			sid := S(s["shipmentId"])
			if S(s["fromNodeId"]) != S(s["pickupPointId"]) && !B(states["arrived:"+sid]) {
				return nil, Fail(409, "invalid_transition", "shipment has not arrived")
			}
		}
	}
	if typ == "arrive" && !B(states["dispatched:"+shID]) {
		return nil, Fail(409, "invalid_transition", "shipment not dispatched")
	}
	if typ == "release" && B(states["issued"]) {
		return nil, Fail(409, "invalid_transition", "issued inventory cannot be released")
	}
	changed := []string{}
	for _, a := range Arr(r["allocations"]) {
		if typ != "issue" && typ != "release" && S(a["shipmentId"]) != shID {
			continue
		}
		b, e := tx.Get("batches", S(a["batchId"]))
		if e != nil {
			return nil, e
		}
		qty := I(a["quantity"])
		var av, re, pa int64
		sid := S(a["shipmentId"])
		switch typ {
		case "pack":
			if B(states["dispatched:"+sid]) {
				return nil, Fail(409, "invalid_transition", "already dispatched")
			}
			re = -qty
			pa = qty
		case "dispatch":
			if B(states["dispatched:"+sid]) {
				continue
			}
			if !B(states["pack:"+sid+":"]) {
				return nil, Fail(409, "invalid_transition", "shipment not packed")
			}
			pa = -qty
		case "issue":
			if B(states["issued"]) {
				continue
			}
			if !B(states["dispatched:"+sid]) {
				if !B(states["pack:"+sid+":"]) {
					return nil, Fail(409, "invalid_transition", "shipment not packed")
				}
				pa = -qty
			}
		case "release":
			if B(states["dispatched:"+sid]) {
				return nil, Fail(409, "invalid_transition", "cannot release physical stock in transit")
			}
			if B(states["pack:"+sid+":"]) {
				pa = -qty
			} else {
				re = -qty
			}
			av = qty
		case "arrive":
			continue
		default:
			return nil, Fail(400, "invalid_event", "unknown inventory event")
		}
		if e = AdjustBatch(tx, b, av, re, pa, 0); e != nil {
			return nil, e
		}
		changed = append(changed, S(b["id"]))
	}
	if typ == "dispatch" {
		states["dispatched:"+shID] = true
	}
	if typ == "arrive" && S(c.Body["nodeId"]) == S(sh["pickupPointId"]) && S(sh["fromNodeId"]) != S(sh["pickupPointId"]) {
		if !B(states["arrived:"+shID]) {
			if e = pointOccupancy(tx, S(sh["pickupPointId"]), "inboundReservedSlots", -I(sh["slots"])); e != nil {
				return nil, e
			}
			if e = pointOccupancy(tx, S(sh["pickupPointId"]), "pickupUsedSlots", I(sh["slots"])); e != nil {
				return nil, e
			}
			states["arrived:"+shID] = true
		}
	}
	if typ == "issue" || typ == "release" {
		for _, s := range Arr(r["shipments"]) {
			sid := S(s["shipmentId"])
			if S(s["fromNodeId"]) == S(s["pickupPointId"]) {
				continue
			}
			field := "inboundReservedSlots"
			if B(states["arrived:"+sid]) {
				field = "pickupUsedSlots"
			}
			if e = pointOccupancy(tx, S(s["pickupPointId"]), field, -I(s["slots"])); e != nil {
				return nil, e
			}
		}
		states["issued"] = typ == "issue"
	}
	states[key] = true
	r["states"] = states
	if e = tx.Put("reservations", r); e != nil {
		return nil, e
	}
	return M{"applied": true, "changedBatchIds": changed}, nil
}

func applyStockMovement(tx *Tx, c Command) (M, error) {
	b := c.Body
	id := S(b["movementId"])
	typ := S(b["type"])
	marker := id + ":" + typ
	old, e := tx.Get("stock-movement-events", marker)
	if e == nil {
		a, _ := json.Marshal(old["_command"])
		body, _ := json.Marshal(b)
		if string(a) != string(body) {
			return nil, Fail(409, "idempotency_mismatch", "stock event already applied with different content")
		}
		return old, nil
	}
	if p, ok := e.(*Problem); !ok || p.Status != 404 {
		return nil, e
	}
	node := S(b["nodeId"])
	target := S(b["targetNodeId"])
	changed := []string{}
	items := Arr(b["items"])
	var slots int64
	for _, i := range items {
		p, e := tx.Get("products", S(i["productId"]))
		if e != nil {
			return nil, e
		}
		if I(i["quantity"]) <= 0 {
			return nil, Fail(400, "invalid_quantity", "movement quantity must be positive")
		}
		itemSlots, e := CheckedMul(I(i["quantity"]), I(p["slotsPerUnit"]))
		if e != nil {
			return nil, e
		}
		slots, e = CheckedAdd(slots, itemSlots)
		if e != nil {
			return nil, e
		}
	}
	if typ == "reserve" || typ == "receive_customer" {
		point := target
		if typ == "receive_customer" {
			point = node
		}
		if point != "" {
			if _, e = tx.Get("pickup-points", point); e == nil {
				cap, e := PointCapacity(tx, point)
				if e != nil {
					return nil, e
				}
				if slots > I(cap["availablePickupSlots"]) {
					return nil, capacityError(slots, I(cap["availablePickupSlots"]))
				}
				field := "inboundReservedSlots"
				if typ == "receive_customer" {
					field = "pickupUsedSlots"
				}
				if e = pointOccupancy(tx, point, field, slots); e != nil {
					return nil, e
				}
			}
		}
		if typ == "reserve" && S(b["kind"]) == "storage_transfer" {
			for _, i := range items {
				batch, e := tx.Get("batches", S(i["sourceBatchId"]))
				if e != nil {
					return nil, e
				}
				if e = AdjustBatch(tx, batch, -I(i["quantity"]), I(i["quantity"]), 0, 0); e != nil {
					return nil, e
				}
				changed = append(changed, S(batch["id"]))
			}
		}
	}
	if typ == "dispatch" {
		if S(b["kind"]) == "storage_transfer" {
			for _, i := range items {
				batch, e := tx.Get("batches", S(i["sourceBatchId"]))
				if e != nil {
					return nil, e
				}
				if e = AdjustBatch(tx, batch, 0, -I(i["quantity"]), 0, 0); e != nil {
					return nil, e
				}
				changed = append(changed, S(batch["id"]))
			}
		} else if _, e = tx.Get("pickup-points", node); e == nil {
			if e = pointOccupancy(tx, node, "pickupUsedSlots", -slots); e != nil {
				return nil, e
			}
		}
	}
	if typ == "arrive" {
		if _, e = tx.Get("pickup-points", node); e == nil {
			if e = pointOccupancy(tx, node, "inboundReservedSlots", -slots); e != nil {
				return nil, e
			}
			if e = pointOccupancy(tx, node, "pickupUsedSlots", slots); e != nil {
				return nil, e
			}
		}
	}
	if typ == "inspect" {
		if _, e = tx.Get("pickup-points", node); e == nil {
			cap, e := PointCapacity(tx, node)
			if e != nil {
				return nil, e
			}
			quota := I(cap["storageQuotaSlots"]) - I(cap["storageUsedSlots"])
			if slots > quota {
				return nil, capacityError(slots, max64(0, quota))
			}
		}
		for _, i := range items {
			q := I(i["quantity"])
			good, bad := I(i["resellableQuantity"]), I(i["quarantinedQuantity"])
			inspected, e := CheckedAdd(good, bad)
			if e != nil {
				return nil, e
			}
			if good < 0 || bad < 0 || inspected != q {
				return nil, Fail(422, "inspection_quantity_mismatch", "inspection quantities must sum to returned quantity")
			}
			source, e := tx.Get("batches", S(i["sourceBatchId"]))
			if e != nil {
				return nil, e
			}
			p, e := tx.Get("products", S(i["productId"]))
			if e != nil {
				return nil, e
			}
			itemSlots, e := CheckedMul(q, I(p["slotsPerUnit"]))
			if e != nil {
				return nil, e
			}
			batch := M{"id": ID(), "sellerId": source["sellerId"], "productId": i["productId"], "storagePointId": node, "receivedAt": b["occurredAt"], "receivedQuantity": q, "availableQuantity": good, "reservedQuantity": 0, "packedQuantity": 0, "quarantinedQuantity": bad, "usedSlots": itemSlots, "tariffVersionId": source["tariffVersionId"], "storageRateMinorPerSlotDay": source["storageRateMinorPerSlotDay"], "sourceBatchId": source["id"]}
			if e = tx.Put("batches", batch); e != nil {
				return nil, e
			}
			changed = append(changed, S(batch["id"]))
		}
		if _, e = tx.Get("pickup-points", node); e == nil {
			if e = pointOccupancy(tx, node, "pickupUsedSlots", -slots); e != nil {
				return nil, e
			}
		}
	}
	if typ == "release" {
		if _, e = tx.Get("pickup-points", target); e == nil {
			if e = pointOccupancy(tx, target, "inboundReservedSlots", -slots); e != nil {
				return nil, e
			}
		}
		if S(b["kind"]) == "storage_transfer" {
			for _, i := range items {
				batch, e := tx.Get("batches", S(i["sourceBatchId"]))
				if e != nil {
					return nil, e
				}
				if e = AdjustBatch(tx, batch, I(i["quantity"]), -I(i["quantity"]), 0, 0); e != nil {
					return nil, e
				}
			}
		}
	}
	if !Contains([]string{"reserve", "receive_customer", "dispatch", "arrive", "inspect", "release"}, typ) {
		return nil, Fail(400, "invalid_event", "unknown stock movement event")
	}
	result := M{"id": marker, "applied": true, "changedBatchIds": changed, "_command": Clone(b)}
	if e = tx.Put("stock-movement-events", result); e != nil {
		return nil, e
	}
	return result, nil
}

func inventorySum(values ...int64) (int64, error) {
	var sum int64
	for _, v := range values {
		var e error
		sum, e = CheckedAdd(sum, v)
		if e != nil {
			return 0, e
		}
	}
	return sum, nil
}
