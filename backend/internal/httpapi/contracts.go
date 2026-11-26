package httpapi

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"math"
	"math/big"
	"meshdelivery/internal/core"
	"os"
	"regexp"
	"strings"
	"time"
)

type document struct {
	Paths      map[string]map[string]core.M `yaml:"paths"`
	Components core.M                       `yaml:"components"`
}
type endpoint struct {
	Path, Method, Name string
	Internal           bool
	Spec               *document
	Definition         core.M
	Status             int
	Schema             core.M
	UserRoles          []string
}

func loadContracts(dir string) ([]endpoint, error) {
	out := []endpoint{}
	aliases := map[string]string{}
	userRoles := map[string][]string{}
	for _, name := range []string{"frontend", "internal"} {
		b, e := os.ReadFile(dir + "/" + name + ".yaml")
		if e != nil {
			return nil, e
		}
		var d document
		if e = yaml.Unmarshal(b, &d); e != nil {
			return nil, e
		}
		for path, methods := range d.Paths {
			for method, o := range methods {
				if o["operationId"] == nil {
					continue
				}
				id := core.S(o["operationId"])
				if name == "frontend" {
					userRoles[id] = core.Strings(o["x-roles"])
				}
				if name == "frontend" {
					for _, svc := range core.Strings(o["x-upstream"]) {
						aliases[svc] = id
					}
				} else if a := aliases[id]; a != "" {
					id = a
				}
				ep := endpoint{Path: path, Method: strings.ToUpper(method), Name: id, Internal: name == "internal", Spec: &d, Definition: o, UserRoles: userRoles[id]}
				for code, v := range core.Obj(o["responses"]) {
					if strings.HasPrefix(code, "2") {
						fmt.Sscan(code, &ep.Status)
						ep.Schema = core.Obj(core.Obj(core.Obj(v)["content"])["application/json"])
						ep.Schema = core.Obj(ep.Schema["schema"])
						break
					}
				}
				out = append(out, ep)
			}
		}
	}
	return out, nil
}
func (d *document) resolve(s core.M) core.M {
	if ref := core.S(s["$ref"]); ref != "" {
		parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
		var v any = core.M{"components": d.Components}
		for _, p := range parts {
			v = core.Obj(v)[p]
		}
		return core.Obj(v)
	}
	return s
}
func (d *document) validate(schema core.M, value any, path string) error {
	s := d.resolve(schema)
	bad := func(msg string) error { return core.Fail(400, "validation_error", path+": "+msg) }
	if value == nil {
		if core.B(s["nullable"]) {
			return nil
		}
		return bad("null is not allowed")
	}
	for _, sub := range core.Arr(s["allOf"]) {
		if e := d.validate(sub, value, path); e != nil {
			return e
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		subs := core.Arr(s[key])
		if len(subs) > 0 {
			n := 0
			for _, sub := range subs {
				if d.validate(sub, value, path) == nil {
					n++
				}
			}
			if n == 0 || (key == "oneOf" && n != 1) {
				return bad("invalid " + key)
			}
		}
	}
	if enums, ok := s["enum"].([]any); ok {
		found := false
		for _, v := range enums {
			if fmt.Sprint(v) == fmt.Sprint(value) {
				found = true
			}
		}
		if !found {
			return bad("value outside enum")
		}
	}
	switch s["type"] {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return bad("expected object")
		}
		for _, key := range core.Strings(s["required"]) {
			if _, ok := obj[key]; !ok {
				return bad("missing " + key)
			}
		}
		props := core.Obj(s["properties"])
		for k, v := range obj {
			if sub, ok := props[k]; ok {
				if e := d.validate(core.Obj(sub), v, path+"."+k); e != nil {
					return e
				}
			} else if allow, ok := s["additionalProperties"].(bool); ok && !allow {
				return bad("unknown field " + k)
			}
		}
	case "array":
		a, ok := value.([]any)
		if !ok {
			if rows, yes := value.([]core.M); yes {
				for _, m := range rows {
					a = append(a, m)
				}
			} else {
				return bad("expected array")
			}
		}
		if x, ok := s["minItems"]; ok && len(a) < int(core.I(x)) {
			return bad("too few items")
		}
		if x, ok := s["maxItems"]; ok && len(a) > int(core.I(x)) {
			return bad("too many items")
		}
		for j, v := range a {
			if e := d.validate(core.Obj(s["items"]), v, fmt.Sprintf("%s[%d]", path, j)); e != nil {
				return e
			}
		}
	case "string":
		v, ok := value.(string)
		if !ok {
			return bad("expected string")
		}
		if x, ok := s["minLength"]; ok && len([]rune(v)) < int(core.I(x)) {
			return bad("string too short")
		}
		if x, ok := s["maxLength"]; ok && len([]rune(v)) > int(core.I(x)) {
			return bad("string too long")
		}
		if p := core.S(s["pattern"]); p != "" {
			r, e := regexp.Compile(p)
			if e != nil {
				return e
			}
			if !r.MatchString(v) {
				return bad("invalid pattern")
			}
		}
		if s["format"] == "date-time" {
			if _, e := time.Parse(time.RFC3339, v); e != nil {
				return bad("invalid timestamp")
			}
		}
	case "integer", "number":
		if raw, ok := value.(json.Number); ok && s["type"] == "integer" {
			r, valid := new(big.Rat).SetString(string(raw))
			if !valid || !r.IsInt() || !r.Num().IsInt64() {
				return bad("expected int64")
			}
		}
		var n float64
		switch v := value.(type) {
		case float64:
			n = v
		case int:
			n = float64(v)
		case int64:
			n = float64(v)
		case json.Number:
			var e error
			n, e = v.Float64()
			if e != nil {
				return bad("invalid number")
			}
		default:
			return bad("expected number")
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return bad("invalid number")
		}
		if s["type"] == "integer" && math.Trunc(n) != n {
			return bad("expected integer")
		}
		if x, ok := s["minimum"]; ok && n < float64(core.I(x)) {
			return bad("below minimum")
		}
		if x, ok := s["maximum"]; ok && n > float64(core.I(x)) {
			return bad("above maximum")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return bad("expected boolean")
		}
	}
	return nil
}
