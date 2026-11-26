package httpapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"meshdelivery/internal/core"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type apiFixture struct {
	t                   *testing.T
	s                   *Server
	db                  *core.Store
	token, run, subject string
}

func apiTest(t *testing.T) *apiFixture {
	t.Helper()
	db, e := core.TestStore()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	auth := Auth{UserKey: []byte(strings.Repeat("u", 32)), ServiceKey: []byte(strings.Repeat("s", 32)), Issuer: "test"}
	s, e := New(db, auth, "../../../docs/openapi")
	if e != nil {
		t.Fatal(e)
	}
	a := &apiFixture{t: t, s: s, db: db, subject: "test_" + core.ID()}
	a.token = Token(auth.UserKey, core.M{"sub": a.subject, "iss": "test", "aud": "mesh-user", "exp": time.Now().Add(time.Hour).Unix(), "roles": []string{"dispatcher", "demo_admin"}, "pvzIds": []string{}, "sellerIds": []string{}})
	if _, e = core.ImportDataset(context.Background(), db, "../../testdata/demo"); e != nil {
		t.Fatal(e)
	}
	return a
}
func (a *apiFixture) request(method, path string, body core.M, key string) (int, core.M, http.Header) {
	a.t.Helper()
	b := ""
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			a.t.Fatal(e)
		}
		b = string(raw)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+a.token)
	if a.run != "" {
		r.Header.Set("X-Demo-Run-Id", a.run)
	}
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	a.s.ServeHTTP(w, r)
	m := core.M{}
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		a.t.Fatalf("%s %s invalid response %d %s", method, path, w.Code, w.Body.String())
	}
	return w.Code, m, w.Header()
}
func (a *apiFixture) call(method, path string, body core.M, want int) core.M {
	a.t.Helper()
	code, m, _ := a.request(method, path, body, core.ID())
	if code != want {
		a.t.Fatalf("%s %s status=%d want=%d body=%v", method, path, code, want, m)
	}
	if code < 300 {
		a.validate(method, path, m)
	}
	return m
}
func (a *apiFixture) validate(method, path string, m core.M) {
	a.t.Helper()
	for _, ep := range a.s.endpoints {
		prefix := "/api/v1"
		if ep.Internal {
			prefix = "/internal/v1"
		}
		if ep.Method != method {
			continue
		}
		pattern := strings.Split(prefix+ep.Path, "/")
		parts := strings.Split(strings.Split(path, "?")[0], "/")
		if len(pattern) != len(parts) {
			continue
		}
		match := true
		for j, p := range pattern {
			if !strings.HasPrefix(p, "{") && p != parts[j] {
				match = false
			}
		}
		if match {
			raw, _ := json.Marshal(clean(m))
			var normalized any
			_ = json.Unmarshal(raw, &normalized)
			if e := ep.Spec.validate(ep.Schema, normalized, "response"); e != nil {
				a.t.Fatalf("%s %s response violates contract: %v; body=%v", method, path, e, m)
			}
			return
		}
	}
	a.t.Fatalf("no contract for %s %s", method, path)
}
func (a *apiFixture) newRun(version int) core.M {
	a.t.Helper()
	r := a.call("POST", "/api/v1/demo/runs", core.M{"datasetId": "backend_demo", "expectedContextVersion": version}, 201)
	a.run = core.S(r["id"])
	return r
}
func (a *apiFixture) async(path string, body core.M) core.M {
	a.t.Helper()
	op := a.call("POST", path, body, 202)
	for n := 0; n < 100; n++ {
		status := a.call("GET", "/api/v1/operations/"+core.S(op["id"]), nil, 200)
		if status["status"] == "succeeded" {
			return status
		}
		if status["status"] == "failed" {
			a.t.Fatalf("%s operation failed: %v", path, status)
		}
		if _, e := a.db.ProcessOne(context.Background()); e != nil {
			a.t.Fatal(e)
		}
	}
	a.t.Fatal("worker did not finish operation")
	return nil
}
func (a *apiFixture) clock(to string) {
	a.t.Helper()
	a.async("/api/v1/demo/clock/advances", core.M{"to": to})
}
func (a *apiFixture) quote(product, pickup string, quantity int) core.M {
	return a.call("POST", "/api/v1/quotes", core.M{"pickupPointId": pickup, "items": []core.M{{"productId": product, "quantity": quantity}}}, 201)
}
func (a *apiFixture) order(q core.M, option string) core.M {
	a.t.Helper()
	var selected string
	for _, candidate := range core.Arr(q["options"]) {
		if candidate["policy"] == option && core.B(candidate["feasible"]) {
			selected = core.S(candidate["id"])
		}
	}
	if selected == "" {
		a.t.Fatalf("no feasible %s: %v", option, q)
	}
	op := a.async("/api/v1/orders", core.M{"quoteId": q["id"], "optionId": selected})
	return a.call("GET", "/api/v1/orders/"+core.S(op["resourceId"]), nil, 200)
}
func (a *apiFixture) event(o core.M, typ, shipment, segment string) core.M {
	a.t.Helper()
	body := core.M{"type": typ, "expectedVersion": o["version"]}
	if shipment != "" {
		body["shipmentId"] = shipment
	}
	if segment != "" {
		body["segmentId"] = segment
	}
	a.async("/api/v1/orders/"+core.S(o["id"])+"/events", body)
	return a.call("GET", "/api/v1/orders/"+core.S(o["id"]), nil, 200)
}
func TestDeliveryHTTPPostgres(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	q := a.quote("101", "13", 1)
	o := a.order(q, "mesh_only")
	shipment := core.S(core.Arr(o["shipments"])[0]["id"])
	o = a.event(o, "packed", shipment, "")
	before := a.call("GET", "/api/v1/batches/41", nil, 200)
	if core.I(before["availableQuantity"]) != 4 || core.I(before["packedQuantity"]) != 1 {
		t.Fatalf("packing must retain physical stock: %v", before)
	}
	a.clock("2026-01-05T12:00:00Z")
	o = a.event(o, "handed_over", shipment, core.S(core.Arr(o["shipments"])[0]["nextSegmentId"]))
	a.clock("2026-01-05T15:00:00Z")
	o = a.event(o, "arrived", shipment, core.S(core.Arr(o["shipments"])[0]["activeSegmentId"]))
	o = a.event(o, "issued", "", "")
	if o["status"] != "issued" {
		t.Fatalf("order not issued %v", o)
	}
	b := a.call("GET", "/api/v1/batches/41", nil, 200)
	if core.I(b["availableQuantity"]) != 4 || core.I(b["packedQuantity"]) != 0 {
		t.Fatalf("wrong final stock %v", b)
	}
	ledger := a.call("GET", "/api/v1/ledger-entries?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z&orderId="+core.S(o["id"]), nil, 200)
	if len(core.Arr(ledger["items"])) == 0 {
		t.Fatal("issue created no ledger postings")
	}
	oldRun := a.run
	a.newRun(1)
	fresh := a.call("GET", "/api/v1/batches/41", nil, 200)
	if core.I(fresh["availableQuantity"]) != 5 {
		t.Fatalf("new run inherited changes %v", fresh)
	}
	a.run = oldRun
	still := a.call("GET", "/api/v1/batches/41", nil, 200)
	if core.I(still["availableQuantity"]) != 4 {
		t.Fatal("old run changed")
	}
}
func TestLocalIssueIdempotencyAndAuthorization(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	q := a.quote("104", "11", 1)
	o := a.order(q, "mesh_only")
	shipment := core.S(core.Arr(o["shipments"])[0]["id"])
	o = a.event(o, "packed", shipment, "")
	b := a.call("GET", "/api/v1/batches/44", nil, 200)
	if core.I(b["packedQuantity"]) != 1 {
		t.Fatalf("local stock left before issue %v", b)
	}
	path := "/api/v1/orders/" + core.S(o["id"]) + "/events"
	body := core.M{"type": "issued", "expectedVersion": o["version"]}
	key := core.ID()
	code, op, h := a.request("POST", path, body, key)
	if code != 202 || h.Get("Location") == "" {
		t.Fatalf("missing operation location %d %v", code, h)
	}
	if _, e := a.db.ProcessOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, replay, _ := a.request("POST", path, body, key)
	if code != 202 || replay["id"] != op["id"] {
		t.Fatalf("idempotency failed %v", replay)
	}
	body["expectedVersion"] = 99
	code, m, _ := a.request("POST", path, body, key)
	if code != 409 || m["code"] != "idempotency_mismatch" {
		t.Fatalf("mismatch not rejected %d %v", code, m)
	}
	b = a.call("GET", "/api/v1/batches/44", nil, 200)
	if core.I(b["availableQuantity"]) != 9 || core.I(b["packedQuantity"]) != 0 {
		t.Fatalf("duplicate issue altered stock %v", b)
	}
	a.token = Token(a.s.Auth.UserKey, core.M{"sub": a.subject, "iss": "test", "aud": "mesh-user", "exp": time.Now().Add(time.Hour).Unix(), "roles": []string{"pvz_operator"}, "pvzIds": []string{"12"}, "sellerIds": []string{}})
	a.call("GET", "/api/v1/orders/"+core.S(o["id"]), nil, 403)
	a.call("GET", "/api/v1/batches/44", nil, 403)
	a.call("GET", "/api/v1/pickup-points/11/capacity", nil, 403)
}
func TestAuthAndValidation(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
	}{{"bad quantity", "POST", "/api/v1/quotes", `{"pickupPointId":"13","items":[{"productId":"101","quantity":0}]}`, 400}, {"bad enum", "GET", "/api/v1/orders?status=made_up", "", 400}, {"fractional limit", "GET", "/api/v1/products?limit=1.2", "", 400}, {"invalid range", "GET", "/api/v1/marketplace/economics?from=2026-01-06T00:00:00Z&to=2026-01-05T00:00:00Z", "", 400}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+a.token)
			r.Header.Set("X-Demo-Run-Id", a.run)
			r.Header.Set("Idempotency-Key", core.ID())
			w := httptest.NewRecorder()
			a.s.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	a.token += "tamper"
	a.call("GET", "/api/v1/me", nil, 401)
}
func TestInternalGatewayRequiresDelegation(t *testing.T) {
	a := apiTest(t)
	claims := core.M{"sub": "gateway", "aud": "mesh-internal", "iss": "test", "exp": time.Now().Add(time.Hour).Unix()}
	a.token = Token(a.s.Auth.ServiceKey, claims)
	a.call("GET", "/internal/v1/demo/datasets", nil, 403)
	r := httptest.NewRequest("GET", "/internal/v1/demo/datasets", nil)
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("X-User-Context", Token(a.s.Auth.UserKey, core.M{"sub": a.subject, "aud": "mesh-user", "iss": "test", "exp": time.Now().Add(time.Hour).Unix(), "roles": []string{"demo_admin"}}))
	w := httptest.NewRecorder()
	a.s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("signed delegation failed: %d %s", w.Code, w.Body.String())
	}
	claims["sub"] = "finance"
	a.token = Token(a.s.Auth.ServiceKey, claims)
	a.call("GET", "/internal/v1/demo/datasets", nil, 403)
}
func TestContractOperationInventory(t *testing.T) {
	eps, e := loadContracts("../../../docs/openapi")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("loaded %d documented operations", len(eps))
	if len(eps) < 100 {
		t.Fatalf("unexpected operation inventory: %d", len(eps))
	}
}
func TestUserCannotSpoofSimulationIdentity(t *testing.T) {
	a := apiTest(t)
	a.newRun(0)
	r := httptest.NewRequest("POST", "/api/v1/orders/missing/events", strings.NewReader(`{"type":"packed","shipmentId":"missing","expectedVersion":1}`))
	r.Header.Set("Authorization", "Bearer "+a.token)
	r.Header.Set("Idempotency-Key", core.ID())
	r.Header.Set("X-Demo-Run-Id", a.run)
	r.Header.Set("X-Simulation-Action-Id", "invented_action")
	w := httptest.NewRecorder()
	a.s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("user supplied simulation authority: %d %s", w.Code, w.Body.String())
	}
}
func TestAllDocumentedOperationsHaveHandlers(t *testing.T) {
	eps, e := loadContracts("../../../docs/openapi")
	if e != nil {
		t.Fatal(e)
	}
	packages, e := parser.ParseDir(token.NewFileSet(), "../core", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	cases := map[string]bool{"getCurrentSession": true, "getOperation": true, "svcGetOrderOperation": true, "svcGetExceptionOperation": true, "svcGetDemoOperation": true}
	for _, pkg := range packages {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				c, ok := n.(*ast.CaseClause)
				if !ok {
					return true
				}
				for _, v := range c.List {
					if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						s, e := strconv.Unquote(lit.Value)
						if e == nil {
							cases[s] = true
						}
					}
				}
				return true
			})
		}
	}
	for _, ep := range eps {
		if !cases[ep.Name] {
			t.Errorf("no implementation branch for %s %s (%s)", ep.Method, ep.Path, ep.Name)
		}
	}
}
