package core

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"time"
)

//go:embed schema.sql
var Schema string

type Store struct{ Pool *pgxpool.Pool }
type Tx struct {
	SQL     pgx.Tx
	Context context.Context
	Scope   string
	Dataset string
	Run     M
	Clock   time.Time
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return &Store{p}, nil
}
func (s *Store) Migrate(ctx context.Context) error { _, e := s.Pool.Exec(ctx, Schema); return e }
func (s *Store) Close()                            { s.Pool.Close() }
func (s *Store) Begin(ctx context.Context, runID string) (*Tx, error) {
	q, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	t := &Tx{SQL: q, Context: ctx, Scope: "global", Clock: time.Now().UTC()}
	if runID != "" {
		if _, e = q.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", runID); e != nil {
			q.Rollback(ctx)
			return nil, e
		}
		r, e := t.Get("runs", runID)
		if e != nil {
			q.Rollback(ctx)
			return nil, e
		}
		t.Run = r
		t.Scope = "run:" + runID
		t.Dataset = S(r["datasetScope"])
		t.Clock = Time(r["clock"])
	}
	return t, nil
}
func (t *Tx) Commit() error  { return t.SQL.Commit(t.Context) }
func (t *Tx) Rollback()      { _ = t.SQL.Rollback(t.Context) }
func (t *Tx) Now() time.Time { return t.Clock }
func (t *Tx) Get(kind, id string) (M, error) {
	var b []byte
	e := t.SQL.QueryRow(t.Context, `SELECT data FROM mesh_records WHERE kind=$1 AND id=$2 AND scope IN ($3,$4) ORDER BY (scope=$3) DESC LIMIT 1`, kind, id, t.Scope, t.Dataset).Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, Fail(404, "not_found", kind+"/"+id)
	}
	if e != nil {
		return nil, e
	}
	var m M
	e = Decode(b, &m)
	if B(m["_deleted"]) {
		return nil, Fail(404, "not_found", kind+"/"+id)
	}
	return m, e
}
func (t *Tx) Put(kind string, m M) error {
	if S(m["id"]) == "" {
		return fmt.Errorf("missing %s id", kind)
	}
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	_, e = t.SQL.Exec(t.Context, `INSERT INTO mesh_records(scope,kind,id,data) VALUES($1,$2,$3,$4) ON CONFLICT(scope,kind,id) DO UPDATE SET data=excluded.data`, t.Scope, kind, S(m["id"]), b)
	return e
}
func (t *Tx) Delete(kind, id string) error { return t.Put(kind, M{"id": id, "_deleted": true}) }
func (t *Tx) List(kind string, filters M) ([]M, error) {
	if filters == nil {
		filters = M{}
	}
	return t.listMatching(kind, []M{filters})
}
func (t *Tx) TripsFrom(nodeID string) ([]M, error) {
	return t.listMatching("trips", []M{{"fromNodeId": nodeID}, {"stops": []M{{"nodeId": nodeID}}}})
}
func (t *Tx) listMatching(kind string, filters []M) ([]M, error) {
	args := []any{kind, t.Scope, t.Dataset}
	predicates := []string{}
	for _, filter := range filters {
		b, e := json.Marshal(filter)
		if e != nil {
			return nil, e
		}
		args = append(args, b)
		predicates = append(predicates, fmt.Sprintf("r.data @> $%d::jsonb", len(args)))
	}
	predicate := "(" + strings.Join(predicates, " OR ") + ")"
	sql := `SELECT data FROM (
 SELECT r.data FROM mesh_records r WHERE r.kind=$1 AND r.scope=$2 AND ` + predicate + ` AND NOT COALESCE((r.data->>'_deleted')::boolean,false)
 UNION ALL
 SELECT r.data FROM mesh_records r WHERE r.kind=$1 AND r.scope=$3 AND r.scope<>$2 AND ` + predicate + ` AND NOT COALESCE((r.data->>'_deleted')::boolean,false) AND NOT EXISTS(SELECT 1 FROM mesh_records overlay WHERE overlay.scope=$2 AND overlay.kind=$1 AND overlay.id=r.id)
 ) found ORDER BY data->>'id'`
	rows, e := t.SQL.Query(t.Context, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []M{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var m M
		if e = Decode(b, &m); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (t *Tx) GlobalGet(kind, id string) (M, error) {
	s, d := t.Scope, t.Dataset
	t.Scope = "global"
	t.Dataset = ""
	defer func() { t.Scope = s; t.Dataset = d }()
	return t.Get(kind, id)
}
func (t *Tx) GlobalPut(kind string, m M) error {
	s := t.Scope
	t.Scope = "global"
	defer func() { t.Scope = s }()
	return t.Put(kind, m)
}
func (t *Tx) GlobalList(kind string, filters M) ([]M, error) {
	s, d := t.Scope, t.Dataset
	t.Scope = "global"
	t.Dataset = ""
	defer func() { t.Scope = s; t.Dataset = d }()
	return t.List(kind, filters)
}

// TestStore refuses ordinary databases: tests are limited to an explicitly marked local synthetic database.
func validateTestDSN(dsn string) error {
	if dsn == "" {
		return errors.New("MESH_TEST_DATABASE_URL is required")
	}
	c, e := pgx.ParseConfig(dsn)
	if e != nil {
		return e
	}
	local := func(host string) bool { return host == "127.0.0.1" || host == "localhost" || host == "::1" }
	if !local(c.Host) || c.Database != "mesh_test" || c.User != "mesh_test" {
		return errors.New("tests require local mesh_test database and user")
	}
	for _, f := range c.Fallbacks {
		if !local(f.Host) {
			return errors.New("external test database fallback forbidden")
		}
	}
	return nil
}
func TestStore() (*Store, error) {
	dsn := os.Getenv("MESH_TEST_DATABASE_URL")
	if e := validateTestDSN(dsn); e != nil {
		return nil, e
	}
	s, e := Open(context.Background(), dsn)
	if e != nil {
		return nil, e
	}
	if e = s.Migrate(context.Background()); e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
