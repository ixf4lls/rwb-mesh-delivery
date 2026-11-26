package core

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

func init() { Register(planningHandler) }
func planningHandler(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "createQuote":
		m, e := CreateQuote(tx, c)
		return m, true, e
	case "svcGetQuoteSnapshot":
		m, e := tx.Get("quote-snapshots", c.Params["quoteId"])
		return m, true, e
	case "svcQuoteMovement":
		m, e := QuoteMovement(tx, c.Body)
		return m, true, e
	case "svcHoldTripCapacity":
		m, e := HoldRoute(tx, c.Body, false)
		return m, true, e
	case "svcUpdateTripHold":
		m, e := UpdateRouteHold(tx, c.Params["holdId"], c.Body)
		return m, true, e
	}
	return nil, false, nil
}
func routingReason(code, msg string) M { return M{"code": code, "message": msg} }
func routingTariff(tx *Tx) (M, error) {
	rows, e := tx.List("tariffs", nil)
	if e != nil {
		return nil, e
	}
	sort.Slice(rows, func(i, j int) bool { return Time(rows[i]["effectiveFrom"]).After(Time(rows[j]["effectiveFrom"])) })
	for _, m := range rows {
		if !Time(m["effectiveFrom"]).After(tx.Now()) && (S(m["effectiveTo"]) == "" || Time(m["effectiveTo"]).After(tx.Now())) {
			return m, nil
		}
	}
	return nil, Fail(409, "tariff_unavailable", "no tariff effective at model time")
}
func routeRate(t M, code string) int64 {
	for _, r := range Arr(t["rates"]) {
		if S(r["code"]) == code {
			return I(r["amount"])
		}
	}
	return 0
}
func routingProductSum(total, a, b int64) (int64, error) {
	p, e := CheckedMul(a, b)
	if e != nil {
		return 0, e
	}
	return CheckedAdd(total, p)
}
func routingFree(capacity, background, held int64) (int64, error) {
	if capacity < 0 || background < 0 || held < 0 {
		return 0, Fail(422, "validation_error", "negative route capacity")
	}
	used, e := CheckedAdd(background, held)
	if e != nil {
		return 0, e
	}
	if used >= capacity {
		return 0, nil
	}
	return capacity - used, nil
}
func routingMinutes(minutes int64) (time.Duration, error) {
	if minutes < 0 {
		return 0, Fail(422, "validation_error", "negative schedule duration")
	}
	n, e := CheckedMul(minutes, int64(time.Minute))
	return time.Duration(n), e
}
func nodeLocation(tx *Tx, id string) (*time.Location, error) {
	n, e := tx.Get("nodes", id)
	if e != nil {
		return nil, e
	}
	city, e := tx.Get("cities", S(n["cityId"]))
	if e != nil {
		return nil, e
	}
	z := S(city["timeZone"])
	if z == "" {
		return nil, Fail(409, "network_incomplete", "city time zone missing")
	}
	loc, e := time.LoadLocation(z)
	if e != nil {
		return nil, Fail(409, "network_incomplete", "invalid city time zone")
	}
	return loc, nil
}
func pointOpen(tx *Tx, id string, at time.Time) (time.Time, error) {
	p, e := tx.Get("pickup-points", id)
	if e != nil {
		return at, nil
	}
	hours := Arr(p["openingHours"])
	if len(hours) == 0 {
		return at, Fail(409, "network_incomplete", "point opening hours missing")
	}
	loc, e := nodeLocation(tx, id)
	if e != nil {
		return at, e
	}
	local := at.In(loc)
	for day := 0; day < 8; day++ {
		d := local.AddDate(0, 0, day)
		weekday := int(d.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		best := time.Time{}
		for _, h := range hours {
			weekdays := h["weekdays"]
			allowed := false
			switch a := weekdays.(type) {
			case []any:
				for _, w := range a {
					if I(w) == int64(weekday) {
						allowed = true
					}
				}
			case []int:
				for _, w := range a {
					if w == weekday {
						allowed = true
					}
				}
			}
			if !allowed {
				continue
			}
			op, e1 := time.Parse("15:04", S(h["opens"]))
			cl, e2 := time.Parse("15:04", S(h["closes"]))
			if e1 != nil || e2 != nil {
				return at, Fail(409, "network_incomplete", "invalid opening interval")
			}
			start := time.Date(d.Year(), d.Month(), d.Day(), op.Hour(), op.Minute(), 0, 0, loc)
			end := time.Date(d.Year(), d.Month(), d.Day(), cl.Hour(), cl.Minute(), 0, 0, loc)
			if !end.After(start) {
				end = end.AddDate(0, 0, 1)
			}
			if at.Before(end) {
				if start.Before(at) {
					start = at
				}
				if best.IsZero() || start.Before(best) {
					best = start
				}
			}
		}
		if !best.IsZero() {
			return best, nil
		}
	}
	return at, Fail(409, "node_unavailable", "point has no opening within next week")
}

type routeEdge struct {
	trip                              M
	from, to                          string
	fromIndex, toIndex                int
	start, end                        time.Time
	slots, background, distance, cost int64
	typ, mode                         string
	cutoff                            time.Time
	extra                             int64
	wholeCost, extraCost              int64
}

func tripTime(tx *Tx, t M, at time.Time) (time.Time, error) {
	if s := S(t["departureAt"]); s != "" {
		d := Time(s)
		if d.IsZero() {
			return d, Fail(409, "network_incomplete", "invalid trip departureAt")
		}
		if d.Before(at) && S(t["frequency"]) != "daily" {
			return time.Time{}, nil
		}
		if S(t["frequency"]) == "daily" {
			loc, e := nodeLocation(tx, S(t["fromNodeId"]))
			if e != nil {
				return time.Time{}, e
			}
			d = d.In(loc)
			if d.Before(at) {
				local := at.In(loc)
				d = time.Date(local.Year(), local.Month(), local.Day(), d.Hour(), d.Minute(), d.Second(), d.Nanosecond(), loc)
				if d.Before(at) {
					d = d.AddDate(0, 0, 1)
				}
			}
		}
		return d, nil
	}
	loc, e := nodeLocation(tx, S(t["fromNodeId"]))
	if e != nil {
		return time.Time{}, e
	}
	h, e := time.Parse("15:04", S(t["departureLocal"]))
	if e != nil {
		return time.Time{}, Fail(409, "network_incomplete", "trip departure schedule missing")
	}
	d := at.In(loc)
	dep := time.Date(d.Year(), d.Month(), d.Day(), h.Hour(), h.Minute(), 0, 0, loc)
	if dep.Before(at) {
		if S(t["frequency"]) != "daily" {
			return time.Time{}, nil
		}
		dep = dep.AddDate(0, 0, 1)
	}
	return dep, nil
}
func tripEdges(tx *Tx, t M, at time.Time, tariff M) ([]routeEdge, error) {
	if v, ok := t["active"]; ok && !B(v) {
		return nil, nil
	}
	cap := I(t["capacitySlots"])
	if cap <= 0 {
		return nil, nil
	}
	scheduleAt := at
	st := Arr(t["stops"])
	maxOffset := I(t["durationMinutes"])
	for _, stop := range st {
		if I(stop["offsetMinutes"]) > maxOffset {
			maxOffset = I(stop["offsetMinutes"])
		}
	}
	offsetDuration, e := routingMinutes(maxOffset)
	if e != nil {
		return nil, e
	}
	if len(st) > 2 {
		scheduleAt = at.Add(-offsetDuration)
	}
	dep, e := tripTime(tx, t, scheduleAt)
	if e != nil {
		return nil, nil
	}
	if dep.IsZero() {
		return nil, nil
	}
	stops := Arr(t["stops"])
	if len(stops) == 0 {
		dur := I(t["durationMinutes"])
		if dur <= 0 {
			return nil, nil
		}
		stops = []M{{"nodeId": t["fromNodeId"], "offsetMinutes": int64(0)}, {"nodeId": t["toNodeId"], "offsetMinutes": dur}}
	}
	if len(stops) < 2 {
		return nil, nil
	}
	out := []routeEdge{}
	firstDeparture := dep
	horizon, e := routingMinutes(I(tariff["maximumDeliveryMinutes"]))
	if e != nil {
		return nil, e
	}
	for day := 0; ; day++ {
		dep = firstDeparture.AddDate(0, 0, day)
		if day > 0 && (S(t["frequency"]) != "daily" || dep.After(at.Add(horizon))) {
			break
		}
		for i := 0; i < len(stops)-1; i++ {
			a, b := stops[i], stops[i+1]
			startOffset, e := routingMinutes(I(a["offsetMinutes"]))
			if e != nil {
				return nil, e
			}
			endOffset, e := routingMinutes(I(b["offsetMinutes"]))
			if e != nil {
				return nil, e
			}
			start := dep.Add(startOffset)
			end := dep.Add(endOffset)
			if s := S(a["departureAt"]); s != "" {
				start = Time(s).AddDate(0, 0, day)
			}
			if s := S(b["arrivalAt"]); s != "" {
				end = Time(s).AddDate(0, 0, day)
			}
			if !end.After(start) {
				continue
			}
			typ := S(t["type"])
			if typ == "" {
				typ = S(t["tripType"])
			}
			if typ == "intercity" {
				typ = "linehaul"
			}
			if typ == "" {
				typ = "city_trip"
			}
			mode := S(t["mode"])
			if mode == "" {
				mode = "physical"
			}
			cost := I(t["costMinor"])
			if cost == 0 {
				rate := typ
				if typ == "linehaul" {
					rate = "intercity"
				}
				cost = routeRate(tariff, rate)
			}
			wholeCost := cost
			if len(stops) > 2 {
				denominator := int64(len(stops) - 1)
				quotient := cost / denominator
				if cost%denominator != 0 {
					quotient++
				}
				cost = quotient
			}
			extra := int64(0)
			extraCost := int64(0)
			if B(b["additionalStop"]) {
				extra = 1
				extraCost = routeRate(tariff, "extra_stop")
				cost, e = CheckedAdd(cost, extraCost)
				if e != nil {
					return nil, e
				}
			}
			cutoffDuration, e := routingMinutes(I(t["cutoffMinutes"]))
			if e != nil {
				return nil, e
			}
			cutoff := start.Add(-cutoffDuration)
			out = append(out, routeEdge{trip: t, from: S(a["nodeId"]), to: S(b["nodeId"]), fromIndex: i, toIndex: i + 1, start: start, end: end, slots: cap, background: I(t["backgroundSlots"]), distance: I(t["distanceMeters"]) / int64(len(stops)-1), cost: cost, typ: typ, mode: mode, cutoff: cutoff, extra: extra, wholeCost: wholeCost, extraCost: extraCost})
		}
	}
	return out, nil
}
func legKey(e routeEdge) string {
	return fmt.Sprintf("%s:%s:%d", S(e.trip["id"]), Stamp(e.start), e.fromIndex)
}
func heldLegs(tx *Tx, except string) (map[string]int64, error) {
	rows, e := tx.List("route-holds", nil)
	if e != nil {
		return nil, e
	}
	used := map[string]int64{}
	for _, h := range rows {
		if S(h["id"]) == except || S(h["status"]) == "released" || S(h["status"]) == "completed" {
			continue
		}
		if S(h["status"]) == "held" && !Time(h["expiresAt"]).After(tx.Now()) {
			continue
		}
		done := Strings(h["completedSegments"])
		for _, s := range Arr(h["legs"]) {
			if !Contains(done, S(s["segmentId"])) {
				key := S(s["key"])
				used[key], e = CheckedAdd(used[key], I(s["slots"]))
				if e != nil {
					return nil, e
				}
			}
		}
	}
	return used, nil
}
func edgeSegment(e routeEdge, at time.Time, slots, free int64) (M, error) {
	if e.slots <= 0 || slots <= 0 || slots > e.slots || e.cost < 0 || e.extraCost < 0 {
		return nil, Fail(422, "validation_error", "invalid transport cost or capacity")
	}
	base := e.cost - e.extraCost
	product := new(big.Int).Mul(big.NewInt(base), big.NewInt(slots))
	product.Quo(product, big.NewInt(e.slots))
	if !product.IsInt64() {
		return nil, Fail(422, "validation_error", "allocated trip cost exceeds int64")
	}
	allocated := product.Int64()
	estimated, err := CheckedAdd(allocated, e.extraCost)
	if err != nil {
		return nil, err
	}
	costs := []M{{"category": e.typ, "amountMinor": allocated}}
	m := M{"id": ID(), "type": e.typ, "fromNodeId": e.from, "toNodeId": e.to, "mode": e.mode, "scheduledStartAt": Stamp(e.start), "scheduledEndAt": Stamp(e.end), "waitMinutes": int64(e.start.Sub(at) / time.Minute), "distanceMeters": e.distance, "estimatedCostMinor": estimated, "wholeTripCostMinor": e.wholeCost, "allocatedCostMinor": allocated, "incrementalCostMinor": e.extraCost, "extraStopCostMinor": e.extraCost, "buyerFeeMinor": e.cost, "costItems": costs, "tripId": e.trip["id"], "cutoffAt": Stamp(e.cutoff), "requestedSlots": slots, "freeTripSlots": free, "incrementalStops": e.extra, "_legKey": legKey(e)}
	if e.extra > 0 {
		m["stopChargeKey"] = S(e.trip["id"]) + "_" + Stamp(e.start) + "_" + e.to
		m["costItems"] = append(costs, M{"category": "extra_stop", "amountMinor": e.extraCost})
	}
	return m, nil
}
func onsiteSegment(node string, at time.Time) M {
	return M{"id": ID(), "type": "on_site", "fromNodeId": node, "toNodeId": node, "mode": "physical", "scheduledStartAt": Stamp(at), "scheduledEndAt": Stamp(at), "waitMinutes": 0, "distanceMeters": 0, "estimatedCostMinor": 0, "costItems": []M{}}
}

// FindRoute searches scheduled adjacency only; it never builds a point-to-point matrix.
func FindRoute(tx *Tx, from, to string, at time.Time, slots int64, tariff M, failure M) ([]M, time.Time, int64, error) {
	horizon, e := routingMinutes(I(tariff["maximumDeliveryMinutes"]))
	if e != nil {
		return nil, time.Time{}, 0, e
	}
	if from == to {
		ready, e := pointOpen(tx, to, at)
		if e != nil {
			return nil, time.Time{}, 0, e
		}
		if ready.After(at.Add(horizon)) {
			return nil, time.Time{}, 0, Fail(409, "sla", "point opens after delivery deadline")
		}
		return []M{onsiteSegment(from, ready)}, ready, 0, nil
	}
	used, e := heldLegs(tx, "")
	if e != nil {
		return nil, time.Time{}, 0, e
	}
	active := map[string]bool{}
	originCity := ""
	for _, id := range []string{from, to} {
		n, e := tx.Get("nodes", id)
		if e != nil {
			return nil, time.Time{}, 0, e
		}
		active[id] = B(n["active"])
		if id == from {
			originCity = S(n["cityId"])
		}
	}
	if !active[from] || !active[to] {
		return nil, time.Time{}, 0, Fail(409, "node_unavailable", "route endpoint inactive")
	}
	type label struct {
		node     string
		at       time.Time
		cost     int64
		segments []M
		seen     map[string]bool
	}
	queue := []label{{from, at, 0, []M{}, map[string]bool{from: true}}}
	best := map[string][]label{}
	var winner *label
	deadline := at.Add(horizon)
	if I(tariff["maximumDeliveryMinutes"]) <= 0 {
		return nil, time.Time{}, 0, Fail(409, "tariff_unavailable", "delivery SLA missing")
	}
	sawCapacity := false
	candidateCache := map[string][]M{}
	for len(queue) > 0 {
		sort.SliceStable(queue, func(i, j int) bool {
			if queue[i].cost != queue[j].cost {
				return queue[i].cost < queue[j].cost
			}
			if !queue[i].at.Equal(queue[j].at) {
				return queue[i].at.Before(queue[j].at)
			}
			return queue[i].node < queue[j].node
		})
		cur := queue[0]
		queue = queue[1:]
		if winner != nil && cur.cost > winner.cost {
			break
		}
		if cur.node == to {
			copy := cur
			winner = &copy
			break
		}
		trips, loaded := candidateCache[cur.node]
		if !loaded {
			trips, e = tx.TripsFrom(cur.node)
			if e != nil {
				return nil, time.Time{}, 0, e
			}
			candidateCache[cur.node] = trips
		}
		for _, trip := range trips {
			if S(trip["id"]) == S(failure["tripId"]) {
				continue
			}
			edges, e := tripEdges(tx, trip, cur.at, tariff)
			if e != nil {
				return nil, time.Time{}, 0, e
			}
			for _, edge := range edges {
				if edge.from != cur.node || cur.seen[edge.to] || edge.to == S(failure["nodeId"]) {
					continue
				}
				available, known := active[edge.to]
				if !known {
					n, e := tx.Get("nodes", edge.to)
					if e != nil {
						return nil, time.Time{}, 0, e
					}
					available = B(n["active"])
					active[edge.to] = available
				}
				if !available {
					continue
				}
				if B(failure["_meshOnly"]) {
					n, e := tx.Get("nodes", edge.to)
					if e != nil {
						return nil, time.Time{}, 0, e
					}
					if S(n["kind"]) == "hub" || S(n["cityId"]) != originCity {
						continue
					}
				}
				if edge.start.Before(cur.at) || edge.cutoff.Before(cur.at) || edge.end.After(deadline) {
					continue
				}
				free, e := routingFree(edge.slots, edge.background, used[legKey(edge)])
				if e != nil {
					return nil, time.Time{}, 0, e
				}
				if free < slots {
					sawCapacity = true
					continue
				}
				segment, e := edgeSegment(edge, cur.at, slots, free)
				if e != nil {
					return nil, time.Time{}, 0, e
				}
				segments := append(append([]M{}, cur.segments...), segment)
				pathCost, e := CheckedAdd(cur.cost, edge.extraCost)
				if e != nil {
					return nil, time.Time{}, 0, e
				}
				next := label{node: edge.to, at: edge.end, cost: pathCost, segments: segments, seen: map[string]bool{}}
				for k, v := range cur.seen {
					next.seen[k] = v
				}
				next.seen[edge.to] = true
				dominated := false
				for _, b := range best[next.node] {
					if b.cost <= next.cost && !b.at.After(next.at) {
						dominated = true
						break
					}
				}
				if !dominated {
					best[next.node] = append(best[next.node], next)
					queue = append(queue, next)
				}
			}
		}
	}
	if winner == nil {
		code := "no_service"
		if sawCapacity {
			code = "trip_capacity"
		}
		return nil, time.Time{}, 0, Fail(409, code, "no scheduled path satisfies capacity, cutoff and SLA")
	}
	ready, e := pointOpen(tx, to, winner.at)
	if e != nil {
		return nil, time.Time{}, 0, e
	}
	if ready.After(deadline) {
		return nil, time.Time{}, 0, Fail(409, "sla", "receiving point opens after delivery deadline")
	}
	allocated := int64(0)
	for _, segment := range winner.segments {
		allocated, e = CheckedAdd(allocated, I(segment["estimatedCostMinor"]))
		if e != nil {
			return nil, time.Time{}, 0, e
		}
	}
	return winner.segments, ready, allocated, nil
}
func makeQuoteOption(tx *Tx, request M, policy string, tariff M) (M, []M, error) {
	option := M{"id": ID(), "policy": policy, "feasible": false, "reasons": []M{routingReason("insufficient_stock", "basket cannot be allocated under selected policy")}}
	pickup := S(request["pickupPointId"])
	allocations := []M{}
	shipments := []M{}
	bySource := map[string]M{}
	totalCost := int64(0)
	ready := tx.Now()
	modes := map[string]bool{}
	usedBatches := map[string]int64{}
	for _, item := range Arr(request["items"]) {
		productID := S(item["productId"])
		qty := I(item["quantity"])
		product, e := tx.Get("products", productID)
		if e != nil {
			return nil, nil, e
		}
		batches, e := tx.List("batches", M{"productId": productID})
		if e != nil {
			return nil, nil, e
		}
		type candidate struct {
			batch    M
			segments []M
			ready    time.Time
			cost     int64
			mode     string
			marginal int64
		}
		candidates := []candidate{}
		lastReason := routingReason("insufficient_stock", "insufficient available stock")
		for _, batch := range batches {
			available := I(batch["availableQuantity"]) - usedBatches[S(batch["id"])]
			if available <= 0 {
				continue
			}
			source := S(batch["storagePointId"])
			node, e := tx.Get("nodes", source)
			if e != nil {
				continue
			}
			kind := S(node["kind"])
			hub := kind == "hub"
			if (policy == "mesh_only" && hub) || (policy == "hub_only" && !hub) {
				continue
			}
			if !B(node["active"]) {
				lastReason = routingReason("node_unavailable", "source node inactive")
				continue
			}
			slots, e := CheckedMul(qty, I(product["slotsPerUnit"]))
			if e != nil {
				return nil, nil, e
			}
			if slots <= 0 {
				return nil, nil, Fail(409, "invalid_product", "product slotsPerUnit missing")
			}
			start := tx.Now()
			if source != pickup {
				packing, e := routingMinutes(I(node["packingMinutes"]))
				if e != nil {
					return nil, nil, e
				}
				start = start.Add(packing)
				if !hub {
					start, e = pointOpen(tx, source, start)
					if e != nil {
						lastReason = routingReason("node_unavailable", e.Error())
						continue
					}
				}
			}
			constraints := M{}
			if policy == "mesh_only" {
				constraints["_meshOnly"] = true
			}
			segments, eta, cost, e := FindRoute(tx, source, pickup, start, slots, tariff, constraints)
			if e != nil {
				if p, ok := e.(*Problem); ok && p.Status == 422 {
					return nil, nil, e
				}
				code := "no_service"
				if p, ok := e.(*Problem); ok {
					code = p.Code
				}
				lastReason = routingReason(code, e.Error())
				continue
			}
			mode := "city_mesh"
			if hub {
				mode = "hub"
			}
			if source == pickup {
				mode = "local_shelf"
			}
			marginal := int64(0)
			for _, segment := range segments {
				marginal, e = CheckedAdd(marginal, I(segment["incrementalCostMinor"]))
				if e != nil {
					return nil, nil, e
				}
			}
			candidates = append(candidates, candidate{batch, segments, eta, cost, mode, marginal})
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if a.marginal != b.marginal {
				return a.marginal < b.marginal
			}
			if !a.ready.Equal(b.ready) {
				return a.ready.Before(b.ready)
			}
			return S(a.batch["id"]) < S(b.batch["id"])
		})
		remaining := qty
		for _, c := range candidates {
			n := I(c.batch["availableQuantity"]) - usedBatches[S(c.batch["id"])]
			if n > remaining {
				n = remaining
			}
			if n <= 0 {
				continue
			}
			source := S(c.batch["storagePointId"])
			sh := bySource[source]
			if sh == nil {
				sh = M{"id": ID(), "fromNodeId": source, "toNodeId": pickup, "contents": []M{}, "readyAt": Stamp(c.ready), "segments": c.segments}
				bySource[source] = sh
				shipments = append(shipments, sh)
				totalCost, e = CheckedAdd(totalCost, c.cost)
				if e != nil {
					return nil, nil, e
				}
				modes[c.mode] = true
				if c.ready.After(ready) {
					ready = c.ready
				}
			}
			contents := Arr(sh["contents"])
			found := false
			for _, content := range contents {
				if S(content["productId"]) == productID {
					content["quantity"] = I(content["quantity"]) + n
					found = true
				}
			}
			if !found {
				contents = append(contents, M{"productId": productID, "quantity": n})
			}
			sh["contents"] = contents
			usedBatches[S(c.batch["id"])] += n
			allocations = append(allocations, M{"productId": productID, "batchId": c.batch["id"], "shipmentId": sh["id"], "quantity": n})
			remaining -= n
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			option["reasons"] = []M{lastReason}
			return option, nil, nil
		}
	}
	// Evaluate the complete basket together; per-line feasibility cannot oversubscribe shared legs.
	usedLegs, e := heldLegs(tx, "")
	if e != nil {
		return nil, nil, e
	}
	inbound := int64(0)
	for _, sh := range shipments {
		slots := int64(0)
		for _, item := range Arr(sh["contents"]) {
			p, e := tx.Get("products", S(item["productId"]))
			if e != nil {
				return nil, nil, e
			}
			slots, e = routingProductSum(slots, I(item["quantity"]), I(p["slotsPerUnit"]))
			if e != nil {
				return nil, nil, e
			}
		}
		if S(sh["fromNodeId"]) != pickup {
			inbound, e = CheckedAdd(inbound, slots)
			if e != nil {
				return nil, nil, e
			}
		}
		for _, seg := range Arr(sh["segments"]) {
			if S(seg["tripId"]) == "" {
				continue
			}
			trip, e := tx.Get("trips", S(seg["tripId"]))
			if e != nil {
				return nil, nil, e
			}
			key := S(seg["_legKey"])
			free, e := routingFree(I(trip["capacitySlots"]), I(trip["backgroundSlots"]), usedLegs[key])
			if e != nil {
				return nil, nil, e
			}
			if free < slots {
				option["reasons"] = []M{routingReason("trip_capacity", "basket exceeds shared trip capacity")}
				return option, nil, nil
			}
			usedLegs[key], e = CheckedAdd(usedLegs[key], slots)
			if e != nil {
				return nil, nil, e
			}
			seg["requestedSlots"] = slots
			seg["freeTripSlots"] = free
			baseFee := I(seg["buyerFeeMinor"]) - I(seg["extraStopCostMinor"])
			product := new(big.Int).Mul(big.NewInt(baseFee), big.NewInt(slots))
			product.Quo(product, big.NewInt(I(trip["capacitySlots"])))
			if !product.IsInt64() {
				return nil, nil, Fail(422, "validation_error", "allocated basket cost exceeds int64")
			}
			seg["allocatedCostMinor"] = product.Int64()
			seg["estimatedCostMinor"], e = CheckedAdd(product.Int64(), I(seg["extraStopCostMinor"]))
			if e != nil {
				return nil, nil, e
			}
			costs := []M{{"category": seg["type"], "amountMinor": product.Int64()}}
			if I(seg["extraStopCostMinor"]) > 0 {
				costs = append(costs, M{"category": "extra_stop", "amountMinor": seg["extraStopCostMinor"]})
			}
			seg["costItems"] = costs
		}
	}
	if inbound > 0 {
		cap, e := PointCapacity(tx, pickup)
		if e != nil {
			return nil, nil, e
		}
		if I(cap["availablePickupSlots"]) < inbound {
			option["reasons"] = []M{routingReason("point_capacity", "receiving point cannot accept complete basket")}
			return option, nil, nil
		}
	}
	mode := "mixed"
	if len(modes) == 1 {
		for m := range modes {
			mode = m
		}
	}
	reason := routingReason("local_stock", "available stock and route satisfy basket")
	if mode == "local_shelf" {
		reason = routingReason("same_pickup_point", "all items available at pickup point")
	}
	if mode == "hub" {
		reason = routingReason("hub_stock", "hub stock and route available")
	}
	if mode == "mixed" {
		reason = routingReason("allocation_split", "basket split across sources")
	}
	buyer, totalCost, marginal := int64(0), int64(0), int64(0)
	chargedStops := map[string]bool{}
	for _, sh := range shipments {
		for _, seg := range Arr(sh["segments"]) {
			if S(seg["type"]) == "on_site" {
				continue
			}
			extra := I(seg["extraStopCostMinor"])
			key := S(seg["stopChargeKey"])
			if key != "" {
				_, postedErr := tx.Get("billed-stops", key)
				if chargedStops[key] || postedErr == nil {
					extra = 0
				} else if problem, ok := postedErr.(*Problem); !ok || problem.Status != 404 {
					return nil, nil, postedErr
				}
				chargedStops[key] = true
			}
			seg["extraStopCostMinor"] = extra
			seg["incrementalCostMinor"] = extra
			seg["estimatedCostMinor"], e = CheckedAdd(I(seg["allocatedCostMinor"]), extra)
			if e != nil {
				return nil, nil, e
			}
			costs := []M{{"category": seg["type"], "amountMinor": seg["allocatedCostMinor"]}}
			if extra > 0 {
				costs = append(costs, M{"category": "extra_stop", "amountMinor": extra})
			}
			seg["costItems"] = costs
			totalCost, e = CheckedAdd(totalCost, I(seg["estimatedCostMinor"]))
			if e != nil {
				return nil, nil, e
			}
			marginal, e = CheckedAdd(marginal, extra)
			if e != nil {
				return nil, nil, e
			}
			buyer, e = CheckedAdd(buyer, I(seg["buyerFeeMinor"]))
			if e != nil {
				return nil, nil, e
			}
		}
	}
	totalCost, e = routingProductSum(totalCost, routeRate(tariff, "handling"), int64(len(shipments)))
	if e != nil {
		return nil, nil, e
	}
	marginal, e = routingProductSum(marginal, routeRate(tariff, "handling"), int64(len(shipments)))
	if e != nil {
		return nil, nil, e
	}
	option["incrementalCostMinor"] = marginal
	option["feasible"] = true
	option["reasons"] = []M{reason}
	option["mode"] = mode
	option["estimatedCostMinor"] = totalCost
	option["buyerFeeMinor"] = buyer
	option["readyAt"] = Stamp(ready)
	option["etaMinutes"] = int64(ready.Sub(tx.Now()) / time.Minute)
	option["shipments"] = shipments
	return option, allocations, nil
}
func CreateQuote(tx *Tx, c Command) (M, error) {
	request := c.Body
	pickup := S(request["pickupPointId"])
	if !c.Actor.Point(pickup) {
		return nil, Fail(403, "forbidden", "pickup point outside caller scope")
	}
	point, e := tx.Get("pickup-points", pickup)
	if e != nil {
		return nil, e
	}
	if !B(point["active"]) || !B(point["acceptsOrders"]) {
		return nil, Fail(409, "node_unavailable", "pickup point cannot accept orders")
	}
	items := Arr(request["items"])
	if len(items) < 1 || len(items) > 50 {
		return nil, Fail(422, "invalid_basket", "basket requires 1..50 unique products")
	}
	seen := map[string]bool{}
	prices := []M{}
	basketPrice, basketCost, basketSlots := int64(0), int64(0), int64(0)
	for _, item := range items {
		id := S(item["productId"])
		if seen[id] || I(item["quantity"]) <= 0 {
			return nil, Fail(422, "invalid_basket", "duplicate product or invalid quantity")
		}
		seen[id] = true
		p, e := tx.Get("products", id)
		if e != nil {
			return nil, e
		}
		basketPrice, e = routingProductSum(basketPrice, I(p["salePriceMinor"]), I(item["quantity"]))
		if e != nil {
			return nil, e
		}
		basketCost, e = routingProductSum(basketCost, I(p["unitCostMinor"]), I(item["quantity"]))
		if e != nil {
			return nil, e
		}
		basketSlots, e = routingProductSum(basketSlots, I(p["slotsPerUnit"]), I(item["quantity"]))
		if e != nil {
			return nil, e
		}
		prices = append(prices, M{"productId": id, "sellerId": p["sellerId"], "unitPriceMinor": p["salePriceMinor"], "unitCostMinor": p["unitCostMinor"]})
	}
	tariff, e := routingTariff(tx)
	if e != nil {
		return nil, e
	}
	quote := M{"id": ID(), "request": Clone(request), "createdAt": Stamp(tx.Now()), "expiresAt": Stamp(tx.Now().Add(15 * time.Minute)), "tariffVersionId": tariff["id"], "currency": "RUB"}
	options := []M{}
	allocations := []M{}
	var best M
	for _, policy := range []string{"mesh_only", "hub_only", "hybrid"} {
		opt, items, e := makeQuoteOption(tx, request, policy, tariff)
		if e != nil {
			return nil, e
		}
		if policy == "hybrid" && (!B(opt["feasible"]) || S(opt["mode"]) != "mixed") {
			continue
		}
		options = append(options, opt)
		if B(opt["feasible"]) {
			allocations = append(allocations, M{"optionId": opt["id"], "items": items})
			if best == nil || I(opt["incrementalCostMinor"]) < I(best["incrementalCostMinor"]) || (I(opt["incrementalCostMinor"]) == I(best["incrementalCostMinor"]) && Time(opt["readyAt"]).Before(Time(best["readyAt"]))) {
				best = opt
			}
		}
	}
	quote["options"] = options
	if best != nil {
		quote["recommendedOptionId"] = best["id"]
	}
	if e = tx.Put("quotes", quote); e != nil {
		return nil, e
	}
	snapshot := M{"id": quote["id"], "quote": quote, "prices": prices, "allocations": allocations}
	return quote, tx.Put("quote-snapshots", snapshot)
}
func QuoteMovement(tx *Tx, body M) (M, error) {
	tariff, e := tx.Get("tariffs", S(body["tariffVersionId"]))
	if e != nil {
		return nil, e
	}
	slots := int64(0)
	for _, item := range Arr(body["items"]) {
		p, e := tx.Get("products", S(item["productId"]))
		if e != nil {
			return nil, e
		}
		slots, e = routingProductSum(slots, I(p["slotsPerUnit"]), I(item["quantity"]))
		if e != nil {
			return nil, e
		}
	}
	if slots <= 0 {
		return nil, Fail(422, "invalid_quantity", "movement requires positive slots")
	}
	start := Time(body["notBefore"])
	if start.Before(tx.Now()) {
		start = tx.Now()
	}
	segments, ready, cost, e := FindRoute(tx, S(body["fromNodeId"]), S(body["toNodeId"]), start, slots, tariff, Obj(body["failureTarget"]))
	quote := M{"id": ID(), "expiresAt": Stamp(tx.Now().Add(15 * time.Minute)), "tariffVersionId": tariff["id"], "options": []M{}, "exclusions": []M{}}
	if e != nil {
		if p, ok := e.(*Problem); ok && p.Status == 422 {
			return nil, e
		}
		code := "no_service"
		if p, ok := e.(*Problem); ok {
			code = p.Code
		}
		quote["exclusions"] = []M{routingReason(code, e.Error())}
	} else {
		quote["options"] = []M{{"id": ID(), "estimatedCostMinor": cost, "readyAt": Stamp(ready), "shipments": []M{{"id": ID(), "fromNodeId": body["fromNodeId"], "toNodeId": body["toNodeId"], "contents": body["items"], "readyAt": Stamp(ready), "segments": segments}}, "reasons": []M{routingReason("lower_cost", "scheduled movement available")}}}
	}
	return quote, tx.Put("movement-quotes", quote)
}
func selectedOption(tx *Tx, quoteID, optionID string) (M, M, error) {
	quote, e := tx.Get("quotes", quoteID)
	if e != nil {
		quote, e = tx.Get("movement-quotes", quoteID)
	}
	if e != nil {
		return nil, nil, e
	}
	if !Time(quote["expiresAt"]).After(tx.Now()) {
		return nil, nil, Fail(409, "stale_quote", "quote expired")
	}
	if optionID == "" {
		optionID = S(quote["recommendedOptionId"])
	}
	for _, o := range Arr(quote["options"]) {
		if S(o["id"]) == optionID {
			if f, exists := o["feasible"]; exists && !B(f) {
				return nil, nil, Fail(409, "stale_quote", "option infeasible")
			}
			return quote, o, nil
		}
	}
	return nil, nil, Fail(409, "stale_quote", "selected option unavailable")
}
func HoldRoute(tx *Tx, body M, confirmed bool) (M, error) {
	_, option, e := selectedOption(tx, S(body["quoteId"]), S(body["optionId"]))
	if e != nil {
		return nil, e
	}
	used, e := heldLegs(tx, S(body["replacesHoldId"]))
	if e != nil {
		return nil, e
	}
	legs := []M{}
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
		for _, seg := range Arr(sh["segments"]) {
			if B(seg["completed"]) || S(seg["tripId"]) == "" {
				continue
			}
			trip, e := tx.Get("trips", S(seg["tripId"]))
			if e != nil {
				return nil, e
			}
			key := S(seg["_legKey"])
			if key == "" {
				return nil, Fail(409, "stale_quote", "quote lacks a leg capacity snapshot")
			}
			free, e := routingFree(I(trip["capacitySlots"]), I(trip["backgroundSlots"]), used[key])
			if e != nil {
				return nil, e
			}
			if free < slots {
				return nil, Fail(409, "reservation_conflict", "trip capacity changed; request new quote")
			}
			used[key], e = CheckedAdd(used[key], slots)
			if e != nil {
				return nil, e
			}
			legs = append(legs, M{"key": key, "slots": slots, "segmentId": seg["id"]})
		}
	}
	h := M{"id": ID(), "subjectId": body["subjectId"], "subjectType": body["subjectType"], "status": "held", "expiresAt": Stamp(tx.Now().Add(15 * time.Minute)), "legs": legs, "completedSegments": []string{}}
	if confirmed {
		h["status"] = "confirmed"
	}
	if old := S(body["replacesHoldId"]); old != "" {
		if _, e = UpdateRouteHold(tx, old, M{"type": "release_remaining"}); e != nil {
			return nil, e
		}
	}
	return h, tx.Put("route-holds", h)
}
func UpdateRouteHold(tx *Tx, id string, body M) (M, error) {
	h, e := tx.Get("route-holds", id)
	if e != nil {
		return nil, e
	}
	switch S(body["type"]) {
	case "confirm":
		if S(h["status"]) == "held" && !Time(h["expiresAt"]).After(tx.Now()) {
			return nil, Fail(409, "reservation_conflict", "route hold expired")
		}
		if S(h["status"]) != "held" && S(h["status"]) != "confirmed" {
			return nil, Fail(409, "invalid_transition", "cannot confirm released hold")
		}
		h["status"] = "confirmed"
	case "release_remaining":
		h["status"] = "released"
	case "segment_completed":
		seg := S(body["segmentId"])
		found := false
		for _, leg := range Arr(h["legs"]) {
			if S(leg["segmentId"]) == seg {
				found = true
			}
		}
		if !found {
			return nil, Fail(422, "invalid_segment", "segment not in hold")
		}
		done := Strings(h["completedSegments"])
		if !Contains(done, seg) {
			done = append(done, seg)
		}
		h["completedSegments"] = done
		if len(done) == len(Arr(h["legs"])) {
			h["status"] = "completed"
		}
	default:
		return nil, Fail(422, "invalid_transition", "unknown route hold event")
	}
	return h, tx.Put("route-holds", h)
}
func routeErrorCode(err error) string {
	if p, ok := err.(*Problem); ok {
		return p.Code
	}
	return strings.TrimSpace(fmt.Sprint(err))
}
