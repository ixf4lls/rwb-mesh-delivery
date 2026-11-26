package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"meshdelivery/internal/core"
	"net/http"
	"strings"
	"time"
)

type Auth struct {
	UserKey, ServiceKey []byte
	Issuer              string
	Mock bool
	Store *core.Store
}

func (a Auth) verify(raw string, key []byte, audience string) (core.M, error) {
	fail := func() (core.M, error) { return nil, core.Fail(401, "unauthorized", "valid bearer token required") }
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || len(key) < 32 {
		return fail()
	}
	decode := base64.RawURLEncoding.DecodeString
	h, e := decode(parts[0])
	if e != nil {
		return fail()
	}
	var header core.M
	if json.Unmarshal(h, &header) != nil || header["alg"] != "HS256" {
		return fail()
	}
	sig, e := decode(parts[2])
	if e != nil {
		return fail()
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return fail()
	}
	b, e := decode(parts[1])
	if e != nil {
		return fail()
	}
	var c core.M
	if json.Unmarshal(b, &c) != nil {
		return fail()
	}
	now := time.Now().Unix()
	aud := c["aud"] == audience || core.Contains(core.Strings(c["aud"]), audience)
	if core.S(c["sub"]) == "" || core.I(c["exp"]) <= now || core.I(c["nbf"]) > now || !aud || c["iss"] != a.Issuer {
		return fail()
	}
	return c, nil
}
func actor(c core.M) core.Actor {
	return core.Actor{Subject: core.S(c["sub"]), Roles: core.Strings(c["roles"]), PointIDs: core.Strings(c["pvzIds"]), SellerIDs: core.Strings(c["sellerIds"])}
}
func (a Auth) authenticate(r *http.Request, ep endpoint) (core.Actor, error) {
	if a.Mock {
        role := r.Header.Get("X-Demo-Role")
        if role == "" { role = "dispatcher" }
        if role != "dispatcher" && role != "seller" && role != "pvz_operator" {
            return core.Actor{}, core.Fail(400, "invalid_role", "Unknown demo role")
        }
        who := core.Actor{Subject: "demo-user", Roles: []string{role}}
        if role == "dispatcher" { who.Roles = append(who.Roles, "demo_admin") }
        allowed := false
        for _, required := range core.Strings(ep.Definition["x-roles"]) {
            if who.Has(required) { allowed = true }
        }
        if ep.Internal || !allowed { return who, core.Fail(403, "forbidden", "Selected role cannot call operation") }
        rows, err := a.Store.Pool.Query(r.Context(), "SELECT DISTINCT kind, id FROM mesh_records WHERE kind IN ('pickup-points', 'sellers')")
        if err != nil { return who, err }
        defer rows.Close()
        for rows.Next() {
            var kind, id string
            if err := rows.Scan(&kind, &id); err != nil { return who, err }
            if kind == "pickup-points" && role != "seller" { who.PointIDs = append(who.PointIDs, id) }; if kind == "sellers" && role != "pvz_operator" { who.SellerIDs = append(who.SellerIDs, id) }
        }
        return who, rows.Err()
    }
    raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ep.Internal {
		c, e := a.verify(raw, a.UserKey, "mesh-user")
		if e != nil {
			return core.Actor{}, e
		}
		who := actor(c)
		for _, role := range core.Strings(ep.Definition["x-roles"]) {
			if who.Has(role) {
				return who, nil
			}
		}
		return who, core.Fail(403, "forbidden", "role cannot call operation")
	}
	claims, e := a.verify(raw, a.ServiceKey, "mesh-internal")
	if e != nil {
		return core.Actor{}, e
	}
	service := core.S(claims["sub"])
	if !core.Contains(core.Strings(ep.Definition["x-callers"]), service) {
		return core.Actor{}, core.Fail(403, "forbidden", "service cannot call operation")
	}
	if ctx := r.Header.Get("X-User-Context"); ctx != "" {
		c, e := a.verify(ctx, a.UserKey, "mesh-user")
		if e != nil {
			return core.Actor{}, e
		}
		who := actor(c)
		who.Service = service
		if len(ep.UserRoles) > 0 {
			allowed := false
			for _, role := range ep.UserRoles {
				if who.Has(role) {
					allowed = true
				}
			}
			if !allowed {
				return who, core.Fail(403, "forbidden", "delegated role cannot call operation")
			}
		}
		return who, nil
	}
	if service == "gateway" {
		return core.Actor{}, core.Fail(403, "forbidden", "gateway must delegate signed user context")
	}
	return core.Actor{Subject: "service_" + service, Roles: []string{"dispatcher"}, Service: service}, nil
}

// Token is a local development helper. Production tokens are issued by an external provider.
func Token(key []byte, claims core.M) string {
	encode := base64.RawURLEncoding.EncodeToString
	h := encode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	b, _ := json.Marshal(claims)
	body := h + "." + encode(b)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	return body + "." + encode(mac.Sum(nil))
}



