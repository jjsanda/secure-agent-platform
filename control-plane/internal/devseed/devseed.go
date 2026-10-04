// Package devseed holds the fixed demo tenants and users used by the zero-config
// local profile. The mock OIDC provider issues tokens for these users and the
// control plane seeds the matching tenant rows on startup, so `docker compose up`
// is immediately usable with two genuinely isolated tenants.
package devseed

// Fixed tenant IDs, so the mock IdP's tenant_id claim always matches a seeded row.
const (
	TenantAID = "11111111-1111-1111-1111-111111111111"
	TenantBID = "22222222-2222-2222-2222-222222222222"
)

// Tenant is a demo tenant to seed.
type Tenant struct {
	ID   string
	Slug string
	Name string
}

// Tenants are the demo tenants.
var Tenants = []Tenant{
	{ID: TenantAID, Slug: "tenant-a", Name: "Tenant A"},
	{ID: TenantBID, Slug: "tenant-b", Name: "Tenant B"},
}

// User is a demo identity the mock IdP can authenticate.
type User struct {
	Subject  string
	Password string // dev only — the mock IdP is not a real credential store
	TenantID string
	Email    string
	Name     string
	Roles    []string
}

// Users are the demo identities. Alice administers Tenant A; Bob is a member of
// Tenant B. Logging in as each demonstrates that neither can see the other's data.
var Users = []User{
	{
		Subject: "alice", Password: "alice", TenantID: TenantAID,
		Email: "alice@tenant-a.example", Name: "Alice (Tenant A admin)",
		Roles: []string{"tenant-admin", "member"},
	},
	{
		Subject: "bob", Password: "bob", TenantID: TenantBID,
		Email: "bob@tenant-b.example", Name: "Bob (Tenant B member)",
		Roles: []string{"member"},
	},
}

// UserBySubject returns a demo user by subject.
func UserBySubject(sub string) (User, bool) {
	for _, u := range Users {
		if u.Subject == sub {
			return u, true
		}
	}
	return User{}, false
}
