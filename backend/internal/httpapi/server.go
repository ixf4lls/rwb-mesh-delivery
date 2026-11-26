package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"meshdelivery/internal/core"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type mapValues = url.Values
type Server struct {
	Store         *core.Store
	Auth          Auth
	Mux           *http.ServeMux
	AllowedOrigin string
	endpoints     []endpoint
}

func New(store *core.Store, auth Auth, contracts string) (*Server, error) {
	eps, e := loadContracts(contracts)
	if e != nil {
		return nil, e
	}
	s := &Server{Store: store, Auth: auth, Mux: http.NewServeMux(), endpoints: eps}
	for _, ep := range eps {
		prefix := "/api/v1"
		if ep.Internal {
			prefix = "/internal/v1"
		}
		s.Mux.HandleFunc(ep.Method+" "+prefix+ep.Path, func(w http.ResponseWriter, r *http.Request) { s.handle(w, r, ep) })
	}
	s.Mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if e := store.Pool.Ping(r.Context()); e != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		write(w, 200, core.M{"status": "ok"})
	})
	return s, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" && o == s.AllowedOrigin {
		w.Header().Set("Access-Control-Allow-Origin", o)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Expose-Headers", "Location, X-Request-Id")
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Demo-Run-Id")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.WriteHeader(204)
			return
		}
	}
	defer func() {
		if e := recover(); e != nil {
			slog.Error("HTTP panic", "error", e)
			writeProblem(w, fmt.Errorf("handler panic"), core.ID())
		}
	}()
	s.Mux.ServeHTTP(w, r)
}
func write(w http.ResponseWriter, status int, m core.M) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(clean(m))
}
func writeProblem(w http.ResponseWriter, e error, trace string) {
	m := core.ProblemBody(e, trace)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(core.I(m["status"])))
	_ = json.NewEncoder(w).Encode(m)
}
func clean(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := core.M{}
		for k, v := range x {
			if !strings.HasPrefix(k, "_") && k != "datasetScope" {
				m[k] = clean(v)
			}
		}
		return m
	case []core.M:
		a := []any{}
		for _, v := range x {
			a = append(a, clean(v))
		}
		return a
	case []any:
		a := []any{}
		for _, v := range x {
			a = append(a, clean(v))
		}
		return a
	default:
		return v
	}
}
func (s *Server) handle(w http.ResponseWriter, r *http.Request, ep endpoint) {
	trace := core.ID()
	w.Header().Set("X-Request-Id", trace)
	var err error
	defer func() {
		if err != nil {
			slog.Warn("request rejected", "requestId", trace, "runId", r.Header.Get("X-Demo-Run-Id"), "operation", ep.Name, "error", err)
			writeProblem(w, err, trace)
		}
	}()
	who, e := s.Auth.authenticate(r, ep)
	if e != nil {
		err = e
		return
	}
	c := core.Command{Name: ep.Name, Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Params: map[string]string{}, Body: core.M{}, Actor: who, RunID: r.Header.Get("X-Demo-Run-Id"), SimulationID: r.Header.Get("X-Simulation-Action-Id")}
	if c.SimulationID != "" && (!ep.Internal || who.Service != "demo") {
		err = core.Fail(403, "forbidden", "simulation action requires demo service identity")
		return
	}
	needKey := false
	for _, raw := range core.Arr(ep.Definition["parameters"]) {
		p := ep.Spec.resolve(raw)
		name := core.S(p["name"])
		var value string
		switch p["in"] {
		case "path":
			value = r.PathValue(name)
			c.Params[name] = value
		case "query":
			value = c.Query.Get(name)
		case "header":
			value = r.Header.Get(name)
		}
		if name == "Idempotency-Key" {
			needKey = true
		}
		if value == "" {
			if core.B(p["required"]) {
				err = core.Fail(400, "validation_error", "missing "+name)
				return
			}
			continue
		}
		var typed any = value
		schema := ep.Spec.resolve(core.Obj(p["schema"]))
		switch schema["type"] {
		case "integer":
			n, er := strconv.ParseInt(value, 10, 64)
			if er != nil {
				err = core.Fail(400, "validation_error", "invalid "+name)
				return
			}
			typed = n
		case "boolean":
			b, er := strconv.ParseBool(value)
			if er != nil {
				err = core.Fail(400, "validation_error", "invalid "+name)
				return
			}
			typed = b
		}
		if e = ep.Spec.validate(schema, typed, name); e != nil {
			err = e
			return
		}
	}
	if len(c.Query["from"]) > 0 && len(c.Query["to"]) > 0 && !core.Time(c.Query.Get("to")).After(core.Time(c.Query.Get("from"))) {
		err = core.Fail(400, "validation_error", "to must be after from")
		return
	}
	if req := core.Obj(ep.Definition["requestBody"]); len(req) > 0 {
		dec := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024+1))
		dec.UseNumber()
		if e = dec.Decode(&c.Body); e != nil {
			err = core.Fail(400, "validation_error", "invalid JSON object")
			return
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			err = core.Fail(400, "validation_error", "request must contain one JSON object")
			return
		}
		schema := core.Obj(core.Obj(core.Obj(req["content"])["application/json"])["schema"])
		if e = ep.Spec.validate(schema, c.Body, "body"); e != nil {
			err = e
			return
		}
	}
	global := ep.Name == "getCurrentSession" || ep.Name == "listDatasets" || ep.Name == "getDemoContext" || ep.Name == "createDemoRun" || ep.Name == "svcGetDemoRun"
	operationRead := strings.Contains(strings.ToLower(ep.Name), "getoperation") || strings.Contains(ep.Name, "GetOrderOperation") || strings.Contains(ep.Name, "GetExceptionOperation") || strings.Contains(ep.Name, "GetDemoOperation")
	if !global && !operationRead && c.RunID == "" {
		err = core.Fail(400, "demo_run_required", "X-Demo-Run-Id is required")
		return
	}
	runID := c.RunID
	if global || operationRead {
		runID = ""
	}
	tx, e := s.Store.Begin(r.Context(), runID)
	if e != nil {
		err = e
		return
	}
	defer tx.Rollback()
	if owner := core.S(tx.Run["_owner"]); owner != "" && owner != who.Subject && who.Service == "" {
		err = core.Fail(403, "forbidden", "run belongs to another user")
		return
	}
	var result core.M
	status := ep.Status
	if needKey {
		key := r.Header.Get("Idempotency-Key")
		hashData, _ := json.Marshal(core.M{"body": c.Body, "query": c.Query, "simulationId": c.SimulationID})
		hash := sha256.Sum256(hashData)
		fingerprint := hex.EncodeToString(hash[:])
		lock := who.Subject + "|" + c.RunID + "|" + r.Method + "|" + r.URL.Path + "|" + key
		if _, e = tx.SQL.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", lock); e != nil {
			err = e
			return
		}
		var oldhash string
		var oldbody []byte
		e = tx.SQL.QueryRow(r.Context(), `SELECT hash,status,body FROM mesh_idempotency WHERE subject=$1 AND scope=$2 AND method=$3 AND path=$4 AND key=$5`, who.Subject, c.RunID, r.Method, r.URL.Path, key).Scan(&oldhash, &status, &oldbody)
		if e == nil {
			if oldhash != fingerprint {
				err = core.Fail(409, "idempotency_mismatch", "key reused with different request")
				return
			}
			if e = core.Decode(oldbody, &result); e != nil {
				err = e
				return
			}
			if status == 202 {
				w.Header().Set("Location", operationLocation(ep, core.S(result["id"])))
			}
			write(w, status, result)
			return
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			err = e
			return
		}
		status = ep.Status
		if status == 202 {
			result, e = enqueue(tx, c)
		} else {
			result, e = core.Dispatch(tx, c)
		}
		if e != nil {
			err = e
			return
		}
		encoded, _ := json.Marshal(result)
		_, e = tx.SQL.Exec(r.Context(), `INSERT INTO mesh_idempotency(subject,scope,method,path,key,hash,status,body) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, who.Subject, c.RunID, r.Method, r.URL.Path, key, fingerprint, status, encoded)
		if e != nil {
			err = e
			return
		}
	} else if ep.Name == "getCurrentSession" {
		result = core.M{"userId": who.Subject, "roles": who.Roles, "pvzIds": who.PointIDs, "sellerIds": who.SellerIDs}
	} else if operationRead {
		result, e = tx.GlobalGet("operations", c.Params["operationId"])
		if e == nil && (core.S(result["_subject"]) != who.Subject || (c.RunID != "" && c.RunID != core.S(result["_runId"]))) {
			e = core.Fail(403, "forbidden", "operation outside caller scope")
		}
	} else {
		result, e = core.Dispatch(tx, c)
	}
	if e != nil {
		err = e
		return
	}
	if rows, ok := result["items"]; ok {
		result, e = paginate(c, result, core.Arr(rows))
		if e != nil {
			err = e
			return
		}
	}
	if e = tx.Commit(); e != nil {
		err = e
		return
	}
	if status == 202 {
		w.Header().Set("Location", operationLocation(ep, core.S(result["id"])))
	}
	slog.Info("request completed", "requestId", trace, "runId", c.RunID, "operation", ep.Name, "resourceId", result["id"], "status", status)
	write(w, status, result)
}
func operationLocation(ep endpoint, id string) string {
	if !ep.Internal {
		return "/api/v1/operations/" + id
	}
	owner := strings.Split(strings.TrimPrefix(ep.Path, "/"), "/")[0]
	return "/internal/v1/" + owner + "/operations/" + id
}
func enqueue(tx *core.Tx, c core.Command) (core.M, error) {
	resource := "order"
	prefix := "ord_"
	switch {
	case strings.Contains(strings.ToLower(c.Name), "return"):
		resource = "return"
		prefix = "exc_"
	case strings.Contains(strings.ToLower(c.Name), "incident"):
		resource = "incident"
		prefix = "exc_"
	case strings.Contains(strings.ToLower(c.Name), "storage"):
		resource = "storage_case"
		prefix = "exc_"
	case strings.Contains(strings.ToLower(c.Name), "clock"):
		resource = "demo_run"
		prefix = "demo_"
	}
	id := prefix + core.ID()
	now := core.Stamp(time.Now())
	op := core.M{"id": id, "status": "queued", "createdAt": now, "updatedAt": now, "resourceType": resource, "_subject": c.Actor.Subject, "_runId": c.RunID}
	if e := tx.GlobalPut("operations", op); e != nil {
		return nil, e
	}
	return op, tx.GlobalPut("jobs", core.M{"id": id, "status": "queued", "createdAt": now, "command": c})
}
func paginate(c core.Command, page core.M, rows []core.M) (core.M, error) {
	q := make(url.Values)
	for k, v := range c.Query {
		q[k] = append([]string(nil), v...)
	}
	if q.Get("cursor") == "" && c.Body["cursor"] != nil {
		q.Set("cursor", core.S(c.Body["cursor"]))
	}
	if q.Get("limit") == "" && c.Body["limit"] != nil {
		q.Set("limit", core.S(c.Body["limit"]))
	}
	rawCursor := q.Get("cursor")
	q.Del("cursor")
	q.Del("limit")
	body := core.Clone(c.Body)
	delete(body, "cursor")
	delete(body, "limit")
	bound, _ := json.Marshal(body)
	hash := sha256.Sum256([]byte(c.Actor.Subject + "|" + c.RunID + "|" + c.Path + "|" + q.Encode() + "|" + string(bound)))
	scope := hex.EncodeToString(hash[:])
	offset := int64(0)
	if rawCursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(rawCursor)
		var cursor core.M
		if e != nil || core.Decode(b, &cursor) != nil || cursor["scope"] != scope {
			return nil, core.Fail(400, "invalid_cursor", "cursor belongs to different query")
		}
		offset = core.I(cursor["offset"])
		if offset < 0 || offset > int64(len(rows)) {
			return nil, core.Fail(400, "invalid_cursor", "invalid cursor offset")
		}
	}
	limit := int64(50)
	if v := c.Query.Get("limit"); v != "" {
		limit = core.I(v)
	} else if c.Body["limit"] != nil {
		limit = core.I(c.Body["limit"])
	}
	if limit < 1 || limit > 1000 {
		return nil, core.Fail(400, "validation_error", "invalid page limit")
	}
	end := offset + limit
	delete(page, "nextCursor")
	if end < int64(len(rows)) {
		b, _ := json.Marshal(core.M{"scope": scope, "offset": end})
		page["nextCursor"] = base64.RawURLEncoding.EncodeToString(b)
	} else {
		end = int64(len(rows))
	}
	page["items"] = rows[offset:end]
	return page, nil
}
