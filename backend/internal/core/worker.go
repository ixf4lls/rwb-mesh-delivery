package core

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"
)

func ProblemBody(e error, trace string) M {
	var p *Problem
	if !errors.As(e, &p) {
		p = &Problem{Status: 500, Code: "internal_error", Detail: "operation failed"}
	}
	m := M{"type": "about:blank", "title": p.Code, "status": p.Status, "code": p.Code, "detail": p.Detail, "traceId": trace}
	if p.Extra != nil {
		m["context"] = p.Extra
	}
	return m
}
func (t *Tx) UseRun(id string) error {
	if id == "" {
		return nil
	}
	if _, e := t.SQL.Exec(t.Context, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); e != nil {
		return e
	}
	r, e := t.GlobalGet("runs", id)
	if e != nil {
		return e
	}
	t.Run = r
	t.Scope = "run:" + id
	t.Dataset = S(r["datasetScope"])
	t.Clock = Time(r["clock"])
	return nil
}
func (s *Store) ProcessOne(ctx context.Context) (bool, error) {
	tx, e := s.Begin(ctx, "")
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	var data []byte
	e = tx.SQL.QueryRow(ctx, `SELECT data FROM mesh_records WHERE scope='global' AND kind='jobs' AND data->>'status'='queued' ORDER BY data->>'createdAt',id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&data)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var job M
	if e = Decode(data, &job); e != nil {
		return false, e
	}
	var command Command
	b, _ := json.Marshal(job["command"])
	if e = Decode(b, &command); e != nil {
		return false, e
	}
	if e = tx.UseRun(command.RunID); e != nil {
		return false, e
	}
	op, e := tx.GlobalGet("operations", S(job["id"]))
	if e != nil {
		return false, e
	}
	if _, e = tx.SQL.Exec(ctx, "SAVEPOINT domain_command"); e != nil {
		return false, e
	}
	result, domainErr := Dispatch(tx, command)
	if domainErr != nil {
		if _, e = tx.SQL.Exec(ctx, "ROLLBACK TO SAVEPOINT domain_command"); e != nil {
			return false, e
		}
		op["status"] = "failed"
		op["error"] = ProblemBody(domainErr, S(op["id"]))
		slog.Warn("operation failed", "operationId", op["id"], "runId", command.RunID, "command", command.Name, "error", domainErr)
	} else {
		op["status"] = "succeeded"
		op["resourceId"] = result["id"]
		if op["resourceId"] == nil {
			op["resourceId"] = command.RunID
		}
		if v := result["resourceId"]; v != nil {
			op["resourceId"] = v
		}
		if e = tx.GlobalPut("results", M{"id": op["id"], "result": result}); e != nil {
			return false, e
		}
	}
	op["updatedAt"] = Stamp(time.Now())
	job["status"] = op["status"]
	if e = tx.GlobalPut("operations", op); e != nil {
		return false, e
	}
	if e = tx.GlobalPut("jobs", job); e != nil {
		return false, e
	}
	return true, tx.Commit()
}
func (s *Store) Work(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, e := s.ProcessOne(ctx)
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("worker retry", "error", e)
		}
		if ok && e == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
