package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sort"
	"time"
)

func init() { Register(handleFinance) }

func EffectiveTariff(tx *Tx, at time.Time) (M, error) {
	rows, e := tx.List("tariffs", nil)
	if e != nil {
		return nil, e
	}
	sort.Slice(rows, func(i, j int) bool { return Time(rows[i]["effectiveFrom"]).After(Time(rows[j]["effectiveFrom"])) })
	for _, r := range rows {
		if !at.Before(Time(r["effectiveFrom"])) && (S(r["effectiveTo"]) == "" || at.Before(Time(r["effectiveTo"]))) {
			return r, nil
		}
	}
	return nil, Fail(409, "tariff_unavailable", "no tariff effective at model time")
}
func Rate(t M, code, point string) int64 {
	var fallback int64
	for _, r := range Arr(t["rates"]) {
		if S(r["code"]) != code {
			continue
		}
		if S(r["pointId"]) == point && point != "" {
			return I(r["amount"])
		}
		if S(r["pointId"]) == "" {
			fallback = I(r["amount"])
		}
	}
	return fallback
}
func financeHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Ratios use a wide intermediate so a representable result is not rejected
// merely because multiplying before division would overflow int64.
func financeRatio(amount, quantity, denominator int64) (int64, error) {
	if denominator <= 0 {
		return 0, Fail(422, "validation_error", "financial denominator must be positive")
	}
	n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(quantity))
	n.Quo(n, big.NewInt(denominator))
	if !n.IsInt64() {
		return 0, Fail(422, "validation_error", "financial amount exceeds int64")
	}
	return n.Int64(), nil
}
func financeAdd(values ...int64) (int64, error) {
	var total int64
	for _, v := range values {
		var e error
		total, e = CheckedAdd(total, v)
		if e != nil {
			return 0, e
		}
	}
	return total, nil
}
func makeEntry(tx *Tx, eventID, tariffID, party, partyID, category string, amount int64, extra M) M {
	m := M{"id": ID(), "partyType": party, "partyId": partyID, "category": category, "amountMinor": amount, "bookedAt": Stamp(tx.Now()), "tariffVersionId": tariffID, "sourceEventId": eventID}
	for k, v := range extra {
		m[k] = v
	}
	return m
}
func postEntries(tx *Tx, eventID string, command M, entries []M) (M, error) {
	old, e := tx.Get("postings", eventID)
	if e == nil {
		if S(old["commandHash"]) != financeHash(command) {
			return nil, Fail(409, "idempotency_mismatch", "source event already posted with different facts")
		}
		return old, nil
	}
	if p, ok := e.(*Problem); !ok || p.Status != 404 {
		return nil, e
	}
	posting := M{"id": eventID, "sourceEventId": eventID, "bookedAt": Stamp(tx.Now()), "entries": entries, "commandHash": financeHash(command), "facts": command}
	for _, entry := range entries {
		if e = tx.Put("ledger-entries", entry); e != nil {
			return nil, e
		}
	}
	return posting, tx.Put("postings", posting)
}
func SettleOrder(tx *Tx, order M, eventID string) error {
	tariffID := S(order["tariffVersionId"])
	if tariffID == "" {
		t, e := EffectiveTariff(tx, tx.Now())
		if e != nil {
			return e
		}
		tariffID = S(t["id"])
	}
	tariff, e := tx.Get("tariffs", tariffID)
	if e != nil {
		return e
	}
	facts := []M{}
	for _, sh := range Arr(order["plannedShipments"]) {
		f, e := transportFacts(Arr(sh["segments"]), tariff)
		if e != nil {
			return e
		}
		facts = append(facts, f...)
	}
	lines := []M{}
	for _, line := range Arr(order["lines"]) {
		x := Clone(line)
		x["orderItemId"] = line["id"]
		if _, ok := x["unitCostMinor"]; !ok {
			p, e := tx.Get("products", S(line["productId"]))
			if e != nil {
				return e
			}
			x["unitCostMinor"] = p["unitCostMinor"]
		}
		lines = append(lines, x)
	}
	body := M{"kind": "order_issued", "occurredAt": Stamp(tx.Now()), "tariffVersionId": tariffID, "orderId": order["id"], "buyerFeeMinor": order["buyerFeeMinor"], "pickupPointId": order["pickupPointId"], "saleLines": lines, "operatingFacts": facts}
	_, e = ApplyPosting(tx, eventID, body)
	return e
}

// The journal books the shipment's allocated network expense. Whole-trip and
// marginal costs remain separate traceable facts; only an additional stop is
// a shared expense that must be booked once across orders.
func transportFacts(segments []M, tariff M) ([]M, error) {
	out := []M{}
	for _, seg := range segments {
		extra := I(seg["extraStopCostMinor"])
		if _, ok := seg["extraStopCostMinor"]; !ok {
			var e error
			extra, e = CheckedMul(I(seg["incrementalStops"]), Rate(tariff, "extra_stop", S(seg["toNodeId"])))
			if e != nil {
				return nil, e
			}
		}
		allocated := I(seg["allocatedCostMinor"])
		if _, ok := seg["allocatedCostMinor"]; !ok {
			allocated = I(seg["estimatedCostMinor"]) - extra
		}
		if allocated < 0 || extra < 0 {
			return nil, Fail(422, "validation_error", "negative transport expense")
		}
		base := M{"code": seg["type"], "unit": "operation", "quantity": 1, "mode": seg["mode"], "segmentId": seg["id"], "tripId": seg["tripId"], "allocatedCostMinor": allocated, "wholeTripCostMinor": seg["wholeTripCostMinor"], "incrementalCostMinor": seg["incrementalCostMinor"]}
		// Older snapshots provide costs only as CostItem.category/amountMinor.
		if _, ok := seg["estimatedCostMinor"]; !ok {
			allocated = 0
			for _, cost := range Arr(seg["costItems"]) {
				var e error
				allocated, e = CheckedAdd(allocated, I(cost["amountMinor"]))
				if e != nil {
					return nil, e
				}
			}
			base["allocatedCostMinor"] = allocated
		}
		if allocated > 0 {
			out = append(out, base)
		}
		if extra > 0 {
			key := S(seg["stopChargeKey"])
			if key == "" {
				key = S(seg["tripId"]) + "_" + S(seg["scheduledStartAt"]) + "_" + S(seg["toNodeId"])
			}
			if S(seg["tripId"]) == "" {
				return nil, Fail(422, "validation_error", "additional stop requires trip identity")
			}
			out = append(out, M{"code": "extra_stop", "unit": "stop", "quantity": 1, "mode": seg["mode"], "segmentId": seg["id"], "tripId": seg["tripId"], "stopChargeKey": key, "allocatedCostMinor": extra, "incrementalCostMinor": extra})
		}
	}
	return out, nil
}
func ApplyPosting(tx *Tx, eventID string, body M) (M, error) {
	if old, e := tx.Get("postings", eventID); e == nil {
		if S(old["commandHash"]) != financeHash(body) {
			return nil, Fail(409, "idempotency_mismatch", "event facts differ")
		}
		return old, nil
	}
	entries := []M{}
	kind := S(body["kind"])
	tariffID := S(body["tariffVersionId"])
	if kind == "storage_accrued" {
		tariffID = S(Obj(body["accrual"])["tariffVersionId"])
	}
	tariff, e := tx.Get("tariffs", tariffID)
	if e != nil {
		return nil, e
	}
	add := func(party, partyID, cat string, amount int64, extra M) {
		if amount == 0 {
			return
		}
		entry := makeEntry(tx, eventID, tariffID, party, partyID, cat, amount, extra)
		if S(body["occurredAt"]) != "" {
			entry["bookedAt"] = body["occurredAt"]
		}
		entries = append(entries, entry)
	}
	switch kind {
	case "order_issued":
		orderID := S(body["orderId"])
		for _, line := range Arr(body["saleLines"]) {
			qty := I(line["quantity"])
			if qty <= 0 {
				return nil, Fail(422, "invalid_quantity", "sale quantity must be positive")
			}
			sale, e := CheckedMul(qty, I(line["unitPriceMinor"]))
			if e != nil {
				return nil, e
			}
			cost, e := CheckedMul(qty, I(line["unitCostMinor"]))
			if e != nil {
				return nil, e
			}
			if sale < 0 || cost < 0 {
				return nil, Fail(422, "validation_error", "sale and goods cost must be nonnegative")
			}
			seller := S(line["sellerId"])
			ext := M{"orderId": orderID, "orderItemId": line["orderItemId"], "quantity": qty}
			add("seller", seller, "sale", sale, ext)
			add("seller", seller, "cost_of_goods", -cost, ext)
			commission, e := financeRatio(sale, Rate(tariff, "commission", ""), 10000)
			if e != nil {
				return nil, e
			}
			add("seller", seller, "commission", -commission, ext)
			add("marketplace", "marketplace", "commission", commission, ext)
		}
		add("marketplace", "marketplace", "delivery_fee", I(body["buyerFeeMinor"]), M{"orderId": orderID})
		point := S(body["pickupPointId"])
		if point == "" {
			if order, e := tx.Get("orders", orderID); e == nil {
				point = S(order["pickupPointId"])
			}
		}
		if point != "" {
			reward := Rate(tariff, "issue_reward", point)
			if reward == 0 {
				reward = Rate(tariff, "handling", point)
			}
			add("pvz", point, "operation_reward", reward, M{"orderId": orderID, "pointId": point})
			add("marketplace", "marketplace", "operation_reward", -reward, M{"orderId": orderID, "pointId": point})
		}
	case "return_inspected":
		orderID := S(body["orderId"])
		order, e := tx.Get("orders", orderID)
		if e != nil {
			return nil, e
		}
		original, e := tx.List("ledger-entries", M{"orderId": orderID})
		if e != nil {
			return nil, e
		}
		byItem := map[string]M{}
		for _, line := range Arr(order["lines"]) {
			byItem[S(line["id"])] = line
		}
		previous, e := tx.List("postings", M{"facts": M{"kind": "return_inspected", "orderId": orderID}})
		if e != nil {
			return nil, e
		}
		consumed, restocked := map[string]int64{}, map[string]int64{}
		for _, posting := range previous {
			for _, line := range Arr(Obj(posting["facts"])["lines"]) {
				item := S(line["orderItemId"])
				qty, e := CheckedAdd(I(line["resellableQuantity"]), I(line["quarantinedQuantity"]))
				if e != nil {
					return nil, e
				}
				consumed[item], e = CheckedAdd(consumed[item], qty)
				if e != nil {
					return nil, e
				}
				restocked[item], e = CheckedAdd(restocked[item], I(line["resellableQuantity"]))
				if e != nil {
					return nil, e
				}
			}
		}
		seen := map[string]bool{}
		for _, line := range Arr(body["lines"]) {
			item := S(line["orderItemId"])
			saleLine, ok := byItem[item]
			if !ok || seen[item] {
				return nil, Fail(422, "invalid_return_line", "unknown issued order item")
			}
			seen[item] = true
			qty, e := CheckedAdd(I(line["resellableQuantity"]), I(line["quarantinedQuantity"]))
			if e != nil {
				return nil, e
			}
			cumulative, e := CheckedAdd(consumed[item], qty)
			if e != nil {
				return nil, e
			}
			if I(line["resellableQuantity"]) < 0 || I(line["quarantinedQuantity"]) < 0 || qty <= 0 || cumulative > I(saleLine["quantity"]) {
				return nil, Fail(409, "return_quantity_exceeded", "return exceeds issued quantity")
			}
			for _, old := range original {
				if S(old["orderItemId"]) != item || S(old["originalEntryId"]) != "" {
					continue
				}
				cat := S(old["category"])
				if cat != "sale" && cat != "commission" && cat != "cost_of_goods" {
					continue
				}
				reversed := int64(0)
				for _, r := range original {
					if S(r["originalEntryId"]) == S(old["id"]) {
						reversed, e = CheckedAdd(reversed, -I(r["amountMinor"]))
						if e != nil {
							return nil, e
						}
					}
				}
				amount, e := financeRatio(-I(old["amountMinor"]), cumulative, I(saleLine["quantity"]))
				if e != nil {
					return nil, e
				}
				if cat == "cost_of_goods" {
					good, e := CheckedAdd(restocked[item], I(line["resellableQuantity"]))
					if e != nil {
						return nil, e
					}
					amount, e = financeRatio(-I(old["amountMinor"]), good, I(saleLine["quantity"]))
					if e != nil {
						return nil, e
					}
				}
				amount, e = CheckedAdd(amount, reversed)
				if e != nil {
					return nil, e
				}
				reversedTotal, e := CheckedAdd(abs64(reversed), abs64(amount))
				if e != nil {
					return nil, e
				}
				if reversedTotal > abs64(I(old["amountMinor"])) {
					return nil, Fail(409, "return_quantity_exceeded", "original posting fully reversed")
				}
				add(S(old["partyType"]), S(old["partyId"]), "return_adjustment", amount, M{"orderId": orderID, "returnId": body["returnId"], "originalEntryId": old["id"], "orderItemId": item, "originalCategory": cat})
			}
		}
	case "storage_accrued":
		a := Obj(body["accrual"])
		amount, e := CheckedMul(I(a["billableSlotDays"]), Rate(tariff, "storage_slot_day", S(a["pointId"])))
		if e != nil {
			return nil, e
		}
		ext := M{"batchId": a["batchId"], "periodStart": a["periodStart"], "periodEnd": a["periodEnd"], "billableSlotDays": a["billableSlotDays"]}
		add("seller", S(a["sellerId"]), "storage_fee", -amount, ext)
		add("pvz", S(a["pointId"]), "storage_fee", amount, ext)
	case "logistics_cost":
	default:
		return nil, Fail(422, "invalid_posting_kind", "unsupported posting kind")
	}
	for _, fact := range Arr(body["operatingFacts"]) {
		amount := I(fact["allocatedCostMinor"])
		if _, ok := fact["allocatedCostMinor"]; !ok {
			amount, e = CheckedMul(I(fact["quantity"]), Rate(tariff, S(fact["code"]), S(fact["pointId"])))
			if e != nil {
				return nil, e
			}
		}
		if amount < 0 {
			return nil, Fail(422, "invalid_cost", "operating cost cannot be negative")
		}
		extra := M{}
		for _, key := range []string{"orderId", "returnId", "batchId", "storageCaseId"} {
			if body[key] != nil {
				extra[key] = body[key]
			}
		}
		for _, key := range []string{"tripId", "segmentId", "wholeTripCostMinor", "allocatedCostMinor", "incrementalCostMinor", "stopChargeKey"} {
			if fact[key] != nil {
				extra[key] = fact[key]
			}
		}
		if S(fact["code"]) == "extra_stop" && S(fact["segmentId"]) != "" {
			key := S(fact["stopChargeKey"])
			if key == "" {
				if order, e := tx.Get("orders", S(body["orderId"])); e == nil {
					for _, sh := range Arr(order["plannedShipments"]) {
						for _, seg := range Arr(sh["segments"]) {
							if S(seg["id"]) == S(fact["segmentId"]) {
								key = S(seg["stopChargeKey"])
								if key == "" {
									key = S(seg["tripId"]) + "_" + S(seg["scheduledStartAt"]) + "_" + S(seg["toNodeId"])
								}
							}
						}
					}
				}
			}
			if key == "" {
				key = "segment_" + S(fact["segmentId"])
			}
			if _, e := tx.Get("billed-stops", key); e == nil {
				continue
			}
			if e := tx.Put("billed-stops", M{"id": key, "sourceEventId": eventID, "amountMinor": amount}); e != nil {
				return nil, e
			}
		}
		add("marketplace", "marketplace", "logistics_cost", -amount, extra)
		if S(fact["pointId"]) != "" {
			add("pvz", S(fact["pointId"]), "operating_cost", -amount, extra)
		}
	}
	return postEntries(tx, eventID, body, entries)
}
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func StorageCostForBatch(tx *Tx, batch M, asOf time.Time) (M, error) {
	start := Time(batch["receivedAt"])
	if asOf.Before(start) {
		return nil, Fail(422, "invalid_time", "storage asOf precedes receipt")
	}
	history, e := tx.List("batch-history", M{"batchId": batch["id"]})
	if e != nil {
		return nil, e
	}
	sort.Slice(history, func(i, j int) bool { return Time(history[i]["occurredAt"]).Before(Time(history[j]["occurredAt"])) })
	slots := I(batch["usedSlots"])
	if len(history) > 0 {
		slots = I(history[0]["usedSlots"])
	}
	days := int64(asOf.Sub(start) / (24 * time.Hour))
	slotDays := int64(0)
	for _, h := range history {
		at := Time(h["occurredAt"])
		if at.After(asOf) {
			break
		}
		after := I(h["afterUsedSlots"])
		if _, ok := h["afterUsedSlots"]; !ok {
			return nil, Fail(422, "invalid_stock_history", "afterUsedSlots required for storage accounting")
		}
		removed := slots - after
		if removed > 0 {
			delta, e := CheckedMul(removed, int64(at.Sub(start)/(24*time.Hour)))
			if e != nil {
				return nil, e
			}
			slotDays, e = CheckedAdd(slotDays, delta)
			if e != nil {
				return nil, e
			}
		}
		slots = after
	}
	remaining, e := CheckedMul(slots, days)
	if e != nil {
		return nil, e
	}
	slotDays, e = CheckedAdd(slotDays, remaining)
	if e != nil {
		return nil, e
	}
	posted, e := tx.List("ledger-entries", M{"batchId": batch["id"], "partyType": "seller", "category": "storage_fee"})
	if e != nil {
		return nil, e
	}
	postedMinor := int64(0)
	for _, entry := range posted {
		if !Time(entry["bookedAt"]).After(asOf) {
			postedMinor, e = CheckedAdd(postedMinor, -I(entry["amountMinor"]))
			if e != nil {
				return nil, e
			}
		}
	}
	accrued, e := CheckedMul(slotDays, I(batch["storageRateMinorPerSlotDay"]))
	if e != nil {
		return nil, e
	}
	return M{"batchId": batch["id"], "from": batch["receivedAt"], "asOf": Stamp(asOf), "billableSlotDays": slotDays, "accruedMinor": accrued, "postedMinor": postedMinor, "tariffVersionId": batch["tariffVersionId"], "rateMinorPerSlotDay": batch["storageRateMinorPerSlotDay"]}, nil
}
func AccrueStorage(tx *Tx) error {
	batches, e := tx.List("batches", nil)
	if e != nil {
		return e
	}
	for _, batch := range batches {
		cost, e := StorageCostForBatch(tx, batch, tx.Now())
		if e != nil {
			return e
		}
		rate := I(batch["storageRateMinorPerSlotDay"])
		if rate == 0 {
			continue
		}
		unposted, e := CheckedAdd(I(cost["accruedMinor"]), -I(cost["postedMinor"]))
		if e != nil {
			return e
		}
		delta := unposted / rate
		if delta <= 0 {
			continue
		}
		eventID := "storage_" + S(batch["id"]) + "_" + S(cost["billableSlotDays"])
		body := M{"kind": "storage_accrued", "occurredAt": Stamp(tx.Now()), "accrual": M{"batchId": batch["id"], "pointId": batch["storagePointId"], "sellerId": batch["sellerId"], "tariffVersionId": batch["tariffVersionId"], "periodStart": batch["receivedAt"], "periodEnd": Stamp(tx.Now()), "billableSlotDays": delta}}
		if _, e = ApplyPosting(tx, eventID, body); e != nil {
			return e
		}
	}
	return nil
}
func handleFinance(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "svcGetEffectiveTariff":
		t, e := EffectiveTariff(tx, tx.Now())
		return t, true, e
	case "svcGetTariffVersion":
		t, e := tx.Get("tariffs", c.Params["tariffVersionId"])
		return t, true, e
	case "svcPostLedgerEntries":
		p, e := ApplyPosting(tx, c.Params["sourceEventId"], c.Body)
		return p, true, e
	case "svcGetPostedStorage":
		b, e := tx.Get("batches", c.Params["batchId"])
		if e != nil {
			return nil, true, e
		}
		cost, e := StorageCostForBatch(tx, b, tx.Now())
		return M{"batchId": b["id"], "postedMinor": cost["postedMinor"], "asOf": Stamp(tx.Now())}, true, e
	case "listLedgerEntries", "getPointEconomics", "getSellerEconomics", "getSellerOrderEconomics", "getMarketplaceEconomics":
		m, e := financeRead(tx, c)
		return m, true, e
	}
	return nil, false, nil
}
func financeRead(tx *Tx, c Command) (M, error) {
	party, partyID := "", ""
	switch c.Name {
	case "getPointEconomics":
		party = "pvz"
		partyID = c.Params["pointId"]
		if !c.Actor.Point(partyID) {
			return nil, Fail(403, "forbidden", "point outside scope")
		}
	case "getSellerEconomics", "getSellerOrderEconomics":
		party = "seller"
		partyID = c.Params["sellerId"]
		if !c.Actor.Seller(partyID) {
			return nil, Fail(403, "forbidden", "seller outside scope")
		}
	case "getMarketplaceEconomics":
		if !c.Actor.Has("dispatcher") {
			return nil, Fail(403, "forbidden", "dispatcher required")
		}
		party = "marketplace"
		partyID = "marketplace"
	}
	filters := M{}
	if party != "" {
		filters["partyType"] = party
		filters["partyId"] = partyID
	}
	for _, k := range []string{"partyType", "partyId", "orderId", "batchId", "returnId", "category"} {
		if v := c.Query.Get(k); v != "" {
			filters[k] = v
		}
	}
	if id := c.Params["orderId"]; id != "" {
		filters["orderId"] = id
	}
	rows, e := tx.List("ledger-entries", filters)
	if e != nil {
		return nil, e
	}
	from, to := Time(c.Query.Get("from")), Time(c.Query.Get("to"))
	if to.IsZero() {
		to = tx.Now().Add(time.Nanosecond)
	}
	if from.IsZero() {
		from = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if !from.Before(to) {
		return nil, Fail(422, "invalid_period", "from must precede to")
	}
	selected := []M{}
	sums := map[string]int64{}
	netSums := map[string]int64{}
	for _, r := range rows {
		if !c.Actor.Has("dispatcher") {
			allowed := S(r["partyType"]) == "pvz" && c.Actor.Point(S(r["partyId"])) || S(r["partyType"]) == "seller" && c.Actor.Seller(S(r["partyId"]))
			if !allowed {
				continue
			}
		}
		at := Time(r["bookedAt"])
		if at.Before(from) || !at.Before(to) {
			continue
		}
		selected = append(selected, r)
		key := S(r["category"])
		sums[key], e = CheckedAdd(sums[key], I(r["amountMinor"]))
		if e != nil {
			return nil, e
		}
		if key == "return_adjustment" && S(r["originalCategory"]) != "" {
			key = S(r["originalCategory"])
		}
		netSums[key], e = CheckedAdd(netSums[key], I(r["amountMinor"]))
		if e != nil {
			return nil, e
		}
	}
	if c.Name == "listLedgerEntries" {
		return Page(selected), nil
	}
	breakdown := []M{}
	keys := []string{}
	total := int64(0)
	pos, neg := int64(0), int64(0)
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := sums[k]
		breakdown = append(breakdown, M{"category": k, "amountMinor": v})
		total, e = CheckedAdd(total, v)
		if e != nil {
			return nil, e
		}
		if v > 0 {
			pos, e = CheckedAdd(pos, v)
			if e != nil {
				return nil, e
			}
		} else {
			neg, e = CheckedAdd(neg, -v)
			if e != nil {
				return nil, e
			}
		}
	}
	result := M{"period": M{"from": Stamp(from), "to": Stamp(to)}, "currency": "RUB", "postedThrough": Stamp(tx.Now()), "profitMinor": total, "breakdown": breakdown}
	switch party {
	case "pvz":
		result["pointId"] = partyID
		reward, e := financeAdd(sums["operation_reward"], sums["storage_fee"], sums["bonus"])
		if e != nil {
			return nil, e
		}
		result["rewardMinor"] = reward
		operating, e := CheckedAdd(-sums["operating_cost"], -sums["logistics_cost"])
		if e != nil {
			return nil, e
		}
		result["operatingCostMinor"] = operating
		result["deductionsMinor"] = -sums["penalty"]
		orders := map[string]bool{}
		returns := map[string]bool{}
		for _, r := range selected {
			if S(r["orderId"]) != "" && S(r["category"]) == "operation_reward" {
				orders[S(r["orderId"])] = true
			}
			if S(r["returnId"]) != "" {
				returns[S(r["returnId"])] = true
			}
		}
		result["issuedOrders"] = len(orders)
		result["processedReturns"] = len(returns)
	case "seller":
		result["sellerId"] = partyID
		result["salesMinor"] = netSums["sale"]
		result["goodsCostMinor"] = -netSums["cost_of_goods"]
		accrual, e := CheckedAdd(total, I(result["goodsCostMinor"]))
		if e != nil {
			return nil, e
		}
		result["accrualMinor"] = accrual
		deductions, e := CheckedAdd(I(result["salesMinor"]), -accrual)
		if e != nil {
			return nil, e
		}
		result["deductionsMinor"] = deductions
		if c.Name == "getSellerOrderEconomics" {
			order, e := tx.Get("orders", c.Params["orderId"])
			if e != nil {
				return nil, e
			}
			found := false
			for _, l := range Arr(order["lines"]) {
				if S(l["sellerId"]) == partyID {
					found = true
				}
			}
			if !found {
				return nil, Fail(404, "not_found", "seller has no lines in order")
			}
			result["orderId"] = order["id"]
			result["settlementStatus"] = order["settlementStatus"]
			delete(result, "period")
			delete(result, "postedThrough")
		}
	case "marketplace":
		result["revenueMinor"] = pos
		result["expenseMinor"] = neg
	}
	return result, nil
}
