package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ctxKey int

const claimsKey ctxKey = iota

// WithClaims stores verified Claims in the context.
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext retrieves the verified Claims placed by Middleware.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}

// Middleware authenticates a request via its Bearer access token and injects the
// verified Claims into the context. A request without a valid token — or without
// a tenant_id claim — is rejected before it can reach any tenant data.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			unauthorized(w, "missing bearer token")
			return
		}
		claims, err := v.verifyToken(r.Context(), raw)
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}
		if claims.TenantID == "" {
			unauthorized(w, "token is missing the tenant_id claim")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// RequireRole rejects requests whose subject lacks the given role.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := ClaimsFromContext(r.Context())
			if !ok || !c.HasRole(role) {
				forbidden(w, "requires role: "+role)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

func unauthorized(w http.ResponseWriter, detail string) {
	writeJSONError(w, http.StatusUnauthorized, "unauthorized", detail)
}

func forbidden(w http.ResponseWriter, detail string) {
	writeJSONError(w, http.StatusForbidden, "forbidden", detail)
}

func writeJSONError(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "detail": detail})
}
