package core

import (
	"context"
	"testing"
)

func TestDemoModelClockAccruesStorageAndRejectsBackwards(t *testing.T) {
	s := inventoryTestStore(t)
	run := inventoryTestRun(t, s)
	tx, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	actor := Actor{Subject: S(tx.Run["subject"]), Roles: []string{"dispatcher", "demo_admin"}}
	to := "2026-01-15T09:00:00Z"
	if _, e = Dispatch(tx, Command{Name: "advanceDemoClock", RunID: run, Actor: actor, Body: M{"to": to}}); e != nil {
		t.Fatal(e)
	}
	cost, e := Dispatch(tx, Command{Name: "getStorageCost", Actor: actor, Params: map[string]string{"batchId": "45"}})
	if e != nil {
		t.Fatal(e)
	}
	if I(cost["billableSlotDays"]) != 50 || I(cost["accruedMinor"]) != 10000 {
		t.Fatalf("unexpected slow batch accrual: %v", cost)
	}
	if _, e = Dispatch(tx, Command{Name: "advanceDemoClock", RunID: run, Actor: actor, Body: M{"to": "2026-01-05T09:00:00Z"}}); e == nil {
		t.Fatal("backwards clock accepted")
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	persisted, e := s.Begin(context.Background(), run)
	if e != nil {
		t.Fatal(e)
	}
	defer persisted.Rollback()
	if Stamp(persisted.Now()) != to {
		t.Fatal("clock did not survive reopening transaction")
	}
}
