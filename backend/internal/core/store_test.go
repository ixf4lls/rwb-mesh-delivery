package core

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func TestInt64MoneySurvivesStorage(t *testing.T) {
	s, e := TestStore()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	tx, e := s.Begin(context.Background(), "")
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	tx.Scope = "test:" + ID()
	const amount int64 = 9007199254740993
	m := M{"id": "price", "amount": amount}
	if e = tx.Put("prices", m); e != nil {
		t.Fatal(e)
	}
	got, e := tx.Get("prices", "price")
	if e != nil {
		t.Fatal(e)
	}
	if I(got["amount"]) != amount || I(Clone(got)["amount"]) != amount {
		t.Fatalf("money lost precision: %v", got)
	}
	rows, e := tx.List("prices", nil)
	if e != nil || len(rows) != 1 || I(rows[0]["amount"]) != amount {
		t.Fatalf("listing lost precision %v %v", rows, e)
	}
	if I(json.Number("1e3")) != 1000 {
		t.Fatal("exponent integer changed value")
	}
}
func TestMigrateFreshSchema(t *testing.T) {
	s, e := TestStore()
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	schema := "migration_" + ID()
	if _, e = s.Pool.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer func() { _, _ = s.Pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	dsn, e := url.Parse(os.Getenv("MESH_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	q := dsn.Query()
	q.Set("search_path", schema)
	dsn.RawQuery = q.Encode()
	isolated, e := Open(ctx, dsn.String())
	if e != nil {
		t.Fatal(e)
	}
	defer isolated.Close()
	for n := 0; n < 2; n++ {
		if e = isolated.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if e = isolated.Pool.QueryRow(ctx, "SELECT count(*) FROM mesh_records").Scan(&count); e != nil || count != 0 {
		t.Fatalf("fresh schema invalid count=%d err=%v", count, e)
	}
}
func TestCheckedArithmetic(t *testing.T) {
	for _, tc := range []struct {
		a, b     int64
		mul, add bool
	}{{1 << 62, 4, true, false}, {-1 << 63, -1, true, true}, {1<<63 - 1, 1, false, true}, {9007199254740993, 1, false, false}} {
		_, me := CheckedMul(tc.a, tc.b)
		_, ae := CheckedAdd(tc.a, tc.b)
		if (me != nil) != tc.mul || (ae != nil) != tc.add {
			t.Fatalf("%d %d multiplication=%v addition=%v", tc.a, tc.b, me, ae)
		}
	}
}
func TestDatabaseIsolationBoundary(t *testing.T) {
	for _, tc := range []struct {
		dsn     string
		allowed bool
	}{{"", false}, {"postgres://mesh_test:mesh_test@127.0.0.1:55432/mesh_test?sslmode=disable", true}, {"postgres://localhost:mesh_test@external.example/mesh_test", false}, {"postgres://mesh_test:localhost@external.example/mesh_test", false}, {"postgres://mesh_test:mesh_test@127.0.0.1/production", false}, {"host=localhost,external.example user=mesh_test dbname=mesh_test", false}, {"postgres://admin:mesh_test@localhost/mesh_test", false}} {
		if got := validateTestDSN(tc.dsn) == nil; got != tc.allowed {
			t.Fatalf("DSN allowed=%v want=%v", got, tc.allowed)
		}
	}
}
