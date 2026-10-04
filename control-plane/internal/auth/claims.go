// Package auth holds the identity contract and OIDC verification used by the
// control plane. Everything downstream keys off Claims — no code branches on
// which provider (Keycloak or the bundled mock) issued the token.
package auth

import (
	"fmt"

	"github.com/google/uuid"
)

// Claims is the identity contract every OIDC provider must satisfy. The custom
// `tenant_id` claim is projected into the Postgres `app.current_tenant` GUC that
// row-level security filters on, so it is the linchpin of tenant isolation.
type Claims struct {
	Subject  string   `json:"sub"`
	TenantID string   `json:"tenant_id"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	Issuer   string   `json:"iss"`
	Audience string   `json:"aud"`
}

// Platform roles. These are carried in the token's realm/client roles and
// mapped into Claims.Roles by the verifier.
const (
	RolePlatformAdmin = "platform-admin"
	RoleTenantAdmin   = "tenant-admin"
	RoleMember        = "member"
)

// HasRole reports whether the subject carries the given role.
func (c Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// TenantUUID parses the tenant_id claim, which becomes the app.current_tenant GUC.
func (c Claims) TenantUUID() (uuid.UUID, error) {
	id, err := uuid.Parse(c.TenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: tenant_id claim %q is not a uuid: %w", c.TenantID, err)
	}
	return id, nil
}
