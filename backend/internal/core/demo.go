package core

import (
	"fmt"
	"sort"
	"time"
)

func init() { Register(demoHandler) }
func demoHandler(tx *Tx, c Command) (M, bool, error) {
	switch c.Name {
	case "listDatasets":
		rows, e := tx.GlobalList("datasets", nil)
		return Page(rows), true, e
	case "getDemoContext":
		m, e := tx.GlobalGet("contexts", c.Actor.Subject)
		if e != nil {
			return nil, true, e
		}
		r, e := tx.GlobalGet("runs", S(m["runId"]))
		return r, true, e
	case "svcGetDemoRun":
		r, e := tx.GlobalGet("runs", c.Params["runId"])
		if e != nil {
			return nil, true, e
		}
		if S(r["subject"]) != c.Actor.Subject && !c.Actor.Has("demo_admin") {
			return nil, true, Fail(403, "forbidden", "run belongs to another demonstration")
		}
		return r, true, nil
	case "createDemoRun":
		if _, e := tx.SQL.Exec(tx.Context, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "context:"+c.Actor.Subject); e != nil {
			return nil, true, e
		}
		context, e := tx.GlobalGet("contexts", c.Actor.Subject)
		if e != nil {
			if p, ok := e.(*Problem); !ok || p.Status != 404 {
				return nil, true, e
			}
			context = M{"id": c.Actor.Subject, "version": int64(0)}
		}
		if I(context["version"]) != I(c.Body["expectedContextVersion"]) {
			return nil, true, Fail(409, "stale_context", "demonstration context changed")
		}
		d, e := tx.GlobalGet("datasets", S(c.Body["datasetId"]))
		if e != nil {
			return nil, true, e
		}
		if S(d["status"]) != "ready" {
			return nil, true, Fail(409, "dataset_unavailable", "dataset is not ready")
		}
		if S(d["datasetScope"]) == "" {
			return nil, true, Fail(422, "dataset_incomplete", "dataset has no imported immutable snapshot")
		}
		version := I(context["version"]) + 1
		r := M{"id": ID(), "datasetId": d["id"], "datasetScope": d["datasetScope"], "datasetVersion": d["version"], "startedAt": Stamp(time.Now()), "clock": d["initialClock"], "initialClock": d["initialClock"], "contextVersion": version, "subject": c.Actor.Subject, "_owner": c.Actor.Subject, "seed": d["seed"]}
		if e = tx.GlobalPut("runs", r); e != nil {
			return nil, true, e
		}
		context["version"] = version
		context["runId"] = r["id"]
		return r, true, tx.GlobalPut("contexts", context)
	case "advanceDemoClock":
		to := Time(c.Body["to"])
		if to.IsZero() || !to.After(tx.Now()) {
			return nil, true, Fail(409, "invalid_clock", "target model time must be after current time")
		}
		if tx.Run == nil {
			return nil, true, Fail(400, "demo_run_required", "clock requires a run")
		}
		if e := advanceModelClock(tx, to, c); e != nil {
			return nil, true, e
		}
		return tx.Run, true, nil
	}
	return nil, false, nil
}
func advanceModelClock(tx *Tx, to time.Time, c Command) error {
	// Advance through scheduled action timestamps so every event observes its own
	// model time; stable IDs resolve coincident actions deterministically.
	actions, e := tx.List("scheduled-actions", nil)
	if e != nil {
		return e
	}
	sort.Slice(actions, func(i, j int) bool {
		a, b := Time(actions[i]["at"]), Time(actions[j]["at"])
		if a.Equal(b) {
			return S(actions[i]["id"]) < S(actions[j]["id"])
		}
		return a.Before(b)
	})
	for _, a := range actions {
		at := Time(a["at"])
		if at.After(to) || B(a["completed"]) {
			continue
		}
		if at.Before(tx.Now()) {
			at = tx.Now()
		}
		tx.Clock = at
		tx.Run["clock"] = Stamp(at)
		if e = AdvanceOrders(tx); e != nil {
			return e
		}
		if e = AdvanceExceptions(tx); e != nil {
			return e
		}
		command := Obj(a["command"])
		name := S(command["name"])
		if name == "" {
			return fmt.Errorf("scheduled action %s has no command", S(a["id"]))
		}
		params := map[string]string{}
		for k, v := range Obj(command["params"]) {
			params[k] = S(v)
		}
		ac := Command{Name: name, Params: params, Body: Obj(command["body"]), Actor: c.Actor, RunID: c.RunID, SimulationID: S(a["id"])}
		if _, e = Dispatch(tx, ac); e != nil {
			return e
		}
		a["completed"] = true
		if e = tx.Put("scheduled-actions", a); e != nil {
			return e
		}
	}
	tx.Clock = to
	tx.Run["clock"] = Stamp(to)
	if e = AdvanceOrders(tx); e != nil {
		return e
	}
	if e = AdvanceExceptions(tx); e != nil {
		return e
	}
	return tx.GlobalPut("runs", tx.Run)
}
