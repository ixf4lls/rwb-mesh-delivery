package core

import (
	"fmt"
	"sort"
	"time"
)

func init() { Register(ordersHandler) }
func ordersHandler(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "createOrder":
		m, e := CreateOrder(tx, c)
		return m, true, e
	case "getOrder":
		m, e := tx.Get("orders", c.Params["orderId"])
		if e == nil {
			m, e = visibleOrder(c, m)
		}
		return m, true, e
	case "listOrders":
		rows, e := tx.List("orders", nil)
		if e != nil {
			return nil, true, e
		}
		out := []M{}
		for _, o := range rows {
			if q := c.Query.Get("status"); q != "" && S(o["status"]) != q {
				continue
			}
			if point := c.Query.Get("pointId"); point != "" {
				found := S(o["pickupPointId"]) == point
				for _, sh := range Arr(o["shipments"]) {
					found = found || S(sh["fromNodeId"]) == point || S(sh["lastKnownNodeId"]) == point
				}
				if !found {
					continue
				}
			}
			if seller := c.Query.Get("sellerId"); seller != "" {
				found := false
				for _, line := range Arr(o["lines"]) {
					found = found || S(line["sellerId"]) == seller
				}
				if !found {
					continue
				}
			}
			m, e := visibleOrder(c, o)
			if e == nil {
				out = append(out, m)
			}
		}
		return Page(out), true, nil
	case "getOrderRoute":
		order, e := tx.Get("orders", c.Params["orderId"])
		if e != nil {
			return nil, true, e
		}
		visible, e := visibleOrder(c, order)
		if e != nil {
			return nil, true, e
		}
		revision := I(order["routeRevision"])
		if q := c.Query.Get("revision"); q != "" {
			revision = I(q)
		}
		r, e := tx.Get("routes", routeID(S(order["id"]), revision))
		if e == nil && c.Actor.Has("seller") && !c.Actor.Has("dispatcher") {
			r = Clone(r)
			allowed := map[string]bool{}
			for _, s := range Arr(visible["shipments"]) {
				allowed[S(s["id"])] = true
			}
			ship := []M{}
			products := map[string]bool{}
			for _, line := range Arr(visible["lines"]) {
				products[S(line["productId"])] = true
			}
			for _, s := range Arr(r["shipments"]) {
				if allowed[S(s["id"])] {
					contents := []M{}
					for _, item := range Arr(s["contents"]) {
						if products[S(item["productId"])] {
							contents = append(contents, item)
						}
					}
					s["contents"] = contents
					ship = append(ship, s)
				}
			}
			r["shipments"] = ship
		}
		return r, true, e
	case "listOrderEvents":
		o, e := tx.Get("orders", c.Params["orderId"])
		if e != nil {
			return nil, true, e
		}
		if _, e = visibleOrder(c, o); e != nil {
			return nil, true, e
		}
		rows, e := tx.List("order-events", M{"orderId": o["id"]})
		if c.Actor.Has("seller") && !c.Actor.Has("dispatcher") {
			visible, _ := visibleOrder(c, o)
			allowed := map[string]bool{}
			for _, sh := range Arr(visible["shipments"]) {
				allowed[S(sh["id"])] = true
			}
			filtered := []M{}
			for _, event := range rows {
				if S(event["shipmentId"]) == "" || allowed[S(event["shipmentId"])] {
					filtered = append(filtered, event)
				}
			}
			rows = filtered
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if I(rows[i]["orderVersion"]) != I(rows[j]["orderVersion"]) {
				return I(rows[i]["orderVersion"]) < I(rows[j]["orderVersion"])
			}
			return S(rows[i]["id"]) < S(rows[j]["id"])
		})
		return Page(rows), true, e
	case "recordOrderEvent":
		m, e := ApplyOrderEvent(tx, c)
		return m, true, e
	case "svcReplaceOrderRoute":
		m, e := ReplaceOrderRoute(tx, c)
		return m, true, e
	case "svcBlockOrderSegment":
		m, e := BlockOrderSegment(tx, c)
		return m, true, e
	case "svcListSimulatedOrderActions":
		rows, e := simulatedOrderActions(tx)
		if e != nil {
			return nil, true, e
		}
		until := Time(c.Query.Get("until"))
		filtered := []M{}
		for _, row := range rows {
			if !Time(row["dueAt"]).After(until) {
				filtered = append(filtered, row)
			}
		}
		return Page(filtered), true, nil
	}
	return nil, false, nil
}
func routeID(order string, revision int64) string { return fmt.Sprintf("%s_%d", order, revision) }
func visibleOrder(c Command, order M) (M, error) {
	if c.Actor.Has("dispatcher") {
		return order, nil
	}
	if c.Actor.Has("pvz_operator") && c.Actor.Access(order) {
		return order, nil
	}
	if c.Actor.Has("seller") {
		copy := Clone(order)
		lines := []M{}
		products := map[string]bool{}
		for _, l := range Arr(copy["lines"]) {
			if c.Actor.Seller(S(l["sellerId"])) {
				lines = append(lines, l)
				products[S(l["productId"])] = true
			}
		}
		if len(lines) > 0 {
			shipments := []M{}
			for _, sh := range Arr(copy["plannedShipments"]) {
				has := false
				for _, it := range Arr(sh["contents"]) {
					has = has || products[S(it["productId"])]
				}
				if has {
					for _, state := range Arr(copy["shipments"]) {
						if S(state["id"]) == S(sh["id"]) {
							shipments = append(shipments, state)
						}
					}
				}
			}
			delete(copy, "plannedShipments")
			delete(copy, "allocations")
			copy["lines"] = lines
			copy["shipments"] = shipments
			return copy, nil
		}
	}
	return nil, Fail(403, "forbidden", "order outside caller scope")
}
func CreateOrder(tx *Tx, c Command) (M, error) {
	quote, option, e := selectedOption(tx, S(c.Body["quoteId"]), S(c.Body["optionId"]))
	if e != nil {
		return nil, e
	}
	pickup := S(Obj(quote["request"])["pickupPointId"])
	if !c.Actor.Point(pickup) {
		return nil, Fail(403, "forbidden", "pickup point outside caller scope")
	}
	snapshot, e := tx.Get("quote-snapshots", S(quote["id"]))
	if e != nil {
		return nil, e
	}
	id := ID()
	lines := []M{}
	lineByProduct := map[string]M{}
	basketPrice, basketCost := int64(0), int64(0)
	for _, it := range Arr(Obj(quote["request"])["items"]) {
		var price M
		for _, p := range Arr(snapshot["prices"]) {
			if S(p["productId"]) == S(it["productId"]) {
				price = p
			}
		}
		if price == nil {
			return nil, Fail(409, "stale_quote", "price snapshot missing")
		}
		basketPrice, e = routingProductSum(basketPrice, I(price["unitPriceMinor"]), I(it["quantity"]))
		if e != nil {
			return nil, e
		}
		basketCost, e = routingProductSum(basketCost, I(price["unitCostMinor"]), I(it["quantity"]))
		if e != nil {
			return nil, e
		}
		line := M{"id": ID(), "productId": it["productId"], "sellerId": price["sellerId"], "quantity": it["quantity"], "unitPriceMinor": price["unitPriceMinor"], "unitCostMinor": price["unitCostMinor"]}
		lines = append(lines, line)
		lineByProduct[S(it["productId"])] = line
	}
	allocations := []M{}
	for _, a := range Arr(snapshot["allocations"]) {
		if S(a["optionId"]) == S(option["id"]) {
			for _, q := range Arr(a["items"]) {
				l := lineByProduct[S(q["productId"])]
				if l == nil {
					return nil, Fail(409, "stale_quote", "allocation line absent")
				}
				allocations = append(allocations, M{"orderItemId": l["id"], "productId": q["productId"], "batchId": q["batchId"], "quantity": q["quantity"], "shipmentId": q["shipmentId"]})
			}
		}
	}
	if len(allocations) == 0 {
		return nil, Fail(409, "stale_quote", "no allocation snapshot")
	}
	assignments := []M{}
	states := []M{}
	for _, sh := range Arr(option["shipments"]) {
		slots := int64(0)
		for _, it := range Arr(sh["contents"]) {
			p, e := tx.Get("products", S(it["productId"]))
			if e != nil {
				return nil, e
			}
			slots, e = routingProductSum(slots, I(p["slotsPerUnit"]), I(it["quantity"]))
			if e != nil {
				return nil, e
			}
		}
		assignments = append(assignments, M{"shipmentId": sh["id"], "fromNodeId": sh["fromNodeId"], "pickupPointId": pickup, "slots": slots})
		state := M{"id": sh["id"], "fromNodeId": sh["fromNodeId"], "pickupPointId": pickup, "lastKnownNodeId": sh["fromNodeId"], "status": "reserved"}
		for _, seg := range Arr(sh["segments"]) {
			if S(seg["type"]) != "on_site" {
				state["nextSegmentId"] = seg["id"]
				break
			}
		}
		states = append(states, state)
	}
	// The transaction owns stock, receiving slots and trip holds for the whole basket.
	if _, e = Dispatch(tx, Command{Name: "svcReserveOrderStock", Actor: c.Actor, Params: map[string]string{"orderId": id}, Body: M{"allocations": allocations, "shipments": assignments}}); e != nil {
		return nil, e
	}
	hold, e := HoldRoute(tx, M{"quoteId": quote["id"], "optionId": option["id"], "subjectType": "order", "subjectId": id}, true)
	if e != nil {
		return nil, e
	}
	order := M{"id": id, "version": 1, "status": "reserved", "pickupPointId": pickup, "lines": lines, "shipments": states, "routeRevision": 1, "quoteId": quote["id"], "buyerFeeMinor": option["buyerFeeMinor"], "promisedReadyAt": option["readyAt"], "estimatedReadyAt": option["readyAt"], "createdAt": Stamp(tx.Now()), "settlementStatus": "not_due", "tariffVersionId": quote["tariffVersionId"], "estimatedCostMinor": option["estimatedCostMinor"], "plannedShipments": Clone(M{"s": option["shipments"]})["s"], "allocations": allocations, "holdId": hold["id"], "mode": option["mode"]}
	route := M{"id": routeID(id, 1), "orderId": id, "revision": 1, "selectedOptionId": option["id"], "tariffVersionId": quote["tariffVersionId"], "expectedReadyAt": option["readyAt"], "remainingEstimatedCostMinor": option["estimatedCostMinor"], "totalEstimatedCostMinor": option["estimatedCostMinor"], "reasons": option["reasons"], "shipments": option["shipments"]}
	if e = tx.Put("routes", route); e != nil {
		return nil, e
	}
	if e = tx.Put("orders", order); e != nil {
		return nil, e
	}
	if e = orderEventRecord(tx, order, M{"type": "created"}, "physical", ""); e != nil {
		return nil, e
	}
	return order, nil
}
func orderEventRecord(tx *Tx, order M, body M, mode, id string) error {
	if id == "" {
		id = ID()
	}
	event := M{"id": id, "orderId": order["id"], "type": body["type"], "occurredAt": Stamp(tx.Now()), "orderVersion": order["version"], "mode": mode}
	for _, k := range []string{"shipmentId", "segmentId", "nodeId"} {
		if S(body[k]) != "" {
			event[k] = body[k]
		}
	}
	return tx.Put("order-events", event)
}
func shipmentPlan(order M, id string) M {
	for _, s := range Arr(order["plannedShipments"]) {
		if S(s["id"]) == id {
			return s
		}
	}
	return nil
}
func plannedSegment(plan M, id string) M {
	for _, s := range Arr(plan["segments"]) {
		if S(s["id"]) == id {
			return s
		}
	}
	return nil
}
func nextPlannedSegment(plan M, after string) M {
	found := after == ""
	for _, s := range Arr(plan["segments"]) {
		if found && S(s["type"]) != "on_site" {
			return s
		}
		if S(s["id"]) == after {
			found = true
		}
	}
	return nil
}
func inventoryOrderEvent(tx *Tx, c Command, order M, body M) error {
	_, e := Dispatch(tx, Command{Name: "svcApplyInventoryOrderEvent", Actor: c.Actor, Params: map[string]string{"orderId": S(order["id"])}, Body: body, SimulationID: c.SimulationID})
	return e
}
func updateOrderAggregate(order M) {
	allReady := true
	anyTransit := false
	anyPacked := false
	for _, s := range Arr(order["shipments"]) {
		status := S(s["status"])
		allReady = allReady && status == "ready"
		anyTransit = anyTransit || status == "in_transit" || status == "at_node"
		anyPacked = anyPacked || status == "packed" || status == "ready"
	}
	switch {
	case allReady:
		order["status"] = "ready"
	case anyTransit:
		order["status"] = "in_transit"
	case anyPacked:
		order["status"] = "packing"
	default:
		order["status"] = "reserved"
	}
}
func ApplyOrderEvent(tx *Tx, c Command) (M, error) {
	order, e := tx.Get("orders", c.Params["orderId"])
	if e != nil {
		return nil, e
	}
	if _, e = visibleOrder(c, order); e != nil {
		return nil, e
	}
	if c.SimulationID != "" {
		if c.Actor.Service != "demo" {
			return nil, Fail(403, "forbidden", "simulation actions require demo service identity")
		}
		if previous, e := tx.Get("order-events", c.SimulationID); e == nil {
			if S(previous["orderId"]) != S(order["id"]) {
				return nil, Fail(409, "simulation_action_mismatch", "action already belongs to another order")
			}
			for _, key := range []string{"type", "shipmentId", "segmentId"} {
				if S(previous[key]) != S(c.Body[key]) {
					return nil, Fail(409, "idempotency_mismatch", "simulation action reused for a different command")
				}
			}
			return order, nil
		}
	}
	if c.SimulationID != "" {
		if c.Actor.Service != "demo" {
			return nil, Fail(403, "forbidden", "simulation actions require demo service identity")
		}
		actions, e := simulatedOrderActions(tx)
		if e != nil {
			return nil, e
		}
		var action M
		for _, candidate := range actions {
			if S(candidate["id"]) == c.SimulationID && S(candidate["orderId"]) == S(order["id"]) {
				action = candidate
				break
			}
		}
		if action == nil {
			return nil, Fail(409, "invalid_simulation_action", "action does not match current planned transition")
		}
		for _, key := range []string{"type", "shipmentId", "segmentId", "expectedVersion"} {
			if S(Obj(action["command"])[key]) != S(c.Body[key]) {
				return nil, Fail(409, "invalid_simulation_action", "command differs from planned action")
			}
		}
		due := Time(action["dueAt"])
		if due.After(tx.Now()) {
			return nil, Fail(409, "invalid_simulation_action", "action is not due")
		}
		originalClock := tx.Clock
		tx.Clock = due
		defer func() { tx.Clock = originalClock }()
	}
	if e = Version(order, c.Body); e != nil {
		return nil, e
	}
	typ := S(c.Body["type"])
	if S(order["status"]) == "issued" || S(order["status"]) == "canceled" {
		return nil, Fail(409, "invalid_transition", "order terminal")
	}
	shipments := Arr(order["shipments"])
	var state M
	for _, sh := range shipments {
		if S(sh["id"]) == S(c.Body["shipmentId"]) {
			state = sh
		}
	}
	inventoryType := ""
	node := ""
	mode := "physical"
	var segment M
	if typ == "issued" || typ == "canceled" {
		if typ == "issued" {
			if S(order["status"]) != "ready" {
				return nil, Fail(409, "invalid_transition", "entire basket must be ready before issue")
			}
			if !c.Actor.Point(S(order["pickupPointId"])) {
				return nil, Fail(403, "forbidden", "issue requires pickup point access")
			}
			inventoryType = "issue"
			node = S(order["pickupPointId"])
		} else {
			for _, sh := range shipments {
				if S(sh["status"]) == "in_transit" || S(sh["status"]) == "at_node" || B(sh["dispatched"]) {
					return nil, Fail(409, "invalid_transition", "dispatched order cannot be canceled")
				}
			}
			inventoryType = "release"
		}
	} else {
		if state == nil {
			return nil, Fail(422, "invalid_shipment", "shipment not in order")
		}
		plan := shipmentPlan(order, S(state["id"]))
		if plan == nil {
			return nil, Fail(409, "route_unavailable", "shipment route missing")
		}
		if B(state["blocked"]) && typ != "arrived" {
			return nil, Fail(409, "segment_failed", "shipment blocked by incident")
		}
		switch typ {
		case "packed":
			if S(state["status"]) != "reserved" {
				return nil, Fail(409, "invalid_transition", "shipment not reserved")
			}
			node = S(state["fromNodeId"])
			next := nextPlannedSegment(plan, "")
			if next != nil && !Time(next["cutoffAt"]).IsZero() && tx.Now().After(Time(next["cutoffAt"])) {
				return nil, Fail(409, "cutoff_missed", "packing missed departure cutoff")
			}
			inventoryType = "pack"
			if n, e := tx.Get("nodes", node); e == nil && S(n["mode"]) == "simulated" {
				mode = "simulated"
			}
			if node == S(order["pickupPointId"]) {
				state["status"] = "ready"
			} else {
				state["status"] = "packed"
			}
		case "handed_over":
			if S(state["status"]) != "packed" && S(state["status"]) != "at_node" {
				return nil, Fail(409, "invalid_transition", "shipment not at departure node")
			}
			if S(state["nextSegmentId"]) != S(c.Body["segmentId"]) {
				return nil, Fail(422, "invalid_segment", "segment not next in shipment")
			}
			segment = plannedSegment(plan, S(c.Body["segmentId"]))
			if segment == nil || S(segment["fromNodeId"]) != S(state["lastKnownNodeId"]) {
				return nil, Fail(422, "invalid_segment", "departure inconsistent with actual node")
			}
			if tx.Now().Before(Time(segment["scheduledStartAt"])) {
				return nil, Fail(409, "invalid_transition", "departure not reached in model time")
			}
			node = S(segment["fromNodeId"])
			mode = S(segment["mode"])
			inventoryType = "dispatch"
			state["status"] = "in_transit"
			state["activeSegmentId"] = segment["id"]
			delete(state, "nextSegmentId")
			state["dispatched"] = true
		case "arrived":
			if S(state["status"]) != "in_transit" || S(state["activeSegmentId"]) != S(c.Body["segmentId"]) {
				return nil, Fail(422, "invalid_segment", "segment is not active in shipment")
			}
			segment = plannedSegment(plan, S(c.Body["segmentId"]))
			if segment == nil {
				return nil, Fail(422, "invalid_segment", "segment missing")
			}
			if tx.Now().Before(Time(segment["scheduledEndAt"])) {
				return nil, Fail(409, "invalid_transition", "arrival not reached in model time")
			}
			node = S(segment["toNodeId"])
			mode = S(segment["mode"])
			inventoryType = "arrive"
			state["lastKnownNodeId"] = node
			delete(state, "activeSegmentId")
			segment["completed"] = true
			next := nextPlannedSegment(plan, S(segment["id"]))
			if next == nil {
				if node != S(order["pickupPointId"]) {
					return nil, Fail(409, "invalid_transition", "route ends outside selected pickup point")
				}
				state["status"] = "ready"
			} else {
				state["status"] = "at_node"
				state["nextSegmentId"] = next["id"]
			}
		default:
			return nil, Fail(422, "invalid_transition", "unknown order event")
		}
		if mode == "simulated" && c.SimulationID == "" {
			return nil, Fail(403, "forbidden", "simulated stages require trusted simulation action")
		}
		if mode == "physical" && c.SimulationID != "" {
			return nil, Fail(403, "forbidden", "simulator cannot execute physical operation")
		}
		if !c.Actor.Point(node) && c.SimulationID == "" {
			return nil, Fail(403, "forbidden", "operator does not own event node")
		}
	}
	if node != "" && c.SimulationID == "" {
		open, e := pointOpen(tx, node, tx.Now())
		if e != nil {
			return nil, e
		}
		if open.After(tx.Now()) {
			return nil, Fail(409, "node_unavailable", "point is closed at model time")
		}
	}
	inv := M{"type": inventoryType, "occurredAt": Stamp(tx.Now())}
	for _, k := range []string{"shipmentId", "segmentId"} {
		if S(c.Body[k]) != "" {
			inv[k] = c.Body[k]
		}
	}
	if node != "" {
		inv["nodeId"] = node
	}
	if e = inventoryOrderEvent(tx, c, order, inv); e != nil {
		return nil, e
	}
	if typ == "arrived" && S(segment["tripId"]) != "" {
		if _, e = UpdateRouteHold(tx, S(order["holdId"]), M{"type": "segment_completed", "segmentId": segment["id"]}); e != nil {
			return nil, e
		}
	}
	if typ == "issued" || typ == "canceled" {
		order["status"] = typ
		for _, sh := range shipments {
			sh["status"] = typ
		}
		if typ == "issued" {
			order["issuedAt"] = Stamp(tx.Now())
			order["settlementStatus"] = "posted"
		} else {
			if _, e = UpdateRouteHold(tx, S(order["holdId"]), M{"type": "release_remaining"}); e != nil {
				return nil, e
			}
		}
	} else {
		updateOrderAggregate(order)
	}
	order["shipments"] = shipments
	order["version"], e = CheckedAdd(I(order["version"]), 1)
	if e != nil {
		return nil, e
	}
	eventID := c.SimulationID
	if eventID == "" {
		eventID = ID()
	}
	if typ == "issued" {
		if e = SettleOrder(tx, order, eventID); e != nil {
			return nil, e
		}
	}
	body := Clone(c.Body)
	if node != "" {
		body["nodeId"] = node
	}
	if e = orderEventRecord(tx, order, body, mode, eventID); e != nil {
		return nil, e
	}
	if e = tx.Put("orders", order); e != nil {
		return nil, e
	}
	route, e := tx.Get("routes", routeID(S(order["id"]), I(order["routeRevision"])))
	if e != nil {
		return nil, e
	}
	route["shipments"] = order["plannedShipments"]
	remaining := int64(0)
	for _, sh := range Arr(route["shipments"]) {
		for _, seg := range Arr(sh["segments"]) {
			if !B(seg["completed"]) {
				remaining, e = CheckedAdd(remaining, I(seg["estimatedCostMinor"]))
				if e != nil {
					return nil, e
				}
			}
		}
	}
	route["remainingEstimatedCostMinor"] = remaining
	return order, tx.Put("routes", route)
}
func simulatedOrderActions(tx *Tx) ([]M, error) {
	orders, e := tx.List("orders", nil)
	if e != nil {
		return nil, e
	}
	out := []M{}
	for _, order := range orders {
		if S(order["status"]) == "issued" || S(order["status"]) == "canceled" {
			continue
		}
		for _, state := range Arr(order["shipments"]) {
			if B(state["blocked"]) {
				continue
			}
			plan := shipmentPlan(order, S(state["id"]))
			if plan == nil {
				continue
			}
			typ := ""
			seg := M{}
			due := time.Time{}
			switch S(state["status"]) {
			case "reserved":
				node, e := tx.Get("nodes", S(state["fromNodeId"]))
				if e != nil {
					return nil, e
				}
				if S(node["mode"]) == "simulated" {
					typ = "packed"
					due = Time(order["createdAt"])
				}
			case "packed", "at_node":
				seg = plannedSegment(plan, S(state["nextSegmentId"]))
				if S(seg["mode"]) == "simulated" {
					typ = "handed_over"
					due = Time(seg["scheduledStartAt"])
				}
			case "in_transit":
				seg = plannedSegment(plan, S(state["activeSegmentId"]))
				if S(seg["mode"]) == "simulated" {
					typ = "arrived"
					due = Time(seg["scheduledEndAt"])
				}
			}
			if typ == "" {
				continue
			}
			command := M{"type": typ, "shipmentId": state["id"], "expectedVersion": order["version"]}
			if S(seg["id"]) != "" {
				command["segmentId"] = seg["id"]
			}
			id := fmt.Sprintf("%s_%s_%s", S(state["id"]), typ, S(seg["id"]))
			out = append(out, M{"id": id, "dueAt": Stamp(due), "orderId": order["id"], "command": command})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if S(out[i]["dueAt"]) != S(out[j]["dueAt"]) {
			return S(out[i]["dueAt"]) < S(out[j]["dueAt"])
		}
		return S(out[i]["id"]) < S(out[j]["id"])
	})
	return out, nil
}

// AdvanceOrders executes only simulated transitions, sorted by due time then stable action ID.
func AdvanceOrders(tx *Tx) error {
	target := tx.Now()
	defer func() { tx.Clock = target }()
	for {
		tx.Clock = target
		actions, e := simulatedOrderActions(tx)
		if e != nil {
			return e
		}
		if len(actions) == 0 || Time(actions[0]["dueAt"]).After(target) {
			return nil
		}
		action := actions[0]
		tx.Clock = Time(action["dueAt"])
		_, e = ApplyOrderEvent(tx, Command{Name: "recordOrderEvent", Params: map[string]string{"orderId": S(action["orderId"])}, Body: Obj(action["command"]), Actor: Actor{Subject: "simulator", Service: "demo", Roles: []string{"dispatcher"}}, SimulationID: S(action["id"])})
		if e != nil {
			return e
		}
	}
}
func BlockOrderSegment(tx *Tx, c Command) (M, error) {
	order, e := tx.Get("orders", c.Params["orderId"])
	if e != nil {
		return nil, e
	}
	if I(order["routeRevision"]) != I(c.Body["expectedRouteRevision"]) {
		return nil, Fail(409, "stale_version", "route revision changed")
	}
	if S(order["status"]) == "issued" || S(order["status"]) == "canceled" {
		return nil, Fail(409, "invalid_transition", "terminal order")
	}
	found := false
	target := Obj(c.Body["failureTarget"])
	for _, s := range Arr(order["shipments"]) {
		if S(s["id"]) == S(c.Body["shipmentId"]) {
			plan := shipmentPlan(order, S(s["id"]))
			for _, seg := range Arr(plan["segments"]) {
				if !B(seg["completed"]) && ((S(target["tripId"]) != "" && S(seg["tripId"]) == S(target["tripId"])) || (S(target["segmentId"]) != "" && S(seg["id"]) == S(target["segmentId"])) || (S(target["nodeId"]) != "" && (S(seg["fromNodeId"]) == S(target["nodeId"]) || S(seg["toNodeId"]) == S(target["nodeId"])))) {
					found = true
					s["blocked"] = true
					s["incidentId"] = c.Params["incidentId"]
				}
			}
		}
	}
	if !found {
		return nil, Fail(422, "invalid_segment", "failure target does not affect unfinished shipment route")
	}
	order["version"], e = CheckedAdd(I(order["version"]), 1)
	if e != nil {
		return nil, e
	}
	return order, tx.Put("orders", order)
}
func ReplaceOrderRoute(tx *Tx, c Command) (M, error) {
	order, e := tx.Get("orders", c.Params["orderId"])
	if e != nil {
		return nil, e
	}
	if e = Version(order, c.Body); e != nil {
		return nil, e
	}
	if S(order["status"]) == "issued" || S(order["status"]) == "canceled" {
		return nil, Fail(409, "invalid_transition", "terminal order")
	}
	var state M
	for _, s := range Arr(order["shipments"]) {
		if S(s["id"]) == S(c.Body["shipmentId"]) {
			state = s
		}
	}
	if state == nil {
		return nil, Fail(422, "invalid_shipment", "shipment not in order")
	}
	if S(state["status"]) == "in_transit" {
		return nil, Fail(409, "route_blocked", "cargo is in transit; confirm its next actual arrival before rerouting")
	}
	plan := shipmentPlan(order, S(state["id"]))
	if plan == nil {
		return nil, Fail(409, "route_unavailable", "shipment route missing")
	}
	tariff, e := tx.Get("tariffs", S(order["tariffVersionId"]))
	if e != nil {
		return nil, e
	}
	slots := int64(0)
	for _, it := range Arr(plan["contents"]) {
		p, e := tx.Get("products", S(it["productId"]))
		if e != nil {
			return nil, e
		}
		slots, e = routingProductSum(slots, I(p["slotsPerUnit"]), I(it["quantity"]))
		if e != nil {
			return nil, e
		}
	}
	segments, ready, cost, e := FindRoute(tx, S(state["lastKnownNodeId"]), S(order["pickupPointId"]), tx.Now(), slots, tariff, Obj(c.Body["failureTarget"]))
	if e != nil {
		return nil, Fail(409, "route_blocked", e.Error())
	}
	previousRoute, e := tx.Get("routes", routeID(S(order["id"]), I(order["routeRevision"])))
	if e != nil {
		return nil, e
	}
	completed := []M{}
	completedCost := int64(0)
	for _, seg := range Arr(plan["segments"]) {
		if B(seg["completed"]) {
			completed = append(completed, seg)
			completedCost, e = CheckedAdd(completedCost, I(seg["estimatedCostMinor"]))
			if e != nil {
				return nil, e
			}
		}
	}
	plan["segments"] = append(completed, segments...)
	plan["readyAt"] = Stamp(ready)
	delete(state, "blocked")
	delete(state, "activeSegmentId")
	if next := nextPlannedSegment(plan, ""); next != nil {
		for _, seg := range Arr(plan["segments"]) {
			if !B(seg["completed"]) && S(seg["type"]) != "on_site" {
				state["nextSegmentId"] = seg["id"]
				break
			}
		}
	}
	if S(state["lastKnownNodeId"]) == S(order["pickupPointId"]) {
		state["status"] = "ready"
	}
	newRevision, e := CheckedAdd(I(order["routeRevision"]), 1)
	if e != nil {
		return nil, e
	}
	route := Clone(previousRoute)
	route["id"] = routeID(S(order["id"]), newRevision)
	route["previousRevision"] = order["routeRevision"]
	route["revision"] = newRevision
	route["incidentId"] = c.Body["incidentId"]
	route["shipments"] = order["plannedShipments"]
	eta := ready
	remaining := int64(0)
	total := int64(0)
	for _, sh := range Arr(route["shipments"]) {
		if Time(sh["readyAt"]).After(eta) {
			eta = Time(sh["readyAt"])
		}
		for _, seg := range Arr(sh["segments"]) {
			total, e = CheckedAdd(total, I(seg["estimatedCostMinor"]))
			if e != nil {
				return nil, e
			}
			if !B(seg["completed"]) {
				remaining, e = CheckedAdd(remaining, I(seg["estimatedCostMinor"]))
				if e != nil {
					return nil, e
				}
			}
		}
	}
	_ = cost
	_ = completedCost
	route["expectedReadyAt"] = Stamp(eta)
	route["remainingEstimatedCostMinor"] = remaining
	route["totalEstimatedCostMinor"] = total
	route["deltaCostMinor"] = total - I(previousRoute["totalEstimatedCostMinor"])
	route["deltaEtaMinutes"] = int64(eta.Sub(Time(order["estimatedReadyAt"])) / time.Minute)
	route["reasons"] = []M{routingReason("segment_failed", "remaining route replanned from last confirmed node")}
	// Replace the remaining transport reservation without touching already allocated inventory.
	movement := M{"id": ID(), "expiresAt": Stamp(tx.Now().Add(15 * time.Minute)), "options": []M{{"id": ID(), "shipments": order["plannedShipments"]}}}
	if e = tx.Put("movement-quotes", movement); e != nil {
		return nil, e
	}
	hold, e := HoldRoute(tx, M{"quoteId": movement["id"], "optionId": Arr(movement["options"])[0]["id"], "subjectId": order["id"], "subjectType": "order", "replacesHoldId": order["holdId"]}, true)
	if e != nil {
		return nil, e
	}
	order["holdId"] = hold["id"]
	order["routeRevision"] = newRevision
	order["estimatedReadyAt"] = Stamp(eta)
	order["estimatedCostMinor"] = total
	order["version"], e = CheckedAdd(I(order["version"]), 1)
	if e != nil {
		return nil, e
	}
	updateOrderAggregate(order)
	if e = tx.Put("routes", route); e != nil {
		return nil, e
	}
	if e = orderEventRecord(tx, order, M{"type": "rerouted", "shipmentId": state["id"]}, "physical", ""); e != nil {
		return nil, e
	}
	return order, tx.Put("orders", order)
}
