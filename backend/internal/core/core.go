package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"time"
)

type M = map[string]any

func S(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func I(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, e := x.Int64()
		if e == nil {
			return n
		}
		r, ok := new(big.Rat).SetString(string(x))
		if ok && r.IsInt() && r.Num().IsInt64() {
			return r.Num().Int64()
		}
		return 0
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
func B(v any) bool { b, _ := v.(bool); return b }
func Obj(v any) M {
	m, _ := v.(map[string]any)
	if m == nil {
		return M{}
	}
	return m
}
func Arr(v any) []M {
	out := []M{}
	switch a := v.(type) {
	case []M:
		return a
	case []any:
		for _, v := range a {
			out = append(out, Obj(v))
		}
	}
	return out
}
func Strings(v any) []string {
	out := []string{}
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		for _, v := range a {
			out = append(out, S(v))
		}
	}
	return out
}
func ID() string {
	b := make([]byte, 12)
	_, e := rand.Read(b)
	if e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func Time(v any) time.Time     { t, _ := time.Parse(time.RFC3339, S(v)); return t }
func Decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(v)
}
func Clone(m M) M { b, _ := json.Marshal(m); var out M; _ = Decode(b, &out); return out }

type Problem struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Extra  M      `json:"-"`
}

func (p *Problem) Error() string { return p.Code + ": " + p.Detail }
func Fail(status int, code, detail string) error {
	return &Problem{Status: status, Code: code, Detail: detail}
}
func Version(m M, body M) error {
	if I(m["version"]) != I(body["expectedVersion"]) {
		return Fail(409, "stale_version", "resource version changed")
	}
	return nil
}

type Actor struct {
	Subject   string   `json:"subject"`
	Roles     []string `json:"roles"`
	PointIDs  []string `json:"pointIds"`
	SellerIDs []string `json:"sellerIds"`
	Service   string   `json:"service,omitempty"`
}

func (a Actor) Has(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}
func Contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func (a Actor) Point(id string) bool  { return a.Has("dispatcher") || Contains(a.PointIDs, id) }
func (a Actor) Seller(id string) bool { return a.Has("dispatcher") || Contains(a.SellerIDs, id) }
func (a Actor) Access(m M) bool {
	if a.Has("dispatcher") {
		return true
	}
	if a.Has("pvz_operator") {
		for _, k := range []string{"pointId", "pickupPointId", "fromNodeId", "toNodeId"} {
			if Contains(a.PointIDs, S(m[k])) {
				return true
			}
		}
		for _, sh := range Arr(m["shipments"]) {
			if a.Access(sh) {
				return true
			}
		}
	}
	return a.Has("seller") && Contains(a.SellerIDs, S(m["sellerId"]))
}

type Command struct {
	Name         string            `json:"name"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Params       map[string]string `json:"params"`
	Query        url.Values        `json:"query"`
	Body         M                 `json:"body"`
	Actor        Actor             `json:"actor"`
	RunID        string            `json:"runId"`
	SimulationID string            `json:"simulationId,omitempty"`
}
type Handler func(*Tx, Command) (M, bool, error)

var handlers []Handler

func Register(h Handler) { handlers = append(handlers, h) }
func Dispatch(tx *Tx, c Command) (M, error) {
	for _, h := range handlers {
		m, ok, e := h(tx, c)
		if ok {
			return m, e
		}
	}
	return nil, Fail(501, "not_implemented", c.Name)
}
func Page(items []M) M { return M{"items": items} }
func CheckAccess(c Command, m M) error {
	if !c.Actor.Access(m) {
		return Fail(403, "forbidden", "resource outside caller scope")
	}
	return nil
}
func GetVisible(tx *Tx, c Command, kind, id string) (M, error) {
	m, e := tx.Get(kind, id)
	if e != nil {
		return nil, e
	}
	return m, CheckAccess(c, m)
}
func FilterVisible(c Command, rows []M) []M {
	out := []M{}
	for _, m := range rows {
		if c.Actor.Access(m) {
			out = append(out, m)
		}
	}
	return out
}
func Background() context.Context { return context.Background() }

// CheckedAdd and CheckedMul keep quantities, slots and minor-unit amounts in int64.
func CheckedAdd(a, b int64) (int64, error) {
	r := a + b
	if (b > 0 && r < a) || (b < 0 && r > a) {
		return 0, Fail(422, "validation_error", "integer aggregate exceeds int64")
	}
	return r, nil
}
func CheckedMul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	const min int64 = -1 << 63
	if (a == min && b == -1) || (b == min && a == -1) {
		return 0, Fail(422, "validation_error", "integer aggregate exceeds int64")
	}
	r := a * b
	if r/b != a {
		return 0, Fail(422, "validation_error", "integer aggregate exceeds int64")
	}
	return r, nil
}
