package httpapi

import (
	"context"
	"fmt"
	"meshdelivery/internal/core"
	"testing"
)

func TestWorkerRestartRollsBackWholeCommand(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	q := a.quote("104", "11", 1)
	o := a.order(q, "mesh_only")
	o = a.event(o, "packed", core.S(core.Arr(o["shipments"])[0]["id"]), "")
	code, op, _ := a.request("POST", "/api/v1/orders/"+core.S(o["id"])+"/events", core.M{"type": "issued", "expectedVersion": o["version"]}, core.ID())
	if code != 202 {
		t.Fatal(op)
	}
	ctx := context.Background()
	name := "crash_" + core.ID()
	sql := fmt.Sprintf(`CREATE SEQUENCE %s;CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.kind='operations' AND NEW.id='%s' AND NEW.data->>'status'='succeeded' AND nextval('%s')=1 THEN RAISE EXCEPTION 'synthetic worker crash'; END IF; RETURN NEW; END $$;CREATE TRIGGER %s BEFORE UPDATE ON mesh_records FOR EACH ROW EXECUTE FUNCTION %s();`, name, name, op["id"], name, name, name)
	if _, e := a.db.Pool.Exec(ctx, sql); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = a.db.Pool.Exec(ctx, fmt.Sprintf("DROP TRIGGER %s ON mesh_records;DROP FUNCTION %s();DROP SEQUENCE %s;", name, name, name))
	})
	if _, e := a.db.ProcessOne(ctx); e == nil {
		t.Fatal("expected synthetic crash before operation commit")
	}
	before := a.call("GET", "/api/v1/batches/44", nil, 200)
	if core.I(before["packedQuantity"]) != 1 || core.I(before["usedSlots"]) != 10 {
		t.Fatalf("crashed command leaked stock mutation %v", before)
	}
	if _, e := a.db.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := a.db.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	status := a.call("GET", "/api/v1/operations/"+core.S(op["id"]), nil, 200)
	if status["status"] != "succeeded" {
		t.Fatal(status)
	}
	after := a.call("GET", "/api/v1/batches/44", nil, 200)
	if core.I(after["packedQuantity"]) != 0 || core.I(after["usedSlots"]) != 9 {
		t.Fatalf("restart duplicated or lost issue %v", after)
	}
	events := a.call("GET", "/api/v1/orders/"+core.S(o["id"])+"/events", nil, 200)
	issued := 0
	for _, e := range core.Arr(events["items"]) {
		if e["type"] == "issued" {
			issued++
		}
	}
	if issued != 1 {
		t.Fatalf("issued events=%d", issued)
	}
}
