package core

import (
	"sort"
	"time"
)

func init() { Register(handleExceptions) }
func exceptionAccess(c Command, m M) error {
	if c.Actor.Has("dispatcher") {
		return nil
	}
	for _, k := range []string{"pointId", "returnPointId", "inspectionNodeId", "storagePointId"} {
		if Contains(c.Actor.PointIDs, S(m[k])) {
			return nil
		}
	}
	if c.Actor.Has("seller") && Contains(c.Actor.SellerIDs, S(m["sellerId"])) {
		return nil
	}
	return Fail(403, "forbidden", "resource outside caller scope")
}
func exceptionOrderAccess(c Command, order M) error {
	if c.Actor.Has("dispatcher") {
		return nil
	}
	if c.Actor.Point(S(order["pickupPointId"])) {
		return nil
	}
	for _, s := range Arr(order["shipments"]) {
		if c.Actor.Point(S(s["fromNodeId"])) || c.Actor.Point(S(s["lastKnownNodeId"])) {
			return nil
		}
	}
	return Fail(403, "forbidden", "order outside caller points")
}
func notice(tx *Tx, kind, resourceType, resourceID, message string, scope M) error {
	n := M{"id": ID(), "type": kind, "resourceType": resourceType, "resourceId": resourceID, "createdAt": Stamp(tx.Now()), "message": message}
	for k, v := range scope {
		n[k] = v
	}
	return tx.Put("notifications", n)
}
func handleExceptions(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "createReturn":
		m, e := createReturn(tx, c)
		return m, true, e
	case "recordReturnEvent":
		m, e := returnEvent(tx, c)
		return m, true, e
	case "createIncident":
		m, e := createIncident(tx, c)
		return m, true, e
	case "rerouteIncident":
		m, e := rerouteIncident(tx, c)
		return m, true, e
	case "decideStorageCase":
		m, e := decideStorageCase(tx, c)
		return m, true, e
	case "recordStorageTransferEvent":
		m, e := storageTransferEvent(tx, c)
		return m, true, e
	case "svcScanLongStorage":
		at := Time(c.Body["asOf"])
		if at.After(tx.Now()) {
			return nil, true, Fail(409, "clock_conflict", "scan cannot run beyond model clock")
		}
		m, e := scanLongStorage(tx, at)
		return m, true, e
	case "svcListSimulatedExceptionActions":
		m, e := simulatedExceptionActions(tx, c)
		return m, true, e
	case "readNotification":
		m, e := tx.Get("notifications", c.Params["notificationId"])
		if e != nil {
			return nil, true, e
		}
		if e = exceptionAccess(c, m); e != nil {
			return nil, true, e
		}
		if S(m["readAt"]) == "" {
			m["readAt"] = Stamp(tx.Now())
			e = tx.Put("notifications", m)
		}
		return m, true, e
	case "listReturns", "listIncidents", "listStorageCases", "listNotifications", "getReturn", "getIncident", "getStorageCase":
		m, e := readException(tx, c)
		return m, true, e
	}
	return nil, false, nil
}
func readException(tx *Tx, c Command) (M, error) {
	kind, id := "", ""
	switch c.Name {
	case "listReturns", "getReturn":
		kind = "returns"
		id = c.Params["returnId"]
	case "listIncidents", "getIncident":
		kind = "incidents"
		id = c.Params["incidentId"]
	case "listStorageCases", "getStorageCase":
		kind = "storage-cases"
		id = c.Params["caseId"]
	case "listNotifications":
		kind = "notifications"
	}
	visible := func(m M) error {
		if kind == "incidents" {
			o, e := tx.Get("orders", S(m["orderId"]))
			if e != nil {
				return e
			}
			return exceptionOrderAccess(c, o)
		}
		return exceptionAccess(c, m)
	}
	if id != "" {
		m, e := tx.Get(kind, id)
		if e != nil {
			return nil, e
		}
		return m, visible(m)
	}
	filters := M{}
	for _, k := range []string{"orderId", "pointId", "sellerId", "status"} {
		if s := c.Query.Get(k); s != "" {
			if kind == "returns" && k == "pointId" {
				filters["returnPointId"] = s
			} else {
				filters[k] = s
			}
		}
	}
	rows, e := tx.List(kind, filters)
	if e != nil {
		return nil, e
	}
	out := []M{}
	for _, m := range rows {
		if visible(m) != nil {
			continue
		}
		if c.Query.Get("unread") == "true" && S(m["readAt"]) != "" {
			continue
		}
		out = append(out, m)
	}
	return Page(out), nil
}
func createReturn(tx *Tx, c Command) (M, error) {
	o, e := tx.Get("orders", S(c.Body["orderId"]))
	if e != nil {
		return nil, e
	}
	if e = exceptionOrderAccess(c, o); e != nil {
		return nil, e
	}
	if S(o["status"]) != "issued" {
		return nil, Fail(409, "order_not_issued", "return requires issued order")
	}
	point := S(c.Body["returnPointId"])
	p, e := tx.Get("pickup-points", point)
	if e != nil {
		return nil, e
	}
	if !B(p["acceptsReturns"]) {
		return nil, Fail(409, "returns_not_accepted", "point does not accept returns")
	}
	if !c.Actor.Point(point) {
		return nil, Fail(403, "forbidden", "dropoff point outside scope")
	}
	lines := Arr(c.Body["lines"])
	issued := map[string]M{}
	for _, l := range Arr(o["lines"]) {
		issued[S(l["id"])] = l
	}
	used := map[string]int64{}
	old, e := tx.List("returns", M{"orderId": o["id"]})
	if e != nil {
		return nil, e
	}
	for _, r := range old {
		if S(r["status"]) == "canceled" {
			continue
		}
		for _, l := range Arr(r["lines"]) {
			key := S(l["orderItemId"])
			used[key], e = CheckedAdd(used[key], I(l["quantity"]))
			if e != nil {
				return nil, e
			}
		}
	}
	items := []M{}
	seen := map[string]bool{}
	slots := int64(0)
	for _, l := range lines {
		id := S(l["orderItemId"])
		source, ok := issued[id]
		if !ok || seen[id] || I(l["quantity"]) <= 0 {
			return nil, Fail(422, "invalid_return_line", "unknown, duplicate or invalid return line")
		}
		seen[id] = true
		requested, e := CheckedAdd(used[id], I(l["quantity"]))
		if e != nil {
			return nil, e
		}
		if requested > I(source["quantity"]) {
			return nil, Fail(409, "return_quantity_exceeded", "previous and open returns exceed issued quantity")
		}
		prod, e := tx.Get("products", S(source["productId"]))
		if e != nil {
			return nil, e
		}
		lineSlots, e := CheckedMul(I(prod["slotsPerUnit"]), I(l["quantity"]))
		if e != nil {
			return nil, e
		}
		slots, e = CheckedAdd(slots, lineSlots)
		if e != nil {
			return nil, e
		}
		items = append(items, M{"productId": source["productId"], "quantity": l["quantity"]})
	}
	if len(items) == 0 {
		return nil, Fail(422, "invalid_return_line", "return lines required")
	}
	inspection := point
	node, e := tx.Get("nodes", point)
	if e == nil && S(node["kind"]) == "pickup_only" {
		nodes, e := tx.List("nodes", nil)
		if e != nil {
			return nil, e
		}
		inspection = ""
		for _, n := range nodes {
			if S(n["kind"]) == "hub" && B(n["active"]) {
				inspection = S(n["id"])
				break
			}
		}
		if inspection == "" {
			return nil, Fail(409, "route_unavailable", "no inspection hub")
		}
	}
	tariff, e := EffectiveTariff(tx, tx.Now())
	if e != nil {
		return nil, e
	}
	id := ID()
	route, e := Dispatch(tx, Command{Name: "svcQuoteMovement", Body: M{"kind": "return", "subjectId": id, "fromNodeId": point, "toNodeId": inspection, "items": items, "notBefore": Stamp(tx.Now()), "tariffVersionId": tariff["id"]}, Actor: c.Actor, RunID: c.RunID})
	if e != nil {
		return nil, e
	}
	options := Arr(route["options"])
	if len(options) == 0 {
		return nil, Fail(409, "route_unavailable", "no return route")
	}
	hold, e := HoldRoute(tx, M{"quoteId": route["id"], "optionId": options[0]["id"], "subjectType": "return", "subjectId": id}, false)
	if e != nil {
		return nil, e
	}
	cap, e := PointCapacity(tx, point)
	if e != nil {
		return nil, e
	}
	if I(cap["availablePickupSlots"]) < slots {
		return nil, Fail(409, "capacity_exceeded", "return point lacks incoming space")
	}
	if e = tx.Put("capacity-holds", M{"id": "return_" + id, "pointId": point, "slots": slots, "state": "held", "resourceId": id}); e != nil {
		return nil, e
	}
	r := M{"id": id, "orderId": o["id"], "returnPointId": point, "inspectionNodeId": inspection, "status": "awaiting_dropoff", "version": 1, "lines": lines, "route": route, "selectedOptionId": options[0]["id"], "settlementStatus": "not_due", "createdAt": Stamp(tx.Now()), "dropoffBy": Stamp(tx.Now().Add(7 * 24 * time.Hour)), "reason": c.Body["reason"], "movementIndex": 0, "movementActive": false, "items": items, "slots": slots, "tariffVersionId": tariff["id"], "routeHoldId": hold["id"]}
	return r, tx.Put("returns", r)
}
func selectedMovement(m M) M {
	for _, o := range Arr(Obj(m["route"])["options"]) {
		if S(o["id"]) == S(m["selectedOptionId"]) {
			return o
		}
	}
	return M{}
}
func movementSegments(m M) []M {
	out := []M{}
	for _, sh := range Arr(selectedMovement(m)["shipments"]) {
		for _, seg := range Arr(sh["segments"]) {
			if S(seg["fromNodeId"]) != S(seg["toNodeId"]) {
				out = append(out, seg)
			}
		}
	}
	return out
}
func exceptionSimulation(tx *Tx, c Command, m M) (func(), error) {
	restore := func() {}
	if c.SimulationID == "" {
		return restore, nil
	}
	if c.Actor.Service != "demo" {
		return restore, Fail(403, "simulation_required", "only demo may execute simulation actions")
	}
	segments := movementSegments(m)
	index := int(I(m["movementIndex"]))
	if index >= len(segments) {
		return restore, Fail(409, "invalid_transition", "no pending movement")
	}
	segment := segments[index]
	typ := "handed_over"
	at := Time(segment["scheduledStartAt"])
	if B(m["movementActive"]) {
		typ = "arrived"
		at = Time(segment["scheduledEndAt"])
	}
	expected := S(m["id"]) + "_" + S(segment["id"]) + "_" + typ
	if S(segment["mode"]) != "simulated" || c.SimulationID != expected || S(c.Body["type"]) != typ || S(c.Body["segmentId"]) != S(segment["id"]) {
		return restore, Fail(409, "invalid_simulation_action", "simulation identity does not match pending action")
	}
	if at.After(tx.Now()) {
		return restore, Fail(409, "not_due", "simulation action has not arrived")
	}
	clock := tx.Clock
	tx.Clock = at
	return func() { tx.Clock = clock }, nil
}
func movementStep(tx *Tx, c Command, m M) (bool, error) {
	segs := movementSegments(m)
	idx := int(I(m["movementIndex"]))
	if idx >= len(segs) {
		return false, Fail(409, "invalid_transition", "movement already arrived")
	}
	seg := segs[idx]
	if S(c.Body["segmentId"]) != S(seg["id"]) {
		return false, Fail(409, "invalid_segment", "segment is not current movement stage")
	}
	simulated := S(seg["mode"]) == "simulated"
	if simulated && c.SimulationID == "" {
		return false, Fail(403, "simulation_required", "simulated stages require action identity")
	}
	at := Time(seg["scheduledStartAt"])
	if S(c.Body["type"]) == "arrived" {
		at = Time(seg["scheduledEndAt"])
	}
	if tx.Now().Before(at) {
		return false, Fail(409, "not_due", "movement time has not arrived")
	}
	if S(c.Body["type"]) == "handed_over" {
		if B(m["movementActive"]) {
			return false, Fail(409, "invalid_transition", "segment already handed over")
		}
		if !simulated && !c.Actor.Point(S(seg["fromNodeId"])) {
			return false, Fail(403, "forbidden", "handoff outside point scope")
		}
		m["movementActive"] = true
		m["status"] = "in_transit"
		return false, nil
	}
	if !B(m["movementActive"]) {
		return false, Fail(409, "invalid_transition", "arrival requires handoff")
	}
	if !simulated && !c.Actor.Point(S(seg["toNodeId"])) {
		return false, Fail(403, "forbidden", "arrival outside point scope")
	}
	if S(m["routeHoldId"]) != "" {
		if _, e := UpdateRouteHold(tx, S(m["routeHoldId"]), M{"type": "segment_completed", "segmentId": seg["id"]}); e != nil {
			return false, e
		}
	}
	m["movementIndex"] = idx + 1
	m["movementActive"] = false
	return idx+1 == len(segs), nil
}
func returnEvent(tx *Tx, c Command) (M, error) {
	r, e := tx.Get("returns", c.Params["returnId"])
	if e != nil {
		return nil, e
	}
	if e = exceptionAccess(c, r); e != nil {
		return nil, e
	}
	if e = Version(r, c.Body); e != nil {
		return nil, e
	}
	restore, e := exceptionSimulation(tx, c, r)
	if e != nil {
		return nil, e
	}
	defer restore()
	typ := S(c.Body["type"])
	status := S(r["status"])
	switch typ {
	case "received_from_customer":
		if status != "awaiting_dropoff" {
			return nil, Fail(409, "invalid_transition", "return already received or closed")
		}
		if tx.Now().After(Time(r["dropoffBy"])) {
			return nil, Fail(409, "dropoff_expired", "dropoff deadline passed")
		}
		if !c.Actor.Point(S(r["returnPointId"])) {
			return nil, Fail(403, "forbidden", "dropoff outside scope")
		}
		r["status"] = "at_return_point"
		if S(r["routeHoldId"]) != "" {
			if _, e = UpdateRouteHold(tx, S(r["routeHoldId"]), M{"type": "confirm"}); e != nil {
				return nil, e
			}
		}
		if len(movementSegments(r)) == 0 {
			r["status"] = "awaiting_inspection"
		}
	case "handed_over", "arrived":
		if status != "at_return_point" && status != "in_transit" {
			return nil, Fail(409, "invalid_transition", "return not ready for movement")
		}
		finished, e := movementStep(tx, c, r)
		if e != nil {
			return nil, e
		}
		if typ == "handed_over" {
			if e = tx.Delete("capacity-holds", "return_"+S(r["id"])); e != nil {
				return nil, e
			}
		}
		if finished {
			r["status"] = "awaiting_inspection"
		} else if typ == "arrived" {
			r["status"] = "at_return_point"
		}
	case "inspected":
		if status != "awaiting_inspection" {
			return nil, Fail(409, "invalid_transition", "return not at inspection node")
		}
		if !c.Actor.Point(S(r["inspectionNodeId"])) && c.SimulationID == "" {
			return nil, Fail(403, "forbidden", "inspection outside scope")
		}
		if e = inspectReturn(tx, c, r); e != nil {
			return nil, e
		}
		r["status"] = "closed"
		r["closedAt"] = Stamp(tx.Now())
		r["settlementStatus"] = "posted"
		if e = tx.Delete("capacity-holds", "return_"+S(r["id"])); e != nil {
			return nil, e
		}
	case "canceled":
		if status != "awaiting_dropoff" {
			return nil, Fail(409, "invalid_transition", "received return cannot be canceled")
		}
		r["status"] = "canceled"
		if S(r["routeHoldId"]) != "" {
			if _, e = UpdateRouteHold(tx, S(r["routeHoldId"]), M{"type": "release_remaining"}); e != nil {
				return nil, e
			}
		}
		if e = tx.Delete("capacity-holds", "return_"+S(r["id"])); e != nil {
			return nil, e
		}
	default:
		return nil, Fail(422, "invalid_return_event", "unknown return event")
	}
	r["version"] = I(r["version"]) + 1
	return r, tx.Put("returns", r)
}
func inspectReturn(tx *Tx, c Command, r M) error {
	order, e := tx.Get("orders", S(r["orderId"]))
	if e != nil {
		return e
	}
	byItem := map[string]M{}
	for _, l := range Arr(order["lines"]) {
		byItem[S(l["id"])] = l
	}
	wanted := map[string]int64{}
	for _, l := range Arr(r["lines"]) {
		wanted[S(l["orderItemId"])] = I(l["quantity"])
	}
	inspected := Arr(c.Body["lines"])
	if len(inspected) != len(wanted) {
		return Fail(422, "inspection_mismatch", "inspect every returned line exactly once")
	}
	seen := map[string]bool{}
	for _, l := range inspected {
		id := S(l["orderItemId"])
		good, bad := I(l["resellableQuantity"]), I(l["quarantinedQuantity"])
		quantity, e := CheckedAdd(good, bad)
		if e != nil {
			return e
		}
		if seen[id] || good < 0 || bad < 0 || quantity != wanted[id] || wanted[id] == 0 {
			return Fail(422, "inspection_mismatch", "inspection quantities differ from returned line")
		}
		seen[id] = true
		line := byItem[id]
		product, e := tx.Get("products", S(line["productId"]))
		if e != nil {
			return e
		}
		slots, e := CheckedMul(quantity, I(product["slotsPerUnit"]))
		if e != nil {
			return e
		}
		batch := M{"id": ID(), "sellerId": line["sellerId"], "productId": line["productId"], "storagePointId": r["inspectionNodeId"], "receivedAt": Stamp(tx.Now()), "receivedQuantity": quantity, "availableQuantity": good, "reservedQuantity": 0, "packedQuantity": 0, "quarantinedQuantity": bad, "usedSlots": slots, "tariffVersionId": r["tariffVersionId"], "storageRateMinorPerSlotDay": 0, "returnId": r["id"]}
		tariff, e := tx.Get("tariffs", S(r["tariffVersionId"]))
		if e != nil {
			return e
		}
		batch["storageRateMinorPerSlotDay"] = Rate(tariff, "storage_slot_day", S(r["inspectionNodeId"]))
		if e = tx.Put("batches", batch); e != nil {
			return e
		}
	}
	_, e = ApplyPosting(tx, "return_"+S(r["id"]), M{"kind": "return_inspected", "occurredAt": Stamp(tx.Now()), "tariffVersionId": r["tariffVersionId"], "orderId": r["orderId"], "returnId": r["id"], "lines": inspected, "operatingFacts": []M{}})
	return e
}
func createIncident(tx *Tx, c Command) (M, error) {
	o, e := tx.Get("orders", S(c.Body["orderId"]))
	if e != nil {
		return nil, e
	}
	if e = exceptionOrderAccess(c, o); e != nil {
		return nil, e
	}
	if S(o["status"]) == "issued" || S(o["status"]) == "canceled" {
		return nil, Fail(409, "invalid_transition", "finished order cannot be disrupted")
	}
	shipment := M{}
	for _, sh := range Arr(o["shipments"]) {
		if S(sh["id"]) == S(c.Body["shipmentId"]) {
			shipment = sh
		}
	}
	if len(shipment) == 0 {
		return nil, Fail(422, "invalid_shipment", "shipment does not belong to order")
	}
	failure := Obj(c.Body["failureTarget"])
	if len(failure) == 0 {
		return nil, Fail(422, "invalid_failure_target", "failure target required")
	}
	typ := S(c.Body["type"])
	field := map[string]string{"node_unavailable": "nodeId", "trip_disrupted": "tripId", "segment_failed": "segmentId"}[typ]
	if field == "" || S(failure[field]) == "" {
		return nil, Fail(422, "invalid_failure_target", "target must match incident type")
	}
	id := ID()
	incident := Clone(c.Body)
	incident["id"] = id
	incident["status"] = "open"
	incident["createdAt"] = Stamp(tx.Now())
	incident["updatedAt"] = Stamp(tx.Now())
	incident["oldRouteRevision"] = o["routeRevision"]
	_, e = Dispatch(tx, Command{Name: "svcBlockOrderSegment", Params: map[string]string{"orderId": S(o["id"]), "incidentId": id}, Body: M{"shipmentId": shipment["id"], "expectedRouteRevision": o["routeRevision"], "failureTarget": failure}, Actor: c.Actor, RunID: c.RunID})
	if e != nil {
		return nil, e
	}
	return incident, tx.Put("incidents", incident)
}
func rerouteIncident(tx *Tx, c Command) (M, error) {
	incident, e := tx.Get("incidents", c.Params["incidentId"])
	if e != nil {
		return nil, e
	}
	if !c.Actor.Has("dispatcher") {
		return nil, Fail(403, "forbidden", "reroute requires dispatcher")
	}
	if S(incident["status"]) == "resolved" {
		return nil, Fail(409, "invalid_transition", "incident already resolved")
	}
	o, e := Dispatch(tx, Command{Name: "svcReplaceOrderRoute", Params: map[string]string{"orderId": S(incident["orderId"])}, Body: M{"incidentId": incident["id"], "shipmentId": incident["shipmentId"], "failureTarget": incident["failureTarget"], "expectedVersion": c.Body["expectedOrderVersion"]}, Actor: c.Actor, RunID: c.RunID})
	if e != nil {
		if p, ok := e.(*Problem); ok && (p.Code == "route_unavailable" || p.Code == "no_route" || p.Code == "route_blocked") {
			incident["status"] = "blocked"
			incident["blockage"] = M{"type": "about:blank", "title": p.Code, "status": p.Status, "code": p.Code, "detail": p.Detail}
			incident["updatedAt"] = Stamp(tx.Now())
			if e = tx.Put("incidents", incident); e != nil {
				return nil, e
			}
			if e = notice(tx, "reroute_blocked", "incident", S(incident["id"]), "No feasible remaining route", M{"pointId": S(o["pickupPointId"])}); e != nil {
				return nil, e
			}
			return incident, nil
		}
		return nil, e
	}
	incident["status"] = "resolved"
	incident["newRouteRevision"] = o["routeRevision"]
	incident["updatedAt"] = Stamp(tx.Now())
	if e = tx.Put("incidents", incident); e != nil {
		return nil, e
	}
	if e = notice(tx, "route_changed", "order", S(incident["orderId"]), "Remaining route recalculated", M{"pointId": o["pickupPointId"]}); e != nil {
		return nil, e
	}
	return incident, nil
}
func scanLongStorage(tx *Tx, asOf time.Time) (M, error) {
	oldClock := tx.Clock
	tx.Clock = asOf
	defer func() { tx.Clock = oldClock }()
	before, e := tx.List("postings", nil)
	if e != nil {
		return nil, e
	}
	if e = AccrueStorage(tx); e != nil {
		return nil, e
	}
	after, e := tx.List("postings", nil)
	if e != nil {
		return nil, e
	}
	batches, e := tx.List("batches", nil)
	if e != nil {
		return nil, e
	}
	changed := int64(0)
	for _, batch := range batches {
		tariff, e := tx.Get("tariffs", S(batch["tariffVersionId"]))
		if e != nil {
			return nil, e
		}
		age := int64(asOf.Sub(Time(batch["receivedAt"])) / 24 / time.Hour)
		threshold := I(tariff["storageThresholdDays"])
		if age < threshold || I(batch["availableQuantity"]) == 0 {
			continue
		}
		id := "storage_" + S(batch["id"])
		existing, e := tx.Get("storage-cases", id)
		if e != nil {
			if p, ok := e.(*Problem); !ok || p.Status != 404 {
				return nil, e
			}
		}
		if e == nil {
			status := S(existing["status"])
			if status == "closed" || status == "transfer_planned" || status == "in_transit" {
				continue
			}
			if status == "deferred" && asOf.Before(Time(existing["nextCheckAt"])) {
				continue
			}
			if S(existing["lastCheckedAt"]) == Stamp(asOf) {
				continue
			}
		}
		cost, e := StorageCostForBatch(tx, batch, asOf)
		if e != nil {
			return nil, e
		}
		m := M{"id": id, "batchId": batch["id"], "pointId": batch["storagePointId"], "sellerId": batch["sellerId"], "status": "open", "version": 1, "detectedAt": Stamp(asOf), "thresholdDays": threshold, "ageDays": age, "availableQuantity": batch["availableQuantity"], "cost": cost, "recommendation": "transfer_to_hub", "reason": "Storage threshold reached; compare further holding with transfer", "lastCheckedAt": Stamp(asOf)}
		if existing != nil {
			m["version"] = I(existing["version"]) + 1
			m["detectedAt"] = existing["detectedAt"]
		}
		if e = tx.Put("storage-cases", m); e != nil {
			return nil, e
		}
		changed++
		if existing == nil || S(existing["status"]) == "deferred" {
			if e = notice(tx, "long_storage", "storage_case", id, "Batch exceeded storage threshold", M{"pointId": m["pointId"], "sellerId": m["sellerId"]}); e != nil {
				return nil, e
			}
		}
	}
	return M{"asOf": Stamp(asOf), "checkedBatches": len(batches), "changedCases": changed, "postedCharges": len(after) - len(before)}, nil
}
func decideStorageCase(tx *Tx, c Command) (M, error) {
	if !c.Actor.Has("dispatcher") {
		return nil, Fail(403, "forbidden", "storage decisions require dispatcher")
	}
	m, e := tx.Get("storage-cases", c.Params["caseId"])
	if e != nil {
		return nil, e
	}
	if e = Version(m, c.Body); e != nil {
		return nil, e
	}
	status := S(m["status"])
	if status != "open" && status != "deferred" {
		return nil, Fail(409, "invalid_transition", "case cannot accept decision")
	}
	switch S(c.Body["action"]) {
	case "keep":
		at := Time(c.Body["recheckAt"])
		if !at.After(tx.Now()) {
			return nil, Fail(422, "invalid_recheck_time", "recheckAt must be in the future")
		}
		m["status"] = "deferred"
		m["nextCheckAt"] = Stamp(at)
	case "transfer_to_hub":
		b, e := tx.Get("batches", S(m["batchId"]))
		if e != nil {
			return nil, e
		}
		qty := I(b["availableQuantity"])
		if qty <= 0 {
			return nil, Fail(409, "insufficient_stock", "no available units to transfer")
		}
		nodes, e := tx.List("nodes", nil)
		if e != nil {
			return nil, e
		}
		var route M
		target := ""
		for _, n := range nodes {
			if S(n["kind"]) != "hub" || !B(n["active"]) {
				continue
			}
			r, e := Dispatch(tx, Command{Name: "svcQuoteMovement", Body: M{"kind": "storage_transfer", "subjectId": m["id"], "fromNodeId": m["pointId"], "toNodeId": n["id"], "items": []M{{"productId": b["productId"], "quantity": qty}}, "notBefore": Stamp(tx.Now()), "tariffVersionId": b["tariffVersionId"]}, Actor: c.Actor, RunID: c.RunID})
			if e != nil {
				return nil, e
			}
			if len(Arr(r["options"])) > 0 {
				route = r
				target = S(n["id"])
				break
			}
		}
		if target == "" {
			return nil, Fail(409, "route_unavailable", "no feasible transfer to hub")
		}
		hold, e := HoldRoute(tx, M{"quoteId": route["id"], "optionId": Arr(route["options"])[0]["id"], "subjectType": "storage_case", "subjectId": m["id"]}, true)
		if e != nil {
			return nil, e
		}
		m["routeHoldId"] = hold["id"]
		if e = AdjustBatch(tx, b, -qty, qty, 0, 0); e != nil {
			return nil, e
		}
		m["status"] = "transfer_planned"
		m["route"] = route
		m["selectedOptionId"] = Arr(route["options"])[0]["id"]
		m["transferQuantity"] = qty
		m["hubId"] = target
		m["movementIndex"] = 0
		m["movementActive"] = false
		m["tariffVersionId"] = b["tariffVersionId"]
		m["productId"] = b["productId"]
	default:
		return nil, Fail(422, "invalid_storage_decision", "unknown decision")
	}
	m["decision"] = Clone(c.Body)
	m["version"] = I(m["version"]) + 1
	m["lastCheckedAt"] = Stamp(tx.Now())
	if e = tx.Put("storage-cases", m); e != nil {
		return nil, e
	}
	if e = notice(tx, "storage_action", "storage_case", S(m["id"]), "Storage decision applied", M{"pointId": m["pointId"], "sellerId": m["sellerId"]}); e != nil {
		return nil, e
	}
	return m, nil
}
func storageTransferEvent(tx *Tx, c Command) (M, error) {
	m, e := tx.Get("storage-cases", c.Params["caseId"])
	if e != nil {
		return nil, e
	}
	if e = exceptionAccess(c, m); e != nil {
		return nil, e
	}
	if e = Version(m, c.Body); e != nil {
		return nil, e
	}
	restore, e := exceptionSimulation(tx, c, m)
	if e != nil {
		return nil, e
	}
	defer restore()
	if S(m["status"]) != "transfer_planned" && S(m["status"]) != "in_transit" {
		return nil, Fail(409, "invalid_transition", "storage transfer is not planned")
	}
	first := I(m["movementIndex"]) == 0 && !B(m["movementActive"])
	finished, e := movementStep(tx, c, m)
	if e != nil {
		return nil, e
	}
	if S(c.Body["type"]) == "handed_over" && first {
		b, e := tx.Get("batches", S(m["batchId"]))
		if e != nil {
			return nil, e
		}
		if e = AdjustBatch(tx, b, 0, -I(m["transferQuantity"]), 0, 0); e != nil {
			return nil, e
		}
	}
	if finished {
		m["status"] = "closed"
		p, e := tx.Get("products", S(m["productId"]))
		if e != nil {
			return nil, e
		}
		slots, e := CheckedMul(I(m["transferQuantity"]), I(p["slotsPerUnit"]))
		if e != nil {
			return nil, e
		}
		b := M{"id": ID(), "sellerId": m["sellerId"], "productId": m["productId"], "storagePointId": m["hubId"], "receivedAt": Stamp(tx.Now()), "receivedQuantity": m["transferQuantity"], "availableQuantity": m["transferQuantity"], "reservedQuantity": 0, "packedQuantity": 0, "quarantinedQuantity": 0, "usedSlots": slots, "tariffVersionId": m["tariffVersionId"], "storageRateMinorPerSlotDay": 0, "sourceBatchId": m["batchId"]}
		if e = tx.Put("batches", b); e != nil {
			return nil, e
		}
		tariff, e := tx.Get("tariffs", S(m["tariffVersionId"]))
		if e != nil {
			return nil, e
		}
		facts, e := transportFacts(movementSegments(m), tariff)
		if e != nil {
			return nil, e
		}
		if _, e = ApplyPosting(tx, "transfer_"+S(m["id"]), M{"kind": "logistics_cost", "occurredAt": Stamp(tx.Now()), "tariffVersionId": m["tariffVersionId"], "storageCaseId": m["id"], "operatingFacts": facts}); e != nil {
			return nil, e
		}
	}
	m["version"] = I(m["version"]) + 1
	return m, tx.Put("storage-cases", m)
}
func simulatedExceptionActions(tx *Tx, c Command) (M, error) {
	until := Time(c.Query.Get("until"))
	if until.IsZero() {
		until = tx.Now()
	}
	actions := []M{}
	for _, kind := range []string{"returns", "storage-cases"} {
		rows, e := tx.List(kind, nil)
		if e != nil {
			return nil, e
		}
		for _, m := range rows {
			status := S(m["status"])
			if kind == "returns" && (status == "closed" || status == "canceled" || status == "awaiting_dropoff" || status == "awaiting_inspection") {
				continue
			}
			if kind == "storage-cases" && status != "transfer_planned" && status != "in_transit" {
				continue
			}
			segs := movementSegments(m)
			idx := int(I(m["movementIndex"]))
			if idx >= len(segs) {
				continue
			}
			seg := segs[idx]
			if S(seg["mode"]) != "simulated" {
				continue
			}
			event := "handed_over"
			at := Time(seg["scheduledStartAt"])
			if B(m["movementActive"]) {
				event = "arrived"
				at = Time(seg["scheduledEndAt"])
			}
			if at.After(until) {
				continue
			}
			a := M{"id": S(m["id"]) + "_" + S(seg["id"]) + "_" + event, "dueAt": Stamp(at)}
			cmd := M{"type": event, "segmentId": seg["id"], "expectedVersion": m["version"]}
			if kind == "returns" {
				a["returnId"] = m["id"]
				a["returnCommand"] = cmd
			} else {
				a["caseId"] = m["id"]
				a["transferCommand"] = cmd
			}
			actions = append(actions, a)
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		a, b := S(actions[i]["dueAt"]), S(actions[j]["dueAt"])
		if a == b {
			return S(actions[i]["id"]) < S(actions[j]["id"])
		}
		return a < b
	})
	return Page(actions), nil
}

// AdvanceExceptions expires undelivered return requests and performs the same scheduled transitions as the internal API.
func AdvanceExceptions(tx *Tx) error {
	returns, e := tx.List("returns", nil)
	if e != nil {
		return e
	}
	for _, r := range returns {
		if S(r["status"]) == "awaiting_dropoff" && !tx.Now().Before(Time(r["dropoffBy"])) {
			r["status"] = "canceled"
			if S(r["routeHoldId"]) != "" {
				if _, e = UpdateRouteHold(tx, S(r["routeHoldId"]), M{"type": "release_remaining"}); e != nil {
					return e
				}
			}
			r["version"] = I(r["version"]) + 1
			if e = tx.Put("returns", r); e != nil {
				return e
			}
			if e = tx.Delete("capacity-holds", "return_"+S(r["id"])); e != nil {
				return e
			}
		}
	}
	for {
		page, e := simulatedExceptionActions(tx, Command{})
		if e != nil {
			return e
		}
		actions := Arr(page["items"])
		if len(actions) == 0 {
			break
		}
		for _, a := range actions {
			cmd := Command{Actor: Actor{Subject: "demo", Service: "demo", Roles: []string{"dispatcher"}}, RunID: S(tx.Run["id"]), SimulationID: S(a["id"])}
			if S(a["returnId"]) != "" {
				cmd.Name = "recordReturnEvent"
				cmd.Params = map[string]string{"returnId": S(a["returnId"])}
				cmd.Body = Obj(a["returnCommand"])
			} else {
				cmd.Name = "recordStorageTransferEvent"
				cmd.Params = map[string]string{"caseId": S(a["caseId"])}
				cmd.Body = Obj(a["transferCommand"])
			}
			if _, e = Dispatch(tx, cmd); e != nil {
				return e
			}
		}
	}
	_, e = scanLongStorage(tx, tx.Now())
	return e
}
